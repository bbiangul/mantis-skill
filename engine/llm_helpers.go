package engine

import (
	"context"
	"fmt"

	"github.com/henomis/lingoose/thread"
)

// ---------------------------------------------------------------------------
// Package-level LLM — thin wrapper around a lingoose-compatible Generate call.
// The original codebase used newLLM() / generateLLM() to create and invoke
// an openai.OpenAI instance directly. In the extracted engine we keep the same
// two-function API but delegate to a pluggable lingooseGenerator.
// ---------------------------------------------------------------------------

// lingooseGenerator is the minimal interface satisfied by any lingoose LLM
// (openai.OpenAI, anthropic.Anthropic, etc.).
type lingooseGenerator interface {
	Generate(ctx context.Context, t *thread.Thread) error
}

// llmInstance is the package-level lingoose LLM set via SetLingooseLLM.
var llmInstance lingooseGenerator

// SetLingooseLLM registers the lingoose LLM instance used by newLLM / generateLLM.
// Typically called once at application startup:
//
//	engine.SetLingooseLLM(openai.New().WithModel(openai.GPT4o))
func SetLingooseLLM(llm lingooseGenerator) {
	llmInstance = llm
}

// newLLM returns the registered lingoose LLM.
// Panics if SetLingooseLLM has not been called.
func newLLM() lingooseGenerator {
	if llmInstance == nil {
		// Return a no-op stub so callers don't crash during init; actual
		// usage will surface the error in generateLLM.
		return nil
	}
	return llmInstance
}

// generateLLM runs generation on the given thread using the provided LLM.
func generateLLM(ctx context.Context, llm lingooseGenerator, t *thread.Thread) error {
	if llm == nil {
		return fmt.Errorf("LLM not configured — call engine.SetLingooseLLM()")
	}
	return llm.Generate(ctx, t)
}
