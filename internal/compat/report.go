package compat

import (
	"fmt"
	"sync"
)

// Reporter collects migration warnings: things the conversion changed or
// dropped that a human should know about. It is safe for concurrent use and
// all methods are no-ops on a nil *Reporter.
type Reporter struct {
	mu   sync.Mutex
	seen map[string]bool
	msgs []string
}

// NewReporter returns an empty Reporter.
func NewReporter() *Reporter {
	return &Reporter{seen: map[string]bool{}}
}

// Warnf records a warning; identical messages are recorded once.
func (r *Reporter) Warnf(format string, args ...any) {
	if r == nil {
		return
	}
	msg := fmt.Sprintf(format, args...)
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.seen[msg] {
		return
	}
	r.seen[msg] = true
	r.msgs = append(r.msgs, msg)
}

// Warnings returns the recorded warnings in order of first occurrence.
func (r *Reporter) Warnings() []string {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.msgs...)
}
