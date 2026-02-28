package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// ---------------------------------------------------------------------------
// Code session types — bridge between the original claudeclient SDK and the
// engine's CodeExecutor interface.
// ---------------------------------------------------------------------------

// codeResult wraps the output of a code execution to match the original SDK pattern.
type codeResult struct {
	text    string
	success bool
	Subtype string
	Errors  []string
}

// Text returns the raw output text.
func (r *codeResult) Text() string { return r.text }

// Success returns whether the execution was successful.
func (r *codeResult) Success() bool { return r.success }

// codeSession wraps a CodeExecutor for the session-based (reuse) pattern.
type codeSession struct {
	executor CodeExecutor
	opts     CodeExecutorOptions
}

// SendPrompt sends a prompt to the code executor.
func (s *codeSession) SendPrompt(ctx context.Context, prompt string) (*codeResult, error) {
	if s.executor == nil {
		return nil, fmt.Errorf("code executor not configured — call engine.SetCodeExecutor()")
	}
	result, err := s.executor.Execute(ctx, prompt, s.opts)
	if err != nil {
		return nil, err
	}
	cr := &codeResult{
		text:    result.Output,
		success: result.Success,
	}
	if !result.Success {
		cr.Subtype = "error"
		cr.Errors = []string{result.Output}
	} else {
		cr.Subtype = "success"
	}
	return cr, nil
}

// Close is a no-op for the interface-based executor.
func (s *codeSession) Close() {}

// Package-level code executor set by the host application.
var codeExecutorInstance CodeExecutor

// SetCodeExecutor registers the code executor.
func SetCodeExecutor(ce CodeExecutor) {
	codeExecutorInstance = ce
}

// newCodeSessionFromOpts creates a new code session with the given options.
func newCodeSessionFromOpts(opts CodeExecutorOptions) *codeSession {
	return &codeSession{
		executor: codeExecutorInstance,
		opts:     opts,
	}
}

// newCodeClientFromOpts creates a one-shot code client (no session reuse).
func newCodeClientFromOpts(opts CodeExecutorOptions) *codeSession {
	return newCodeSessionFromOpts(opts)
}

// runCodeWithOpts runs a prompt through the code executor with options, returning the result.
func runCodeWithOpts(ctx context.Context, prompt string, opts CodeExecutorOptions) (*codeResult, error) {
	if codeExecutorInstance == nil {
		return nil, fmt.Errorf("code executor not configured — call engine.SetCodeExecutor()")
	}
	result, err := codeExecutorInstance.Execute(ctx, prompt, opts)
	if err != nil {
		return nil, err
	}
	cr := &codeResult{
		text:    result.Output,
		success: result.Success,
	}
	if !result.Success {
		cr.Subtype = "error"
		cr.Errors = []string{result.Output}
	} else {
		cr.Subtype = "success"
	}
	return cr, nil
}

// getDataDir returns the data directory for the application.
func getDataDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".connectai"), nil
}
