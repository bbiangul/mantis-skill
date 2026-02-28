package engine

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// MemoryGeneratorInterface defines the contract for creating enriched memories from execution context.
type MemoryGeneratorInterface interface {
	GenerateMemory(ctx context.Context, executionData ExecutionMemoryData) (string, error)
}

// ExecutionMemoryData contains all the data needed to generate a memory.
type ExecutionMemoryData struct {
	EventKey        string
	MessageID       string
	ClientID        string
	FunctionName    string
	ToolName        string
	Inputs          string
	Output          string
	Error           error
	Timestamp       time.Time
	StepRationale   string
	UserMessage     string
	ContextForAgent string
}

// LLMMemoryGenerator implements MemoryGeneratorInterface.
// In the OSS version this generates a structured summary without LLM calls.
// Host applications can replace this with an LLM-backed implementation via
// SetMemoryGenerator.
type LLMMemoryGenerator struct{}

// package-level generator; can be overridden by the host.
var memoryGen MemoryGeneratorInterface = &LLMMemoryGenerator{}

// SetMemoryGenerator allows host applications to inject an LLM-backed generator.
func SetMemoryGenerator(g MemoryGeneratorInterface) {
	if g != nil {
		memoryGen = g
	}
}

// NewMemoryGenerator returns the current memory generator.
func NewMemoryGenerator() MemoryGeneratorInterface {
	return memoryGen
}

// GenerateMemory creates a structured memory chunk from execution data.
// The default implementation produces a human-readable summary without requiring
// an LLM call. Host applications that need richer memories should call
// SetMemoryGenerator with an LLM-backed implementation.
func (g *LLMMemoryGenerator) GenerateMemory(_ context.Context, data ExecutionMemoryData) (string, error) {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("[%s] Function %s (tool: %s) executed.\n",
		data.Timestamp.Format(time.RFC3339), data.FunctionName, data.ToolName))

	if data.StepRationale != "" {
		b.WriteString(fmt.Sprintf("Rationale: %s\n", data.StepRationale))
	}
	if data.UserMessage != "" {
		b.WriteString(fmt.Sprintf("User message: %s\n", data.UserMessage))
	}
	if data.Inputs != "" {
		b.WriteString(fmt.Sprintf("Inputs: %s\n", data.Inputs))
	}
	if data.Output != "" {
		// Truncate long outputs for memory
		output := data.Output
		if len(output) > 2000 {
			output = output[:2000] + "... (truncated)"
		}
		b.WriteString(fmt.Sprintf("Output: %s\n", output))
	}
	if data.Error != nil {
		b.WriteString(fmt.Sprintf("Error: %s\n", data.Error.Error()))
	}

	return strings.TrimSpace(b.String()), nil
}

// getMemoryGenerationSystemPrompt returns the system prompt for LLM-backed memory generation.
// Exported so host applications can use it when providing their own generator.
func GetMemoryGenerationSystemPrompt() string {
	return fmt.Sprintf(`Today is %s.
You are a Memory Generation Agent responsible for creating rich, contextual memory chunks from AI system execution data.

Your task is to transform raw execution data into meaningful, searchable memories that capture:

1. **What happened**: The core action/function that was executed
2. **Why it happened**: The user intent and step rationale that led to this execution
3. **How it happened**: The technical details of the execution (inputs, outputs, errors)
4. **When it happened**: Temporal context and timing
5. **Who was involved**: User, client, and system context
6. **Broader context**: The overall workflow, conversation, and environmental context
7. All the important data. Do not discard useful info

Memory Generation Guidelines:

- **Rich Context**: Include all relevant context variables to make the memory self-contained and meaningful
- **Natural Language**: Write in clear, natural language that would be understandable to both AI systems and humans
- **Searchable**: Include key terms and concepts that would be useful for future retrieval
- **Causal Relationships**: Explain why this action was taken and how it relates to the user's goals
- **Outcome Focus**: Clearly state what was accomplished or what went wrong
- **Temporal Awareness**: Include timing and sequence information
- **Error Context**: If there were errors, explain the context and potential impact
- **Use only the provided data**: Do not create or infer any information that is not present in the execution data

Format your memory as a coherent narrative that tells the complete story of this execution step within its broader context.
`, time.Now().Format("Monday, January 2, 2006"))
}
