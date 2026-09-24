package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type transcriptionError struct {
	code      string
	retryable bool
}

func (e *transcriptionError) Error() string { return e.code }
func dgError(code string, retry bool) error { return &transcriptionError{code, retry} }

// Fixed endpoints prevent admin configuration from becoming an SSRF/credential exfiltration route.
func endpoint(c configuration) string {
	host := "api.deepgram.com"
	if c.DeepgramRegion == "eu" {
		host = "api.eu.deepgram.com"
	}
	q := url.Values{"model": {c.Model}, "smart_format": {"true"}, "punctuate": {"true"}, "mip_opt_out": {"true"}}
	if c.Language == "auto" {
		q.Set("detect_language", "true")
	} else {
		q.Set("language", c.Language)
	}
	return "https://" + host + "/v1/listen?" + q.Encode()
}

func transcribe(ctx context.Context, client *http.Client, c configuration, data []byte, mime string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint(c), bytes.NewReader(data))
	if err != nil {
		return "", dgError("request", false)
	}
	req.Header.Set("Authorization", "Token "+c.DeepgramAPIKey)
	req.Header.Set("Content-Type", mime)
	res, err := client.Do(req)
	if err != nil {
		return "", dgError("network", true)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		// Provider bodies and request headers may contain sensitive material. Never log them.
		return "", dgError(fmt.Sprintf("deepgram_%d", res.StatusCode), res.StatusCode == 429 || res.StatusCode >= 500)
	}
	const limit = 2 * 1024 * 1024
	b, err := io.ReadAll(io.LimitReader(res.Body, limit+1))
	if err != nil {
		return "", dgError("response_read", true)
	}
	if len(b) > limit {
		return "", dgError("response_too_large", false)
	}
	var result struct {
		Results struct {
			Channels []struct {
				Alternatives []struct {
					Transcript string `json:"transcript"`
				} `json:"alternatives"`
			} `json:"channels"`
		} `json:"results"`
	}
	if json.Unmarshal(b, &result) != nil || len(result.Results.Channels) == 0 || len(result.Results.Channels[0].Alternatives) == 0 {
		return "", dgError("invalid_response", true)
	}
	return strings.TrimSpace(result.Results.Channels[0].Alternatives[0].Transcript), nil
}

// Treat recognized speech as text. In particular, spoken @channel must never notify a channel.
func safeTranscript(s string) string {
	s = strings.ReplaceAll(s, "@", "@\u200b")
	s = strings.NewReplacer("\\", "\\\\", "`", "\\`", "*", "\\*", "_", "\\_", "[", "\\[", "]", "\\]", "<", "&lt;", ">", "&gt;", "#", "\\#", "!", "\\!", "|", "\\|").Replace(s)
	// Mattermost's default limit is 16383 runes. Leave room for header and link.
	r := []rune(s)
	if len(r) > 14000 {
		s = string(r[:14000]) + "\n\n[Текст сокращён; полная запись — в аудиофайле.]"
	}
	return s
}
