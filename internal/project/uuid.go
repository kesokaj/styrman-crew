package project

import (
	"crypto/rand"
	"fmt"
)

// NewUUID returns a random v4 UUID (stdlib-only). A crypto/rand failure is
// an error, not a fallback id: the old "fallback-<nanos>" shape was
// quarantined by the store's own loader on the next restart, i.e. silent
// data loss (audit 2026-09-13).
func NewUUID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("cannot generate a project id: %w", err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}
