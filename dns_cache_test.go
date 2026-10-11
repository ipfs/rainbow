package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestCachedDNS_CloseStopsRefreshGoroutine verifies that Close terminates the
// background refresh goroutine and is safe to call more than once.
//
// The refresh interval is set very long so the goroutine is parked waiting on
// either a tick (which would not arrive for an hour) or the done channel.
// With a long interval, the only way Close can return promptly AND the worker
// can have exited is if Close actually signals the worker to stop. A buggy
// implementation that only calls time.Ticker.Stop lets Close return while the
// goroutine stays blocked on `for range refresher.C` forever.
//
// We assert goroutine termination via the same sync.WaitGroup the production
// code uses (this is a same-package white-box test), which is deterministic
// and does not rely on the flaky runtime.NumGoroutine.
func TestCachedDNS_CloseStopsRefreshGoroutine(t *testing.T) {
	cdns := newCachedDNS(time.Hour)

	closed := make(chan struct{})
	go func() {
		_ = cdns.Close()
		close(closed)
	}()

	select {
	case <-closed:
		// Close returned promptly.
	case <-time.After(5 * time.Second):
		t.Fatal("Close() did not return; refresh goroutine appears blocked on the ticker channel")
	}

	// The worker goroutine must have actually exited by now.
	workerExited := make(chan struct{})
	go func() {
		cdns.wg.Wait()
		close(workerExited)
	}()
	select {
	case <-workerExited:
		// Worker exited: the goroutine leak is fixed.
	case <-time.After(5 * time.Second):
		t.Fatal("refresh goroutine did not exit after Close(); cachedDNS still leaks a goroutine")
	}

	// A second Close must be safe and idempotent (no panic from double close).
	require.NoError(t, cdns.Close())
}
