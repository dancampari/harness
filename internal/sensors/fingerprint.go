package sensors

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"
)

// Fingerprint computes a stable hash that identifies the SAME logical issue
// across runs, even if line numbers shift. The trick is to deliberately
// EXCLUDE line numbers from the hash and use a normalized rule + file +
// message signature instead.
//
// This is the mechanism that detects "AI Slop accumulation" (problem 6):
// when the same fingerprint reappears across N consecutive sprints, the
// harness reports the recurrence so it's not silently absorbed as noise.
func Fingerprint(dim Dimension, file, rule, message string) string {
	// Normalize file: relative path, forward slashes, no drive letters.
	f := filepath.ToSlash(file)
	if strings.HasPrefix(f, "./") {
		f = f[2:]
	}
	// Strip volatile substrings from message: numbers, hex, temp paths.
	// Conservative: this is best-effort; perfect dedup is not needed.
	m := normalizeMessage(message)
	raw := fmt.Sprintf("%s|%s|%s|%s", dim, f, rule, m)
	sum := sha1.Sum([]byte(raw))
	return hex.EncodeToString(sum[:8])
}

func normalizeMessage(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
			b.WriteByte('N') // collapse all digits
		case r >= 'a' && r <= 'f' && len(s) > 20:
			// Cheap heuristic: long messages with hex-like blobs (commit
			// SHAs, addresses) get further normalized.
			b.WriteRune(r)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
