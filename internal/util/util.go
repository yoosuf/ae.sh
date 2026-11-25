// Package util contains small, dependency-free helpers shared by AE packages.
package util

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Digest returns a stable sha256:<hex> digest of raw bytes.
func Digest(raw []byte) string {
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// DigestFile returns the sha256 digest of a file.
func DigestFile(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return Digest(raw), nil
}

// JSONValue converts any value into a plain JSON-compatible structure using a
// YAML-friendly round trip through encoding/json. It rejects values that cannot
// be represented in JSON (NaN, channels, functions).
func JSONValue(v any) (any, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var out any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\b(sk-[a-z0-9_-]{16,})`),
	regexp.MustCompile(`(?i)\b(akia[0-9a-z]{16})`),
	regexp.MustCompile(`(?i)\b(gh[pousr]_[a-z0-9]{20,})`),
	regexp.MustCompile(`(?i)\b(bearer\s+[a-z0-9._~-]{12,})`),
	regexp.MustCompile(`(?i)\b(xox[baprs]-[a-z0-9-]{10,})`),
}

// Sanitize redacts common credential shapes from untrusted text before it is
// written to evidence or logs. It is a backstop, not a substitute for keeping
// secrets out of inputs.
func Sanitize(text string) string {
	for _, pattern := range secretPatterns {
		text = pattern.ReplaceAllString(text, "[REDACTED]")
	}
	return text
}

// SecretShapeCount counts credential-shaped strings without returning the
// matched text. It is a deterministic backstop for local evidence scans.
func SecretShapeCount(text string) int {
	count := 0
	for _, pattern := range secretPatterns {
		for _, match := range pattern.FindAllString(text, -1) {
			// Canonical AE fixtures use explicit markers to make synthetic
			// shapes safe. They are redacted by Sanitize but are not findings.
			if isSyntheticSecretMatch(match) {
				continue
			}
			count++
		}
	}
	return count
}

func isSyntheticSecretMatch(match string) bool {
	upper := strings.ToUpper(match)
	for _, marker := range []string{"EXAMPLE", "SYNTHETIC", "DUMMY", "FAKE"} {
		if strings.Contains(upper, marker) {
			return true
		}
	}
	return false
}

// SanitizeValue recursively sanitizes strings in a JSON-compatible value.
func SanitizeValue(v any) any {
	switch value := v.(type) {
	case string:
		return Sanitize(value)
	case map[string]any:
		out := make(map[string]any, len(value))
		for k, item := range value {
			out[k] = SanitizeValue(item)
		}
		return out
	case map[any]any:
		out := make(map[string]any, len(value))
		for k, item := range value {
			out[fmt.Sprint(k)] = SanitizeValue(item)
		}
		return out
	case []any:
		out := make([]any, len(value))
		for i, item := range value {
			out[i] = SanitizeValue(item)
		}
		return out
	default:
		return v
	}
}

// Now returns an RFC3339 UTC timestamp, truncated to seconds for stable output.
func Now() string {
	return time.Now().UTC().Truncate(time.Second).Format(time.RFC3339)
}

// NewID returns a short, sortable, collision-resistant identifier.
func NewID(prefix string) string {
	return prefix + "-" + strings.ToLower(hex.EncodeToString([]byte(fmt.Sprintf("%d", time.Now().UnixNano())))) +
		"-" + strings.ToLower(randomSuffix())
}

func randomSuffix() string {
	// Run identifiers are human-facing correlation IDs, not security tokens.
	buf := make([]byte, 6)
	f, err := os.Open("/dev/urandom")
	if err == nil {
		_, _ = f.Read(buf)
		_ = f.Close()
	} else {
		for i := range buf {
			buf[i] = byte(time.Now().UnixNano() >> (i * 8))
		}
	}
	return hex.EncodeToString(buf)
}

// EnsureDir creates dir and its parents with 0o755 permissions.
func EnsureDir(dir string) error {
	return os.MkdirAll(dir, 0o755)
}

// ResolveExistingFile tries the path as given and then relative to each
// ancestor directory up to the filesystem root. This lets commands run from
// subdirectories of the repository.
func ResolveExistingFile(path string) (string, error) {
	if filepath.IsAbs(path) {
		if _, err := os.Stat(path); err == nil {
			return filepath.Clean(path), nil
		}
		return "", fmt.Errorf("file not found: %s", path)
	}
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		candidate := filepath.Join(dir, path)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", fmt.Errorf("file not found: %s", path)
}
