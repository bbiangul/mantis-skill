// Package engine implements a system for dynamically creating and executing
// agent tools based on YAML definitions.
package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/bbiangul/mantis-skill/engine/models"
	"github.com/bbiangul/mantis-skill/engine/utils"
	"github.com/bbiangul/mantis-skill/skill"
	"github.com/bbiangul/mantis-skill/types"

	"github.com/santhosh-tekuri/jsonschema"
)

// Ensure InputFulfiller satisfies the IInputFulfiller interface.
var _ models.IInputFulfiller = (*InputFulfiller)(nil)

// Common errors
var (
	ErrInputOriginNotImplemented     = errors.New("input origin not implemented")
	ErrOnErrorStrategyNotImplemented = errors.New("on error strategy not implemented")
	ErrRequiredInputMissing          = errors.New("required input is missing")
	ErrFunctionNotFound              = errors.New("function not found")
	ErrInputFulfillmentPending       = errors.New("input fulfillment is pending")
)

// RegexValidationError represents a value that failed regex validation.
type RegexValidationError struct {
	ReceivedValue string
	Pattern       string
	Explanation   string
	Err           error
}

func (e *RegexValidationError) Error() string { return e.Err.Error() }
func (e *RegexValidationError) Unwrap() error { return e.Err }

const (
	langChainGoFinishAgentLoopMessage = "finalAnswerRequestUserInfo"
	NoLLmManagedError                 = "no LLM managed error"
	MaxAsyncInputConcurrency          = 10
)

// InputFulfillerRepository defines the subset of repository methods needed by InputFulfiller.
type InputFulfillerRepository interface {
	GetToolExecutionState(ctx context.Context, stateID int64) (*InputExecutionState, error)
	UpdateToolExecutionState(ctx context.Context, state *InputExecutionState) error
}

// InputExecutionState represents the state of a pending input resolution.
type InputExecutionState struct {
	ID         int64
	MessageID  string
	InputName  string
	InputValue string
	Status     string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// InputFulfillerDeps holds all injectable dependencies for InputFulfiller.
type InputFulfillerDeps struct {
	// Repository for tool execution state persistence
	Repository InputFulfillerRepository
	// Engine is the tool engine for executing dependent functions
	Engine models.IToolEngine
	// SystemFunctions provides agentic inference capabilities
	SystemFunctions IAgenticInference
	// Callback is the workflow callback for event tracking
	Callback models.AgenticWorkflowCallback
	// LLM provides language model inference for extraction
	LLM LLMProvider
	// VariableReplacer replaces variables in text
	VariableReplacer models.IVariableReplacer
	// AskHumanFunc handles asking a human for input
	AskHumanFunc func(ctx context.Context, query string) (string, error)
}

// InputFulfiller is responsible for fulfilling input parameters for agent tools.
type InputFulfiller struct {
	deps InputFulfillerDeps
	// Repository provides CRUD for confirmation states (set by host).
	Repository toolRepository
	// Database provides audit logging and settings (set by host).
	Database dataStore
}

// InputFulfillerOptions holds optional dependencies for InputFulfiller.
type InputFulfillerOptions struct {
	// Repository provides CRUD for confirmation states (user approval flows).
	Repository toolRepository
	// Database provides audit logging and settings persistence.
	Database dataStore
}

// NewInputFulfiller creates a new InputFulfiller with the given dependencies.
func NewInputFulfiller(deps InputFulfillerDeps, opts ...InputFulfillerOptions) (*InputFulfiller, error) {
	if deps.Engine == nil {
		return nil, errors.New("engine cannot be nil")
	}
	f := &InputFulfiller{deps: deps}
	if len(opts) > 0 {
		f.Repository = opts[0].Repository
		f.Database = opts[0].Database
	}
	return f, nil
}

// FulfillInputs attempts to fill all required inputs for a function based on
// the provided input definitions and context.
func (f *InputFulfiller) FulfillInputs(
	ctx context.Context,
	messageID,
	clientID string,
	tool *skill.Tool,
	functionName string,
	inputs []skill.Input,
) (map[string]interface{}, error) {
	results := make(map[string]interface{})
	var asyncInputs []skill.Input
	var syncInputs []skill.Input

	// Check for pre-fulfilled inputs from checkpoint restoration
	functionKey := fmt.Sprintf("%s.%s", tool.Name, functionName)
	contextKey := fmt.Sprintf("pre_fulfilled_inputs_%s_%s_%s", clientID, messageID, functionKey)

	preFulfilledInputs, hasPrefilled := ctx.Value(contextKey).(map[string]interface{})
	if !hasPrefilled {
		preFulfilledInputs = make(map[string]interface{})
	}

	if hasPrefilled && len(preFulfilledInputs) > 0 {
		if logger != nil {
			logger.Infof("Found %d pre-fulfilled inputs for %s", len(preFulfilledInputs), functionKey)
		}
		for key, value := range preFulfilledInputs {
			if value != nil && fmt.Sprintf("%v", value) != "" {
				results[key] = value
			}
		}
	}

	// Separate inputs into sync and async based on origin
	for _, input := range inputs {
		if _, alreadyFulfilled := results[input.Name]; alreadyFulfilled {
			continue
		}

		switch input.Origin {
		case skill.DataOriginInference:
			asyncInputs = append(asyncInputs, input)
		default:
			syncInputs = append(syncInputs, input)
		}
	}

	// Process synchronous inputs first
	for _, input := range syncInputs {
		if _, alreadyFulfilled := results[input.Name]; alreadyFulfilled {
			continue
		}

		value, err := f.extractValueByOrigin(ctx, messageID, clientID, input, tool, results)
		if err != nil {
			// Handle error based on strategy
			handled, handledValue, handledErr := f.handleInputError(ctx, messageID, input, err, tool)
			if handledErr != nil {
				return nil, handledErr
			}
			if handled {
				if handledValue != "" {
					results[input.Name] = handledValue
				}
				continue
			}
			return nil, err
		}

		if value != "" {
			results[input.Name] = value
		}
	}

	// Process async inputs concurrently
	if len(asyncInputs) > 0 {
		asyncResults, err := f.processAsyncInputs(ctx, messageID, clientID, tool, asyncInputs, results)
		if err != nil {
			return nil, err
		}
		for key, value := range asyncResults {
			results[key] = value
		}
	}

	return results, nil
}

// processAsyncInputs processes inputs that can be resolved concurrently.
func (f *InputFulfiller) processAsyncInputs(
	ctx context.Context,
	messageID, clientID string,
	tool *skill.Tool,
	inputs []skill.Input,
	existingResults map[string]interface{},
) (map[string]interface{}, error) {
	results := make(map[string]interface{})
	var mu sync.Mutex
	var wg sync.WaitGroup
	var firstErr error
	var errOnce sync.Once

	semaphore := make(chan struct{}, MaxAsyncInputConcurrency)

	for _, input := range inputs {
		input := input // capture loop variable
		wg.Add(1)
		semaphore <- struct{}{} // acquire semaphore

		go func() {
			defer wg.Done()
			defer func() { <-semaphore }() // release semaphore

			// Add accumulated inputs to context for inference
			ctxWithInputs := context.WithValue(ctx, "accumulatedInputs", existingResults)

			value, err := f.extractValueByOrigin(ctxWithInputs, messageID, clientID, input, tool, existingResults)
			if err != nil {
				handled, handledValue, handledErr := f.handleInputError(ctx, messageID, input, err, tool)
				if handledErr != nil {
					errOnce.Do(func() { firstErr = handledErr })
					return
				}
				if handled && handledValue != "" {
					mu.Lock()
					results[input.Name] = handledValue
					mu.Unlock()
				}
				return
			}

			if value != "" {
				mu.Lock()
				results[input.Name] = value
				mu.Unlock()
			}
		}()
	}

	wg.Wait()

	if firstErr != nil {
		return nil, firstErr
	}

	return results, nil
}

// ResolveToolExecution updates a pending tool execution with a value.
func (f *InputFulfiller) ResolveToolExecution(ctx context.Context, stateID int64, value string) error {
	if f.deps.Repository == nil {
		return fmt.Errorf("repository not configured")
	}

	state, err := f.deps.Repository.GetToolExecutionState(ctx, stateID)
	if err != nil {
		return err
	}

	if state.Status != "pending" {
		return fmt.Errorf("tool execution already %s", state.Status)
	}

	state.InputValue = value
	state.Status = "complete"
	state.UpdatedAt = time.Now()

	return f.deps.Repository.UpdateToolExecutionState(ctx, state)
}

// AgenticInfer performs agentic inference to determine an input value.
func (f *InputFulfiller) AgenticInfer(
	ctx context.Context,
	messageID,
	clientID string,
	input skill.Input,
	tool *skill.Tool,
) (string, error) {
	if f.deps.SystemFunctions == nil {
		return "", fmt.Errorf("agentic inference not configured")
	}

	// Apply variable replacement to description
	processedDescription := input.Description
	if f.deps.VariableReplacer != nil {
		if replaced, err := f.deps.VariableReplacer.ReplaceVariables(ctx, processedDescription, nil); err == nil {
			processedDescription = replaced
		}
	}

	// Parse success criteria
	var processedSuccessCriteria string
	var allowedSystemFunctions []string
	var disableAllSystemFunctions bool

	successCriteriaObj, err := skill.ParseSuccessCriteria(input.SuccessCriteria)
	if err != nil {
		if logger != nil {
			logger.Errorf("Failed to parse successCriteria: %v", err)
		}
	}

	if successCriteriaObj != nil {
		processedSuccessCriteria = successCriteriaObj.Condition
		if len(successCriteriaObj.AllowedSystemFunctions) > 0 {
			allowedSystemFunctions = successCriteriaObj.AllowedSystemFunctions
		}
		if successCriteriaObj.DisableAllSystemFunctions {
			disableAllSystemFunctions = true
		}

		// Inject context variables from success criteria
		if len(successCriteriaObj.ClientIds) > 0 {
			clientIdsJSON, _ := json.Marshal(successCriteriaObj.ClientIds)
			ctx = context.WithValue(ctx, ClientIdsInContextKey, string(clientIdsJSON))
		}
		if successCriteriaObj.MemoryFilters != nil {
			ctx = context.WithValue(ctx, MemoryFiltersInContextKey, successCriteriaObj.MemoryFilters)
		}
		if successCriteriaObj.CodebaseDirs != "" {
			ctx = context.WithValue(ctx, CodebaseDirsInContextKey, successCriteriaObj.CodebaseDirs)
		}
		if successCriteriaObj.DocumentDbName != "" {
			ctx = context.WithValue(ctx, DocumentDbNameInContextKey, successCriteriaObj.DocumentDbName)
		}
		if successCriteriaObj.DocumentEnableGraph {
			ctx = context.WithValue(ctx, DocumentEnableGraphInContextKey, true)
		}

		// Apply variable replacement to success criteria
		if f.deps.VariableReplacer != nil {
			if replaced, err := f.deps.VariableReplacer.ReplaceVariables(ctx, processedSuccessCriteria, nil); err == nil {
				processedSuccessCriteria = replaced
			}
		}
	} else {
		// Fallback: use raw success criteria as string
		if scStr, ok := input.SuccessCriteria.(string); ok {
			processedSuccessCriteria = scStr
		}
	}

	// Build requesting tool description
	requestingTool := fmt.Sprintf("%s: %s", tool.Name, tool.Description)

	// Invoke inference
	result, err := f.deps.SystemFunctions.InferValue(
		ctx,
		input.Name,
		processedDescription,
		requestingTool,
		processedSuccessCriteria,
		allowedSystemFunctions,
		disableAllSystemFunctions,
	)

	if err != nil {
		return "", err
	}

	// Validate with regex if specified
	if input.RegexValidator != "" {
		if valid, _ := validateRegex(result, input.RegexValidator); !valid {
			// Try to fix with LLM
			if f.deps.LLM != nil {
				fixed, fixErr := f.tryFixWithRegex(ctx, result, input)
				if fixErr == nil && fixed != "" {
					return fixed, nil
				}
			}
			return "", &RegexValidationError{
				ReceivedValue: result,
				Pattern:       input.RegexValidator,
				Err:           fmt.Errorf("value '%s' does not match pattern '%s'", result, input.RegexValidator),
			}
		}
	}

	// Validate with JSON schema if specified
	if input.JsonSchemaValidator != "" {
		if err := validateJSONSchema(result, input.JsonSchemaValidator); err != nil {
			return "", fmt.Errorf("JSON schema validation failed: %w", err)
		}
	}

	return result, nil
}

// RequestUserInput requests input directly from the user.
func (f *InputFulfiller) RequestUserInput(
	ctx context.Context,
	messageID string,
	paramName string,
	additionalInfo string,
) (string, error) {
	if f.deps.AskHumanFunc != nil {
		query := fmt.Sprintf("Please provide the value for '%s'", paramName)
		if additionalInfo != "" {
			query = additionalInfo
		}
		return f.deps.AskHumanFunc(ctx, query)
	}
	return "", ErrInputFulfillmentPending
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

// extractValueByOrigin extracts a value based on the input's origin type.
func (f *InputFulfiller) extractValueByOrigin(
	ctx context.Context,
	messageID, clientID string,
	input skill.Input,
	tool *skill.Tool,
	accumulatedInputs map[string]interface{},
) (string, error) {
	// If input has a pre-defined value, use it (with variable replacement)
	if input.Value != "" {
		value := input.Value
		if f.deps.VariableReplacer != nil {
			if replaced, err := f.deps.VariableReplacer.ReplaceVariables(ctx, value, accumulatedInputs); err == nil {
				value = replaced
			}
		}
		return value, nil
	}

	switch input.Origin {
	case skill.DataOriginChat:
		return f.extractFromChat(ctx, messageID, input, tool)
	case skill.DataOriginInference:
		ctxWithInputs := context.WithValue(ctx, "accumulatedInputs", accumulatedInputs)
		return f.AgenticInfer(ctxWithInputs, messageID, clientID, input, tool)
	case skill.DataOriginFunction:
		return f.extractFromFunction(ctx, messageID, input, tool, accumulatedInputs)
	case skill.DataOriginKnowledge:
		return f.extractFromKnowledge(ctx, messageID, input, tool)
	case skill.DataOriginSearch:
		return f.extractFromSearch(ctx, messageID, input)
	case skill.DataOriginMemory:
		return f.extractFromMemory(ctx, messageID, input, tool)
	default:
		if input.Origin == "" {
			// Default to chat extraction
			return f.extractFromChat(ctx, messageID, input, tool)
		}
		return "", fmt.Errorf("%w: %s", ErrInputOriginNotImplemented, input.Origin)
	}
}

// extractFromChat extracts a value from the conversation context using LLM.
func (f *InputFulfiller) extractFromChat(
	ctx context.Context,
	messageID string,
	input skill.Input,
	tool *skill.Tool,
) (string, error) {
	if f.deps.LLM == nil {
		return "", fmt.Errorf("LLM provider not configured for chat extraction")
	}

	// Build extraction prompt
	systemPrompt := fmt.Sprintf(`You are an information extraction assistant. Extract the value for the parameter '%s' from the conversation context.

Parameter description: %s

Rules:
- Extract ONLY the value, no explanation
- If the value is not present in the conversation, respond with exactly: NOT_FOUND
- If the value is ambiguous, extract the most recent/relevant one
- Do not infer or guess values`, input.Name, input.Description)

	// Get message from context for conversation data
	retrievedMsg, ok := ctx.Value(MessageInContextKey).(Message)
	if !ok {
		return "", &ExtractionNotFoundError{
			InputName:  input.Name,
			Rationale:  "no conversation context available",
			Confidence: 0,
		}
	}

	userPrompt := fmt.Sprintf("User message: %s\n\nExtract the value for '%s'.", retrievedMsg.Body, input.Name)

	result, err := f.deps.LLM.Generate(ctx, systemPrompt, userPrompt, LLMOptions{})
	if err != nil {
		return "", err
	}

	result = strings.TrimSpace(result)

	if strings.ToUpper(result) == "NOT_FOUND" || result == "" {
		return "", &ExtractionNotFoundError{
			InputName:  input.Name,
			Rationale:  fmt.Sprintf("parameter '%s' not found in conversation", input.Name),
			Confidence: 0,
		}
	}

	// Validate with regex if specified
	if input.RegexValidator != "" {
		if valid, _ := validateRegex(result, input.RegexValidator); !valid {
			return "", &RegexValidationError{
				ReceivedValue: result,
				Pattern:       input.RegexValidator,
				Err:           fmt.Errorf("extracted value '%s' does not match pattern '%s'", result, input.RegexValidator),
			}
		}
	}

	return result, nil
}

// extractFromFunction extracts a value from a prior function execution result.
func (f *InputFulfiller) extractFromFunction(
	ctx context.Context,
	messageID string,
	input skill.Input,
	tool *skill.Tool,
	accumulatedInputs map[string]interface{},
) (string, error) {
	// Function-origin inputs resolve via the "from" field referencing another function
	if input.From != "" && f.deps.Engine != nil {
		// Execute the dependency function
		result, err := f.deps.Engine.ExecuteFunctionByNameAndVersion(ctx, messageID, tool.Name, tool.Version, input.From, f)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%v", result), nil
	}

	// Fallback: use variable replacement on the input value
	if input.Value != "" && f.deps.VariableReplacer != nil {
		replaced, err := f.deps.VariableReplacer.ReplaceVariables(ctx, input.Value, accumulatedInputs)
		if err != nil {
			return "", err
		}
		return replaced, nil
	}

	return "", &ExtractionNotFoundError{
		InputName: input.Name,
		Rationale: "function dependency not resolved",
	}
}

// extractFromKnowledge extracts a value from the knowledge base.
func (f *InputFulfiller) extractFromKnowledge(ctx context.Context, messageID string, input skill.Input, tool *skill.Tool) (string, error) {
	return "", fmt.Errorf("knowledge extraction not yet implemented in mantis-skill")
}

// extractFromMemory extracts a value from memory storage.
func (f *InputFulfiller) extractFromMemory(ctx context.Context, messageID string, input skill.Input, tool *skill.Tool) (string, error) {
	return "", fmt.Errorf("memory extraction not yet implemented in mantis-skill")
}

// extractFromSearch extracts a value using web search.
func (f *InputFulfiller) extractFromSearch(ctx context.Context, messageID string, input skill.Input) (string, error) {
	return "", fmt.Errorf("search extraction not yet implemented in mantis-skill")
}

// handleInputError handles errors from input extraction based on the input's onError strategy.
func (f *InputFulfiller) handleInputError(
	ctx context.Context,
	messageID string,
	input skill.Input,
	originalErr error,
	tool *skill.Tool,
) (bool, string, error) {
	if input.OnError == nil {
		if input.IsOptional {
			return true, "", nil // Optional input, skip silently
		}
		return false, "", originalErr
	}

	switch input.OnError.Strategy {
	case "inference":
		if input.IsOptional {
			return true, "", nil
		}
		// Try inference as fallback
		if f.deps.SystemFunctions != nil {
			retrievedMsg, ok := ctx.Value(MessageInContextKey).(Message)
			if ok {
				result, err := f.AgenticInfer(ctx, retrievedMsg.Id, retrievedMsg.ClientID, input, tool)
				if err == nil && result != "" {
					return true, result, nil
				}
			}
		}
		if input.IsOptional {
			return true, "", nil
		}
		return false, "", originalErr

	case "default":
		if input.Value != "" {
			return true, input.Value, nil
		}
		return true, "", nil

	case "message":
		if input.OnError.Message != "" {
			return false, "", fmt.Errorf("%s", input.OnError.Message)
		}
		return false, "", originalErr

	default:
		if input.IsOptional {
			return true, "", nil
		}
		return false, "", originalErr
	}
}

// tryFixWithRegex attempts to fix a value that failed regex validation using LLM.
func (f *InputFulfiller) tryFixWithRegex(ctx context.Context, value string, input skill.Input) (string, error) {
	systemPrompt := fmt.Sprintf(`You are a data formatting assistant. The value '%s' needs to match the regex pattern '%s'.
Reformat the value to match the pattern if possible. Return ONLY the reformatted value, nothing else.
If you cannot reformat it, return exactly: CANNOT_FORMAT`, value, input.RegexValidator)

	result, err := f.deps.LLM.Generate(ctx, systemPrompt, value, LLMOptions{})
	if err != nil {
		return "", err
	}

	result = strings.TrimSpace(result)
	if result == "CANNOT_FORMAT" || result == "" {
		return "", fmt.Errorf("cannot fix value to match regex")
	}

	if valid, _ := validateRegex(result, input.RegexValidator); valid {
		return result, nil
	}
	return "", fmt.Errorf("LLM fix still does not match regex")
}

// validateRegex validates a value against a regex pattern.
func validateRegex(value, pattern string) (bool, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return false, fmt.Errorf("invalid regex pattern: %w", err)
	}
	return re.MatchString(value), nil
}

// validateJSONSchema validates a JSON string against a JSON schema.
func validateJSONSchema(value, schemaStr string) error {
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("schema.json", strings.NewReader(schemaStr)); err != nil {
		return fmt.Errorf("failed to compile JSON schema: %w", err)
	}

	schema, err := compiler.Compile("schema.json")
	if err != nil {
		return fmt.Errorf("failed to compile JSON schema: %w", err)
	}

	if err := schema.Validate(strings.NewReader(value)); err != nil {
		return fmt.Errorf("JSON schema validation failed: %w", err)
	}

	return nil
}

// GenerateFunctionKey generates a unique key for a tool.function combination.
func GenerateFunctionKey(toolName, functionName string) string {
	return fmt.Sprintf("%s.%s", toolName, functionName)
}

// Ensure the unused imports are referenced
var _ = utils.TruncateForLogging
var _ types.Message
