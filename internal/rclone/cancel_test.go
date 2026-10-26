//go:build linux

package rclone

import "testing"

func TestCancelWhenNothingIsRunning(t *testing.T) {
	jobMu.Lock()
	current = nil
	jobMu.Unlock()
	if _, err := Cancel(); err == nil {
		t.Fatal("expected no transfer")
	}
}
