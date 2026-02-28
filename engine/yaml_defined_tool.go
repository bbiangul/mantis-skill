package engine

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	tool_engine_models "github.com/bbiangul/mantis-skill/engine/models"
	tool_engine_utils "github.com/bbiangul/mantis-skill/engine/utils"
	tool_protocol "github.com/bbiangul/mantis-skill/skill"
	"github.com/henomis/lingoose/thread"
)

// SkippedStepInfo contains information about a step that was skipped due to runOnlyIf
type SkippedStepInfo struct {
	Name       string `json:"name"`       // Step name
	Reason     string `json:"reason"`     // Why it was skipped
	Expression string `json:"expression"` // The runOnlyIf expression
	EvalResult string `json:"evalResult"` // The evaluated expression showing actual values
}

// CodeStepDebugInfo captures debug data for a single code step execution.
// Stored in original_output for runtime debugging via API.
type CodeStepDebugInfo struct {
	StepName  string                 `json:"stepName"`
	Prompt    string                 `json:"prompt"`
	Options   map[string]interface{} `json:"options"`
	RawOutput string                 `json:"rawOutput"`
	Success   bool                   `json:"success"`
	Error     string                 `json:"error,omitempty"`
}

// CodeExecutionDetails aggregates debug info for all steps in a code operation.
type CodeExecutionDetails struct {
	Steps []CodeStepDebugInfo `json:"steps"`
}

// hookDepthContextKey is used to track hook execution depth for recursion protection
type hookDepthContextKey struct{}

// maxHookDepth is the maximum allowed hook nesting depth to prevent infinite recursion
// When circular hooks are registered (A → B → A), this limit prevents stack overflow
const maxHookDepth = 10

// Pre-compiled regexes for step runOnlyIf evaluation
var (
	// Matches result[N] with optional path like .field, [0], .field[0].subfield
	stepResultReferenceRegex = regexp.MustCompile(`result\[(\d+)\]((?:\.[a-zA-Z0-9_]+|\[\d+\])*)`)
	// Matches array index like [0], [1], etc.
	arrayIndexRegex = regexp.MustCompile(`\[(\d+)\]`)
)

// YAMLDefinedTool is a langchaingo tool created from a YAML definition
type YAMLDefinedTool struct {
	tool              *tool_protocol.Tool
	function          *tool_protocol.Function
	inputFulfiller    tool_engine_models.IInputFulfiller
	executionTracker  tool_engine_models.IFunctionExecutionTracker
	variableReplacer  tool_engine_models.IVariableReplacer
	outputFormatter   *OutputFormatter
	toolEngine        tool_engine_models.IToolEngine
	mcpToolExecutor   MCPExecutor
	apiExecutor       APIExecutor
	workflowInitiator tool_engine_models.IWorkflowInitiator
	skippedSteps      []SkippedStepInfo     // Steps skipped due to runOnlyIf evaluation
	codeDebugDetails  *CodeExecutionDetails // Debug data for code operations (stored in original_output)
}

// StepExecutionDetail contains details about a single step execution for debugging
type StepExecutionDetail struct {
	StepName string      `json:"step_name"`
	Request  interface{} `json:"request,omitempty"`
	Response string      `json:"response,omitempty"`
	Error    string      `json:"error,omitempty"`
}

// APIExecutionDetails contains all step execution details for an API operation
type APIExecutionDetails struct {
	Steps []StepExecutionDetail `json:"steps"`
}

// NewYAMLDefinedTool creates a new YAMLDefinedTool
func NewYAMLDefinedTool(
	inputFulfiller tool_engine_models.IInputFulfiller,
	tool *tool_protocol.Tool,
	function *tool_protocol.Function,
	executionTracker tool_engine_models.IFunctionExecutionTracker,
	variableReplacer tool_engine_models.IVariableReplacer,
	toolEngine tool_engine_models.IToolEngine,
	workflowInitiator tool_engine_models.IWorkflowInitiator,
) *YAMLDefinedTool {
	outputFormatter := NewOutputFormatter(variableReplacer)
	var mcpExecutor MCPExecutor
	var apiExecutor APIExecutor

	// Log auth provider status
	if authProvider := toolEngine.GetAuthProvider(); authProvider != nil {
		if logger != nil {
			logger.Infof("[NewYAMLDefinedTool] Auth provider available for tool %s, function %s", tool.Name, function.Name)
		}
	} else {
		if logger != nil {
			logger.Warnf("[NewYAMLDefinedTool] Auth provider is NIL for tool %s, function %s", tool.Name, function.Name)
		}
	}

	return &YAMLDefinedTool{
		apiExecutor:       apiExecutor,
		tool:              tool,
		function:          function,
		inputFulfiller:    inputFulfiller,
		executionTracker:  executionTracker,
		variableReplacer:  variableReplacer,
		outputFormatter:   outputFormatter,
		toolEngine:        toolEngine,
		workflowInitiator: workflowInitiator,
		mcpToolExecutor:   mcpExecutor,
	}
}

// Name returns the name of the tool
func (t *YAMLDefinedTool) Name() string {
	return t.function.Name
}

// Description returns a description of the tool
func (t *YAMLDefinedTool) Description() string {
	description := fmt.Sprintf("%s (From %s v%s)", t.function.Description, t.tool.Name, t.tool.Version)

	if len(t.function.Needs) > 0 {
		needNames := make([]string, len(t.function.Needs))
		for i, need := range t.function.Needs {
			needNames[i] = need.Name
		}
		description += fmt.Sprintf(" [Requires: %s]", strings.Join(needNames, ", "))
	}

	// If the function has onSuccess functions, include them in the description
	if len(t.function.OnSuccess) > 0 {
		var names []string
		for _, fc := range t.function.OnSuccess {
			names = append(names, fc.Name)
		}
		description += fmt.Sprintf(" [OnSuccess: %s]", strings.Join(names, ", "))
	}

	// If the function has onFailure functions, include them in the description
	if len(t.function.OnFailure) > 0 {
		var names []string
		for _, fc := range t.function.OnFailure {
			names = append(names, fc.Name)
		}
		description += fmt.Sprintf(" [OnFailure: %s]", strings.Join(names, ", "))
	}

	return description
}

// parseRunOnlyIfResponse parses the LLM response for RunOnlyIf condition evaluation.
// It expects the format: "done: {true or false} || {message}"
// Returns shouldExecute (bool) and explanationMessage (string)
func parseRunOnlyIfResponse(rawResponse string) (shouldExecute bool, explanationMessage string) {
	// Priority 1: Check for "true ||" or "false ||" format (without "done:" prefix)
	if strings.Contains(rawResponse, "||") {
		parts := strings.SplitN(rawResponse, "||", 2)
		if len(parts) == 2 {
			boolPart := strings.TrimSpace(parts[0])
			messagePart := strings.TrimSpace(parts[1])
			boolLower := strings.ToLower(boolPart)

			// First priority: direct boolean value (true, false, yes, no, etc.)
			// Don't check for "done:" prefix here
			if !strings.Contains(boolLower, "done:") {
				if boolLower == "true" || boolLower == "yes" || boolLower == "y" ||
					boolLower == "sim" || boolLower == "oui" || boolLower == "> true" {
					shouldExecute = true
					explanationMessage = messagePart
					return shouldExecute, explanationMessage
				}
				if boolLower == "false" || boolLower == "no" || boolLower == "n" ||
					boolLower == "não" || boolLower == "nao" || boolLower == "non" || boolLower == "> false" {
					shouldExecute = false
					explanationMessage = messagePart
					return shouldExecute, explanationMessage
				}
			}

			// Priority 2: Check for "done: true ||" or "done: false ||" format
			if strings.Contains(boolLower, "done:") {
				// Extract the value after "done:"
				boolStr := strings.TrimSpace(strings.SplitN(boolPart, ":", 2)[1])
				boolStrLower := strings.ToLower(boolStr)

				if boolStrLower == "true" || boolStrLower == "yes" ||
					boolStrLower == "y" || boolStrLower == "sim" || boolStrLower == "oui" {
					shouldExecute = true
				} else if boolStrLower == "false" || boolStrLower == "no" ||
					boolStrLower == "n" || boolStrLower == "não" || boolStrLower == "nao" || boolStrLower == "non" {
					shouldExecute = false
				}

				explanationMessage = messagePart
				return shouldExecute, explanationMessage
			}
		}
	}

	// Priority 3: Fallback to legacy parsing for backward compatibility
	// Check for boolean keywords
	// Note: We avoid single-letter checks like "y" to prevent false positives (e.g., "ready" contains "y")
	responseLower := strings.ToLower(rawResponse)
	if strings.Contains(responseLower, "true") || strings.Contains(responseLower, "yes") ||
		strings.Contains(responseLower, "sim") || strings.Contains(responseLower, "oui") {
		shouldExecute = true
	}
	explanationMessage = rawResponse

	return shouldExecute, explanationMessage
}

// parseMockRunOnlyIfResponse parses a mock response for runOnlyIf inference conditions.
// Supports multiple formats:
// 1. bool: true/false directly
// 2. string: "true"/"false" (case insensitive)
// 3. map with "result" (bool) and optional "explanation" (string)
func parseMockRunOnlyIfResponse(response interface{}) (bool, string) {
	// Handle nil response
	if response == nil {
		return false, "mock response was nil"
	}

	// Handle bool directly
	if b, ok := response.(bool); ok {
		if b {
			return true, "mocked: condition passed"
		}
		return false, "mocked: condition not met"
	}

	// Handle string
	if s, ok := response.(string); ok {
		sLower := strings.ToLower(strings.TrimSpace(s))
		if sLower == "true" || sLower == "yes" || sLower == "sim" {
			return true, "mocked: condition passed"
		}
		return false, "mocked: condition not met"
	}

	// Handle map with "result" and optional "explanation"
	if m, ok := response.(map[string]interface{}); ok {
		result := false
		explanation := "mocked: condition evaluated"

		// Check for "result" key
		if r, exists := m["result"]; exists {
			switch v := r.(type) {
			case bool:
				result = v
			case string:
				vLower := strings.ToLower(strings.TrimSpace(v))
				result = vLower == "true" || vLower == "yes" || vLower == "sim"
			}
		}

		// Check for "explanation" key
		if e, exists := m["explanation"]; exists {
			if es, ok := e.(string); ok {
				explanation = es
			}
		}

		return result, explanation
	}

	// Default: false
	return false, "mocked: unable to parse response"
}

// Call executes the tool with the given input
func (t *YAMLDefinedTool) Call(ctx context.Context, input string) (string, error) {

	logger.Debugf("Calling YAMLDefinedTool...")
	startTime := time.Now()

	// Log method entry with key parameters
	logger.Debugf("YAMLDefinedTool.Call started - tool: %s, function: %s, input_length: %d",
		t.tool.Name, t.function.Name, len(input))

	retrievedMsg, ok := ctx.Value(MessageInContextKey).(Message)
	if !ok {
		logger.Errorf("error getting message from context")
		return "", fmt.Errorf("failed to get message from context")
	}
	logger.Debugf("Context loaded - messageID: %s, clientID: %s", retrievedMsg.Id, retrievedMsg.ClientID)
	messageID := retrievedMsg.Id

	// Add messageID to context for variable replacer to access
	ctx = context.WithValue(ctx, "message_id", messageID)
	logger.Debugf("Added message_id to context: %s", messageID)
	clientID := retrievedMsg.ClientID

	// Prepare variables to store execution results
	var output string
	var originalOutput *string // Original output before transformation (nil if no transformation)
	var inputs map[string]interface{}
	var inputsHash string // Store hash calculated during CallRule check
	status := tool_protocol.StatusComplete
	executionRecorded := false // Track if execution was already saved synchronously
	t.skippedSteps = nil       // Reset skipped steps for this execution

	// Ensure the execution is recorded regardless of success or failure
	defer func() {
		// Skip if execution was already recorded synchronously (for onSuccess functions)
		if executionRecorded {
			logger.Debugf("Skipping deferred execution recording for %s.%s - already recorded synchronously", t.tool.Name, t.function.Name)
			return
		}

		// If we didn't set inputs (early return), initialize to empty map
		if inputs == nil {
			inputs = make(map[string]interface{})
		}

		// If there are skipped steps, include them in original_output
		if len(t.skippedSteps) > 0 {
			skippedJSON, err := json.Marshal(map[string]interface{}{
				"_skippedSteps": t.skippedSteps,
			})
			if err == nil {
				skippedStr := string(skippedJSON)
				originalOutput = &skippedStr
			}
		}

		// Record the execution (hash will be calculated if not already done)
		err := t.executionTracker.RecordExecution(
			ctx,
			messageID,
			clientID,
			t.tool.Name,
			t.function.Name,
			t.Description(),
			inputs,
			inputsHash, // Pass the pre-calculated hash (or empty string)
			output,
			originalOutput, // Original output before transformation (nil if no transformation), or skipped steps info
			startTime,
			status,
			t.function,
		)
		if err != nil {
			fmt.Printf("Warning: Failed to record function execution: %v\n", err)
		}
	}()

	var callback *ExecutorAgenticWorkflowCallback
	if callback, ok = ctx.Value(CallbackInContextKey).(*ExecutorAgenticWorkflowCallback); !ok {
		return "", fmt.Errorf("failed to load callback")
	}

	// Step -1: Check if function requires initiate_workflow context
	if t.function.RequiresInitiateWorkflow {
		_, hasWorkflowContext := ctx.Value(InitiateWorkflowOriginContextKey).(bool)
		if !hasWorkflowContext {
			status = tool_protocol.StatusSkipped
			output = fmt.Sprintf("Function '%s.%s' requires workflow context and cannot be called directly. "+
				"This function must be triggered through an initiate_workflow operation with context.params injection.",
				t.tool.Name, t.function.Name)
			logger.Infof("[WORKFLOW-REQUIRED] Function %s.%s skipped - requires initiate_workflow context but called directly",
				t.tool.Name, t.function.Name)
			return output, nil // Not an error - expected behavior
		}
	}

	// Step 0: Early cache check for client/message scoped cache with includeInputs: false
	// This allows returning cached results without executing ANY steps (including input resolution)
	if t.function.ParsedCache != nil &&
		t.function.ParsedCache.TTL > 0 &&
		t.function.ParsedCache.GetScope() != tool_protocol.CacheScopeGlobal &&
		!t.function.ParsedCache.GetIncludeInputs() {

		scope := t.function.ParsedCache.GetScope()
		scopeID := ""
		switch scope {
		case tool_protocol.CacheScopeClient:
			scopeID = clientID
		case tool_protocol.CacheScopeMessage:
			scopeID = messageID
		}

		if scopeID != "" {
			cachedOutput, found, err := t.executionTracker.GetFromScopedCache(ctx, t.tool.Name, t.function.Name, scope, scopeID, nil)
			if err != nil {
				logger.Warnf("Error in early scoped cache check: %v", err)
			} else if found {
				output = cachedOutput
				logger.Infof("EARLY cache HIT (scope: %s, includeInputs: false) - Returning cached result for %s.%s without any execution",
					scope, t.tool.Name, t.function.Name)
				return output, nil
			}
		}
	}

	// Step 1: Check and execute needed functions (dependencies) - must run first to provide context for runOnlyIf
	logger.Debugf("Step 1: Dependency execution phase - checking %d dependencies: %v", len(t.function.Needs), t.function.Needs)
	if len(t.function.Needs) > 0 {
		logger.Infof("Executing %d dependencies for %s.%s", len(t.function.Needs), t.tool.Name, t.function.Name)
		allMet, executedResults, err := t.toolEngine.ExecuteDependencies(ctx, messageID, clientID, t.tool, t.function.Needs, t.inputFulfiller, callback, t.function.Name)
		if err != nil {
			logger.Errorf("Dependency execution failed: %v", err)
			// If we have system functions that need to be executed, we can't proceed
			if strings.Contains(err.Error(), "system functions need to be executed") {
				output = err.Error()
				status = tool_protocol.StatusFailed
				output = t.executeOnFailureFunctions(ctx, messageID, clientID, output, originalOutput, inputs, inputsHash, startTime, callback)
				executionRecorded = true // Mark as recorded to skip the defer
				return output, nil
			}

			// For other errors, also report failure
			output = fmt.Sprintf("Error executing dependencies: %s", err.Error())
			status = tool_protocol.StatusFailed
			output = t.executeOnFailureFunctions(ctx, messageID, clientID, output, originalOutput, inputs, inputsHash, startTime, callback)
			executionRecorded = true // Mark as recorded to skip the defer
			return output, nil
		}

		// Check if dependencies returned special messages even without error
		logger.Debugf("Dependencies execution completed - allMet: %t, results_count: %d", allMet, len(executedResults))
		if !allMet && len(executedResults) > 0 {
			// Check each result for special return messages
			for _, result := range executedResults {
				if tool_engine_utils.IsSpecialReturn(result) {
					// Extract just the message part after the function name
					parts := strings.SplitN(result, ": ", 2)
					if len(parts) > 1 {
						output = parts[1]
					} else {
						output = result
					}
					status = tool_protocol.StatusPending
					return output, nil
				}
			}
		}

		if len(executedResults) > 0 {
			logger.Debugf("Auto-executed dependencies for %s: %s",
				t.function.Name, tool_engine_utils.TruncateForLogging(strings.Join(executedResults, "; ")))
		}
	}

	// Step 2: Evaluate conditional execution (RunOnlyIf) - now has access to dependency results
	if t.function.RunOnlyIf != nil {
		// Parse runOnlyIf to get the object
		// Note: Validation already happened at parse-time in parser.go
		runOnlyIfObj, err := tool_protocol.ParseRunOnlyIf(t.function.RunOnlyIf)
		if err != nil {
			logger.Errorf("Failed to parse runOnlyIf: %v", err)
			return output, fmt.Errorf("error parsing runOnlyIf: %w", err)
		}

		// Evaluate deterministic condition if present
		var deterministicResult bool
		var deterministicEvaluated bool
		var deterministicOriginalExpr string
		var deterministicReplacedExpr string
		if runOnlyIfObj.Deterministic != "" {
			deterministicOriginalExpr = runOnlyIfObj.Deterministic
			logger.Infof("Evaluating DETERMINISTIC runOnlyIf condition: %s", deterministicOriginalExpr)

			// Build dependency results map from execution tracker
			// This fetches all function results from the 'needs' block
			dependencyResults := t.buildDependencyResultsMap(ctx, messageID)
			logger.Debugf("Built dependency results map with %d entries for deterministic evaluation", len(dependencyResults))

			// Use VariableReplacer to replace all $functionName references in the expression
			// The VariableReplacer will handle:
			// - Navigating paths like $functionName.field or $functionName[0].field
			// - Replacing with literal values from dependencyResults
			// This maintains proper separation of concerns:
			// - tool_engine: handles variable replacement and data access
			// - tool-protocol: handles expression parsing and evaluation
			replacedExpression, err := t.variableReplacer.ReplaceVariables(ctx, deterministicOriginalExpr, dependencyResults)
			if err != nil {
				logger.Errorf("Failed to replace variables in deterministic condition '%s': %v", deterministicOriginalExpr, err)
				return output, fmt.Errorf("error replacing variables in deterministic condition: %w", err)
			}
			deterministicReplacedExpr = replacedExpression
			logger.Debugf("Deterministic expression after variable replacement: %s", deterministicReplacedExpr)

			// Now evaluate the expression with literal values only
			deterministicResult, err = tool_protocol.EvaluateDeterministicExpression(deterministicReplacedExpr)
			if err != nil {
				logger.Errorf("Failed to evaluate deterministic condition: %v", err)
				return output, fmt.Errorf("error evaluating deterministic runOnlyIf condition: %w", err)
			}
			deterministicEvaluated = true
			logger.Infof("Deterministic runOnlyIf result: %t", deterministicResult)
		}

		// Evaluate inference condition if present (either simple Condition or advanced Inference object)
		var inferenceResult bool
		var inferenceEvaluated bool
		var inferenceExplanation string

		if runOnlyIfObj.Condition != "" || runOnlyIfObj.Inference != nil {
			// Check for mock service in context FIRST (test mode only - zero-cost in production)
			// Production code NEVER sets TestMockServiceKey, so this check is always false in production
			functionKey := GenerateFunctionKey(t.tool.Name, t.function.Name)
			if mockSvc, ok := ctx.Value(tool_engine_models.TestMockServiceKey).(tool_engine_models.IMockService); ok && mockSvc != nil {
				// Check if we should mock runOnlyIf_inference for this function
				// Use "runOnlyIf" as the step name for the mock
				if mockSvc.ShouldMock("runOnlyIf_inference", functionKey, "runOnlyIf") {
					if response, exists := mockSvc.GetMockResponseValue(functionKey, "runOnlyIf"); exists {
						logger.Infof("[TEST MOCK] Returning mock for runOnlyIf_inference %s", functionKey)

						// Parse the mock response
						mockResult, mockExplanation := parseMockRunOnlyIfResponse(response)
						mockSvc.RecordCall(functionKey, "runOnlyIf", "runOnlyIf_inference", nil, response, "")

						inferenceResult = mockResult
						inferenceExplanation = mockExplanation
						inferenceEvaluated = true

						logger.Infof("Mocked runOnlyIf result: %v, explanation: %s", inferenceResult, inferenceExplanation)
					}
				}
			}

			// Only run actual inference evaluation if not already mocked
			if !inferenceEvaluated {
				// Determine which inference configuration to use
				var inferenceCondition string
				var inferenceFrom []string
				var inferenceAllowedFuncs []string
				var inferenceDisableAll bool
				var inferenceClientIds string
				var inferenceCodebaseDirs string
				var inferenceDocumentDbName string
				var inferenceDocumentEnableGraph bool

				if runOnlyIfObj.Inference != nil {
					// Advanced inference mode
					logger.Infof("Evaluating INFERENCE runOnlyIf condition (advanced mode): %s", runOnlyIfObj.Inference.Condition)
					inferenceCondition = runOnlyIfObj.Inference.Condition
					inferenceFrom = runOnlyIfObj.Inference.From
					inferenceAllowedFuncs = runOnlyIfObj.Inference.AllowedSystemFunctions
					inferenceDisableAll = runOnlyIfObj.Inference.DisableAllSystemFunctions
					inferenceClientIds = runOnlyIfObj.Inference.ClientIds
					inferenceCodebaseDirs = runOnlyIfObj.Inference.CodebaseDirs
					inferenceDocumentDbName = runOnlyIfObj.Inference.DocumentDbName
					inferenceDocumentEnableGraph = runOnlyIfObj.Inference.DocumentEnableGraph
				} else {
					// Simple inference mode (backward compatible)
					logger.Infof("Evaluating INFERENCE runOnlyIf condition (simple mode): %s", runOnlyIfObj.Condition)
					inferenceCondition = runOnlyIfObj.Condition
					inferenceFrom = runOnlyIfObj.From
					inferenceAllowedFuncs = runOnlyIfObj.AllowedSystemFunctions
					inferenceDisableAll = runOnlyIfObj.DisableAllSystemFunctions
					inferenceClientIds = runOnlyIfObj.ClientIds
					inferenceCodebaseDirs = runOnlyIfObj.CodebaseDirs
					inferenceDocumentDbName = runOnlyIfObj.DocumentDbName
					inferenceDocumentEnableGraph = runOnlyIfObj.DocumentEnableGraph
				}

				condition, err := t.variableReplacer.ReplaceVariablesWithOptions(ctx, inferenceCondition, nil, tool_engine_models.ReplaceOptions{
					KeepUnresolvedPlaceholders: true, // Keep $var for LLM readability
				})
				if err != nil {
					logger.Errorf("Failed to replace variables in condition '%s': %v", inferenceCondition, err)
					return output, fmt.Errorf("error replacing variables in condition: %w", err)
				}
				logger.Debugf("Condition after variable replacement: %s", condition)

				// Prepare context for condition evaluation
				evalCtx := ctx
				if len(inferenceFrom) > 0 {
					logger.Debugf("Filtering context for runOnlyIf evaluation to functions: %v", inferenceFrom)
					evalCtx = t.filterContextForFunctions(ctx, inferenceFrom)
				}

				// Inject clientIds into context if specified (for queryCustomerServiceChats)
				if inferenceClientIds != "" {
					resolvedClientIds, err := t.variableReplacer.ReplaceVariablesWithOptions(evalCtx, inferenceClientIds, nil, tool_engine_models.ReplaceOptions{
						KeepUnresolvedPlaceholders: true, // Keep $var for LLM readability
					})
					if err != nil {
						logger.Warnf("Failed to resolve clientIds variable '%s': %v", inferenceClientIds, err)
					} else if resolvedClientIds != "" {
						evalCtx = context.WithValue(evalCtx, ClientIdsInContextKey, resolvedClientIds)
						logger.Debugf("Injected clientIds into runOnlyIf context: %s", resolvedClientIds)
					}
				}

				// Inject codebaseDirs into context if specified (for searchCodebase)
				if inferenceCodebaseDirs != "" {
					resolvedDirs, err := t.variableReplacer.ReplaceVariablesWithOptions(evalCtx, inferenceCodebaseDirs, nil, tool_engine_models.ReplaceOptions{
						KeepUnresolvedPlaceholders: true, // Keep $var for LLM readability
					})
					if err != nil {
						logger.Warnf("Failed to resolve codebaseDirs variable '%s': %v", inferenceCodebaseDirs, err)
					} else if resolvedDirs != "" {
						evalCtx = context.WithValue(evalCtx, CodebaseDirsInContextKey, resolvedDirs)
						logger.Debugf("Injected codebaseDirs into runOnlyIf context: %s", resolvedDirs)
					}
				}

				// Inject documentDbName into context if specified (for queryDocuments)
				if inferenceDocumentDbName != "" {
					resolvedDbName, err := t.variableReplacer.ReplaceVariablesWithOptions(evalCtx, inferenceDocumentDbName, nil, tool_engine_models.ReplaceOptions{
						KeepUnresolvedPlaceholders: true, // Keep $var for LLM readability
					})
					if err != nil {
						logger.Warnf("Failed to resolve documentDbName variable '%s': %v", inferenceDocumentDbName, err)
					} else if resolvedDbName != "" {
						evalCtx = context.WithValue(evalCtx, DocumentDbNameInContextKey, resolvedDbName)
						logger.Debugf("Injected documentDbName into runOnlyIf context: %s", resolvedDbName)
					}
				}

				// Inject documentEnableGraph into context if specified (for queryDocuments)
				if inferenceDocumentEnableGraph {
					evalCtx = context.WithValue(evalCtx, DocumentEnableGraphInContextKey, true)
					logger.Debugf("Injected documentEnableGraph=true into runOnlyIf context")
				}

				// Inject memoryFilters into context if specified (for queryMemories)
				if runOnlyIfObj.MemoryFilters != nil {
					resolvedFilters := resolveMemoryFiltersYAML(evalCtx, runOnlyIfObj.MemoryFilters, t.variableReplacer, nil)
					if resolvedFilters != nil {
						evalCtx = context.WithValue(evalCtx, MemoryFiltersInContextKey, resolvedFilters)
						logger.Debugf("Injected memoryFilters into runOnlyIf context: topic=%s, metadata=%v",
							resolvedFilters.Topic, resolvedFilters.Metadata)
					}
				}

				// Prepare success criteria for inference
				var successCriteria interface{}
				successCriteriaCondition := `Evaluate if the function should be executed based on the description.
Return your response in this EXACT format:
done: {true or false} || {clear explanation for the user}

Examples:
- done: true || The user has confirmed they want to schedule the appointment for 3 PM tomorrow.
- done: false || The user hasn't explicitly confirmed they want to schedule an appointment yet.

The explanation should be user-friendly and explain your decision clearly. Output ONLY in the format shown above.`

				if len(inferenceAllowedFuncs) > 0 {
					logger.Debugf("Restricting runOnlyIf agentic inference to system functions: %v", inferenceAllowedFuncs)
					successCriteria = &tool_protocol.SuccessCriteriaObject{
						Condition:              successCriteriaCondition,
						AllowedSystemFunctions: inferenceAllowedFuncs,
					}
				} else if inferenceDisableAll {
					logger.Debugf("Disabling all system functions for runOnlyIf agentic inference")
					successCriteria = &tool_protocol.SuccessCriteriaObject{
						Condition:                 successCriteriaCondition,
						DisableAllSystemFunctions: true,
					}
				} else {
					successCriteria = successCriteriaCondition
				}

				inference := tool_protocol.Input{
					Name:            fmt.Sprintf("should_execute_the_function_%s", t.function.Name),
					Description:     condition,
					SuccessCriteria: successCriteria,
					OnError:         runOnlyIfObj.OnError,
				}
				logger.Debugf("Invoking agentic inference for condition evaluation")

				rawResponse, err := t.inputFulfiller.AgenticInfer(evalCtx, messageID, clientID, inference, t.tool)

				if strings.Contains(strings.ToLower(rawResponse), "i need some additional information to complete this action") ||
					strings.Contains(strings.ToLower(rawResponse), "i need the user confirmation before proceed") ||
					strings.Contains(strings.ToLower(rawResponse), "this action requires team approval") {
					return rawResponse, ErrInputFulfillmentPending
				}

				if err != nil && !strings.Contains(err.Error(), "not found in conversation") {
					logger.Errorf("Agentic inference failed for condition evaluation: %v", err)

					if runOnlyIfObj.OnError != nil {
						logger.Infof("Handling runOnlyIf evaluation error with strategy: %s", runOnlyIfObj.OnError.Strategy)
						return t.handleRunOnlyIfError(ctx, messageID, clientID, runOnlyIfObj.OnError, err, callback, rawResponse)
					}

					output = fmt.Sprintf("Error evaluating semantic condition: %s", err.Error())
					status = tool_protocol.StatusFailed
					output = t.executeOnFailureFunctions(ctx, messageID, clientID, output, originalOutput, inputs, inputsHash, startTime, callback)
					executionRecorded = true
					return output, nil
				}
				logger.Debugf("Agentic inference raw result: %s", rawResponse)

				var errMessage string
				if err != nil {
					errMessage = err.Error()
				}

				if strings.Contains(errMessage, "not found in conversation") {
					inferenceResult = false
					inferenceExplanation = errMessage
				} else {
					inferenceResult, inferenceExplanation = parseRunOnlyIfResponse(rawResponse)
				}
				inferenceEvaluated = true
				logger.Infof("Inference runOnlyIf result: %t, explanation: %s", inferenceResult, inferenceExplanation)
			} // End of if !inferenceEvaluated
		}

		// Combine deterministic and inference results
		var finalResult bool
		var finalExplanation string

		if deterministicEvaluated && inferenceEvaluated {
			// Hybrid mode - combine both results
			combineMode := runOnlyIfObj.CombineMode
			if combineMode == "" {
				combineMode = tool_protocol.CombineModeAND // Default
			}

			if combineMode == tool_protocol.CombineModeAND {
				finalResult = deterministicResult && inferenceResult
				finalExplanation = fmt.Sprintf("Deterministic: %t, Inference: %t (%s), Combined (AND): %t",
					deterministicResult, inferenceResult, inferenceExplanation, finalResult)
			} else if combineMode == tool_protocol.CombineModeOR {
				finalResult = deterministicResult || inferenceResult
				finalExplanation = fmt.Sprintf("Deterministic: %t, Inference: %t (%s), Combined (OR): %t",
					deterministicResult, inferenceResult, inferenceExplanation, finalResult)
			} else {
				// This should never happen due to parse-time validation, but be defensive
				logger.Errorf("Invalid CombineMode '%s', defaulting to AND", combineMode)
				finalResult = deterministicResult && inferenceResult
				finalExplanation = fmt.Sprintf("Deterministic: %t, Inference: %t (%s), Combined (AND - fallback): %t",
					deterministicResult, inferenceResult, inferenceExplanation, finalResult)
			}
			logger.Infof("Hybrid runOnlyIf evaluation - %s", finalExplanation)
		} else if deterministicEvaluated {
			finalResult = deterministicResult
			finalExplanation = fmt.Sprintf("Deterministic evaluation: %s → %s → %t", deterministicOriginalExpr, deterministicReplacedExpr, deterministicResult)
		} else if inferenceEvaluated {
			finalResult = inferenceResult
			finalExplanation = inferenceExplanation
		} else {
			// No evaluation performed (shouldn't happen due to validation)
			logger.Warnf("No runOnlyIf evaluation performed despite runOnlyIf being set")
			finalResult = true // Default to allowing execution
		}

		// Act on the final result
		if !finalResult {
			logger.Infof("Execution skipped due to runOnlyIf condition - tool: %s.%s, explanation: %s",
				t.tool.Name, t.function.Name, finalExplanation)

			var stepKey string
			stepKey, ok = ctx.Value(StepKeyInContextKey).(string)
			if !ok {
				logger.Errorf("Error getting step key from ctx")
			}

			enhancedMessage := fmt.Sprintf("TOOL_EXECUTION_SKIPPED: Pre-condition not met - %s. This tool was NOT executed and did NOT complete its intended action.", finalExplanation)
			callback.OnStepExecuted(ctx, stepKey, messageID, clientID, t.tool.Name, enhancedMessage, fmt.Errorf("skipped due to runOnlyIf condition"))
			output = enhancedMessage
			status = tool_protocol.StatusSkipped

			// Execute onSkip callbacks if defined
			if len(t.function.OnSkip) > 0 {
				logger.Infof("Executing %d onSkip callbacks for %s.%s", len(t.function.OnSkip), t.tool.Name, t.function.Name)
				output = t.executeOnSkipCallbacks(ctx, messageID, clientID, finalExplanation, callback)
			}

			if runOnlyIfObj.OnError != nil {
				logger.Infof("Handling runOnlyIf evaluation error with strategy: %s", runOnlyIfObj.OnError.Strategy)
				return t.handleRunOnlyIfError(ctx, messageID, clientID, runOnlyIfObj.OnError, nil, callback, finalExplanation)
			}

			return output, nil
		}

		logger.Infof("RunOnlyIf condition MET for %s.%s - proceeding with execution. Reason: %s",
			t.tool.Name, t.function.Name, finalExplanation)
	}

	// Step 2.5: Check CallRule constraints for "once" type rules (before input fulfillment)
	// This optimization allows us to skip input fulfillment if the function has already been executed
	if t.function.CallRule != nil && t.function.CallRule.Type == tool_protocol.CallRuleTypeOnce {
		logger.Debugf("Step 2.5: Pre-fulfillment CallRule check - checking 'once' rule for %s.%s", t.tool.Name, t.function.Name)
		violates, message, err := t.executionTracker.CheckCallRuleViolationWithoutInputs(
			ctx, messageID, clientID, t.tool.Name, t.function.Name, t.function.CallRule)
		if err != nil {
			logger.Errorf("CallRule check failed: %v", err)
			output = fmt.Sprintf("This function was NOT executed due: %s", err.Error())
			status = tool_protocol.StatusFailed
			return output, nil
		}
		if violates {
			logger.Infof("CallRule violation detected for %s.%s: %s", t.tool.Name, t.function.Name, message)
			output = message
			status = tool_protocol.StatusFailed
			return output, nil
		}
		logger.Debugf("Pre-fulfillment CallRule check passed for %s.%s", t.tool.Name, t.function.Name)
	}

	// Step 3: Fulfill the inputs based on their origin
	logger.Debugf("Step 3: Input fulfillment phase - processing %d input fields", len(t.function.Input))
	logger.Infof("Fulfilling inputs for %s.%s", t.tool.Name, t.function.Name)
	fulfillInputs, err := t.inputFulfiller.FulfillInputs(
		ctx,
		messageID,
		clientID,
		t.tool,
		t.function.Name,
		t.function.Input,
	)

	if err != nil {
		logger.Errorf("Input fulfillment failed for %s.%s: %v", t.tool.Name, t.function.Name, err)
		if errors.Is(err, ErrInputFulfillmentPending) {
			logger.Debugf("Input fulfillment pending - checking for missing input details")
			// Check if we have information about missing inputs
			if missingInputsData, ok := fulfillInputs["missingInputs"]; ok {
				// Type assert to get our structured data
				missingInputs, ok := missingInputsData.([]tool_protocol.MissingInputInfo)
				if ok && len(missingInputs) > 0 {
					logger.Infof("Found %d missing inputs that require user attention", len(missingInputs))
					// Build a comprehensive message for all missing inputs
					var sb strings.Builder
					sb.WriteString("I need some additional information to complete this action:\n\n")

					for _, mi := range missingInputs {
						sb.WriteString(fmt.Sprintf("- %s: %s\n", mi.Name, mi.Message))
						if mi.Description != "" {
							sb.WriteString(fmt.Sprintf("  (Description: %s)\n", mi.Description))
						}
					}

					sb.WriteString("\nPlease provide this information so I can continue.")

					output = sb.String()
					status = tool_protocol.StatusPending
					// Execute onMissingUserInfo callbacks before returning (inputs not yet fulfilled, use empty map)
					t.executeMagicStringCallbacks(ctx, messageID, clientID, tool_engine_utils.SpecialReturnNeedsInfo, nil, output, callback)
					return output, nil
				}
			}

			if missingInputsValue, ok := fulfillInputs["missingValueOfInputs"]; ok {
				missingValues, ok := missingInputsValue.(string)
				if ok {
					var sb strings.Builder
					sb.WriteString("I need some additional information to complete this action:\n\n")
					sb.WriteString(missingValues)
					sb.WriteString("\nPlease provide this information so I can continue.")

					output = sb.String()
					status = tool_protocol.StatusPending
					// Execute onMissingUserInfo callbacks before returning (inputs not yet fulfilled, use empty map)
					t.executeMagicStringCallbacks(ctx, messageID, clientID, tool_engine_utils.SpecialReturnNeedsInfo, nil, output, callback)
					return output, nil
				}
			}

			output = "I'm working on gathering the necessary information to complete this action. I'll get back to you as soon as I have everything I need."
			status = tool_protocol.StatusPending
			// Execute onMissingUserInfo callbacks before returning (inputs not yet fulfilled, use empty map)
			t.executeMagicStringCallbacks(ctx, messageID, clientID, tool_engine_utils.SpecialReturnNeedsInfo, nil, output, callback)
			return output, nil
		}

		if errors.Is(err, context.Canceled) {
			return "", err
		}

		if strings.Contains(err.Error(), NoLLmManagedError) {
			return "", err
		}

		output = fmt.Sprintf("Error preparing inputs: %s", err.Error())
		status = tool_protocol.StatusFailed
		return output, nil
	}

	// before saving, lets try to parse complex objects
	logger.Debugf("Parsing JSON inputs - raw inputs count: %d", len(fulfillInputs))
	parsedInputs := ParseJSONInputs(fulfillInputs)
	inputs = parsedInputs

	// Step 4: Check CallRule constraints for "unique" type rules (after input fulfillment)
	// Note: "once" rules are checked in Step 2.5 before input fulfillment for efficiency
	logger.Debugf("Step 4: Post-fulfillment CallRule check - checking 'unique' rule for %s.%s", t.tool.Name, t.function.Name)
	if t.function.CallRule != nil && t.function.CallRule.Type == tool_protocol.CallRuleTypeUnique {
		// Check for CallRule violations using the parsed inputs
		// This also returns the inputs hash to avoid recalculating it later
		violates, message, hash, err := t.executionTracker.CheckCallRuleViolation(
			ctx, messageID, clientID, t.tool.Name, t.function.Name, inputs, t.function.CallRule)
		if err != nil {
			logger.Errorf("CallRule check failed: %v", err)
			output = fmt.Sprintf("This function was NOT executed due: %s", err.Error())
			status = tool_protocol.StatusFailed
			return output, nil
		}
		// Store the calculated hash for reuse in RecordExecution
		inputsHash = hash
		if violates {
			logger.Infof("CallRule violation detected for %s.%s: %s", t.tool.Name, t.function.Name, message)
			output = message
			status = tool_protocol.StatusFailed
			return output, nil
		}
		logger.Debugf("Post-fulfillment CallRule check passed for %s.%s", t.tool.Name, t.function.Name)
	}

	callback.OnInputsFulfilled(ctx, messageID, t.tool.Name, t.function.Name, inputs)

	// Execute beforeExecution hooks (after inputs fulfilled, before main operation)
	if err := t.executeHooks(ctx, messageID, clientID, "beforeExecution", inputs, "", callback); err != nil {
		logger.Warnf("beforeExecution hooks error: %v", err)
	}

	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}

	// Step 5: Pre-resolve and cache inputs for onSuccess functions if defined
	// This allows us to fail early if onSuccess functions have critical missing inputs
	// Note: Inputs with origin="memory" or "inference" will be resolved after parent executes
	onSuccessInputsCache := make(map[string]map[string]interface{})
	onSuccessPendingMemoryInputs := make(map[string][]tool_protocol.Input) // Track deferred (memory/inference) inputs to resolve later

	if len(t.function.OnSuccess) > 0 {
		logger.Debugf("Pre-resolving inputs for %d onSuccess functions", len(t.function.OnSuccess))
		logger.Infof("[ONSUCCESS-PRESOLVE] Parent function: %s.%s, onSuccess count: %d", t.tool.Name, t.function.Name, len(t.function.OnSuccess))

		var criticalMissingInputs []string
		for _, successCall := range t.function.OnSuccess {
			successFuncName := successCall.Name
			logger.Infof("[ONSUCCESS-PRESOLVE] Processing onSuccess function '%s', params: %v", successFuncName, successCall.Params)

			// Find the onSuccess function definition
			var successFunc *tool_protocol.Function
			for i := range t.tool.Functions {
				if t.tool.Functions[i].Name == successFuncName {
					successFunc = &t.tool.Functions[i]
					break
				}
			}

			// Skip system functions as they don't have input definitions in the tool
			if successFunc == nil {
				logger.Debugf("Skipping input resolution for '%s' (likely a system function)", successFuncName)
				continue
			}

			// Check if we should skip pre-evaluation for this function
			// When SkipInputPreEvaluation=true, inputs will be evaluated during actual execution
			if successFunc.SkipInputPreEvaluation {
				logger.Debugf("Skipping pre-evaluation for '%s' (SkipInputPreEvaluation=true, inputs will be evaluated during execution)", successFuncName)
				continue
			}

			// Separate inputs by origin type
			var nonMemoryInputs []tool_protocol.Input
			var memoryFromParentInputs []tool_protocol.Input

			for _, input := range successFunc.Input {
				if input.Origin == tool_protocol.DataOriginMemory || input.Origin == tool_protocol.DataOriginInference {
					// These inputs may need prior function outputs, defer resolution
					memoryFromParentInputs = append(memoryFromParentInputs, input)
					logger.Debugf("Deferring resolution of %s input '%s' for onSuccess function '%s' (may need prior function outputs)",
						input.Origin, input.Name, successFuncName)
				} else {
					// This input can be resolved now
					nonMemoryInputs = append(nonMemoryInputs, input)
				}
			}

			// Store deferred inputs (memory/inference) for later resolution
			if len(memoryFromParentInputs) > 0 {
				onSuccessPendingMemoryInputs[successFuncName] = memoryFromParentInputs
			}

			// Try to fulfill only non-deferred inputs for now
			if len(nonMemoryInputs) > 0 {
				logger.Debugf("Pre-resolving %d non-deferred inputs for onSuccess function: %s",
					len(nonMemoryInputs), successFuncName)

				onSuccessInputs, err := t.inputFulfiller.FulfillInputs(
					ctx,
					messageID,
					clientID,
					t.tool,
					successFuncName,
					nonMemoryInputs,
				)

				if err != nil {
					if errors.Is(err, ErrInputFulfillmentPending) {
						// Check for missing required inputs (excluding deferred inputs we'll resolve later)
						hasRequiredMissing := false
						if missingInputsData, ok := onSuccessInputs["missingInputs"]; ok {
							if missingInputs, ok := missingInputsData.([]tool_protocol.MissingInputInfo); ok && len(missingInputs) > 0 {
								for _, mi := range missingInputs {
									// Check if this is a required input (not optional)
									for _, inputDef := range nonMemoryInputs {
										if inputDef.Name == mi.Name && !inputDef.IsOptional {
											hasRequiredMissing = true
											criticalMissingInputs = append(criticalMissingInputs,
												fmt.Sprintf("%s.%s: %s", successFuncName, mi.Name, mi.Message))
											logger.Errorf("OnSuccess function '%s' has missing required input '%s': %s",
												successFuncName, mi.Name, mi.Message)
											break
										}
									}
								}
							}
						}

						if hasRequiredMissing {
							logger.Errorf("OnSuccess function '%s' has missing required inputs", successFuncName)
						} else {
							// Only optional inputs missing, cache what we have
							if onSuccessInputs != nil {
								parsedOnSuccessInputs := ParseJSONInputs(onSuccessInputs)
								onSuccessInputsCache[successFuncName] = parsedOnSuccessInputs
								logger.Infof("Cached partial inputs for onSuccess function '%s'", successFuncName)
							}
						}
					} else {
						logger.Warnf("Failed to pre-resolve inputs for onSuccess function '%s': %v", successFuncName, err)
					}
				} else {
					// Successfully resolved non-deferred inputs, cache them
					parsedOnSuccessInputs := ParseJSONInputs(onSuccessInputs)
					onSuccessInputsCache[successFuncName] = parsedOnSuccessInputs

					// Don't store in callback yet if we have pending deferred inputs
					if len(memoryFromParentInputs) == 0 {
						// All inputs resolved, store in callback
						callback.OnInputsFulfilled(ctx, messageID, t.tool.Name, successFuncName, parsedOnSuccessInputs)
						logger.Infof("Successfully pre-resolved all inputs for onSuccess function '%s'", successFuncName)
					} else {
						logger.Infof("Pre-resolved %d non-deferred inputs for onSuccess function '%s', %d deferred inputs pending",
							len(nonMemoryInputs), successFuncName, len(memoryFromParentInputs))
					}
				}
			} else if len(memoryFromParentInputs) > 0 {
				// All inputs are deferred (memory/inference), will resolve after parent executes
				logger.Infof("All %d inputs for onSuccess function '%s' require parent output, will resolve after execution",
					len(memoryFromParentInputs), successFuncName)
			}
		}

		// Fail early if any onSuccess function has critical missing inputs (non-deferred)
		if len(criticalMissingInputs) > 0 {
			logger.Errorf("[ONSUCCESS-PRESOLVE] CRITICAL: %d missing inputs detected for onSuccess functions: %v", len(criticalMissingInputs), criticalMissingInputs)
			var sb strings.Builder
			sb.WriteString("I need some additional information to complete this action:\n\n")
			for _, missing := range criticalMissingInputs {
				sb.WriteString(fmt.Sprintf("- %s\n", missing))
			}
			sb.WriteString("\nPlease provide this information so I can continue.")

			output = sb.String()
			status = tool_protocol.StatusPending
			// Execute onMissingUserInfo callbacks before returning
			t.executeMagicStringCallbacks(ctx, messageID, clientID, tool_engine_utils.SpecialReturnNeedsInfo, inputs, output, callback)
			return output, nil
		}

		logger.Debugf("Completed pre-resolution of onSuccess function inputs (excluding deferred memory/inference inputs)")
	}

	// Step 6: Check cache for existing results
	// Skip if Step 0 already checked (non-global scope with includeInputs: false)
	step0AlreadyChecked := t.function.ParsedCache != nil &&
		t.function.ParsedCache.TTL > 0 &&
		t.function.ParsedCache.GetScope() != tool_protocol.CacheScopeGlobal &&
		!t.function.ParsedCache.GetIncludeInputs()

	if t.function.ParsedCache != nil && t.function.ParsedCache.TTL > 0 && !step0AlreadyChecked {
		logger.Debugf("Step 6: Cache check - TTL: %d seconds, scope: %s for %s.%s",
			t.function.ParsedCache.TTL, t.function.ParsedCache.GetScope(), t.tool.Name, t.function.Name)

		// Determine scope ID and inputs for cache key
		scope := t.function.ParsedCache.GetScope()
		scopeID := ""
		switch scope {
		case tool_protocol.CacheScopeClient:
			scopeID = clientID
		case tool_protocol.CacheScopeMessage:
			scopeID = messageID
		}

		// Validate scopeID for non-global scopes
		if scope != tool_protocol.CacheScopeGlobal && scopeID == "" {
			logger.Warnf("Step 6: Empty scopeID for %s scope - skipping cache check to prevent cache pollution", scope)
		} else {
			// Determine if we should include inputs in cache key
			var inputsForKey map[string]interface{}
			if t.function.ParsedCache.GetIncludeInputs() {
				inputsForKey = inputs
			}

			cachedOutput, found, err := t.executionTracker.GetFromScopedCache(ctx, t.tool.Name, t.function.Name, scope, scopeID, inputsForKey)
			if err != nil {
				logger.Warnf("Error checking scoped cache: %v", err)
			} else if found {
				output = cachedOutput
				logger.Infof("Cache HIT (scope: %s) - Using cached result for %s.%s (TTL: %d seconds)",
					scope, t.tool.Name, t.function.Name, t.function.ParsedCache.TTL)
				return output, nil
			}
		}
	}

	// Step 7: Check for team approval requirement
	logger.Debugf("Step 7: Team approval check - requires_approval: %t", t.function.RequiresTeamApproval)
	if t.function.RequiresTeamApproval {
		logger.Infof("Team approval required for %s.%s - checking approval status", t.tool.Name, t.function.Name)
		// First check if auto_approval is enabled
		db, dbErr := t.getDatabase()
		if dbErr != nil {
			logger.Warnf("Failed to get database for auto_approval check: %v", dbErr)
		} else {
			autoApproval, autoApprovalErr := db.GetAutoApprovalSetting(ctx)
			if autoApprovalErr != nil {
				logger.Warnf("Failed to get auto_approval setting: %v", autoApprovalErr)
			} else if autoApproval {
				// Auto approval is enabled, skip the approval process
				logger.Infof("Auto approval is enabled for %s.%s, proceeding without manual approval", t.tool.Name, t.function.Name)
				// Continue with execution - don't return here, let it proceed
			} else {
				// Auto approval is disabled, proceed with normal approval process
				logger.Debugf("Auto approval disabled, checking for existing team approval")
				stepKey, ok := ctx.Value(StepKeyInContextKey).(string)
				if !ok {
					stepKey = fmt.Sprintf("step_%d", time.Now().UnixNano())
					logger.Debugf("Generated step key for approval: %s", stepKey)
				}

				logger.Debugf("Checking team approval status for step: %s", stepKey)
				approved, existingCheckpoint, err := t.checkTeamApproval(ctx, clientID, messageID, t.tool.Name, t.function.Name, stepKey, inputs)
				if err != nil {
					logger.Errorf("Error checking team approval for %s.%s: %v", t.tool.Name, t.function.Name, err)
					output = fmt.Sprintf("Error checking team approval: %s", err.Error())
					status = tool_protocol.StatusFailed
					return output, nil
				}

				if approved {
					// Continue with execution
					logger.Infof("Team approval found for %s.%s, proceeding", t.tool.Name, t.function.Name)
				} else if existingCheckpoint != nil {
					// Still waiting for approval
					logger.Infof("Team approval pending for %s.%s - checkpoint exists", t.tool.Name, t.function.Name)
					output = "This action is pending team approval. You'll be notified once it's reviewed."
					status = tool_protocol.StatusPending
					// Execute onTeamApprovalRequest callbacks before returning
					t.executeMagicStringCallbacks(ctx, messageID, clientID, tool_engine_utils.SpecialReturnNeedsApproval, inputs, output, callback)
					return output, nil
				} else {
					// Create checkpoint and request approval will happen on agentic_coordinator once it recognizes the returned output as a pending approval request
					teamMessage, err := t.generateUserMessageRequestingApproval(ctx, inputs)
					if err != nil {
						output = fmt.Sprintf("Error generating team approval message: %s", err.Error())
						status = tool_protocol.StatusFailed
						return output, nil
					}

					output = fmt.Sprintf("%s|||%s", "This action requires team approval. I will let you know when i hear back from the team, please allow me some time. thank you", teamMessage)
					status = tool_protocol.StatusPending
					// Execute onTeamApprovalRequest callbacks before returning
					t.executeMagicStringCallbacks(ctx, messageID, clientID, tool_engine_utils.SpecialReturnNeedsApproval, inputs, output, callback)
					return output, nil
				}
			}
		}
	}

	// Step 8: Check requiresUserConfirmation with call-site override support
	// Check for call-site override first, then fall back to function-level setting
	requiresConfirmationRaw := t.function.RequiresUserConfirmation
	confirmFunctionKey := GenerateFunctionKey(t.tool.Name, t.function.Name)
	confirmOverrideKey := fmt.Sprintf("requiresUserConfirmation_override_%s_%s_%s", clientID, messageID, confirmFunctionKey)
	if overrideVal := ctx.Value(confirmOverrideKey); overrideVal != nil {
		requiresConfirmationRaw = overrideVal
		logger.Debugf("[USER_CONFIRM] Using call-site override for %s.%s", t.tool.Name, t.function.Name)
	}

	// Parse RequiresUserConfirmation field (can be bool or object)
	requiresConfirmation, customMessage, parseErr := tool_protocol.ParseRequiresUserConfirmation(requiresConfirmationRaw)
	logger.Debugf("Step 8: RequiresUserConfirmation - enabled: %t, hasCustomMessage: %t", requiresConfirmation, customMessage != "")
	if parseErr != nil {
		output = fmt.Sprintf("Error parsing RequiresUserConfirmation: %s", parseErr.Error())
		status = tool_protocol.StatusFailed
		return output, nil
	}

	if requiresConfirmation {
		ttlMinutes := 720

		// Check if user has already confirmed this action
		isConfirmed, existingState, err := t.checkUserConfirmation(ctx, clientID, t.tool.Name, t.function.Name, inputs, ttlMinutes)
		if err != nil {
			output = fmt.Sprintf("Error checking confirmation state: %s", err.Error())
			status = tool_protocol.StatusFailed
			return output, nil
		}

		// If already confirmed and not expired, proceed with execution
		if isConfirmed {
			logger.Infof("User confirmation found for %s.%s, proceeding with execution", t.tool.Name, t.function.Name)
			// Delete the confirmation state since it's been used (one-time use)
			if existingState != nil {
				repo, repoErr := t.getRepository()
				if repoErr != nil {
					logger.Warnf("Failed to get repository: %v", repoErr)
				} else if deleteErr := repo.DeleteConfirmationState(ctx, existingState.ID); deleteErr != nil {
					logger.Warnf("Failed to delete used confirmation state: %v", deleteErr)
				}
			}
			// Continue with normal execution (skip to cache check or execution)
		} else {
			// Generate confirmation message
			confirmationMessage, err := t.generateConfirmationMessage(ctx, messageID, inputs)
			if err != nil {
				output = fmt.Sprintf("Error generating confirmation message: %s", err.Error())
				status = tool_protocol.StatusFailed
				return output, nil
			}

			// Check if this is a new request or if we should try to extract confirmation from chat
			if existingState == nil {
				// Create new confirmation state
				inputsHash := generateInputsHash(inputs)

				logger.Infof("[INPUTSHASH DEBUG] Creating NEW ConfirmationState with inputsHash=%s for %s.%s (client=%s, message=%s)",
					inputsHash, t.tool.Name, t.function.Name, clientID, messageID)

				newState := &confirmationState{
					ClientID:         clientID,
					ToolName:         t.tool.Name,
					FunctionName:     t.function.Name,
					InputsHash:       inputsHash,
					ConfirmationText: confirmationMessage,
					PauseReason:      confirmationMessage, // Always save the generated confirmation message as pause reason
					IsConfirmed:      false,
					MessageID:        messageID,
					TTLMinutes:       ttlMinutes,
					CreatedAt:        time.Now(),
					UpdatedAt:        time.Now(),
				}

				repo, repoErr := t.getRepository()
				if repoErr != nil {
					logger.Warnf("Failed to get repository: %v", repoErr)
				} else if _, createErr := repo.CreateConfirmationState(ctx, newState); createErr != nil {
					logger.Warnf("Failed to create confirmation state: %v", createErr)
				} else {
					logger.Infof("[INPUTSHASH DEBUG] ✓ Successfully saved ConfirmationState with inputsHash=%s", inputsHash)
				}

				// First time asking - return confirmation request
				var sb strings.Builder
				sb.WriteString("i need the user confirmation before proceed:\n\n")

				// If custom message provided, use it; otherwise use generated confirmation message
				if customMessage != "" {
					sb.WriteString(customMessage)
				} else {
					sb.WriteString(confirmationMessage)
				}

				sb.WriteString("\nI need your confirmation so I can move forward. Please confirm so I can continue. (Request the confirmation, ask if i can move forward)")

				output = sb.String()
				status = tool_protocol.StatusPending
				// Execute onUserConfirmationRequest callbacks before returning
				t.executeMagicStringCallbacks(ctx, messageID, clientID, tool_engine_utils.SpecialReturnNeedsConfirmation, inputs, output, callback)
				return output, nil
			} else {
				// We have an existing state but user hasn't confirmed yet
				// Try to extract confirmation from recent chat
				logger.Infof("[CONFIRMATION LOOP DEBUG] Attempting to extract confirmation from chat for %s.%s (existingState.ID=%s, messageID=%s)",
					t.tool.Name, t.function.Name, existingState.ID, messageID)

				confirmed, extractErr := t.extractConfirmationFromChat(ctx, messageID, existingState.ConfirmationText)

				// DEBUG: Log the extraction result
				logger.Infof("[CONFIRMATION LOOP DEBUG] Extraction result: confirmed=%v, error=%v", confirmed, extractErr)

				if extractErr == nil && confirmed {
					// User confirmed! Update state and proceed
					now := time.Now()
					existingState.IsConfirmed = true
					existingState.ConfirmedAt = &now
					existingState.UpdatedAt = now

					repo, repoErr := t.getRepository()
					if repoErr != nil {
						logger.Warnf("Failed to get repository: %v", repoErr)
					} else if updateErr := repo.UpdateConfirmationState(ctx, existingState); updateErr != nil {
						logger.Warnf("Failed to update confirmation state: %v", updateErr)
					}

					logger.Infof("User confirmation extracted from chat for %s.%s, proceeding with execution", t.tool.Name, t.function.Name)

					// Delete the confirmation state since it's been used (one-time use)
					repo2, repoErr2 := t.getRepository()
					if repoErr2 != nil {
						logger.Warnf("Failed to get repository: %v", repoErr2)
					} else if deleteErr := repo2.DeleteConfirmationState(ctx, existingState.ID); deleteErr != nil {
						logger.Warnf("Failed to delete used confirmation state: %v", deleteErr)
					}

					// Continue with normal execution
				} else {
					// Still no clear confirmation - re-ask
					logger.Warnf("[CONFIRMATION LOOP DEBUG] Failed to extract confirmation (confirmed=%v, err=%v). RE-ASKING for confirmation (THIS CAUSES THE LOOP!)",
						confirmed, extractErr)

					var sb strings.Builder
					sb.WriteString("i need the user confirmation before proceed:\n\n")

					// If custom message provided, use it; otherwise use generated confirmation message
					if customMessage != "" {
						sb.WriteString(customMessage)
					} else {
						sb.WriteString(confirmationMessage)
					}

					sb.WriteString("\nPlease confirm so I can continue.(Request the confirmation, ask if i can move forward)")

					output = sb.String()
					status = tool_protocol.StatusPending
					// Execute onUserConfirmationRequest callbacks before returning
					t.executeMagicStringCallbacks(ctx, messageID, clientID, tool_engine_utils.SpecialReturnNeedsConfirmation, inputs, output, callback)
					return output, nil
				}
			}
		}
	}

	// Step 9: Execute the appropriate operation with retry logic
	// This includes reRunIf retry loop that allows re-execution based on output conditions
	logger.Debugf("Step 9: Operation execution phase - operation: %s, max_retries: %d", t.function.Operation, 1)
	var execErr error
	// Initialize stepResults BEFORE retry loop to preserve results between retries
	// This allows operations to skip steps that already completed successfully
	stepResults := make(map[int]interface{})
	var apiExecutionDetails *APIExecutionDetails // Capture API execution details for debugging
	var retryCount int = 0
	const maxRetries = 1

	// reRunIf retry loop variables
	reRunIfRetryCount := 0
	reRunIfConfig := t.function.ParsedReRunIf

	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}

	logger.Infof("Executing tool %s.%s", t.tool.Name, t.function.Name)

	// reRunIfLoop: Outer loop for reRunIf retry functionality
	// This loop allows the operation execution (Steps 9-12) to be repeated based on reRunIf conditions
reRunIfLoop:

	if db, dbErr := t.getDatabase(); dbErr == nil {
		go func() {
			defer func() {
				if r := recover(); r != nil {
					if logger != nil {
						logger.Errorf("Panic in audit logging: %v", r)
					}
				}
			}()
			_, err := db.CreateAudit(ctx, auditRecord{
				MessageID:   messageID,
				Action:      "tool_execution_started",
				Observation: fmt.Sprintf("Tool: %s.%s, Operation: %s, Inputs: %v", t.tool.Name, t.function.Name, t.function.Operation, inputs),
			})
			if err != nil {
				logger.WithError(err).Errorf("error creating audit for tool execution start")
			}
		}()
	}

	for retryCount <= maxRetries {
		if retryCount > 0 {
			logger.Warnf("Retry attempt %d of %d for %s.%s", retryCount, maxRetries, t.tool.Name, t.function.Name)
		}
		logger.Debugf("Executing operation: %s (attempt %d)", t.function.Operation, retryCount+1)
		switch t.function.Operation {
		case tool_protocol.OperationWeb:
			output, stepResults, execErr = t.executeWebBrowseOperation(ctx, messageID, inputs)
		case tool_protocol.OperationAPI:
			// Pass stepResults to allow skipping already-completed steps on retry
			output, stepResults, apiExecutionDetails, execErr = t.executeApiCallOperation(ctx, messageID, inputs, stepResults)
		case tool_protocol.OperationDesktop:
			output, stepResults, execErr = t.executeDesktopOperation(ctx, messageID, inputs)
		case tool_protocol.OperationMCP:
			output, stepResults, execErr = t.executeMCPOperation(ctx, messageID, inputs)
		case tool_protocol.OperationFormat:
			inputInference := tool_protocol.FindLastInferenceInputName(t.function.Input)
			if _, ok := inputs[inputInference]; !ok {
				output = fmt.Sprintf("Error: I am unable to complete this action now. i tried to find the input (%s) but not found as evaluated. all inputs below: \n %v", inputInference, inputs)
				status = tool_protocol.StatusFailed
				return output, nil
			}
			stepResults = make(map[int]interface{})
			stepResults[0] = inputs[inputInference]

			// Handle both string and JSON object inputs - convert JSON to string if needed
			if strValue, ok := inputs[inputInference].(string); ok {
				output = strValue
			} else {
				// If it's not a string, try to marshal it as JSON
				jsonBytes, err := json.Marshal(inputs[inputInference])
				if err != nil {
					output = fmt.Sprintf("Error: input %s could not be converted to string: %v", inputInference, err)
					status = tool_protocol.StatusFailed
					return output, nil
				}
				output = string(jsonBytes)
				logger.Debugf("Converted non-string input %s to JSON string for format operation", inputInference)
			}

			// Check if this input should be handled as message to user
			for _, input := range t.function.Input {
				if input.Name == inputInference && input.ShouldBeHandledAsMessageToUser {
					stepResults = make(map[int]interface{})
					// Add to gather info cache instead of prefixing output
					t.executionTracker.AddGatherInfo(messageID, output)
					break
				}
			}
		case tool_protocol.OperationDB:
			// Pass stepResults to allow skipping already-completed steps on retry
			output, stepResults, execErr = t.executeDBOperation(ctx, messageID, inputs, stepResults)
		case tool_protocol.OperationTerminal:
			// Pass stepResults to allow skipping already-completed steps on retry
			output, stepResults, execErr = t.executeTerminalOperation(ctx, messageID, inputs, stepResults)
		case tool_protocol.OperationInitiateWorkflow:
			output, stepResults, execErr = t.executeInitiateWorkflowOperation(ctx, messageID, inputs)
		case tool_protocol.OperationPolicy:
			// Policy operation returns a fixed value from output.value with variable substitution
			if t.function.Output == nil || t.function.Output.Value == "" {
				output = "Error: policy operation requires output.value to be defined"
				status = tool_protocol.StatusFailed
				return output, nil
			}

			// Perform variable replacement on output.value using inputs
			formattedValue, err := t.variableReplacer.ReplaceVariables(ctx, t.function.Output.Value, inputs)
			if err != nil {
				output = fmt.Sprintf("Error replacing variables in policy value: %v", err)
				status = tool_protocol.StatusFailed
				return output, nil
			}

			stepResults = make(map[int]interface{})
			stepResults[0] = formattedValue
			output = formattedValue
		case tool_protocol.OperationPDF:
			output, stepResults, execErr = t.executePDFOperation(ctx, messageID, inputs)
		case tool_protocol.OperationCode:
			output, stepResults, execErr = t.executeCodeOperation(ctx, messageID, inputs, stepResults)
		case tool_protocol.OperationGDrive:
			output, stepResults, execErr = t.executeGDriveOperation(ctx, messageID, inputs, stepResults)
		default:
			logger.Errorf("Unknown operation type: %s for %s.%s", t.function.Operation, t.tool.Name, t.function.Name)
			output = fmt.Sprintf("Unknown operation type: %s", t.function.Operation)
			execErr = nil
		}

		logger.Debugf("execution result for %s.%s: output: %s, error: %v", t.tool.Name, t.function.Name, tool_engine_utils.TruncateForLogging(output), execErr)

		// Store code execution debug details as original_output for runtime debugging via API
		if t.function.Operation == tool_protocol.OperationCode && t.codeDebugDetails != nil && len(t.codeDebugDetails.Steps) > 0 {
			codeDetailsJSON, codeErr := json.Marshal(t.codeDebugDetails)
			if codeErr != nil {
				logger.Warnf("Failed to marshal code execution debug details: %v", codeErr)
			} else {
				detailsStr := string(codeDetailsJSON)
				originalOutput = &detailsStr
				logger.Debugf("Stored code execution debug details as original_output — %d steps, length: %d", len(t.codeDebugDetails.Steps), len(detailsStr))
			}
			t.codeDebugDetails = nil // Reset for potential retry
		}

		// Store API execution details as original output immediately after operation completes
		// This ensures the original_output is saved even if there are early returns due to errors
		// On retry, APPEND to existing details rather than overwriting
		if t.function.Operation == tool_protocol.OperationAPI && apiExecutionDetails != nil && len(apiExecutionDetails.Steps) > 0 {
			// Check if we already have execution details from a previous attempt
			if originalOutput != nil && *originalOutput != "" {
				var existingDetails APIExecutionDetails
				if err := json.Unmarshal([]byte(*originalOutput), &existingDetails); err == nil {
					// Mark new steps as retry and append them to existing steps
					for i := range apiExecutionDetails.Steps {
						if !strings.HasSuffix(apiExecutionDetails.Steps[i].StepName, " (retry)") {
							apiExecutionDetails.Steps[i].StepName += " (retry)"
						}
					}
					existingDetails.Steps = append(existingDetails.Steps, apiExecutionDetails.Steps...)
					apiExecutionDetails = &existingDetails
				}
			}

			executionDetailsJSON, err := json.Marshal(apiExecutionDetails)
			if err != nil {
				logger.Warnf("Failed to marshal API execution details: %v", err)
			} else {
				detailsStr := string(executionDetailsJSON)
				originalOutput = &detailsStr
				logger.Debugf("Stored API execution details as original_output - %d steps, length: %d", len(apiExecutionDetails.Steps), len(detailsStr))
			}
		}

		// TODO: thats a nice idea but its not working as expected.
		//if output != "" {
		//	var msg string
		//	if execErr != nil {
		//		msg = fmt.Sprintf("output: %s \n executionError: %s", output, execErr)
		//	} else {
		//		msg = fmt.Sprintf("output: %s", output)
		//	}
		//
		//	logger.Debugf("Attempting to fix inputs based on output/error message")
		//	fixedInputs, shouldRetry, err := t.attemptToFixInputs(ctx, inputs, msg)
		//	if err != nil || !shouldRetry {
		//		logger.Debugf("Cannot fix inputs or retry not recommended - breaking retry loop")
		//		break
		//	}
		//
		//	logger.Infof("Fixed inputs successfully, retrying execution - attempt %d", retryCount+1)
		//	inputs = fixedInputs
		//	retryCount++
		//	continue
		//}

		if execErr != nil {
			if strings.Contains(execErr.Error(), "missing data in output") {
				var sb strings.Builder
				sb.WriteString("I need some additional information to complete this action:\n\n")
				sb.WriteString(output)
				sb.WriteString("\nPlease provide this information so I can continue.")

				output = sb.String()
				status = tool_protocol.StatusPending
				// Execute onMissingUserInfo callbacks before returning
				t.executeMagicStringCallbacks(ctx, messageID, clientID, tool_engine_utils.SpecialReturnNeedsInfo, inputs, output, callback)
				return output, nil
			}

			//fixedInputs, shouldRetry, err := t.attemptToFixInputs(ctx, inputs, execErr.Error())
			//if err != nil || !shouldRetry {
			//	// Cannot fix inputs, exit the retry loop
			//	break
			//}

			//inputs = fixedInputs
			retryCount++
			continue
		}

		// If we reach here, operation completed successfully (no error)
		// Break out of retry loop to prevent infinite loops
		logger.Debugf("Operation completed successfully without error - breaking retry loop")
		break
	}

	// Step 10: Handle final execution results
	logger.Debugf("Step 10: Final execution result processing - has_error: %t", execErr != nil)
	if execErr != nil {
		// Enhanced error message with more context
		output = fmt.Sprintf("Error executing operation: %s\n\nTool: %s\nFunction: %s\nRetry attempts: %d\nInputs: %v",
			execErr.Error(), t.tool.Name, t.function.Name, retryCount, inputs)
		status = tool_protocol.StatusFailed

		// Enhanced logging
		logger.WithFields(map[string]interface{}{
			"tool":       t.tool.Name,
			"function":   t.function.Name,
			"error":      execErr.Error(),
			"retryCount": retryCount,
			"inputs":     inputs,
		}).Errorf("Function execution failed")

		// Audit: Tool execution failed
		if db, dbErr := t.getDatabase(); dbErr == nil {
			go func() {
				defer func() {
					if r := recover(); r != nil {
						if logger != nil {
							logger.Errorf("Panic in audit logging: %v", r)
						}
					}
				}()
				_, err := db.CreateAudit(ctx, auditRecord{
					MessageID:   messageID,
					Action:      "tool_execution_failed",
					Observation: fmt.Sprintf("Tool: %s.%s, Error: %s, Retry count: %d", t.tool.Name, t.function.Name, execErr.Error(), retryCount),
				})
				if err != nil {
					logger.WithError(err).Errorf("error creating audit for tool execution failure")
				}
			}()
		}

		// Execute onFailure callbacks if defined - this allows fallback logic when operations fail
		// For example: createOrgForInbound.onFailure can call searchExistingOrgByName when org creation fails
		if len(t.function.OnFailure) > 0 {
			logger.Infof("Executing onFailure callbacks for %s.%s due to operation error", t.tool.Name, t.function.Name)
			output = t.executeOnFailureFunctions(ctx, messageID, clientID, output, originalOutput, inputs, inputsHash, startTime, callback)
			executionRecorded = true // Mark as recorded since executeOnFailureFunctions handles recording
		}

		return output, nil
	}
	output = strings.TrimSpace(output)
	// Step 10.5: Check for error patterns in output even when execErr is nil
	// This catches cases where the operation completes but returns error content
	logger.Debugf("Step 10.5: Checking output for error patterns")
	if t.detectErrorInOutput(output) {
		logger.Warnf("Error pattern detected in output for %s.%s - marking as failed", t.tool.Name, t.function.Name)
		status = tool_protocol.StatusFailed

		// Audit: Tool execution failed due to error patterns in output
		if db, dbErr := t.getDatabase(); dbErr == nil {
			go func() {
				defer func() {
					if r := recover(); r != nil {
						if logger != nil {
							logger.Errorf("Panic in audit logging: %v", r)
						}
					}
				}()
				_, err := db.CreateAudit(ctx, auditRecord{
					MessageID:   messageID,
					Action:      "tool_execution_failed",
					Observation: fmt.Sprintf("Tool: %s.%s, Error detected in output (pattern-based), Retry count: %d", t.tool.Name, t.function.Name, retryCount),
				})
				if err != nil {
					logger.WithError(err).Errorf("error creating audit for tool execution failure")
				}
			}()
		}

		// Execute onFailure callbacks if defined - allows fallback logic when error patterns detected in output
		if len(t.function.OnFailure) > 0 {
			logger.Infof("Executing onFailure callbacks for %s.%s due to error pattern in output", t.tool.Name, t.function.Name)
			output = t.executeOnFailureFunctions(ctx, messageID, clientID, output, originalOutput, inputs, inputsHash, startTime, callback)
			executionRecorded = true
		}

		return output, nil
	}
	logger.Debugf("No error patterns detected in output - execution successful")

	// Step 10.6: Check for magic strings in terminal operation output
	// Terminal operations may return output that contains magic strings, which should trigger callbacks
	if t.function.Operation == tool_protocol.OperationTerminal {
		specialReturn := tool_engine_utils.CheckSpecialReturn(output)
		if specialReturn.Type != tool_engine_utils.SpecialReturnNone {
			logger.Infof("Magic string detected in terminal output for %s.%s - type: %d", t.tool.Name, t.function.Name, specialReturn.Type)
			status = tool_protocol.StatusPending
			// Execute the appropriate callback based on the magic string type
			t.executeMagicStringCallbacks(ctx, messageID, clientID, specialReturn.Type, inputs, output, callback)
			return output, nil
		}
	}

	// Audit: Tool execution completed successfully
	if db, dbErr := t.getDatabase(); dbErr == nil {
		go func() {
			defer func() {
				if r := recover(); r != nil {
					if logger != nil {
						logger.Errorf("Panic in audit logging: %v", r)
					}
				}
			}()
			_, err := db.CreateAudit(ctx, auditRecord{
				MessageID:   messageID,
				Action:      "tool_execution_completed",
				Observation: fmt.Sprintf("Tool: %s.%s, Output length: %d chars, Step results: %d steps", t.tool.Name, t.function.Name, len(output), len(stepResults)),
			})
			if err != nil {
				logger.WithError(err).Errorf("error creating audit for tool execution completion")
			}
		}()
	}

	// Step 11: Record any step results
	// Note: We only record step results to the execution tracker, NOT append them to output.
	// Appending "Step N result: ..." to output pollutes the return value when functions
	// are used as inputs to other functions (e.g., forEach with origin: function).
	// The step results are accessible via stepResults map for output formatting.
	if stepResults != nil {
		for index, result := range stepResults {
			err := t.executionTracker.RecordStepResult(ctx, messageID, t.function.Name, index, result)
			if err != nil {
				fmt.Printf("Warning: Failed to record step result for index %d: %v\n", index, err)
			}
		}
	}

	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}

	// Step 12: Format the output based on the function's output definition
	// Only format output if execution was successful
	logger.Debugf("Step 12: Output formatting phase - has_output_definition: %t, raw_output_length: %d, status: %s", t.function.Output != nil, len(output), status)
	if t.function.Output != nil && status == tool_protocol.StatusComplete {
		// Capture original output before transformation
		rawOutput := output
		formattedOutput, err := t.outputFormatter.FormatOutput(ctx, messageID, output, stepResults, t.function.Output, inputs)
		if err != nil {
			// Log the error but still return the raw output
			fmt.Printf("Warning: Error formatting output: %v\n", err)
		} else {
			logger.Debugf("Output formatted successfully, checking output validity")
			revisedOutput, err := t.CheckOutput(ctx, t.function.Output, formattedOutput, fmt.Sprintf("%s \n\n %s", MapToString(stepResults), MapToString(inputs)))
			if err != nil {
				fmt.Printf("Warning: Error checking output: %v\n", err)
			} else {
				logger.Debugf("Output validated and revised - length: %d", len(revisedOutput))
				formattedOutput = revisedOutput
			}

			// Use only the formatted output when formatting succeeds
			// The raw output (step results) is already available separately via stepResults
			output = formattedOutput
			logger.Debugf("Using formatted output only - length: %d", len(output))

			// Store original output only if it's different from the formatted output
			if rawOutput != output {
				originalOutput = &rawOutput
			}
		}
	} else if t.function.Output != nil && status != tool_protocol.StatusComplete {
		logger.Debugf("Skipping output formatting and concatenation due to non-complete status: %s", status)
	}

	// Step 13: Cache results if caching is enabled
	if t.function.ParsedCache != nil && t.function.ParsedCache.TTL > 0 {
		logger.Debugf("Step 13: Caching results - TTL: %d seconds, scope: %s, output_length: %d",
			t.function.ParsedCache.TTL, t.function.ParsedCache.GetScope(), len(output))

		// Determine scope ID and inputs for cache key
		scope := t.function.ParsedCache.GetScope()
		scopeID := ""
		switch scope {
		case tool_protocol.CacheScopeClient:
			scopeID = clientID
		case tool_protocol.CacheScopeMessage:
			scopeID = messageID
		}

		// Validate scopeID for non-global scopes to prevent cache pollution
		if scope != tool_protocol.CacheScopeGlobal && scopeID == "" {
			logger.Warnf("Step 13: Empty scopeID for %s scope - skipping cache storage to prevent cache pollution", scope)
		} else {
			// Determine if we should include inputs in cache key
			var inputsForKey map[string]interface{}
			if t.function.ParsedCache.GetIncludeInputs() {
				inputsForKey = inputs
			}

			err := t.executionTracker.AddToScopedCache(ctx, t.tool.Name, t.function.Name, scope, scopeID, inputsForKey, output, t.function.ParsedCache.TTL)
			if err != nil {
				logger.Warnf("Error adding result to scoped cache: %v", err)
			} else {
				logger.Infof("Added result to scoped cache (scope: %s) for %s.%s (TTL: %d seconds)",
					scope, t.tool.Name, t.function.Name, t.function.ParsedCache.TTL)
			}
		}
	}

	// Step 13.5: Evaluate reRunIf condition for retry loop
	// This is evaluated AFTER output formatting and BEFORE afterSuccess hooks
	if reRunIfConfig != nil && reRunIfConfig.HasConditions() && status == tool_protocol.StatusComplete {
		logger.Debugf("Step 13.5: Evaluating reRunIf condition (attempt %d)", reRunIfRetryCount)

		shouldReRun, reason, reRunIfErr := t.evaluateReRunIf(
			ctx,
			messageID,
			clientID,
			reRunIfConfig,
			inputs,
			output,
			stepResults,
			reRunIfRetryCount,
			callback,
		)

		if reRunIfErr != nil {
			logger.Errorf("reRunIf evaluation error: %v", reRunIfErr)
			// Don't fail the execution, just log and continue
		} else if shouldReRun {
			reRunIfRetryCount++
			logger.Infof("reRunIf condition met, retry %d/%d: %s", reRunIfRetryCount, reRunIfConfig.GetMaxRetries(), reason)

			// Apply delay if configured
			if reRunIfConfig.DelayMs > 0 {
				logger.Debugf("reRunIf: applying delay of %d ms before retry", reRunIfConfig.DelayMs)
				time.Sleep(time.Duration(reRunIfConfig.DelayMs) * time.Millisecond)
			}

			// Handle scope
			scope := reRunIfConfig.GetScope()
			if scope == tool_protocol.ReRunScopeFull {
				// scope="full": Re-evaluate inputs and dependencies
				logger.Infof("reRunIf: scope=full, re-evaluating inputs and dependencies")

				// Re-execute dependencies
				if len(t.function.Needs) > 0 {
					allMet, _, err := t.toolEngine.ExecuteDependencies(ctx, messageID, clientID, t.tool, t.function.Needs, t.inputFulfiller, callback, t.function.Name)
					if err != nil {
						logger.Errorf("reRunIf: failed to re-execute dependencies: %v", err)
						// Don't retry on dependency error
					} else if !allMet {
						logger.Warnf("reRunIf: not all dependencies met on retry")
					}
				}

				// Re-fulfill inputs
				newInputs, inputErr := t.inputFulfiller.FulfillInputs(ctx, messageID, clientID, t.tool, t.function.Name, t.function.Input)
				if inputErr != nil {
					logger.Errorf("reRunIf: failed to re-fulfill inputs: %v", inputErr)
					// Don't retry on input error
				} else {
					inputs = ParseJSONInputs(newInputs)
					logger.Debugf("reRunIf: re-fulfilled %d inputs", len(inputs))
				}
			}

			// Apply param overrides (works with both scope: "steps" and scope: "full")
			if len(reRunIfConfig.Params) > 0 {
				logger.Debugf("reRunIf: applying %d param overrides", len(reRunIfConfig.Params))

				// Build context map (same as onSuccess callback context)
				reRunContext := make(map[string]interface{})
				for k, v := range inputs {
					reRunContext[k] = v
				}

				// Add result
				if output != "" {
					if parsed, ok := tryParseJSON(output); ok {
						reRunContext["result"] = parsed
					} else {
						reRunContext["result"] = output
					}
				}

				// Add step results as result[N]
				for idx, stepResult := range stepResults {
					reRunContext[fmt.Sprintf("result[%d]", idx)] = stepResult
				}

				// Add RETRY context
				reRunContext["RETRY"] = map[string]interface{}{"count": reRunIfRetryCount}

				// Process params using existing method (same as onSuccess)
				processedParams, err := t.processCallbackParams(ctx, reRunIfConfig.Params, reRunContext)
				if err != nil {
					logger.Errorf("reRunIf: failed to process params: %v", err)
				} else {
					// Override inputs with processed params
					for paramName, paramValue := range processedParams {
						inputs[paramName] = paramValue
						logger.Debugf("reRunIf: param %s = %v", paramName, paramValue)
					}
				}
			}

			// Clear step results for re-execution (both scopes)
			stepResults = make(map[int]interface{})

			// Reset error retry count
			retryCount = 0

			// Check context cancellation before retry
			select {
			case <-ctx.Done():
				logger.Infof("reRunIf: context cancelled, stopping retry loop")
				return "", ctx.Err()
			default:
			}

			// Jump back to reRunIfLoop to re-execute operation
			goto reRunIfLoop
		} else {
			logger.Debugf("reRunIf condition not met: %s", reason)
		}
	}

	// Execute afterSuccess hooks (before built-in onSuccess handlers)
	if status == tool_protocol.StatusComplete {
		if err := t.executeHooks(ctx, messageID, clientID, "afterSuccess", inputs, output, callback); err != nil {
			logger.Warnf("afterSuccess hooks error: %v", err)
		}
	}

	// Step 14: Execute onSuccess functions if defined (synchronously)
	if len(t.function.OnSuccess) > 0 && status == tool_protocol.StatusComplete {
		logger.Infof("Executing %d onSuccess functions for %s.%s", len(t.function.OnSuccess), t.tool.Name, t.function.Name)

		// CRITICAL: Save parent execution to database BEFORE onSuccess functions execute
		// This prevents race conditions where onSuccess functions try to look up parent execution
		// that hasn't been saved yet (due to defer func only executing after function returns)
		logger.Debugf("Saving parent function execution synchronously before onSuccess execution")

		// If there are skipped steps, include them in original_output
		if len(t.skippedSteps) > 0 {
			skippedJSON, jsonErr := json.Marshal(map[string]interface{}{
				"_skippedSteps": t.skippedSteps,
			})
			if jsonErr == nil {
				skippedStr := string(skippedJSON)
				originalOutput = &skippedStr
			}
		}

		err := t.executionTracker.RecordExecution(
			ctx,
			messageID,
			clientID,
			t.tool.Name,
			t.function.Name,
			t.Description(),
			inputs,
			inputsHash,
			output,
			originalOutput,
			startTime,
			status,
			t.function,
		)
		if err != nil {
			logger.Errorf("Failed to record function execution before onSuccess: %v", err)
			// Don't fail the entire execution, but log the error
		} else {
			executionRecorded = true // Mark as recorded to skip the defer
			logger.Debugf("Parent function execution saved successfully before onSuccess execution")
		}

		// COMMENTED OUT: Pre-resolution of deferred inputs for onSuccess functions
		//
		// REASON: This pre-resolution causes race conditions when inference/memory inputs reference
		// functions that are dependencies (via 'needs' field) or sibling onSuccess functions.
		//
		// PROBLEM SCENARIO:
		// 1. Parent function (e.g., ScheduleAppointment) completes
		// 2. This code tries to pre-resolve ALL deferred inputs for onSuccess functions
		// 3. If an onSuccess function (e.g., sendInternalSummaryEmail) has an inference input that
		//    references another function via 'from' field (e.g., formatInternalSummaryEmail)
		// 4. That referenced function hasn't executed yet (it's a dependency that runs during normal execution)
		// 5. The inference fails with "no execution found" because it's looking for data that doesn't exist yet
		//
		// SOLUTION: Let each onSuccess function follow its normal execution workflow:
		// - Step 1: Execute dependencies (line 234-237) - ensures 'needs' functions run first
		// - Step 2: Resolve ALL inputs naturally - by this point, all dependencies have executed
		// - Step 3: Execute the function
		//
		// This approach is simpler, avoids race conditions, and leverages existing caching mechanisms
		// in the input fulfiller. The pre-resolution was a premature optimization that added complexity
		// and caused bugs.
		//
		// for successFuncName, pendingMemoryInputs := range onSuccessPendingMemoryInputs {
		// 	if len(pendingMemoryInputs) > 0 {
		// 		logger.Debugf("Resolving %d pending deferred inputs for onSuccess function '%s'",
		// 			len(pendingMemoryInputs), successFuncName)
		//
		// 		// Get pre-cached non-deferred inputs if any
		// 		cachedInputs := onSuccessInputsCache[successFuncName]
		// 		if cachedInputs == nil {
		// 			cachedInputs = make(map[string]interface{})
		// 		}
		//
		// 		// Now resolve the deferred inputs (memory/inference)
		// 		memoryInputs, err := t.inputFulfiller.FulfillInputs(
		// 			ctx,
		// 			messageID,
		// 			clientID,
		// 			t.tool,
		// 			successFuncName,
		// 			pendingMemoryInputs,
		// 		)
		//
		// 		if err != nil {
		// 			logger.Errorf("Failed to resolve deferred inputs for onSuccess function '%s': %v",
		// 				successFuncName, err)
		// 			// Check if any are required
		// 			hasRequiredMissing := false
		// 			for _, input := range pendingMemoryInputs {
		// 				if !input.IsOptional {
		// 					hasRequiredMissing = true
		// 					break
		// 				}
		// 			}
		// 			if hasRequiredMissing {
		// 				logger.Errorf("OnSuccess function '%s' cannot execute due to missing required deferred inputs",
		// 					successFuncName)
		// 				continue // Skip this onSuccess function
		// 			}
		// 		} else {
		// 			// Merge deferred inputs with cached inputs
		// 			parsedMemoryInputs := ParseJSONInputs(memoryInputs)
		// 			for k, v := range parsedMemoryInputs {
		// 				cachedInputs[k] = v
		// 			}
		// 			onSuccessInputsCache[successFuncName] = cachedInputs
		//
		// 			// Store complete inputs in callback
		// 			callback.OnInputsFulfilled(ctx, messageID, t.tool.Name, successFuncName, cachedInputs)
		// 			logger.Infof("Resolved %d deferred inputs for onSuccess function '%s', now has complete inputs",
		// 				len(pendingMemoryInputs), successFuncName)
		// 		}
		// 	}
		// }

		// Track outputs from onSuccess siblings to enable chaining like $FuncA.field in FuncB's params
		onSuccessSiblingOutputs := make(map[string]interface{})

		for _, successCall := range t.function.OnSuccess {
			successFuncName := successCall.Name
			logger.Infof("[ONSUCCESS-TRACE] Processing onSuccess call - Name: '%s', HasParams: %v, ParamCount: %d",
				successFuncName, len(successCall.Params) > 0, len(successCall.Params))
			logger.Debugf("Executing onSuccess function: %s", successFuncName)

			// Evaluate runOnlyIf condition if present
			if successCall.RunOnlyIf != nil {
				shouldExecute, skipReason, evalErr := t.evaluateFunctionCallRunOnlyIf(ctx, messageID, successCall.RunOnlyIf, inputs, output, stepResults)
				if evalErr != nil {
					logger.Errorf("Error evaluating runOnlyIf for onSuccess function '%s': %v", successFuncName, evalErr)
					// Continue to next function - don't fail the parent
					continue
				}

				if !shouldExecute {
					logger.Infof("Skipping onSuccess function '%s' due to runOnlyIf condition: %s", successFuncName, skipReason)

					// Record the skip to function_execution table
					skipOutput := fmt.Sprintf("ON_SUCCESS_SKIPPED: runOnlyIf condition not met - %s", skipReason)
					if recordErr := t.executionTracker.RecordExecution(
						ctx,
						messageID,
						clientID,
						t.tool.Name,
						successFuncName,
						"onSuccess callback skipped",
						nil,
						"",
						skipOutput,
						nil,
						time.Now(),
						tool_protocol.StatusSkipped,
						nil,
					); recordErr != nil {
						logger.Warnf("Failed to record skipped onSuccess execution: %v", recordErr)
					}

					// Audit the skip
					if db, dbErr := t.getDatabase(); dbErr == nil {
						go func() {
							defer func() {
								if r := recover(); r != nil {
									if logger != nil {
										logger.Errorf("Panic in audit logging: %v", r)
									}
								}
							}()
							_, auditErr := db.CreateAudit(ctx, auditRecord{
								MessageID: messageID,
								Action:    "on_success_skipped",
								Observation: fmt.Sprintf("Skipped onSuccess function '%s' for %s.%s: runOnlyIf condition not met - %s",
									successFuncName, t.tool.Name, t.function.Name, skipReason),
							})
							if auditErr != nil {
								logger.WithError(auditErr).Errorf("error creating audit for onSuccess skip")
							}
						}()
					}
					continue // Skip this function
				}

				logger.Infof("runOnlyIf condition met for onSuccess function '%s', proceeding", successFuncName)
			}

			// Check if we have complete inputs (including resolved memory inputs) for this onSuccess function
			execCtx := ctx
			logger.Infof("[ONSUCCESS-DEBUG] Starting onSuccess execution for '%s', checking for cached inputs...", successFuncName)
			if cachedInputs, found := callback.GetFulfilledInputs(messageID, t.tool.Name, successFuncName); found {
				logger.Infof("[ONSUCCESS-DEBUG] Found %d cached inputs from callback for '%s': %v", len(cachedInputs), successFuncName, cachedInputs)
				// Add the pre-fulfilled inputs to the context using secure function-specific key
				// This must match the format used in input_fulfiller.go for consistency and security
				functionKey := GenerateFunctionKey(t.tool.Name, successFuncName)
				contextKey := fmt.Sprintf("pre_fulfilled_inputs_%s_%s_%s", clientID, messageID, functionKey)
				execCtx = context.WithValue(ctx, contextKey, cachedInputs)
				logger.Infof("[ONSUCCESS-DEBUG] Injected cached inputs into context with key '%s'", contextKey)
			} else if cachedInputs, exists := onSuccessInputsCache[successFuncName]; exists && len(cachedInputs) > 0 {
				// Use from our local cache if callback doesn't have it
				logger.Infof("[ONSUCCESS-DEBUG] Found %d cached inputs from local cache for '%s': %v", len(cachedInputs), successFuncName, cachedInputs)
				// Use secure function-specific context key instead of global key
				functionKey := GenerateFunctionKey(t.tool.Name, successFuncName)
				contextKey := fmt.Sprintf("pre_fulfilled_inputs_%s_%s_%s", clientID, messageID, functionKey)
				execCtx = context.WithValue(ctx, contextKey, cachedInputs)
				// Also store in callback for consistency
				callback.OnInputsFulfilled(ctx, messageID, t.tool.Name, successFuncName, cachedInputs)
				logger.Infof("[ONSUCCESS-DEBUG] Injected local cached inputs into context with key '%s'", contextKey)
			} else {
				logger.Infof("[ONSUCCESS-DEBUG] No cached inputs found for '%s'", successFuncName)
			}

			// Inject parent function's inputs into context for variable resolution
			// This ensures $varName params in onSuccess can be resolved from parent's inputs
			// Also add "result" which contains the parent function's output
			inputsWithResult := make(map[string]interface{})
			for k, v := range inputs {
				inputsWithResult[k] = v
			}
			// Add parent function's output as "result" for $result.field access in params
			if output != "" {
				if parsed, ok := tryParseJSON(output); ok {
					inputsWithResult["result"] = parsed
				} else {
					inputsWithResult["result"] = output
				}
			}
			// Add step results as result[N] for onSuccess params (same as reRunIf)
			logger.Infof("[ONSUCCESS-CONTEXT] Adding %d stepResults to inputsWithResult for '%s'", len(stepResults), successFuncName)
			for idx, stepResult := range stepResults {
				key := fmt.Sprintf("result[%d]", idx)
				inputsWithResult[key] = stepResult
				logger.Infof("[ONSUCCESS-CONTEXT] Added %s = %T (value: %v)", key, stepResult, stepResult)
			}
			// Add previous onSuccess sibling outputs for $SiblingFunc.field access in params
			// This enables chaining like: JoinMeetingBot runs first, then storeAppointmentInCache uses $JoinMeetingBot.botId
			if len(onSuccessSiblingOutputs) > 0 {
				for k, v := range onSuccessSiblingOutputs {
					inputsWithResult[k] = v
				}
				logger.Infof("[ONSUCCESS] Merged %d sibling outputs into inputsWithResult: %v", len(onSuccessSiblingOutputs), onSuccessSiblingOutputs)
			}

			// Handle forEach iteration if present
			if successCall.ForEach != nil {
				forEachResults, forEachErr := t.executeCallbackWithForEach(ctx, messageID, clientID, successCall, inputsWithResult, onSuccessSiblingOutputs, callback, "onSuccess")

				// Handle ErrInputFulfillmentPending - surface to caller
				if errors.Is(forEachErr, ErrInputFulfillmentPending) {
					logger.Warnf("OnSuccess forEach for '%s' has pending inputs", successFuncName)
					return output, ErrInputFulfillmentPending
				}

				if forEachErr != nil {
					logger.Errorf("Error executing onSuccess forEach for '%s': %v", successFuncName, forEachErr)
				}

				// Store forEach results for sibling output access (subsequent callbacks can use $successFuncName)
				if len(forEachResults) > 0 {
					onSuccessSiblingOutputs[successFuncName] = forEachResults
					logger.Infof("[ONSUCCESS-FOREACH] Stored %d results for sibling access under key '%s'",
						len(forEachResults), successFuncName)
				}

				// Continue to next onSuccess callback - forEach handles its own iterations
				continue
			}

			parentFunctionKey := GenerateFunctionKey(t.tool.Name, t.function.Name)
			parentContextKey := fmt.Sprintf("pre_fulfilled_inputs_%s_%s_%s", clientID, messageID, parentFunctionKey)
			execCtx = context.WithValue(execCtx, parentContextKey, inputsWithResult)
			logger.Infof("[ONSUCCESS] Injected parent inputs for '%s.%s' into context with key '%s' (%d inputs including result: %v)",
				t.tool.Name, t.function.Name, parentContextKey, len(inputsWithResult), inputsWithResult)

			// Execute the onSuccess function using the tool engine
			logger.Infof("[ONSUCCESS-DEBUG] Calling ExecuteDependencies for '%s' with params: %v", successFuncName, successCall.Params)
			allMet, successOutput, err := t.toolEngine.ExecuteDependencies(
				execCtx,
				messageID,
				clientID,
				t.tool,
				[]tool_protocol.NeedItem{{Name: successFuncName, Params: successCall.Params, ShouldBeHandledAsMessageToUser: successCall.ShouldBeHandledAsMessageToUser, RequiresUserConfirmation: successCall.RequiresUserConfirmation}},
				t.inputFulfiller,
				callback,
				t.function.Name,
			)

			if err != nil {
				// Check if error is due to pending inputs (input fulfillment pending)
				if errors.Is(err, ErrInputFulfillmentPending) {
					logger.Warnf("OnSuccess function '%s' has pending inputs and cannot be executed",
						successFuncName)
					// Return error to caller indicating onSuccess function needs input
					// This will surface to the user as a pending status
					return output + "\n\n" + strings.Join(successOutput, "\n\n"), ErrInputFulfillmentPending
				}

				// Log error but don't fail the parent function
				logger.Errorf("Error executing onSuccess function '%s' for %s.%s: %v",
					successFuncName, t.tool.Name, t.function.Name, err)

				// Audit the failure
				if db, dbErr := t.getDatabase(); dbErr == nil {
					go func() {
						defer func() {
							if r := recover(); r != nil {
								if logger != nil {
									logger.Errorf("Panic in audit logging: %v", r)
								}
							}
						}()
						_, auditErr := db.CreateAudit(ctx, auditRecord{
							MessageID: messageID,
							Action:    "on_success_execution_failed",
							Observation: fmt.Sprintf("Failed to execute onSuccess function '%s' for %s.%s: %v",
								successFuncName, t.tool.Name, t.function.Name, err),
						})
						if auditErr != nil {
							logger.WithError(auditErr).Errorf("error creating audit for onSuccess failure")
						}
					}()
				}
			} else {
				// Check if onSuccess function returned special messages even without error
				if !allMet && len(successOutput) > 0 {
					// Check each result for special return messages
					for _, result := range successOutput {
						if tool_engine_utils.IsSpecialReturn(result) {
							return output + "\n\n" + strings.Join(successOutput, "\n\n"), ErrInputFulfillmentPending
						}
					}
				}

				logger.Infof("Successfully executed onSuccess function '%s' for %s.%s",
					successFuncName, t.tool.Name, t.function.Name)

				// Log the results (no longer concatenating to parent output)
				if len(successOutput) > 0 {
					for i, result := range successOutput {
						if result != "" {
							logger.Debugf("OnSuccess function '%s' result[%d]: %s", successFuncName, i, result)
						}
					}
				}

				// Store this function's output for subsequent onSuccess siblings
				// Try to parse as JSON to enable field access like $FuncA.field
				if len(successOutput) > 0 && successOutput[0] != "" {
					if parsed, ok := tryParseJSON(successOutput[0]); ok {
						onSuccessSiblingOutputs[successFuncName] = parsed
						logger.Infof("[ONSUCCESS] Stored sibling output for '%s' (parsed JSON): %v", successFuncName, parsed)
					} else {
						onSuccessSiblingOutputs[successFuncName] = successOutput[0]
						logger.Infof("[ONSUCCESS] Stored sibling output for '%s' (string): %s", successFuncName, successOutput[0])
					}
				}

				// Audit the success
				if db, dbErr := t.getDatabase(); dbErr == nil {
					go func() {
						defer func() {
							if r := recover(); r != nil {
								if logger != nil {
									logger.Errorf("Panic in audit logging: %v", r)
								}
							}
						}()
						auditObservation := fmt.Sprintf("Successfully executed onSuccess function '%s' for %s.%s",
							successFuncName, t.tool.Name, t.function.Name)
						if !allMet {
							auditObservation += " (with special return messages)"
						}
						_, auditErr := db.CreateAudit(ctx, auditRecord{
							MessageID:   messageID,
							Action:      "on_success_executed",
							Observation: auditObservation,
						})
						if auditErr != nil {
							logger.WithError(auditErr).Errorf("error creating audit for onSuccess execution")
						}
					}()
				}
			}
		}

		logger.Debugf("Completed execution of all onSuccess functions for %s.%s", t.tool.Name, t.function.Name)
	}

	// Step 15: Successful completion
	executionDuration := time.Since(startTime)
	logger.Infof("YAMLDefinedTool.Call completed successfully for %s.%s - duration: %v, output_length: %d",
		t.tool.Name, t.function.Name, executionDuration, len(output))

	// Set response language override if specified on the function
	if t.function.ResponseLanguage != "" {
		resolvedLang, err := t.variableReplacer.ReplaceVariables(ctx, t.function.ResponseLanguage, inputs)
		if err == nil && resolvedLang != "" {
			t.executionTracker.SetResponseLanguage(messageID, resolvedLang)
			logger.Debugf("[RESPONSE_LANG] Set response language override for %s.%s: %s", t.tool.Name, t.function.Name, resolvedLang)
		}
	}

	// Add to gather info cache if shouldBeHandledAsMessageToUser is enabled
	// Check for call-site override first, then fall back to function-level setting
	shouldAddToGatherInfo := t.function.ShouldBeHandledAsMessageToUser
	functionKey := GenerateFunctionKey(t.tool.Name, t.function.Name)
	overrideKey := fmt.Sprintf("shouldBeHandledAsMessageToUser_override_%s_%s_%s", clientID, messageID, functionKey)
	if overrideVal, ok := ctx.Value(overrideKey).(bool); ok {
		shouldAddToGatherInfo = overrideVal
		logger.Debugf("[GATHER_INFO] Using call-site override for %s.%s: shouldBeHandledAsMessageToUser=%v",
			t.tool.Name, t.function.Name, overrideVal)
	}

	// For PDF operations with shouldBeHandledAsMessageToUser, the file is already added to filesToShare
	// (for attachment), so we skip adding to gatherInfo to avoid duplicate text output
	if shouldAddToGatherInfo && t.function.Operation == tool_protocol.OperationPDF {
		logger.Debugf("[GATHER_INFO] Skipping gatherInfo for PDF operation %s.%s - file already added to filesToShare",
			t.tool.Name, t.function.Name)
		shouldAddToGatherInfo = false
	}

	if shouldAddToGatherInfo {
		t.executionTracker.AddGatherInfo(messageID, output)
	}

	// Execute afterCompletion hooks (always fires, regardless of success/failure)
	if err := t.executeHooks(ctx, messageID, clientID, "afterCompletion", inputs, output, callback); err != nil {
		logger.Warnf("afterCompletion hooks error: %v", err)
	}

	// Success case - status remains tool_protocol.StatusComplete
	return output, nil
}

// executeOnFailureFunctions executes the onFailure callback functions when a function fails
func (t *YAMLDefinedTool) executeOnFailureFunctions(
	ctx context.Context,
	messageID string,
	clientID string,
	output string,
	originalOutput *string,
	inputs map[string]interface{},
	inputsHash string,
	startTime time.Time,
	callback *ExecutorAgenticWorkflowCallback,
) string {

	if len(t.function.OnFailure) == 0 {
		// Execute afterCompletion hooks even when there are no onFailure handlers
		if err := t.executeHooks(ctx, messageID, clientID, "afterCompletion", inputs, output, callback); err != nil {
			logger.Warnf("afterCompletion hooks error: %v", err)
		}
		return output
	}

	logger.Infof("Executing %d onFailure functions for %s.%s", len(t.function.OnFailure), t.tool.Name, t.function.Name)

	// Save parent execution to database BEFORE onFailure functions execute
	logger.Debugf("Saving parent function execution synchronously before onFailure execution")
	err := t.executionTracker.RecordExecution(
		ctx,
		messageID,
		clientID,
		t.tool.Name,
		t.function.Name,
		t.Description(),
		inputs,
		inputsHash,
		output,
		originalOutput,
		startTime,
		tool_protocol.StatusFailed,
		t.function,
	)
	if err != nil {
		logger.Errorf("Failed to record function execution before onFailure: %v", err)
	} else {
		logger.Debugf("Parent function execution saved successfully before onFailure execution")
	}

	// Execute afterFailure hooks (before built-in onFailure handlers)
	if err := t.executeHooks(ctx, messageID, clientID, "afterFailure", inputs, output, callback); err != nil {
		logger.Warnf("afterFailure hooks error: %v", err)
	}

	for _, failureCall := range t.function.OnFailure {
		failureFuncName := failureCall.Name
		logger.Debugf("Executing onFailure function: %s", failureFuncName)

		// Evaluate runOnlyIf condition if present
		if failureCall.RunOnlyIf != nil {
			shouldExecute, skipReason, evalErr := t.evaluateFunctionCallRunOnlyIf(ctx, messageID, failureCall.RunOnlyIf, inputs, output, nil)
			if evalErr != nil {
				logger.Errorf("Error evaluating runOnlyIf for onFailure function '%s': %v", failureFuncName, evalErr)
				// Continue to next function - don't fail the parent
				continue
			}

			if !shouldExecute {
				logger.Infof("Skipping onFailure function '%s' due to runOnlyIf condition: %s", failureFuncName, skipReason)

				// Record the skip to function_execution table
				skipOutput := fmt.Sprintf("ON_FAILURE_SKIPPED: runOnlyIf condition not met - %s", skipReason)
				if recordErr := t.executionTracker.RecordExecution(
					ctx,
					messageID,
					clientID,
					t.tool.Name,
					failureFuncName,
					"onFailure callback skipped",
					nil,
					"",
					skipOutput,
					nil,
					time.Now(),
					tool_protocol.StatusSkipped,
					nil,
				); recordErr != nil {
					logger.Warnf("Failed to record skipped onFailure execution: %v", recordErr)
				}

				// Audit the skip
				if db, dbErr := t.getDatabase(); dbErr == nil {
					go func() {
						defer func() {
							if r := recover(); r != nil {
								if logger != nil {
									logger.Errorf("Panic in audit logging: %v", r)
								}
							}
						}()
						_, auditErr := db.CreateAudit(ctx, auditRecord{
							MessageID: messageID,
							Action:    "on_failure_skipped",
							Observation: fmt.Sprintf("Skipped onFailure function '%s' for %s.%s: runOnlyIf condition not met - %s",
								failureFuncName, t.tool.Name, t.function.Name, skipReason),
						})
						if auditErr != nil {
							logger.WithError(auditErr).Errorf("error creating audit for onFailure skip")
						}
					}()
				}
				continue // Skip this function
			}

			logger.Infof("runOnlyIf condition met for onFailure function '%s', proceeding", failureFuncName)
		}

		// Inject parent function's inputs into context for variable resolution
		// This ensures $varName params in onFailure can be resolved from parent's inputs
		// Also add "result" which contains the error information from the parent function
		inputsWithResult := make(map[string]interface{})
		for k, v := range inputs {
			inputsWithResult[k] = v
		}
		// Add error info as "result" for $result.error access in onFailure params
		// For onFailure, result contains the error details
		inputsWithResult["result"] = map[string]interface{}{
			"error":   output, // output contains error message in failure case
			"success": false,
		}

		// Handle forEach iteration if present
		if failureCall.ForEach != nil {
			// Note: onFailure doesn't have sibling chaining like onSuccess, so pass empty map
			_, forEachErr := t.executeCallbackWithForEach(ctx, messageID, clientID, failureCall, inputsWithResult, nil, callback, "onFailure")

			// For onFailure, we log pending inputs but continue (can't return error from this function)
			if errors.Is(forEachErr, ErrInputFulfillmentPending) {
				logger.Warnf("OnFailure forEach for '%s' has pending inputs - some iterations may be incomplete", failureFuncName)
			} else if forEachErr != nil {
				logger.Errorf("Error executing onFailure forEach for '%s': %v", failureFuncName, forEachErr)
			}
			// Continue to next onFailure callback - forEach handles its own iterations
			continue
		}

		parentFunctionKey := GenerateFunctionKey(t.tool.Name, t.function.Name)
		parentContextKey := fmt.Sprintf("pre_fulfilled_inputs_%s_%s_%s", clientID, messageID, parentFunctionKey)
		execCtx := context.WithValue(ctx, parentContextKey, inputsWithResult)
		logger.Debugf("[ONFAILURE] Injected parent inputs for '%s.%s' with key '%s' (%d inputs + result)", t.tool.Name, t.function.Name, parentContextKey, len(inputs))

		// Execute the onFailure function using the tool engine
		allMet, failureOutput, err := t.toolEngine.ExecuteDependencies(
			execCtx,
			messageID,
			clientID,
			t.tool,
			[]tool_protocol.NeedItem{{Name: failureFuncName, Params: failureCall.Params, ShouldBeHandledAsMessageToUser: failureCall.ShouldBeHandledAsMessageToUser, RequiresUserConfirmation: failureCall.RequiresUserConfirmation}},
			t.inputFulfiller,
			callback,
			t.function.Name,
		)

		if err != nil {
			// Check if error is due to pending inputs
			if errors.Is(err, ErrInputFulfillmentPending) {
				logger.Warnf("OnFailure function '%s' has pending inputs and cannot be executed", failureFuncName)
				continue
			}

			// Log error but don't fail the parent function
			logger.Errorf("Error executing onFailure function '%s' for %s.%s: %v",
				failureFuncName, t.tool.Name, t.function.Name, err)

			// Audit the failure
			if db, dbErr := t.getDatabase(); dbErr == nil {
				go func() {
					defer func() {
						if r := recover(); r != nil {
							if logger != nil {
								logger.Errorf("Panic in audit logging: %v", r)
							}
						}
					}()
					_, auditErr := db.CreateAudit(ctx, auditRecord{
						MessageID: messageID,
						Action:    "on_failure_execution_failed",
						Observation: fmt.Sprintf("Failed to execute onFailure function '%s' for %s.%s: %v",
							failureFuncName, t.tool.Name, t.function.Name, err),
					})
					if auditErr != nil {
						logger.WithError(auditErr).Errorf("error creating audit for onFailure failure")
					}
				}()
			}
		} else {
			// Check if onFailure function returned special messages
			if !allMet && len(failureOutput) > 0 {
				for _, result := range failureOutput {
					if tool_engine_utils.IsSpecialReturn(result) {
						logger.Warnf("OnFailure function '%s' returned special message, skipping", failureFuncName)
						continue
					}
				}
			}

			logger.Infof("Successfully executed onFailure function '%s' for %s.%s",
				failureFuncName, t.tool.Name, t.function.Name)

			// Log the results (no longer concatenating to parent output)
			if len(failureOutput) > 0 {
				for i, result := range failureOutput {
					if result != "" {
						logger.Debugf("OnFailure function '%s' result[%d]: %s", failureFuncName, i, result)
					}
				}
			}

			// Audit the success
			if db, dbErr := t.getDatabase(); dbErr == nil {
				go func() {
					defer func() {
						if r := recover(); r != nil {
							if logger != nil {
								logger.Errorf("Panic in audit logging: %v", r)
							}
						}
					}()
					auditObservation := fmt.Sprintf("Successfully executed onFailure function '%s' for %s.%s",
						failureFuncName, t.tool.Name, t.function.Name)
					if !allMet {
						auditObservation += " (with special return messages)"
					}
					_, auditErr := db.CreateAudit(ctx, auditRecord{
						MessageID:   messageID,
						Action:      "on_failure_executed",
						Observation: auditObservation,
					})
					if auditErr != nil {
						logger.WithError(auditErr).Errorf("error creating audit for onFailure execution")
					}
				}()
			}
		}
	}

	logger.Debugf("Completed execution of all onFailure functions for %s.%s", t.tool.Name, t.function.Name)

	// Execute afterCompletion hooks (fires after all onFailure handlers)
	if err := t.executeHooks(ctx, messageID, clientID, "afterCompletion", inputs, output, callback); err != nil {
		logger.Warnf("afterCompletion hooks error: %v", err)
	}

	return output
}

// executeCallbackWithForEach executes a callback function (onSuccess/onFailure) with forEach iteration.
// It iterates over the items array specified in forEach.Items, executing the callback function for each item.
// Available variables in params: $item (current item), $index (current index), $result (parent output),
// and all parent function inputs.
// Returns: (aggregated results as []interface{}, error)
// The aggregated results can be used by subsequent callbacks via sibling output access.
func (t *YAMLDefinedTool) executeCallbackWithForEach(
	ctx context.Context,
	messageID string,
	clientID string,
	callbackCall tool_protocol.FunctionCall,
	inputsWithResult map[string]interface{},
	siblingOutputs map[string]interface{},
	callback *ExecutorAgenticWorkflowCallback,
	callbackType string,
) ([]interface{}, error) {

	forEach := callbackCall.ForEach
	callbackFuncName := callbackCall.Name

	logger.Infof("[%s-FOREACH] Starting forEach execution for callback '%s' with items: %s",
		strings.ToUpper(callbackType), callbackFuncName, forEach.Items)

	// Resolve the items array from inputsWithResult
	itemsPath := strings.TrimPrefix(forEach.Items, "$")
	rawItemsValue, err := t.variableReplacer.NavigatePathWithTransformation(inputsWithResult, itemsPath)
	if err != nil || rawItemsValue == nil {
		logger.Warnf("[%s-FOREACH] Variable '%s' not found in inputs: %v", strings.ToUpper(callbackType), forEach.Items, err)
		// Return nil - don't fail the parent function if items not found
		return nil, nil
	}

	// Convert to []interface{}
	items, err := t.resolveForEachItems(rawItemsValue, forEach.Separator)
	if err != nil {
		logger.Errorf("[%s-FOREACH] Error resolving items: %v", strings.ToUpper(callbackType), err)
		return nil, nil
	}

	if len(items) == 0 {
		logger.Infof("[%s-FOREACH] No items to iterate over for callback '%s'", strings.ToUpper(callbackType), callbackFuncName)
		return nil, nil
	}

	logger.Infof("[%s-FOREACH] Iterating over %d items for callback '%s'", strings.ToUpper(callbackType), len(items), callbackFuncName)

	// Get variable names (use defaults if not specified)
	indexVar := forEach.IndexVar
	if indexVar == "" {
		indexVar = tool_protocol.DefaultForEachIndexVar
	}
	itemVar := forEach.ItemVar
	if itemVar == "" {
		itemVar = tool_protocol.DefaultForEachItemVar
	}

	// Accumulate results from each iteration for sibling output access
	aggregatedResults := make([]interface{}, 0, len(items))

	// Execute sequentially for each item
	for i, item := range items {
		logger.Debugf("[%s-FOREACH] Processing iteration %d/%d for callback '%s'",
			strings.ToUpper(callbackType), i+1, len(items), callbackFuncName)

		// Build loop inputs: merge parent inputs with $item and $index
		loopInputs := make(map[string]interface{})
		for k, v := range inputsWithResult {
			loopInputs[k] = v
		}
		// Add sibling outputs (including previous forEach results)
		for k, v := range siblingOutputs {
			loopInputs[k] = v
		}
		// Add loop variables
		loopInputs[itemVar] = item
		loopInputs[indexVar] = i

		// Process params with loop context
		loopContext := &tool_engine_models.LoopContext{
			IndexVar: indexVar,
			ItemVar:  itemVar,
			Index:    i,
			Item:     item,
		}

		// Evaluate per-item runOnlyIf if present
		if callbackCall.RunOnlyIf != nil {
			shouldRun, skipReason := t.evaluateRunOnlyIfWithLoop(ctx, callbackCall.RunOnlyIf, loopInputs, loopContext)
			if !shouldRun {
				logger.Infof("[%s-FOREACH] Skipping iteration %d for callback '%s': %s",
					strings.ToUpper(callbackType), i, callbackFuncName, skipReason)
				aggregatedResults = append(aggregatedResults, nil) // Preserve index alignment
				continue
			}
		}

		processedParams, err := t.processCallbackParamsWithLoop(ctx, callbackCall.Params, loopInputs, loopContext)
		if err != nil {
			logger.Errorf("[%s-FOREACH] Error processing params for iteration %d: %v",
				strings.ToUpper(callbackType), i, err)
			aggregatedResults = append(aggregatedResults, nil)
			continue
		}

		// Check for cached inputs for the callback function
		execCtx := ctx
		if cachedInputs, found := callback.GetFulfilledInputs(messageID, t.tool.Name, callbackFuncName); found {
			logger.Debugf("[%s-FOREACH] Found %d cached inputs for callback '%s' iteration %d",
				strings.ToUpper(callbackType), len(cachedInputs), callbackFuncName, i)
			// Merge cached inputs with loop inputs (loop inputs take precedence)
			for k, v := range cachedInputs {
				if _, exists := loopInputs[k]; !exists {
					loopInputs[k] = v
				}
			}
		}

		// Inject context via pre_fulfilled_inputs key for both parent function and callback function
		parentFunctionKey := GenerateFunctionKey(t.tool.Name, t.function.Name)
		parentContextKey := fmt.Sprintf("pre_fulfilled_inputs_%s_%s_%s", clientID, messageID, parentFunctionKey)
		execCtx = context.WithValue(execCtx, parentContextKey, loopInputs)

		// Also inject for the callback function itself (for input fulfillment)
		callbackFunctionKey := GenerateFunctionKey(t.tool.Name, callbackFuncName)
		callbackContextKey := fmt.Sprintf("pre_fulfilled_inputs_%s_%s_%s", clientID, messageID, callbackFunctionKey)
		execCtx = context.WithValue(execCtx, callbackContextKey, loopInputs)

		// Execute the callback function
		_, callbackOutput, execErr := t.toolEngine.ExecuteDependencies(
			execCtx,
			messageID,
			clientID,
			t.tool,
			[]tool_protocol.NeedItem{{
				Name:                           callbackFuncName,
				Params:                         processedParams,
				ShouldBeHandledAsMessageToUser: callbackCall.ShouldBeHandledAsMessageToUser,
				RequiresUserConfirmation:       callbackCall.RequiresUserConfirmation,
			}},
			t.inputFulfiller,
			callback,
			t.function.Name,
		)

		if execErr != nil {
			// Check if error is due to pending inputs - surface this to caller
			if errors.Is(execErr, ErrInputFulfillmentPending) {
				logger.Warnf("[%s-FOREACH] Callback '%s' iteration %d has pending inputs",
					strings.ToUpper(callbackType), callbackFuncName, i)
				return aggregatedResults, ErrInputFulfillmentPending
			}

			logger.Errorf("[%s-FOREACH] Error executing callback '%s' for iteration %d: %v",
				strings.ToUpper(callbackType), callbackFuncName, i, execErr)
			aggregatedResults = append(aggregatedResults, nil)
			continue
		}

		// Parse and store the callback output for sibling access
		var iterationResult interface{}
		if len(callbackOutput) > 0 {
			outputStr := strings.Join(callbackOutput, "\n")
			if err := json.Unmarshal([]byte(outputStr), &iterationResult); err != nil {
				iterationResult = outputStr // Use as string if not valid JSON
			}
		}
		aggregatedResults = append(aggregatedResults, iterationResult)

		logger.Debugf("[%s-FOREACH] Successfully executed callback '%s' for iteration %d, output: %v",
			strings.ToUpper(callbackType), callbackFuncName, i, iterationResult)
	}

	logger.Infof("[%s-FOREACH] Completed forEach execution for callback '%s' (%d iterations, %d results)",
		strings.ToUpper(callbackType), callbackFuncName, len(items), len(aggregatedResults))

	return aggregatedResults, nil
}

// evaluateRunOnlyIfWithLoop evaluates runOnlyIf condition with loop context variables.
// Supports deterministic expressions with $item and $index variables.
// Returns (shouldRun bool, skipReason string).
func (t *YAMLDefinedTool) evaluateRunOnlyIfWithLoop(
	ctx context.Context,
	runOnlyIf interface{},
	inputs map[string]interface{},
	loopContext *tool_engine_models.LoopContext,
) (bool, string) {

	// Parse runOnlyIf using the standard parser
	runOnlyIfObj, err := tool_protocol.ParseRunOnlyIf(runOnlyIf)
	if err != nil {
		logger.Warnf("Error parsing runOnlyIf in forEach: %v", err)
		return true, "" // Default to running if condition can't be parsed
	}

	if runOnlyIfObj == nil {
		return true, "" // No condition, always execute
	}

	// Only deterministic mode is supported for forEach runOnlyIf
	if runOnlyIfObj.Deterministic == "" {
		logger.Warnf("forEach runOnlyIf requires 'deterministic' expression, got: %+v", runOnlyIfObj)
		return true, ""
	}

	originalExpr := runOnlyIfObj.Deterministic
	logger.Debugf("Evaluating forEach runOnlyIf deterministic condition: %s", originalExpr)

	// Replace variables in the expression (including $item and $index from loop context)
	replacedExpr, err := t.variableReplacer.ReplaceVariablesWithLoop(ctx, originalExpr, inputs, loopContext)
	if err != nil {
		logger.Warnf("Error replacing variables in forEach runOnlyIf: %v", err)
		return true, ""
	}
	logger.Debugf("Expression after variable replacement: %s", replacedExpr)

	// Evaluate the deterministic expression
	result, err := tool_protocol.EvaluateDeterministicExpression(replacedExpr)
	if err != nil {
		logger.Warnf("Error evaluating forEach runOnlyIf expression '%s': %v", replacedExpr, err)
		return true, ""
	}

	skipReason := fmt.Sprintf("%s -> %s -> %t", originalExpr, replacedExpr, result)
	logger.Debugf("forEach runOnlyIf evaluation: %s", skipReason)

	return result, skipReason
}

// resolveForEachItems converts the raw items value to []interface{}.
// Handles: []interface{}, []map[string]interface{}, []string, string (comma-separated).
func (t *YAMLDefinedTool) resolveForEachItems(rawValue interface{}, separator string) ([]interface{}, error) {
	if rawValue == nil {
		return nil, nil
	}

	if separator == "" {
		separator = tool_protocol.DefaultForEachSeparator
	}

	switch v := rawValue.(type) {
	case []interface{}:
		return v, nil
	case []map[string]interface{}:
		items := make([]interface{}, len(v))
		for i, item := range v {
			items[i] = item
		}
		return items, nil
	case []string:
		items := make([]interface{}, len(v))
		for i, item := range v {
			items[i] = item
		}
		return items, nil
	case string:
		parts := strings.Split(v, separator)
		items := make([]interface{}, len(parts))
		for i, part := range parts {
			items[i] = strings.TrimSpace(part)
		}
		return items, nil
	default:
		return nil, fmt.Errorf("forEach items must be array or string, got %T", v)
	}
}

// processCallbackParamsWithLoop processes callback params, replacing $item and $index variables
// along with other input variables using the loop context. Handles nested maps and arrays recursively.
func (t *YAMLDefinedTool) processCallbackParamsWithLoop(
	ctx context.Context,
	params map[string]interface{},
	inputs map[string]interface{},
	loopContext *tool_engine_models.LoopContext,
) (map[string]interface{}, error) {
	if params == nil {
		return nil, nil
	}

	processed := make(map[string]interface{})
	for key, value := range params {
		processedValue, err := t.processCallbackParamValueWithLoop(ctx, key, value, inputs, loopContext)
		if err != nil {
			return nil, err
		}
		processed[key] = processedValue
	}

	return processed, nil
}

// processCallbackParamValueWithLoop recursively processes a parameter value with loop context.
// Handles strings, nested maps, and arrays.
func (t *YAMLDefinedTool) processCallbackParamValueWithLoop(
	ctx context.Context,
	key string,
	value interface{},
	inputs map[string]interface{},
	loopContext *tool_engine_models.LoopContext,
) (interface{}, error) {
	switch v := value.(type) {
	case string:
		// Replace variables including loop context ($item, $index)
		replaced, err := t.variableReplacer.ReplaceVariablesWithLoop(ctx, v, inputs, loopContext)
		if err != nil {
			return nil, fmt.Errorf("error replacing variables in param '%s': %w", key, err)
		}
		return replaced, nil

	case map[string]interface{}:
		// Process nested maps recursively
		processedMap := make(map[string]interface{})
		for nestedKey, nestedValue := range v {
			processedNestedValue, err := t.processCallbackParamValueWithLoop(ctx, nestedKey, nestedValue, inputs, loopContext)
			if err != nil {
				return nil, err
			}
			processedMap[nestedKey] = processedNestedValue
		}
		return processedMap, nil

	case map[interface{}]interface{}:
		// Handle YAML-style maps with interface{} keys
		processedMap := make(map[string]interface{})
		for nestedKey, nestedValue := range v {
			nestedKeyStr, ok := nestedKey.(string)
			if !ok {
				return nil, fmt.Errorf("nested key '%v' is not a string", nestedKey)
			}
			processedNestedValue, err := t.processCallbackParamValueWithLoop(ctx, nestedKeyStr, nestedValue, inputs, loopContext)
			if err != nil {
				return nil, err
			}
			processedMap[nestedKeyStr] = processedNestedValue
		}
		return processedMap, nil

	case []interface{}:
		// Process arrays recursively
		processedArray := make([]interface{}, len(v))
		for i, item := range v {
			processedItem, err := t.processCallbackParamValueWithLoop(ctx, fmt.Sprintf("%s[%d]", key, i), item, inputs, loopContext)
			if err != nil {
				return nil, err
			}
			processedArray[i] = processedItem
		}
		return processedArray, nil

	default:
		// For other types (int, bool, float, etc.), return as-is
		return value, nil
	}
}

// processCallbackParams processes callback params, replacing variables.
// Handles nested maps and arrays recursively. Same as processCallbackParamsWithLoop but without loop context.
func (t *YAMLDefinedTool) processCallbackParams(
	ctx context.Context,
	params map[string]interface{},
	inputs map[string]interface{},
) (map[string]interface{}, error) {
	if params == nil {
		return nil, nil
	}

	processed := make(map[string]interface{})
	for key, value := range params {
		processedValue, err := t.processCallbackParamValue(ctx, key, value, inputs)
		if err != nil {
			return nil, err
		}
		processed[key] = processedValue
	}
	return processed, nil
}

// processCallbackParamValue recursively processes a parameter value.
// Handles strings, nested maps, and arrays.
func (t *YAMLDefinedTool) processCallbackParamValue(
	ctx context.Context,
	key string,
	value interface{},
	inputs map[string]interface{},
) (interface{}, error) {
	switch v := value.(type) {
	case string:
		replaced, err := t.variableReplacer.ReplaceVariables(ctx, v, inputs)
		if err != nil {
			return nil, fmt.Errorf("error replacing variables in param '%s': %w", key, err)
		}
		return replaced, nil

	case map[string]interface{}:
		// Process nested maps recursively
		return t.processCallbackParams(ctx, v, inputs)

	case map[interface{}]interface{}:
		// Handle YAML-style maps with interface{} keys
		converted := make(map[string]interface{})
		for k, val := range v {
			if keyStr, ok := k.(string); ok {
				converted[keyStr] = val
			}
		}
		return t.processCallbackParams(ctx, converted, inputs)

	case []interface{}:
		// Process arrays recursively
		processedArray := make([]interface{}, len(v))
		for i, item := range v {
			processed, err := t.processCallbackParamValue(ctx, fmt.Sprintf("%s[%d]", key, i), item, inputs)
			if err != nil {
				return nil, err
			}
			processedArray[i] = processed
		}
		return processedArray, nil

	default:
		// For other types (int, bool, float, etc.), return as-is
		return value, nil
	}
}

// executeOnSkipCallbacks executes the onSkip callback functions when a function is skipped due to runOnlyIf.
// This is triggered when a function's runOnlyIf condition evaluates to false/skip.
// onSkip callbacks have access to:
// - Dependency results (from needs block)
// - $skipReason (the explanation of why the function was skipped)
// - Previous sibling onSkip callback outputs (for chaining)
func (t *YAMLDefinedTool) executeOnSkipCallbacks(
	ctx context.Context,
	messageID string,
	clientID string,
	skipReason string,
	callback *ExecutorAgenticWorkflowCallback,
) string {

	output := fmt.Sprintf("TOOL_EXECUTION_SKIPPED: Pre-condition not met - %s", skipReason)

	if len(t.function.OnSkip) == 0 {
		return output
	}

	logger.Infof("Executing %d onSkip callbacks for %s.%s", len(t.function.OnSkip), t.tool.Name, t.function.Name)

	// Build dependency results map from execution tracker - these are the results from 'needs' block
	// that were already executed before runOnlyIf evaluation
	dependencyResults := t.buildDependencyResultsMap(ctx, messageID)
	logger.Debugf("Built dependency results map with %d entries for onSkip callbacks", len(dependencyResults))

	// Track outputs from onSkip siblings to enable chaining like $FuncA.field in FuncB's params
	onSkipSiblingOutputs := make(map[string]interface{})

	for _, skipCall := range t.function.OnSkip {
		skipFuncName := skipCall.Name
		logger.Debugf("Executing onSkip callback: %s", skipFuncName)

		// Evaluate runOnlyIf condition if present
		if skipCall.RunOnlyIf != nil {
			// For onSkip callbacks, we pass dependencyResults as inputs since we don't have fulfilled inputs
			shouldExecute, callbackSkipReason, evalErr := t.evaluateFunctionCallRunOnlyIf(ctx, messageID, skipCall.RunOnlyIf, dependencyResults, "", nil)
			if evalErr != nil {
				logger.Errorf("Error evaluating runOnlyIf for onSkip callback '%s': %v", skipFuncName, evalErr)
				// Continue to next callback - don't fail
				continue
			}

			if !shouldExecute {
				logger.Infof("Skipping onSkip callback '%s' due to runOnlyIf condition: %s", skipFuncName, callbackSkipReason)

				// Record the skip to function_execution table
				skipOutput := fmt.Sprintf("ON_SKIP_SKIPPED: runOnlyIf condition not met - %s", callbackSkipReason)
				if recordErr := t.executionTracker.RecordExecution(
					ctx,
					messageID,
					clientID,
					t.tool.Name,
					skipFuncName,
					"onSkip callback skipped",
					nil,
					"",
					skipOutput,
					nil,
					time.Now(),
					tool_protocol.StatusSkipped,
					nil,
				); recordErr != nil {
					logger.Warnf("Failed to record skipped onSkip execution: %v", recordErr)
				}

				// Audit the skip
				if db, dbErr := t.getDatabase(); dbErr == nil {
					go func() {
						defer func() {
							if r := recover(); r != nil {
								if logger != nil {
									logger.Errorf("Panic in audit logging: %v", r)
								}
							}
						}()
						_, auditErr := db.CreateAudit(ctx, auditRecord{
							MessageID: messageID,
							Action:    "on_skip_skipped",
							Observation: fmt.Sprintf("Skipped onSkip callback '%s' for %s.%s: runOnlyIf condition not met - %s",
								skipFuncName, t.tool.Name, t.function.Name, callbackSkipReason),
						})
						if auditErr != nil {
							logger.WithError(auditErr).Errorf("error creating audit for onSkip skip")
						}
					}()
				}
				continue // Skip this callback
			}

			logger.Infof("runOnlyIf condition met for onSkip callback '%s', proceeding", skipFuncName)
		}

		// Build context with dependency results, $skipReason, and sibling outputs
		inputsWithSkipReason := make(map[string]interface{})

		// Add dependency results (from needs block)
		for k, v := range dependencyResults {
			inputsWithSkipReason[k] = v
		}

		// Add $skipReason special variable
		inputsWithSkipReason["skipReason"] = skipReason

		// Add previous onSkip sibling outputs for $SiblingFunc.field access in params
		if len(onSkipSiblingOutputs) > 0 {
			for k, v := range onSkipSiblingOutputs {
				inputsWithSkipReason[k] = v
			}
			logger.Debugf("[ONSKIP] Merged %d sibling outputs into context: %v", len(onSkipSiblingOutputs), onSkipSiblingOutputs)
		}

		// Inject into context
		parentFunctionKey := GenerateFunctionKey(t.tool.Name, t.function.Name)
		parentContextKey := fmt.Sprintf("pre_fulfilled_inputs_%s_%s_%s", clientID, messageID, parentFunctionKey)
		execCtx := context.WithValue(ctx, parentContextKey, inputsWithSkipReason)
		logger.Debugf("[ONSKIP] Injected context for '%s.%s' with key '%s' (%d values including skipReason)",
			t.tool.Name, t.function.Name, parentContextKey, len(inputsWithSkipReason))

		// Execute the onSkip callback using the tool engine
		allMet, skipOutput, err := t.toolEngine.ExecuteDependencies(
			execCtx,
			messageID,
			clientID,
			t.tool,
			[]tool_protocol.NeedItem{{Name: skipFuncName, Params: skipCall.Params, ShouldBeHandledAsMessageToUser: skipCall.ShouldBeHandledAsMessageToUser, RequiresUserConfirmation: skipCall.RequiresUserConfirmation}},
			t.inputFulfiller,
			callback,
			t.function.Name,
		)

		if err != nil {
			// Check if error is due to pending inputs
			if errors.Is(err, ErrInputFulfillmentPending) {
				logger.Warnf("OnSkip callback '%s' has pending inputs and cannot be executed", skipFuncName)
				continue
			}

			// Log error but don't fail the parent function
			logger.Errorf("Error executing onSkip callback '%s' for %s.%s: %v",
				skipFuncName, t.tool.Name, t.function.Name, err)

			// Audit the failure
			if db, dbErr := t.getDatabase(); dbErr == nil {
				go func() {
					defer func() {
						if r := recover(); r != nil {
							if logger != nil {
								logger.Errorf("Panic in audit logging: %v", r)
							}
						}
					}()
					_, auditErr := db.CreateAudit(ctx, auditRecord{
						MessageID: messageID,
						Action:    "on_skip_execution_failed",
						Observation: fmt.Sprintf("Failed to execute onSkip callback '%s' for %s.%s: %v",
							skipFuncName, t.tool.Name, t.function.Name, err),
					})
					if auditErr != nil {
						logger.WithError(auditErr).Errorf("error creating audit for onSkip failure")
					}
				}()
			}
		} else {
			// Check if onSkip callback returned special messages
			if !allMet && len(skipOutput) > 0 {
				for _, result := range skipOutput {
					if tool_engine_utils.IsSpecialReturn(result) {
						logger.Warnf("OnSkip callback '%s' returned special message, skipping", skipFuncName)
						continue
					}
				}
			}

			logger.Infof("Successfully executed onSkip callback '%s' for %s.%s",
				skipFuncName, t.tool.Name, t.function.Name)

			// Store this callback's output for sibling chaining
			// Try to parse as JSON to enable field access like $FuncA.field
			if len(skipOutput) > 0 && skipOutput[0] != "" {
				if parsed, ok := tryParseJSON(skipOutput[0]); ok {
					onSkipSiblingOutputs[skipFuncName] = parsed
					logger.Debugf("[ONSKIP] Stored sibling output for '%s' (parsed JSON): %v", skipFuncName, parsed)
				} else {
					onSkipSiblingOutputs[skipFuncName] = skipOutput[0]
					logger.Debugf("[ONSKIP] Stored sibling output for '%s' (string): %s", skipFuncName, skipOutput[0])
				}
			}

			// Log the results
			if len(skipOutput) > 0 {
				for i, result := range skipOutput {
					if result != "" {
						logger.Debugf("OnSkip callback '%s' result[%d]: %s", skipFuncName, i, result)
					}
				}
			}

			// Audit the success
			if db, dbErr := t.getDatabase(); dbErr == nil {
				go func() {
					defer func() {
						if r := recover(); r != nil {
							if logger != nil {
								logger.Errorf("Panic in audit logging: %v", r)
							}
						}
					}()
					auditObservation := fmt.Sprintf("Successfully executed onSkip callback '%s' for %s.%s (parent skipped: %s)",
						skipFuncName, t.tool.Name, t.function.Name, skipReason)
					if !allMet {
						auditObservation += " (with special return messages)"
					}
					_, auditErr := db.CreateAudit(ctx, auditRecord{
						MessageID:   messageID,
						Action:      "on_skip_executed",
						Observation: auditObservation,
					})
					if auditErr != nil {
						logger.WithError(auditErr).Errorf("error creating audit for onSkip execution")
					}
				}()
			}
		}
	}

	logger.Debugf("Completed execution of all onSkip callbacks for %s.%s", t.tool.Name, t.function.Name)
	return output
}

// executeMagicStringCallbacks executes the appropriate callback functions when a magic string condition is triggered.
// This is a side-effect operation - callbacks execute but the magic string still propagates.
// Supports onMissingUserInfo, onUserConfirmationRequest, and onTeamApprovalRequest callbacks.
func (t *YAMLDefinedTool) executeMagicStringCallbacks(
	ctx context.Context,
	messageID string,
	clientID string,
	specialReturnType tool_engine_utils.SpecialReturnType,
	inputs map[string]interface{},
	output string,
	callback *ExecutorAgenticWorkflowCallback,
) {

	// Get the appropriate callback list based on special return type
	var callbacks []tool_protocol.FunctionCall
	var callbackType string

	switch specialReturnType {
	case tool_engine_utils.SpecialReturnNeedsInfo:
		callbacks = t.function.OnMissingUserInfo
		callbackType = "onMissingUserInfo"
	case tool_engine_utils.SpecialReturnNeedsConfirmation:
		callbacks = t.function.OnUserConfirmationRequest
		callbackType = "onUserConfirmationRequest"
	case tool_engine_utils.SpecialReturnNeedsApproval:
		callbacks = t.function.OnTeamApprovalRequest
		callbackType = "onTeamApprovalRequest"
	default:
		return // No callback for this type
	}

	if len(callbacks) == 0 {
		return
	}

	logger.Infof("Executing %d %s callbacks for %s.%s", len(callbacks), callbackType, t.tool.Name, t.function.Name)

	for _, callbackCall := range callbacks {
		callbackFuncName := callbackCall.Name
		logger.Debugf("Executing %s callback function: %s", callbackType, callbackFuncName)

		// Evaluate runOnlyIf condition if present
		if callbackCall.RunOnlyIf != nil {
			shouldExecute, skipReason, evalErr := t.evaluateFunctionCallRunOnlyIf(ctx, messageID, callbackCall.RunOnlyIf, inputs, output, nil)
			if evalErr != nil {
				logger.Errorf("Error evaluating runOnlyIf for %s function '%s': %v", callbackType, callbackFuncName, evalErr)
				continue
			}

			if !shouldExecute {
				logger.Infof("Skipping %s function '%s' due to runOnlyIf condition: %s", callbackType, callbackFuncName, skipReason)

				// Record the skip to function_execution table
				skipOutput := fmt.Sprintf("%s_SKIPPED: runOnlyIf condition not met - %s", strings.ToUpper(callbackType), skipReason)
				if recordErr := t.executionTracker.RecordExecution(
					ctx,
					messageID,
					clientID,
					t.tool.Name,
					callbackFuncName,
					fmt.Sprintf("%s callback skipped", callbackType),
					nil,
					"",
					skipOutput,
					nil,
					time.Now(),
					tool_protocol.StatusSkipped,
					nil,
				); recordErr != nil {
					logger.Warnf("Failed to record skipped %s execution: %v", callbackType, recordErr)
				}

				// Audit the skip
				if db, dbErr := t.getDatabase(); dbErr == nil {
					go func(funcName, skipReasonCopy string) {
						defer func() {
							if r := recover(); r != nil {
								if logger != nil {
									logger.Errorf("Panic in audit logging: %v", r)
								}
							}
						}()
						_, auditErr := db.CreateAudit(ctx, auditRecord{
							MessageID: messageID,
							Action:    fmt.Sprintf("%s_skipped", callbackType),
							Observation: fmt.Sprintf("Skipped %s function '%s' for %s.%s: runOnlyIf condition not met - %s",
								callbackType, funcName, t.tool.Name, t.function.Name, skipReasonCopy),
						})
						if auditErr != nil {
							logger.WithError(auditErr).Errorf("error creating audit for %s skip", callbackType)
						}
					}(callbackFuncName, skipReason)
				}
				continue
			}

			logger.Infof("runOnlyIf condition met for %s function '%s', proceeding", callbackType, callbackFuncName)
		}

		// Prepare context with parent function's inputs (including output as "result")
		execCtx := ctx
		inputsWithResult := make(map[string]interface{})
		for k, v := range inputs {
			inputsWithResult[k] = v
		}
		// Add parent function's output as "result" for $result.field access in params
		if output != "" {
			if parsed, ok := tryParseJSON(output); ok {
				inputsWithResult["result"] = parsed
			} else {
				inputsWithResult["result"] = output
			}
		}
		parentFunctionKey := GenerateFunctionKey(t.tool.Name, t.function.Name)
		parentContextKey := fmt.Sprintf("pre_fulfilled_inputs_%s_%s_%s", clientID, messageID, parentFunctionKey)
		execCtx = context.WithValue(execCtx, parentContextKey, inputsWithResult)
		logger.Debugf("[%s] Injected parent inputs for '%s.%s' into context with key '%s' (%d inputs including result)",
			callbackType, t.tool.Name, t.function.Name, parentContextKey, len(inputsWithResult))

		// Execute the callback function using the tool engine
		logger.Debugf("[%s-DEBUG] Calling ExecuteDependencies for '%s' with params: %v", callbackType, callbackFuncName, callbackCall.Params)
		allMet, callbackOutput, err := t.toolEngine.ExecuteDependencies(
			execCtx,
			messageID,
			clientID,
			t.tool,
			[]tool_protocol.NeedItem{{Name: callbackFuncName, Params: callbackCall.Params, ShouldBeHandledAsMessageToUser: callbackCall.ShouldBeHandledAsMessageToUser, RequiresUserConfirmation: callbackCall.RequiresUserConfirmation}},
			t.inputFulfiller,
			callback,
			t.function.Name,
		)

		if err != nil {
			// Log error but don't fail the parent function - these are side-effect callbacks
			logger.Errorf("Error executing %s function '%s' for %s.%s: %v",
				callbackType, callbackFuncName, t.tool.Name, t.function.Name, err)

			// Audit the failure
			if db, dbErr := t.getDatabase(); dbErr == nil {
				go func(funcName string, execErr error) {
					defer func() {
						if r := recover(); r != nil {
							if logger != nil {
								logger.Errorf("Panic in audit logging: %v", r)
							}
						}
					}()
					_, auditErr := db.CreateAudit(ctx, auditRecord{
						MessageID: messageID,
						Action:    fmt.Sprintf("%s_execution_failed", callbackType),
						Observation: fmt.Sprintf("Failed to execute %s function '%s' for %s.%s: %v",
							callbackType, funcName, t.tool.Name, t.function.Name, execErr),
					})
					if auditErr != nil {
						logger.WithError(auditErr).Errorf("error creating audit for %s failure", callbackType)
					}
				}(callbackFuncName, err)
			}
		} else {
			logger.Infof("Successfully executed %s function '%s' for %s.%s",
				callbackType, callbackFuncName, t.tool.Name, t.function.Name)

			// Log the results
			if len(callbackOutput) > 0 {
				for i, result := range callbackOutput {
					if result != "" {
						logger.Debugf("%s function '%s' result[%d]: %s", callbackType, callbackFuncName, i, result)
					}
				}
			}

			// Audit the success
			if db, dbErr := t.getDatabase(); dbErr == nil {
				go func(funcName string) {
					defer func() {
						if r := recover(); r != nil {
							if logger != nil {
								logger.Errorf("Panic in audit logging: %v", r)
							}
						}
					}()
					auditObservation := fmt.Sprintf("Successfully executed %s function '%s' for %s.%s",
						callbackType, funcName, t.tool.Name, t.function.Name)
					if !allMet {
						auditObservation += " (with special return messages)"
					}
					_, auditErr := db.CreateAudit(ctx, auditRecord{
						MessageID:   messageID,
						Action:      fmt.Sprintf("%s_executed", callbackType),
						Observation: auditObservation,
					})
					if auditErr != nil {
						logger.WithError(auditErr).Errorf("error creating audit for %s execution", callbackType)
					}
				}(callbackFuncName)
			}
		}
	}

	logger.Debugf("Completed execution of all %s callbacks for %s.%s", callbackType, t.tool.Name, t.function.Name)
}

func (t *YAMLDefinedTool) attemptToFixInputs(ctx context.Context, inputs map[string]interface{}, errorMsg string) (map[string]interface{}, bool, error) {
	// Create a deep copy of inputs to avoid modifying the original
	fixedInputs := make(map[string]interface{})
	for k, v := range inputs {
		fixedInputs[k] = v
	}

	var inputsDescription string
	for _, in := range t.function.Input {
		inputsDescription += fmt.Sprintf("%s: %s -> %s \n\n", in.Name, in.Description, in.SuccessCriteria)
	}

	inputsJSON, err := json.Marshal(inputs)
	if err != nil {
		return nil, false, fmt.Errorf("error marshaling inputs: %w", err)
	}

	messages := thread.New()
	systemPrompt := fmt.Sprintf(`You are an input format fixer for tool operations. You will analyze a execution log and the input values that caused it.
If you identify that the execution log is talking about an issue during execution related to input format issues (like date formats, number formats, etc.), 
suggest the corrected value for each problematic input field.
Below you will find the input(s) specification: 
%s

Return your response in this JSON format:
{
  "analysis": "Brief explanation of what's wrong with the input format",
  "shouldRetry": true/false,
  "fixedInputs": {
    "input_key_1": "fixed_value_1",
    "input_key_2": "fixed_value_2"
  }
}

Set "shouldRetry" to true only if you believe that the execution logs represents an error and not a successful execution and in case of error, that it is due to an input format issue that can be fixed. Set "shouldRetry" to false if the execution log is not related to execution issues or not related to the input format. Set "shouldRetry" to false if the execution was successful
Attention: Include only the keys that need fixing in the "fixedInputs" object. Output only the json filled, nothing else`, inputsDescription)

	userMsg := fmt.Sprintf("execution log: %s\n\nCurrent input values: %s", errorMsg, string(inputsJSON))

	messages.AddMessages(
		thread.NewSystemMessage().AddContent(thread.NewTextContent(systemPrompt)),
		thread.NewUserMessage().AddContent(thread.NewTextContent(userMsg)),
	)

	llm := newLLM()

	const maxLLMRetries = 3
	var result struct {
		Analysis    string                 `json:"analysis"`
		ShouldRetry bool                   `json:"shouldRetry"`
		FixedInputs map[string]interface{} `json:"fixedInputs"`
	}

	for retry := 0; retry < maxLLMRetries; retry++ {
		err = generateLLM(ctx, llm, messages)
		if err != nil {
			logger.Warnf("Error generating LLM response (attempt %d of %d): %v",
				retry+1, maxLLMRetries, err)
			continue
		}

		if len(messages.LastMessage().Contents) == 0 {
			logger.Warnf("No message generated by LLM (attempt %d of %d)",
				retry+1, maxLLMRetries)
			continue
		}

		llmResponse := messages.LastMessage().Contents[0].AsString()

		jsonStart := strings.Index(llmResponse, "{")
		jsonEnd := strings.LastIndex(llmResponse, "}")
		if jsonStart == -1 || jsonEnd == -1 || jsonEnd <= jsonStart {
			// Add a new message to the thread asking for properly formatted JSON
			messages.AddMessages(
				thread.NewUserMessage().AddContent(thread.NewTextContent(
					"Please respond with properly formatted JSON only, following the exact format I specified.")),
			)
			continue
		}

		jsonStr := llmResponse[jsonStart : jsonEnd+1]
		if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
			logger.Warnf("Error unmarshaling LLM response (attempt %d of %d): %v",
				retry+1, maxLLMRetries, err)
			// Add a new message to the thread with the error
			messages.AddMessages(
				thread.NewUserMessage().AddContent(thread.NewTextContent(
					fmt.Sprintf("There was an error parsing your JSON response: %v. Please provide a valid JSON in the exact format specified.", err))),
			)
			continue
		}

		break
	}

	// If we didn't get a valid JSON response after all retries
	if result.Analysis == "" && result.FixedInputs == nil {
		return nil, false, fmt.Errorf("failed to get a valid JSON response from LLM after %d attempts", maxLLMRetries)
	}

	if result.ShouldRetry && len(result.FixedInputs) > 0 {
		// Update only the keys that need fixing
		for k, v := range result.FixedInputs {
			if _, exists := fixedInputs[k]; exists {
				fixedInputs[k] = v
				logger.Infof("Fixed input format for '%s': '%v' -> '%v'", k, inputs[k], v)
			}
		}

		return fixedInputs, true, nil
	}

	// LLM determined we shouldn't retry or didn't provide any fixes
	return nil, false, nil
}

func (t *YAMLDefinedTool) CheckOutput(ctx context.Context, outputDef *tool_protocol.Output, formattedOutput, rawOutput string) (string, error) {
	if outputDef.Type == "string" || outputDef.Type == "number" {
		return formattedOutput, nil
	}

	// Skip LLM validation if we have explicit field definitions (deterministic extraction)
	// The deterministic formatter already extracted exactly what we need
	if len(outputDef.Fields) > 0 {
		return formattedOutput, nil
	}

	// Only use LLM fallback if explicitly enabled via allowInference: true
	if !outputDef.AllowInference {
		logger.Debugf("LLM output formatting disabled (allowInference: false)")
		return formattedOutput, nil
	}

	logger.WithFields(map[string]interface{}{"formattedOutput": formattedOutput}).Debugf("formatting with LLM (allowInference: true)")
	return t.formatOutputWithAI(ctx, formattedOutput, rawOutput)
}

func (t *YAMLDefinedTool) formatOutputWithAI(ctx context.Context, formattedOutput, rawOutput string) (string, error) {
	messages := thread.New()

	systemPrompt := fmt.Sprintf(`You are a output formatter. you will receive strings for references of the values that must be filled in the desired output format. output only the desired output with the filled values only based on the strings that you receive. if the received values are not enough to fill the desired output, just output the string NONE. attention, do not create any information, just use the received values as reference.
____
desired output format: 
%s `,
		formattedOutput)

	messages.AddMessages(
		thread.NewSystemMessage().AddContent(thread.NewTextContent(systemPrompt)),
		thread.NewUserMessage().AddContent(thread.NewTextContent(rawOutput)),
	)

	llm := newLLM() // or llama 8b

	var userMessage string
	for i := 0; i < 3; i++ {
		err := generateLLM(ctx, llm, messages)
		if err != nil {
			return "", fmt.Errorf("error generating user message: %w", err)
		}

		if len(messages.LastMessage().Contents) == 0 {
			return "", errors.New("no message generated by LLM")
		}

		userMessage = messages.LastMessage().Contents[0].AsString()
		if userMessage != "" && !(strings.Contains(strings.ToLower(userMessage), "none")) {
			return userMessage, nil
		}
	}

	return formattedOutput, nil
}

// executeWebBrowseOperation handles web browsing operations
func (t *YAMLDefinedTool) executeWebBrowseOperation(ctx context.Context, messageID string, inputs map[string]interface{}) (string, map[int]interface{}, error) {
	stepResults := make(map[int]interface{})

	// Execute zero-state steps if defined
	if len(t.function.ZeroState.Steps) > 0 {
		err := t.executeSteps(ctx, messageID, t.function.ZeroState.Steps, inputs, stepResults)
		if err != nil {
			output := ""
			if len(stepResults) > 0 {
				for _, result := range stepResults {
					if result != nil {
						if strResult, ok := result.(string); ok {
							output = strResult
							break
						}
					}
				}
			}

			return output, nil, fmt.Errorf("error executing zero-state steps: %w", err)
		}
	}

	err := t.executeSteps(ctx, messageID, t.function.Steps, inputs, stepResults)
	if err != nil {
		output := ""
		if len(stepResults) > 0 {
			for _, result := range stepResults {
				if result != nil {
					if strResult, ok := result.(string); ok {
						output = strResult
						break
					}
				}
			}
		}

		return output, nil, fmt.Errorf("error executing steps: %w", err)
	}

	if len(stepResults) > 0 {
		for _, result := range stepResults {
			if result != nil {
				if strResult, ok := result.(string); ok {
					return strResult, stepResults, nil
				}
			}
		}
	}

	output := fmt.Sprintf("Web browsing operation for function '%s' executed successfully", t.function.Name)

	return output, stepResults, nil
}

// executeApiCallOperation handles API call operations
// Returns output, step results, execution details for debugging, and error
// existingStepResults allows preserving results from previous retry attempts, enabling skip of completed steps
func (t *YAMLDefinedTool) executeApiCallOperation(ctx context.Context, messageID string, inputs map[string]interface{}, existingStepResults map[int]interface{}) (string, map[int]interface{}, *APIExecutionDetails, error) {

	logger.Debugf("executeApiCallOperation started - messageID: %s, tool: %s, function: %s, inputs: %#v",
		messageID, t.tool.Name, t.function.Name, inputs)

	// Use existing step results if provided (for retry scenarios)
	// This allows skipping steps that already completed successfully
	stepResults := existingStepResults
	if stepResults == nil {
		stepResults = make(map[int]interface{})
	}
	var output string

	// Collect execution details for all steps
	executionDetails := &APIExecutionDetails{
		Steps: make([]StepExecutionDetail, 0),
	}

	// Check if we have any authentication steps
	logger.Debugf("Checking for authentication steps in %d total steps", len(t.function.Steps))
	hasAuthStep := false
	var authStepIndex int
	for i, step := range t.function.Steps {
		if step.IsAuthentication {
			hasAuthStep = true
			authStepIndex = i
			logger.Debugf("Found authentication step at index %d: %s", i, step.Name)
			break
		}
	}
	logger.Debugf("Authentication step check complete - hasAuthStep: %t, authStepIndex: %d", hasAuthStep, authStepIndex)

	// Retry counter to prevent infinite loops
	maxAuthRetries := 2
	authRetryCount := 0
	logger.Debugf("Initialized retry counters - maxAuthRetries: %d, authRetryCount: %d", maxAuthRetries, authRetryCount)

	cookieRepo, err := newCookieRepository()
	if err != nil {
		logger.WithError(err).Warnf("Failed to create cookie repository")
	}

	// Initialize auth token repository for $AUTH_TOKEN support
	authTokenRepo, err := newAuthTokenRepository()
	if err != nil {
		logger.WithError(err).Warnf("Failed to create auth token repository")
	}

	// Create a shared HTTP client for all steps
	var sharedHTTPClient HTTPClient

	// NOTE: SetAuthProvider / SetCookies are not part of the HTTPClient interface.
	// The host application should configure auth at the client level before passing it.
	if authProvider := t.toolEngine.GetAuthProvider(); authProvider != nil {
		logger.Debugf("Auth provider available for shared HTTP client (set by host)")
	}

	logger.Debugf("Created shared HTTP client")

	// Try to load cookies from DB if we have authentication steps
	cookiesLoaded := false
	if hasAuthStep && cookieRepo != nil {
		logger.Debugf("Attempting to load cookies for %s.%s", t.tool.Name, t.function.Name)
		cookies, err := cookieRepo.GetCookies(t.tool.Name, t.function.Name)
		if err == nil && len(cookies) > 0 {
			// NOTE: SetCookies not part of HTTPClient interface; cookies managed by host.
			_ = cookies
			cookiesLoaded = true
			logger.Infof("Loaded %d cookies from DB for %s.%s", len(cookies), t.tool.Name, t.function.Name)
		} else {
			logger.WithError(err).Errorf("failed to load cookies")
		}
	}

	// Try to load cached auth token if we have authentication steps with extractAuthToken
	tokenLoaded := false
	hasExtractAuthToken := false
	if hasAuthStep && authTokenRepo != nil {
		// Check if auth step has extractAuthToken configuration
		authStep := t.function.Steps[authStepIndex]
		if authStep.With != nil {
			if _, hasConfig := authStep.With[tool_protocol.StepWithExtractAuthToken]; hasConfig {
				hasExtractAuthToken = true
				logger.Debugf("Auth step has extractAuthToken config, attempting to load cached token for %s.%s", t.tool.Name, t.function.Name)
				token, err := authTokenRepo.GetToken(t.tool.Name, t.function.Name)
				if err == nil && token != "" {
					// Set token on variable replacer
					t.toolEngine.GetVariableReplacer().SetAuthToken(token)
					tokenLoaded = true
					logger.Infof("Loaded cached auth token for %s.%s", t.tool.Name, t.function.Name)
				} else {
					logger.Debugf("No valid cached token found for %s.%s: %v", t.tool.Name, t.function.Name, err)
				}
			}
		}
	}

	logger.Debugf("Starting execution of %d steps", len(t.function.Steps))
	for stepIndex, step := range t.function.Steps {
		logger.Debugf("Processing step %d/%d: %s (IsAuth: %t, ForEach: %t)",
			stepIndex+1, len(t.function.Steps), step.Name, step.IsAuthentication, step.ForEach != nil)

		// Skip step if it already has a result from a previous retry attempt
		// This prevents re-executing steps that already completed successfully
		if step.ResultIndex != 0 {
			if _, hasResult := stepResults[step.ResultIndex]; hasResult {
				logger.Infof("Skipping step '%s' - already completed in previous attempt (result at index %d)",
					step.Name, step.ResultIndex)
				executionDetails.Steps = append(executionDetails.Steps, StepExecutionDetail{
					StepName: fmt.Sprintf("%s (skipped - already completed)", step.Name),
					Request:  nil,
					Response: "Step skipped because result already exists from previous attempt",
				})
				continue
			}
		}

		if step.IsAuthentication && (cookiesLoaded || tokenLoaded) {
			skipReason := "cookies valid"
			if tokenLoaded && !cookiesLoaded {
				skipReason = "cached token valid"
			} else if tokenLoaded && cookiesLoaded {
				skipReason = "cookies and cached token valid"
			}
			logger.Infof("Skipping authentication step '%s' as %s", step.Name, skipReason)
			// Add placeholder entry for skipped auth step
			executionDetails.Steps = append(executionDetails.Steps, StepExecutionDetail{
				StepName: fmt.Sprintf("%s (skipped - %s)", step.Name, skipReason),
				Request:  nil,
				Response: fmt.Sprintf("Authentication step skipped because %s", skipReason),
			})
			continue
		}

		if step.ForEach != nil {
			logger.Debugf("Executing foreach step: %s", step.Name)
			err := t.executeForEachApiStep(ctx, messageID, step, inputs, stepResults, sharedHTTPClient, cookieRepo, hasAuthStep, &authRetryCount, maxAuthRetries, authStepIndex, &cookiesLoaded, executionDetails)
			if err != nil {
				return "", stepResults, executionDetails, fmt.Errorf("error executing foreach step '%s': %w", step.Name, err)
			}

			logger.Debugf("Foreach step completed successfully: %s", step.Name)
			if step.ResultIndex != 0 {
				if results, ok := stepResults[step.ResultIndex].([]interface{}); ok && len(results) > 0 {
					logger.Debugf("Processing foreach results for step %s - found %d results", step.Name, len(results))
					// For foreach steps, try to create a meaningful output from all results
					var outputParts []string
					for _, result := range results {
						if strResult, ok := result.(string); ok && strResult != "" {
							outputParts = append(outputParts, strResult)
						}
					}
					if len(outputParts) > 0 {
						output = strings.Join(outputParts, "\n")
						logger.Debugf("Created combined output from %d parts for step %s", len(outputParts), step.Name)
					}
				}
			}
		} else {
			logger.Debugf("Processing parameters for regular step: %s", step.Name)
			processedParams, err := t.processStepParameters(ctx, messageID, step.With, inputs, stepResults)
			if err != nil {
				return "", stepResults, executionDetails, fmt.Errorf("error processing step parameters for step '%s': %w", step.Name, err)
			}
			step.With = processedParams
			logger.Debugf("Parameters processed successfully for step %s: %#v", step.Name, processedParams)

			logger.Debugf("Executing API step: %s", step.Name)
			result, requestDetails, err := t.executeAPIStep(ctx, messageID, step, sharedHTTPClient)
			resultForLogging, _ := result.(string)

			// Collect step execution details for debugging
			stepDetail := StepExecutionDetail{
				StepName: step.Name,
				Request:  requestDetails,
				Response: resultForLogging,
			}
			if err != nil {
				stepDetail.Error = err.Error()
			}
			executionDetails.Steps = append(executionDetails.Steps, stepDetail)

			logger.Debugf("API step execution completed for %s - hasError: %t, result: %#v", step.Name, err != nil, tool_engine_utils.TruncateForLogging(resultForLogging))

			if !step.IsAuthentication && hasAuthStep && t.isAuthenticationError(err, result) {
				logger.Warnf("Authentication error detected for step %s (attempt %d/%d)", step.Name, authRetryCount+1, maxAuthRetries)
				if authRetryCount >= maxAuthRetries {
					logger.Warnf("Maximum authentication retries (%d) reached, failing with authentication error", maxAuthRetries)
					return "", stepResults, executionDetails, fmt.Errorf("authentication failed after %d retries - unable to authenticate with provided credentials", maxAuthRetries)
				}

				authRetryCount++
				logger.Infof("Authentication error detected (attempt %d/%d), retrying from authentication step", authRetryCount, maxAuthRetries)

				cookiesLoaded = false
				if cookieRepo != nil {
					logger.Debugf("Deleting existing cookies for %s.%s", t.tool.Name, t.function.Name)
					cookieRepo.DeleteCookies(t.tool.Name, t.function.Name)
				}

				// Clear cached auth token on authentication error
				tokenLoaded = false
				if authTokenRepo != nil && hasExtractAuthToken {
					logger.Debugf("Deleting cached auth token for %s.%s", t.tool.Name, t.function.Name)
					authTokenRepo.DeleteToken(t.tool.Name, t.function.Name)
					t.toolEngine.GetVariableReplacer().ClearAuthToken()
				}

				logger.Debugf("Starting retry sequence from step %d to %d", authStepIndex, len(t.function.Steps)-1)
				for retryIndex := authStepIndex; retryIndex < len(t.function.Steps); retryIndex++ {
					retryStep := t.function.Steps[retryIndex]
					logger.Debugf("Executing retry step %d: %s", retryIndex, retryStep.Name)

					processedParams, err := t.processStepParameters(ctx, messageID, retryStep.With, inputs, stepResults)
					if err != nil {
						return "", stepResults, executionDetails, fmt.Errorf("error processing retry step parameters: %w", err)
					}
					retryStep.With = processedParams

					retryResult, retryRequestDetails, retryErr := t.executeAPIStep(ctx, messageID, retryStep, sharedHTTPClient)
					retryResultForLogging, _ := retryResult.(string)

					// Collect retry step execution details for debugging
					retryStepDetail := StepExecutionDetail{
						StepName: retryStep.Name + " (retry)",
						Request:  retryRequestDetails,
						Response: retryResultForLogging,
					}
					if retryErr != nil {
						retryStepDetail.Error = retryErr.Error()
					}
					executionDetails.Steps = append(executionDetails.Steps, retryStepDetail)

					if retryErr != nil {
						// Check if this is another authentication error during retry
						if !retryStep.IsAuthentication && t.isAuthenticationError(retryErr, retryResult) {
							return "", stepResults, executionDetails, fmt.Errorf("authentication failed during retry step '%s' after %d attempts", retryStep.Name, authRetryCount)
						}
						logger.Errorf("Error executing retry step '%s': %v", retryStep.Name, retryErr)
						return "", stepResults, executionDetails, fmt.Errorf("error executing retry step '%s': %w", retryStep.Name, retryErr)
					}
					logger.Debugf("Retry step %s executed successfully", retryStep.Name)

					if retryStep.IsAuthentication && cookieRepo != nil {
						logger.Debugf("Saving cookies for authentication retry step: %s", retryStep.Name)
						t.saveCookiesFromClient(ctx, sharedHTTPClient, cookieRepo)
					}

					// Extract and cache auth token for retry auth step
					if retryStep.IsAuthentication && hasExtractAuthToken && authTokenRepo != nil {
						logger.Debugf("Extracting auth token for authentication retry step: %s", retryStep.Name)
						token, ttl, extractErr := t.extractAuthTokenFromResult(ctx, retryStep, retryResult)
						if extractErr != nil {
							logger.WithError(extractErr).Warnf("Failed to extract auth token from retry step %s", retryStep.Name)
						} else if token != "" {
							t.toolEngine.GetVariableReplacer().SetAuthToken(token)
							if cacheErr := authTokenRepo.SaveToken(t.tool.Name, t.function.Name, token, time.Duration(ttl)*time.Second); cacheErr != nil {
								logger.WithError(cacheErr).Warnf("Failed to cache auth token for %s.%s", t.tool.Name, t.function.Name)
							} else {
								logger.Infof("Cached auth token for %s.%s with TTL %d seconds (retry)", t.tool.Name, t.function.Name, ttl)
							}
						}
					}

					// Handle saveAsFile for retry steps
					if retryStep.SaveAsFile != nil && retryErr == nil && retryRequestDetails != nil {
						fileResult, fileErr := t.processStepAsFileDownload(ctx, messageID, retryStep, retryRequestDetails, inputs)
						if fileErr != nil {
							logger.Errorf("saveAsFile failed for retry step '%s': %v", retryStep.Name, fileErr)
							return "", stepResults, executionDetails, fmt.Errorf("saveAsFile failed for retry step '%s': %w", retryStep.Name, fileErr)
						}
						if retryStep.ResultIndex != 0 {
							stepResults[retryStep.ResultIndex] = fileResult
							logger.Debugf("Stored FileResult for retry step %s at index %d", retryStep.Name, retryStep.ResultIndex)
						}
						fileResultJSON, jsonErr := json.Marshal(fileResult)
						if jsonErr == nil {
							output = string(fileResultJSON)
						}
					} else {
						if retryStep.ResultIndex != 0 {
							stepResults[retryStep.ResultIndex] = retryResult
							logger.Debugf("Stored retry result for step %s at index %d", retryStep.Name, retryStep.ResultIndex)
						}

						// FIX: Update output from retry result (same logic as normal flow)
						if strResult, ok := retryResult.(string); ok && strResult != "" {
							output = strResult
							logger.Debugf("Updated output from retry step %s: %s", retryStep.Name, tool_engine_utils.TruncateForLogging(strResult))
						}
					}
				}

				logger.Debugf("Retry sequence completed, breaking from main loop")
				break
			}

			if err != nil {
				logger.Errorf("Error executing step '%s': %v", step.Name, err)
				return "", stepResults, executionDetails, fmt.Errorf("error executing step '%s': %w", step.Name, err)
			}

			if step.IsAuthentication && cookieRepo != nil {
				logger.Debugf("Saving cookies for authentication step: %s", step.Name)
				t.saveCookiesFromClient(ctx, sharedHTTPClient, cookieRepo)
			}

			// Extract and cache auth token if extractAuthToken is configured
			if step.IsAuthentication && hasExtractAuthToken && authTokenRepo != nil {
				logger.Debugf("Extracting auth token for authentication step: %s", step.Name)
				token, ttl, extractErr := t.extractAuthTokenFromResult(ctx, step, result)
				if extractErr != nil {
					logger.WithError(extractErr).Warnf("Failed to extract auth token from step %s", step.Name)
				} else if token != "" {
					// Set token on variable replacer for immediate use
					t.toolEngine.GetVariableReplacer().SetAuthToken(token)
					// Cache token for future requests
					if cacheErr := authTokenRepo.SaveToken(t.tool.Name, t.function.Name, token, time.Duration(ttl)*time.Second); cacheErr != nil {
						logger.WithError(cacheErr).Warnf("Failed to cache auth token for %s.%s", t.tool.Name, t.function.Name)
					} else {
						logger.Infof("Cached auth token for %s.%s with TTL %d seconds", t.tool.Name, t.function.Name, ttl)
					}
				}
			}

			// Handle saveAsFile: convert raw HTTP response to FileResult
			if step.SaveAsFile != nil && err == nil && requestDetails != nil {
				fileResult, fileErr := t.processStepAsFileDownload(ctx, messageID, step, requestDetails, inputs)
				if fileErr != nil {
					logger.Errorf("saveAsFile failed for step '%s': %v", step.Name, fileErr)
					return "", stepResults, executionDetails, fmt.Errorf("saveAsFile failed for step '%s': %w", step.Name, fileErr)
				}
				// Override the step result with the FileResult
				if step.ResultIndex != 0 {
					stepResults[step.ResultIndex] = fileResult
					logger.Debugf("Stored FileResult for step %s at index %d", step.Name, step.ResultIndex)
				}
				// Set output to FileResult JSON
				fileResultJSON, jsonErr := json.Marshal(fileResult)
				if jsonErr == nil {
					output = string(fileResultJSON)
				}
			} else {
				if step.ResultIndex != 0 {
					stepResults[step.ResultIndex] = result
					logger.Debugf("Stored result for step %s at index %d", step.Name, step.ResultIndex)
				}

				if strResult, ok := result.(string); ok && strResult != "" {
					if stepIndex == len(t.function.Steps)-1 || strResult != "" {
						output = strResult
						logger.Debugf("Updated output from step %s: %s", step.Name, tool_engine_utils.TruncateForLogging(strResult))
					}
				}
			}
		}
	}

	logger.Debugf("Main step execution loop completed")

	// If output is still empty, try to get it from step results
	if output == "" && len(stepResults) > 0 {
		logger.Debugf("Output is empty, searching %d step results for fallback output", len(stepResults))
		// Find the last result to use as output
		maxIndex := 0
		for index := range stepResults {
			if index > maxIndex {
				maxIndex = index
			}
		}
		logger.Debugf("Found maximum result index: %d", maxIndex)

		if result, ok := stepResults[maxIndex]; ok {
			if strResult, ok := result.(string); ok {
				output = strResult
				logger.Debugf("Using result from index %d as fallback output: %s", maxIndex, strResult)
			}
		}
	}

	logger.Debugf("executeApiCallOperation completed successfully - output length: %d, stepResults count: %d, executionDetails steps: %d", len(output), len(stepResults), len(executionDetails.Steps))
	return output, stepResults, executionDetails, nil
}

// executeDesktopOperation handles desktop operations
func (t *YAMLDefinedTool) executeDesktopOperation(ctx context.Context, messageID string, inputs map[string]interface{}) (string, map[int]interface{}, error) {
	// Initialize step results map
	stepResults := make(map[int]interface{})

	// Execute zero-state steps if defined
	if len(t.function.ZeroState.Steps) > 0 {
		err := t.executeSteps(ctx, messageID, t.function.ZeroState.Steps, inputs, stepResults)
		if err != nil {
			return "", nil, fmt.Errorf("error executing zero-state steps: %w", err)
		}
	}

	// Execute main steps
	err := t.executeSteps(ctx, messageID, t.function.Steps, inputs, stepResults)
	if err != nil {
		return "", nil, fmt.Errorf("error executing steps: %w", err)
	}

	// For now, return a mock success response
	output := fmt.Sprintf("Desktop operation for function '%s' executed successfully", t.function.Name)

	return output, stepResults, nil
}

// executeMCPOperation handles MCP operations
func (t *YAMLDefinedTool) executeMCPOperation(ctx context.Context, messageID string, inputs map[string]interface{}) (string, map[int]interface{}, error) {
	if t.function.MCP == nil {
		return "", nil, errors.New("MCP configuration is missing")
	}

	if t.mcpToolExecutor == nil {
		return "", nil, errors.New("MCP executor not configured")
	}

	// Pre-process environment variables for stdio transport
	if t.function.MCP.Protocol == tool_protocol.MCPProtocolStdio && t.function.MCP.Stdio != nil {
		processedEnv := make(map[string]string, len(t.function.MCP.Stdio.Env))
		for k, v := range t.function.MCP.Stdio.Env {
			processedValue, err := t.processParameterValue(ctx, messageID, k, v, inputs, nil)
			if err != nil {
				return "", nil, fmt.Errorf("error processing environment variable '%s': %w", k, err)
			}
			processedEnv[k] = fmt.Sprintf("%v", processedValue)
		}
		// Merge with system environment variables
		for k, v := range t.variableReplacer.GetEnvironmentVariables() {
			processedEnv[k] = v
		}
		t.function.MCP.Stdio.Env = processedEnv
	}

	result, err := t.mcpToolExecutor.ExecuteMCPOperation(ctx, t.function.MCP, inputs)
	if err != nil {
		return "", nil, err
	}

	resultStr := fmt.Sprintf("%v", result)
	return resultStr, nil, nil
}

// dbGetter is a function variable that can be overridden for testing (used for system tools)
var dbGetter = getDB

// toolDBGetter is a function variable that can be overridden for testing (used for user tools)
// Returns the database for a specific tool name
var toolDBGetter func(toolName string) (*sql.DB, error) = defaultToolDBGetter

func defaultToolDBGetter(toolName string) (*sql.DB, error) {
	mgr, err := getToolDBManager()
	if err != nil {
		return nil, err
	}
	return mgr.GetToolDB(toolName)
}

// getToolDatabase returns the appropriate database for this tool.
// System tools use the shared connectai.db, user tools get their own database.
func (t *YAMLDefinedTool) getToolDatabase() (*sql.DB, error) {
	// System tools use the shared database
	if t.tool.IsSystemApp {
		return dbGetter()
	}
	// User tools get their own database
	return toolDBGetter(t.tool.Name)
}

// migrationCaches holds cached migration data to avoid repeated parsing and table creation.
// Key is tool name (for user tools) or "__system__" (for system tools).
var (
	// toolMigrationsCache caches collected migrations per tool
	toolMigrationsCache sync.Map // map[string][]tool_protocol.Migration

	// migrationTableEnsured tracks if migration table was already created for each DB
	migrationTableEnsured sync.Map // map[string]bool
)

// getMigrationCacheKey returns the cache key for migrations based on tool type
func (t *YAMLDefinedTool) getMigrationCacheKey() string {
	if t.tool.IsSystemApp {
		return "__system__"
	}
	return t.tool.Name
}

// ensureToolMigrationTable creates the tool_schema_migrations table if it doesn't exist.
// This table tracks which migrations have been applied for each tool.
// Uses caching to avoid repeated CREATE TABLE calls.
func (t *YAMLDefinedTool) ensureToolMigrationTable(ctx context.Context, db *sql.DB) error {
	cacheKey := t.getMigrationCacheKey()

	// Check if already ensured for this DB
	if _, ok := migrationTableEnsured.Load(cacheKey); ok {
		return nil
	}

	createSQL := `
		CREATE TABLE IF NOT EXISTS tool_schema_migrations (
			tool_name TEXT NOT NULL,
			version INTEGER NOT NULL,
			applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (tool_name, version)
		)
	`
	if _, err := db.ExecContext(ctx, createSQL); err != nil {
		return err
	}

	migrationTableEnsured.Store(cacheKey, true)
	return nil
}

// collectToolMigrations collects unique migrations from all functions in the tool.
// Since validation ensures same version has same SQL, we can just take the first occurrence.
// Results are cached per tool to avoid repeated parsing.
func (t *YAMLDefinedTool) collectToolMigrations() []tool_protocol.Migration {
	// Check cache first
	if cached, ok := toolMigrationsCache.Load(t.tool.Name); ok {
		return cached.([]tool_protocol.Migration)
	}

	versionMap := make(map[int]tool_protocol.Migration)

	for _, fn := range t.tool.Functions {
		if fn.With == nil || fn.With[tool_protocol.WithMigrations] == nil {
			continue
		}

		// Parse migrations from the With block
		raw, ok := fn.With[tool_protocol.WithMigrations].([]interface{})
		if !ok {
			continue
		}

		for _, item := range raw {
			// Handle both map[string]interface{} and map[interface{}]interface{}
			var m map[string]interface{}
			switch v := item.(type) {
			case map[string]interface{}:
				m = v
			case map[interface{}]interface{}:
				m = make(map[string]interface{})
				for k, val := range v {
					if kStr, ok := k.(string); ok {
						m[kStr] = val
					}
				}
			default:
				continue
			}

			version, hasVersion := m["version"]
			sqlVal, hasSQL := m["sql"]
			if !hasVersion || !hasSQL {
				continue
			}

			// Parse version
			var versionInt int
			switch v := version.(type) {
			case int:
				versionInt = v
			case float64:
				versionInt = int(v)
			default:
				continue
			}

			sqlStr, ok := sqlVal.(string)
			if !ok || sqlStr == "" {
				continue
			}

			// Only add if not already seen (first wins, all should be identical per validation)
			if _, exists := versionMap[versionInt]; !exists {
				versionMap[versionInt] = tool_protocol.Migration{Version: versionInt, SQL: sqlStr}
			}
		}
	}

	// Convert map to sorted slice
	result := make([]tool_protocol.Migration, 0, len(versionMap))
	for _, m := range versionMap {
		result = append(result, m)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Version < result[j].Version
	})

	// Cache the result
	toolMigrationsCache.Store(t.tool.Name, result)

	return result
}

// applyPendingMigrations checks for and applies any unapplied migrations.
// Migrations are tracked in tool_schema_migrations table to ensure each version runs exactly once.
// Each migration is applied within a transaction to ensure atomicity.
// Race conditions are handled by using INSERT OR IGNORE and re-checking after insert.
func (t *YAMLDefinedTool) applyPendingMigrations(ctx context.Context, db *sql.DB) error {
	// Collect migrations from all functions in the tool
	migrations := t.collectToolMigrations()
	if len(migrations) == 0 {
		return nil
	}

	// Create tracking table if needed
	if err := t.ensureToolMigrationTable(ctx, db); err != nil {
		return fmt.Errorf("failed to create migration tracking table: %w", err)
	}

	// Get applied versions for this tool
	appliedVersions := make(map[int]bool)
	rows, err := db.QueryContext(ctx, "SELECT version FROM tool_schema_migrations WHERE tool_name = ?", t.tool.Name)
	if err != nil {
		return fmt.Errorf("failed to query applied migrations: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var version int
		if err := rows.Scan(&version); err != nil {
			return fmt.Errorf("failed to scan migration version: %w", err)
		}
		appliedVersions[version] = true
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("error iterating applied migrations: %w", err)
	}

	// Apply pending migrations in version order
	for _, m := range migrations {
		if appliedVersions[m.Version] {
			continue // Already applied
		}

		// Apply each migration in a transaction for atomicity
		if err := t.applySingleMigration(ctx, db, m); err != nil {
			return err
		}
	}

	return nil
}

// applySingleMigration applies a single migration within a transaction.
// Uses INSERT OR IGNORE to handle race conditions where multiple processes
// might try to apply the same migration concurrently.
func (t *YAMLDefinedTool) applySingleMigration(ctx context.Context, db *sql.DB, m tool_protocol.Migration) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction for migration v%d: %w", m.Version, err)
	}
	defer func() {
		if tx != nil {
			tx.Rollback()
		}
	}()

	// Try to claim this migration version using INSERT OR IGNORE
	// This handles race conditions: only one process will successfully insert
	insertSQL := "INSERT OR IGNORE INTO tool_schema_migrations (tool_name, version) VALUES (?, ?)"
	result, err := tx.ExecContext(ctx, insertSQL, t.tool.Name, m.Version)
	if err != nil {
		return fmt.Errorf("failed to claim migration v%d for tool '%s': %w", m.Version, t.tool.Name, err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check migration claim for v%d: %w", m.Version, err)
	}

	if rowsAffected == 0 {
		// Another process already claimed/applied this migration
		logger.Debugf("Migration v%d for tool '%s' already applied by another process", m.Version, t.tool.Name)
		tx.Rollback()
		tx = nil
		return nil
	}

	// We claimed it - now execute the migration SQL
	logger.Infof("Applying migration v%d for tool '%s'", m.Version, t.tool.Name)

	if _, err := tx.ExecContext(ctx, m.SQL); err != nil {
		return fmt.Errorf("migration v%d failed for tool '%s': %w", m.Version, t.tool.Name, err)
	}

	// Commit the transaction (migration SQL + tracking record)
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit migration v%d for tool '%s': %w", m.Version, t.tool.Name, err)
	}
	tx = nil // Prevent rollback in defer

	return nil
}

// executeDBOperation executes database operations
// existingStepResults allows preserving results from previous retry attempts, enabling skip of completed steps
func (t *YAMLDefinedTool) executeDBOperation(ctx context.Context, messageID string, inputs map[string]interface{}, existingStepResults map[int]interface{}) (string, map[int]interface{}, error) {
	// Get the database connection (tool-specific for user tools, shared for system tools)
	db, err := t.getToolDatabase()
	if err != nil {
		return "", existingStepResults, fmt.Errorf("error getting database connection: %w", err)
	}

	// Initialize tables if specified (runs first to ensure tables exist)
	if t.function.With != nil && t.function.With[tool_protocol.WithInit] != nil {
		initSQL, ok := t.function.With[tool_protocol.WithInit].(string)
		if ok && initSQL != "" {
			// Execute the initialization SQL
			if _, err := db.ExecContext(ctx, initSQL); err != nil {
				return "", existingStepResults, fmt.Errorf("error executing init SQL: %w", err)
			}
		}
	}

	// Apply pending migrations after init (migrations are tracked and run once per version)
	if err := t.applyPendingMigrations(ctx, db); err != nil {
		return "", existingStepResults, fmt.Errorf("error applying migrations: %w", err)
	}

	// Process inputs
	processedInputs := make(map[string]interface{})

	// First, ensure all defined inputs exist in the map
	for _, inputDef := range t.function.Input {
		if _, exists := inputs[inputDef.Name]; !exists {
			// Input doesn't exist, add it as nil
			processedInputs[inputDef.Name] = nil
		} else if val, ok := inputs[inputDef.Name].(string); ok && val == "" {
			// Empty string, convert to nil for SQL NULL
			processedInputs[inputDef.Name] = nil
		} else {
			// Copy the existing value
			processedInputs[inputDef.Name] = inputs[inputDef.Name]
		}
	}

	// Process any additional inputs not in the definition
	for key, value := range inputs {
		if _, exists := processedInputs[key]; exists {
			continue // Already processed above
		}

		// Replace variables in input values
		if strValue, ok := value.(string); ok {
			processedValue, err := t.variableReplacer.ReplaceVariables(ctx, strValue, inputs)
			if err != nil {
				processedInputs[key] = value
			} else {
				processedInputs[key] = processedValue
			}
		} else {
			processedInputs[key] = value
		}
	}

	// Execute steps
	// Use existing step results if provided (for retry scenarios)
	// This allows skipping steps that already completed successfully
	stepResults := existingStepResults
	if stepResults == nil {
		stepResults = make(map[int]interface{})
	}
	var lastOutput string

	for _, step := range t.function.Steps {
		// Skip step if it already has a result from a previous retry attempt
		// This prevents re-executing steps that already completed successfully
		if step.ResultIndex > 0 {
			if _, hasResult := stepResults[step.ResultIndex]; hasResult {
				logger.Infof("Skipping DB step '%s' - already completed in previous attempt (result at index %d)",
					step.Name, step.ResultIndex)
				continue
			}
		}

		// Evaluate runOnlyIf condition
		if step.RunOnlyIf != nil {
			shouldRun, skipInfo, err := t.evaluateStepRunOnlyIf(ctx, messageID, step, processedInputs, stepResults)
			if err != nil {
				return "", stepResults, fmt.Errorf("error evaluating runOnlyIf for db step '%s': %w", step.Name, err)
			}
			if !shouldRun {
				logger.Infof("Skipping db step '%s' due to runOnlyIf condition: %s", step.Name, skipInfo.Reason)
				if step.ResultIndex > 0 {
					skipJSON, _ := json.Marshal(skipInfo)
					stepResults[step.ResultIndex] = string(skipJSON)
				}
				continue
			}
		}

		// Handle forEach
		if step.ForEach != nil {
			forEachResult, forEachErr := t.executeForEachDBStep(ctx, db, step, processedInputs, stepResults)
			if forEachErr != nil {
				return "", stepResults, fmt.Errorf("error in forEach for db step '%s': %w", step.Name, forEachErr)
			}
			if step.ResultIndex > 0 {
				stepResults[step.ResultIndex] = forEachResult
			}
			if strResult, ok := forEachResult.(string); ok {
				lastOutput = strResult
			}
			continue
		}

		stepResult, err := t.executeDBStep(ctx, db, step, processedInputs, stepResults)
		if err != nil {
			// Extract SQL query from the step for self-fix context
			var sqlQuery string
			if step.With != nil && step.With[step.Action] != nil {
				sqlQuery, _ = step.With[step.Action].(string)
			}

			// Attempt self-fix
			fixResult, fixErr := t.attemptSelfFixDB(ctx, messageID, step.Name, sqlQuery, err.Error(), processedInputs)
			if fixErr != nil {
				logger.Warnf("[SelfFix] Failed to attempt self-fix: %v", fixErr)
			}

			if fixResult != nil && fixResult.Success {
				logger.Infof("[SelfFix] Successfully fixed database operation. Recommended action: %s. Reason: %s",
					fixResult.RecommendedAction, fixResult.Reason)

				// Record the successful self-fix event
				t.recordSelfFixExecution(ctx, messageID, "db", step.Name, err.Error(), fixResult, processedInputs)

				// Always return ErrWorkflowRestartRequired after successful fix.
				// The coordinator will handle ForceReloadTools and restart the workflow.
				// We can't retry in-place because t.function still holds the old definition.
				return "", stepResults, ErrWorkflowRestartRequired
			} else {
				// Record the failed self-fix attempt if we have a result
				if fixResult != nil {
					t.recordSelfFixExecution(ctx, messageID, "db", step.Name, err.Error(), fixResult, processedInputs)
				}
				return "", stepResults, fmt.Errorf("error executing step '%s': %w", step.Name, err)
			}
		}

		if step.ResultIndex > 0 {
			stepResults[step.ResultIndex] = stepResult
		}

		// Keep the last result as output
		if result, ok := stepResult.(string); ok {
			lastOutput = result
		} else if result, ok := stepResult.([]map[string]interface{}); ok {
			// Convert result to JSON for output
			jsonBytes, err := json.Marshal(result)
			if err == nil {
				lastOutput = string(jsonBytes)
			}
		}
	}

	return lastOutput, stepResults, nil
}

// executeTerminalOperation executes terminal/shell operations
// existingStepResults allows preserving results from previous retry attempts, enabling skip of completed steps
func (t *YAMLDefinedTool) executeTerminalOperation(ctx context.Context, messageID string, inputs map[string]interface{}, existingStepResults map[int]interface{}) (string, map[int]interface{}, error) {
	// Use existing step results if provided (for retry scenarios)
	// This allows skipping steps that already completed successfully
	stepResults := existingStepResults
	if stepResults == nil {
		stepResults = make(map[int]interface{})
	}
	var lastOutput string

	for _, step := range t.function.Steps {
		// Skip step if it already has a result from a previous retry attempt
		// This prevents re-executing steps that already completed successfully
		if step.ResultIndex > 0 {
			if _, hasResult := stepResults[step.ResultIndex]; hasResult {
				logger.Infof("Skipping terminal step '%s' - already completed in previous attempt (result at index %d)",
					step.Name, step.ResultIndex)
				continue
			}
		}

		// Use processStepParametersForTerminal to keep bash variables like $input_val
		// instead of replacing them with empty strings
		processedParams, err := t.processStepParametersForTerminal(ctx, messageID, step.With, inputs, stepResults)
		if err != nil {
			return "", stepResults, fmt.Errorf("error processing step parameters: %w", err)
		}

		result, err := t.executeTerminalStep(ctx, messageID, step, processedParams)
		if err != nil {
			// Extract script and output from error for self-fix context
			currentOS := t.detectOS()
			var script string
			switch currentOS {
			case "linux":
				script, _ = processedParams[tool_protocol.StepWithLinux].(string)
			case "darwin":
				if macScript, ok := processedParams[tool_protocol.StepWithMacOS].(string); ok {
					script = macScript
				} else {
					script, _ = processedParams[tool_protocol.StepWithLinux].(string)
				}
			case "windows":
				script, _ = processedParams[tool_protocol.StepWithWindows].(string)
			}

			// Extract output from error message if present (format: "script execution failed: ...\nOutput: ...")
			errorMsg := err.Error()
			execOutput := ""
			if idx := strings.Index(errorMsg, "\nOutput: "); idx != -1 {
				execOutput = errorMsg[idx+len("\nOutput: "):]
				errorMsg = errorMsg[:idx]
			}

			// Record the original failed execution BEFORE attempting selffix
			// This ensures we have a record of what failed even if selffix succeeds
			t.recordFailedStepExecution(ctx, messageID, step.Name, errorMsg, execOutput, inputs)

			// Attempt self-fix
			fixResult, fixErr := t.attemptSelfFixTerminal(ctx, messageID, step.Name, script, errorMsg, execOutput, inputs)
			if fixErr != nil {
				logger.Warnf("[SelfFix] Failed to attempt self-fix: %v", fixErr)
			}

			if fixResult != nil && fixResult.Success {
				logger.Infof("[SelfFix] Successfully fixed terminal operation. Recommended action: %s. Reason: %s",
					fixResult.RecommendedAction, fixResult.Reason)

				// Record the successful self-fix event
				t.recordSelfFixExecution(ctx, messageID, "terminal", step.Name, err.Error(), fixResult, inputs)

				// Always return ErrWorkflowRestartRequired after successful fix.
				// The coordinator will handle ForceReloadTools and restart the workflow.
				// We can't retry in-place because t.function still holds the old definition.
				return "", stepResults, ErrWorkflowRestartRequired
			} else {
				// Record the failed self-fix attempt if we have a result
				if fixResult != nil {
					t.recordSelfFixExecution(ctx, messageID, "terminal", step.Name, err.Error(), fixResult, inputs)
				}
				return "", stepResults, fmt.Errorf("error executing terminal step '%s': %w", step.Name, err)
			}
		}

		if step.ResultIndex > 0 {
			stepResults[step.ResultIndex] = result
		}

		if strResult, ok := result.(string); ok {
			lastOutput = strResult
		}
	}

	return lastOutput, stepResults, nil
}

func (t *YAMLDefinedTool) executeTerminalStep(ctx context.Context, messageID string, step tool_protocol.Step, params map[string]interface{}) (interface{}, error) {
	// Check for mock service in context (test mode only - zero-cost in production)
	if mockSvc, ok := ctx.Value(tool_engine_models.TestMockServiceKey).(tool_engine_models.IMockService); ok && mockSvc != nil {
		functionKey := GenerateFunctionKey(t.tool.Name, t.function.Name)
		if mockSvc.ShouldMock("terminal", functionKey, step.Name) {
			if response, exists := mockSvc.GetMockResponseValue(functionKey, step.Name); exists {
				mockSvc.RecordCall(functionKey, step.Name, "terminal", params, response, "")
				logger.Infof("[TEST MOCK] Returning mock for terminal %s.%s", functionKey, step.Name)
				return response, nil
			}
		}
	}

	// Check for container executor in context (test mode with container_operations)
	// This allows terminal commands to run in an isolated container matching production environment
	if containerExec, ok := ctx.Value(tool_engine_models.TestContainerExecutorKey).(tool_engine_models.IContainerExecutor); ok && containerExec != nil {
		if containerExec.IsStarted() {
			// Use linux script in container (container is always Linux-based)
			script, exists := params[tool_protocol.StepWithLinux].(string)
			if !exists || script == "" {
				return nil, fmt.Errorf("no linux script found for container execution")
			}

			logger.Infof("[TEST CONTAINER] Executing terminal step %s in container", step.Name)

			// Execute script in container
			output, err := containerExec.ExecuteBash(ctx, script)
			if err != nil {
				return nil, fmt.Errorf("container execution failed: %w", err)
			}

			return output, nil
		}
	}

	currentOS := t.detectOS()

	var script string
	var exists bool

	switch currentOS {
	case "linux":
		script, exists = params[tool_protocol.StepWithLinux].(string)
	case "darwin":
		if macScript, ok := params[tool_protocol.StepWithMacOS].(string); ok {
			script = macScript
			exists = true
		} else {
			script, exists = params[tool_protocol.StepWithLinux].(string)
		}
	case "windows":
		script, exists = params[tool_protocol.StepWithWindows].(string)
	default:
		return nil, fmt.Errorf("unsupported operating system: %s", currentOS)
	}

	if !exists || script == "" {
		return nil, fmt.Errorf("no script found for operating system: %s", currentOS)
	}

	timeout := 30
	if timeoutVal, exists := params[tool_protocol.StepWithTimeout]; exists {
		if timeoutInt, ok := timeoutVal.(int); ok {
			timeout = timeoutInt
		}
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	return t.executeScript(timeoutCtx, messageID, script, step.Action, currentOS)
}

// executeCodeOperation executes a code operation by running Claude Code SDK prompts.
// Follows the same step-loop pattern as executeTerminalOperation.
// When reuseSession is true, all steps share a single Claude Code process.
func (t *YAMLDefinedTool) executeCodeOperation(ctx context.Context, messageID string, inputs map[string]interface{}, existingStepResults map[int]interface{}) (string, map[int]interface{}, error) {
	stepResults := existingStepResults
	if stepResults == nil {
		stepResults = make(map[int]interface{})
	}
	var lastOutput string

	// Collect debug info for all steps
	var codeDebugDetails CodeExecutionDetails

	// Session management for reuseSession mode
	var session *codeSession
	if t.function.ReuseSession {
		defer func() {
			if session != nil {
				session.Close()
			}
		}()
	}

	for _, step := range t.function.Steps {
		// Skip step if it already has a result from a previous retry attempt
		if step.ResultIndex > 0 {
			if _, hasResult := stepResults[step.ResultIndex]; hasResult {
				logger.Infof("Skipping code step '%s' - already completed in previous attempt (result at index %d)",
					step.Name, step.ResultIndex)
				continue
			}
		}

		// Evaluate runOnlyIf condition
		if step.RunOnlyIf != nil {
			shouldRun, skipInfo, err := t.evaluateStepRunOnlyIf(ctx, messageID, step, inputs, stepResults)
			if err != nil {
				return "", stepResults, fmt.Errorf("error evaluating runOnlyIf for code step '%s': %w", step.Name, err)
			}
			if !shouldRun {
				logger.Infof("Skipping code step '%s' due to runOnlyIf condition: %s", step.Name, skipInfo.Reason)
				if step.ResultIndex > 0 {
					skipJSON, _ := json.Marshal(skipInfo)
					stepResults[step.ResultIndex] = string(skipJSON)
				}
				continue
			}
		}

		// Process step parameters (standard variant, not terminal)
		processedParams, err := t.processStepParameters(ctx, messageID, step.With, inputs, stepResults)
		if err != nil {
			return "", stepResults, fmt.Errorf("error processing step parameters for code step '%s': %w", step.Name, err)
		}

		// Handle forEach
		if step.ForEach != nil {
			forEachResult, forEachErr := t.executeForEachCodeStep(ctx, messageID, step, processedParams, inputs, stepResults)
			if forEachErr != nil {
				return "", stepResults, fmt.Errorf("error in forEach for code step '%s': %w", step.Name, forEachErr)
			}
			if step.ResultIndex > 0 {
				stepResults[step.ResultIndex] = forEachResult
			}
			if strResult, ok := forEachResult.(string); ok {
				lastOutput = strResult
			}
			continue
		}

		var result interface{}
		var debugInfo *CodeStepDebugInfo
		if t.function.ReuseSession {
			result, debugInfo, err = t.executeCodeStepWithSession(ctx, messageID, step, processedParams, &session)
		} else {
			result, debugInfo, err = t.executeCodeStep(ctx, messageID, step, processedParams)
		}
		if debugInfo != nil {
			codeDebugDetails.Steps = append(codeDebugDetails.Steps, *debugInfo)
		}
		if err != nil {
			// Record the failed step execution for diagnostics (matches terminal/db pattern)
			t.recordFailedStepExecution(ctx, messageID, step.Name, err.Error(), "", inputs)
			// Store debug details collected so far
			t.storeCodeDebugDetails(ctx, &codeDebugDetails)
			return "", stepResults, fmt.Errorf("error executing code step '%s': %w", step.Name, err)
		}

		if step.ResultIndex > 0 {
			stepResults[step.ResultIndex] = result
		}

		if strResult, ok := result.(string); ok {
			lastOutput = strResult
			logger.Infof("Code step '%s' completed (result length: %d)", step.Name, len(strResult))
		}
	}

	// Store debug details for all steps
	t.storeCodeDebugDetails(ctx, &codeDebugDetails)

	return lastOutput, stepResults, nil
}

// executeForEachCodeStep handles forEach iteration for code steps
func (t *YAMLDefinedTool) executeForEachCodeStep(ctx context.Context, messageID string, step tool_protocol.Step, params map[string]interface{}, inputs map[string]interface{}, stepResults map[int]interface{}) (interface{}, error) {
	forEach := step.ForEach

	// Resolve forEach items
	separator := forEach.Separator
	if separator == "" {
		separator = tool_protocol.DefaultForEachSeparator
	}
	indexVar := forEach.IndexVar
	if indexVar == "" {
		indexVar = tool_protocol.DefaultForEachIndexVar
	}
	itemVar := forEach.ItemVar
	if itemVar == "" {
		itemVar = tool_protocol.DefaultForEachItemVar
	}

	// Get the items to iterate over
	itemsStr, ok := params[tool_protocol.ForEachItems].(string)
	if !ok {
		// Try from forEach.Items directly with variable replacement
		replaced, err := t.variableReplacer.ReplaceVariables(ctx, forEach.Items, inputs)
		if err != nil {
			return nil, fmt.Errorf("error resolving forEach items: %w", err)
		}
		itemsStr = replaced
	}

	items := strings.Split(itemsStr, separator)
	results := make([]interface{}, 0)

	for idx, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}

		// Build loop-specific params
		loopParams := make(map[string]interface{})
		for k, v := range params {
			loopParams[k] = v
		}
		loopParams[indexVar] = idx
		loopParams[itemVar] = item

		// Re-process step parameters with loop context
		loopInputs := make(map[string]interface{})
		for k, v := range inputs {
			loopInputs[k] = v
		}
		loopInputs[indexVar] = idx
		loopInputs[itemVar] = item

		processedLoopParams, err := t.processStepParameters(ctx, messageID, step.With, loopInputs, stepResults)
		if err != nil {
			return nil, fmt.Errorf("error processing forEach params at index %d: %w", idx, err)
		}

		result, _, err := t.executeCodeStep(ctx, messageID, step, processedLoopParams)
		if err != nil {
			return nil, fmt.Errorf("error in forEach iteration %d: %w", idx, err)
		}
		results = append(results, result)
	}

	// Join results
	resultJSON, err := json.Marshal(results)
	if err != nil {
		return nil, fmt.Errorf("error marshaling forEach results: %w", err)
	}
	return string(resultJSON), nil
}

// executeCodeStepWithSession executes a code step using a shared persistent session.
// On first call (session is nil), builds options from step params and creates the session.
// On subsequent calls, only the prompt is used — other config fields are ignored with a warning.
func (t *YAMLDefinedTool) executeCodeStepWithSession(ctx context.Context, messageID string, step tool_protocol.Step, params map[string]interface{}, session **codeSession) (interface{}, *CodeStepDebugInfo, error) {
	// Check for mock service in context (test mode only - zero-cost in production)
	if mockSvc, ok := ctx.Value(tool_engine_models.TestMockServiceKey).(tool_engine_models.IMockService); ok && mockSvc != nil {
		functionKey := GenerateFunctionKey(t.tool.Name, t.function.Name)
		if mockSvc.ShouldMock("code", functionKey, step.Name) {
			if response, exists := mockSvc.GetMockResponseValue(functionKey, step.Name); exists {
				mockSvc.RecordCall(functionKey, step.Name, "code", params, response, "")
				logger.Infof("[TEST MOCK] Returning mock for code %s.%s", functionKey, step.Name)
				return response, nil, nil
			}
		}
	}

	// Extract prompt (required for every step)
	prompt, _ := params[tool_protocol.StepWithPrompt].(string)
	if prompt == "" {
		return nil, nil, fmt.Errorf("code step '%s' missing required prompt", step.Name)
	}

	// Append structured output suffix
	prompt = prompt + tool_protocol.CodeStepOutputSuffix

	debugInfo := &CodeStepDebugInfo{
		StepName: step.Name,
		Prompt:   prompt,
		Options:  codeStepOptionsDebugMap(params),
	}

	// First step creates the session with full configuration
	if *session == nil {
		opts := t.buildCodeStepOptions(params)

		timeout := tool_protocol.DefaultCodeTimeout
		if t2, ok := toIntFromInterface(params[tool_protocol.StepWithTimeout]); ok && t2 > 0 {
			timeout = t2
		}

		timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
		defer cancel()

		*session = newCodeSessionFromOpts(opts)

		result, err := (*session).SendPrompt(timeoutCtx, prompt)
		if err != nil {
			debugInfo.Success = false
			debugInfo.Error = err.Error()
			return nil, debugInfo, fmt.Errorf("code step '%s' session execution failed: %w", step.Name, err)
		}

		resultText := result.Text()
		debugInfo.RawOutput = resultText

		if !result.Success() {
			errDetail := result.Subtype
			if len(result.Errors) > 0 {
				errDetail = strings.Join(result.Errors, "; ")
			}
			debugInfo.Success = false
			debugInfo.Error = fmt.Sprintf("completed with error (%s): %s", result.Subtype, errDetail)
			return nil, debugInfo, fmt.Errorf("code step '%s' completed with error (%s): %s", step.Name, result.Subtype, errDetail)
		}

		// Parse structured success/error from JSON response
		if trimmed := strings.TrimSpace(resultText); len(trimmed) > 0 && trimmed[0] == '{' {
			var parsed map[string]interface{}
			if err := json.Unmarshal([]byte(trimmed), &parsed); err == nil {
				if success, ok := parsed["success"].(bool); ok && !success {
					errMsg := "task failed without error details"
					if e, ok := parsed["error"].(string); ok && e != "" {
						errMsg = e
					}
					debugInfo.Success = false
					debugInfo.Error = errMsg
					return nil, debugInfo, fmt.Errorf("code step '%s' task failed: %s", step.Name, errMsg)
				}
			}
		}

		debugInfo.Success = true
		return resultText, debugInfo, nil
	}

	// Subsequent steps: warn if config fields are specified (they're ignored)
	configFields := []string{
		tool_protocol.StepWithModel, tool_protocol.StepWithCwd, tool_protocol.StepWithMaxTurns,
		tool_protocol.StepWithSystemPrompt, tool_protocol.StepWithAllowedTools,
		tool_protocol.StepWithDisallowedTools, tool_protocol.StepWithIsPlanMode,
		tool_protocol.StepWithAdditionalDirs, tool_protocol.StepWithTaskComplexityLevel,
	}
	for _, field := range configFields {
		if _, exists := params[field]; exists {
			logger.Warnf("Code step '%s' has '%s' but reuseSession is true — config from first step is used, this field is ignored", step.Name, field)
		}
	}

	timeout := tool_protocol.DefaultCodeTimeout
	if t2, ok := toIntFromInterface(params[tool_protocol.StepWithTimeout]); ok && t2 > 0 {
		timeout = t2
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	result, err := (*session).SendPrompt(timeoutCtx, prompt)
	if err != nil {
		debugInfo.Success = false
		debugInfo.Error = err.Error()
		return nil, debugInfo, fmt.Errorf("code step '%s' session execution failed: %w", step.Name, err)
	}

	resultText := result.Text()
	debugInfo.RawOutput = resultText

	if !result.Success() {
		errDetail := result.Subtype
		if len(result.Errors) > 0 {
			errDetail = strings.Join(result.Errors, "; ")
		}
		debugInfo.Success = false
		debugInfo.Error = fmt.Sprintf("completed with error (%s): %s", result.Subtype, errDetail)
		return nil, debugInfo, fmt.Errorf("code step '%s' completed with error (%s): %s", step.Name, result.Subtype, errDetail)
	}

	// Parse structured success/error from JSON response.
	// Claude often returns prose before the JSON, so find the last valid JSON object.
	if parsed := extractLastJSONObject(resultText); parsed != nil {
		if success, ok := parsed["success"].(bool); ok && !success {
			errMsg := "task failed without error details"
			if e, ok := parsed["error"].(string); ok && e != "" {
				errMsg = e
			}
			debugInfo.Success = false
			debugInfo.Error = errMsg
			return nil, debugInfo, fmt.Errorf("code step '%s' task failed: %s", step.Name, errMsg)
		}
	}

	debugInfo.Success = true
	return resultText, debugInfo, nil
}

// storeCodeDebugDetails saves collected debug details onto the tool struct for later inclusion in original_output.
func (t *YAMLDefinedTool) storeCodeDebugDetails(ctx context.Context, details *CodeExecutionDetails) {
	if details != nil && len(details.Steps) > 0 {
		t.codeDebugDetails = details
		logger.Debugf("Stored code execution debug details — %d steps", len(details.Steps))
	}
}

// codeStepOptionsDebugMap extracts the resolved options from step params into a debug-friendly map.
func codeStepOptionsDebugMap(params map[string]interface{}) map[string]interface{} {
	debug := make(map[string]interface{})
	for _, key := range []string{
		tool_protocol.StepWithModel, tool_protocol.StepWithCwd, tool_protocol.StepWithMaxTurns,
		tool_protocol.StepWithSystemPrompt, tool_protocol.StepWithAllowedTools,
		tool_protocol.StepWithDisallowedTools, tool_protocol.StepWithIsPlanMode,
		tool_protocol.StepWithAdditionalDirs, tool_protocol.StepWithTaskComplexityLevel,
		tool_protocol.StepWithTimeout,
	} {
		if v, ok := params[key]; ok {
			debug[key] = v
		}
	}
	if envModel := os.Getenv("CLAUDE_MODEL"); envModel != "" {
		debug["CLAUDE_MODEL_env"] = envModel
	}
	if os.Getenv("ANTHROPIC_API_KEY") != "" {
		debug["ANTHROPIC_API_KEY_override"] = "cleared (OAuth fallback)"
	}
	return debug
}

// extractLastJSONObject finds the last valid JSON object in text.
// Claude Code often returns prose followed by JSON. This searches backwards
// for the last '{' that parses as a complete JSON object.
// Returns nil if no valid JSON object is found.
func extractLastJSONObject(text string) map[string]interface{} {
	trimmed := strings.TrimSpace(text)
	if len(trimmed) == 0 {
		return nil
	}

	// Search backwards for opening braces
	for i := len(trimmed) - 1; i >= 0; i-- {
		if trimmed[i] != '{' {
			continue
		}
		candidate := trimmed[i:]
		var parsed map[string]interface{}
		if err := json.Unmarshal([]byte(candidate), &parsed); err == nil {
			return parsed
		}
	}
	return nil
}

// extractDirsFromValue parses a JSON array value into a list of directory paths.
// Handles two formats:
//   - Array of objects with "local_path" field (from getProjectRepos)
//   - Array of plain strings
//
// Returns nil if the value is not a JSON array, allowing the caller to use it as-is.
func extractDirsFromValue(val string) []string {
	val = strings.TrimSpace(val)
	if val == "" || val[0] != '[' {
		return nil
	}
	// Try JSON array of objects with local_path (from getProjectRepos)
	var repos []map[string]interface{}
	if err := json.Unmarshal([]byte(val), &repos); err == nil && len(repos) > 0 {
		var dirs []string
		for _, repo := range repos {
			if path, ok := repo["local_path"].(string); ok && path != "" {
				dirs = append(dirs, path)
			}
		}
		if len(dirs) > 0 {
			return dirs
		}
	}
	// Try JSON array of strings
	var paths []string
	if err := json.Unmarshal([]byte(val), &paths); err == nil && len(paths) > 0 {
		return paths
	}
	return nil
}

// buildCodeStepOptions builds CodeExecutorOptions from step parameters.
// Shared between executeCodeStep and executeCodeStepWithSession (first step).
func (t *YAMLDefinedTool) buildCodeStepOptions(params map[string]interface{}) CodeExecutorOptions {
	opts := CodeExecutorOptions{}

	// Priority: CLAUDE_MODEL env > taskComplexityLevel > model > DefaultCodeModel
	opts.Model = tool_protocol.DefaultCodeModel
	if m, ok := params[tool_protocol.StepWithModel].(string); ok && m != "" {
		opts.Model = m
	}
	if level, ok := params[tool_protocol.StepWithTaskComplexityLevel].(string); ok && level != "" {
		opts.Model = resolveModelForComplexity(level)
	}
	if envModel := os.Getenv("CLAUDE_MODEL"); envModel != "" {
		opts.Model = envModel
	}

	opts.MaxTurns = tool_protocol.DefaultCodeMaxTurns
	if mt, ok := toIntFromInterface(params[tool_protocol.StepWithMaxTurns]); ok && mt > 0 {
		opts.MaxTurns = mt
	}

	if pm, ok := params[tool_protocol.StepWithIsPlanMode].(bool); ok {
		opts.IsPlanMode = pm
	}

	if cwd, ok := params[tool_protocol.StepWithCwd].(string); ok && cwd != "" {
		if dirs := extractDirsFromValue(cwd); len(dirs) > 0 {
			opts.WorkDir = dirs[0]
		} else {
			opts.WorkDir = cwd
		}
	}

	// Permission logic: allowed tools
	if !opts.IsPlanMode {
		if allowedTools := extractStringSlice(params[tool_protocol.StepWithAllowedTools]); len(allowedTools) > 0 {
			opts.AllowedTools = allowedTools
		}
	}

	return opts
}

// executeCodeStep executes a single code step by sending a prompt to Claude Code SDK.
func (t *YAMLDefinedTool) executeCodeStep(ctx context.Context, messageID string, step tool_protocol.Step, params map[string]interface{}) (interface{}, *CodeStepDebugInfo, error) {
	// Check for mock service in context (test mode only - zero-cost in production)
	if mockSvc, ok := ctx.Value(tool_engine_models.TestMockServiceKey).(tool_engine_models.IMockService); ok && mockSvc != nil {
		functionKey := GenerateFunctionKey(t.tool.Name, t.function.Name)
		if mockSvc.ShouldMock("code", functionKey, step.Name) {
			if response, exists := mockSvc.GetMockResponseValue(functionKey, step.Name); exists {
				mockSvc.RecordCall(functionKey, step.Name, "code", params, response, "")
				logger.Infof("[TEST MOCK] Returning mock for code %s.%s", functionKey, step.Name)
				return response, nil, nil
			}
		}
	}

	// Extract prompt (required)
	prompt, _ := params[tool_protocol.StepWithPrompt].(string)
	if prompt == "" {
		return nil, nil, fmt.Errorf("code step '%s' missing required prompt", step.Name)
	}

	// Append structured output suffix
	prompt = prompt + tool_protocol.CodeStepOutputSuffix

	opts := t.buildCodeStepOptions(params)
	debugOpts := codeStepOptionsDebugMap(params)

	timeout := tool_protocol.DefaultCodeTimeout
	if t2, ok := toIntFromInterface(params[tool_protocol.StepWithTimeout]); ok && t2 > 0 {
		timeout = t2
	}

	// Execute with timeout
	timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	client := newCodeClientFromOpts(opts)
	result, err := client.SendPrompt(timeoutCtx, prompt)

	// Build debug info regardless of outcome
	debugInfo := &CodeStepDebugInfo{
		StepName: step.Name,
		Prompt:   prompt,
		Options:  debugOpts,
	}

	if err != nil {
		debugInfo.Success = false
		debugInfo.Error = err.Error()
		return nil, debugInfo, fmt.Errorf("code step '%s' execution failed: %w", step.Name, err)
	}

	resultText := result.Text()
	debugInfo.RawOutput = resultText

	if !result.Success() {
		errDetail := result.Subtype
		if len(result.Errors) > 0 {
			errDetail = strings.Join(result.Errors, "; ")
		}
		debugInfo.Success = false
		debugInfo.Error = fmt.Sprintf("completed with error (%s): %s", result.Subtype, errDetail)
		return nil, debugInfo, fmt.Errorf("code step '%s' completed with error (%s): %s", step.Name, result.Subtype, errDetail)
	}

	// Try to extract structured success/error from JSON response.
	// Claude often returns prose before the JSON, so find the last valid JSON object.
	if parsed := extractLastJSONObject(resultText); parsed != nil {
		if success, ok := parsed["success"].(bool); ok && !success {
			errMsg := "task failed without error details"
			if e, ok := parsed["error"].(string); ok && e != "" {
				errMsg = e
			}
			debugInfo.Success = false
			debugInfo.Error = errMsg
			return nil, debugInfo, fmt.Errorf("code step '%s' task failed: %s", step.Name, errMsg)
		}
	}

	debugInfo.Success = true
	return resultText, debugInfo, nil
}

// resolveModelForComplexity maps a task complexity level to a specific Claude model ID.
func resolveModelForComplexity(level string) string {
	switch strings.ToLower(level) {
	case tool_protocol.TaskComplexityLow:
		return tool_protocol.DefaultModelLow
	case tool_protocol.TaskComplexityMedium:
		return tool_protocol.DefaultModelMedium
	case tool_protocol.TaskComplexityHigh:
		return tool_protocol.DefaultModelHigh
	default:
		return tool_protocol.DefaultCodeModel
	}
}

// extractStringSlice converts an interface{} (string, []interface{}, or []string) to []string.
func extractStringSlice(v interface{}) []string {
	if v == nil {
		return nil
	}
	switch s := v.(type) {
	case string:
		if s != "" {
			return []string{s}
		}
		return nil
	case []string:
		return s
	case []interface{}:
		result := make([]string, 0, len(s))
		for _, item := range s {
			if str, ok := item.(string); ok {
				result = append(result, str)
			}
		}
		return result
	}
	return nil
}

// toIntFromInterface converts interface{} to int, handling int, float64, int64.
func toIntFromInterface(v interface{}) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case float64:
		return int(n), true
	case int64:
		return int(n), true
	}
	return 0, false
}

func (t *YAMLDefinedTool) detectOS() string {
	switch runtime.GOOS {
	case "linux":
		return "linux"
	case "darwin":
		return "darwin"
	case "windows":
		return "windows"
	default:
		return "unknown"
	}
}

// maxArgSize is the threshold above which we use a temp file instead of passing script as argument.
// Linux ARG_MAX is typically 128KB-2MB. We use 100KB to be safe.
const maxArgSize = 100 * 1024 // 100KB

func (t *YAMLDefinedTool) executeScript(ctx context.Context, messageID, script, action, osType string) (interface{}, error) {
	var cmd *exec.Cmd
	var tempFile string

	// If script is large, write to temp file to avoid "argument list too long" error
	useTempFile := len(script) > maxArgSize

	switch osType {
	case "linux", "darwin":
		if useTempFile {
			// Write script to temp file
			var err error
			tempFile, err = t.writeScriptToTempFile(script, osType, action, messageID)
			if err != nil {
				return nil, fmt.Errorf("failed to write script to temp file: %w", err)
			}
			defer os.Remove(tempFile)

			if action == tool_protocol.StepActionBash {
				cmd = exec.CommandContext(ctx, "bash", tempFile)
			} else {
				cmd = exec.CommandContext(ctx, "sh", tempFile)
			}
		} else {
			if action == tool_protocol.StepActionBash {
				cmd = exec.CommandContext(ctx, "bash", "-c", script)
			} else {
				cmd = exec.CommandContext(ctx, "sh", "-c", script)
			}
		}
	case "windows":
		if useTempFile {
			var err error
			tempFile, err = t.writeScriptToTempFile(script, osType, action, messageID)
			if err != nil {
				return nil, fmt.Errorf("failed to write script to temp file: %w", err)
			}
			defer os.Remove(tempFile)
			cmd = exec.CommandContext(ctx, "cmd", "/C", tempFile)
		} else {
			cmd = exec.CommandContext(ctx, "cmd", "/C", script)
		}
	default:
		return nil, fmt.Errorf("unsupported operating system: %s", osType)
	}

	output, err := cmd.CombinedOutput()
	defer func() {
		// Save the failed script to a file
		if saveErr := t.saveFailedScript(messageID, script, osType, action, err, string(output)); saveErr != nil {
			fmt.Printf("Warning: Failed to save failed script: %v\n", saveErr)
		}
	}()

	if err != nil {
		return nil, fmt.Errorf("script execution failed: %w\nOutput: %s", err, truncateString(string(output), 1000))
	}

	// Trim whitespace from output to avoid issues with trailing newlines in userIds, etc.
	return strings.TrimSpace(string(output)), nil
}

// writeScriptToTempFile writes the script to a temporary file and returns the file path
func (t *YAMLDefinedTool) writeScriptToTempFile(script, osType, action, messageID string) (string, error) {
	// Determine file extension
	ext := ".sh"
	if osType == "windows" {
		ext = ".bat"
	} else if action == tool_protocol.StepActionBash {
		ext = ".bash"
	}

	// Include timestamp and messageID for uniqueness and traceability
	timestamp := time.Now().UnixNano()
	pattern := fmt.Sprintf("connectai-script-%s-%d-*%s", messageID, timestamp, ext)

	// Create temp file
	tempFile, err := os.CreateTemp("", pattern)
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	defer tempFile.Close()

	// Write script content
	if _, err := tempFile.WriteString(script); err != nil {
		os.Remove(tempFile.Name())
		return "", fmt.Errorf("failed to write script to temp file: %w", err)
	}

	// Make executable on Unix systems
	if osType != "windows" {
		if err := os.Chmod(tempFile.Name(), 0755); err != nil {
			os.Remove(tempFile.Name())
			return "", fmt.Errorf("failed to make script executable: %w", err)
		}
	}

	return tempFile.Name(), nil
}

// saveFailedScript saves a failed script execution to the .connectai directory (only in LOCAL environment)
func (t *YAMLDefinedTool) saveFailedScript(messageID, script, osType, action string, execErr error, output string) error {
	// Only save failed scripts in LOCAL environment
	if os.Getenv("ENVIRONMENT") != "LOCAL" {
		return nil
	}

	// Get the .connectai directory
	connectAIDir, err := getDataDir()
	if err != nil {
		return fmt.Errorf("failed to get ConnectAI directory: %w", err)
	}

	// Create a failed-scripts subdirectory
	failedScriptsDir := filepath.Join(connectAIDir, "failed-scripts")
	if err := os.MkdirAll(failedScriptsDir, 0755); err != nil {
		return fmt.Errorf("failed to create failed-scripts directory: %w", err)
	}

	// Generate a filename with tool name, function name, message ID and timestamp
	timestamp := time.Now().Format("20060102-150405")
	toolName := "unknown"
	if t.tool != nil {
		toolName = t.tool.Name
	}
	functionName := "unknown"
	if t.function != nil {
		functionName = t.function.Name
	}

	// Determine file extension based on OS and action
	ext := ".sh"
	if osType == "windows" {
		ext = ".bat"
	} else if action == tool_protocol.StepActionBash {
		ext = ".bash"
	}

	filename := fmt.Sprintf("%s_%s_%s_%s_%s%s", toolName, functionName, messageID, timestamp, osType, ext)
	filepath := filepath.Join(failedScriptsDir, filename)

	// Create the file content with metadata
	content := fmt.Sprintf(`#!/bin/bash
# Failed Script Execution
# Tool: %s
# Function: %s
# Message ID: %s
# OS: %s
# Action: %s
# Timestamp: %s
# Error: %v
# Output:
# %s
#
# ===== SCRIPT CONTENT =====

%s
`, toolName, functionName, messageID, osType, action, time.Now().Format(time.RFC3339), execErr, strings.ReplaceAll(output, "\n", "\n# "), script)

	// Write the file
	if err := os.WriteFile(filepath, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write script file: %w", err)
	}

	fmt.Printf("Failed script saved to: %s\n", filepath)
	return nil
}

// getSelfFixer attempts to get the SelfFixer from the toolEngine via type assertion
func (t *YAMLDefinedTool) getSelfFixer() SelfFixerInterface {
	if te, ok := t.toolEngine.(*ToolEngine); ok {
		return te.GetSelfFixer()
	}
	return nil
}

// getToolFilePath gets the file path for the current tool from the toolEngine
func (t *YAMLDefinedTool) getToolFilePath() string {
	if te, ok := t.toolEngine.(*ToolEngine); ok {
		return te.GetToolFilePath(t.tool.Name, t.tool.Version)
	}
	return ""
}

// getToolYAML reads and returns the current tool YAML content
func (t *YAMLDefinedTool) getToolYAML() (string, error) {
	filePath := t.getToolFilePath()
	if filePath == "" {
		return "", fmt.Errorf("tool file path not found")
	}
	content, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to read tool YAML: %w", err)
	}
	return string(content), nil
}

// getEnvironmentInfo returns the current environment information for self-fix
func (t *YAMLDefinedTool) getEnvironmentInfo() selfFixEnvironmentInfo {
	return selfFixEnvironmentInfo{
		OS:             runtime.GOOS,
		DBType:         "sqlite",
		AvailableTools: []string{"bash", "sh", "jq", "sqlite3"},
	}
}

// getToolsDBPath returns the path to the tools.db database
func (t *YAMLDefinedTool) getToolsDBPath() string {
	connectaiDir, err := getDataDir()
	if err != nil {
		return ""
	}
	return filepath.Join(connectaiDir, "tools.db")
}

// getValidateYAMLBin returns the path to the validate-yaml binary
func (t *YAMLDefinedTool) getValidateYAMLBin() string {
	toolFilePath := t.getToolFilePath()
	if toolFilePath == "" {
		return ""
	}

	// The validate-yaml binary can be in several locations:
	// 1. Same directory as the tool YAML file (e.g., ~/.connectai/instance_xxx/tools/validate-yaml)
	// 2. Parent of tools directory (e.g., ~/.connectai/instance_xxx/validate-yaml)

	toolDir := filepath.Dir(toolFilePath)

	// Check in the same directory as the YAML file
	validatePath := filepath.Join(toolDir, "validate-yaml")
	if _, err := os.Stat(validatePath); err == nil {
		return validatePath
	}

	// Check in parent directory (instance root)
	instanceRoot := filepath.Dir(toolDir)
	validatePath = filepath.Join(instanceRoot, "validate-yaml")
	if _, err := os.Stat(validatePath); err == nil {
		return validatePath
	}

	return ""
}

// attemptSelfFixTerminal attempts to fix a terminal operation failure
func (t *YAMLDefinedTool) attemptSelfFixTerminal(
	ctx context.Context,
	messageID string,
	stepName string,
	script string,
	errorMessage string,
	output string,
	inputs map[string]interface{},
) (*selfFixResult, error) {
	sf := t.getSelfFixer()
	if sf == nil || !sf.IsEnabled() {
		return nil, nil // Self-fix not enabled
	}

	execCtx := selfFixExecutionContext{
		OperationType:   selfFixOpTerminal,
		ToolFilePath:    t.getToolFilePath(),
		FunctionName:    t.function.Name,
		StepName:        stepName,
		Script:          script,
		ErrorMessage:    errorMessage,
		Output:          output,
		Inputs:          inputs,
		Environment:     t.getEnvironmentInfo(),
		MessageID:       messageID,
		ToolsDBPath:     t.getToolsDBPath(),
		ValidateYAMLBin: t.getValidateYAMLBin(),
	}

	logger.Infof("[SelfFix] Attempting to fix terminal operation failure in function '%s', step '%s'", t.function.Name, stepName)
	raw, err := sf.AttemptFix(ctx, execCtx)
	if err != nil {
		return nil, err
	}
	if result, ok := raw.(*selfFixResult); ok {
		return result, nil
	}
	return nil, nil
}

// attemptSelfFixDB attempts to fix a database operation failure
func (t *YAMLDefinedTool) attemptSelfFixDB(
	ctx context.Context,
	messageID string,
	stepName string,
	sqlQuery string,
	errorMessage string,
	inputs map[string]interface{},
) (*selfFixResult, error) {
	sf := t.getSelfFixer()
	if sf == nil || !sf.IsEnabled() {
		return nil, nil // Self-fix not enabled
	}

	execCtx := selfFixExecutionContext{
		OperationType:   selfFixOpDB,
		ToolFilePath:    t.getToolFilePath(),
		FunctionName:    t.function.Name,
		StepName:        stepName,
		Script:          sqlQuery,
		ErrorMessage:    errorMessage,
		Output:          "",
		Inputs:          inputs,
		Environment:     t.getEnvironmentInfo(),
		MessageID:       messageID,
		ToolsDBPath:     t.getToolsDBPath(),
		ValidateYAMLBin: t.getValidateYAMLBin(),
	}

	logger.Infof("[SelfFix] Attempting to fix database operation failure in function '%s', step '%s'", t.function.Name, stepName)
	raw, err := sf.AttemptFix(ctx, execCtx)
	if err != nil {
		return nil, err
	}
	if result, ok := raw.(*selfFixResult); ok {
		return result, nil
	}
	return nil, nil
}

// recordFailedStepExecution records a failed step execution to the function_executions table
// This is called BEFORE attempting selffix to ensure the original failure is captured
func (t *YAMLDefinedTool) recordFailedStepExecution(
	ctx context.Context,
	messageID string,
	stepName string,
	errorMessage string,
	execOutput string,
	inputs map[string]interface{},
) {
	if t.executionTracker == nil {
		return
	}

	// Get clientID from context
	var clientID string
	if retrievedMsg, ok := ctx.Value(MessageInContextKey).(Message); ok {
		clientID = retrievedMsg.ClientID
	}

	// Build the output message with failure details
	output := fmt.Sprintf("[FAILED] Step '%s' execution failed.\nError: %s", stepName, errorMessage)
	if execOutput != "" {
		// Truncate output if too long
		if len(execOutput) > 2000 {
			execOutput = execOutput[:2000] + "... (truncated)"
		}
		output += fmt.Sprintf("\nOutput: %s", execOutput)
	}

	// Record with a special function name to distinguish pre-selffix failures
	failedFunctionName := fmt.Sprintf("__failed__%s", t.function.Name)

	err := t.executionTracker.RecordExecution(
		ctx,
		messageID,
		clientID,
		t.tool.Name,
		failedFunctionName,
		fmt.Sprintf("Failed execution of %s.%s (step: %s) - before selffix attempt", t.tool.Name, t.function.Name, stepName),
		inputs,
		"", // No input hash for failed records
		output,
		nil, // No original output
		time.Now(),
		tool_protocol.StatusFailed,
		nil, // No function definition for failed records
	)
	if err != nil {
		logger.Warnf("[SelfFix] Failed to record failed step execution: %v", err)
	} else {
		logger.Debugf("[SelfFix] Recorded failed step execution for %s.%s step '%s'", t.tool.Name, t.function.Name, stepName)
	}
}

// recordSelfFixExecution records a self-fix event to the function_executions table
func (t *YAMLDefinedTool) recordSelfFixExecution(
	ctx context.Context,
	messageID string,
	operationType string,
	stepName string,
	originalError string,
	fixResult *selfFixResult,
	inputs map[string]interface{},
) {
	if t.executionTracker == nil {
		return
	}

	// Get clientID from context
	var clientID string
	if retrievedMsg, ok := ctx.Value(MessageInContextKey).(Message); ok {
		clientID = retrievedMsg.ClientID
	}

	// Build the output message with self-fix details
	var output string
	if fixResult.Success {
		output = fmt.Sprintf("[SelfFix] Successfully fixed %s operation in step '%s'. "+
			"Attempts: %d. Recommended action: %s. Reason: %s. "+
			"Original error: %s",
			operationType, stepName,
			fixResult.Attempts, fixResult.RecommendedAction, fixResult.Reason,
			originalError)
	} else {
		output = fmt.Sprintf("[SelfFix] Failed to fix %s operation in step '%s'. "+
			"Attempts: %d. Error: %v. Original error: %s",
			operationType, stepName,
			fixResult.Attempts, fixResult.Error,
			originalError)
	}

	// Record with a special function name to distinguish self-fix events
	selfFixFunctionName := fmt.Sprintf("__selffix__%s", t.function.Name)

	err := t.executionTracker.RecordExecution(
		ctx,
		messageID,
		clientID,
		t.tool.Name,
		selfFixFunctionName,
		fmt.Sprintf("Self-fix attempt for %s.%s (%s operation)", t.tool.Name, t.function.Name, operationType),
		inputs,
		"", // No input hash for self-fix records
		output,
		nil, // No original output
		time.Now(),
		tool_protocol.StatusComplete,
		nil, // No function definition for self-fix records
	)
	if err != nil {
		logger.Warnf("[SelfFix] Failed to record self-fix execution: %v", err)
	}
}

// executeDBStep executes a single database step
func (t *YAMLDefinedTool) executeDBStep(ctx context.Context, db *sql.DB, step tool_protocol.Step, inputs map[string]interface{}, stepResults map[int]interface{}) (interface{}, error) {
	// Check for mock service in context (test mode only - zero-cost in production)
	if mockSvc, ok := ctx.Value(tool_engine_models.TestMockServiceKey).(tool_engine_models.IMockService); ok && mockSvc != nil {
		functionKey := GenerateFunctionKey(t.tool.Name, t.function.Name)
		if mockSvc.ShouldMock("db", functionKey, step.Name) {
			if response, exists := mockSvc.GetMockResponseValue(functionKey, step.Name); exists {
				mockSvc.RecordCall(functionKey, step.Name, "db", inputs, response, "")
				logger.Infof("[TEST MOCK] Returning mock for db %s.%s", functionKey, step.Name)
				return response, nil
			}
		}
	}

	// Get the SQL query from the step's with block
	if step.With == nil || step.With[step.Action] == nil {
		return nil, fmt.Errorf("step '%s' missing SQL query in 'with' block", step.Name)
	}

	sqlQuery, ok := step.With[step.Action].(string)
	if !ok {
		return nil, fmt.Errorf("step '%s' has invalid SQL query: must be a string", step.Name)
	}

	// Use the new parameterized query approach
	processedSQL, params, err := t.variableReplacer.ReplaceVariablesForDB(ctx, sqlQuery, inputs)
	if err != nil {
		return nil, fmt.Errorf("error processing SQL variables: %w", err)
	}

	// Handle result[n] references that might not be in inputs
	// These need to be replaced with actual values, not placeholders
	for resultIndex, result := range stepResults {
		placeholder := fmt.Sprintf("result[%d]", resultIndex)
		if strings.Contains(processedSQL, placeholder) {
			if resultStr, ok := result.(string); ok {
				// For simple string results, we can replace directly
				// But we need to add it as a parameter
				processedSQL = strings.Replace(processedSQL, placeholder, "?", 1)
				params = append(params, resultStr)
			} else if resultMap, ok := result.(map[string]interface{}); ok {
				// For complex results, convert to JSON
				if jsonBytes, err := json.Marshal(resultMap); err == nil {
					processedSQL = strings.Replace(processedSQL, placeholder, "?", 1)
					params = append(params, string(jsonBytes))
				}
			}
		}
	}

	switch step.Action {
	case tool_protocol.StepActionSelect:
		// Execute SELECT query with parameters
		rows, err := db.QueryContext(ctx, processedSQL, params...)
		if err != nil {
			return nil, fmt.Errorf("error executing SELECT: %w", err)
		}
		defer rows.Close()

		// Get column names
		columns, err := rows.Columns()
		if err != nil {
			return nil, fmt.Errorf("error getting columns: %w", err)
		}

		// Prepare result slice
		var results []map[string]interface{}

		// Create a slice of interface{} to hold column values
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		// Iterate through rows
		for rows.Next() {
			err := rows.Scan(valuePtrs...)
			if err != nil {
				return nil, fmt.Errorf("error scanning row: %w", err)
			}

			// Create a map for this row
			rowMap := make(map[string]interface{})
			for i, col := range columns {
				// Convert byte arrays to strings
				if b, ok := values[i].([]byte); ok {
					rowMap[col] = string(b)
				} else {
					rowMap[col] = values[i]
				}
			}

			results = append(results, rowMap)
		}

		if err = rows.Err(); err != nil {
			return nil, fmt.Errorf("error iterating rows: %w", err)
		}

		return results, nil

	case tool_protocol.StepActionWrite:
		// Execute INSERT/UPDATE/DELETE query with parameters
		result, err := db.ExecContext(ctx, processedSQL, params...)
		if err != nil {
			return nil, fmt.Errorf("error executing write operation: %w", err)
		}

		// Get affected rows
		rowsAffected, err := result.RowsAffected()
		if err != nil {
			return fmt.Sprintf("Write operation completed"), nil
		}

		return fmt.Sprintf("Write operation completed. Rows affected: %d", rowsAffected), nil

	default:
		return nil, fmt.Errorf("unknown database action: %s", step.Action)
	}
}

// executeForEachDBStep handles forEach iteration for db steps.
// Uses NavigatePathWithTransformation + resolveForEachItems (same as API/terminal)
// so it supports structured arrays, nested paths, and string splitting.
func (t *YAMLDefinedTool) executeForEachDBStep(ctx context.Context, db *sql.DB, step tool_protocol.Step, inputs map[string]interface{}, stepResults map[int]interface{}) (interface{}, error) {
	forEach := step.ForEach

	// Resolve forEach config with defaults
	indexVar := forEach.IndexVar
	if indexVar == "" {
		indexVar = tool_protocol.DefaultForEachIndexVar
	}
	itemVar := forEach.ItemVar
	if itemVar == "" {
		itemVar = tool_protocol.DefaultForEachItemVar
	}

	// Resolve the items using NavigatePathWithTransformation (supports nested paths, transformations, arrays)
	itemsPath := strings.TrimPrefix(forEach.Items, "$")
	rawItemsValue, err := t.variableReplacer.NavigatePathWithTransformation(inputs, itemsPath)
	if err != nil || rawItemsValue == nil {
		logger.Errorf("variable '%s' not found in inputs: %v", forEach.Items, err)
		return "[]", nil
	}

	// Type-switch: resolves []interface{}, []map[string]interface{}, []string, or comma-separated string
	items, err := t.resolveForEachItems(rawItemsValue, forEach.Separator)
	if err != nil {
		return nil, fmt.Errorf("error resolving forEach items for db step '%s': %w", step.Name, err)
	}

	results := make([]interface{}, 0, len(items))

	for idx, item := range items {
		// Skip empty string items (matches other forEach implementations)
		if strItem, ok := item.(string); ok && strings.TrimSpace(strItem) == "" {
			continue
		}

		// Build per-iteration inputs with loop variables
		loopInputs := make(map[string]interface{})
		for k, v := range inputs {
			loopInputs[k] = v
		}
		loopInputs[indexVar] = idx
		loopInputs[itemVar] = item

		result, err := t.executeDBStep(ctx, db, step, loopInputs, stepResults)
		if err != nil {
			return nil, fmt.Errorf("error in forEach iteration %d for db step '%s': %w", idx, step.Name, err)
		}
		results = append(results, result)
	}

	// Marshal aggregated results
	resultJSON, err := json.Marshal(results)
	if err != nil {
		return nil, fmt.Errorf("error marshaling forEach results: %w", err)
	}
	return string(resultJSON), nil
}

// executeSteps executes a sequence of steps
func (t *YAMLDefinedTool) executeSteps(
	ctx context.Context,
	messageID string,
	steps []tool_protocol.Step,
	inputs map[string]interface{},
	stepResults map[int]interface{},
) error {
	if t.function.Operation == "web_browse" {
		guide, err := t.generateWebBrowsingGuide(ctx, messageID, steps, inputs)
		if err != nil {
			return fmt.Errorf("error generating web browsing guide: %w", err)
		}

		// Check if we need to use the user's browser
		useUserBrowser := false
		for _, step := range steps {
			if step.Action == "open_url" && step.With != nil {
				if incognitoMode, ok := step.With[tool_protocol.StepWithIncognitoMode]; ok {
					if incognito, ok := incognitoMode.(bool); ok && !incognito {
						useUserBrowser = true
						break
					}
				}
			}
		}

		formattedOutput, err := t.outputFormatter.OutputToJSON(t.function.Output)
		if err != nil {
			return fmt.Errorf("error formatting output: %w", err)
		}

		result, err := GlobalBrowserManager.ExecuteBrowserOperation(
			ctx,
			messageID,
			useUserBrowser,
			guide,
			t.function.SuccessCriteria,
			inputs,
			formattedOutput,
		)

		if err != nil {
			return fmt.Errorf("error executing web browsing guide: %w", err)
		}

		stepResults[0] = result

		return nil
	}

	for _, step := range steps {
		// Check runOnlyIf before executing the step (for api_call, terminal, db operations)
		if step.RunOnlyIf != nil {
			shouldExecute, skipInfo, err := t.evaluateStepRunOnlyIf(ctx, messageID, step, inputs, stepResults)
			if err != nil {
				return fmt.Errorf("error evaluating runOnlyIf for step '%s': %w", step.Name, err)
			}
			if !shouldExecute {
				t.skippedSteps = append(t.skippedSteps, *skipInfo)
				logger.Infof("Step '%s' skipped due to runOnlyIf: %s", step.Name, skipInfo.Reason)
				continue
			}
		}

		if step.ForEach != nil {
			err := t.executeForEachStep(ctx, messageID, step, inputs, stepResults)
			if err != nil {
				return fmt.Errorf("error executing foreach step '%s': %w", step.Name, err)
			}
		} else {
			// Replace variables in the step parameters
			// Pass stepResults so result[X] references resolve to this function's step results first
			processedParams, err := t.processStepParameters(ctx, messageID, step.With, inputs, stepResults)
			if err != nil {
				return fmt.Errorf("error processing step parameters for step '%s': %w", step.Name, err)
			}

			result, err := t.executeStep(ctx, messageID, step, processedParams)
			if err != nil {
				// TODO: implement error handling based on step.OnError
				//if step.OnError != nil {
				//	// Handle error based on onError strategy
				//	handledErr := t.handleStepError(ctx, messageID, step, err)
				//	if handledErr != nil {
				//		return fmt.Errorf("error handling step error: %w", handledErr)
				//	}
				//	continue
				//}
				return fmt.Errorf("error executing step '%s': %w", step.Name, err)
			}

			// Store the result if the step has a resultIndex
			if step.ResultIndex != 0 && result != nil {
				stepResults[step.ResultIndex] = result
			}
		}
	}

	return nil
}

// evaluateStepRunOnlyIf evaluates the runOnlyIf condition for a step
// Returns (shouldExecute, skipInfo, error)
func (t *YAMLDefinedTool) evaluateStepRunOnlyIf(
	ctx context.Context,
	messageID string,
	step tool_protocol.Step,
	inputs map[string]interface{},
	stepResults map[int]interface{},
) (bool, *SkippedStepInfo, error) {

	// Parse the runOnlyIf
	runOnlyIfObj, err := tool_protocol.ParseRunOnlyIf(step.RunOnlyIf)
	if err != nil {
		return false, nil, fmt.Errorf("failed to parse runOnlyIf: %w", err)
	}

	if runOnlyIfObj == nil || runOnlyIfObj.Deterministic == "" {
		// No runOnlyIf or empty deterministic - execute the step
		return true, nil, nil
	}

	originalExpression := runOnlyIfObj.Deterministic

	// Build variables map for replacement
	// Start with function inputs
	variables := make(map[string]interface{})
	for key, value := range inputs {
		variables[key] = value
	}

	// Replace result[X] references from local stepResults BEFORE calling ReplaceVariables
	// This is necessary because ReplaceVariables fetches from the database, but step results
	// from the current function execution are not yet in the database - they're only in stepResults
	expression := t.replaceResultReferencesFromStepResults(originalExpression, stepResults)

	// Replace variables in the expression using the full variable replacer
	// This handles: input variables ($var), system vars ($USER), env vars ($API_KEY)
	// Note: result[X] references should already be replaced from stepResults above
	replacedExpr, err := t.variableReplacer.ReplaceVariables(ctx, expression, variables)
	if err != nil {
		logger.Warnf("Step '%s' runOnlyIf variable replacement failed: %v - treating as false (skip step)", step.Name, err)
		skipInfo := &SkippedStepInfo{
			Name:       step.Name,
			Reason:     "runOnlyIf variable replacement failed",
			Expression: originalExpression,
			EvalResult: fmt.Sprintf("error: %v", err),
		}
		return false, skipInfo, nil
	}

	// Evaluate the deterministic expression
	result, err := tool_protocol.EvaluateDeterministicExpression(replacedExpr)
	if err != nil {
		logger.Warnf("Step '%s' runOnlyIf evaluation failed: %v - treating as false (skip step)", step.Name, err)
		skipInfo := &SkippedStepInfo{
			Name:       step.Name,
			Reason:     "runOnlyIf evaluation failed",
			Expression: originalExpression,
			EvalResult: fmt.Sprintf("%s -> error: %v", replacedExpr, err),
		}
		return false, skipInfo, nil
	}

	if !result {
		skipInfo := &SkippedStepInfo{
			Name:       step.Name,
			Reason:     "runOnlyIf evaluated to false",
			Expression: originalExpression,
			EvalResult: fmt.Sprintf("%s -> false", replacedExpr),
		}
		return false, skipInfo, nil
	}

	logger.Debugf("Step '%s' runOnlyIf evaluated to true: %s -> %s", step.Name, originalExpression, replacedExpr)
	return true, nil, nil
}

// replaceResultReferencesFromStepResults replaces result[X] patterns with values from the local stepResults map
// This is used for step-level runOnlyIf where previous step results from the same function
// are not yet in the database but are available in the stepResults map
func (t *YAMLDefinedTool) replaceResultReferencesFromStepResults(expression string, stepResults map[int]interface{}) string {
	if stepResults == nil || len(stepResults) == 0 {
		return expression
	}

	return stepResultReferenceRegex.ReplaceAllStringFunc(expression, func(match string) string {
		// Extract result index and path
		submatches := stepResultReferenceRegex.FindStringSubmatch(match)
		if len(submatches) < 2 {
			return match
		}

		indexStr := submatches[1]
		index, err := strconv.Atoi(indexStr)
		if err != nil {
			return match
		}

		result, ok := stepResults[index]
		if !ok {
			// Step result not available - leave unchanged for ReplaceVariables to try database
			return match
		}

		// If result is a JSON string, parse it for path navigation
		// API call steps return raw JSON strings that need to be parsed
		parsedResult := result
		if strResult, isString := result.(string); isString {
			trimmed := strings.TrimSpace(strResult)
			// Check if it looks like a JSON object or array
			if (strings.HasPrefix(trimmed, "{") && strings.HasSuffix(trimmed, "}")) ||
				(strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]")) {
				// Try to parse as JSON object
				var jsonObj map[string]interface{}
				if err := json.Unmarshal([]byte(strResult), &jsonObj); err == nil {
					parsedResult = jsonObj
				} else {
					// Try to parse as JSON array
					var jsonArr []interface{}
					if err := json.Unmarshal([]byte(strResult), &jsonArr); err == nil {
						parsedResult = jsonArr
					}
					// If both fail, keep as string
				}
			}
		}

		// If there's a path (e.g., result[1].field or result[1][0].field), navigate to that value
		if len(submatches) > 2 && submatches[2] != "" {
			path := submatches[2]
			// Remove leading dot if present
			if strings.HasPrefix(path, ".") {
				path = path[1:]
			}
			// Navigate to the nested value
			value, err := t.navigateToPathForRunOnlyIf(parsedResult, path)
			if err != nil {
				return match // Leave unchanged if navigation fails
			}
			return t.valueToStringForRunOnlyIf(value)
		}

		return t.valueToStringForRunOnlyIf(parsedResult)
	})
}

// navigateToPathForRunOnlyIf navigates to a nested value using a path like "field.subfield" or "field[0].subfield"
func (t *YAMLDefinedTool) navigateToPathForRunOnlyIf(data interface{}, path string) (interface{}, error) {
	if path == "" {
		return data, nil
	}

	current := data
	// Split by dot but handle array indices
	segments := strings.Split(path, ".")

	for _, segment := range segments {
		if current == nil {
			return nil, nil
		}

		// Check for array index in segment like "field[0]"
		if bracketIdx := strings.Index(segment, "["); bracketIdx >= 0 {
			fieldName := segment[:bracketIdx]
			indexPart := segment[bracketIdx:]

			// First navigate to the field if there is one
			if fieldName != "" {
				obj, ok := current.(map[string]interface{})
				if !ok {
					return nil, fmt.Errorf("cannot access field %s in non-object", fieldName)
				}
				current = obj[fieldName]
			}

			// Then handle array index
			indexMatch := arrayIndexRegex.FindStringSubmatch(indexPart)
			if len(indexMatch) < 2 {
				return nil, fmt.Errorf("invalid array index: %s", indexPart)
			}
			arrIdx, _ := strconv.Atoi(indexMatch[1])

			arr, ok := current.([]interface{})
			if !ok {
				return nil, fmt.Errorf("cannot access index %d in non-array", arrIdx)
			}
			if arrIdx >= len(arr) {
				return nil, fmt.Errorf("array index %d out of bounds", arrIdx)
			}
			current = arr[arrIdx]
		} else {
			// Simple field access
			obj, ok := current.(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("cannot access field %s in non-object", segment)
			}
			current = obj[segment]
		}
	}

	return current, nil
}

// valueToStringForRunOnlyIf converts a value to its string representation for use in runOnlyIf expressions
func (t *YAMLDefinedTool) valueToStringForRunOnlyIf(value interface{}) string {
	if value == nil {
		return "null"
	}

	switch v := value.(type) {
	case string:
		return fmt.Sprintf("'%s'", v)
	case bool:
		if v {
			return "true"
		}
		return "false"
	case float64:
		// Check if it's an integer
		if v == float64(int64(v)) {
			return fmt.Sprintf("%d", int64(v))
		}
		return fmt.Sprintf("%g", v)
	case int, int32, int64:
		return fmt.Sprintf("%d", v)
	case map[string]interface{}, []interface{}:
		// For complex types, return JSON representation
		jsonBytes, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprintf("%v", v)
		}
		return string(jsonBytes)
	default:
		return fmt.Sprintf("%v", v)
	}
}

// executeForEachStep executes a step with foreach iteration
func (t *YAMLDefinedTool) executeForEachStep(
	ctx context.Context,
	messageID string,
	step tool_protocol.Step,
	inputs map[string]interface{},
	stepResults map[int]interface{},
) error {
	// Get the variable path from the items field (e.g., "$conversations.conversations" -> "conversations.conversations")
	itemsPath := strings.TrimPrefix(step.ForEach.Items, "$")

	// Use NavigatePathWithTransformation to resolve nested paths and apply date transformations
	// This supports syntax like "$datesToQuery.toISO()" in foreach items
	rawItemsValue, err := t.variableReplacer.NavigatePathWithTransformation(inputs, itemsPath)
	if err != nil || rawItemsValue == nil {
		logger.Errorf("variable '%s' not found in inputs: %v", step.ForEach.Items, err)
		return nil
	}

	// Handle different types of items
	var items []interface{}
	switch v := rawItemsValue.(type) {
	case string:
		// Split string by separator
		separator := step.ForEach.Separator
		if separator == "" {
			separator = tool_protocol.DefaultForEachSeparator
		}
		parts := strings.Split(v, separator)
		items = make([]interface{}, len(parts))
		for i, part := range parts {
			items[i] = strings.TrimSpace(part)
		}
	case []interface{}:
		items = v
	case []string:
		items = make([]interface{}, len(v))
		for i, item := range v {
			items[i] = item
		}
	case map[string]interface{}:
		// Single object — wrap in array so foreach iterates once
		items = []interface{}{v}
	default:
		return fmt.Errorf("foreach items must be string or array, got %T", v)
	}

	// Set default variable names if not provided
	indexVar := step.ForEach.IndexVar
	if indexVar == "" {
		indexVar = tool_protocol.DefaultForEachIndexVar
	}
	itemVar := step.ForEach.ItemVar
	if itemVar == "" {
		itemVar = tool_protocol.DefaultForEachItemVar
	}

	// Execute step for each item
	var results []interface{}
	shouldSkipConditions := step.ForEach.GetShouldSkipConditions()
	for i, item := range items {
		// DEBUG: Log the item type and value to find where truncation happens
		logger.Debugf("[ExecuteForEach] Iteration %d: item type=%T, value=%+v", i, item, item)
		if strItem, ok := item.(string); ok {
			logger.Debugf("[ExecuteForEach] Item is string, first 50 chars: %q", strItem[:min(len(strItem), 50)])
		}
		// Check shouldSkip conditions at the start of each iteration
		// Uses OR logic: if ANY condition returns true, skip the iteration
		if len(shouldSkipConditions) > 0 {
			skip, err := t.shouldSkipIterationMultiple(ctx, messageID, shouldSkipConditions, i, item, indexVar, itemVar, inputs)
			if err != nil {
				// Error already logged in shouldSkipIterationMultiple, skip this iteration
				continue
			}
			if skip {
				continue
			}
		}

		loopContext := &tool_engine_models.LoopContext{
			IndexVar: indexVar,
			ItemVar:  itemVar,
			Index:    i,
			Item:     item,
		}

		// Process step parameters with loop context
		// Pass stepResults so result[X] references resolve to this function's step results first
		processedParams, err := t.processStepParametersWithLoop(ctx, messageID, step.With, inputs, loopContext, stepResults)
		if err != nil {
			return fmt.Errorf("error processing step parameters for iteration %d: %w", i, err)
		}

		result, err := t.executeStep(ctx, messageID, step, processedParams)
		if err != nil {
			return fmt.Errorf("error executing step for iteration %d: %w", i, err)
		}

		results = append(results, result)
	}

	// Store the results
	if step.ResultIndex != 0 {
		stepResults[step.ResultIndex] = results
	}

	return nil
}

func (t *YAMLDefinedTool) executeForEachApiStep(
	ctx context.Context,
	messageID string,
	step tool_protocol.Step,
	inputs map[string]interface{},
	stepResults map[int]interface{},
	sharedHTTPClient HTTPClient,
	cookieRepo cookieRepository,
	hasAuthStep bool,
	authRetryCount *int,
	maxAuthRetries int,
	authStepIndex int,
	cookiesLoaded *bool,
	executionDetails *APIExecutionDetails,
) error {
	// Get the variable path from the items field (e.g., "$conversations.conversations" -> "conversations.conversations")
	itemsPath := strings.TrimPrefix(step.ForEach.Items, "$")

	// Use NavigatePathWithTransformation to resolve nested paths and apply date transformations
	// This supports syntax like "$datesToQuery.toISO()" in foreach items
	rawItemsValue, err := t.variableReplacer.NavigatePathWithTransformation(inputs, itemsPath)
	if err != nil || rawItemsValue == nil {
		logger.Errorf("variable '%s' not found in inputs: %v", step.ForEach.Items, err)
		return nil
	}

	var items []interface{}
	switch v := rawItemsValue.(type) {
	case string:
		separator := step.ForEach.Separator
		if separator == "" {
			separator = tool_protocol.DefaultForEachSeparator
		}
		parts := strings.Split(v, separator)
		items = make([]interface{}, len(parts))
		for i, part := range parts {
			items[i] = strings.TrimSpace(part)
		}
	case []interface{}:
		items = v
	case []string:
		items = make([]interface{}, len(v))
		for i, item := range v {
			items[i] = item
		}
	case map[string]interface{}:
		// Single object — wrap in array so foreach iterates once
		items = []interface{}{v}
	default:
		return fmt.Errorf("foreach items must be string or array, got %T", v)
	}

	indexVar := step.ForEach.IndexVar
	if indexVar == "" {
		indexVar = tool_protocol.DefaultForEachIndexVar
	}
	itemVar := step.ForEach.ItemVar
	if itemVar == "" {
		itemVar = tool_protocol.DefaultForEachItemVar
	}

	// If breakIf, waitFor, or shouldSkip is present, execute sequentially; otherwise, execute in parallel
	if step.ForEach.BreakIf != nil || step.ForEach.WaitFor != nil || step.ForEach.HasShouldSkip() {
		return t.executeForEachSequentialWithBreak(ctx, messageID, step, items, indexVar, itemVar, inputs, stepResults, sharedHTTPClient, cookieRepo, hasAuthStep, authRetryCount, maxAuthRetries, authStepIndex, cookiesLoaded, executionDetails)
	}

	results := make([]interface{}, len(items))

	// Check for existing partial results from a previous retry attempt
	// This allows resuming forEach execution from where it left off
	var existingResultsCount int
	if step.ResultIndex != 0 {
		if existingResults, ok := stepResults[step.ResultIndex].([]interface{}); ok && len(existingResults) > 0 {
			existingResultsCount = len(existingResults)
			// Copy existing results to the results array
			for i, r := range existingResults {
				if i < len(results) {
					results[i] = r
				}
			}
			logger.Infof("Resuming forEach with %d existing results from previous attempt", existingResultsCount)
		}
	}

	type workItem struct {
		index int
		item  interface{}
	}

	type workResult struct {
		index   int
		result  interface{}
		details []StepExecutionDetail
		err     error
	}

	// Calculate how many items need to be processed (skip items with existing results)
	itemsToProcess := len(items) - existingResultsCount
	if itemsToProcess <= 0 {
		// All items already have results, nothing to do
		logger.Infof("All %d forEach items already completed in previous attempt, skipping execution", len(items))
		return nil
	}

	workChan := make(chan workItem, itemsToProcess)
	resultChan := make(chan workResult, itemsToProcess)

	for i, item := range items {
		// Skip items that already have results from previous attempt
		if i < existingResultsCount && results[i] != nil {
			logger.Debugf("Skipping forEach item %d - already completed in previous attempt", i)
			continue
		}
		workChan <- workItem{index: i, item: item}
	}
	close(workChan)

	numWorkers := 2
	if len(items) < 2 {
		numWorkers = len(items)
	}

	for w := 0; w < numWorkers; w++ {
		go func(workerID int) {
			defer func() {
				if r := recover(); r != nil {
					logger.Errorf("Worker %d panicked during foreach execution: %v\nStack trace:\n%s", workerID, r, string(debug.Stack()))
					for work := range workChan {
						resultChan <- workResult{
							index:   work.index,
							result:  nil,
							details: nil,
							err:     fmt.Errorf("worker %d panicked during execution: %v", workerID, r),
						}
					}
				}
			}()

			for work := range workChan {
				result, iterationDetails, err := t.executeForEachItemWithRetry(ctx, messageID, step, work.index, work.item, indexVar, itemVar, inputs, sharedHTTPClient, cookieRepo, hasAuthStep, authRetryCount, maxAuthRetries, authStepIndex, cookiesLoaded)
				resultChan <- workResult{index: work.index, result: result, details: iterationDetails, err: err}

				if len(workChan) > 0 {
					select {
					case <-ctx.Done():
						return
					case <-time.After(100 * time.Millisecond):
					}
				}
			}
		}(w)
	}

	var firstError error
	// Only wait for results from items we actually processed (not skipped ones)
	for i := 0; i < itemsToProcess; i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case res := <-resultChan:
			if res.err != nil && firstError == nil {
				firstError = res.err
			}
			results[res.index] = res.result
			// Aggregate execution details from all iterations
			if executionDetails != nil && len(res.details) > 0 {
				executionDetails.Steps = append(executionDetails.Steps, res.details...)
			}
		}
	}

	if firstError != nil {
		return firstError
	}

	if step.ResultIndex != 0 {
		stepResults[step.ResultIndex] = results
	}

	return nil
}

func (t *YAMLDefinedTool) executeForEachSequentialWithBreak(
	ctx context.Context,
	messageID string,
	step tool_protocol.Step,
	items []interface{},
	indexVar, itemVar string,
	inputs map[string]interface{},
	stepResults map[int]interface{},
	sharedHTTPClient HTTPClient,
	cookieRepo cookieRepository,
	hasAuthStep bool,
	authRetryCount *int,
	maxAuthRetries int,
	authStepIndex int,
	cookiesLoaded *bool,
	executionDetails *APIExecutionDetails,
) error {

	results := make([]interface{}, 0, len(items))

	// Check for existing partial results from a previous retry attempt
	// This allows resuming forEach execution from where it left off
	startIndex := 0
	if step.ResultIndex != 0 {
		if existingResults, ok := stepResults[step.ResultIndex].([]interface{}); ok && len(existingResults) > 0 {
			startIndex = len(existingResults)
			results = existingResults // Continue with existing results
			logger.Infof("Resuming sequential forEach from item %d (previous %d items succeeded)", startIndex, len(existingResults))

			// If all items already completed, skip execution
			if startIndex >= len(items) {
				logger.Infof("All %d forEach items already completed in previous attempt, skipping execution", len(items))
				return nil
			}
		}
	}

	// Parse breakIf condition if present
	var strCond string
	var objCond *tool_protocol.SuccessCriteriaObject
	if step.ForEach.BreakIf != nil {
		logger.Debugf("Parsing breakIf condition from forEach configuration")
		var err error
		strCond, objCond, err = tool_protocol.ParseBreakIf(step.ForEach.BreakIf)
		if err != nil {
			logger.Errorf("Failed to parse breakIf condition: %v", err)
			return fmt.Errorf("error parsing breakIf condition: %w", err)
		}

		if strCond != "" {
			logger.Infof("Executing forEach with DETERMINISTIC breakIf condition sequentially for %d items: '%s'", len(items), strCond)
		} else if objCond != nil {
			logger.Infof("Executing forEach with INFERENCE breakIf condition sequentially for %d items: '%s'", len(items), objCond.Condition)
			if len(objCond.From) > 0 {
				logger.Debugf("BreakIf inference will use context from functions: %v", objCond.From)
			}
			if len(objCond.AllowedSystemFunctions) > 0 {
				logger.Debugf("BreakIf inference restricted to system functions: %v", objCond.AllowedSystemFunctions)
			}
		}
	}

	// Log waitFor configuration if present
	if step.ForEach.WaitFor != nil {
		logger.Infof("Executing forEach with waitFor function '%s' (pollInterval=%ds, maxWait=%ds)",
			step.ForEach.WaitFor.Name, step.ForEach.WaitFor.PollIntervalSeconds, step.ForEach.WaitFor.MaxWaitingSeconds)
	}

	// Log shouldSkip configuration if present (supports multiple conditions)
	shouldSkipConditions := step.ForEach.GetShouldSkipConditions()
	if len(shouldSkipConditions) > 0 {
		names := make([]string, len(shouldSkipConditions))
		for i, cond := range shouldSkipConditions {
			names[i] = cond.Name
		}
		logger.Infof("Executing forEach with %d shouldSkip condition(s): %v", len(shouldSkipConditions), names)
	}

	// Start from startIndex to resume from where we left off (for partial retry support)
	for i := startIndex; i < len(items); i++ {
		item := items[i]
		select {
		case <-ctx.Done():
			logger.Warnf("Context cancelled during forEach execution at index %d/%d", i, len(items))
			return ctx.Err()
		default:
		}

		logger.Debugf("Processing forEach item %d/%d", i+1, len(items))

		// Check shouldSkip conditions at the START of each iteration (before executing)
		// Uses OR logic: if ANY condition returns true, skip the iteration
		if len(shouldSkipConditions) > 0 {
			skip, err := t.shouldSkipIterationMultiple(ctx, messageID, shouldSkipConditions, i, item, indexVar, itemVar, inputs)
			if err != nil {
				// Error already logged in shouldSkipIterationMultiple, skip this iteration
				logger.Debugf("Skipping item %d/%d due to shouldSkip error", i+1, len(items))
				continue
			}
			if skip {
				logger.Debugf("Skipping item %d/%d (shouldSkip returned true)", i+1, len(items))
				continue
			}
		}

		// Execute current item
		result, iterationDetails, err := t.executeForEachItemWithRetry(ctx, messageID, step, i, item, indexVar, itemVar, inputs, sharedHTTPClient, cookieRepo, hasAuthStep, authRetryCount, maxAuthRetries, authStepIndex, cookiesLoaded)

		// Aggregate execution details from this iteration
		if executionDetails != nil && len(iterationDetails) > 0 {
			executionDetails.Steps = append(executionDetails.Steps, iterationDetails...)
		}

		if err != nil {
			logger.Errorf("Error executing forEach item at index %d: %v", i, err)
			return err
		}

		logger.Debugf("Successfully executed forEach item %d/%d", i+1, len(items))
		results = append(results, result)

		// Update stepResults with partial results after each iteration
		// This allows waitFor/breakIf to access result[N][$index].field syntax
		if step.ResultIndex != 0 {
			stepResults[step.ResultIndex] = results
		}

		// Evaluate break condition FIRST (if present)
		if step.ForEach.BreakIf != nil {
			logger.Debugf("Evaluating breakIf condition for item %d/%d", i+1, len(items))
			shouldBreak, err := t.evaluateBreakCondition(ctx, messageID, strCond, objCond, i, item, result, indexVar, itemVar, inputs, stepResults)
			if err != nil {
				logger.Warnf("Error evaluating breakIf condition at index %d: %v. Continuing execution.", i, err)
			} else if shouldBreak {
				logger.Infof("✓ Break condition MET at index %d/%d. Stopping forEach execution. Collected %d results.", i+1, len(items), len(results))
				break
			} else {
				logger.Debugf("✗ Break condition NOT met at index %d/%d. Continuing to next item.", i+1, len(items))
			}
		}

		// Apply waitFor after each item (if present)
		// This allows recording/tracking results for ALL items including the last one
		if step.ForEach.WaitFor != nil {
			logger.Debugf("Applying waitFor callback after item %d/%d", i+1, len(items))
			err := t.waitForCondition(ctx, messageID, step.ForEach.WaitFor, i, item, result, indexVar, itemVar, inputs, stepResults)
			if err != nil {
				logger.Errorf("waitFor callback failed at index %d: %v", i, err)
				return err
			}
			logger.Debugf("waitFor callback completed for item %d/%d", i+1, len(items))
		}
	}

	logger.Infof("ForEach execution completed. Total items processed: %d/%d, Results collected: %d", len(results), len(items), len(results))

	if step.ResultIndex != 0 {
		stepResults[step.ResultIndex] = results
		logger.Debugf("Stored forEach results in stepResults[%d]", step.ResultIndex)
	}

	return nil
}

// evaluateBreakCondition evaluates the break condition (deterministic or inference)
func (t *YAMLDefinedTool) evaluateBreakCondition(
	ctx context.Context,
	messageID string,
	deterministicCond string,
	inferenceCond *tool_protocol.SuccessCriteriaObject,
	index int,
	item interface{},
	result interface{},
	indexVar, itemVar string,
	inputs map[string]interface{},
	stepResults map[int]interface{},
) (bool, error) {

	// Create loop-scoped inputs for variable replacement
	loopInputs := make(map[string]interface{})
	for k, v := range inputs {
		loopInputs[k] = v
	}
	loopInputs[indexVar] = index
	loopInputs[itemVar] = item

	logger.Debugf("BreakIf evaluation context - index=%d, indexVar=%s, itemVar=%s", index, indexVar, itemVar)

	if deterministicCond != "" {
		// Deterministic condition - evaluate using simple comparison
		logger.Debugf("Evaluating DETERMINISTIC breakIf condition: '%s'", deterministicCond)
		return t.evaluateDeterministicBreakCondition(ctx, deterministicCond, loopInputs, indexVar, itemVar, stepResults)
	} else if inferenceCond != nil {
		// Inference condition - use agentic evaluation
		logger.Infof("Evaluating INFERENCE breakIf condition: '%s'", inferenceCond.Condition)

		// First, replace result[N] references from local stepResults
		// This allows inference breakIf to access result[N][$index].field syntax
		localReplaced := t.replaceResultReferencesFromStepResults(inferenceCond.Condition, stepResults)

		// Replace variables in the condition
		logger.Debugf("Replacing variables in breakIf condition with loop context")
		condition, err := t.variableReplacer.ReplaceVariablesWithOptions(ctx, localReplaced, loopInputs, tool_engine_models.ReplaceOptions{
			KeepUnresolvedPlaceholders: true, // Keep $var for LLM readability
		})
		if err != nil {
			logger.Errorf("Failed to replace variables in breakIf condition '%s': %v", inferenceCond.Condition, err)
			return false, fmt.Errorf("error replacing variables in condition: %w", err)
		}
		logger.Debugf("BreakIf condition after variable replacement: '%s'", condition)

		// Prepare context for condition evaluation
		evalCtx := ctx
		if len(inferenceCond.From) > 0 {
			// Filter context to only include functions specified in From
			logger.Debugf("Filtering context for breakIf evaluation to functions: %v", inferenceCond.From)
			evalCtx = t.filterContextForFunctions(ctx, inferenceCond.From)
		}

		// Prepare success criteria for inference - request simple formatted string response
		var successCriteria interface{}
		successCriteriaCondition := `Evaluate if the break condition is met based on the description.
Return your response in this EXACT format:
done: {true or false} || {clear explanation}

Examples:
- done: true || Found the preferred dentist in the current item.
- done: false || This item doesn't match the break condition yet.

The explanation should be clear and explain your decision. Output ONLY in the format shown above.`

		if len(inferenceCond.AllowedSystemFunctions) > 0 {
			// Use object format to include allowedSystemFunctions
			logger.Debugf("Restricting breakIf agentic inference to system functions: %v", inferenceCond.AllowedSystemFunctions)
			successCriteria = &tool_protocol.SuccessCriteriaObject{
				Condition:              successCriteriaCondition,
				AllowedSystemFunctions: inferenceCond.AllowedSystemFunctions,
			}
		} else {
			// Use simple string format
			logger.Debugf("Using default system functions for breakIf agentic inference")
			successCriteria = successCriteriaCondition
		}

		inference := tool_protocol.Input{
			Name:            "should_break_foreach_loop",
			Description:     condition,
			SuccessCriteria: successCriteria,
		}
		logger.Debugf("Invoking agentic inference for breakIf evaluation")

		// Get clientID from context
		retrievedMsg, ok := ctx.Value(MessageInContextKey).(Message)
		if !ok {
			logger.Warnf("Failed to get message from context for breakIf inference")
			return false, fmt.Errorf("failed to get message from context")
		}
		clientID := retrievedMsg.ClientID

		rawResponse, err := t.inputFulfiller.AgenticInfer(evalCtx, messageID, clientID, inference, t.tool)
		if err != nil {
			logger.Errorf("Agentic inference failed for breakIf evaluation: %v", err)
			return false, fmt.Errorf("error evaluating inference condition: %w", err)
		}
		logger.Debugf("Agentic inference raw result for breakIf: '%s'", rawResponse)

		// Parse the response using the same parser as runOnlyIf
		shouldBreak, explanationMessage := parseRunOnlyIfResponse(rawResponse)
		logger.Infof("Parsed breakIf response - shouldBreak: %t, explanation: '%s'", shouldBreak, explanationMessage)

		return shouldBreak, nil
	}

	logger.Warnf("No breakIf condition found (neither deterministic nor inference)")
	return false, nil
}

// evaluateDeterministicBreakCondition evaluates simple comparison conditions like "$item.status == 'completed'"
func (t *YAMLDefinedTool) evaluateDeterministicBreakCondition(
	ctx context.Context,
	condition string,
	loopInputs map[string]interface{},
	indexVar, itemVar string,
	stepResults map[int]interface{},
) (bool, error) {

	logger.Debugf("Starting deterministic breakIf evaluation with loop inputs: indexVar=%s, itemVar=%s", indexVar, itemVar)

	// First, replace result[N] references from local stepResults
	// This allows breakIf to access result[N][$index].field syntax
	localReplaced := t.replaceResultReferencesFromStepResults(condition, stepResults)

	// Replace variables in the condition
	replacer := NewVariableReplacer(nil)
	processedCondition, err := replacer.ReplaceVariables(ctx, localReplaced, loopInputs)
	if err != nil {
		logger.Errorf("Failed to replace variables in deterministic breakIf condition: %v", err)
		return false, fmt.Errorf("error replacing variables: %w", err)
	}

	logger.Infof("Deterministic breakIf evaluation - original: '%s', processed: '%s'", condition, processedCondition)

	// Parse and evaluate the condition
	// Support: ==, !=, >, <, >=, <=
	operators := []struct {
		op   string
		eval func(left, right string) bool
	}{
		{"==", func(l, r string) bool {
			result := strings.TrimSpace(l) == strings.TrimSpace(r)
			logger.Debugf("Comparing '%s' == '%s': %t", l, r, result)
			return result
		}},
		{"!=", func(l, r string) bool {
			result := strings.TrimSpace(l) != strings.TrimSpace(r)
			logger.Debugf("Comparing '%s' != '%s': %t", l, r, result)
			return result
		}},
		{">=", func(l, r string) bool {
			lVal, lErr := strconv.ParseFloat(strings.TrimSpace(l), 64)
			rVal, rErr := strconv.ParseFloat(strings.TrimSpace(r), 64)
			if lErr != nil || rErr != nil {
				logger.Debugf("Failed to parse numbers for >= comparison: left='%s' (err: %v), right='%s' (err: %v)", l, lErr, r, rErr)
				return false
			}
			result := lVal >= rVal
			logger.Debugf("Comparing %f >= %f: %t", lVal, rVal, result)
			return result
		}},
		{"<=", func(l, r string) bool {
			lVal, lErr := strconv.ParseFloat(strings.TrimSpace(l), 64)
			rVal, rErr := strconv.ParseFloat(strings.TrimSpace(r), 64)
			if lErr != nil || rErr != nil {
				logger.Debugf("Failed to parse numbers for <= comparison: left='%s' (err: %v), right='%s' (err: %v)", l, lErr, r, rErr)
				return false
			}
			result := lVal <= rVal
			logger.Debugf("Comparing %f <= %f: %t", lVal, rVal, result)
			return result
		}},
		{">", func(l, r string) bool {
			lVal, lErr := strconv.ParseFloat(strings.TrimSpace(l), 64)
			rVal, rErr := strconv.ParseFloat(strings.TrimSpace(r), 64)
			if lErr != nil || rErr != nil {
				logger.Debugf("Failed to parse numbers for > comparison: left='%s' (err: %v), right='%s' (err: %v)", l, lErr, r, rErr)
				return false
			}
			result := lVal > rVal
			logger.Debugf("Comparing %f > %f: %t", lVal, rVal, result)
			return result
		}},
		{"<", func(l, r string) bool {
			lVal, lErr := strconv.ParseFloat(strings.TrimSpace(l), 64)
			rVal, rErr := strconv.ParseFloat(strings.TrimSpace(r), 64)
			if lErr != nil || rErr != nil {
				logger.Debugf("Failed to parse numbers for < comparison: left='%s' (err: %v), right='%s' (err: %v)", l, lErr, r, rErr)
				return false
			}
			result := lVal < rVal
			logger.Debugf("Comparing %f < %f: %t", lVal, rVal, result)
			return result
		}},
	}

	for _, op := range operators {
		if strings.Contains(processedCondition, op.op) {
			logger.Debugf("Found operator '%s' in condition", op.op)
			parts := strings.SplitN(processedCondition, op.op, 2)
			if len(parts) != 2 {
				logger.Debugf("Invalid split for operator '%s', got %d parts", op.op, len(parts))
				continue
			}

			left := strings.TrimSpace(parts[0])
			right := strings.TrimSpace(parts[1])

			logger.Debugf("Split condition - left: '%s', right: '%s'", left, right)

			// Remove quotes from string literals
			left = strings.Trim(left, `"'`)
			right = strings.Trim(right, `"'`)

			logger.Debugf("After removing quotes - left: '%s', right: '%s'", left, right)

			result := op.eval(left, right)
			logger.Infof("✓ Deterministic breakIf result: %t (condition: '%s' %s '%s')", result, left, op.op, right)
			return result, nil
		}
	}

	logger.Errorf("No valid comparison operator found in condition: '%s'", processedCondition)
	return false, fmt.Errorf("no valid comparison operator found in condition: %s", processedCondition)
}

// waitForCondition polls the specified function until it returns true or times out
func (t *YAMLDefinedTool) waitForCondition(
	ctx context.Context,
	messageID string,
	waitFor *tool_protocol.WaitFor,
	index int,
	item interface{},
	result interface{},
	indexVar, itemVar string,
	inputs map[string]interface{},
	stepResults map[int]interface{},
) error {

	// Build params: user-defined params first, then auto-inject item/index/result (so they take priority)
	params := make(map[string]interface{})

	// Process user-defined params first (with variable replacement)
	if waitFor.Params != nil {
		loopContext := &tool_engine_models.LoopContext{
			IndexVar: indexVar,
			ItemVar:  itemVar,
			Index:    index,
			Item:     item,
			Result:   result,
		}
		for k, v := range waitFor.Params {
			// Pass stepResults to allow result[N][$index].field syntax in waitFor params
			processedValue, err := t.processParameterValueWithLoop(ctx, messageID, k, v, inputs, loopContext, stepResults)
			if err != nil {
				logger.Warnf("Error processing waitFor param '%s': %v, using original value", k, err)
				params[k] = v
			} else {
				params[k] = processedValue
			}
		}
	}

	// Auto-inject item, index, and result AFTER user params (so they're always available)
	params[indexVar] = index
	params[itemVar] = item
	params["result"] = result

	startTime := time.Now()
	maxDuration := time.Duration(waitFor.MaxWaitingSeconds) * time.Second
	pollInterval := time.Duration(waitFor.PollIntervalSeconds) * time.Second

	logger.Infof("Starting waitFor polling for function '%s' (maxWait=%v, pollInterval=%v)", waitFor.Name, maxDuration, pollInterval)

	for {
		// Check context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Call the waitFor function
		logger.Debugf("Calling waitFor function '%s' with params: %v", waitFor.Name, params)
		result, err := t.callWaitForFunction(ctx, messageID, waitFor.Name, params)
		if err != nil {
			logger.Warnf("waitFor function '%s' returned error: %v, continuing to poll", waitFor.Name, err)
			// Continue polling even on error
		} else {
			// Check if result is true (bool or string)
			if isTrueResult(result) {
				logger.Infof("waitFor condition met after %v", time.Since(startTime))
				return nil
			}
			logger.Debugf("waitFor function returned non-true result: %v", result)
		}

		// Check timeout
		if time.Since(startTime) >= maxDuration {
			return fmt.Errorf("waitFor timeout: function '%s' did not return true after %d seconds", waitFor.Name, waitFor.MaxWaitingSeconds)
		}

		// Wait before next poll
		logger.Debugf("waitFor: condition not met, waiting %v before retry", pollInterval)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(pollInterval):
		}
	}
}

// callWaitForFunction calls the specified function and returns its result
// Uses ExecuteFunctionByNameAndVersion for proper execution (same pattern as onSuccess/needs)
func (t *YAMLDefinedTool) callWaitForFunction(
	ctx context.Context,
	messageID string,
	functionName string,
	params map[string]interface{},
) (interface{}, error) {

	// Get message from context to obtain clientID
	retrievedMsg, ok := ctx.Value(MessageInContextKey).(Message)
	if !ok {
		return nil, fmt.Errorf("failed to get message from context for waitFor function execution")
	}
	clientID := retrievedMsg.ClientID

	// Handle cross-tool references (e.g., "utils_shared.shouldSkipIfContains")
	// Parse dot notation to determine correct tool name and function name
	// This MUST happen before context key generation so params are stored correctly
	targetToolName := t.tool.Name
	targetToolVersion := t.tool.Version
	targetFunctionName := functionName

	if parts := strings.SplitN(functionName, ".", 2); len(parts) == 2 {
		// Cross-tool reference: get the tool by name
		crossToolName := parts[0]
		crossFunctionName := parts[1]

		// Try to find the cross-referenced tool (shared tools have their own versions)
		if crossTool, found := t.toolEngine.GetToolByName(crossToolName, ""); found {
			targetToolName = crossTool.Name
			targetToolVersion = crossTool.Version
			targetFunctionName = crossFunctionName
			logger.Debugf("Resolved cross-tool waitFor function '%s' to tool '%s:%s'", functionName, targetToolName, targetToolVersion)
		} else {
			// Tool not found - try searching all tools for one with matching name
			allTools := t.toolEngine.GetAllTools()
			for _, tool := range allTools {
				if tool.Name == crossToolName {
					targetToolName = tool.Name
					targetToolVersion = tool.Version
					targetFunctionName = crossFunctionName
					logger.Debugf("Resolved cross-tool waitFor function '%s' to tool '%s:%s' (via GetAllTools)", functionName, targetToolName, targetToolVersion)
					break
				}
			}
		}
	}

	// Inject params via context (same pattern as onSuccess/needs)
	// Use resolved tool/function names for the context key so params are found correctly
	functionKey := GenerateFunctionKey(targetToolName, targetFunctionName)
	contextKey := fmt.Sprintf("pre_fulfilled_inputs_%s_%s_%s", clientID, messageID, functionKey)

	// Check if checkpoint already has inputs for this function (merge logic from ExecuteDependencies)
	var execCtx context.Context
	if existingInputs, hasCheckpoint := ctx.Value(contextKey).(map[string]interface{}); hasCheckpoint && len(existingInputs) > 0 {
		// Merge: checkpoint inputs take precedence, params fill gaps
		merged := make(map[string]interface{})
		for k, v := range existingInputs {
			merged[k] = v // Checkpoint values have priority
		}
		for k, v := range params {
			if _, exists := merged[k]; !exists {
				merged[k] = v // Only add params that don't exist in checkpoint
			}
		}
		execCtx = context.WithValue(ctx, contextKey, merged)
		logger.Debugf("Merged %d params with %d checkpoint inputs for waitFor function '%s'", len(params), len(existingInputs), functionName)
	} else {
		execCtx = context.WithValue(ctx, contextKey, params)
		logger.Debugf("Executing waitFor function '%s' with params: %v (context key: %s)", functionName, params, contextKey)
	}

	// Execute using ExecuteFunctionByNameAndVersion (handles all operation types)
	result, err := t.toolEngine.ExecuteFunctionByNameAndVersion(
		execCtx,
		messageID,
		targetToolName,
		targetToolVersion,
		targetFunctionName,
		t.inputFulfiller,
	)

	if err != nil {
		return nil, fmt.Errorf("error executing waitFor function '%s': %w", functionName, err)
	}

	// Return the result string - isTrueResult handles strings
	return result, nil
}

// isTrueResult checks if the result indicates true (bool true, string "true", numeric 1)
func isTrueResult(result interface{}) bool {
	if result == nil {
		return false
	}

	switch v := result.(type) {
	case bool:
		return v
	case int:
		return v == 1
	case int64:
		return v == 1
	case float64:
		return v == 1
	case string:
		lowerStr := strings.ToLower(strings.TrimSpace(v))
		return lowerStr == "true" || lowerStr == "1"
	case map[string]interface{}:
		// Check for common patterns in map results
		if val, ok := v["result"]; ok {
			return isTrueResult(val)
		}
		if val, ok := v["success"]; ok {
			return isTrueResult(val)
		}
		if val, ok := v["ready"]; ok {
			return isTrueResult(val)
		}
		if val, ok := v["done"]; ok {
			return isTrueResult(val)
		}
	default:
		// For any other type, check string representation
		str := fmt.Sprintf("%v", v)
		lowerStr := strings.ToLower(strings.TrimSpace(str))
		return lowerStr == "true" || lowerStr == "1"
	}
	return false
}

// shouldSkipIteration checks if the current iteration should be skipped by calling the shouldSkip function
// Returns true if the iteration should be skipped (function returned true or there was an error)
func (t *YAMLDefinedTool) shouldSkipIteration(
	ctx context.Context,
	messageID string,
	shouldSkip *tool_protocol.ShouldSkip,
	index int,
	item interface{},
	indexVar, itemVar string,
	inputs map[string]interface{},
) (bool, error) {

	// Build params: user-defined params first, then auto-inject item/index (so they take priority)
	params := make(map[string]interface{})

	// Process user-defined params first (with variable replacement)
	if shouldSkip.Params != nil {
		loopContext := &tool_engine_models.LoopContext{
			IndexVar: indexVar,
			ItemVar:  itemVar,
			Index:    index,
			Item:     item,
		}
		for k, v := range shouldSkip.Params {
			// Pass nil for stepResults since this is shouldSkip context
			processedValue, err := t.processParameterValueWithLoop(ctx, messageID, k, v, inputs, loopContext, nil)
			if err != nil {
				logger.Warnf("Error processing shouldSkip param '%s': %v, using original value", k, err)
				params[k] = v
			} else {
				params[k] = processedValue
			}
		}
	}

	// Auto-inject item and index AFTER user params (so they're always available)
	params[indexVar] = index
	params[itemVar] = item

	logger.Debugf("Calling shouldSkip function '%s' with params: %v", shouldSkip.Name, params)

	// Call the function (reuse callWaitForFunction since it has the same pattern)
	result, err := t.callWaitForFunction(ctx, messageID, shouldSkip.Name, params)
	if err != nil {
		// On error, skip the iteration (per user requirement)
		logger.Warnf("Error calling shouldSkip function '%s' for iteration %d: %v, skipping iteration", shouldSkip.Name, index, err)
		return true, err
	}

	shouldSkipResult := isTrueResult(result)
	if shouldSkipResult {
		logger.Infof("shouldSkip function '%s' returned true for iteration %d, skipping", shouldSkip.Name, index)
	} else {
		logger.Debugf("shouldSkip function '%s' returned false for iteration %d, proceeding", shouldSkip.Name, index)
	}

	return shouldSkipResult, nil
}

// shouldSkipIterationMultiple checks multiple shouldSkip conditions using OR logic.
// Returns true if ANY condition returns true or errors (iteration should be skipped).
// Uses short-circuit evaluation: stops checking after first true result.
func (t *YAMLDefinedTool) shouldSkipIterationMultiple(
	ctx context.Context,
	messageID string,
	conditions []*tool_protocol.ShouldSkip,
	index int,
	item interface{},
	indexVar, itemVar string,
	inputs map[string]interface{},
) (bool, error) {

	for i, cond := range conditions {
		skip, err := t.shouldSkipIteration(ctx, messageID, cond, index, item, indexVar, itemVar, inputs)
		if err != nil {
			// Error in any condition means skip (fail-safe)
			logger.Warnf("shouldSkip condition %d/%d ('%s') errored for iteration %d: %v, skipping iteration",
				i+1, len(conditions), cond.Name, index, err)
			return true, err
		}
		if skip {
			// Short-circuit: first true result means skip
			logger.Debugf("shouldSkip condition %d/%d ('%s') returned true for iteration %d, skipping",
				i+1, len(conditions), cond.Name, index)
			return true, nil
		}
		logger.Debugf("shouldSkip condition %d/%d ('%s') returned false for iteration %d, checking next",
			i+1, len(conditions), cond.Name, index)
	}

	// All conditions returned false
	logger.Debugf("All %d shouldSkip conditions returned false for iteration %d, proceeding", len(conditions), index)
	return false, nil
}

func (t *YAMLDefinedTool) executeForEachItemWithRetry(
	ctx context.Context,
	messageID string,
	step tool_protocol.Step,
	index int,
	item interface{},
	indexVar, itemVar string,
	inputs map[string]interface{},
	sharedHTTPClient HTTPClient,
	cookieRepo cookieRepository,
	hasAuthStep bool,
	authRetryCount *int,
	maxAuthRetries int,
	authStepIndex int,
	cookiesLoaded *bool,
) (interface{}, []StepExecutionDetail, error) {
	// Collect execution details for this iteration
	var iterationDetails []StepExecutionDetail

	loopContext := &tool_engine_models.LoopContext{
		IndexVar: indexVar,
		ItemVar:  itemVar,
		Index:    index,
		Item:     item,
	}

	// Pass nil for stepResults since this is forEach iteration (no accumulated step results)
	processedParams, err := t.processStepParametersWithLoop(ctx, messageID, step.With, inputs, loopContext, nil)
	if err != nil {
		return nil, iterationDetails, fmt.Errorf("error processing step parameters for iteration %d: %w", index, err)
	}

	tempStep := step
	tempStep.With = processedParams

	const maxRetries = 3
	baseDelay := 1 * time.Second

	for attempt := 0; attempt <= maxRetries; attempt++ {
		result, requestDetails, err := t.executeAPIStep(ctx, messageID, tempStep, sharedHTTPClient)
		resultStr, _ := result.(string)

		// Collect execution details for this attempt
		forEachStepDetail := StepExecutionDetail{
			StepName: fmt.Sprintf("%s [forEach index=%d]", step.Name, index),
			Request:  requestDetails,
			Response: resultStr,
		}
		if err != nil {
			forEachStepDetail.Error = err.Error()
		}
		iterationDetails = append(iterationDetails, forEachStepDetail)

		if !step.IsAuthentication && hasAuthStep && t.isAuthenticationError(err, result) {
			if *authRetryCount >= maxAuthRetries {
				logger.Warnf("Maximum authentication retries (%d) reached, failing with authentication error", maxAuthRetries)
				return nil, iterationDetails, fmt.Errorf("authentication failed after %d retries - unable to authenticate with provided credentials", maxAuthRetries)
			}

			(*authRetryCount)++
			logger.Infof("Authentication error detected in foreach iteration %d (attempt %d/%d), retrying from authentication step", index, *authRetryCount, maxAuthRetries)

			*cookiesLoaded = false
			if cookieRepo != nil {
				cookieRepo.DeleteCookies(t.tool.Name, t.function.Name)
			}

			for retryIndex := authStepIndex; retryIndex < len(t.function.Steps); retryIndex++ {
				retryStep := t.function.Steps[retryIndex]
				// Pass nil for stepResults since this is a retry in forEach context (no accumulated results)
				processedParams, err := t.processStepParameters(ctx, messageID, retryStep.With, inputs, nil)
				if err != nil {
					return nil, iterationDetails, fmt.Errorf("error processing retry step parameters: %w", err)
				}
				retryStep.With = processedParams

				retryResult, retryRequestDetails, retryErr := t.executeAPIStep(ctx, messageID, retryStep, sharedHTTPClient)
				retryResultStr, _ := retryResult.(string)

				// Collect retry step details
				forEachRetryStepDetail := StepExecutionDetail{
					StepName: fmt.Sprintf("%s [forEach index=%d retry]", retryStep.Name, index),
					Request:  retryRequestDetails,
					Response: retryResultStr,
				}
				if retryErr != nil {
					forEachRetryStepDetail.Error = retryErr.Error()
				}
				iterationDetails = append(iterationDetails, forEachRetryStepDetail)

				if retryErr != nil {
					// Check if this is another authentication error during retry
					if !retryStep.IsAuthentication && t.isAuthenticationError(retryErr, retryResult) {
						return nil, iterationDetails, fmt.Errorf("authentication failed during foreach retry step '%s' after %d attempts", retryStep.Name, *authRetryCount)
					}
					return nil, iterationDetails, fmt.Errorf("error executing retry step '%s': %w", retryStep.Name, retryErr)
				}

				if retryStep.IsAuthentication && cookieRepo != nil {
					t.saveCookiesFromClient(ctx, sharedHTTPClient, cookieRepo)
				}
			}

			result, requestDetails, err = t.executeAPIStep(ctx, messageID, tempStep, sharedHTTPClient)
			resultStr, _ = result.(string)

			// Collect final retry attempt details
			forEachAfterAuthRetryDetail := StepExecutionDetail{
				StepName: fmt.Sprintf("%s [forEach index=%d after auth retry]", step.Name, index),
				Request:  requestDetails,
				Response: resultStr,
			}
			if err != nil {
				forEachAfterAuthRetryDetail.Error = err.Error()
			}
			iterationDetails = append(iterationDetails, forEachAfterAuthRetryDetail)
		}

		if err == nil {
			if step.IsAuthentication && cookieRepo != nil {
				t.saveCookiesFromClient(ctx, sharedHTTPClient, cookieRepo)
			}
			// Handle saveAsFile for forEach iterations — convert raw response to FileResult
			if step.SaveAsFile != nil && requestDetails != nil {
				fileResult, fileErr := t.processStepAsFileDownload(ctx, messageID, step, requestDetails, inputs)
				if fileErr != nil {
					return nil, iterationDetails, fmt.Errorf("saveAsFile failed for forEach iteration %d: %w", index, fileErr)
				}
				return fileResult, iterationDetails, nil
			}
			return result, iterationDetails, nil
		}

		if !t.isRetryableError(err) || attempt == maxRetries {
			return nil, iterationDetails, fmt.Errorf("error executing step for iteration %d after %d attempts: %w", index, attempt+1, err)
		}

		multiplier := 1 << uint(attempt)
		delay := time.Duration(int64(baseDelay) * int64(multiplier))
		logger.Warnf("Retryable error in iteration %d (attempt %d/%d), retrying in %v: %v", index, attempt+1, maxRetries+1, delay, err)

		select {
		case <-ctx.Done():
			return nil, iterationDetails, ctx.Err()
		case <-time.After(delay):
		}
	}

	return nil, iterationDetails, fmt.Errorf("max retries exceeded for iteration %d", index)
}

func (t *YAMLDefinedTool) isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	errStr := strings.ToLower(err.Error())

	retryablePatterns := []string{
		"timeout",
		"connection reset",
		"connection refused",
		"temporary failure",
		"service unavailable",
		"internal server error",
		"bad gateway",
		"gateway timeout",
		"too many requests",
		"500",
		"502",
		"503",
		"504",
		"429",
	}

	for _, pattern := range retryablePatterns {
		if strings.Contains(errStr, pattern) {
			return true
		}
	}

	return false
}

// processStepParametersWithLoop processes step parameters with loop context
// stepResults allows result[X] references to be resolved from local state first
func (t *YAMLDefinedTool) processStepParametersWithLoop(
	ctx context.Context,
	messageID string,
	params map[string]interface{},
	inputs map[string]interface{},
	loopContext *tool_engine_models.LoopContext,
	stepResults map[int]interface{},
) (map[string]interface{}, error) {
	if params == nil {
		return nil, nil
	}

	processedParams := make(map[string]interface{})

	for key, value := range params {
		processedValue, err := t.processParameterValueWithLoop(ctx, messageID, key, value, inputs, loopContext, stepResults)
		if err != nil {
			return nil, err
		}
		processedParams[key] = processedValue
	}

	return processedParams, nil
}

// processParameterValueWithLoop processes a parameter value with loop context
// stepResults allows result[X] references to be resolved from local state first
func (t *YAMLDefinedTool) processParameterValueWithLoop(
	ctx context.Context,
	messageID string,
	key string,
	value interface{},
	inputs map[string]interface{},
	loopContext *tool_engine_models.LoopContext,
	stepResults map[int]interface{},
) (interface{}, error) {
	switch v := value.(type) {
	case string:
		// First, replace result[X] references from local stepResults (current function's results)
		// This ensures we use this function's step results, not results from other functions
		localReplaced := t.replaceResultReferencesFromStepResults(v, stepResults)

		// Then replace variables in string values including loop variables
		replaced, err := t.variableReplacer.ReplaceVariablesWithLoop(ctx, localReplaced, inputs, loopContext)
		if err != nil {
			return nil, fmt.Errorf("error replacing variables in parameter '%s': %w", key, err)
		}
		return replaced, nil

	case map[string]interface{}:
		// Process nested maps recursively
		processedMap := make(map[string]interface{})
		for nestedKey, nestedValue := range v {
			processedNestedValue, err := t.processParameterValueWithLoop(ctx, messageID, nestedKey, nestedValue, inputs, loopContext, stepResults)
			if err != nil {
				return nil, err
			}
			processedMap[nestedKey] = processedNestedValue
		}
		return processedMap, nil

	case map[interface{}]interface{}:
		processedMap := make(map[string]interface{})
		for nestedKey, nestedValue := range v {
			nestedKeyStr, ok := nestedKey.(string)
			if !ok {
				return nil, fmt.Errorf("nested key '%v' is not a string", nestedKey)
			}

			processedNestedValue, err := t.processParameterValueWithLoop(ctx, messageID, nestedKeyStr, nestedValue, inputs, loopContext, stepResults)
			if err != nil {
				return nil, err
			}
			processedMap[nestedKeyStr] = processedNestedValue
		}
		return processedMap, nil

	case []interface{}:
		// Process arrays recursively
		processedArray := make([]interface{}, len(v))
		for i, item := range v {
			processedItem, err := t.processParameterValueWithLoop(ctx, messageID, fmt.Sprintf("%s[%d]", key, i), item, inputs, loopContext, stepResults)
			if err != nil {
				return nil, err
			}
			processedArray[i] = processedItem
		}
		return processedArray, nil

	default:
		// For other types, return as-is
		return value, nil
	}
}

// processStepParameters processes and replaces variables in step parameters
// stepResults contains the results of previous steps in the current function execution,
// allowing result[X] references to be resolved from local state before falling back to DB
func (t *YAMLDefinedTool) processStepParameters(
	ctx context.Context,
	messageID string,
	params map[string]interface{},
	inputs map[string]interface{},
	stepResults map[int]interface{},
) (map[string]interface{}, error) {
	return t.processStepParametersWithOptions(ctx, messageID, params, inputs, stepResults, false)
}

// processStepParametersForTerminal processes step parameters for terminal operations
// It keeps unresolved variables (like bash $variables) instead of replacing with empty string
func (t *YAMLDefinedTool) processStepParametersForTerminal(
	ctx context.Context,
	messageID string,
	params map[string]interface{},
	inputs map[string]interface{},
	stepResults map[int]interface{},
) (map[string]interface{}, error) {
	return t.processStepParametersWithOptions(ctx, messageID, params, inputs, stepResults, true)
}

func (t *YAMLDefinedTool) processStepParametersWithOptions(
	ctx context.Context,
	messageID string,
	params map[string]interface{},
	inputs map[string]interface{},
	stepResults map[int]interface{},
	keepUnresolved bool,
) (map[string]interface{}, error) {
	if params == nil {
		return nil, nil
	}

	processedParams := make(map[string]interface{})

	for key, value := range params {
		processedValue, err := t.processParameterValueWithOptions(ctx, messageID, key, value, inputs, stepResults, keepUnresolved)
		if err != nil {
			return nil, err
		}
		processedParams[key] = processedValue
	}

	return processedParams, nil
}

// processParameterValue handles different types of parameter values
// stepResults allows result[X] references to be resolved from local state first
func (t *YAMLDefinedTool) processParameterValue(
	ctx context.Context,
	messageID string,
	key string,
	value interface{},
	inputs map[string]interface{},
	stepResults map[int]interface{},
) (interface{}, error) {
	return t.processParameterValueWithOptions(ctx, messageID, key, value, inputs, stepResults, false)
}

// processParameterValueWithOptions handles different types of parameter values with options
// keepUnresolved: if true, keeps unresolved variables (like bash $vars) instead of replacing with empty string
func (t *YAMLDefinedTool) processParameterValueWithOptions(
	ctx context.Context,
	messageID string,
	key string,
	value interface{},
	inputs map[string]interface{},
	stepResults map[int]interface{},
	keepUnresolved bool,
) (interface{}, error) {
	switch v := value.(type) {
	case string:
		// First, replace result[X] references from local stepResults (current function's results)
		// This ensures we use this function's step results, not results from other functions
		// that might share the same resultIndex in the database
		localReplaced := t.replaceResultReferencesFromStepResults(v, stepResults)

		// Then do full variable replacement (handles $vars, env vars, and DB fallback for result[X])
		// For terminal operations: keep unresolved variables (bash $vars) and shell-escape replaced values
		// to prevent single quotes in LLM/user content from breaking bash echo '...' commands
		replaced, err := t.variableReplacer.ReplaceVariablesWithOptions(ctx, localReplaced, inputs, tool_engine_models.ReplaceOptions{
			DBContext:                  false,
			KeepUnresolvedPlaceholders: keepUnresolved,
			ShellEscape:                keepUnresolved, // terminal operations need shell-safe values
		})
		if err != nil {
			return nil, fmt.Errorf("error replacing variables in parameter '%s': %w", key, err)
		}
		return replaced, nil

	case map[string]interface{}:
		// Process nested maps recursively
		processedMap := make(map[string]interface{})
		for nestedKey, nestedValue := range v {
			processedNestedValue, err := t.processParameterValueWithOptions(ctx, messageID, nestedKey, nestedValue, inputs, stepResults, keepUnresolved)
			if err != nil {
				return nil, err
			}
			processedMap[nestedKey] = processedNestedValue
		}
		return processedMap, nil

	case map[interface{}]interface{}:
		processedMap := make(map[string]interface{})
		for nestedKey, nestedValue := range v {
			nestedKeyStr, ok := nestedKey.(string)
			if !ok {
				return nil, fmt.Errorf("nested key '%v' is not a string", nestedKey)
			}

			processedNestedValue, err := t.processParameterValueWithOptions(ctx, messageID, nestedKeyStr, nestedValue, inputs, stepResults, keepUnresolved)
			if err != nil {
				return nil, err
			}
			processedMap[nestedKeyStr] = processedNestedValue
		}
		return processedMap, nil
	case []interface{}:
		// Process arrays recursively
		processedArray := make([]interface{}, len(v))
		for i, item := range v {
			processedItem, err := t.processParameterValueWithOptions(ctx, messageID, fmt.Sprintf("%s[%d]", key, i), item, inputs, stepResults, keepUnresolved)
			if err != nil {
				return nil, err
			}
			processedArray[i] = processedItem
		}
		return processedArray, nil

	default:
		// For other types (numbers, booleans, etc.), just return as is
		return value, nil
	}
}

// executeStep executes a single step (mock implementation)
func (t *YAMLDefinedTool) executeStep(
	ctx context.Context,
	messageID string,
	step tool_protocol.Step,
	params map[string]interface{},
) (interface{}, error) {
	// Mock implementation for each action type

	step.With = params

	switch step.Action {
	case "open_url":
		// Mock a web browser action
		url, ok := params["url"].(string)
		if !ok {
			return nil, fmt.Errorf("missing or invalid url parameter")
		}
		// In a real implementation, this would interact with a web browser
		fmt.Printf("Would open URL: %s\n", url)
		return nil, nil

	case "read":
		// Mock reading content from a web page or UI
		// In a real implementation, this would extract content
		mockContent := map[string]interface{}{
			"title":     "Mock Page Title",
			"content":   "This is mock content that would be read from a web page or UI element.",
			"timestamp": time.Now().Format(time.RFC3339),
		}
		return mockContent, nil

	case "find_and_click":
		// Mock clicking an element
		findBy, ok := params["findBy"].(string)
		if !ok {
			return nil, fmt.Errorf("missing or invalid findBy parameter")
		}
		findValue, ok := params["findValue"].(string)
		if !ok {
			return nil, fmt.Errorf("missing or invalid findValue parameter")
		}

		fmt.Printf("Would find and click element by %s with value: %s\n", findBy, findValue)
		return nil, nil

	case "find_fill_and_tab", "find_fill_and_return":
		// Mock filling a form field
		findBy, ok := params["findBy"].(string)
		if !ok {
			return nil, fmt.Errorf("missing or invalid findBy parameter")
		}
		findValue, ok := params["findValue"].(string)
		if !ok {
			return nil, fmt.Errorf("missing or invalid findValue parameter")
		}
		fillValue, ok := params["fillValue"].(string)
		if !ok {
			return nil, fmt.Errorf("missing or invalid fillValue parameter")
		}

		fmt.Printf("Would find element by %s with value %s and fill with: %s\n",
			findBy, findValue, fillValue)
		return nil, nil

	// HTTP methods for API calls
	case "GET", "POST", "PUT", "DELETE", "PATCH":
		if t.apiExecutor == nil {
			return nil, nil // no API executor configured
		}

		// Execute the API call via the APIExecutor interface
		_, rawOutput, _, err := t.apiExecutor.ExecuteAPICall(
			ctx,
			step,
			params,
			nil, // headers
		)
		if err != nil {
			return nil, fmt.Errorf("error executing API call: %w", err)
		}

		return rawOutput, nil
	default:
		return nil, fmt.Errorf("unsupported action: %s", step.Action)
	}
}

// generateWebBrowsingGuide formats steps for the LLM and generates a detailed web browsing guide
func (t *YAMLDefinedTool) generateWebBrowsingGuide(
	ctx context.Context,
	messageID string,
	steps []tool_protocol.Step,
	inputs map[string]interface{},
) (string, error) {
	stepsDescription := ""
	for i, step := range steps {
		// Process parameters for this step
		// Pass nil for stepResults since this is guide generation, not execution
		processedParams, err := t.processStepParameters(ctx, messageID, step.With, inputs, nil)
		if err != nil {
			return "", fmt.Errorf("error processing step parameters for guide generation: %w", err)
		}
		step.With = processedParams

		stepDesc := fmt.Sprintf("Step %d: %s\n", i+1, step.Name)
		stepDesc += fmt.Sprintf("Action: %s\n", step.Action)

		if len(processedParams) > 0 {
			stepDesc += "Parameters:\n"
			for key, value := range processedParams {
				stepDesc += fmt.Sprintf("  - %s: %v\n", key, value)
			}
		}

		// Add goals if present
		if step.Goal != "" {
			processedGoal, err := t.variableReplacer.ReplaceVariablesWithOptions(ctx, step.Goal, inputs, tool_engine_models.ReplaceOptions{
				KeepUnresolvedPlaceholders: true, // Keep $var for LLM readability
			})
			if err != nil {
				return "", fmt.Errorf("error processing goal for guide generation: %w", err)
			}
			stepDesc += fmt.Sprintf("Goal: %s\n", processedGoal)
		}

		// Add error handling information if present
		if step.OnError != nil {
			stepDesc += fmt.Sprintf("Error Handling: %s\n", step.OnError.Message)
		}

		stepsDescription += stepDesc + "\n"
	}

	stepsDescription += "<inputs>"
	for key, value := range inputs {
		stepsDescription += fmt.Sprintf("\nInput: %s: %v\n", key, value)
	}
	stepsDescription += "</inputs>"

	// todo: check if we need to process inputs because it should be evaluated in the parameters
	//inputDescription := "User Inputs:\n"
	//for key, value := range inputs {
	//	inputDescription += fmt.Sprintf("  - %s: %v\n", key, value)
	//}

	prompt := fmt.Sprintf(`You are a web navigation assistant that converts technical steps into clear step-by-step instructions. 
Given the following web browsing steps and user inputs, create a detailed step-by-step guide that a AI Agent could follow to accomplish the task.
Include specifics about what to look for, where to click, what to type, and what to expect on each page.
Make the guide comprehensive but easy to follow, and include troubleshooting tips where appropriate. Be short as possible. If any of the steps require the fulfillment of inputs and the inputs are wrong or missing, advise in the step-by-step guide that it must complete the task saying that which inputs are missing or wrong. this is very important guide.

%s

**Resolve any conditional steps** at generation time.  
   - If a step says “if $foo is provided, do A; otherwise, do B,” and $foo is *not* in <inputs>, only output B.  
   - Never emit “if…” clauses in your final guide—only the branch that applies.

For all the Find UI elements operations:
   1. **Semantic equal match or Exact match** on the text or label.  
   2. **Domain-specific semantic match** (e.g. treat “pedido para viagem” .. “delivery” as equivalent for a restaurant context, “curso intensivo” .. “bootcamp” in courses, etc.).  
   3. **Error out** if neither literal nor semantic match succeeds—do *not* guess or skip, use the action request_missing_info to request clarification from user.
   4. If some step is about extracting data from a page, your guide should explain that this extracted data should be printed to the results.`,
		stepsDescription)

	messages := thread.New()
	messages.AddMessages(
		thread.NewSystemMessage().AddContent(thread.NewTextContent(prompt)),
		thread.NewUserMessage().AddContent(thread.NewTextContent("Please provide a detailed human-readable web browsing guide based on these steps but keep in mind it will be used by ai agent")),
	)

	llm := newLLM()
	err := generateLLM(ctx, llm, messages)
	if err != nil {
		return "", fmt.Errorf("error generating web browsing guide with LLM: %w", err)
	}

	if len(messages.LastMessage().Contents) == 0 {
		return "", errors.New("no guide generated by LLM")
	}

	guide := messages.LastMessage().Contents[0].AsString()
	sc, err := t.variableReplacer.ReplaceVariablesWithOptions(ctx, t.function.SuccessCriteria, inputs, tool_engine_models.ReplaceOptions{
		KeepUnresolvedPlaceholders: true, // Keep $var for LLM readability
	})
	if err != nil {
		sc = t.function.SuccessCriteria
	}

	guide = fmt.Sprintf("%s\n\n%s\n\nsuccess criteria ->%s",
		guide,
		"\n Attention: if some input is not correct or known, use the action request_missing_info to request clarification from user. \n  THINK HARD STEP BY STEP",
		sc,
	)

	return guide, nil
}

// TODO: implement
func (t *YAMLDefinedTool) handleStepError(ctx context.Context, messageID string, step tool_protocol.Step, err error) error {
	if step.OnError == nil {
		return err
	}

	// Log the error
	fmt.Printf("Error in step '%s': %v (Strategy: %s)\n",
		step.Name, err, step.OnError.Strategy)

	// Handle based on strategy
	switch step.OnError.Strategy {
	case tool_protocol.OnErrorRequestUserInput:
		// This would normally initiate a user input request
		fmt.Printf("Would request user input: %s\n", step.OnError.Message)

	case tool_protocol.OnErrorRequestN1Support:
		// This would normally escalate to N1 support
		fmt.Printf("Would escalate to N1 support: %s\n", step.OnError.Message)

	// Add other strategies as needed

	default:
		return fmt.Errorf("unsupported error strategy: %s", step.OnError.Strategy)
	}

	return nil
}

// filterContextForFunctions creates a filtered context that only includes outputs from specified functions
func (t *YAMLDefinedTool) filterContextForFunctions(ctx context.Context, functionNames []string) context.Context {
	// Inject the function names list into context for filtering at the workflow event level
	newCtx := context.WithValue(ctx, tool_engine_models.FilteredFunctionsInContextKey, functionNames)
	return newCtx
}

// buildDependencyResultsMap builds a map of dependency results from the execution tracker
// Returns: map[functionName]interface{} where interface{} is the parsed JSON output or raw string
// This map is used to pass function results to VariableReplacer for $functionName references
// NOTE: This loads ALL TRANSITIVE dependencies, not just direct ones, to align with parser validation
// which allows referencing any transitively reachable function in runOnlyIf.deterministic expressions
func (t *YAMLDefinedTool) buildDependencyResultsMap(ctx context.Context, messageID string) map[string]interface{} {

	resultsMap := make(map[string]interface{})

	// Get all needs/dependencies for this function
	if len(t.function.Needs) == 0 {
		return resultsMap
	}

	// Build transitive dependency map for ALL functions in the tool
	// This computes which functions are transitively reachable from the current function
	allTransitiveDeps := tool_protocol.BuildTransitiveDependencies(t.tool.Functions)
	transitiveDepsForCurrentFunc := allTransitiveDeps[t.function.Name]

	if transitiveDepsForCurrentFunc == nil || len(transitiveDepsForCurrentFunc) == 0 {
		logger.Debugf("No transitive dependencies found for function '%s'", t.function.Name)
		return resultsMap
	}

	logger.Debugf("Loading %d transitive dependencies for function '%s'", len(transitiveDepsForCurrentFunc), t.function.Name)

	// Load results for all transitively reachable dependencies
	for depName := range transitiveDepsForCurrentFunc {
		// Check if this function has been executed
		executed, output, err := t.executionTracker.HasExecuted(ctx, messageID, depName)
		if err != nil {
			logger.Warnf("Error checking execution for dependency '%s': %v", depName, err)
			continue
		}

		if !executed {
			logger.Debugf("Transitive dependency '%s' not yet executed, skipping", depName)
			continue
		}

		// Check if the function was skipped by examining the output message
		// Skipped functions have output starting with "TOOL_EXECUTION_SKIPPED:"
		if strings.HasPrefix(output, "TOOL_EXECUTION_SKIPPED:") {
			logger.Debugf("Transitive dependency '%s' was skipped, storing nil for null semantics", depName)
			resultsMap[depName] = nil
			continue
		}

		// Try to parse the output as JSON
		if parsed, ok := tryParseJSON(output); ok {
			// Store the parsed JSON directly (no wrapper)
			// Access pattern: $functionName.field for objects, $functionName[0] for arrays
			logger.Debugf("Transitive dependency '%s' output parsed successfully as JSON, storing directly", depName)
			resultsMap[depName] = parsed
		} else {
			// If not valid JSON, store the raw string directly
			// Access pattern: $functionName (compares entire value as string)
			logger.Debugf("Transitive dependency '%s' output is not valid JSON, storing as raw string", depName)
			resultsMap[depName] = output
		}
	}

	logger.Infof("Built dependency results map with %d entries (including transitive dependencies) for function '%s'", len(resultsMap), t.function.Name)
	return resultsMap
}

// evaluateFunctionCallRunOnlyIf evaluates a runOnlyIf condition for a FunctionCall (onSuccess/onFailure).
// Returns: (shouldExecute bool, skipReason string, error)
// The context available includes:
// - Parent function's fulfilled inputs (e.g., $dealId)
// - Dependency results (e.g., $getDeal.result.stage)
// - Parent function's output as "result" (e.g., $result.field)
// - System variables ($NOW, $USER, etc.)
func (t *YAMLDefinedTool) evaluateFunctionCallRunOnlyIf(
	ctx context.Context,
	messageID string,
	runOnlyIf interface{},
	parentInputs map[string]interface{},
	parentOutput string,
	stepResults map[int]interface{},
) (bool, string, error) {

	// Parse runOnlyIf
	runOnlyIfObj, err := tool_protocol.ParseRunOnlyIf(runOnlyIf)
	if err != nil {
		return false, "", fmt.Errorf("failed to parse runOnlyIf: %w", err)
	}

	if runOnlyIfObj == nil {
		return true, "", nil // No condition, always execute
	}

	// Only deterministic mode is supported for FunctionCall runOnlyIf
	if runOnlyIfObj.Deterministic == "" {
		return false, "", fmt.Errorf("FunctionCall runOnlyIf requires 'deterministic' expression")
	}

	originalExpr := runOnlyIfObj.Deterministic
	logger.Debugf("Evaluating FunctionCall runOnlyIf deterministic condition: %s", originalExpr)

	// Build variables map combining:
	// 1. Parent function's fulfilled inputs ($dealId, $customerId, etc.)
	// 2. Dependency results from execution tracker ($getDeal.stage, etc.)
	// 3. Parent function's output as "result" ($result, $result.field, etc.)
	variablesMap := make(map[string]interface{})

	// Add parent function's inputs
	for k, v := range parentInputs {
		variablesMap[k] = v
	}

	// Add dependency results (reuse existing method)
	dependencyResults := t.buildDependencyResultsMap(ctx, messageID)
	for k, v := range dependencyResults {
		variablesMap[k] = v
	}

	// Add parent function's output as "result"
	if parentOutput != "" {
		if parsed, ok := tryParseJSON(parentOutput); ok {
			variablesMap["result"] = parsed
		} else {
			variablesMap["result"] = parentOutput
		}
	}

	// Add step results as result[N] for consistent resolution with params.
	// Without this, result[N] in runOnlyIf falls back to DB lookup which may
	// return stale data after a sibling onSuccess function overwrites the record.
	for idx, stepResult := range stepResults {
		key := fmt.Sprintf("result[%d]", idx)
		variablesMap[key] = stepResult
	}

	logger.Debugf("Built variables map with %d entries for FunctionCall runOnlyIf evaluation", len(variablesMap))

	// Replace variables in the expression
	replacedExpr, err := t.variableReplacer.ReplaceVariables(ctx, originalExpr, variablesMap)
	if err != nil {
		return false, "", fmt.Errorf("failed to replace variables in expression '%s': %w", originalExpr, err)
	}
	logger.Debugf("Expression after variable replacement: %s", replacedExpr)

	// Evaluate the deterministic expression
	result, err := tool_protocol.EvaluateDeterministicExpression(replacedExpr)
	if err != nil {
		return false, "", fmt.Errorf("failed to evaluate expression '%s': %w", replacedExpr, err)
	}

	skipReason := fmt.Sprintf("%s -> %s -> %t", originalExpr, replacedExpr, result)
	logger.Infof("FunctionCall runOnlyIf evaluation: %s", skipReason)

	return result, skipReason, nil
}

// evaluateInferenceCondition evaluates a semantic condition using LLM inference.
// Returns: (result bool, explanation string)
func (t *YAMLDefinedTool) evaluateInferenceCondition(
	ctx context.Context,
	messageID string,
	clientID string,
	condition string,
	allowedSystemFunctions []string,
	disableAllSystemFunctions bool,
	callback tool_engine_models.AgenticWorkflowCallback,
) (bool, string) {

	functionKey := GenerateFunctionKey(t.tool.Name, t.function.Name)

	// Check for mock service in context (test mode only - zero-cost in production)
	// Production code NEVER sets TestMockServiceKey, so this check is always false in production
	if mockSvc, ok := ctx.Value(tool_engine_models.TestMockServiceKey).(tool_engine_models.IMockService); ok && mockSvc != nil {
		// Check if we should mock runOnlyIf_inference for this function
		// Use "runOnlyIf" as the step name for the mock
		if mockSvc.ShouldMock("runOnlyIf_inference", functionKey, "runOnlyIf") {
			if response, exists := mockSvc.GetMockResponseValue(functionKey, "runOnlyIf"); exists {
				logger.Infof("[TEST MOCK] Returning mock for runOnlyIf_inference %s", functionKey)

				// Parse the mock response
				// Supported formats:
				// 1. bool: true/false
				// 2. map with "result" (bool) and optional "explanation" (string)
				// 3. string: "true"/"false"
				result, explanation := parseMockRunOnlyIfResponse(response)
				mockSvc.RecordCall(functionKey, "runOnlyIf", "runOnlyIf_inference", nil, response, "")
				return result, explanation
			}
		}
	}

	// Prepare success criteria for inference - similar to runOnlyIf
	successCriteriaCondition := `Evaluate if the re-run condition is met based on the description.
Return your response in this EXACT format:
done: {true or false} || {clear explanation}

Examples:
- done: true || The result indicates processing is still in progress.
- done: false || The result shows the operation has completed successfully.

The explanation should be clear and explain your decision. Output ONLY in the format shown above.`

	var successCriteria interface{}
	if len(allowedSystemFunctions) > 0 {
		logger.Debugf("reRunIf: restricting inference to system functions: %v", allowedSystemFunctions)
		successCriteria = &tool_protocol.SuccessCriteriaObject{
			Condition:              successCriteriaCondition,
			AllowedSystemFunctions: allowedSystemFunctions,
		}
	} else if disableAllSystemFunctions {
		logger.Debugf("reRunIf: disabling all system functions for inference")
		successCriteria = &tool_protocol.SuccessCriteriaObject{
			Condition:                 successCriteriaCondition,
			DisableAllSystemFunctions: true,
		}
	} else {
		successCriteria = successCriteriaCondition
	}

	// Build inference input similar to runOnlyIf
	inference := tool_protocol.Input{
		Name:            fmt.Sprintf("rerunif_condition_%s", t.function.Name),
		Description:     condition,
		SuccessCriteria: successCriteria,
	}

	logger.Debugf("evaluateInferenceCondition: invoking agentic inference for: %s", condition)

	rawResponse, err := t.inputFulfiller.AgenticInfer(ctx, messageID, clientID, inference, t.tool)
	if err != nil {
		logger.Warnf("evaluateInferenceCondition: inference error: %v", err)
		return false, fmt.Sprintf("inference error: %v", err)
	}

	logger.Debugf("evaluateInferenceCondition: raw response: %s", rawResponse)

	// Parse the response using existing logic
	result, explanation := parseRunOnlyIfResponse(rawResponse)
	return result, explanation
}

// evaluateReRunIf evaluates the reRunIf condition for a function after execution.
// Returns: (shouldReRun bool, reason string, error)
// The context available includes:
// - Function's fulfilled inputs ($inputName)
// - Dependency results from execution tracker ($funcName.field)
// - Function's output as "result" ($result, $result.field)
// - Retry context as "RETRY" ($RETRY.count)
// - Step results (result[N])
// - System variables ($NOW, $USER, etc.)
func (t *YAMLDefinedTool) evaluateReRunIf(
	ctx context.Context,
	messageID string,
	clientID string,
	config *tool_protocol.ReRunIfConfig,
	inputs map[string]interface{},
	output string,
	stepResults map[int]interface{},
	retryCount int,
	callback tool_engine_models.AgenticWorkflowCallback,
) (bool, string, error) {

	if config == nil || !config.HasConditions() {
		return false, "no condition defined", nil
	}

	// Check max retries
	maxRetries := config.GetMaxRetries()
	if retryCount >= maxRetries {
		logger.Infof("reRunIf: max retries reached (%d/%d), stopping", retryCount, maxRetries)
		return false, fmt.Sprintf("max retries reached (%d)", maxRetries), nil
	}

	// Build variables map for condition evaluation
	variablesMap := make(map[string]interface{})

	// Add function's fulfilled inputs
	for k, v := range inputs {
		variablesMap[k] = v
	}

	// Add dependency results (reuse existing method)
	dependencyResults := t.buildDependencyResultsMap(ctx, messageID)
	for k, v := range dependencyResults {
		variablesMap[k] = v
	}

	// Add function's output as "result"
	if output != "" {
		if parsed, ok := tryParseJSON(output); ok {
			variablesMap["result"] = parsed
		} else {
			variablesMap["result"] = output
		}
	}

	// Add retry context as "RETRY" (for $RETRY.count access)
	variablesMap["RETRY"] = map[string]interface{}{
		"count": retryCount,
	}

	// Add step results (for result[N] access)
	for idx, result := range stepResults {
		key := fmt.Sprintf("result[%d]", idx)
		variablesMap[key] = result
	}

	logger.Debugf("reRunIf: built variables map with %d entries, retryCount=%d", len(variablesMap), retryCount)

	// Track condition results for combine mode
	var conditionResults []bool
	var conditionReasons []string
	combineMode := config.GetCombineMode()

	// 1. Evaluate deterministic condition if present
	if config.Deterministic != "" {
		originalExpr := config.Deterministic
		logger.Debugf("reRunIf: evaluating deterministic condition: %s", originalExpr)

		replacedExpr, err := t.variableReplacer.ReplaceVariables(ctx, originalExpr, variablesMap)
		if err != nil {
			logger.Errorf("reRunIf: failed to replace variables in deterministic condition: %v", err)
			return false, "", fmt.Errorf("failed to replace variables in deterministic condition '%s': %w", originalExpr, err)
		}
		logger.Debugf("reRunIf: deterministic expression after replacement: %s", replacedExpr)

		result, err := tool_protocol.EvaluateDeterministicExpression(replacedExpr)
		if err != nil {
			logger.Errorf("reRunIf: failed to evaluate deterministic condition: %v", err)
			return false, "", fmt.Errorf("failed to evaluate deterministic condition '%s': %w", replacedExpr, err)
		}

		reason := fmt.Sprintf("deterministic: %s -> %s -> %t", originalExpr, replacedExpr, result)
		conditionResults = append(conditionResults, result)
		conditionReasons = append(conditionReasons, reason)
		logger.Infof("reRunIf: %s", reason)
	}

	// 2. Evaluate function call if present
	if config.Call != nil {
		logger.Debugf("reRunIf: evaluating function call: %s", config.Call.Name)

		// Prepare params with variable replacement
		callParams := make(map[string]interface{})
		for k, v := range config.Call.Params {
			if strVal, ok := v.(string); ok {
				replaced, err := t.variableReplacer.ReplaceVariables(ctx, strVal, variablesMap)
				if err != nil {
					logger.Warnf("reRunIf: failed to replace variables in call param %s: %v", k, err)
					callParams[k] = v
				} else {
					callParams[k] = replaced
				}
			} else {
				callParams[k] = v
			}
		}

		// Execute the function call
		callResult, err := t.executeReRunIfFunctionCall(ctx, messageID, clientID, config.Call.Name, callParams, callback)
		if err != nil {
			logger.Errorf("reRunIf: failed to execute function call: %v", err)
			return false, "", fmt.Errorf("failed to execute reRunIf function call '%s': %w", config.Call.Name, err)
		}

		// Parse result as truthy/falsy
		result := isTruthy(callResult)
		reason := fmt.Sprintf("call(%s): %s -> %t", config.Call.Name, callResult, result)
		conditionResults = append(conditionResults, result)
		conditionReasons = append(conditionReasons, reason)
		logger.Infof("reRunIf: %s", reason)
	}

	// 3. Evaluate inference condition if present
	if config.Condition != "" || config.Inference != nil {
		var inferenceCondition string
		var inferenceFrom []string
		var inferenceAllowedFuncs []string
		var inferenceDisableAll bool

		if config.Inference != nil {
			inferenceCondition = config.Inference.Condition
			inferenceFrom = config.Inference.From
			inferenceAllowedFuncs = config.Inference.AllowedSystemFunctions
			inferenceDisableAll = config.Inference.DisableAllSystemFunctions
		} else {
			inferenceCondition = config.Condition
			inferenceFrom = config.From
			inferenceAllowedFuncs = config.AllowedSystemFunctions
			inferenceDisableAll = config.DisableAllSystemFunctions
		}

		logger.Debugf("reRunIf: evaluating inference condition: %s", inferenceCondition)

		// Prepare context for inference evaluation
		evalCtx := ctx
		if len(inferenceFrom) > 0 {
			evalCtx = t.filterContextForFunctions(ctx, inferenceFrom)
		}

		// Evaluate using inference
		result, explanation := t.evaluateInferenceCondition(
			evalCtx,
			messageID,
			clientID,
			inferenceCondition,
			inferenceAllowedFuncs,
			inferenceDisableAll,
			callback,
		)

		reason := fmt.Sprintf("inference: %s -> %t (%s)", inferenceCondition, result, explanation)
		conditionResults = append(conditionResults, result)
		conditionReasons = append(conditionReasons, reason)
		logger.Infof("reRunIf: %s", reason)
	}

	// Combine results based on combine mode
	if len(conditionResults) == 0 {
		return false, "no conditions evaluated", nil
	}

	var shouldReRun bool
	if combineMode == tool_protocol.CombineModeAND {
		shouldReRun = true
		for _, r := range conditionResults {
			if !r {
				shouldReRun = false
				break
			}
		}
	} else { // CombineModeOR (default)
		shouldReRun = false
		for _, r := range conditionResults {
			if r {
				shouldReRun = true
				break
			}
		}
	}

	combinedReason := fmt.Sprintf("combineMode=%s, results=%v -> shouldReRun=%t", combineMode, conditionReasons, shouldReRun)
	logger.Infof("reRunIf: final evaluation: %s", combinedReason)

	return shouldReRun, combinedReason, nil
}

// executeReRunIfFunctionCall executes a function call for reRunIf and returns its output
func (t *YAMLDefinedTool) executeReRunIfFunctionCall(
	ctx context.Context,
	messageID string,
	clientID string,
	funcName string,
	params map[string]interface{},
	callback tool_engine_models.AgenticWorkflowCallback,
) (string, error) {

	logger.Debugf("reRunIf: executing function call %s with params: %v", funcName, params)

	// Find the function in the tool
	var targetFunc *tool_protocol.Function
	for i := range t.tool.Functions {
		if t.tool.Functions[i].Name == funcName {
			targetFunc = &t.tool.Functions[i]
			break
		}
	}

	if targetFunc == nil {
		return "", fmt.Errorf("function '%s' not found in tool '%s'", funcName, t.tool.Name)
	}

	// Create a new tool instance for the function call
	funcTool := NewYAMLDefinedTool(
		t.inputFulfiller,
		t.tool,
		targetFunc,
		t.executionTracker,
		t.variableReplacer,
		t.toolEngine,
		t.workflowInitiator,
	)

	// Prepare input JSON
	inputJSON, err := json.Marshal(params)
	if err != nil {
		return "", fmt.Errorf("failed to marshal params: %w", err)
	}

	// Store params in callback for input fulfillment
	if callback != nil {
		callback.OnInputsFulfilled(ctx, messageID, t.tool.Name, funcName, params)
	}

	// Execute the function
	output, err := funcTool.Call(ctx, string(inputJSON))
	if err != nil {
		return "", fmt.Errorf("function call failed: %w", err)
	}

	return output, nil
}

// isTruthy determines if a value is truthy for reRunIf evaluation.
// This function handles both string values and JSON strings, using isTrueResult for parsed JSON objects.
// Returns true for: true, "true", non-zero numbers, non-empty strings (except "false", "0")
// Returns false for: false, "false", 0, "", nil, "0"
// For JSON objects: checks common keys like "result", "success", "ready", "done"
func isTruthy(value string) bool {
	value = strings.TrimSpace(value)

	// Empty string is falsy
	if value == "" {
		return false
	}

	// Try to parse as JSON first (handles objects like {"success": true})
	if strings.HasPrefix(value, "{") || strings.HasPrefix(value, "[") {
		var parsed interface{}
		if err := json.Unmarshal([]byte(value), &parsed); err == nil {
			// Use isTrueResult for parsed JSON (handles maps with common keys)
			return isTrueResult(parsed)
		}
	}

	// For plain strings, check explicit false values (case insensitive)
	lowerValue := strings.ToLower(value)
	if lowerValue == "false" || lowerValue == "0" || lowerValue == "null" || lowerValue == "nil" {
		return false
	}

	// Explicit true
	if lowerValue == "true" {
		return true
	}

	// Try to parse as number
	if num, err := strconv.ParseFloat(value, 64); err == nil {
		return num != 0
	}

	// Any other non-empty string is truthy
	return true
}

// handleRunOnlyIfError handles errors that occur during runOnlyIf evaluation
func (t *YAMLDefinedTool) handleRunOnlyIfError(
	ctx context.Context,
	messageID string,
	clientID string,
	onError *tool_protocol.OnError,
	originalErr error,
	callback tool_engine_models.AgenticWorkflowCallback,
	response string,
) (string, error) {

	logger.Infof("Handling runOnlyIf error with strategy: %s", onError.Strategy)

	switch onError.Strategy {
	case tool_protocol.OnErrorRequestUserInput:
		// Request user input to resolve the condition
		logger.Infof("Requesting user input for runOnlyIf condition")

		// Build the message to show to the user
		message := onError.Message
		if message == "" {
			message = fmt.Sprintf("The condition for %s was not met.", t.function.Name)
		}

		// If we have a response from the evaluation, include it for context
		if response != "" {
			message = fmt.Sprintf("%s\n\n%s", message, response)
		}

		description, err := t.inputFulfiller.RequestUserInput(ctx, messageID, "runOnlyIf_condition", message)
		if err != nil {
			logger.Errorf("Failed to request user input: %v", err)
			return "", err
		}
		return description, ErrInputFulfillmentPending

	case tool_protocol.OnErrorRequestN1Support,
		tool_protocol.OnErrorRequestN2Support,
		tool_protocol.OnErrorRequestN3Support,
		tool_protocol.OnErrorRequestApplicationSupport:
		// Escalate to support
		message := fmt.Sprintf("RunOnlyIf evaluation failed for %s.%s: %s. %s",
			t.tool.Name, t.function.Name, originalErr.Error(), onError.Message)
		return message, fmt.Errorf("not implemented: %w", originalErr)

	case tool_protocol.OnErrorSearch:
		// Attempt to resolve through search
		logger.Infof("Attempting to resolve runOnlyIf through search")
		// Implementation would go here
		return "", fmt.Errorf("search strategy not yet implemented for runOnlyIf: %w", originalErr)

	case tool_protocol.OnErrorInference:
		// Attempt to resolve through inference
		logger.Infof("Attempting to resolve runOnlyIf through inference")
		// Implementation would go here
		return "", fmt.Errorf("inference strategy not yet implemented for runOnlyIf: %w", originalErr)

	default:
		return "", fmt.Errorf("unsupported onError strategy '%s' for runOnlyIf: %w", onError.Strategy, originalErr)
	}
}

func (t *YAMLDefinedTool) generateConfirmationMessage(
	ctx context.Context,
	messageID string,
	inputs map[string]interface{},
) (string, error) {
	messages := thread.New()

	systemPrompt := fmt.Sprintf(`You are responsible for creating a clear, friendly message asking the user to confirm an operation.

The AI assistant is about to execute the function '%s' - '%s' with the following inputs:
%s

Your task is to create a confirmation message that:
1. Clearly explains what operation will be performed. Be human friendly. I mean, if the function is "get_weather", say "I will get the weather for you" instead of "I will execute the function get_weather".
2. Lists all the relevant details from the inputs in a human-friendly way
3. Be friendly and conversational
4. Ends with a clear request for confirmation
5. Do not output IDs to user because user is not able to confirm them. So, if you have the description (or label) for the input IDs, use them. Otherwise, just confirm the function execution (skipping the IDs)

Format the message so that it summarizes the operation to be performed and asks the user if they would like to proceed.
Output just the formatted message without any JSON wrapping or additional explanation.`,
		t.function.Name,
		t.function.Description,
		formatInputsForPrompt(inputs))

	messages.AddMessages(
		thread.NewSystemMessage().AddContent(
			thread.NewTextContent(systemPrompt),
		),
		thread.NewUserMessage().AddContent(
			thread.NewTextContent(fmt.Sprintf(`please output the confirmation message`)),
		),
	)

	llm := newLLM()

	for i := 0; i < 3; i++ {
		err := generateLLM(ctx, llm, messages)
		if err != nil {
			return "", fmt.Errorf("error generating confirmation message: %w", err)
		}

		if len(messages.LastMessage().Contents) > 0 {
			return messages.LastMessage().Contents[0].AsString(), nil
		}
	}

	// Fallback message if LLM generation fails
	return fmt.Sprintf("I'm about to %s with these details: %s. Would you like me to proceed?",
		HumanizeOperationName(t.function.Name),
		formatInputsForDisplay(inputs)), nil
}

func HumanizeOperationName(functionName string) string {
	// Add spaces before capital letters and convert to lowercase
	re := regexp.MustCompile(`([a-z0-9])([A-Z])`)
	return strings.ToLower(re.ReplaceAllString(functionName, "$1 $2"))
}

func formatInputsForDisplay(inputs map[string]interface{}) string {
	var items []string

	for key, value := range inputs {
		items = append(items, fmt.Sprintf("%s: %v", key, value))
	}

	return strings.Join(items, ", ")
}

func formatInputsForPrompt(inputs map[string]interface{}) string {
	var sb strings.Builder

	for key, value := range inputs {
		sb.WriteString(fmt.Sprintf("- %s: %v\n", key, value))
	}

	return sb.String()
}

// formatOutput formats the output according to the function's output definition
func (t *YAMLDefinedTool) formatOutput(ctx context.Context, messageID string, output string, stepResults map[int]interface{}) (string, error) {
	if t.function.Output == nil {
		return output, nil
	}

	switch t.function.Output.Type {
	case "string":
		// For string output, replace variables in the value field
		if t.function.Output.Value != "" {
			// Create a combined map of inputs and step results
			data := make(map[string]interface{})

			// Add step results as result[index]
			for index, result := range stepResults {
				data[fmt.Sprintf("result[%d]", index)] = result
			}

			// Replace variables
			return t.variableReplacer.ReplaceVariables(ctx, t.function.Output.Value, data)
		}
		return output, nil

	case "object":
		// For object output, generate a JSON object with the specified fields
		resultObj := make(map[string]interface{})

		for _, field := range t.function.Output.Fields {
			fieldName := field.Name
			if fieldName == "" {
				fieldName = field.Value
			}

			// Try to extract the field from step results
			// In a real implementation, this would be more sophisticated
			resultObj[fieldName] = fmt.Sprintf("Mock value for %s", fieldName)
		}

		// Serialize to JSON
		jsonBytes, err := json.MarshalIndent(resultObj, "", "  ")
		if err != nil {
			return "", fmt.Errorf("error serializing output to JSON: %w", err)
		}

		return string(jsonBytes), nil

	case "list[object]", "list[string]", "list[number]":
		// For list types, return a mock array
		// In a real implementation, this would properly format the output
		return fmt.Sprintf("Mock %s output for function '%s'",
			t.function.Output.Type, t.function.Name), nil

	default:
		return "", fmt.Errorf("unsupported output type: %s", t.function.Output.Type)
	}
}

// ParseJSONInputs converts any string JSON values in the inputs map to proper Go objects
func ParseJSONInputs(inputs map[string]interface{}) map[string]interface{} {
	if inputs == nil {
		return nil
	}

	result := make(map[string]interface{})

	for key, value := range inputs {
		strValue, ok := value.(string)
		if !ok {
			// Not a string, keep as is
			result[key] = value
			continue
		}

		trimmed := strings.TrimSpace(strValue)

		// Check if it looks like JSON (starts with { or [ and ends with } or ])
		if (strings.HasPrefix(trimmed, "{") && strings.HasSuffix(trimmed, "}")) ||
			(strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]")) {

			// First try to parse as an object
			var objValue map[string]interface{}
			err := json.Unmarshal([]byte(trimmed), &objValue)
			if err == nil {
				result[key] = objValue
				continue
			}

			// If not an object, try as an array
			var arrValue []interface{}
			err = json.Unmarshal([]byte(trimmed), &arrValue)
			if err == nil {
				result[key] = arrValue
				continue
			}

			// If parsing failed, keep original string
			result[key] = value
		} else {
			// Not JSON-formatted, keep original
			result[key] = value
		}
	}

	return result
}

// MapToString accepts either a map[int]interface{} or map[string]interface{}.
func MapToString[K int | string](m map[K]interface{}) string {
	if m == nil {
		return ""
	}

	var sb strings.Builder
	for key, value := range m {
		sb.WriteString(fmt.Sprintf("{%v}->%v\n", key, value))
	}
	return sb.String()
}

// Confirmation helper methods

// checkUserConfirmation checks if user has already confirmed this action
func (t *YAMLDefinedTool) checkUserConfirmation(
	ctx context.Context,
	clientID, toolName, functionName string,
	inputs map[string]interface{},
	ttlMinutes int,
) (bool, *confirmationState, error) {
	// In test mode, skip confirmation check - tests don't require user confirmation.
	// Zero-cost in production: TestMockServiceKey is never set outside test execution.
	if _, ok := ctx.Value(tool_engine_models.TestMockServiceKey).(tool_engine_models.IMockService); ok {
		logger.Infof("[TEST MODE] Skipping confirmation check for %s.%s - auto-confirming", toolName, functionName)
		return true, nil, nil
	}

	// Get repository safely
	inputFulfiller, ok := t.inputFulfiller.(*InputFulfiller)
	if !ok {
		return false, nil, errors.New("inputFulfiller is not of type *InputFulfiller")
	}
	if inputFulfiller.Repository == nil {
		return false, nil, errors.New("toolRepository not configured — confirmation state requires Repository")
	}

	// DEBUG: Log inputs before hashing
	inputsJSON, _ := json.Marshal(inputs)
	logger.Infof("[INPUTSHASH DEBUG] Calculating hash for %s.%s with inputs: %s (client=%s)",
		toolName, functionName, string(inputsJSON), clientID)

	// Generate hash of inputs for semantic matching
	inputsHash := generateInputsHash(inputs)

	logger.Infof("[INPUTSHASH DEBUG] Generated inputsHash: %s for %s.%s (client=%s)",
		inputsHash, toolName, functionName, clientID)

	// Check for existing confirmation state by hash
	state, err := inputFulfiller.Repository.GetConfirmationState(ctx, clientID, toolName, functionName, inputsHash)
	if err != nil && !errors.Is(err, errNotFound) {
		return false, nil, fmt.Errorf("failed to get confirmation state: %w", err)
	}

	// If not found by hash, try fallback: find ANY confirmation for this function (ignoring hash)
	// This handles the case where inputs were deserialized and the hash changed
	if errors.Is(err, errNotFound) {
		logger.Warnf("[INPUTSHASH DEBUG] ❌ NO ConfirmationState found for inputsHash=%s, %s.%s (client=%s) - trying FALLBACK lookup (ignoring hash)",
			inputsHash, toolName, functionName, clientID)

		state, err = inputFulfiller.Repository.FindConfirmationStateByFunction(ctx, clientID, toolName, functionName)
		if err != nil && !errors.Is(err, errNotFound) {
			return false, nil, fmt.Errorf("failed to find confirmation state by function: %w", err)
		}

		// Still no state found even with fallback
		if errors.Is(err, errNotFound) {
			logger.Warnf("[INPUTSHASH DEBUG] ❌ NO ConfirmationState found even with fallback for %s.%s (client=%s) - will create NEW confirmation request",
				toolName, functionName, clientID)
			return false, nil, nil
		}

		logger.Infof("[INPUTSHASH DEBUG] ✓ FALLBACK SUCCESS! Found ConfirmationState (ID=%d, InputsHash=%s, IsConfirmed=%v, CreatedAt=%s) for %s.%s (client=%s)",
			state.ID, state.InputsHash, state.IsConfirmed, state.CreatedAt.Format("2006-01-02 15:04:05"), toolName, functionName, clientID)
	} else {
		logger.Infof("[INPUTSHASH DEBUG] ✓ Found existing ConfirmationState (ID=%d, IsConfirmed=%v, CreatedAt=%s) for inputsHash=%s",
			state.ID, state.IsConfirmed, state.CreatedAt.Format("2006-01-02 15:04:05"), inputsHash)
	}

	// Check if confirmation is still valid (not expired)
	if time.Since(state.CreatedAt).Minutes() >= float64(ttlMinutes) {
		// Confirmation expired, delete it and return false
		if deleteErr := inputFulfiller.Repository.DeleteConfirmationState(ctx, state.ID); deleteErr != nil {
			logger.Warnf("Failed to delete expired confirmation state: %v", deleteErr)
		}
		return false, nil, nil
	}

	// State exists and is valid - check if user has confirmed
	return state.IsConfirmed, state, nil
}

// extractConfirmationFromChat tries to extract confirmation from recent chat messages
func (t *YAMLDefinedTool) extractConfirmationFromChat(
	ctx context.Context,
	messageID, confirmationText string,
) (bool, error) {
	// Get conversation history from context
	msgVal := ctx.Value(ConversationHistoryInContextKey)
	if msgVal == nil {
		logger.Warnf("[EXTRACT CONFIRMATION DEBUG] No conversation history in context!")
		return false, errors.New("no conversation history provided in context")
	}

	conversation, ok := msgVal.([]threadMessage)
	if !ok {
		logger.Warnf("[EXTRACT CONFIRMATION DEBUG] Cannot cast conversation history to []threadMessage")
		return false, errors.New("could not cast conversation history to []threadMessage")
	}

	logger.Infof("[EXTRACT CONFIRMATION DEBUG] Found %d messages in conversation history", len(conversation))

	// Get recent conversation history (last 5 groups of messages)
	history := getLatestNGroups(conversation, 5)
	conversationHistory := threadMessages(history).ConversationHistory()

	logger.Infof("[EXTRACT CONFIRMATION DEBUG] Latest 5 groups resulted in %d messages. ConversationHistory length: %d chars",
		len(history), len(conversationHistory))
	logger.Debugf("[EXTRACT CONFIRMATION DEBUG] ConversationHistory content:\n%s", conversationHistory)

	if conversationHistory == "" {
		logger.Warnf("[EXTRACT CONFIRMATION DEBUG] ConversationHistory is empty!")
		return false, errors.New("no recent user message found")
	}

	llm := newLLM()
	messages := thread.New()

	// Create the system prompt
	systemPrompt := fmt.Sprintf(`You are analyzing a conversation to determine if a user has confirmed an operation.

<confirmation_request>
%s
</confirmation_request>

<conversation_history>
%s
</conversation_history>

Task: Determine if the user has provided clear confirmation for the requested operation in their recent messages.

Look for:
- Explicit confirmations: "yes", "confirm", "proceed", "go ahead", "do it", "ok", "sure", "absolutely", etc...
- Explicit denials: "no", "cancel", "don't", "stop", "abort", "wait", "hold on", etc...
- Context-aware responses that clearly indicate agreement or disagreement

Output format:
{
  "confirmed": true/false,
  "confidence": 0-100,
  "response_type": "confirmed|denied|unclear",
  "explanation": "brief explanation of what the user said that led to this conclusion"
}

Only return "confirmed": true if you are highly confident (>80%%) that the user has explicitly confirmed the operation.`,
		confirmationText,
		conversationHistory)

	messages.AddMessages(
		thread.NewSystemMessage().AddContent(thread.NewTextContent(systemPrompt)),
		thread.NewUserMessage().AddContent(thread.NewTextContent("Analyze the conversation and output a valid JSON response with the required fields.")),
	)

	// Create a struct to unmarshal the response into
	type ConfirmationResponse struct {
		Confirmed    bool   `json:"confirmed"`
		Confidence   int    `json:"confidence"`
		ResponseType string `json:"response_type"`
		Explanation  string `json:"explanation"`
	}

	// Try up to 3 times to get a properly formatted response
	var result ConfirmationResponse
	for i := 0; i < 3; i++ {
		err := generateLLM(ctx, llm, messages)
		if err != nil {
			return false, fmt.Errorf("error generating LLM response: %w", err)
		}

		if len(messages.LastMessage().Contents) == 0 {
			return false, errors.New("no LLM response generated")
		}

		assistantResponse := messages.LastMessage().Contents[0].AsString()

		err = unmarshalAssistantResponse(ctx, assistantResponse, messages, &result)
		if err != nil {
			if errors.Is(err, errNoJSONContent) {
				// Add a message asking for proper JSON format
				messages.AddMessages(
					thread.NewUserMessage().AddContent(thread.NewTextContent(
						"Your response doesn't contain valid JSON. Please provide a proper JSON response with the exact format specified.")),
				)
				continue
			}
			return false, fmt.Errorf("error unmarshalling assistant response: %w", err)
		}

		break
	}

	logger.Infof("[EXTRACT CONFIRMATION DEBUG] LLM analysis result: confirmed=%v, confidence=%d, type=%s, explanation=%s",
		result.Confirmed, result.Confidence, result.ResponseType, result.Explanation)

	// Only confirm if we have high confidence
	if result.Confirmed && result.Confidence >= 80 {
		logger.Infof("[EXTRACT CONFIRMATION DEBUG] ✓ Confirmation ACCEPTED (confidence=%d >= 80)", result.Confidence)
		return true, nil
	}

	// If explicitly denied, return error
	if result.ResponseType == "denied" && result.Confidence >= 70 {
		logger.Warnf("[EXTRACT CONFIRMATION DEBUG] ✗ User DENIED operation: %s", result.Explanation)
		return false, fmt.Errorf("user denied the operation: %s", result.Explanation)
	}

	// Otherwise, no clear confirmation found
	logger.Warnf("[EXTRACT CONFIRMATION DEBUG] ✗ No clear confirmation (confirmed=%v, confidence=%d < 80)", result.Confirmed, result.Confidence)
	return false, nil
}

// checkTeamApproval checks if team approval has been granted for a tool execution
func (t *YAMLDefinedTool) checkTeamApproval(
	ctx context.Context,
	clientID, messageID, toolName, functionName, stepKey string,
	inputs map[string]interface{},
) (bool, *ExecutionCheckpoint, error) {
	sessionID := generateSessionID(clientID, messageID, toolName, functionName, stepKey)

	// Check if approval already exists
	checkpointDB, err := getCheckpointDB()
	if err != nil {
		return false, nil, fmt.Errorf("failed to get checkpoint DB: %w", err)
	}

	ckptData, err := checkpointDB.GetCheckpoint(ctx, sessionID)
	if err != nil && !errors.Is(err, errNotFound) {
		return false, nil, err
	}

	if ckptData != nil {
		// Convert to exported type
		exportedCheckpoint := &ExecutionCheckpoint{
			ID:           ckptData.ID,
			SessionID:    ckptData.SessionID,
			MessageID:    ckptData.MessageID,
			ClientID:     ckptData.ClientID,
			ToolName:     ckptData.ToolName,
			FunctionName: ckptData.FunctionName,
			StepKey:      ckptData.StepKey,
			Status:       CheckpointStatus(ckptData.Status),
			CreatedAt:    ckptData.CreatedAt,
			UpdatedAt:    ckptData.UpdatedAt,
		}
		// Check if approval was granted
		if ckptData.ApprovalGranted {
			// Cleanup checkpoint after use
			if deleteErr := checkpointDB.DeleteCheckpoint(ctx, sessionID); deleteErr != nil {
				logger.Warnf("Failed to delete used checkpoint: %v", deleteErr)
			}
			return true, exportedCheckpoint, nil
		}

		// Still waiting for approval
		return false, exportedCheckpoint, nil
	}

	// No existing checkpoint - need to create one
	return false, nil, nil
}

func (t *YAMLDefinedTool) generateUserMessageRequestingApproval(
	ctx context.Context,
	inputs map[string]interface{},
) (string, error) {
	messages := thread.New()

	retrievedMsg, ok := ctx.Value(MessageInContextKey).(Message)
	if !ok {
		return "", errors.New("could not get message from context")
	}

	var paramDetails []string
	for key, value := range inputs {
		if value == nil {
			continue
		}
		valStr := fmt.Sprintf("%v", value)

		paramDetails = append(paramDetails, fmt.Sprintf("- %s: %s", key, valStr))
	}

	var additionalInfo string
	if len(paramDetails) > 0 {
		additionalInfo = "with the following parameters:\n" + strings.Join(paramDetails, "\n")
	}

	systemPrompt := fmt.Sprintf(`You are responsible for creating a clear, professional and friendly message to the user that requests the approval of a function execution.

The assistant is requesting approval to execute the function '%s' that belongs to the tool '%s' %s

Your task is to:
1. Clearly explain that you need a approval or deny
2. Provide relevant context from the user's request.
3. Be professional and courteous.
4. Be direct and to the point.
5. If the required field is an ID but you have access to the label of the IDs, prioritize to send the labels for the users instead of unknown IDs.
6. Replace technical names to human friendly names when handling tools, functions and inputs. 

The user's original message was: "%s" 
and you must generate the message on the language of the user (%s). do not create fake info, use only the data provided
`,
		t.function.Name,
		t.tool.Name,
		additionalInfo,
		retrievedMsg.Body,
		i18nStub(ctx))

	messages.AddMessages(
		thread.NewSystemMessage().AddContent(thread.NewTextContent(systemPrompt)),
		thread.NewUserMessage().AddContent(thread.NewTextContent("Please generate a concise message to request the required information. use only the info that you have on the context, anything else. output just the message to be sent to the team.")),
	)

	llm := newLLM()

	var userMessage string
	for i := 0; i < 3; i++ {
		err := generateLLM(ctx, llm, messages)
		if err != nil {
			return "", fmt.Errorf("error generating user message: %w", err)
		}
		if len(messages.LastMessage().Contents) == 0 {
			return "", errors.New("no message generated by LLM")
		}
		userMessage = messages.LastMessage().Contents[0].AsString()
		if userMessage != "" {
			return userMessage, nil
		}
	}
	return "", errors.New("failed to generate user message after 3 attempts")
}

// createExecutionCheckpoint creates a checkpoint for the current execution state
func (t *YAMLDefinedTool) createExecutionCheckpoint(
	ctx context.Context,
	clientID, messageID, stepKey string,
	inputs map[string]interface{},
) (*ExecutionCheckpoint, error) {
	sessionID := generateSessionID(clientID, messageID, t.tool.Name, t.function.Name, stepKey)

	// Extract context information
	ctxSnapshot := t.extractContextSnapshot(ctx)
	wfParams := t.extractExecuteWorkflowParams(ctx)

	now := time.Now()

	// Create internal checkpoint data (richer than the exported type)
	ckptData := &executionCheckpointData{
		ID:                    fmt.Sprintf("ckpt_%d", now.UnixNano()),
		SessionID:             sessionID,
		MessageID:             messageID,
		ClientID:              clientID,
		ToolName:              t.tool.Name,
		FunctionName:          t.function.Name,
		StepKey:               stepKey,
		Status:                checkpointStatusPaused,
		ExecuteWorkflowParams: wfParams,
		ContextSnapshot:       ctxSnapshot,
		PauseReason:           "team_approval_required",
		ResumeCondition: resumeCondition{
			Type: "approval",
			Parameters: map[string]interface{}{
				"tool_name":     t.tool.Name,
				"function_name": t.function.Name,
				"inputs":        inputs,
			},
		},
		ApprovalRequested: true,
		Inputs:            inputs,
		CreatedAt:         now,
		UpdatedAt:         now,
	}

	// Save to checkpoint store
	checkpointDB, err := getCheckpointDB()
	if err != nil {
		return nil, fmt.Errorf("failed to get checkpoint DB: %w", err)
	}

	err = checkpointDB.SaveCheckpoint(ctx, ckptData)
	if err != nil {
		return nil, fmt.Errorf("failed to save checkpoint: %w", err)
	}

	// Return the exported type
	return &ExecutionCheckpoint{
		ID:           ckptData.ID,
		SessionID:    ckptData.SessionID,
		MessageID:    ckptData.MessageID,
		ClientID:     ckptData.ClientID,
		ToolName:     ckptData.ToolName,
		FunctionName: ckptData.FunctionName,
		StepKey:      ckptData.StepKey,
		Status:       CheckpointStatus(ckptData.Status),
		CreatedAt:    ckptData.CreatedAt,
		UpdatedAt:    ckptData.UpdatedAt,
	}, nil
}

// extractContextSnapshot extracts context information for checkpoint
func (t *YAMLDefinedTool) extractContextSnapshot(ctx context.Context) contextSnapshot {
	snapshot := contextSnapshot{
		AdditionalContext: make(map[string]interface{}),
	}

	// Extract standard context values
	if stepKey, ok := ctx.Value(StepKeyInContextKey).(string); ok {
		snapshot.StepKey = stepKey
	}

	if conversationType, ok := ctx.Value(ConversationTypeInContextKey).(string); ok {
		snapshot.ConversationType = conversationType
	}

	if messageData, ok := ctx.Value(MessageInContextKey).(Message); ok {
		snapshot.MessageData = messageData
	}

	if conversationHistory, ok := ctx.Value(ConversationHistoryInContextKey).([]threadMessage); ok {
		snapshot.ConversationHistory = conversationHistory
	}

	return snapshot
}

// extractExecuteWorkflowParams extracts workflow execution parameters
func (t *YAMLDefinedTool) extractExecuteWorkflowParams(ctx context.Context) executeWorkflowParams {
	params := executeWorkflowParams{
		ContextKeys: make(map[string]interface{}),
	}

	// Extract what we can from context - in a real implementation,
	// these would be passed down through the call chain
	if messageData, ok := ctx.Value(MessageInContextKey).(Message); ok {
		params.UserMessage = messageData.Id // Using ID for now since Message field doesn't exist
		// Note: contextForAgent and toolsAndFunctions would need to be
		// passed through context or stored in the coordinator state
	}

	return params
}

// getRepository safely gets the repository from the inputFulfiller.
// Returns an error if the repository is not configured (nil).
func (t *YAMLDefinedTool) getRepository() (toolRepository, error) {
	inputFulfiller, ok := t.inputFulfiller.(*InputFulfiller)
	if !ok {
		return nil, errors.New("inputFulfiller is not of type *InputFulfiller")
	}
	if inputFulfiller.Repository == nil {
		return nil, errors.New("toolRepository not configured — pass InputFulfillerOptions with Repository set")
	}
	return inputFulfiller.Repository, nil
}

// getDatabase safely gets the dataStore from the inputFulfiller.
// Returns an error if the database is not configured (nil).
func (t *YAMLDefinedTool) getDatabase() (dataStore, error) {
	inputFulfiller, ok := t.inputFulfiller.(*InputFulfiller)
	if !ok {
		return nil, errors.New("inputFulfiller is not of type *InputFulfiller")
	}
	if inputFulfiller.Database == nil {
		return nil, errors.New("dataStore not configured — pass InputFulfillerOptions with Database set")
	}
	return inputFulfiller.Database, nil
}

// ============================================
// HOOKS SYSTEM
// ============================================

// HookConfig represents a registered hook configuration
type HookConfig struct {
	CallbackFunction string // Can be "functionName" or "toolName.functionName"
	CallbackParams   string // JSON string of params
	TriggerType      string
	// Escalation-scoped hook fields
	Scope           string // "self" or "escalation"
	OwnerClientID   string // Hook owner's client_id (User A)
	EscalationID    int64  // human_qa_requests.id (0 for self-scoped)
	RequesterUserID string // User A's user.id (for initiate_workflow)
}

// getRegisteredHooks queries the database for active hooks matching the criteria
// It returns both self-scoped hooks (where client_id matches) and escalation-scoped hooks
// (where target_client_id matches the current caller)
func (t *YAMLDefinedTool) getRegisteredHooks(ctx context.Context, clientID, toolName, functionName, triggerType string) ([]HookConfig, error) {

	logger.Debugf("[HOOKS] getRegisteredHooks: tool=%s func=%s trigger=%s clientID=%s",
		toolName, functionName, triggerType, clientID)

	// Use the singleton database connection (same as DB operations use)
	db, err := getDB()
	if err != nil {
		return nil, err
	}

	// Query for both self-scoped and escalation-scoped hooks:
	// - Self-scoped: scope='self' AND client_id matches the caller
	// - Escalation-scoped: scope='escalation' AND target_client_id matches the caller
	query := `
		SELECT callback_function, callback_params, trigger_type,
		       COALESCE(scope, 'self'), client_id, COALESCE(escalation_id, 0),
		       COALESCE(requester_user_id, '')
		FROM hooks
		WHERE tool_name = ?
		  AND function_name = ?
		  AND trigger_type = ?
		  AND expires_at > datetime('now')
		  AND (
		    (COALESCE(scope, 'self') = 'self' AND client_id = ?)
		    OR
		    (scope = 'escalation' AND target_client_id = ?)
		  )
	`

	rows, err := db.QueryContext(ctx, query, toolName, functionName, triggerType, clientID, clientID)
	if err != nil {
		// Handle "no such table" error gracefully - table is created on first registerHook call
		if strings.Contains(err.Error(), "no such table") {
			return nil, nil // No hooks registered yet, return empty
		}
		return nil, fmt.Errorf("failed to query hooks: %w", err)
	}
	defer rows.Close()

	var hooks []HookConfig
	for rows.Next() {
		var h HookConfig
		var callbackParams sql.NullString
		if err := rows.Scan(&h.CallbackFunction, &callbackParams, &h.TriggerType,
			&h.Scope, &h.OwnerClientID, &h.EscalationID, &h.RequesterUserID); err != nil {
			logger.Warnf("[HOOKS] Error scanning hook row: %v", err)
			continue
		}
		h.CallbackParams = callbackParams.String
		hooks = append(hooks, h)
		logger.Debugf("[HOOKS] Found hook: callback=%s scope=%s escalationId=%d requesterUserId=%s",
			h.CallbackFunction, h.Scope, h.EscalationID, h.RequesterUserID)
	}
	logger.Debugf("[HOOKS] getRegisteredHooks returning %d hooks", len(hooks))
	return hooks, nil
}

// executeHooks finds and executes all registered hooks for the given trigger
// callbackFunction supports:
// - Simple name: "sendNotification" (searches current tool, then system functions)
// - Dot notation: "crm_tool.sendNotification" (explicit tool reference)
// - System functions: "registerKpi", "sendTeamMessage"
func (t *YAMLDefinedTool) executeHooks(
	ctx context.Context,
	messageID, clientID string,
	triggerType string,
	inputs map[string]interface{},
	output string,
	callback tool_engine_models.AgenticWorkflowCallback,
) error {

	// Debug logging for hook execution
	logger.Debugf("[HOOKS] executeHooks called for %s.%s trigger=%s clientID=%s",
		t.tool.Name, t.function.Name, triggerType, clientID)

	// Get current hook depth from context (defaults to 0)
	currentDepth := 0
	if depth, ok := ctx.Value(hookDepthContextKey{}).(int); ok {
		currentDepth = depth
	}

	// Check depth limit to prevent infinite recursion from circular hooks
	if currentDepth >= maxHookDepth {
		logger.Warnf("Hook execution depth limit reached (%d) for %s.%s trigger %s - possible circular hooks detected",
			maxHookDepth, t.tool.Name, t.function.Name, triggerType)
		return fmt.Errorf("hook execution depth limit (%d) exceeded - possible circular hooks", maxHookDepth)
	}

	hooks, err := t.getRegisteredHooks(ctx, clientID, t.tool.Name, t.function.Name, triggerType)
	if err != nil {
		logger.Warnf("Error fetching hooks for %s.%s trigger %s: %v",
			t.tool.Name, t.function.Name, triggerType, err)
		return nil // Don't fail main execution
	}

	if len(hooks) == 0 {
		logger.Debugf("[HOOKS] No hooks found for %s.%s trigger=%s clientID=%s",
			t.tool.Name, t.function.Name, triggerType, clientID)
		return nil
	}

	// Create context with incremented depth for nested hook executions
	hookCtx := context.WithValue(ctx, hookDepthContextKey{}, currentDepth+1)

	logger.Infof("Executing %d registered hooks for %s.%s trigger %s (depth: %d)",
		len(hooks), t.tool.Name, t.function.Name, triggerType, currentDepth+1)

	for _, hook := range hooks {
		// Build inputs with parent context + result
		hookInputs := make(map[string]interface{})
		for k, v := range inputs {
			hookInputs[k] = v
		}

		// Parse output as JSON if possible, otherwise use string
		if output != "" {
			if parsed, ok := tryParseJSON(output); ok {
				hookInputs["result"] = parsed
			} else {
				hookInputs["result"] = output
			}
		}
		hookInputs["_triggerType"] = triggerType
		hookInputs["_parentFunction"] = t.function.Name
		hookInputs["_parentTool"] = t.tool.Name

		// Inject escalation context for cross-user hooks
		if hook.Scope == "escalation" {
			// Inject variables for initiate_workflow callback
			// These become available as input params in the callback function
			hookInputs["hookEscalationId"] = hook.EscalationID
			hookInputs["hookRequesterUserId"] = hook.RequesterUserID // User A's user.id
			hookInputs["hookRequesterClientId"] = hook.OwnerClientID // User A's client_id
			hookInputs["hookTriggeredByClientId"] = clientID         // User B's client_id (current caller)
			hookInputs["hookScope"] = "escalation"

			logger.Infof("Executing escalation hook: requester=%s, responder=%s, escalation=%d",
				hook.RequesterUserID, clientID, hook.EscalationID)
		}

		// Merge callback params if provided
		if hook.CallbackParams != "" {
			var params map[string]interface{}
			if err := json.Unmarshal([]byte(hook.CallbackParams), &params); err == nil {
				for k, v := range params {
					hookInputs[k] = v
				}
			}
		}

		// Parse callbackFunction - supports dot notation "toolName.functionName" or just "functionName"
		callbackFuncName := hook.CallbackFunction
		targetTool := t.tool // Default: same tool

		if strings.Contains(callbackFuncName, ".") {
			parts := strings.SplitN(callbackFuncName, ".", 2)
			targetToolName := parts[0]
			callbackFuncName = parts[1]

			// Try to find the target tool by name
			if foundTool, found := t.toolEngine.GetToolByName(targetToolName, ""); found {
				targetTool = &foundTool
			} else {
				logger.Warnf("Callback tool '%s' not found, falling back to current tool", targetToolName)
			}
		}

		// Execute callback function using ExecuteDependencies (same pattern as onSuccess)
		// This properly handles system functions and tool functions
		// Use hookCtx to propagate the incremented depth for recursion protection
		startTime := time.Now()
		_, hookOutputSlice, execErr := t.toolEngine.ExecuteDependencies(
			hookCtx, messageID, clientID, targetTool,
			[]tool_protocol.NeedItem{{
				Name:   callbackFuncName,
				Params: hookInputs,
			}},
			t.inputFulfiller, callback, t.function.Name,
		)

		// Record hook execution to function_executions (even on error)
		var status string
		var recordOutput string
		if execErr != nil {
			status = tool_protocol.StatusFailed
			recordOutput = fmt.Sprintf(`{"error": %q, "trigger": %q, "parent": "%s.%s"}`,
				execErr.Error(), triggerType, t.tool.Name, t.function.Name)
			logger.Warnf("Hook callback '%s' failed: %v", hook.CallbackFunction, execErr)
		} else {
			status = tool_protocol.StatusComplete
			recordOutput = strings.Join(hookOutputSlice, "\n")
			logger.Infof("Hook callback '%s' executed successfully", hook.CallbackFunction)
		}

		// Record execution for audit trail
		if recordErr := t.executionTracker.RecordExecution(
			ctx, messageID, clientID,
			targetTool.Name, callbackFuncName,
			fmt.Sprintf("hook callback from %s.%s (%s)", t.tool.Name, t.function.Name, triggerType),
			hookInputs,
			"", // inputsHash - not needed for hooks
			recordOutput,
			nil, // originalOutput
			startTime,
			status,
			nil, // function pointer - not available for hooks
		); recordErr != nil {
			logger.Warnf("Failed to record hook execution: %v", recordErr)
		}
	}
	return nil
}

// executeAPIStep executes a single API step with a specific HTTP client
// Returns the raw output, request details for debugging, and any error
func (t *YAMLDefinedTool) executeAPIStep(ctx context.Context, messageID string, step tool_protocol.Step, httpClient HTTPClient) (interface{}, interface{}, error) {
	// Check for mock service in context (test mode only - zero-cost in production)
	// Production code NEVER sets TestMockServiceKey, so this check is always false
	if mockSvc, ok := ctx.Value(tool_engine_models.TestMockServiceKey).(tool_engine_models.IMockService); ok && mockSvc != nil {
		functionKey := GenerateFunctionKey(t.tool.Name, t.function.Name)
		if mockSvc.ShouldMock("api_call", functionKey, step.Name) {
			if response, exists := mockSvc.GetMockResponseValue(functionKey, step.Name); exists {
				mockSvc.RecordCall(functionKey, step.Name, "api_call", nil, response, "")
				logger.Infof("[TEST MOCK] Returning mock for api_call %s.%s", functionKey, step.Name)
				// Convert mock response to JSON string to match real API behavior
				// Real ExecuteAPICallOperation returns (string, ...) so we need to match that format
				if strResp, ok := response.(string); ok {
					return strResp, nil, nil
				}
				// If not already a string, convert to JSON
				jsonBytes, err := json.Marshal(response)
				if err != nil {
					logger.Errorf("[TEST MOCK] Failed to marshal mock response to JSON: %v", err)
					return response, nil, nil
				}
				return string(jsonBytes), nil, nil
			}
		}
	}

	if t.apiExecutor == nil {
		return nil, nil, fmt.Errorf("API executor not configured")
	}

	// Execute via the APIExecutor interface
	_, rawOutput, _, err := t.apiExecutor.ExecuteAPICall(
		ctx,
		step,
		nil, // inputs resolved from step.With
		nil, // headers
	)

	if err != nil {
		return rawOutput, nil, err
	}

	return rawOutput, nil, nil
}

// isAuthenticationError checks if the error or response indicates authentication is required
func (t *YAMLDefinedTool) isAuthenticationError(err error, result interface{}) bool {
	// Check error message for common auth indicators
	if err != nil {
		errStr := err.Error()
		authIndicators := []string{"401", "403", "unauthorized", "authentication", "login"}
		for _, indicator := range authIndicators {
			if strings.Contains(strings.ToLower(errStr), indicator) {
				return true
			}
		}
	}

	// Check if result contains HTML (likely a login page redirect)
	if strResult, ok := result.(string); ok {
		// Skip authentication detection for JSON responses
		trimmedResult := strings.TrimSpace(strResult)
		if strings.HasPrefix(trimmedResult, "[") && strings.HasSuffix(trimmedResult, "]") {
			// This looks like a JSON array response, not a login page
			return false
		}
		if strings.HasPrefix(trimmedResult, "{") && strings.HasSuffix(trimmedResult, "}") {
			// This looks like a JSON object response, not a login page
			return false
		}

		// Check for HTML structure first (more reliable)
		lowerResult := strings.ToLower(strResult)
		htmlStructureIndicators := []string{"<!doctype html", "<html", "<head>", "<body>"}

		hasHtmlStructure := false
		for _, indicator := range htmlStructureIndicators {
			if strings.Contains(lowerResult, indicator) {
				hasHtmlStructure = true
				break
			}
		}

		// Only check for login keywords if we have HTML structure
		if hasHtmlStructure {
			loginIndicators := []string{"login", "sign in", "entrar", "enviar", "submit", "senha", "password", "authentication"}
			for _, indicator := range loginIndicators {
				if strings.Contains(lowerResult, indicator) {
					return true
				}
			}
		}
	}

	return false
}

// saveCookiesFromClient extracts cookies from the HTTP client and saves them to the repository.
// NOTE: GetCookies is not part of the HTTPClient interface. Cookie management
// should be handled by the host application's HTTPClient implementation.
func (t *YAMLDefinedTool) saveCookiesFromClient(ctx context.Context, httpClient HTTPClient, cookieRepo cookieRepository) {
	// HTTPClient interface doesn't expose GetCookies — cookie persistence
	// must be managed by the host application's HTTP client.
	_ = ctx
	_ = httpClient
	_ = cookieRepo
}

// extractAuthTokenFromResult extracts an authentication token from the API response
// based on the extractAuthToken configuration in the step.
// Returns the token, TTL (in seconds), and any error
func (t *YAMLDefinedTool) extractAuthTokenFromResult(ctx context.Context, step tool_protocol.Step, result interface{}) (string, int, error) {

	// Get extractAuthToken config from step.With
	if step.With == nil {
		return "", 0, fmt.Errorf("step has no 'with' configuration")
	}

	extractAuthTokenConfig, ok := step.With[tool_protocol.StepWithExtractAuthToken]
	if !ok {
		return "", 0, fmt.Errorf("step has no extractAuthToken configuration")
	}

	// Parse the config
	configMap, ok := extractAuthTokenConfig.(map[string]interface{})
	if !ok {
		// Try map[interface{}]interface{}
		if interfaceMap, ok := extractAuthTokenConfig.(map[interface{}]interface{}); ok {
			configMap = make(map[string]interface{})
			for k, v := range interfaceMap {
				if kStr, ok := k.(string); ok {
					configMap[kStr] = v
				}
			}
		} else {
			return "", 0, fmt.Errorf("invalid extractAuthToken config format")
		}
	}

	// Get 'from' field
	fromVal, ok := configMap["from"]
	if !ok {
		return "", 0, fmt.Errorf("extractAuthToken missing 'from' field")
	}
	from, ok := fromVal.(string)
	if !ok {
		return "", 0, fmt.Errorf("extractAuthToken 'from' is not a string")
	}

	// Get 'key' field
	keyVal, ok := configMap["key"]
	if !ok {
		return "", 0, fmt.Errorf("extractAuthToken missing 'key' field")
	}
	key, ok := keyVal.(string)
	if !ok {
		return "", 0, fmt.Errorf("extractAuthToken 'key' is not a string")
	}

	// Get 'cache' field (default: 7200 = 2 hours)
	ttl := tool_protocol.ExtractAuthTokenDefaultCacheTTL
	if cacheVal, ok := configMap["cache"]; ok {
		switch v := cacheVal.(type) {
		case int:
			ttl = v
		case int64:
			ttl = int(v)
		case float64:
			ttl = int(v)
		}
	}

	var token string

	switch from {
	case tool_protocol.ExtractAuthTokenFromHeader:
		// Extract from RESPONSE headers (stored in result["_headers"])
		// Response headers are stored as map[string][]string in the result
		var responseHeaders map[string]interface{}

		// First, parse result to get _headers
		if strResult, ok := result.(string); ok {
			var parsedResult map[string]interface{}
			if err := json.Unmarshal([]byte(strResult), &parsedResult); err != nil {
				return "", 0, fmt.Errorf("failed to parse response as JSON to extract headers: %w", err)
			}
			if headers, ok := parsedResult["_headers"]; ok {
				if headersMap, ok := headers.(map[string]interface{}); ok {
					responseHeaders = headersMap
				}
			}
		} else if mapResult, ok := result.(map[string]interface{}); ok {
			if headers, ok := mapResult["_headers"]; ok {
				if headersMap, ok := headers.(map[string]interface{}); ok {
					responseHeaders = headersMap
				}
			}
		}

		if responseHeaders == nil {
			return "", 0, fmt.Errorf("no response headers available in result")
		}

		// Search for the header (case-insensitive)
		for headerName, headerValue := range responseHeaders {
			if strings.EqualFold(headerName, key) {
				// Header values can be []interface{} (from JSON) or []string
				switch v := headerValue.(type) {
				case []interface{}:
					if len(v) > 0 {
						if strVal, ok := v[0].(string); ok {
							token = strVal
						}
					}
				case []string:
					if len(v) > 0 {
						token = v[0]
					}
				case string:
					token = v
				}
				break
			}
		}
		if token == "" {
			return "", 0, fmt.Errorf("header '%s' not found in response headers", key)
		}

	case tool_protocol.ExtractAuthTokenFromResponseBody:
		// Extract from response body using JSON path
		var data interface{}
		if strResult, ok := result.(string); ok {
			if err := json.Unmarshal([]byte(strResult), &data); err != nil {
				return "", 0, fmt.Errorf("failed to parse response body as JSON: %w", err)
			}
		} else if mapResult, ok := result.(map[string]interface{}); ok {
			data = mapResult
		} else {
			return "", 0, fmt.Errorf("unexpected result type: %T", result)
		}

		// Navigate JSON path to get the token
		tokenValue, err := t.toolEngine.GetVariableReplacer().NavigatePath(data, key)
		if err != nil {
			return "", 0, fmt.Errorf("failed to navigate path '%s' in response: %w", key, err)
		}
		if tokenValue == nil {
			return "", 0, fmt.Errorf("path '%s' returned nil", key)
		}
		token, ok = tokenValue.(string)
		if !ok {
			return "", 0, fmt.Errorf("path '%s' did not return a string, got %T", key, tokenValue)
		}

	default:
		return "", 0, fmt.Errorf("invalid extractAuthToken 'from' value: %s", from)
	}

	// Strip "Bearer " prefix if present (case-insensitive)
	token = strings.TrimSpace(token)
	if strings.HasPrefix(strings.ToLower(token), "bearer ") {
		token = strings.TrimSpace(token[7:])
	}

	logger.Debugf("Extracted auth token (length: %d) from %s with key '%s', TTL: %d seconds", len(token), from, key, ttl)

	return token, ttl, nil
}

// isKeywordNegated checks if an error keyword appears only in negated contexts in the output.
// Returns true if ALL occurrences of the keyword are negated, meaning the keyword is not
// indicating an actual error. Negated contexts include:
//   - JSON fields set to false: "invalid":false, "error": false
//   - Zero counts: "0 failed", "0 errors"
//   - Explicit negation: "no error", "no errors", "not failed"
func isKeywordNegated(lowerOutput string, keyword string) bool {
	// Patterns where the keyword is negated (not an actual error)
	negatedPatterns := []string{
		// JSON boolean false: "keyword":false, "keyword": false
		`"` + keyword + `":false`,
		`"` + keyword + `": false`,
		`"` + keyword + `" : false`,
		// Zero count before keyword: "0 failed", "0 errors"
		"0 " + keyword,
		// Explicit negation
		"no " + keyword,
		"not " + keyword,
		"non-" + keyword,
		// Success context: "without error", "without failure"
		"without " + keyword,
	}

	for _, neg := range negatedPatterns {
		if strings.Contains(lowerOutput, neg) {
			// Verify that ALL occurrences are negated by removing negated matches
			// and checking if the keyword still appears
			cleaned := lowerOutput
			for _, n := range negatedPatterns {
				cleaned = strings.ReplaceAll(cleaned, n, "")
			}
			// If keyword no longer appears after removing negated contexts, it's fully negated
			pattern := regexp.MustCompile(`\b` + regexp.QuoteMeta(keyword) + `\b`)
			if !pattern.MatchString(cleaned) {
				return true
			}
			// Keyword still appears outside negated contexts — not fully negated
			return false
		}
	}
	return false
}

// detectErrorInOutput checks for common error patterns in the operation output
// Returns true if an error pattern is detected, false otherwise
func (t *YAMLDefinedTool) detectErrorInOutput(output string) bool {
	if output == "" {
		return false
	}

	// Convert to lowercase for case-insensitive matching
	lowerOutput := strings.ToLower(output)

	// Early return for successful API responses - don't scan user-generated content
	// (like hotel reviews) for error keywords when the API clearly succeeded
	if strings.HasPrefix(lowerOutput, `{"status":true`) || strings.HasPrefix(lowerOutput, `{"status": true`) {
		return false
	}

	// Early return for valid JSON structures (arrays/objects from DB operations).
	// These contain user-generated content embedded in JSON field values and should
	// not be scanned for error keywords (e.g., a Jira description saying "invalid password"
	// would cause a false positive). The output formatting step (Step 12) will handle
	// structured output properly — we should not short-circuit before it runs.
	trimmedOutput := strings.TrimSpace(output)
	if len(trimmedOutput) > 0 && (trimmedOutput[0] == '[' || trimmedOutput[0] == '{') {
		if json.Valid([]byte(trimmedOutput)) {
			return false
		}
	}

	// 1. Check for common error keywords using word boundaries
	// This prevents false positives like "invalida" (Portuguese verb) matching "invalid"
	errorKeywordsWithBoundary := []string{
		"error",
		"exception",
		"failed",
		"failure",
		"fatal",
		"panic",
		"invalid",
		"unauthorized",
		"forbidden",
		"timeout",
		"cannot",
	}

	for _, keyword := range errorKeywordsWithBoundary {
		// Use regex with word boundaries to avoid false positives
		pattern := regexp.MustCompile(`\b` + regexp.QuoteMeta(keyword) + `\b`)
		if pattern.MatchString(lowerOutput) {
			// Check if the match is in a negated context (false positive)
			if isKeywordNegated(lowerOutput, keyword) {
				continue
			}
			return true
		}
	}

	// 2. Multi-word phrases can use simple contains (less likely to have false positives)
	errorPhrases := []string{
		"not found",
		"bad request",
		"internal server error",
		"service unavailable",
		"timed out",
		"connection refused",
		"connection reset",
		"unable to",
		"could not",
		"permission denied",
		"access denied",
		"authentication failed",
		"authentication required",
	}

	for _, phrase := range errorPhrases {
		if strings.Contains(lowerOutput, phrase) {
			if isKeywordNegated(lowerOutput, phrase) {
				continue
			}
			return true
		}
	}

	// 2. Check for HTTP error status codes (400-599)
	httpErrorPatterns := []string{
		"status code: 4",
		"status code: 5",
		"status: 4",
		"status: 5",
		"http 4",
		"http 5",
		"400 bad request",
		"401 unauthorized",
		"403 forbidden",
		"404 not found",
		"405 method not allowed",
		"408 request timeout",
		"409 conflict",
		"422 unprocessable entity",
		"429 too many requests",
		"500 internal server error",
		"502 bad gateway",
		"503 service unavailable",
		"504 gateway timeout",
	}

	for _, pattern := range httpErrorPatterns {
		if strings.Contains(lowerOutput, pattern) {
			return true
		}
	}

	// 3. Check for JSON error structures
	jsonErrorPatterns := []string{
		`"error"`,
		`"errors"`,
		`"error_message"`,
		`"error_code"`,
		`"errorMessage"`,
		`"errorCode"`,
		`"success": false`,
		`"success":false`,
		`"status": "error"`,
		`"status":"error"`,
		`"status": "failed"`,
		`"status":"failed"`,
	}

	for _, pattern := range jsonErrorPatterns {
		if strings.Contains(lowerOutput, pattern) {
			return true
		}
	}

	// 4. Check for stack traces
	stackTraceIndicators := []string{
		"traceback",
		"stack trace",
		"at line",
		"at position",
		"\tat ",
		"raised exception",
	}

	for _, indicator := range stackTraceIndicators {
		if strings.Contains(lowerOutput, indicator) {
			return true
		}
	}

	// 5. Check for common exit codes in terminal operations
	if t.function.Operation == tool_protocol.OperationTerminal {
		exitCodePatterns := []string{
			"exit status 1",
			"exit code: 1",
			"exit code 1",
			"returned non-zero",
		}

		for _, pattern := range exitCodePatterns {
			if strings.Contains(lowerOutput, pattern) {
				return true
			}
		}
	}

	return false
}

func (t *YAMLDefinedTool) HasCacheConfiguration() bool {
	return t.function.ParsedCache != nil && t.function.ParsedCache.TTL > 0
}

// resolveMemoryFiltersYAML resolves variable placeholders in MemoryFilters for YAML-defined tools
// Returns nil if all fields are empty after resolution
func resolveMemoryFiltersYAML(
	ctx context.Context,
	filters *tool_protocol.MemoryFilters,
	variableReplacer tool_engine_models.IVariableReplacer,
	accumulatedInputs map[string]interface{},
) *tool_protocol.MemoryFilters {
	if filters == nil {
		return nil
	}

	result := &tool_protocol.MemoryFilters{}

	// Resolve topics (now an array)
	if len(filters.Topic) > 0 {
		resolvedTopics := make([]string, 0, len(filters.Topic))
		for _, topic := range filters.Topic {
			resolvedTopic, err := variableReplacer.ReplaceVariables(ctx, topic, accumulatedInputs)
			if err != nil {
				logger.Warnf("Failed to resolve memoryFilters.topic variable '%s': %v", topic, err)
				resolvedTopics = append(resolvedTopics, topic) // Keep original if resolution fails
			} else if resolvedTopic != "" && !strings.HasPrefix(resolvedTopic, "$") {
				resolvedTopics = append(resolvedTopics, resolvedTopic)
			} else {
				logger.Warnf("memoryFilters.topic references unavailable or empty variable '%s' (skipping)", topic)
			}
		}
		result.Topic = resolvedTopics
	}

	// Resolve metadata fields
	if len(filters.Metadata) > 0 {
		result.Metadata = make(map[string]interface{})
		for key, value := range filters.Metadata {
			switch v := value.(type) {
			case string:
				// Try to resolve string variables
				resolvedValue, err := variableReplacer.ReplaceVariables(ctx, v, accumulatedInputs)
				if err != nil {
					logger.Warnf("Failed to resolve memoryFilters.metadata.%s variable '%s': %v", key, v, err)
					result.Metadata[key] = v // Keep original if resolution fails
				} else if resolvedValue != "" && !strings.HasPrefix(resolvedValue, "$") {
					// Only add if variable was actually resolved (not empty and not still a $var)
					result.Metadata[key] = resolvedValue
				} else {
					logger.Warnf("memoryFilters.metadata.%s references unavailable or empty variable '%s' (skipping)", key, v)
				}
			case *tool_protocol.MetadataFilterValue:
				// Object form with value and operation - resolve the value
				resolvedValue, err := variableReplacer.ReplaceVariables(ctx, v.Value, accumulatedInputs)
				if err != nil {
					logger.Warnf("Failed to resolve memoryFilters.metadata.%s.value variable '%s': %v", key, v.Value, err)
					result.Metadata[key] = v // Keep original if resolution fails
				} else if resolvedValue != "" && !strings.HasPrefix(resolvedValue, "$") {
					// Create new MetadataFilterValue with resolved value
					result.Metadata[key] = &tool_protocol.MetadataFilterValue{
						Value:     resolvedValue,
						Operation: v.Operation,
					}
				} else {
					logger.Warnf("memoryFilters.metadata.%s.value references unavailable or empty variable '%s' (skipping)", key, v.Value)
				}
			default:
				// Other types (int, bool, etc.) - use as-is
				result.Metadata[key] = value
			}
		}
	}

	// Resolve timeRange fields
	if filters.TimeRange != nil {
		result.TimeRange = &tool_protocol.MemoryTimeRange{}

		// Resolve 'after' field
		if filters.TimeRange.After != "" {
			resolvedAfter, err := variableReplacer.ReplaceVariables(ctx, filters.TimeRange.After, accumulatedInputs)
			if err != nil {
				logger.Warnf("Failed to resolve memoryFilters.timeRange.after variable '%s': %v", filters.TimeRange.After, err)
			} else if resolvedAfter != "" && !strings.HasPrefix(resolvedAfter, "$") {
				// Only set if variable was actually resolved (not empty) and is in valid format
				if tool_protocol.IsValidMemoryDateFormat(resolvedAfter) {
					result.TimeRange.After = resolvedAfter
				} else {
					logger.Warnf("memoryFilters.timeRange.after resolved to invalid date format '%s' (expected YYYY-MM-DD, skipping)", resolvedAfter)
				}
			} else {
				logger.Warnf("memoryFilters.timeRange.after references unavailable or empty variable '%s' (skipping)", filters.TimeRange.After)
			}
		}

		// Resolve 'before' field
		if filters.TimeRange.Before != "" {
			resolvedBefore, err := variableReplacer.ReplaceVariables(ctx, filters.TimeRange.Before, accumulatedInputs)
			if err != nil {
				logger.Warnf("Failed to resolve memoryFilters.timeRange.before variable '%s': %v", filters.TimeRange.Before, err)
			} else if resolvedBefore != "" && !strings.HasPrefix(resolvedBefore, "$") {
				// Only set if variable was actually resolved (not empty) and is in valid format
				if tool_protocol.IsValidMemoryDateFormat(resolvedBefore) {
					result.TimeRange.Before = resolvedBefore
				} else {
					logger.Warnf("memoryFilters.timeRange.before resolved to invalid date format '%s' (expected YYYY-MM-DD, skipping)", resolvedBefore)
				}
			} else {
				logger.Warnf("memoryFilters.timeRange.before references unavailable or empty variable '%s' (skipping)", filters.TimeRange.Before)
			}
		}

		// If both timeRange fields are empty after resolution, set TimeRange to nil
		if result.TimeRange.After == "" && result.TimeRange.Before == "" {
			result.TimeRange = nil
		}
	}

	// Return nil if everything is empty
	if len(result.Topic) == 0 && len(result.Metadata) == 0 && result.TimeRange == nil {
		return nil
	}

	return result
}

// executePDFOperation executes a PDF generation operation
func (t *YAMLDefinedTool) executePDFOperation(ctx context.Context, messageID string, inputs map[string]interface{}) (string, map[int]interface{}, error) {

	logger.Debugf("Executing PDF operation for %s.%s", t.tool.Name, t.function.Name)

	if t.function.PDF == nil {
		return "", nil, fmt.Errorf("PDF configuration is missing")
	}

	// Create PDF generator
	pdfGen := pdf.NewPDFGenerator(t.variableReplacer)

	// Generate PDF bytes and FileResult
	pdfBytes, fileResult, err := pdfGen.GeneratePDF(ctx, t.function.PDF, inputs)
	if err != nil {
		logger.Errorf("Failed to generate PDF: %v", err)
		return fmt.Sprintf("Error generating PDF: %v", err), nil, err
	}

	logger.Debugf("PDF generated successfully: %s (%d bytes)", fileResult.FileName, len(pdfBytes))

	company := getCompanyInstance()

	// Determine retention (default: 600 seconds)
	retention := tool_protocol.DefaultFileRetention
	if t.function.Output != nil && t.function.Output.Retention > 0 {
		retention = t.function.Output.Retention
	}

	// Save to temp file if TempFileManager is available
	if tempManager := t.toolEngine.GetTempFileManager(); tempManager != nil {
		tempPath, err := tempManager.SaveTempFile(company.ID, fileResult.FileName, pdfBytes, retention)
		if err != nil {
			logger.Warnf("Failed to save temp file: %v", err)
		} else {
			fileResult.TempPath = tempPath
			logger.Debugf("PDF saved to temp file: %s (retention: %ds)", tempPath, retention)
		}
	}

	// Determine if we should upload
	// Default: NO upload (base64 available via variable replacer for other functions)
	// Only upload if explicitly configured with output.upload: true
	shouldUpload := false
	if t.function.Output != nil && t.function.Output.Upload {
		shouldUpload = true
	}

	// Upload to agent proxy if configured
	if shouldUpload {
		agentProxyClient := newAgentProxyClient()
		pdfURL, err := agentProxyClient.UploadPDFFile(ctx, pdfBytes, fileResult.FileName, company.ID)
		if err != nil {
			logger.Errorf("Failed to upload PDF: %v", err)
			return fmt.Sprintf("Error uploading PDF: %v", err), nil, err
		}

		fileResult.URL = pdfURL
		logger.Debugf("PDF uploaded successfully: %s", pdfURL)
	}

	// Add to filesToShare regardless of upload — moved OUTSIDE the upload block
	// This allows files to be sent to users even without uploading to agent-proxy (via base64 fallback)
	if t.function.ShouldBeHandledAsMessageToUser {
		mediaType := FileResultToMediaType(fileResult)
		if mediaType != nil {
			// If no URL (upload skipped/failed), populate Base64Data as fallback
			if mediaType.Url == "" && fileResult.TempPath != "" {
				fileBytes, readErr := os.ReadFile(fileResult.TempPath)
				if readErr == nil {
					mediaType.Base64Data = base64.StdEncoding.EncodeToString(fileBytes)
				} else {
					logger.Warnf("Failed to read temp file for base64 fallback: %v", readErr)
				}
			}
			t.executionTracker.AddFileToShare(messageID, mediaType)
			logger.Debugf("PDF file added to share for message %s: %s", messageID, fileResult.FileName)
		}
	}

	// Store FileResult in stepResults[1] for variable replacer access
	stepResults := map[int]interface{}{
		1: fileResult,
	}

	// Return the full FileResult as JSON so other functions can access all fields
	// including tempPath (needed for $FunctionName.base64 access)
	fileResultJSON, err := json.Marshal(fileResult)
	if err != nil {
		// Fallback to simple format if marshaling fails
		logger.Warnf("Failed to marshal FileResult to JSON: %v", err)
		return fmt.Sprintf(`{"fileName":"%s","mimeType":"%s","size":%d}`,
			fileResult.FileName, fileResult.MimeType, fileResult.Size), stepResults, nil
	}

	return string(fileResultJSON), stepResults, nil
}

// processStepAsFileDownload converts an api_call step's raw HTTP response bytes into a FileResult.
// It handles MIME type detection, file size validation, temp file storage, upload to agent-proxy,
// and adding to filesToShare for shouldBeHandledAsMessageToUser functions.
func (t *YAMLDefinedTool) processStepAsFileDownload(ctx context.Context, messageID string, step tool_protocol.Step, requestDetails interface{}, inputs map[string]interface{}) (*tool_protocol.FileResult, error) {

	config := step.SaveAsFile

	details, ok := requestDetails.(*apiRequestDetails)
	if !ok || details == nil || len(details.RawResponseBytes) == 0 {
		return nil, fmt.Errorf("saveAsFile: no response bytes available from step '%s'", step.Name)
	}

	rawBytes := details.RawResponseBytes

	// 1. Enforce file size limit
	maxSize := config.MaxFileSize
	if maxSize <= 0 {
		maxSize = tool_protocol.MaxFileSizeDefault
	}
	if int64(len(rawBytes)) > maxSize {
		return nil, fmt.Errorf("saveAsFile: response size (%d bytes) exceeds max file size (%d bytes) for step '%s'",
			len(rawBytes), maxSize, step.Name)
	}

	// 2. Resolve filename via variable replacer
	fileName := config.FileName
	if vr := t.toolEngine.GetVariableReplacer(); vr != nil {
		resolved, err := vr.ReplaceVariables(ctx, fileName, inputs)
		if err != nil {
			logger.Warnf("saveAsFile: failed to resolve fileName variables: %v", err)
		} else {
			fileName = resolved
		}
	}
	if fileName == "" {
		fileName = "download"
	}

	// 3. Detect MIME type: explicit config > response Content-Type > file extension > http.DetectContentType
	mimeType := config.MimeType
	if mimeType == "" && details.ResponseContentType != "" {
		// Parse Content-Type to strip parameters (e.g., "application/pdf; charset=utf-8" -> "application/pdf")
		mediaType, _, _ := mime.ParseMediaType(details.ResponseContentType)
		if mediaType != "" {
			mimeType = mediaType
		} else {
			mimeType = details.ResponseContentType
		}
	}
	if mimeType == "" {
		// Try extension-based detection
		ext := filepath.Ext(fileName)
		if ext != "" {
			mimeType = mime.TypeByExtension(ext)
		}
	}
	if mimeType == "" {
		// Fallback to content sniffing
		mimeType = http.DetectContentType(rawBytes)
	}

	// 4. Validate Content-Type - warn on suspicious types
	suspiciousTypes := map[string]bool{
		"text/html":              true,
		"application/javascript": true,
	}
	if suspiciousTypes[mimeType] {
		logger.Warnf("saveAsFile: suspicious Content-Type '%s' for file download in step '%s' — expected a binary file type", mimeType, step.Name)
	}

	// 5. Create FileResult
	fileResult := &tool_protocol.FileResult{
		FileName: fileName,
		MimeType: mimeType,
		Size:     int64(len(rawBytes)),
	}

	company := getCompanyInstance()

	// 6. Determine retention
	retention := tool_protocol.DefaultFileRetention
	if t.function.Output != nil && t.function.Output.Retention > 0 {
		retention = t.function.Output.Retention
	}

	// 7. Save to temp file
	if tempManager := t.toolEngine.GetTempFileManager(); tempManager != nil {
		tempPath, err := tempManager.SaveTempFile(company.ID, fileResult.FileName, rawBytes, retention)
		if err != nil {
			logger.Warnf("saveAsFile: failed to save temp file: %v", err)
		} else {
			fileResult.TempPath = tempPath
			logger.Debugf("saveAsFile: file saved to temp: %s (retention: %ds)", tempPath, retention)
		}
	}

	// 8. Upload to agent proxy if configured
	shouldUpload := t.function.Output != nil && t.function.Output.Upload
	if shouldUpload {
		agentProxyClient := newAgentProxyClient()
		fileURL, err := agentProxyClient.UploadPDFFile(ctx, rawBytes, fileResult.FileName, company.ID)
		if err != nil {
			logger.Errorf("saveAsFile: failed to upload file: %v", err)
			// Don't fail the whole operation — file is still available locally
		} else {
			fileResult.URL = fileURL
			logger.Debugf("saveAsFile: file uploaded successfully: %s", fileURL)
		}
	}

	// 9. Add to filesToShare if shouldBeHandledAsMessageToUser
	if t.function.ShouldBeHandledAsMessageToUser {
		mediaType := FileResultToMediaType(fileResult)
		if mediaType != nil {
			// If no URL (upload skipped/failed), populate Base64Data as fallback
			if mediaType.Url == "" && fileResult.TempPath != "" {
				fileBytes, err := os.ReadFile(fileResult.TempPath)
				if err == nil {
					mediaType.Base64Data = base64.StdEncoding.EncodeToString(fileBytes)
				} else {
					logger.Warnf("saveAsFile: failed to read temp file for base64 fallback: %v", err)
				}
			}
			t.executionTracker.AddFileToShare(messageID, mediaType)
			logger.Debugf("saveAsFile: file added to share for message %s: %s", messageID, fileResult.FileName)
		}
	}

	logger.Debugf("saveAsFile: completed for step '%s' — file: %s, mime: %s, size: %d bytes",
		step.Name, fileResult.FileName, fileResult.MimeType, fileResult.Size)

	return fileResult, nil
}

// executeGDriveOperation executes a gdrive operation with its step loop.
// Follows the same pattern as executeCodeOperation (retry skip, runOnlyIf, forEach, variable replacement).
func (t *YAMLDefinedTool) executeGDriveOperation(ctx context.Context, messageID string, inputs map[string]interface{}, existingStepResults map[int]interface{}) (string, map[int]interface{}, error) {

	logger.Debugf("Executing gdrive operation for %s.%s", t.tool.Name, t.function.Name)

	// Create GDrive client
	gdriveClient, err := t.createGDriveClient(ctx)
	if err != nil {
		return fmt.Sprintf("Error creating GDrive client: %v", err), nil, err
	}

	// Use existing step results if provided (for retry scenarios)
	stepResults := existingStepResults
	if stepResults == nil {
		stepResults = make(map[int]interface{})
	}
	var lastOutput string

	for _, step := range t.function.Steps {
		// Skip step if it already has a result from a previous retry attempt
		if step.ResultIndex > 0 {
			if _, hasResult := stepResults[step.ResultIndex]; hasResult {
				logger.Infof("Skipping gdrive step '%s' - already completed in previous attempt (result at index %d)",
					step.Name, step.ResultIndex)
				continue
			}
		}

		// Evaluate runOnlyIf condition
		if step.RunOnlyIf != nil {
			shouldRun, skipInfo, err := t.evaluateStepRunOnlyIf(ctx, messageID, step, inputs, stepResults)
			if err != nil {
				return "", stepResults, fmt.Errorf("error evaluating runOnlyIf for gdrive step '%s': %w", step.Name, err)
			}
			if !shouldRun {
				logger.Infof("Skipping gdrive step '%s' due to runOnlyIf condition: %s", step.Name, skipInfo.Reason)
				if step.ResultIndex > 0 {
					skipJSON, _ := json.Marshal(skipInfo)
					stepResults[step.ResultIndex] = string(skipJSON)
				}
				continue
			}
		}

		// Process step parameters (standard variant)
		processedParams, err := t.processStepParameters(ctx, messageID, step.With, inputs, stepResults)
		if err != nil {
			return "", stepResults, fmt.Errorf("error processing step parameters for gdrive step '%s': %w", step.Name, err)
		}

		// Handle forEach
		if step.ForEach != nil {
			forEachResult, forEachErr := t.executeForEachGDriveStep(ctx, messageID, step, processedParams, inputs, stepResults, gdriveClient)
			if forEachErr != nil {
				return "", stepResults, fmt.Errorf("error in forEach for gdrive step '%s': %w", step.Name, forEachErr)
			}
			if step.ResultIndex > 0 {
				stepResults[step.ResultIndex] = forEachResult
			}
			if strResult, ok := forEachResult.(string); ok {
				lastOutput = strResult
			}
			continue
		}

		result, err := t.executeGDriveStep(ctx, messageID, step, processedParams, gdriveClient)
		if err != nil {
			t.recordFailedStepExecution(ctx, messageID, step.Name, err.Error(), "", inputs)
			return "", stepResults, fmt.Errorf("error executing gdrive step '%s': %w", step.Name, err)
		}

		if step.ResultIndex > 0 {
			stepResults[step.ResultIndex] = result
		}

		if strResult, ok := result.(string); ok {
			lastOutput = strResult
			logger.Infof("GDrive step '%s' completed (result length: %d)", step.Name, len(strResult))
		}
	}

	return lastOutput, stepResults, nil
}

// executeGDriveStep dispatches a single gdrive step to the appropriate client method.
func (t *YAMLDefinedTool) executeGDriveStep(ctx context.Context, messageID string, step tool_protocol.Step, params map[string]interface{}, client GDriveClient) (interface{}, error) {
	switch step.Action {
	case tool_protocol.StepActionGDriveList:
		results, err := client.List(ctx, params)
		if err != nil {
			return nil, err
		}
		return marshalGDriveResult(results)

	case tool_protocol.StepActionGDriveUpload:
		result, err := client.Upload(ctx, params)
		if err != nil {
			return nil, err
		}
		return marshalGDriveResult(result)

	case tool_protocol.StepActionGDriveDownload:
		return t.handleGDriveFileResult(ctx, messageID, func() ([]byte, *tool_protocol.FileResult, error) {
			return client.Download(ctx, params)
		})

	case tool_protocol.StepActionGDriveCreateFolder:
		result, err := client.CreateFolder(ctx, params)
		if err != nil {
			return nil, err
		}
		return marshalGDriveResult(result)

	case tool_protocol.StepActionGDriveDelete:
		result, err := client.Delete(ctx, params)
		if err != nil {
			return nil, err
		}
		return marshalGDriveResult(result)

	case tool_protocol.StepActionGDriveMove:
		result, err := client.Move(ctx, params)
		if err != nil {
			return nil, err
		}
		return marshalGDriveResult(result)

	case tool_protocol.StepActionGDriveSearch:
		results, err := client.Search(ctx, params)
		if err != nil {
			return nil, err
		}
		return marshalGDriveResult(results)

	case tool_protocol.StepActionGDriveGetMetadata:
		result, err := client.GetMetadata(ctx, params)
		if err != nil {
			return nil, err
		}
		return marshalGDriveResult(result)

	case tool_protocol.StepActionGDriveUpdate:
		result, err := client.Update(ctx, params)
		if err != nil {
			return nil, err
		}
		return marshalGDriveResult(result)

	case tool_protocol.StepActionGDriveExport:
		return t.handleGDriveFileResult(ctx, messageID, func() ([]byte, *tool_protocol.FileResult, error) {
			return client.Export(ctx, params)
		})

	default:
		logger.Errorf("Unknown gdrive action: %s", step.Action)
		return nil, fmt.Errorf("unknown gdrive action: %s", step.Action)
	}
}

// handleGDriveFileResult processes download/export results through the FileResult pipeline.
// Same pattern as executePDFOperation: temp file → optional upload → filesToShare.
func (t *YAMLDefinedTool) handleGDriveFileResult(ctx context.Context, messageID string, fetchFn func() ([]byte, *tool_protocol.FileResult, error)) (interface{}, error) {

	data, fileResult, err := fetchFn()
	if err != nil {
		return nil, err
	}

	logger.Debugf("GDrive file fetched: %s (%d bytes, %s)", fileResult.FileName, len(data), fileResult.MimeType)

	company := getCompanyInstance()

	// Determine retention
	retention := tool_protocol.DefaultFileRetention
	if t.function.Output != nil && t.function.Output.Retention > 0 {
		retention = t.function.Output.Retention
	}

	// Save to temp file
	if tempManager := t.toolEngine.GetTempFileManager(); tempManager != nil {
		tempPath, err := tempManager.SaveTempFile(company.ID, fileResult.FileName, data, retention)
		if err != nil {
			logger.Warnf("Failed to save gdrive temp file: %v", err)
		} else {
			fileResult.TempPath = tempPath
			logger.Debugf("GDrive file saved to temp: %s (retention: %ds)", tempPath, retention)
		}
	}

	// Upload to agent-proxy if configured
	if t.function.Output != nil && t.function.Output.Upload {
		agentProxyClient := newAgentProxyClient()
		fileURL, err := agentProxyClient.UploadPDFFile(ctx, data, fileResult.FileName, company.ID)
		if err != nil {
			logger.Errorf("Failed to upload gdrive file: %v", err)
			// Don't fail the whole operation — file is still available locally via temp path
		} else {
			fileResult.URL = fileURL
			logger.Debugf("GDrive file uploaded: %s", fileURL)
		}
	}

	// Add to filesToShare for message delivery
	if t.function.ShouldBeHandledAsMessageToUser {
		mediaType := FileResultToMediaType(fileResult)
		if mediaType != nil {
			if mediaType.Url == "" && fileResult.TempPath != "" {
				fileBytes, readErr := os.ReadFile(fileResult.TempPath)
				if readErr == nil {
					mediaType.Base64Data = base64.StdEncoding.EncodeToString(fileBytes)
				} else {
					logger.Warnf("Failed to read temp file for base64 fallback: %v", readErr)
				}
			}
			t.executionTracker.AddFileToShare(messageID, mediaType)
			logger.Debugf("GDrive file added to share for message %s: %s", messageID, fileResult.FileName)
		}
	}

	// Return FileResult as JSON
	fileResultJSON, err := json.Marshal(fileResult)
	if err != nil {
		logger.Warnf("Failed to marshal GDrive FileResult: %v", err)
		fallback := map[string]interface{}{
			"fileName": fileResult.FileName,
			"mimeType": fileResult.MimeType,
			"size":     fileResult.Size,
		}
		if fb, fbErr := json.Marshal(fallback); fbErr == nil {
			return string(fb), nil
		}
		return fmt.Sprintf(`{"error":"failed to marshal file result"}`), nil
	}

	return string(fileResultJSON), nil
}

// marshalGDriveResult converts a result to JSON string for step results storage.
func marshalGDriveResult(result interface{}) (interface{}, error) {
	jsonBytes, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal result: %w", err)
	}
	return string(jsonBytes), nil
}

// executeForEachGDriveStep handles forEach iteration for gdrive steps.
func (t *YAMLDefinedTool) executeForEachGDriveStep(ctx context.Context, messageID string, step tool_protocol.Step, params map[string]interface{}, inputs map[string]interface{}, stepResults map[int]interface{}, gdriveClient GDriveClient) (interface{}, error) {
	forEach := step.ForEach

	separator := forEach.Separator
	if separator == "" {
		separator = tool_protocol.DefaultForEachSeparator
	}
	indexVar := forEach.IndexVar
	if indexVar == "" {
		indexVar = tool_protocol.DefaultForEachIndexVar
	}
	itemVar := forEach.ItemVar
	if itemVar == "" {
		itemVar = tool_protocol.DefaultForEachItemVar
	}

	// Get the items to iterate over
	itemsStr, ok := params[tool_protocol.ForEachItems].(string)
	if !ok {
		replaced, err := t.variableReplacer.ReplaceVariables(ctx, forEach.Items, inputs)
		if err != nil {
			return nil, fmt.Errorf("error resolving forEach items: %w", err)
		}
		itemsStr = replaced
	}

	items := strings.Split(itemsStr, separator)
	results := make([]interface{}, 0, len(items))

	for idx, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}

		// Create per-iteration params with index/item vars
		iterParams := make(map[string]interface{})
		for k, v := range params {
			iterParams[k] = v
		}
		iterParams[indexVar] = idx
		iterParams[itemVar] = item

		// Re-process step parameters for this iteration
		iterProcessed, err := t.processStepParameters(ctx, messageID, step.With, iterParams, stepResults)
		if err != nil {
			return nil, fmt.Errorf("error processing forEach iteration %d: %w", idx, err)
		}

		result, err := t.executeGDriveStep(ctx, messageID, step, iterProcessed, gdriveClient)
		if err != nil {
			return nil, fmt.Errorf("error in forEach iteration %d: %w", idx, err)
		}

		results = append(results, result)
	}

	return marshalGDriveResult(results)
}
