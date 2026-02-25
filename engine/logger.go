package engine

// ---------------------------------------------------------------------------
// Package-level logger — shared across all files in the engine package.
// Set via SetLogger(); nil-safe (all log calls are silently skipped when nil).
// ---------------------------------------------------------------------------

// logger is the package-level Logger instance. It is nil until SetLogger is called.
var logger Logger

// SetLogger configures the package-level logger used by the engine package.
// If not called, all log calls are silently skipped.
func SetLogger(l Logger) {
	logger = l
}
