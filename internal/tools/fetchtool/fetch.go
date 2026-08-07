package fetchtool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/pulseaiclub/phi/internal/tools/tooldef"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/pulseaiclub/phi/internal/llm"
	"github.com/pulseaiclub/phi/internal/util"
)

// =============================================================================
// Constants
// =============================================================================

const (
	MaxURLLength          = 2000
	MaxHTTPContentLength  = 10 * 1024 * 1024
	MaxRedirects          = 10
	DefaultTimeout        = 30
	MaxTimeout            = 120
	MaxCacheEntryBytes    = 512 * 1024
	CacheTTL              = 15 * time.Minute
	llmsProbeTimeout      = 5
	maxMarkdownLines      = 1000
	maxMarkdownChars      = 100_000
	probeMinContentLength = 100
	lowQualityMaxLines    = 10
	lowQualityShortRatio  = 0.7
	maxDocumentLinks      = 20
	feedMaxItems          = 10
	maxDocLinksShown      = 10
)

// =============================================================================
// FetchInput and Definition
// =============================================================================

type FetchInput struct {
	URL     string            `json:"url"`
	Method  string            `json:"method,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    string            `json:"body,omitempty"`
	Raw     bool              `json:"raw,omitempty"`
	Timeout int               `json:"timeout,omitempty"`
}

const fetchDescription = `Fetch a URL and return clean text suitable for reading.

Set raw=true to get the unprocessed HTTP body. HTML is converted to text,
JSON is formatted, and feeds are parsed.`

// FetchTool returns the fetch tool definition + handler.
func FetchTool() tooldef.Tool {
	return tooldef.Tool{
		Definition: llm.ToolDefinition{
			Name:        "fetch",
			Description: fetchDescription,
			Params: &llm.FunctionParameters{
				Type: "object",
				Properties: llm.Object{
					"url": llm.Object{
						"type":        "string",
						"description": "URL to fetch. Example: https://example.com/docs",
					},
					"method": llm.Object{
						"type":        "string",
						"description": "HTTP method: GET, POST, PUT, DELETE, PATCH, HEAD (default: GET)",
						"enum":        []string{"GET", "POST", "PUT", "DELETE", "PATCH", "HEAD"},
					},
					"headers": llm.Object{
						"type":        "object",
						"description": "Request headers as key-value pairs. Example: {\"Accept\":\"application/json\"}",
					},
					"body": llm.Object{
						"type":        "string",
						"description": "Request body for POST/PUT/PATCH. Example: {\"query\":\"panda\"}",
					},
					"raw": llm.Object{
						"type":        "boolean",
						"description": "true to skip HTML processing and return the raw body (default: false)",
					},
					"timeout": llm.Object{
						"type":        "integer",
						"description": "Timeout in seconds, 1-120. Example: 30 (default: 30)",
					},
				},
				Required: []string{"url"},
			},
			Readable: true,
		},
		DetailFromArgs: func(input json.RawMessage) string {
			var in FetchInput
			_ = json.Unmarshal(input, &in)
			method := in.Method
			if method == "" {
				method = http.MethodGet
			}
			detail := method
			if in.Raw {
				detail += " raw"
			}
			if strings.TrimSpace(in.Body) != "" {
				detail += " (body)"
			}
			detail += " " + strings.TrimSpace(in.URL)
			return detail
		},
		Run: Fetch,
	}
}

// =============================================================================
// Fetch main entry point
// =============================================================================

// Fetch fetches content from a URL and returns processed text.
func Fetch(ctx context.Context, input json.RawMessage) (tooldef.Result, error) {
	fetchInput := FetchInput{}
	if err := json.Unmarshal(input, &fetchInput); err != nil {
		return tooldef.Result{}, fmt.Errorf("failed to parse fetch arguments: %w", err)
	}

	if strings.TrimSpace(fetchInput.URL) == "" {
		return tooldef.Result{}, errors.New("url is required")
	}

	parsedURL, err := validateAndNormalizeURL(fetchInput.URL)
	if err != nil {
		return tooldef.Result{}, err
	}

	method := strings.ToUpper(strings.TrimSpace(fetchInput.Method))
	if method == "" {
		method = http.MethodGet
	}

	body := strings.TrimSpace(fetchInput.Body)
	if body != "" && method != http.MethodPost && method != http.MethodPut && method != http.MethodPatch {
		return tooldef.Result{}, fmt.Errorf("body is only allowed for POST, PUT, or PATCH requests (got %s)", method)
	}

	timeoutSec := fetchInput.Timeout
	if timeoutSec <= 0 {
		timeoutSec = DefaultTimeout
	} else if timeoutSec > MaxTimeout {
		timeoutSec = MaxTimeout
	}
	timeout := time.Duration(timeoutSec) * time.Second

	// Cache key: only for GET requests without custom headers/body (idempotent reads)
	cacheKey := ""
	canCache := method == http.MethodGet && len(fetchInput.Headers) == 0 && body == ""
	if canCache {
		rawSuffix := ""
		if fetchInput.Raw {
			rawSuffix = "::raw"
		}
		cacheKey = parsedURL.String() + rawSuffix
		if cachedContent, ok := fetchCacheGet(cacheKey); ok {
			return tooldef.Result{Content: cachedContent, Detail: parsedURL.String(), Output: cachedContent}, nil
		}
	}

	// Pipeline
	var result string
	var notes []string
	var methodUsed string

	if fetchInput.Raw {
		// Raw mode: bypass all processing
		result, notes, methodUsed, err = doRawFetch(ctx, parsedURL.String(), method, body, fetchInput.Headers, timeout)
	} else if method == http.MethodGet && body == "" {
		// GET without body: use content processing pipeline
		result, notes, methodUsed, err = doProcessedFetch(ctx, parsedURL.String(), timeout)
	} else {
		// Non-GET or with body: use raw fetch but with processed output
		result, notes, methodUsed, err = doRawFetch(ctx, parsedURL.String(), method, body, fetchInput.Headers, timeout)
	}

	if err != nil {
		return tooldef.Result{}, err
	}

	// Build final output with metadata header
	var sb strings.Builder
	fmt.Fprintf(&sb, "URL: %s\n", parsedURL.String())
	fmt.Fprintf(&sb, "Method: %s\n", methodUsed)
	if len(notes) > 0 {
		fmt.Fprintf(&sb, "Notes: %s\n", strings.Join(notes, "; "))
	}
	sb.WriteString("---\n\n")
	sb.WriteString(result)

	final := sb.String()

	// Cache the result for GET requests
	if canCache {
		fetchCacheSet(cacheKey, final)
	}

	return tooldef.Result{Content: final, Detail: parsedURL.String(), Output: final}, nil
}

// =============================================================================
// Content Processing Pipeline
// =============================================================================

// doProcessedFetch runs the multi-stage content pipeline for a GET request.
func doProcessedFetch(ctx context.Context, reqURL string, timeout time.Duration) (content string, notes []string, method string, err error) {
	// Stage 1: Content negotiation (Accept: text/markdown)
	negResult, negOK := tryContentNegotiation(ctx, reqURL, timeout)
	if negOK {
		return negResult, nil, "content-negotiation", nil
	}

	// Stage 2: llms.txt probing
	llmResult, llmOK := tryLlmEndpoints(ctx, reqURL)
	if llmOK {
		return llmResult.content, []string{fmt.Sprintf("llms.txt: %s", llmResult.endpoint)}, "llms.txt", nil
	}

	// Stage 3: Main HTTP fetch
	statusCode, finalURL, rawBody, respContentType, respNotes, err := doHTTPFetch(ctx, reqURL, timeout)
	if err != nil {
		return "", nil, "", err
	}
	notes = respNotes

	if statusCode < 200 || statusCode >= 300 {
		return "", notes, fmt.Sprintf("http-%d", statusCode), fmt.Errorf("HTTP %d", statusCode)
	}

	if len(rawBody) == 0 {
		return "(empty response)", notes, "empty", nil
	}

	mime := normalizeMime(respContentType)

	// Stage 4: Handle binary / non-text content
	if !isTextContent(mime, rawBody) {
		notes = append(notes, fmt.Sprintf("binary content (%s)", mime))
		return fmt.Sprintf("[Binary content: %d bytes, Content-Type: %s]", len(rawBody), mime), notes, "binary", nil
	}

	bodyStr := string(rawBody)

	// Stage 5: JSON formatting
	if isJSON(mime) {
		formatted := formatJSON(bodyStr)
		return formatted, notes, "json", nil
	}

	// Stage 6: Feed detection and parsing
	if isFeed(mime, bodyStr) {
		parsed := parseFeedToText(bodyStr)
		return parsed, notes, "feed", nil
	}

	// Stage 7: Plain text / markdown — return as-is
	if isPlainText(mime) && !looksLikeHTML(bodyStr) {
		return bodyStr, notes, "text", nil
	}

	// Stage 8: HTML processing
	if isHTML(mime) || looksLikeHTML(bodyStr) {
		// 8a: Try .md suffix convention
		mdResult, mdOK := tryMdSuffix(ctx, finalURL, timeout)
		if mdOK {
			notes = append(notes, "found .md suffix version")
			return mdResult, notes, "md-suffix", nil
		}

		// 8b: Convert HTML to clean text
		cleaned := htmlToText(bodyStr, finalURL)

		// 8c: Low quality detection
		if isLowQualityContent(cleaned) {
			notes = append(notes, "low quality output — page may require JavaScript")

			// Try to extract document links as fallback
			docLinks := extractDocumentLinks(bodyStr, finalURL)
			if len(docLinks) > 0 {
				notes = append(notes, fmt.Sprintf("found %d document link(s)", len(docLinks)))
				cleaned += "\n\n[Document links found on page:]\n"
				for i, link := range docLinks {
					if i >= maxDocLinksShown {
						cleaned += fmt.Sprintf("... and %d more\n", len(docLinks)-maxDocLinksShown)
						break
					}
					cleaned += fmt.Sprintf("- %s\n", link)
				}
			}
		}

		return cleaned, notes, "html", nil
	}

	// Fallback: return raw body
	return bodyStr, notes, "raw", nil
}

// doRawFetch performs a raw HTTP fetch with full method/headers/body support.
func doRawFetch(
	ctx context.Context,
	reqURL, method, body string,
	headers map[string]string,
	timeout time.Duration,
) (content string, notes []string, methodUsed string, err error) {
	var bodyReader io.Reader
	if body != "" {
		bodyReader = strings.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, reqURL, bodyReader)
	if err != nil {
		return "", nil, "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; Panda/1.0)")
	req.Header.Set("Accept", "text/plain, text/html, application/json, */*")

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	ctxTimeout, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	client := newFetchClient(MaxRedirects, true, nil)
	req = req.WithContext(ctxTimeout)

	resp, err := client.Do(req)
	if err != nil {
		if ctxTimeout.Err() != nil {
			return "", nil, "", fmt.Errorf("request timed out or canceled: %w", ctxTimeout.Err())
		}
		return "", nil, "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	limitedReader := io.LimitReader(resp.Body, int64(MaxHTTPContentLength)+1)
	rawBody, err := io.ReadAll(limitedReader)
	if err != nil {
		return "", nil, "", fmt.Errorf("failed to read response body: %w", err)
	}

	truncated := len(rawBody) > MaxHTTPContentLength
	if truncated {
		rawBody = rawBody[:MaxHTTPContentLength]
	}

	statusText := http.StatusText(resp.StatusCode)
	if statusText == "" {
		statusText = "Unknown"
	}

	contentType := resp.Header.Get("Content-Type")
	contentLength := resp.ContentLength
	if contentLength < 0 {
		contentLength = int64(len(rawBody))
	}

	var notesList []string

	var sb strings.Builder
	fmt.Fprintf(&sb, "HTTP %d %s\n", resp.StatusCode, statusText)
	fmt.Fprintf(&sb, "Content-Type: %s\n", contentType)
	sb.WriteString(fmt.Sprintf("Content-Length: %d\n", contentLength))
	sb.WriteString("---\n")

	if len(rawBody) > 0 {
		contentStr := string(rawBody)
		if isTextContent(contentType, rawBody) {
			sb.WriteString(contentStr)
		} else {
			sb.WriteString(fmt.Sprintf("[Binary content: %d bytes, Content-Type: %s]", len(rawBody), contentType))
		}
	} else {
		sb.WriteString("(empty response body)")
	}

	if truncated {
		truncMsg := fmt.Sprintf("\n\n[Response truncated at %d bytes]", MaxHTTPContentLength)
		sb.WriteString(truncMsg)
		notesList = append(notesList, fmt.Sprintf("truncated at %d bytes", MaxHTTPContentLength))
	}

	return sb.String(), notesList, method, nil
}

// =============================================================================
// HTTP Fetch Helper
// =============================================================================

// newFetchClient builds an HTTP client that follows up to maxRedirects redirects.
// When blockCrossHost is true, redirects to a different host are rejected.
// When finalURL is non-nil, it is set to the URL after following redirects.
func newFetchClient(maxRedirects int, blockCrossHost bool, finalURL *string) *http.Client {
	return &http.Client{
		Transport: util.SharedHTTPTransport(),
		Timeout:   0,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if finalURL != nil {
				*finalURL = req.URL.String()
			}
			if len(via) >= maxRedirects {
				return fmt.Errorf("too many redirects (exceeded %d)", maxRedirects)
			}
			if blockCrossHost && !isSameHost(via[0].URL.Hostname(), req.URL.Hostname()) {
				return fmt.Errorf(
					"redirect blocked: target host %q differs from original host %q",
					req.URL.Hostname(), via[0].URL.Hostname(),
				)
			}
			return nil
		},
	}
}

// doHTTPFetch performs a GET request and returns status, final URL, body, and metadata.
func doHTTPFetch(ctx context.Context, reqURL string, timeout time.Duration) (statusCode int, finalURL string, body []byte, contentType string, notes []string, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return 0, "", nil, "", nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; Panda/1.0)")
	req.Header.Set("Accept", "text/html, application/xhtml+xml, application/xml;q=0.9, */*;q=0.8")

	ctxTimeout, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var finalURLTracked string
	client := newFetchClient(MaxRedirects, true, &finalURLTracked)
	req = req.WithContext(ctxTimeout)

	resp, err := client.Do(req)
	if err != nil {
		if ctxTimeout.Err() != nil {
			return 0, "", nil, "", nil, fmt.Errorf("request timed out or canceled: %w", ctxTimeout.Err())
		}
		return 0, "", nil, "", nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if finalURLTracked == "" {
		finalURLTracked = resp.Request.URL.String()
	}

	limitedReader := io.LimitReader(resp.Body, int64(MaxHTTPContentLength)+1)
	rawBody, err := io.ReadAll(limitedReader)
	if err != nil {
		return 0, "", nil, "", nil, fmt.Errorf("failed to read response body: %w", err)
	}

	truncated := len(rawBody) > MaxHTTPContentLength
	if truncated {
		rawBody = rawBody[:MaxHTTPContentLength]
	}

	var respNotes []string
	if truncated {
		respNotes = append(respNotes, fmt.Sprintf("body truncated at %d bytes", MaxHTTPContentLength))
	}

	return resp.StatusCode, finalURLTracked, rawBody, resp.Header.Get("Content-Type"), respNotes, nil
}

// =============================================================================
// Content Negotiation — try Accept: text/markdown
// =============================================================================

func tryContentNegotiation(ctx context.Context, reqURL string, timeout time.Duration) (string, bool) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return "", false
	}
	req.Header.Set("Accept", "text/markdown, text/plain;q=0.9, text/html;q=0.8")
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; Panda/1.0)")

	ctxTimeout, cancel := context.WithTimeout(ctx, minDuration(timeout, 10*time.Second))
	defer cancel()

	client := newFetchClient(MaxRedirects, false, nil)
	req = req.WithContext(ctxTimeout)

	resp, err := client.Do(req)
	if err != nil {
		return "", false
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", false
	}

	mime := normalizeMime(resp.Header.Get("Content-Type"))
	if !strings.Contains(mime, "markdown") && mime != "text/plain" {
		return "", false
	}

	limitedReader := io.LimitReader(resp.Body, int64(MaxHTTPContentLength))
	body, err := io.ReadAll(limitedReader)
	if err != nil || len(body) < probeMinContentLength {
		return "", false
	}

	content := string(body)
	if looksLikeHTML(content) {
		return "", false
	}

	return content, true
}

// =============================================================================
// llms.txt probing
// =============================================================================

type llmResult struct {
	content  string
	endpoint string
}

func tryLlmEndpoints(ctx context.Context, reqURL string) (llmResult, bool) {
	parsed, err := url.Parse(reqURL)
	if err != nil {
		return llmResult{}, false
	}

	endpoints := buildLlmCandidates(parsed)
	if len(endpoints) == 0 {
		return llmResult{}, false
	}

	for _, ep := range endpoints {
		probeCtx, cancel := context.WithTimeout(ctx, llmsProbeTimeout*time.Second)
		result, ok := tryFetchLlmEndpoint(probeCtx, ep)
		cancel()
		if ok {
			return llmResult{content: result, endpoint: ep}, true
		}
	}

	return llmResult{}, false
}

func buildLlmCandidates(parsed *url.URL) []string {
	var candidates []string
	origin := fmt.Sprintf("%s://%s", parsed.Scheme, parsed.Host)

	if parsed.Path == "" || parsed.Path == "/" {
		candidates = append(
			candidates,
			origin+"/.well-known/llms.txt",
			origin+"/llms.txt",
		)
		return candidates
	}

	// Try scoped paths for non-root URLs
	trimmedPath := strings.TrimRight(parsed.Path, "/")
	segments := strings.Split(strings.TrimLeft(trimmedPath, "/"), "/")

	// Determine scope depth
	scopeDepth := len(segments)
	if !strings.HasSuffix(parsed.Path, "/") && len(segments) > 1 {
		scopeDepth = len(segments) - 1
	}
	if scopeDepth < 1 {
		scopeDepth = 1
	}

	for depth := scopeDepth; depth >= 1; depth-- {
		scope := "/" + strings.Join(segments[:depth], "/") + "/"
		candidates = append(
			candidates,
			origin+scope+"llms.txt",
		)
	}

	return candidates
}

func tryFetchLlmEndpoint(ctx context.Context, endpoint string) (string, bool) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", false
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; Panda/1.0)")

	client := newFetchClient(3, false, nil)

	resp, err := client.Do(req)
	if err != nil {
		return "", false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", false
	}

	mime := normalizeMime(resp.Header.Get("Content-Type"))
	if !strings.Contains(mime, "text/") && !strings.Contains(mime, "markdown") && mime != "application/octet-stream" {
		return "", false
	}

	limitedReader := io.LimitReader(resp.Body, int64(512*1024))
	body, err := io.ReadAll(limitedReader)
	if err != nil || len(body) < probeMinContentLength {
		return "", false
	}

	content := string(body)
	if looksLikeHTML(content) {
		return "", false
	}

	return content, true
}

// =============================================================================
// .md suffix probing
// =============================================================================

func tryMdSuffix(ctx context.Context, reqURL string, timeout time.Duration) (string, bool) {
	parsed, err := url.Parse(reqURL)
	if err != nil {
		return "", false
	}

	var candidates []string
	pathname := parsed.Path

	if strings.HasSuffix(pathname, "/") {
		candidates = append(
			candidates,
			fmt.Sprintf("%s://%s%sindex.html.md", parsed.Scheme, parsed.Host, pathname),
		)
	} else {
		candidates = append(
			candidates,
			fmt.Sprintf("%s://%s%s.md", parsed.Scheme, parsed.Host, pathname),
		)
	}

	for _, candidate := range candidates {
		probeCtx, cancel := context.WithTimeout(ctx, minDuration(timeout, 5*time.Second))
		result, ok := tryFetchLlmEndpoint(probeCtx, candidate)
		cancel()
		if ok && len(result) > probeMinContentLength && !looksLikeHTML(result) {
			return result, true
		}
	}

	return "", false
}

// =============================================================================
// HTML → Clean Text Converter
// =============================================================================

// htmlToText converts HTML to clean readable text without external dependencies.
func htmlToText(html, pageURL string) string {
	s := html

	// Remove doctype, comments, script, style, nav, footer, header, svg, etc.
	s = removeTagContent(s, "script")
	s = removeTagContent(s, "style")
	s = removeTagContent(s, "svg")
	s = removeTagContent(s, "noscript")
	s = removeTagContent(s, "nav")
	s = removeTagContent(s, "footer")
	s = removeTagContent(s, "header")
	s = removeTagContent(s, "aside")
	s = removeComments(s)

	// Replace <a href="..."> with [text](url) links
	s = convertLinks(s)

	// Replace <img> with alt text
	s = convertImages(s)

	// Remove all remaining HTML tags
	s = stripHTMLTags(s)

	// Decode common HTML entities
	s = decodeHTMLEntities(s)

	// Clean up whitespace
	s = cleanWhitespace(s)

	// Truncate if too long
	if len(s) > maxMarkdownChars {
		s = s[:maxMarkdownChars] + "\n\n[Content truncated due to length...]"
	}

	return s
}

func removeTagContent(s, tag string) string {
	result := ""
	i := 0
	for i < len(s) {
		// Find opening tag
		openStart := indexOfTag(s[i:], "<"+tag)
		if openStart == -1 {
			result += s[i:]
			break
		}
		openStart += i
		result += s[i:openStart]

		// Find end of opening tag
		openEnd := strings.IndexByte(s[openStart:], '>')
		if openEnd == -1 {
			result += s[openStart:]
			break
		}
		openEnd += openStart + 1

		// Check if self-closing
		if openEnd >= 2 && s[openEnd-2] == '/' {
			i = openEnd
			continue
		}

		// Find closing tag
		closeTag := "</" + tag + ">"
		closeStart := indexOfTag(s[openEnd:], closeTag)
		if closeStart == -1 {
			i = openEnd
			continue
		}
		closeEnd := closeStart + len(closeTag) + openEnd

		i = closeEnd
	}
	return result
}

func removeComments(s string) string {
	result := ""
	for {
		start := strings.Index(s, "<!--")
		if start == -1 {
			result += s
			break
		}
		result += s[:start]
		end := strings.Index(s[start+4:], "-->")
		if end == -1 {
			break
		}
		s = s[start+4+end+3:]
	}
	return result
}

func convertLinks(s string) string {
	result := ""
	i := 0
	for i < len(s) {
		aStart := indexOfTag(s[i:], "<a")
		if aStart == -1 {
			result += s[i:]
			break
		}
		aStart += i
		result += s[i:aStart]

		// Extract href
		tagEnd := strings.IndexByte(s[aStart:], '>')
		if tagEnd == -1 {
			result += s[aStart:]
			break
		}
		tag := s[aStart : aStart+tagEnd+1]
		href := extractAttribute(tag, "href")

		// Find content and closing tag
		innerStart := aStart + tagEnd + 1
		closeTag := "</a>"
		aEnd := indexOfTag(s[innerStart:], closeTag)
		if aEnd == -1 {
			result += s[innerStart:]
			break
		}
		linkText := stripHTMLTags(s[innerStart : innerStart+aEnd])
		linkText = cleanWhitespace(linkText)
		linkText = strings.TrimSpace(linkText)

		if href != "" && linkText != "" && linkText != href {
			result += fmt.Sprintf("[%s](%s)", linkText, href)
		} else if linkText != "" {
			result += linkText
		} else if href != "" {
			result += href
		}

		i = innerStart + aEnd + len(closeTag)
	}
	return result
}

func convertImages(s string) string {
	result := ""
	i := 0
	for i < len(s) {
		imgStart := indexOfTag(s[i:], "<img")
		if imgStart == -1 {
			result += s[i:]
			break
		}
		imgStart += i
		result += s[i:imgStart]

		tagEnd := strings.IndexByte(s[imgStart:], '>')
		if tagEnd == -1 {
			result += s[imgStart:]
			break
		}
		tag := s[imgStart : imgStart+tagEnd+1]
		alt := extractAttribute(tag, "alt")
		src := extractAttribute(tag, "src")

		if alt != "" {
			result += fmt.Sprintf("[Image: %s]", alt)
		} else if src != "" {
			result += fmt.Sprintf("[Image: %s]", src)
		} else {
			result += "[Image]"
		}

		i = imgStart + tagEnd + 1
	}
	return result
}

func stripHTMLTags(s string) string {
	result := ""
	i := 0
	for i < len(s) {
		if s[i] == '<' {
			end := strings.IndexByte(s[i+1:], '>')
			if end == -1 {
				result += s[i:]
				break
			}
			// Check if it's </tag> — add newline for safety
			if i+1 < len(s) && s[i+1] == '/' {
				result += "\n"
			}
			i += end + 2
		} else {
			result += string(s[i])
			i++
		}
	}
	return result
}

func decodeHTMLEntities(s string) string {
	entities := map[string]string{
		"&amp;": "&", "&lt;": "<", "&gt;": ">", "&quot;": "\"",
		"&apos;": "'", "&nbsp;": " ", "&mdash;": "—", "&ndash;": "–",
		"&ldquo;": "\"", "&rdquo;": "\"", "&lsquo;": "'", "&rsquo;": "'",
		"&laquo;": "«", "&raquo;": "»", "&bull;": "•", "&hellip;": "...",
		"&copy;": "(c)", "&reg;": "(r)", "&trade;": "(tm)",
		"&euro;": "€", "&pound;": "£", "&yen;": "¥",
		"&#39;": "'", "&#x27;": "'", "&#x2F;": "/",
	}
	for entity, replacement := range entities {
		s = strings.ReplaceAll(s, entity, replacement)
	}
	// Decode numeric entities (simplified)
	s = decodeNumericEntities(s)
	return s
}

func decodeNumericEntities(s string) string {
	result := ""
	i := 0
	for i < len(s) {
		amp := strings.IndexByte(s[i:], '&')
		if amp == -1 {
			result += s[i:]
			break
		}
		amp += i
		result += s[i:amp]

		semi := strings.IndexByte(s[amp:], ';')
		if semi == -1 {
			result += s[amp:]
			break
		}
		semi += amp
		entity := s[amp : semi+1]

		if strings.HasPrefix(entity, "&#x") || strings.HasPrefix(entity, "&#X") {
			// Hex entity
			hexStr := entity[3 : len(entity)-1]
			var val rune
		hexLoop:
			for _, c := range hexStr {
				val *= 16
				switch {
				case c >= '0' && c <= '9':
					val += c - '0'
				case c >= 'a' && c <= 'f':
					val += c - 'a' + 10
				case c >= 'A' && c <= 'F':
					val += c - 'A' + 10
				default:
					val = 0
					break hexLoop
				}
			}
			if val > 0 {
				result += string(val)
			} else {
				result += entity
			}
		} else if strings.HasPrefix(entity, "&#") {
			// Decimal entity
			numStr := entity[2 : len(entity)-1]
			var val int
			for _, c := range numStr {
				if c >= '0' && c <= '9' {
					val = val*10 + int(c-'0')
				} else {
					val = 0
					break
				}
			}
			if val > 0 {
				result += string(rune(val))
			} else {
				result += entity
			}
		} else {
			result += entity
		}

		i = semi + 1
	}
	return result
}

func cleanWhitespace(s string) string {
	// Replace multiple newlines with double newline
	result := ""
	lines := strings.Split(s, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		// Collapse internal whitespace
		collapsed := collapseSpaces(trimmed)
		if result != "" {
			result += "\n"
		}
		result += collapsed
	}

	// Ensure max lines
	allLines := strings.Split(result, "\n")
	if len(allLines) > maxMarkdownLines {
		allLines = allLines[:maxMarkdownLines]
		allLines = append(allLines, fmt.Sprintf("... truncated at %d lines", maxMarkdownLines))
	}
	return strings.Join(allLines, "\n")
}

func collapseSpaces(s string) string {
	result := ""
	prevSpace := false
	for _, r := range s {
		if r == ' ' || r == '\t' {
			if !prevSpace {
				result += " "
				prevSpace = true
			}
		} else {
			result += string(r)
			prevSpace = false
		}
	}
	return strings.TrimSpace(result)
}

// =============================================================================
// Feed parsing (RSS/Atom)
// =============================================================================

func parseFeedToText(content string) string {
	content = removeComments(content)

	// Try RSS
	title := extractFeedTag(content, "<channel>", "</channel>", "title")
	items := extractFeedItems(content, "item", "title", "link", "description", "pubDate")
	if title != "" || len(items) > 0 {
		return renderFeed(title, items, "item")
	}

	// Try Atom
	atomTitle := extractFeedTag(content, "<feed", "</feed>", "title")
	entries := extractFeedItems(content, "entry", "title", "link", "summary", "updated")
	if atomTitle != "" || len(entries) > 0 {
		return renderFeed(atomTitle, entries, "entry")
	}

	return content
}

// renderFeed renders a parsed RSS or Atom feed as clean markdown.
func renderFeed(title string, items []feedItem, itemName string) string {
	var sb strings.Builder
	if title != "" {
		fmt.Fprintf(&sb, "# %s\n\n", cleanFeedText(title))
	}
	for i, item := range items {
		if i >= feedMaxItems {
			fmt.Fprintf(&sb, "\n... and %d more %ss\n", len(items)-feedMaxItems, itemName)
			break
		}
		if item.title != "" {
			fmt.Fprintf(&sb, "## %s\n", cleanFeedText(item.title))
		}
		if item.pubDate != "" {
			fmt.Fprintf(&sb, "*%s*\n\n", cleanFeedText(item.pubDate))
		}
		if item.description != "" {
			desc := cleanFeedText(item.description)
			if len(desc) > 500 {
				desc = desc[:500] + "..."
			}
			sb.WriteString(desc + "\n\n")
		}
		if item.link != "" {
			fmt.Fprintf(&sb, "[Read more](%s)\n\n", item.link)
		}
		sb.WriteString("---\n\n")
	}
	return sb.String()
}

type feedItem struct {
	title       string
	link        string
	description string
	pubDate     string
}

func extractFeedTag(content, containerStart, containerEnd, tag string) string {
	start := strings.Index(content, containerStart)
	if start == -1 {
		return ""
	}
	end := strings.Index(content[start:], containerEnd)
	if end == -1 {
		end = len(content) - start
	}
	container := content[start : start+end]

	tagStart := indexOfTag(container, "<"+tag+">")
	if tagStart == -1 {
		tagStart = indexOfTag(container, "<"+tag+" ")
	}
	if tagStart == -1 {
		return ""
	}
	tagContentEnd := indexOfTag(container[tagStart:], "</"+tag+">")
	if tagContentEnd == -1 {
		return ""
	}
	innerStart := strings.IndexByte(container[tagStart:], '>')
	if innerStart == -1 {
		return ""
	}
	return container[tagStart+innerStart+1 : tagStart+tagContentEnd]
}

func extractFeedItems(content, itemTag, titleTag, linkTag, descTag, dateTag string) []feedItem {
	var items []feedItem
	searchFrom := 0
	for {
		itemStart := indexOfTag(content[searchFrom:], "<"+itemTag+">")
		if itemStart == -1 {
			itemStart = indexOfTag(content[searchFrom:], "<"+itemTag+" ")
		}
		if itemStart == -1 {
			break
		}
		itemStart += searchFrom

		itemEnd := indexOfTag(content[itemStart+1:], "</"+itemTag+">")
		if itemEnd == -1 {
			break
		}
		itemEnd += itemStart + 1 + len(itemTag) + 3

		itemContent := content[itemStart:itemEnd]

		title := extractSimpleTag(itemContent, titleTag)
		link := extractLinkTag(itemContent, linkTag)
		desc := extractSimpleTag(itemContent, descTag)
		date := extractSimpleTag(itemContent, dateTag)

		items = append(items, feedItem{title, link, desc, date})

		searchFrom = itemEnd
	}
	return items
}

func extractSimpleTag(content, tag string) string {
	start := indexOfTag(content, "<"+tag+">")
	if start == -1 {
		start = indexOfTag(content, "<"+tag+" ")
	}
	if start == -1 {
		return ""
	}
	close := strings.IndexByte(content[start:], '>')
	if close == -1 {
		return ""
	}
	innerStart := start + close + 1
	end := strings.Index(content[innerStart:], "</"+tag+">")
	if end == -1 {
		return ""
	}
	return content[innerStart : innerStart+end]
}

func extractLinkTag(content, tag string) string {
	start := indexOfTag(content, "<"+tag+" ")
	if start == -1 {
		start = indexOfTag(content, "<"+tag+">")
		if start == -1 {
			return ""
		}
		// <link>text</link> for RSS
		close := strings.Index(content[start:], "</"+tag+">")
		if close == -1 {
			return ""
		}
		innerStart := start + len(tag) + 2
		return strings.TrimSpace(content[innerStart : start+close])
	}
	// <link href="..." /> for Atom
	tagEnd := strings.IndexByte(content[start:], '>')
	if tagEnd == -1 {
		return ""
	}
	tagContent := content[start : start+tagEnd+1]
	return extractAttribute(tagContent, "href")
}

func cleanFeedText(text string) string {
	text = strings.ReplaceAll(text, "<![CDATA[", "")
	text = strings.ReplaceAll(text, "]]>", "")
	text = stripHTMLTags(text)
	text = decodeHTMLEntities(text)
	return strings.TrimSpace(text)
}

// =============================================================================
// MIME and Content-Type Helpers
// =============================================================================

func normalizeMime(contentType string) string {
	idx := strings.IndexByte(contentType, ';')
	if idx != -1 {
		contentType = contentType[:idx]
	}
	return strings.TrimSpace(strings.ToLower(contentType))
}

func isHTML(mime string) bool {
	return strings.Contains(mime, "html") || strings.Contains(mime, "xhtml")
}

func isJSON(mime string) bool {
	return mime == "application/json" || strings.HasSuffix(mime, "+json")
}

func isFeed(mime string, content string) bool {
	if strings.Contains(mime, "rss") || strings.Contains(mime, "atom") || strings.Contains(mime, "feed") {
		return true
	}
	// XML content that looks like RSS/Atom
	if strings.Contains(mime, "xml") || mime == "" {
		lower := strings.ToLower(content)
		return strings.Contains(lower, "<rss") || strings.Contains(lower, "<feed")
	}
	return false
}

func isPlainText(mime string) bool {
	return mime == "text/plain" || strings.Contains(mime, "markdown")
}

// =============================================================================
// Content quality detection
// =============================================================================

func isLowQualityContent(content string) bool {
	lower := strings.ToLower(content)

	// JS-gated indicators
	jsIndicators := []string{
		"enable javascript",
		"javascript required",
		"turn on javascript",
		"please enable javascript",
		"browser not supported",
	}
	if len(content) < 1024 {
		for _, indicator := range jsIndicators {
			if strings.Contains(lower, indicator) {
				return true
			}
		}
	}

	// Mostly navigation lines (short lines indicating menu/nav)
	lines := strings.Split(content, "\n")
	var nonEmptyLines []string
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			nonEmptyLines = append(nonEmptyLines, line)
		}
	}
	if len(nonEmptyLines) > lowQualityMaxLines {
		shortCount := 0
		for _, line := range nonEmptyLines {
			if len(strings.TrimSpace(line)) < 40 {
				shortCount++
			}
		}
		if float64(shortCount)/float64(len(nonEmptyLines)) > lowQualityShortRatio {
			return true
		}
	}

	return false
}

// =============================================================================
// Document link extraction from HTML
// =============================================================================

func extractDocumentLinks(html, baseURL string) []string {
	var links []string
	seen := make(map[string]bool)

	if len(html) > 512*1024 {
		html = html[:512*1024]
	}

	i := 0
	for i < len(html) {
		aStart := indexOfTag(html[i:], "<a")
		if aStart == -1 || len(links) >= maxDocumentLinks {
			break
		}
		aStart += i

		tagEnd := strings.IndexByte(html[aStart:], '>')
		if tagEnd == -1 {
			break
		}
		tag := html[aStart : aStart+tagEnd+1]
		href := extractAttribute(tag, "href")
		if href == "" {
			i = aStart + tagEnd + 1
			continue
		}

		ext := ""
		if dot := strings.LastIndexByte(href, '.'); dot != -1 {
			ext = strings.ToLower(href[dot:])
			if idx := strings.IndexAny(ext, "/?#"); idx != -1 {
				ext = ext[:idx]
			}
		}

		convertibleExts := map[string]bool{
			".pdf": true, ".doc": true, ".docx": true, ".ppt": true, ".pptx": true,
			".xls": true, ".xlsx": true, ".rtf": true, ".epub": true, ".ipynb": true,
		}
		if !convertibleExts[ext] {
			i = aStart + tagEnd + 1
			continue
		}

		resolved := href
		if !strings.HasPrefix(href, "http://") && !strings.HasPrefix(href, "https://") {
			base, err := url.Parse(baseURL)
			if err != nil {
				i = aStart + tagEnd + 1
				continue
			}
			rel, err := url.Parse(href)
			if err != nil {
				i = aStart + tagEnd + 1
				continue
			}
			resolved = base.ResolveReference(rel).String()
		}

		if seen[resolved] {
			i = aStart + tagEnd + 1
			continue
		}
		seen[resolved] = true
		links = append(links, resolved)

		i = aStart + tagEnd + 1
	}

	return links
}

// =============================================================================
// JSON formatting
// =============================================================================

func formatJSON(content string) string {
	var parsed any
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		return content
	}
	formatted, err := json.MarshalIndent(parsed, "", "  ")
	if err != nil {
		return content
	}
	return string(formatted)
}

// =============================================================================
// Cache
// =============================================================================

type cacheEntry struct {
	content   string
	expiresAt time.Time
}

var (
	fetchCache   = make(map[string]cacheEntry)
	fetchCacheMu sync.RWMutex
)

func fetchCacheGet(key string) (string, bool) {
	fetchCacheMu.RLock()
	defer fetchCacheMu.RUnlock()

	entry, ok := fetchCache[key]
	if !ok {
		return "", false
	}
	if time.Now().After(entry.expiresAt) {
		delete(fetchCache, key)
		return "", false
	}
	return entry.content, true
}

func fetchCacheSet(key, content string) {
	if len(content) > MaxCacheEntryBytes {
		return
	}
	fetchCacheMu.Lock()
	defer fetchCacheMu.Unlock()

	// Evict oldest entries if cache grows too large
	if len(fetchCache) > 100 {
		now := time.Now()
		for k, v := range fetchCache {
			if now.After(v.expiresAt) {
				delete(fetchCache, k)
			}
		}
		// Hard limit
		if len(fetchCache) > 200 {
			newCache := make(map[string]cacheEntry, 100)
			count := 0
			for k, v := range fetchCache {
				newCache[k] = v
				count++
				if count >= 100 {
					break
				}
			}
			fetchCache = newCache
		}
	}

	fetchCache[key] = cacheEntry{
		content:   content,
		expiresAt: time.Now().Add(CacheTTL),
	}
}

// =============================================================================
// String/HTML helpers
// =============================================================================

func indexOfTag(s, tag string) int {
	lower := strings.ToLower(s)
	tagLower := strings.ToLower(tag)
	return strings.Index(lower, tagLower)
}

func extractAttribute(tag, attr string) string {
	// Build pattern: attr="value" or attr='value' or attr=value
	lower := strings.ToLower(tag)
	attrLower := strings.ToLower(attr)

	// Try "value"
	patterns := []string{
		attrLower + `="`,
		attrLower + `='`,
		attrLower + `=`,
	}
	for _, pattern := range patterns {
		start := strings.Index(lower, pattern)
		if start == -1 {
			continue
		}
		valueStart := start + len(pattern)
		if valueStart >= len(tag) {
			continue
		}
		quote := tag[valueStart-1]
		var end int
		if quote == '"' || quote == '\'' {
			end = strings.IndexByte(tag[valueStart:], quote)
			if end == -1 {
				continue
			}
			return tag[valueStart : valueStart+end]
		}
		// Unquoted value
		end = strings.IndexAny(tag[valueStart:], " >/")
		if end == -1 {
			return tag[valueStart:]
		}
		return tag[valueStart : valueStart+end]
	}
	return ""
}

func looksLikeHTML(content string) bool {
	trimmed := strings.TrimLeftFunc(content, unicode.IsSpace)
	lower := strings.ToLower(trimmed)
	return strings.HasPrefix(lower, "<!doctype") ||
		strings.HasPrefix(lower, "<html") ||
		strings.HasPrefix(lower, "<head") ||
		strings.HasPrefix(lower, "<body")
}

func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}

// =============================================================================
// URL validation, redirect checks, content type detection
// =============================================================================

// validateAndNormalizeURL validates the URL and upgrades HTTP to HTTPS.
func validateAndNormalizeURL(rawURL string) (*url.URL, error) {
	if len(rawURL) > MaxURLLength {
		return nil, fmt.Errorf("URL exceeds maximum length of %d characters", MaxURLLength)
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL %q: %w", rawURL, err)
	}

	if parsed.Scheme == "" {
		return nil, fmt.Errorf("URL must have a scheme (http:// or https://)")
	}

	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("unsupported URL scheme %q (only http and https are allowed)", parsed.Scheme)
	}

	if parsed.Scheme == "http" {
		parsed.Scheme = "https"
	}

	if parsed.User != nil {
		return nil, fmt.Errorf("URL must not contain username or password")
	}

	hostname := parsed.Hostname()
	parts := strings.Split(hostname, ".")
	if len(parts) < 2 {
		return nil, fmt.Errorf("hostname %q must contain a dot. Use a fully qualified domain name like example.com", hostname)
	}

	return parsed, nil
}

// isSameHost checks if two hostnames are the same host (allowing www. prefix/suffix variations).
func isSameHost(a, b string) bool {
	a = strings.TrimPrefix(a, "www.")
	b = strings.TrimPrefix(b, "www.")
	return strings.EqualFold(a, b)
}

// isTextContent returns true if the response content appears to be text rather than binary.
func isTextContent(contentType string, body []byte) bool {
	ct := strings.ToLower(contentType)

	if strings.Contains(ct, "text/") ||
		strings.Contains(ct, "application/json") ||
		strings.Contains(ct, "application/xml") ||
		strings.Contains(ct, "application/xhtml") ||
		strings.Contains(ct, "application/javascript") ||
		strings.Contains(ct, "application/ecmascript") ||
		strings.Contains(ct, "application/yaml") ||
		strings.Contains(ct, "application/toml") ||
		strings.Contains(ct, "application/x-www-form-urlencoded") ||
		strings.Contains(ct, "+xml") ||
		strings.Contains(ct, "+json") {
		return true
	}

	if strings.Contains(ct, "application/octet-stream") ||
		strings.Contains(ct, "application/pdf") ||
		strings.Contains(ct, "application/zip") ||
		strings.Contains(ct, "application/gzip") ||
		strings.Contains(ct, "image/") ||
		strings.Contains(ct, "audio/") ||
		strings.Contains(ct, "video/") ||
		strings.Contains(ct, "font/") {
		return false
	}

	if len(body) > 0 && len(body) < 1024 {
		for _, b := range body {
			if b == 0 {
				return false
			}
		}
	}

	return true
}
