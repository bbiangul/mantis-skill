package models

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/bbiangul/mantis-skill/types"
	"github.com/bbiangul/mantis-skill/engine/sanitizer"
	"github.com/bbiangul/mantis-skill/skill"
	"github.com/henomis/lingoose/thread"
)

// ExecutionHistoryEntry represents a single tool execution step
type ExecutionHistoryEntry struct {
	ToolName  string
	Rationale string
	Result    string
	Error     error
	Duration  time.Duration
	Timestamp time.Time
}

// CoordinatorState keeps track of the state during a workflow execution
type CoordinatorState struct {
	UserMessage                           string
	OriginalMessage                       string
	UpdatedAt                             *time.Time
	ConversationContext                   string
	ToolResults                           map[string]string
	ExecutedTools                         []string
	ExecutionHistory                      []ExecutionHistoryEntry
	StartTime                             time.Time
	ConversationThread                    *thread.Thread
	ActiveWorkflow                        *types.Workflow
	CurrentStepIndex                      int
	ValidationRetryPerFunc                map[string]int // Track retry attempts per function to prevent infinite loops
	TaskID                                string
	LastFunctionThatTriggeredAskToContext string // Tracks the last function that triggered the askToContext guidance
	SelfFixRestartCount                   int    // Track self-fix restart attempts to prevent infinite loops
	TestModeStepIndex                     int    // Track current step index in test mode execution plan

	// Prompt injection protection
	FenceNonce         string                        // Per-session nonce for fence delimiters
	SanitizedFunctions map[string]*skill.SanitizeConfig // Function name → sanitize config (only functions with sanitize enabled)

	// Async execution state
	asyncResults []*AsyncResult // all async jobs for this workflow
	asyncMu      sync.Mutex     // protects the asyncResults slice
}

// AsyncResult tracks a single async job. The goroutine writes Result/Error/Duration
// BEFORE close(Done). The main goroutine reads AFTER <-Done. Go's memory model
// guarantees a receive from a closed channel happens-after the close, so no mutex
// is needed on individual fields.
type AsyncResult struct {
	ToolName  string
	Rationale string
	Result    string        // written by goroutine BEFORE close(Done)
	Error     error         // written by goroutine BEFORE close(Done)
	Duration  time.Duration // written by goroutine BEFORE close(Done)
	Done      chan struct{} // closed when goroutine completes
}

// NewCoordinatorState creates a new CoordinatorState
func NewCoordinatorState() *CoordinatorState {
	return &CoordinatorState{
		ToolResults:            make(map[string]string),
		ExecutedTools:          []string{},
		ExecutionHistory:       []ExecutionHistoryEntry{},
		StartTime:              time.Now(),
		ActiveWorkflow:         nil,
		CurrentStepIndex:       0,
		ValidationRetryPerFunc: make(map[string]int),
		FenceNonce:             sanitizer.GenerateNonce(),
		SanitizedFunctions:     make(map[string]*skill.SanitizeConfig),
	}
}

// Reset clears the state for a new workflow
func (s *CoordinatorState) Reset(userMessage string) {
	s.UserMessage = userMessage
	s.OriginalMessage = userMessage
	s.UpdatedAt = nil
	s.ToolResults = make(map[string]string)
	s.ExecutedTools = []string{}
	s.ExecutionHistory = []ExecutionHistoryEntry{}
	s.StartTime = time.Now()
	s.ConversationThread = nil
	s.ActiveWorkflow = nil
	s.CurrentStepIndex = 0
	s.ValidationRetryPerFunc = make(map[string]int)
	s.LastFunctionThatTriggeredAskToContext = ""
	s.TestModeStepIndex = 0
	s.FenceNonce = sanitizer.GenerateNonce()
	s.SanitizedFunctions = make(map[string]*skill.SanitizeConfig)
	s.asyncResults = nil
}

// UpdateUserMessage updates the user message during workflow execution
func (s *CoordinatorState) UpdateUserMessage(newMessage string) {
	if s.OriginalMessage == "" {
		s.OriginalMessage = s.UserMessage
	}
	s.UserMessage = newMessage
	now := time.Now()
	s.UpdatedAt = &now
}

// HasMessageUpdate returns true if the message has been updated during execution
func (s *CoordinatorState) HasMessageUpdate() bool {
	return s.UpdatedAt != nil && s.OriginalMessage != s.UserMessage
}

// wasLLMAbleToAnswerLegacy checks if the LLM was able to provide a meaningful answer
// (inlined from the original utils.WasLLMAbleToAnswerLegacy).
func wasLLMAbleToAnswerLegacy(input string) bool {
	lowerInput := strings.ToLower(input)
	return !(strings.Contains(lowerInput, "i do not have enough information") ||
		strings.Contains(lowerInput, "i don't have enough information") ||
		strings.Contains(lowerInput, "insufficient information") ||
		strings.Contains(lowerInput, "i don't know") ||
		strings.Contains(lowerInput, "i do not know") ||
		strings.Contains(lowerInput, "i'm not sure") ||
		strings.Contains(lowerInput, "i am not sure") ||
		strings.Contains(lowerInput, "not found") ||
		strings.Contains(lowerInput, "none") ||
		strings.Contains(lowerInput, "n/a") ||
		strings.Contains(lowerInput, "i cannot assist") ||
		strings.Contains(lowerInput, "not provided") ||
		strings.Contains(lowerInput, "not able") ||
		strings.Contains(lowerInput, "unable") ||
		strings.Contains(lowerInput, "didn't find") ||
		strings.Contains(lowerInput, "did not find") ||
		strings.Contains(lowerInput, "couldn't find") ||
		strings.Contains(lowerInput, "could not find") ||
		strings.Contains(lowerInput, "no information")) ||
		strings.Contains(lowerInput, "not possible") ||
		strings.Contains(lowerInput, "does not include information")
}

// AddToolExecution adds a complete tool execution record to the state.
// The result is expected to be already sanitized by the caller (e.g., processToolExecutionResult).
// This method only stores and records — it does NOT apply sanitization.
func (s *CoordinatorState) AddToolExecution(toolName, rationale, result string, err error, duration time.Duration) {
	// Update old fields for backward compatibility
	s.ToolResults[toolName] = result
	s.ExecutedTools = append(s.ExecutedTools, toolName)

	// Add to execution history
	entry := ExecutionHistoryEntry{
		ToolName:  toolName,
		Rationale: rationale,
		Result:    result,
		Error:     err,
		Duration:  duration,
		Timestamp: time.Now(),
	}

	s.ExecutionHistory = append(s.ExecutionHistory, entry)

	// If we have a conversation thread, add the tool result as a new user message
	if s.ConversationThread != nil {
		var resultMessage string
		if err != nil {
			resultMessage = fmt.Sprintf("answer: Error executing %s: %s",
				toolName, err.Error())
		} else {
			resultMessage = fmt.Sprintf("answer: %s", result)

			// Special handling for askToContext: if LLM was able to answer, add attention message
			if toolName == SystemFunctionAskToContext && wasLLMAbleToAnswerLegacy(result) {
				resultMessage += "\n\nATTENTION: ** THE INFO IN THE CONTEXT WAS NOT SUBMITTED TO THE USER YET, THIS IS THE CHAIN OF THOUGHTS THAT WILL FORM THE REPLY THAT WILL STILL BE SENT TO THE USER."
			}
		}

		s.ConversationThread.AddMessages(
			thread.NewUserMessage().AddContent(thread.NewTextContent(resultMessage)),
		)
	}
}

// RegisterSanitizedFunction registers a function for sanitization during this workflow session.
// Only functions explicitly registered will have their output sanitized.
func (s *CoordinatorState) RegisterSanitizedFunction(functionName string, config *skill.SanitizeConfig) {
	if config != nil {
		s.SanitizedFunctions[functionName] = config
	}
}

// HasSanitizedFunctions returns true if any functions have sanitization configured
func (s *CoordinatorState) HasSanitizedFunctions() bool {
	return len(s.SanitizedFunctions) > 0
}

// FormatForLLM formats the state for the LLM prompt
func (s *CoordinatorState) FormatForLLM(availableTools []string, contextForAgent string) string {
	var sb strings.Builder

	// Add the user message (using the latest/updated message)
	sb.WriteString(fmt.Sprintf("<userMessage>\n %s\n</userMessage>\n", s.UserMessage))

	// If the message was updated during execution, mention it
	if s.HasMessageUpdate() {
		sb.WriteString(fmt.Sprintf("<messageUpdate>\nNote: The user has sent an updated message during execution. Original message: \"%s\"\nCurrent message: \"%s\"\nPlease review carefully what was done and what still missing given the most recent user message and updated context.\n</messageUpdate>\n", s.OriginalMessage, s.UserMessage))
	}

	sb.WriteString(fmt.Sprintf("<context>\n %s\n</context>\n", contextForAgent))

	// Add available tools
	sb.WriteString("<availableTools>\n ")
	sb.WriteString(strings.Join(availableTools, ",\n "))
	sb.WriteString("\n</availableTools>\n")

	// Add tool execution history
	if len(s.ExecutionHistory) > 0 {
		for _, entry := range s.ExecutionHistory {
			if entry.Error != nil {
				// Include both the detailed result message AND the error for better context
				if entry.Result != "" {
					sb.WriteString(fmt.Sprintf("answer: Error executing %s: %s (Error: %s)\n",
						entry.ToolName, entry.Result, entry.Error.Error()))
				} else {
					sb.WriteString(fmt.Sprintf("answer: Error executing %s: %s\n",
						entry.ToolName, entry.Error.Error()))
				}
			} else {
				sb.WriteString(fmt.Sprintf("answer: %s\n", entry.Result))
			}
		}
	}

	return sb.String()
}

// FormatExecutionHistory formats the execution history for validation
func (s *CoordinatorState) FormatExecutionHistory() string {
	var sb strings.Builder

	for i, entry := range s.ExecutionHistory {
		sb.WriteString(fmt.Sprintf("Step %d: %s\n", i+1, entry.ToolName))
		sb.WriteString(fmt.Sprintf("Rationale: %s\n", entry.Rationale))

		if entry.Error != nil {
			// Include both the detailed result message AND the error for better context
			if entry.Result != "" {
				sb.WriteString(fmt.Sprintf("Result (ERROR): %s (Error: %s)\n", entry.Result, entry.Error.Error()))
			} else {
				sb.WriteString(fmt.Sprintf("Result (ERROR): %s\n", entry.Error.Error()))
			}
		} else {
			sb.WriteString(fmt.Sprintf("Result: %s\n", entry.Result))
		}

		// Add a separator except after the last item
		if i < len(s.ExecutionHistory)-1 {
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

// GetExecutedTools returns the list of executed tools
func (s *CoordinatorState) GetExecutedTools() []string {
	return s.ExecutedTools
}

// GetToolResult returns the result of a specific tool execution
func (s *CoordinatorState) GetToolResult(toolName string) (string, bool) {
	result, exists := s.ToolResults[toolName]
	return result, exists
}

// GetExecutionMetrics returns execution metrics for the workflow
func (s *CoordinatorState) GetExecutionMetrics() map[string]interface{} {
	metrics := map[string]interface{}{
		"totalTools": len(s.ExecutedTools),
		"toolNames":  s.ExecutedTools,
		"totalTime":  time.Since(s.StartTime),
	}
	return metrics
}

// ====================================
// Async Execution Methods
// ====================================

// RegisterAsyncJob creates a pending async job and returns it.
// The caller starts a goroutine that calls ar.Complete() when done.
func (s *CoordinatorState) RegisterAsyncJob(toolName, rationale string) *AsyncResult {
	ar := &AsyncResult{
		ToolName:  toolName,
		Rationale: rationale,
		Done:      make(chan struct{}),
	}
	s.asyncMu.Lock()
	s.asyncResults = append(s.asyncResults, ar)
	s.asyncMu.Unlock()
	return ar
}

// Complete writes results and signals completion via channel close.
// MUST only be called once per AsyncResult.
func (ar *AsyncResult) Complete(result string, err error, duration time.Duration) {
	ar.Result = result     // write BEFORE close
	ar.Error = err         // write BEFORE close
	ar.Duration = duration // write BEFORE close
	close(ar.Done)         // memory barrier — readers see all writes above
}

// IsCompleted returns true if the async job has finished (non-blocking).
func (ar *AsyncResult) IsCompleted() bool {
	select {
	case <-ar.Done:
		return true
	default:
		return false
	}
}

// HasPendingAsync returns true if any async job hasn't completed yet.
func (s *CoordinatorState) HasPendingAsync() bool {
	s.asyncMu.Lock()
	defer s.asyncMu.Unlock()
	for _, ar := range s.asyncResults {
		if !ar.IsCompleted() {
			return true
		}
	}
	return false
}

// PendingAsyncCount returns the number of pending async jobs (for logging).
func (s *CoordinatorState) PendingAsyncCount() int {
	s.asyncMu.Lock()
	defer s.asyncMu.Unlock()
	count := 0
	for _, ar := range s.asyncResults {
		if !ar.IsCompleted() {
			count++
		}
	}
	return count
}

// WaitForAllPendingAsync blocks until all pending async jobs complete.
// Uses channels — zero CPU while waiting, instant wake-up on completion.
// Respects context cancellation for graceful shutdown.
func (s *CoordinatorState) WaitForAllPendingAsync(ctx context.Context) error {
	s.asyncMu.Lock()
	pending := make([]*AsyncResult, 0)
	for _, ar := range s.asyncResults {
		if !ar.IsCompleted() {
			pending = append(pending, ar)
		}
	}
	s.asyncMu.Unlock()

	for _, ar := range pending {
		select {
		case <-ar.Done:
			// completed — zero CPU cost to get here
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

// DrainCompletedAsync returns all completed async jobs and removes them
// from the list. Non-blocking — only returns what's already done.
func (s *CoordinatorState) DrainCompletedAsync() []*AsyncResult {
	s.asyncMu.Lock()
	defer s.asyncMu.Unlock()

	var completed []*AsyncResult
	var remaining []*AsyncResult
	for _, ar := range s.asyncResults {
		if ar.IsCompleted() {
			completed = append(completed, ar)
		} else {
			remaining = append(remaining, ar)
		}
	}
	s.asyncResults = remaining
	return completed
}
