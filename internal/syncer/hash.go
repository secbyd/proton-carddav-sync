package syncer

import (
	"crypto/sha1" //nolint:gosec // sha1 used only for change-detection, not security
)

// sha1OfString returns the SHA-1 digest bytes of s.
// This is used only for contact change-detection (not for security).
func sha1OfString(s string) []byte {
	h := sha1.New() //nolint:gosec
	_, _ = h.Write([]byte(s))
	return h.Sum(nil)
}
