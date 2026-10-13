package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	glog "github.com/henrocdotnet/grumbler/internal/log"
)

var jsonFenceRe = regexp.MustCompile("(?s)```(?:json)?\\s*\n?(.*?)\\s*```")

// ExtractJSON strips markdown code fences and extracts JSON content.
func ExtractJSON(s string) string {
	s = strings.TrimSpace(s)
	preview := s
	if len(preview) > 80 {
		preview = preview[:80]
	}
	glog.L().Debug("ExtractJSON", "inputLen", len(s), "preview", preview)

	// If already valid JSON, return as-is
	if (strings.HasPrefix(s, "{") || strings.HasPrefix(s, "[")) && json.Valid([]byte(s)) {
		glog.L().Debug("ExtractJSON strategy=raw_json", "resultLen", len(s))
		return s
	}

	// Try extracting from code fences
	if candidate, ok := extractFenced(s); ok {
		glog.L().Debug("ExtractJSON strategy=code_fence", "resultLen", len(candidate))
		return candidate
	}

	// Try to find JSON object/array boundaries
	if candidate, ok := scanJSONBoundary(s); ok {
		glog.L().Debug("ExtractJSON strategy=boundary_scan", "resultLen", len(candidate))
		return candidate
	}

	glog.L().Debug("ExtractJSON strategy=passthrough", "resultLen", len(s))
	return s
}

// extractFenced returns the first code-fenced block if it is valid JSON.
func extractFenced(s string) (string, bool) {
	matches := jsonFenceRe.FindStringSubmatch(s)
	if len(matches) < 2 {
		return "", false
	}
	candidate := strings.TrimSpace(matches[1])
	return candidate, json.Valid([]byte(candidate))
}

// scanJSONBoundary finds the first '{' or '[' and returns the shortest
// balanced, valid JSON value starting there, skipping brackets inside strings.
func scanJSONBoundary(s string) (string, bool) {
	start := strings.IndexAny(s, "{[")
	if start < 0 {
		return "", false
	}
	opener := s[start]
	var closer byte = '}'
	if opener == '[' {
		closer = ']'
	}
	for end := balancedEnd(s, start, opener, closer); end >= 0; end = balancedEnd(s, end+1, opener, closer) {
		candidate := s[start : end+1]
		if json.Valid([]byte(candidate)) {
			return candidate, true
		}
		glog.L().Debug("ExtractJSON boundary_scan candidate invalid", "candidateLen", len(candidate))
	}
	return "", false
}

// balancedEnd scans s from i and returns the index where opener/closer depth
// returns to zero, ignoring brackets inside strings, or -1.
func balancedEnd(s string, i int, opener, closer byte) int {
	depth := 0
	inString := false
	for ; i < len(s); i++ {
		ch := s[i]
		if inString {
			if ch == '\\' {
				i++ // skip escaped char
			} else if ch == '"' {
				inString = false
			}
			continue
		}
		switch ch {
		case '"':
			inString = true
		case opener:
			depth++
		case closer:
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// StripCodeFences removes any wrapping markdown code fences.
func StripCodeFences(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		lines := strings.Split(s, "\n")
		if len(lines) > 2 {
			lines = lines[1:]
		}
		if len(lines) > 0 && strings.HasPrefix(strings.TrimSpace(lines[len(lines)-1]), "```") {
			lines = lines[:len(lines)-1]
		}
		return strings.Join(lines, "\n")
	}
	return s
}

// errEmptyResponse marks a successful call that returned no content, so the
// final error is never a nil-wrapped %!w(<nil>).
var errEmptyResponse = errors.New("empty response from LLM")

// Retry retries a completion call up to n times on failure.
func Retry(ctx context.Context, n int, p Provider, msgs []Message, opts CompletionOpts) (string, error) {
	var lastErr error
	for i := 0; i < n; i++ {
		if i > 0 {
			glog.L().Warn("Retry", "attempt", i+1, "maxAttempts", n, "prevErr", lastErr)
		}
		resp, err := p.Complete(ctx, msgs, opts)
		if err == nil && strings.TrimSpace(resp) != "" {
			return resp, nil
		}
		if err == nil {
			glog.L().Warn("empty response from LLM", "attempt", i+1)
			err = errEmptyResponse
		}
		lastErr = err
		if i < n-1 {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(time.Duration(i+1) * time.Second):
			}
		}
	}
	return "", fmt.Errorf("after %d retries: %w", n, lastErr)
}
