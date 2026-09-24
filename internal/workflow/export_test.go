package workflow

// SetBeforeCommitHook lets tests force a failure at the very end of a
// transition transaction, after actions have been enqueued.
func SetBeforeCommitHook(e *Engine, f func() error) { e.beforeCommit = f }
