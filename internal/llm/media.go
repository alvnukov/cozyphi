package llm

import (
	"encoding/base64"
	"fmt"
	"regexp"
	"strings"
)

// Media is an inline binary attachment (currently an image) sent to a model.
// Data holds base64-encoded bytes, or a full data: URL.
type Media struct {
	MediaType string `json:"media_type"`
	Data      string `json:"data"`
	Filename  string `json:"filename,omitempty"`
}

// ValidatedMedia is a validated media payload plus its canonical data URL.
type ValidatedMedia struct {
	MIME    string
	Base64  string
	DataURL string
	Bytes   []byte
}

// ImageMIMEs are the image media types sent inline to providers.
var ImageMIMEs = map[string]bool{
	"image/png":  true,
	"image/jpeg": true,
	"image/gif":  true,
	"image/webp": true,
}

const (
	// MaxMediaDecodedBytes caps the decoded media size (mirrors opencode).
	MaxMediaDecodedBytes = 20 * 1024 * 1024
	// MaxMediaEncodedBytes caps the base64 text length (mirrors opencode).
	MaxMediaEncodedBytes = 28 * 1024 * 1024
)

// dataURLRE matches an inline data URL and captures its media type and base64.
var dataURLRE = regexp.MustCompile(`^data:([^;,]+);base64,([A-Za-z0-9+/]*={0,2})$`)

// base64Pattern matches a canonical padded base64 string.
var base64Pattern = regexp.MustCompile(`^(?:[A-Za-z0-9+/]{4})*(?:[A-Za-z0-9+/]{2}==|[A-Za-z0-9+/]{3}=)?$`)

// ValidateMedia validates an image attachment and returns its canonical
// base64 representation plus a data URL for providers.
func ValidateMedia(m Media) (ValidatedMedia, error) {
	mime := strings.ToLower(m.MediaType)
	if !ImageMIMEs[mime] {
		return ValidatedMedia{}, fmt.Errorf("media does not support media type %s", mime)
	}
	if m.Data == "" {
		return ValidatedMedia{}, fmt.Errorf("media must contain valid base64")
	}

	var base64Data string
	if strings.HasPrefix(m.Data, "data:") {
		match := dataURLRE.FindStringSubmatch(m.Data)
		if match == nil {
			return ValidatedMedia{}, fmt.Errorf("media data URL must contain valid base64")
		}
		if strings.ToLower(match[1]) != mime {
			return ValidatedMedia{}, fmt.Errorf("media type %s does not match data URL type %s", mime, match[1])
		}
		base64Data = match[2]
	} else {
		base64Data = m.Data
	}

	if len(base64Data) > MaxMediaEncodedBytes {
		return ValidatedMedia{}, fmt.Errorf("media exceeds the %d byte encoded limit", MaxMediaEncodedBytes)
	}
	if len(base64Data) == 0 || len(base64Data)%4 != 0 || !base64Pattern.MatchString(base64Data) {
		return ValidatedMedia{}, fmt.Errorf("media must contain valid base64")
	}
	decoded, err := base64.StdEncoding.DecodeString(base64Data)
	if err != nil {
		return ValidatedMedia{}, fmt.Errorf("media must contain valid base64")
	}
	if len(decoded) > MaxMediaDecodedBytes {
		return ValidatedMedia{}, fmt.Errorf("media exceeds the %d byte decoded limit", MaxMediaDecodedBytes)
	}
	canonical := base64.StdEncoding.EncodeToString(decoded)
	if canonical != base64Data {
		return ValidatedMedia{}, fmt.Errorf("media must contain canonical base64")
	}

	return ValidatedMedia{
		MIME:    mime,
		Base64:  canonical,
		DataURL: "data:" + mime + ";base64," + canonical,
		Bytes:   decoded,
	}, nil
}
