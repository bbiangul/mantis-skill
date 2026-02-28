package engine

// ---------------------------------------------------------------------------
// Package-level logger — shared across all files in the engine package.
// Set via SetLogger(); defaults to a no-op logger so callers never panic.
// ---------------------------------------------------------------------------

// noopLogger discards all log output. Used as the default so that calling
// logger methods before SetLogger() is safe (no nil-pointer panic).
type noopLogger struct{}

func (noopLogger) Debugf(string, ...interface{})              {}
func (noopLogger) Infof(string, ...interface{})               {}
func (noopLogger) Warnf(string, ...interface{})               {}
func (noopLogger) Errorf(string, ...interface{})              {}
func (n noopLogger) WithError(error) Logger                   { return n }
func (n noopLogger) WithFields(map[string]interface{}) Logger { return n }

// logger is the package-level Logger instance. Defaults to noopLogger so
// that all log calls are safe even if SetLogger is never called.
var logger Logger = noopLogger{}

// SetLogger configures the package-level logger used by the engine package.
func SetLogger(l Logger) {
	if l == nil {
		logger = noopLogger{}
		return
	}
	logger = l
}
