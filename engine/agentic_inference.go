package engine

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/bbiangul/mantis-skill/engine/models"
	"github.com/bbiangul/mantis-skill/engine/utils"
	"github.com/bbiangul/mantis-skill/skill"
	"github.com/bbiangul/mantis-skill/types"

	"github.com/henomis/lingoose/thread"
	"github.com/pemistahl/lingua-go"
)

// InferenceRepository defines the subset of tool repository methods needed by AgenticInference.
// This replaces the full database.ToolRepository dependency.
type InferenceRepository interface {
	CreateFunctionExecution(ctx context.Context, record InferenceFunctionExecution) (int64, error)
}

// InferenceFunctionExecution is a lightweight execution record for inference system functions.
type InferenceFunctionExecution struct {
	MessageID     string
	ClientID      string
	ToolName      string
	FunctionName  string
	Inputs        string
	InputsHash    string
	Output        string
	ExecutedAt    time.Time
	ExecutionTime int64
	Status        string
}

type IAgenticInference interface {
	InferValue(ctx context.Context, inputName, inputDescription, requestingTool, successCriteria string, allowedSystemFunctions []string, disableAllSystemFunctions bool) (string, error)
	SetMaxTurns(maxTurns int)
	SetCallback(callback AgenticInferenceCallback)
	StateStore() *models.StateStore
}

// AgenticInferenceCallback is for inference process events
type AgenticInferenceCallback interface {
	OnStepProposed(messageID, toolName, rationale string)
	OnStepExecuted(messageID, toolName, result string, err error)
	OnInferenceCompleted(messageID string, summary InferenceSummary)
}

// InferenceSummary holds information about the inference process
type InferenceSummary struct {
	InputName      string
	RequestingTool string
	ExecutedSteps  []types.StepExecutionSummary
	FinalValue     string
	ExecutionTime  time.Duration
	CompletionType string
}

// ExtractionNotFoundError indicates the inference could not find a value
type ExtractionNotFoundError struct {
	InputName  string
	Rationale  string
	Confidence float64
}

func (e *ExtractionNotFoundError) Error() string {
	return fmt.Sprintf("extraction not found for '%s': %s", e.InputName, e.Rationale)
}

// NullAgenticInferenceCallback is a no-op implementation of AgenticInferenceCallback
type NullAgenticInferenceCallback struct{}

func (c *NullAgenticInferenceCallback) OnStepProposed(messageID, toolName, rationale string) {}
func (c *NullAgenticInferenceCallback) OnStepExecuted(messageID, toolName, result string, err error) {
}
func (c *NullAgenticInferenceCallback) OnInferenceCompleted(messageID string, summary InferenceSummary) {
}

// AgenticInference implements the Stateful Value Inference Agent
type AgenticInference struct {
	toolFunctions  models.ToolFunctions
	stateStore     models.StateStore
	maxTurns       int
	callback       AgenticInferenceCallback
	toolRepository InferenceRepository
	llm            LLMProvider
}

// NewAgenticInference creates a new AgenticInference
func NewAgenticInference(toolFunctions models.ToolFunctions, toolRepository InferenceRepository, llm LLMProvider) IAgenticInference {
	return &AgenticInference{
		toolFunctions:  toolFunctions,
		maxTurns:       30, // Default max turns
		stateStore:     models.NewConcurrentStateStore(),
		callback:       &NullAgenticInferenceCallback{}, // Default empty callback
		toolRepository: toolRepository,
		llm:            llm,
	}
}

// SetMaxTurns allows changing the maximum number of turns
func (a *AgenticInference) SetMaxTurns(maxTurns int) {
	if maxTurns > 0 {
		a.maxTurns = maxTurns
	}
}

// SetCallback sets the callback for inference events
func (a *AgenticInference) SetCallback(callback AgenticInferenceCallback) {
	if callback != nil {
		a.callback = callback
	} else {
		a.callback = &NullAgenticInferenceCallback{}
	}
}

// StateStore returns the state store
func (a *AgenticInference) StateStore() *models.StateStore {
	return &a.stateStore
}

// InferValue processes a request to generate a value for an input parameter
func (a *AgenticInference) InferValue(
	ctx context.Context,
	inputName,
	inputDescription,
	requestingTool,
	successCriteria string,
	allowedSystemFunctions []string,
	disableAllSystemFunctions bool,
) (string, error) {
	startTime := time.Now()
	if logger != nil {
		logger.Infof("Starting value inference for: %s", inputName)
	}

	// Add input context to the context for use by system functions
	ctx = context.WithValue(ctx, "inputName", inputName)
	ctx = context.WithValue(ctx, "inputDescription", inputDescription)
	ctx = context.WithValue(ctx, "successCriteria", successCriteria)

	retrievedMsg, ok := ctx.Value(MessageInContextKey).(Message)
	if !ok {
		return "", errors.New("error getting message from context")
	}
	messageID := retrievedMsg.Id

	// Get (or create) state for this specific message
	state := a.stateStore.GetState(messageID)
	state.Reset(inputName) // Reuse the state with initial context

	currentTurn := 0
	completionType := "success"

	for currentTurn < a.maxTurns {
		currentTurn++
		if logger != nil {
			logger.Debugf("Executing inference turn %d/%d", currentTurn, a.maxTurns)
		}

		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}

		action, err := a.getNextStep(ctx, messageID, inputName, inputDescription, requestingTool, successCriteria, "", allowedSystemFunctions, disableAllSystemFunctions)
		if err != nil {
			completionType = "error"
			finalMsg := fmt.Sprintf("Error determining next step: %v", err)
			a.finalizeInference(ctx, messageID, state, inputName, requestingTool, finalMsg, time.Since(startTime), completionType)
			return "", fmt.Errorf("failed to determine next step: %w", err)
		}

		if strings.HasPrefix(strings.ToLower(action), "done:") {
			finalValue := strings.TrimPrefix(action, "done:")
			finalValue = strings.TrimSpace(finalValue)

			// Check for NOT_FOUND|rationale format first
			if strings.HasPrefix(strings.ToUpper(finalValue), "NOT_FOUND|") {
				rationale := finalValue[len("NOT_FOUND|"):]
				rationale = strings.TrimSpace(rationale)
				if logger != nil {
					logger.Infof("Value inference for '%s' returned NOT_FOUND with rationale: %s", inputName, rationale)
				}
				completionType = "not_found_with_rationale"
				a.finalizeInference(ctx, messageID, state, inputName, requestingTool, finalValue, time.Since(startTime), completionType)
				return "", &ExtractionNotFoundError{
					InputName:  inputName,
					Rationale:  rationale,
					Confidence: 0,
				}
			}

			// Validation is currently disabled (was generating false negatives)
			confirmed := true
			newAction := ""
			if !confirmed {
				if newAction != "" {
					action = newAction
				} else {
					if logger != nil {
						logger.Warnf("Inference completion validation failed but no new action suggested")
					}
					completionType = "validation_failed"
					a.finalizeInference(ctx, messageID, state, inputName, requestingTool, finalValue, time.Since(startTime), completionType)

					if finalValue == "" {
						return "", &ExtractionNotFoundError{
							InputName:  inputName,
							Rationale:  fmt.Sprintf("Could not infer '%s' from conversation context. LLM returned empty value.", inputName),
							Confidence: 0,
						}
					}
					return finalValue, nil
				}
			} else {
				// Check if it's a PROPOSAL response
				if strings.HasPrefix(newAction, "PROPOSAL:") {
					enhancedValue := strings.TrimPrefix(newAction, "PROPOSAL:")
					enhancedValue = strings.TrimSpace(enhancedValue)

					if logger != nil {
						logger.Infof("Value inference completed with enhanced output after %d turns in %v: %s",
							currentTurn, time.Since(startTime), enhancedValue)
					}

					completionType = "proposal_enhanced"
					a.finalizeInference(ctx, messageID, state, inputName, requestingTool, enhancedValue, time.Since(startTime), completionType)
					return enhancedValue, nil
				}

				// Regular completion
				if logger != nil {
					logger.Debugf("Value inference completed for (%s) after %d turns in %v: %s",
						inputName, currentTurn, time.Since(startTime), finalValue)
				}

				if finalValue == "" {
					completionType = "not_found_fallback"
					a.finalizeInference(ctx, messageID, state, inputName, requestingTool, finalValue, time.Since(startTime), completionType)
					return "", &ExtractionNotFoundError{
						InputName:  inputName,
						Rationale:  fmt.Sprintf("Could not infer '%s' from conversation context. LLM returned empty value.", inputName),
						Confidence: 0,
					}
				}

				completionType = "validation_success"
				a.finalizeInference(ctx, messageID, state, inputName, requestingTool, finalValue, time.Since(startTime), completionType)
				return finalValue, nil
			}
		}

		parts := strings.SplitN(action, ":", 2)
		if len(parts) != 2 {
			return "", fmt.Errorf("invalid action format from LLM: %s", action)
		}

		toolName := strings.TrimSpace(parts[0])
		rationale := strings.TrimSpace(parts[1])

		// agentic inference callback
		a.callback.OnStepProposed(messageID, toolName, rationale)

		if logger != nil {
			logger.Infof("Executing tool '%s' for input '%s'. Reason: %s", toolName, inputName, rationale)
		}

		if strings.Contains(strings.ToLower(toolName), "requestUserInfo") {
			finalMsg := fmt.Sprintf("User information requested: %s \n Please provide so I can continue", rationale)
			a.finalizeInference(ctx, messageID, state, inputName, requestingTool, finalMsg, time.Since(startTime), completionType)
			return finalMsg, nil
		}

		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}

		stepStartTime := time.Now()
		result, err := a.executeTool(ctx, toolName, rationale)
		stepDuration := time.Since(stepStartTime)

		// Record execution with rationale and update the conversation thread
		state.AddToolExecution(toolName, rationale, result, err, stepDuration)

		// Call step executed callback
		a.callback.OnStepExecuted(messageID, toolName, result, err)
		Callback(ctx, toolName, result, err)

		if err != nil {
			errorMessage := fmt.Sprintf("Error executing tool '%s': %s", toolName, err.Error())
			if logger != nil {
				logger.Warnf(errorMessage)
			}
		} else {
			if logger != nil {
				logger.Infof("Tool '%s' execution result: %s", toolName, utils.TruncateForLogging(result))
			}
		}
	}

	// If we got here, we hit the max turns
	completionType = "max_turns"
	finalMessage := "Maximum number of turns reached without inference completion."

	if logger != nil {
		logger.Warnf("Maximum number of turns reached without value inference")
	}
	a.finalizeInference(ctx, messageID, state, inputName, requestingTool, "", time.Since(startTime), completionType)
	return "", fmt.Errorf("%s", finalMessage)
}

// getNextStep consults the LLM to determine the next step in value inference
func (a *AgenticInference) getNextStep(
	ctx context.Context,
	messageID,
	inputName,
	inputDescription,
	requestingTool,
	successCriteria string,
	contextForAgent string,
	allowedSystemFunctions []string,
	disableAllSystemFunctions bool,
) (string, error) {
	state := a.stateStore.GetState(messageID)
	maxAttempts := 6

	toolNames := make([]string, 0, 3)
	functionNames := make([]string, 0, 6)

	// Default system functions - available unless explicitly disabled
	defaultSystemFunctions := map[string]string{
		models.SystemFunctionAskConversationHistory: models.DescriptionAskConversationHistory,
		models.SystemFunctionAskKnowledgeBase:       models.DescriptionAskKnowledgeBase,
		models.SystemFunctionAskToContext:           models.DescriptionAskToContext,
		models.SystemFunctionDoDeepWebResearch:      models.DescriptionDoDeepWebResearch,
		models.SystemFunctionDoSimpleWebSearch:      models.DescriptionDoSimpleWebSearch,
		models.SystemFunctionGetWeekdayFromDate:     models.DescriptionGetWeekdayFromDate,
		models.SystemFunctionQueryMemories:          models.DescriptionQueryMemories,
	}

	// Opt-in only system functions
	optInOnlySystemFunctions := map[string]string{
		models.SystemFunctionFetchWebContent:           models.DescriptionFetchWebContent,
		models.SystemFunctionQueryCustomerServiceChats: models.DescriptionQueryCustomerServiceChats,
		models.SystemFunctionAnalyzeImage:              models.DescriptionAnalyzeImage,
		models.SystemFunctionSearchCodebase:            models.DescriptionSearchCodebase,
		models.SystemFunctionQueryDocuments:            models.DescriptionQueryDocuments,
	}

	// All system functions (for validation when allowedSystemFunctions is provided)
	allSystemFunctions := make(map[string]string)
	for k, v := range defaultSystemFunctions {
		allSystemFunctions[k] = v
	}
	for k, v := range optInOnlySystemFunctions {
		allSystemFunctions[k] = v
	}

	// Filter based on disableAllSystemFunctions and allowedSystemFunctions
	availableTools := make(map[string]string)
	if disableAllSystemFunctions {
		// availableTools remains empty
	} else if len(allowedSystemFunctions) > 0 {
		for _, allowedFunc := range allowedSystemFunctions {
			if description, ok := allSystemFunctions[allowedFunc]; ok {
				availableTools[allowedFunc] = description
			}
		}
	} else {
		availableTools = defaultSystemFunctions
	}

	for name, description := range availableTools {
		toolNames = append(toolNames, fmt.Sprintf("tool_name: %s - description: %s \n", name, description))
		functionNames = append(functionNames, name)
	}
	functionNames = append(functionNames, "done")

	// Check if we already have a thread for this message
	var messages *thread.Thread
	if state.ConversationThread == nil {
		messages = thread.New()

		systemPrompt := GetInferenceSystemPrompt()
		messages.AddMessages(
			thread.NewSystemMessage().AddContent(thread.NewTextContent(systemPrompt)),
		)

		// Extract detected language from context to enforce it in generated outputs
		languageInstruction := ""
		if userLanguage, ok := ctx.Value(MessageLanguageInContextKey).(lingua.Language); ok {
			languageName := strings.ToUpper(userLanguage.String())
			languageInstruction = fmt.Sprintf(`<language_requirement>
IMPORTANT: Any text that will be shown to the user MUST be generated in %s.
This includes questions, messages, or any output that the user will see.
Do NOT use English unless the user's detected language is English.
</language_requirement>

`, languageName)
		}

		// Build tools section
		toolsSection := ""
		if len(availableTools) == 0 {
			toolsSection = `<no_tools_available>
CRITICAL: No system functions are available. You can ONLY use information from the immediate context.
You MUST respond in the format: done: {your answer}
Do NOT suggest using any tools or functions. Simply provide your answer based on the available context.
</no_tools_available>`
		} else {
			toolsSection = fmt.Sprintf("<available_Tools> %s </available_Tools>", strings.Join(toolNames, "\n"))
		}

		userContent := fmt.Sprintf(`%s<input_name> %s </input_name>
<input_description> %s </input_description>
<requesting_tool> %s </requesting_tool>
<success_criteria> %s </success_criteria>
%s`,
			languageInstruction,
			inputName,
			inputDescription,
			requestingTool,
			successCriteria,
			toolsSection)

		messages.AddMessages(
			thread.NewUserMessage().AddContent(thread.NewTextContent(userContent)),
		)

		state.ConversationThread = messages
	} else {
		messages = state.ConversationThread
	}

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		result, err := a.llm.GenerateWithThread(ctx, messages, LLMOptions{})
		if err != nil {
			return "", fmt.Errorf("error generating LLM action: %w", err)
		}

		// Append result to thread (like the original llms.Generate does)
		if result != "" {
			messages.AddMessages(
				thread.NewAssistantMessage().AddContent(thread.NewTextContent(result)),
			)
		}

		if len(messages.LastMessage().Contents) == 0 {
			return "", errors.New("no message generated by LLM")
		}

		action := messages.LastMessage().Contents[0].AsString()
		action = strings.TrimSpace(action)

		if logger != nil {
			logger.Debugf("Attempt %d - LLM action: %s", attempt, action)
		}

		originalAction := action
		isValid := true

		// Try to extract the tool/done info from the response
		extractedAction := extractToolInfo(action)
		if extractedAction != "" {
			action = extractedAction
		} else if !strings.Contains(action, ":") {
			isValid = false
		}

		if !isValid {
			errorMsg := fmt.Sprintf("Your response '%s' is not in the required format. You must respond with either '{tool_name}: {reason}' or 'done: {generated_value}'. Please try again.", originalAction)
			messages.AddMessages(
				thread.NewUserMessage().AddContent(thread.NewTextContent(errorMsg)),
			)
			continue
		}

		// Check if it's a valid tool (if not "done")
		if !strings.HasPrefix(strings.ToLower(action), "done:") {
			parts := strings.SplitN(action, ":", 2)
			if len(parts) != 2 {
				extractedAction := extractToolInfo(originalAction)
				if extractedAction != "" {
					parts = strings.SplitN(extractedAction, ":", 2)
					if len(parts) == 2 {
						action = extractedAction
					} else {
						errorMsg := fmt.Sprintf("Your response '%s' is not properly formatted. It should be '{tool_name}: {reason}' or 'done: {generated_value}'. Please try again.", originalAction)
						messages.AddMessages(
							thread.NewUserMessage().AddContent(thread.NewTextContent(errorMsg)),
						)
						continue
					}
				} else {
					errorMsg := fmt.Sprintf("Your response '%s' is not properly formatted. It should be '{tool_name}: {reason}' or 'done: {generated_value}'. Please try again.", originalAction)
					messages.AddMessages(
						thread.NewUserMessage().AddContent(thread.NewTextContent(errorMsg)),
					)
					continue
				}
			}

			toolName := strings.TrimSpace(parts[0])
			toolName = strings.ReplaceAll(toolName, "\n", "")
			toolName = strings.ReplaceAll(toolName, "\t", "")
			toolName = strings.TrimSpace(toolName)

			if !stringInSlice(functionNames, toolName) {
				extractedAction := extractToolInfo(originalAction)
				if extractedAction != "" {
					extractedParts := strings.SplitN(extractedAction, ":", 2)
					if len(extractedParts) == 2 {
						extractedToolName := strings.TrimSpace(extractedParts[0])
						extractedToolName = strings.ReplaceAll(extractedToolName, "\n", "")
						extractedToolName = strings.ReplaceAll(extractedToolName, "\t", "")
						extractedToolName = strings.TrimSpace(extractedToolName)

						if stringInSlice(functionNames, extractedToolName) {
							action = extractedAction
						} else {
							toolsList := strings.Join(toolNames, ", ")
							errorMsg := fmt.Sprintf("'%s' is not a valid tool. Available tools are: %s. Please select one of these tools or respond with 'done: {generated_value}'.",
								toolName, toolsList)
							messages.AddMessages(
								thread.NewUserMessage().AddContent(thread.NewTextContent(errorMsg)),
							)
							continue
						}
					} else {
						toolsList := strings.Join(toolNames, ", ")
						errorMsg := fmt.Sprintf("'%s' is not a valid tool. Available tools are: %s. Please select one of these tools or respond with 'done: {generated_value}'.",
							toolName, toolsList)
						messages.AddMessages(
							thread.NewUserMessage().AddContent(thread.NewTextContent(errorMsg)),
						)
						continue
					}
				} else {
					toolsList := strings.Join(toolNames, ", ")
					errorMsg := fmt.Sprintf("'%s' is not a valid tool. Available tools are: %s. Please select one of these tools or respond with 'done: {generated_value}'.",
						toolName, toolsList)
					messages.AddMessages(
						thread.NewUserMessage().AddContent(thread.NewTextContent(errorMsg)),
					)
					continue
				}
			}
		}

		// Dump inference messages for debugging (only in LOCAL environment)
		dumpInferenceMessages(messageID, inputName, action, messages)

		return action, nil
	}

	// If we've exhausted all attempts, return a default fallback
	if logger != nil {
		logger.Warnf("Failed to get valid action after %d attempts, using fallback", maxAttempts)
	}
	return "done: ERROR_UNABLE_TO_INFER_VALUE", nil
}

// executeTool executes the specified tool and returns its result
func (a *AgenticInference) executeTool(ctx context.Context, toolName, rationale string) (string, error) {
	res, err := a.checkSystemFn(ctx, toolName, rationale)
	if err != nil {
		if !strings.Contains(err.Error(), "not found in system tools") {
			return res, err
		}
	}
	if res != "" {
		return res, nil
	}

	return "", fmt.Errorf("tool '%s' not found in available tools", toolName)
}

// recordSystemFunctionExecution records a system function execution
func (a *AgenticInference) recordSystemFunctionExecution(
	ctx context.Context,
	messageID string,
	clientID string,
	functionName string,
	rationale string,
	inputName string,
	result string,
	executionTime int64,
	err error,
) {
	inputs := make(map[string]interface{})
	inputs["rationale"] = rationale
	inputs["input_name"] = inputName

	inputsJSON, jsonErr := json.Marshal(inputs)
	if jsonErr != nil {
		if logger != nil {
			logger.Warnf("Failed to marshal system function inputs for recording: %v", jsonErr)
		}
		return
	}

	inputsHash := generateInputsHash(inputs)

	status := "success"
	if err != nil {
		status = "failed"
	}

	execution := InferenceFunctionExecution{
		MessageID:     messageID,
		ClientID:      clientID,
		ToolName:      "inference_system_function",
		FunctionName:  functionName,
		Inputs:        string(inputsJSON),
		InputsHash:    inputsHash,
		Output:        result,
		ExecutedAt:    time.Now(),
		ExecutionTime: executionTime,
		Status:        status,
	}

	id, saveErr := a.toolRepository.CreateFunctionExecution(ctx, execution)
	if saveErr != nil {
		if logger != nil {
			logger.Warnf("Failed to record system function execution: %v", saveErr)
		}
		return
	}

	if logger != nil {
		logger.Debugf("Successfully recorded system function execution (ID: %d) for %s with status %s", id, functionName, status)
	}
}

// executeAndRecordSystemFunction wraps a system function call with execution recording
func (a *AgenticInference) executeAndRecordSystemFunction(
	ctx context.Context,
	functionName string,
	rationale string,
	fn func() (string, error),
) (string, error) {
	retrievedMsg, ok := ctx.Value(MessageInContextKey).(Message)
	if !ok {
		if logger != nil {
			logger.Warnf("Could not get message from context, skipping system function execution recording")
		}
		return fn()
	}

	inputName := ""
	if name, nameOk := ctx.Value("inputName").(string); nameOk {
		inputName = name
	}

	startTime := time.Now()
	result, err := fn()
	executionTime := time.Since(startTime).Milliseconds()

	if a.toolRepository != nil {
		go func() {
			defer func() {
				if r := recover(); r != nil {
					if logger != nil {
						logger.Errorf("Panic in system function recording: %v", r)
					}
				}
			}()
			a.recordSystemFunctionExecution(ctx, retrievedMsg.Id, retrievedMsg.ClientID, functionName, rationale, inputName, result, executionTime, err)
		}()
	}

	return result, err
}

// checkSystemFn handles system function execution
func (a *AgenticInference) checkSystemFn(ctx context.Context, toolName, rationale string) (string, error) {
	switch toolName {
	case models.SystemFunctionRequestInternalTeamInfo:
		return a.executeAndRecordSystemFunction(ctx, models.SystemFunctionRequestInternalTeamInfo, rationale, func() (string, error) {
			return a.toolFunctions.AskHuman(ctx, rationale)
		})
	case models.SystemFunctionRequestInternalTeamAction:
		return a.executeAndRecordSystemFunction(ctx, models.SystemFunctionRequestInternalTeamAction, rationale, func() (string, error) {
			return a.toolFunctions.RequestHumanAction(ctx, rationale)
		})
	case models.SystemFunctionAskConversationHistory:
		return a.executeAndRecordSystemFunction(ctx, models.SystemFunctionAskConversationHistory, rationale, func() (string, error) {
			return a.toolFunctions.AskBasedOnConversation(ctx, rationale)
		})
	case models.SystemFunctionAskKnowledgeBase:
		return a.executeAndRecordSystemFunction(ctx, models.SystemFunctionAskKnowledgeBase, rationale, func() (string, error) {
			return a.toolFunctions.AskKnowledge(ctx, rationale)
		})
	case models.SystemFunctionAskToContext:
		return a.executeAndRecordSystemFunction(ctx, models.SystemFunctionAskToContext, rationale, func() (string, error) {
			return a.toolFunctions.AskAboutAgenticEvents(ctx, rationale)
		})
	case models.SystemFunctionDoDeepWebResearch:
		return a.executeAndRecordSystemFunction(ctx, models.SystemFunctionDoDeepWebResearch, rationale, func() (string, error) {
			return a.toolFunctions.DeepResearch(ctx, rationale, "a complete and detailed research report.")
		})
	case models.SystemFunctionDoSimpleWebSearch:
		return a.executeAndRecordSystemFunction(ctx, models.SystemFunctionDoSimpleWebSearch, rationale, func() (string, error) {
			return a.toolFunctions.Search(ctx, rationale)
		})
	case models.SystemFunctionGetWeekdayFromDate:
		return a.executeAndRecordSystemFunction(ctx, models.SystemFunctionGetWeekdayFromDate, rationale, func() (string, error) {
			return a.toolFunctions.GetWeekdayFromDate(ctx, rationale)
		})
	case models.SystemFunctionQueryMemories:
		var memoryFilters *skill.MemoryFilters
		if filtersRaw := ctx.Value(MemoryFiltersInContextKey); filtersRaw != nil {
			if filters, ok := filtersRaw.(*skill.MemoryFilters); ok {
				memoryFilters = filters
			}
		}
		return a.executeAndRecordSystemFunction(ctx, models.SystemFunctionQueryMemories, rationale, func() (string, error) {
			return a.toolFunctions.QueryMemoriesWithFilters(ctx, rationale, memoryFilters)
		})
	case models.SystemFunctionFetchWebContent:
		return a.executeAndRecordSystemFunction(ctx, models.SystemFunctionFetchWebContent, rationale, func() (string, error) {
			return a.toolFunctions.FetchWebContent(ctx, rationale)
		})
	case models.SystemFunctionQueryCustomerServiceChats:
		var clientIds []string
		if clientIdsRaw := ctx.Value(ClientIdsInContextKey); clientIdsRaw != nil {
			switch v := clientIdsRaw.(type) {
			case string:
				for _, id := range strings.Split(v, ",") {
					if trimmed := strings.TrimSpace(id); trimmed != "" {
						clientIds = append(clientIds, trimmed)
					}
				}
			case []string:
				clientIds = v
			case []interface{}:
				for _, id := range v {
					if idStr, ok := id.(string); ok && idStr != "" {
						clientIds = append(clientIds, idStr)
					}
				}
			}
		}
		return a.executeAndRecordSystemFunction(ctx, models.SystemFunctionQueryCustomerServiceChats, rationale, func() (string, error) {
			return a.toolFunctions.QueryCustomerServiceChats(ctx, rationale, clientIds)
		})
	case models.SystemFunctionAnalyzeImage:
		return a.executeAndRecordSystemFunction(ctx, models.SystemFunctionAnalyzeImage, rationale, func() (string, error) {
			return a.toolFunctions.AnalyzeImage(ctx, rationale)
		})
	case models.SystemFunctionSearchCodebase:
		var dirs []string
		if dirsRaw := ctx.Value(CodebaseDirsInContextKey); dirsRaw != nil {
			if dirsStr, ok := dirsRaw.(string); ok {
				var repos []map[string]interface{}
				if err := json.Unmarshal([]byte(dirsStr), &repos); err == nil {
					for _, repo := range repos {
						if path, ok := repo["local_path"].(string); ok {
							dirs = append(dirs, path)
						}
					}
				} else {
					json.Unmarshal([]byte(dirsStr), &dirs)
				}
			}
		}
		if len(dirs) == 0 {
			return "", fmt.Errorf("searchCodebase: no codebase directories configured")
		}
		return a.executeAndRecordSystemFunction(ctx, models.SystemFunctionSearchCodebase, rationale, func() (string, error) {
			return a.toolFunctions.SearchCodebase(ctx, rationale, dirs)
		})
	case models.SystemFunctionQueryDocuments:
		var dbName string
		var enableGraph bool
		if dbNameRaw := ctx.Value(DocumentDbNameInContextKey); dbNameRaw != nil {
			if dbNameStr, ok := dbNameRaw.(string); ok {
				dbName = dbNameStr
			}
		}
		if enableGraphRaw := ctx.Value(DocumentEnableGraphInContextKey); enableGraphRaw != nil {
			if enableGraphBool, ok := enableGraphRaw.(bool); ok {
				enableGraph = enableGraphBool
			}
		}
		return a.executeAndRecordSystemFunction(ctx, models.SystemFunctionQueryDocuments, rationale, func() (string, error) {
			return a.toolFunctions.QueryDocuments(ctx, rationale, dbName, enableGraph)
		})
	default:
		return "", fmt.Errorf("tool '%s' not found in system tools", toolName)
	}
}

// recordInferenceExecution records the agentic inference's final response
func (a *AgenticInference) recordInferenceExecution(
	ctx context.Context,
	messageID string,
	clientID string,
	inputName string,
	requestingTool string,
	inputDescription string,
	successCriteria string,
	finalValue string,
	executionTime int64,
	completionType string,
) {
	inputs := make(map[string]interface{})
	inputs["input_name"] = inputName
	inputs["input_description"] = inputDescription
	inputs["success_criteria"] = successCriteria

	var toolName, functionName string
	if parts := strings.SplitN(requestingTool, ":", 2); len(parts) >= 1 {
		toolName = strings.TrimSpace(parts[0])
		functionName = fmt.Sprintf("infer_%s", inputName)
	} else {
		toolName = "unknown_tool"
		functionName = fmt.Sprintf("infer_%s", inputName)
	}
	inputs["requesting_tool"] = requestingTool

	inputsJSON, err := json.Marshal(inputs)
	if err != nil {
		if logger != nil {
			logger.Warnf("Failed to marshal agentic_inference inputs for recording: %v", err)
		}
		return
	}

	inputsHash := generateInputsHash(inputs)

	status := "success"
	if completionType == "error" || completionType == "validation_failed" {
		status = "failed"
	} else if completionType == "max_turns" {
		status = "failed"
	}

	execution := InferenceFunctionExecution{
		MessageID:     messageID,
		ClientID:      clientID,
		ToolName:      toolName,
		FunctionName:  functionName,
		Inputs:        string(inputsJSON),
		InputsHash:    inputsHash,
		Output:        finalValue,
		ExecutedAt:    time.Now().Add(-time.Duration(executionTime) * time.Millisecond),
		ExecutionTime: executionTime,
		Status:        status,
	}

	id, saveErr := a.toolRepository.CreateFunctionExecution(ctx, execution)
	if saveErr != nil {
		if logger != nil {
			logger.Warnf("Failed to record agentic_inference execution: %v", saveErr)
		}
		return
	}

	if logger != nil {
		logger.Debugf("Successfully recorded agentic_inference execution (ID: %d) for input %s with status %s", id, inputName, status)
	}
}

// finalizeInference handles completion, including callbacks and cleanup
func (a *AgenticInference) finalizeInference(
	ctx context.Context,
	messageID string,
	state *models.CoordinatorState,
	inputName string,
	requestingTool string,
	finalValue string,
	duration time.Duration,
	completionType string,
) {
	summary := a.buildInferenceSummary(state, inputName, requestingTool, finalValue, duration, completionType)

	retrievedMsg, ok := ctx.Value(MessageInContextKey).(Message)
	if ok && a.toolRepository != nil {
		inputDescription := ""
		successCriteria := ""
		if desc, descOk := ctx.Value("inputDescription").(string); descOk {
			inputDescription = desc
		}
		if criteria, criteriaOk := ctx.Value("successCriteria").(string); criteriaOk {
			successCriteria = criteria
		}

		a.recordInferenceExecution(
			ctx,
			messageID,
			retrievedMsg.ClientID,
			inputName,
			requestingTool,
			inputDescription,
			successCriteria,
			finalValue,
			duration.Milliseconds(),
			completionType,
		)
	}

	a.callback.OnInferenceCompleted(messageID, summary)
}

// buildInferenceSummary creates a summary of the inference execution
func (a *AgenticInference) buildInferenceSummary(
	state *models.CoordinatorState,
	inputName string,
	requestingTool string,
	finalValue string,
	duration time.Duration,
	completionType string,
) InferenceSummary {
	stepSummaries := make([]types.StepExecutionSummary, 0, len(state.ExecutionHistory))

	for _, exec := range state.ExecutionHistory {
		errorStr := ""
		if exec.Error != nil {
			errorStr = exec.Error.Error()
		}

		stepSummary := types.StepExecutionSummary{
			ToolName:      exec.ToolName,
			Rationale:     exec.Rationale,
			Result:        exec.Result,
			Error:         errorStr,
			ExecutionTime: exec.Duration,
		}

		stepSummaries = append(stepSummaries, stepSummary)
	}

	return InferenceSummary{
		InputName:      inputName,
		RequestingTool: requestingTool,
		ExecutedSteps:  stepSummaries,
		FinalValue:     finalValue,
		ExecutionTime:  duration,
		CompletionType: completionType,
	}
}

// Helper function to check if a string is in a slice
func stringInSlice(slice []string, str string) bool {
	for _, s := range slice {
		if s == str {
			return true
		}
	}
	return false
}

// GetInferenceSystemPrompt returns the system prompt for the inference agent
func GetInferenceSystemPrompt() string {
	dayOfWeek := time.Now().Weekday()
	today := fmt.Sprintf("TODAY IS: %s - %s", time.Now().String(), dayOfWeek.String())

	return fmt.Sprintf(` %s

CRITICAL TIME REFERENCE INSTRUCTION:
The date and time shown above is your ONLY time reference. Use this EXACT date/time for ALL time-based inferences, calculations, and queries.
- When the user asks about "today", "yesterday", "this week", "last month" etc., calculate based on the date/time shown above
- When checking if events are in the past or future, compare against the date/time shown above
- When calculating date ranges or relative dates, use the date/time shown above as your reference point
- DO NOT use your training cutoff date or any other date - ONLY use the date/time explicitly provided above

You are a Stateful Input Value Fulfillment Agent. Your mission is to determine and generate the *exact value* for a specified input parameter ('input_name') needed by a specific system tool ('requesting_tool'). Your generation must be guided by the provided description ('input_description') and explicit 'success_criteria'. You must use available tools to gather any missing information required to generate a value that meets these criteria. You operate in a turn-based manner.

Your Goal: Generate the single, final value for the 'input_name' that satisfies the provided 'success_criteria' for the 'requesting_tool'. identify and execute the single best tool *right now* to gather the specific information needed to eventually generate a value that meets these criteria.

Interaction Flow:

1.  **Receive Input Requirements:** You will receive the 'input_name' (the parameter to generate), 'input_description' (what the parameter represents), 'context' (initial information), 'requesting_tool' (the tool needing this input), 'success_criteria' (how to judge the quality/completeness of the generated value), and the 'Available Tools'.
2.  **Analyze State:** Evaluate if the 'context' (and any subsequent 'answer:' data) contains enough information to confidently generate the value for 'input_name' according to 'input_description' and meet the specified 'success_criteria' for the 'requesting_tool'.
3.  **Output Instruction:** Output *exactly one* instruction in one of the following formats:
    *   **To gather more information:** '{tool_name}: {A concise reason explaining precisely what specific information is missing to generate the value for '{input_name}' according to the success_criteria, and why executing *this specific tool* is the best way to obtain it right now.}'
    *   **To provide the final value:** 'done: {The generated value for the input_name}' - Use this *only* when you have gathered all necessary information and are ready to output the final, complete value that meets the 'success_criteria'. The content after 'done:' IS the value itself.
4.  **Receive Tool Result:** The system (I) will execute the requested tool (if applicable) and provide the result back to you in the next turn, prefixed with 'answer:'.
5.  **Repeat Analysis:** Analyze the updated state (including the latest 'answer:') and decide the next single step (another tool execution or the final 'done: {value}').
6.  **Completion:** The process ends when you output 'done: {generated_value OR none}'.

(You must choose one of these names for {tool_name} from the provided 'Available Tools')

Crucial Constraints:

*   **One Step Only:** You MUST output only one '{tool_name}: {reason}' or one 'done: {generated_value}' per turn.
*   **Value Focus & Quality:** Your primary goal is to generate the value for 'input_name' that meets the 'success_criteria'. Only use tools if they are essential for gathering missing details *specifically needed* to construct this value accurately, contextually (based on 'input_description' and 'context'), and successfully (based on 'success_criteria').
*   **No Inference:** Do NOT assume information not explicitly present in the 'context' or the 'answer:' history. If you lack details required by the 'input_description' or 'success_criteria' to generate the value, request the appropriate tool to get them. No inference, no assumption. you are allow to create new data. you can just format or select something semanticly equivalent but that is it. If you are not able to fill the input, use the failure format below - done:NOT_FOUND|rationale.
*   **Strict Formatting:** Adhere strictly to the specified output formats. The 'done:' output formats:
    - Success: 'done: {generated_value}' - the exact value to fill the input parameter
    - Failure: 'done:NOT_FOUND|{rationale explaining what information was searched for and why it could not be found}'
    Example success: 'done: 11987654321'
    Example failure: 'done:NOT_FOUND|I searched the conversation history and found the user mentioned they are from Sao Paulo (city), but no phone number was mentioned or provided in any message.'
*   **Stateful Reasoning:** Your decision in each step must be based on the target 'input_name', 'input_description', 'requesting_tool', 'success_criteria', the initial 'context', and the cumulative information received via 'answer:' so far.
*   **Clarity in Reasons:** Your reasons for using a tool must clearly state what information you are seeking *for the purpose of generating the final value according to the success criteria*. When a question is applicable, include it in your rationale.
*   **Do not create data:** you MUST rely only on the answers you GOT. Do not create any kind info. You are allowed to format data (e.g. if you expect a ID and the answer includes context message with the ID, extract only the ID from its allowed). But never create. You are allowed to repeat actions, with follow up questions/requests/remarks in order to clarify the output.*.
*   **No Direct User Interaction:** Do not attempt to talk to the end-user or ask them questions directly. Your output guides the system or provides the final value.
*   If the answer generated from a tool is not expected, you should output a different question to try to get the expected output from the tool.
* If there are multiple values for one choice, always prioritize the latest (most recent) info provided.
Think step by step in order to produce the best content generation.
%s`, today, today)
}

// GetInferenceCompletionValidationPrompt returns the system prompt for validating inference completion
func GetInferenceCompletionValidationPrompt() string {
	return `You are an Inference Validation Assistant. Your job is to analyze an inference execution and determine if it's truly complete with the proposed value or if there are critical steps still missing.

You will receive:
1. The input parameter name and description that needs to be inferred
2. Success criteria for determining a complete inference
3. A history of already executed tools and their results
4. A proposed final value for the input parameter
5. A list of available tools that could be used

Your task:
- Carefully analyze if all necessary information gathering steps have been taken to generate the required input value
- Consider whether the proposed value meets the specified success criteria
- Determine if any available tools should still be used to provide a more accurate or complete value
- Evaluate if the proposed value contains all the information required by the input description
- Assess if the proposed value could be enhanced, enriched, formatted, or made more detailed without calling additional tools

You must respond in ONE of the following three formats:

1. If you believe the inference is complete and the proposed value adequately fulfills the requirements, respond with:
"COMPLETE: <brief explanation why the proposed value is sufficient>"

2. If you believe the inference is incomplete and should continue with more tool calls, respond with:
"INCOMPLETE: <tool_name>: <rationale for using this tool>"

3. If you believe all essential data is present but the output could be improved without calling new tools, respond with:
"PROPOSAL: <enhanced/formatted/enriched/detailed version of the output that better meets the success criteria>"

Be thorough in your analysis but also practical - don't suggest additional tools if they only add minimal value.`
}

// ---------------------------------------------------------------------------
// Inlined utility functions (from connect-ai/pkg/utils)
// ---------------------------------------------------------------------------

// extractToolInfo extracts tool name and argument from LLM output
func extractToolInfo(input string) string {
	// Check for "done:" pattern at start of line (last occurrence)
	doneRe := regexp.MustCompile(`(?i)(?m)^done:\s*(.+)$`)
	allDoneMatches := doneRe.FindAllStringSubmatch(input, -1)
	if len(allDoneMatches) > 0 {
		lastMatch := allDoneMatches[len(allDoneMatches)-1]
		if len(lastMatch) >= 2 {
			matchStartIndex := strings.LastIndex(input, lastMatch[0])
			if matchStartIndex != -1 {
				afterDone := input[matchStartIndex+len("done:"):]
				afterDone = strings.TrimSpace(afterDone)
				return "done: " + afterDone
			}
			return "done: " + strings.TrimSpace(lastMatch[1])
		}
	}

	// Fallback: done: anywhere in text (last occurrence, truncate at newline)
	doneRe = regexp.MustCompile(`(?i)done:\s*(.*)`)
	allDoneMatches = doneRe.FindAllStringSubmatch(input, -1)
	if len(allDoneMatches) > 0 {
		lastMatch := allDoneMatches[len(allDoneMatches)-1]
		if len(lastMatch) >= 2 {
			content := strings.TrimSpace(lastMatch[1])
			if idx := strings.Index(content, "\n"); idx != -1 {
				content = strings.TrimSpace(content[:idx])
			}
			return "done: " + content
		}
	}

	toolNamePattern := `([a-zA-Z0-9_-]+(?::(?:blocking|non-blocking))?)`

	// Backtick-quoted content
	backticksRe := regexp.MustCompile("`" + toolNamePattern + `:\s*([^` + "`" + `]*)` + "`")
	matches := backticksRe.FindAllStringSubmatch(input, -1)
	if len(matches) > 0 {
		lastMatch := matches[len(matches)-1]
		if len(lastMatch) >= 3 {
			return strings.TrimSpace(lastMatch[1] + ": " + lastMatch[2])
		}
	}

	// Single-quoted content
	singleQuoteRe := regexp.MustCompile(`'` + toolNamePattern + `:\s*([^']*)'`)
	matches = singleQuoteRe.FindAllStringSubmatch(input, -1)
	if len(matches) > 0 {
		lastMatch := matches[len(matches)-1]
		if len(lastMatch) >= 3 {
			return strings.TrimSpace(lastMatch[1] + ": " + lastMatch[2])
		}
	}

	// Unquoted pattern at start of line
	re := regexp.MustCompile(`(?m)^` + toolNamePattern + `:\s*(.+)$`)
	matches = re.FindAllStringSubmatch(input, -1)
	if len(matches) > 0 {
		lastMatch := matches[len(matches)-1]
		if len(lastMatch) >= 3 {
			return strings.TrimSpace(lastMatch[1] + ": " + lastMatch[2])
		}
	}

	return ""
}

// generateInputsHash creates a deterministic hash of inputs map
func generateInputsHash(inputs map[string]interface{}) string {
	var keys []string
	for k := range inputs {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var parts []string
	for _, k := range keys {
		value := fmt.Sprintf("%v", inputs[k])
		parts = append(parts, fmt.Sprintf("%s=%s", k, value))
	}

	content := strings.Join(parts, "|")
	hash := sha256.Sum256([]byte(content))
	return fmt.Sprintf("%x", hash)
}

// dumpInferenceMessages writes the LLM thread messages to a file for debugging (only in LOCAL environment)
func dumpInferenceMessages(messageID, inputName, action string, messages *thread.Thread) {
	if os.Getenv("ENVIRONMENT") != "LOCAL" {
		return
	}

	// Use a local directory for dumps
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return
	}
	dir := filepath.Join(homeDir, ".mantis-skill", "inference_dumps")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return
	}

	timestamp := time.Now().Format("20060102_150405")
	safeInputName := strings.ReplaceAll(inputName, "/", "_")
	safeInputName = strings.ReplaceAll(safeInputName, "\\", "_")
	safeInputName = strings.ReplaceAll(safeInputName, " ", "_")
	if len(safeInputName) > 30 {
		safeInputName = safeInputName[:30]
	}
	filename := filepath.Join(dir, fmt.Sprintf("%s_%s_%s.txt", messageID, safeInputName, timestamp))

	var content strings.Builder
	content.WriteString("=== Inference Messages Dump ===\n")
	content.WriteString(fmt.Sprintf("Message ID: %s\n", messageID))
	content.WriteString(fmt.Sprintf("Input Name: %s\n", inputName))
	content.WriteString(fmt.Sprintf("Timestamp: %s\n", time.Now().Format(time.RFC3339)))
	content.WriteString(fmt.Sprintf("Final Action/Value: %s\n", action))
	content.WriteString(fmt.Sprintf("Total Messages: %d\n", len(messages.Messages)))
	content.WriteString("================================\n\n")

	for i, msg := range messages.Messages {
		var roleStr string
		switch msg.Role {
		case thread.RoleSystem:
			roleStr = "SYSTEM"
		case thread.RoleUser:
			roleStr = "USER"
		case thread.RoleAssistant:
			roleStr = "ASSISTANT"
		default:
			roleStr = fmt.Sprintf("UNKNOWN(%v)", msg.Role)
		}

		content.WriteString(fmt.Sprintf("--- Message %d [%s] ---\n", i+1, roleStr))
		for j, c := range msg.Contents {
			contentStr := c.AsString()
			if len(contentStr) > 5000 {
				contentStr = contentStr[:5000] + "\n... [truncated - full content too long]"
			}
			content.WriteString(fmt.Sprintf("[Content %d]\n%s\n", j+1, contentStr))
		}
		content.WriteString("\n")
	}

	_ = os.WriteFile(filename, []byte(content.String()), 0644)
}
