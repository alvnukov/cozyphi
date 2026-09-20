package llm

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// requestBindingFingerprintKey keeps endpoint credentials out of the display
// fingerprint while preserving exact route invalidation. Its process lifetime
// deliberately prevents a preflight verdict from surviving a restart.
var requestBindingFingerprintKey = func() []byte {
	key := make([]byte, sha256.Size)
	if _, err := rand.Read(key); err != nil {
		return nil
	}
	return key
}()

// RequestBindingFingerprint identifies the effective recipient and request
// configuration without disclosing credentials. It is a process-bound, keyed
// invalidation token: callers may compare it for equality, but cannot use it
// as an offline oracle for secret endpoint components.
// An empty result means the effective configuration could not be encoded or
// the process could not create the key needed to protect it.
func (c ModelConfig) RequestBindingFingerprint() string {
	projection := struct {
		Name               string       `json:"name"`
		RequestModel       string       `json:"request_model"`
		ProviderID         string       `json:"provider"`
		Protocol           Protocol     `json:"protocol"`
		BaseURL            string       `json:"base_url"`
		ConnectionIdentity string       `json:"connection_identity"`
		MaxOutputTokens    int          `json:"max_output_tokens"`
		ReasoningEffort    string       `json:"reasoning_effort"`
		Options            ModelOptions `json:"options"`
	}{
		Name:               c.Name,
		RequestModel:       c.RequestModel(),
		ProviderID:         c.ProviderID,
		Protocol:           c.Protocol,
		BaseURL:            c.BaseURL,
		ConnectionIdentity: c.ConnectionIdentity,
		MaxOutputTokens:    c.MaxOutputTokens,
		ReasoningEffort:    c.EffectiveReasoningEffort(),
		Options:            c.EffectiveOptions(),
	}
	data, err := json.Marshal(projection)
	if err != nil {
		return ""
	}
	if len(requestBindingFingerprintKey) == 0 {
		return ""
	}
	digest := hmac.New(sha256.New, requestBindingFingerprintKey)
	_, _ = digest.Write(data)
	return hex.EncodeToString(digest.Sum(nil))
}
