// Package githubrelease provides shared helpers for GitHub Releases API and asset downloads.
package githubrelease

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/pulseaiclub/phi/internal/util"
)

const apiVersion = "2022-11-28"

// Release is metadata from GET /repos/{owner}/{repo}/releases/latest.
type Release struct {
	TagName string
	HTMLURL string
}

// FetchLatest queries the latest published release for owner/repo.
func FetchLatest(ctx context.Context, repo string) (Release, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Release{}, fmt.Errorf("create request for %q: %w", repo, err)
	}
	req.Header.Set("accept", "application/vnd.github+json")
	req.Header.Set("x-github-api-version", apiVersion)
	if tok := os.Getenv("GITHUB_TOKEN"); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}

	resp, err := util.DefaultHTTPClient().Do(req)
	if err != nil {
		return Release{}, fmt.Errorf("fetch latest release from %q: %w", repo, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		// GitHub returns 404 both for missing repos and for repos with no
		// published releases. For public pulseaiclub/phi the latter is common
		// before the first tag-triggered GoReleaser run.
		return Release{}, fmt.Errorf("no published release for %s (publish one with ./scripts/bump.sh vX.Y.Z && git push --follow-tags)", repo)
	}
	if resp.StatusCode != http.StatusOK {
		return Release{}, fmt.Errorf("github api %s: status %d", repo, resp.StatusCode)
	}

	var body struct {
		TagName string `json:"tag_name"`
		HTMLURL string `json:"html_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return Release{}, fmt.Errorf("decode release response from %q: %w", repo, err)
	}
	return Release{TagName: body.TagName, HTMLURL: body.HTMLURL}, nil
}

// TagVersion returns the tag without a leading v/V prefix (for tool asset names).
func TagVersion(tag string) string {
	if len(tag) > 0 && (tag[0] == 'v' || tag[0] == 'V') {
		return tag[1:]
	}
	return tag
}

// GetLatestVersion is like FetchLatest but returns the version string without a v prefix.
func GetLatestVersion(ctx context.Context, repo string) (string, error) {
	rel, err := FetchLatest(ctx, repo)
	if err != nil {
		return "", err
	}
	return TagVersion(rel.TagName), nil
}

// DownloadBaseURL converts a release tag page URL to the release download root.
func DownloadBaseURL(htmlURL string) string {
	base := strings.TrimSuffix(htmlURL, "/")
	return strings.Replace(base, "/releases/tag/", "/releases/download/", 1)
}