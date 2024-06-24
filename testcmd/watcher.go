package testcmd

import "context"

// StdoutWatcher is a helper for watching the stdout of a command for a
// specific string. It is used by Runner to watch for specific output from
// the commands. The Signal channel is signaled when the match string is found.
type StdoutWatcher struct {
	match  string
	signal chan struct{}
}

func NewStdoutWatcher(match string) StdoutWatcher {
	return StdoutWatcher{
		match:  match,
		signal: make(chan struct{}, 1),
	}
}

// Wait waits for the watcher to be signaled for the the context to be
// canceled. If the context is canceled then the context error is returned.
func (w StdoutWatcher) Wait(ctx context.Context) error {
	select {
	case <-w.signal:
	case <-ctx.Done():
		return ctx.Err()
	}
	return nil
}
