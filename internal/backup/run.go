package backup

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
)

var errCancelled = errors.New("backup cancelled")

var (
	runMu sync.Mutex
	runs  = map[string]context.CancelFunc{}
	// dead remembers a cancel that arrived before the backup process started.
	dead = map[string]struct{}{}
)

func beginRun(id string) (context.Context, func(), error) {
	ctx, cancel := context.WithCancel(context.Background())
	id = strings.TrimSpace(id)
	if id == "" {
		return ctx, cancel, nil
	}
	runMu.Lock()
	defer runMu.Unlock()
	if _, stopped := dead[id]; stopped {
		delete(dead, id)
		cancel()
		return nil, nil, errCancelled
	}
	if _, exists := runs[id]; exists {
		cancel()
		return nil, nil, fmt.Errorf("backup is already running")
	}
	runs[id] = cancel
	return ctx, func() {
		runMu.Lock()
		delete(runs, id)
		runMu.Unlock()
		cancel()
	}, nil
}

// CancelRun stops the backup whose id was passed to the agent.
// A cancel that arrives before the process starts is remembered.
func CancelRun(id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("backup is not running")
	}
	runMu.Lock()
	cancel := runs[id]
	if cancel == nil {
		dead[id] = struct{}{}
		runMu.Unlock()
		return nil
	}
	runMu.Unlock()
	cancel()
	return nil
}
