package engine

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/bbiangul/mantis-skill/engine/models"
	"github.com/bbiangul/mantis-skill/skill"
	"github.com/bbiangul/mantis-skill/types"
)

// ---------------------------------------------------------------------------
// Local interfaces — kept minimal to avoid pulling in heavy external deps.
// ---------------------------------------------------------------------------

// CallbackTaskService is the subset of task-service operations the callback needs.
// Host applications provide a concrete implementation.
type CallbackTaskService interface {
	Plan(ctx context.Context, messageID, clientID string, dto interface{}, isNew, isNewMessage bool) (interface{}, error)
	Aggregate(ctx context.Context, eventKey, messageID, toolName, rationale, output, eventType string)
	MarkStepAsCompleted(ctx context.Context, messageID, eventKey, result string)
	ExpectHumanFeedback(ctx context.Context, messageID, clientID, eventKey string, humanSupportType types.HumanSupportType, humanQaId string, humanSupportMessage types.HumanSupportMessage)
	ConcludeWorkflow(ctx context.Context, messageID, clientID string, summary types.WorkflowSummary, stateStore models.StateStore, isNewMessage bool)
}

// CallbackToolRepository is the subset of ToolRepository needed for DB-fallback.
type CallbackToolRepository interface {
	GetFunctionExecutionsByClientID(ctx context.Context, clientID string) ([]CallbackFunctionExecution, error)
}

// CallbackFunctionExecution is the lightweight struct returned by the fallback query.
type CallbackFunctionExecution struct {
	ExecutedAt   time.Time
	MessageID    string
	ClientID     string
	FunctionName string
	Output       string
	Inputs       string
}

// MemoryRAG is the interface for persisting memories.
type MemoryRAG interface {
	PersistMemoryWithTopic(ctx context.Context, memory, topic string, metadata map[string]interface{}) error
}

// MemoryRAGFactory creates a MemoryRAG instance. Nil means memory persistence is disabled.
type MemoryRAGFactory func(ctx context.Context) (MemoryRAG, error)

// ---------------------------------------------------------------------------
// Context key constants — replaces connect-ai/pkg/domain/agents/prompts keys.
// ---------------------------------------------------------------------------

const (
	contextKeyUserMessage     = "userMessageInContext"
	contextKeyContextForAgent = "contextForAgentInContext"
)

// CompletionTypeUserConfirmation mirrors the original constant.
const CompletionTypeUserConfirmation = "user_confirmation"

// ---------------------------------------------------------------------------
// ExecutorAgenticWorkflowCallback — the main callback implementation.
// ---------------------------------------------------------------------------

// ExecutorAgenticWorkflowCallback implements models.AgenticWorkflowCallback
// and records events in the workflow event cache.
type ExecutorAgenticWorkflowCallback struct {
	Cache            *models.WorkflowEventCache
	TaskService      CallbackTaskService
	inputsCache      map[string]map[string]interface{}
	inputsMutex      sync.RWMutex
	toolRepository   CallbackToolRepository
	executionTracker models.IFunctionExecutionTracker
	memoryRAGFactory MemoryRAGFactory
}

var callbackInstance *ExecutorAgenticWorkflowCallback
var callbackInitOnce sync.Once

// NewExecutorAgenticWorkflowCallback creates (or returns) the singleton callback.
// taskService may be nil if no task service is available.
func NewExecutorAgenticWorkflowCallback(taskService CallbackTaskService) (models.AgenticWorkflowCallback, error) {
	callbackInitOnce.Do(func() {
		callbackInstance = &ExecutorAgenticWorkflowCallback{
			Cache:       models.NewWorkflowEventCache(),
			TaskService: taskService,
			inputsCache: make(map[string]map[string]interface{}),
		}
		callbackInstance.startBackgroundCleanup()
	})
	return callbackInstance, nil
}

// GetExecutorAgenticWorkflowCallback returns the singleton (nil-safe).
func GetExecutorAgenticWorkflowCallback() (*ExecutorAgenticWorkflowCallback, error) {
	if callbackInstance == nil {
		return nil, fmt.Errorf("ExecutorAgenticWorkflowCallback instance is nil")
	}
	return callbackInstance, nil
}

// SetTaskService replaces the task service (useful for late binding).
func (c *ExecutorAgenticWorkflowCallback) SetTaskService(taskService interface{}) {
	if ts, ok := taskService.(CallbackTaskService); ok {
		c.TaskService = ts
	}
}

// SetToolRepository sets the tool repository for DB fallback queries.
func (c *ExecutorAgenticWorkflowCallback) SetToolRepository(repo CallbackToolRepository) {
	c.toolRepository = repo
}

// SetExecutionTracker sets the execution tracker for cache cleanup.
func (c *ExecutorAgenticWorkflowCallback) SetExecutionTracker(tracker models.IFunctionExecutionTracker) {
	c.executionTracker = tracker
}

// SetMemoryRAGFactory configures the factory used to create MemoryRAG instances.
func (c *ExecutorAgenticWorkflowCallback) SetMemoryRAGFactory(factory MemoryRAGFactory) {
	c.memoryRAGFactory = factory
}

// ---------------------------------------------------------------------------
// AgenticWorkflowCallback interface methods
// ---------------------------------------------------------------------------

func (c *ExecutorAgenticWorkflowCallback) OnWorkflowPlanned(ctx context.Context, messageID string, clientID string, state *models.CoordinatorState, humanVerified bool, isNewMessage bool) (interface{}, error) {
	if c == nil {
		return nil, fmt.Errorf("ExecutorAgenticWorkflowCallback is nil")
	}
	if state.ActiveWorkflow == nil {
		return nil, fmt.Errorf("workflow is nil")
	}
	if c.Cache == nil {
		return nil, fmt.Errorf("cache is nil")
	}

	event := models.WorkflowEvent{
		Timestamp: time.Now(),
		EventType: models.WorkflowEventTypePlanned,
		MessageID: messageID,
		ClientID:  clientID,
		Planned: models.WorkflowPlanned{
			Category: state.ActiveWorkflow.CategoryName,
			IsNew:    humanVerified,
			Steps:    state.ActiveWorkflow.Steps,
		},
	}

	var task interface{}
	var err error
	if c.TaskService != nil {
		type stateDTO struct {
			TaskID         string
			ActiveWorkflow *types.Workflow
		}
		dto := stateDTO{
			TaskID:         state.TaskID,
			ActiveWorkflow: state.ActiveWorkflow,
		}
		task, err = c.TaskService.Plan(ctx, messageID, clientID, dto, humanVerified, isNewMessage)
	}

	c.Cache.AddEvent(event)
	return task, err
}

func (c *ExecutorAgenticWorkflowCallback) OnStepProposed(ctx context.Context, eventKey string, messageID string, clientID string, toolName string, rationale string) {
	if c == nil || c.Cache == nil {
		return
	}

	event := models.WorkflowEvent{
		Key:       eventKey,
		Timestamp: time.Now(),
		EventType: models.WorkflowEventTypeProposed,
		MessageID: messageID,
		ClientID:  clientID,
		Step:      eventKey,
		Rationale: rationale,
	}

	if c.TaskService != nil {
		c.TaskService.Aggregate(ctx, eventKey, messageID, toolName, rationale, "", string(models.WorkflowEventTypeProposed))
	}
	c.Cache.AddEvent(event)
}

func (c *ExecutorAgenticWorkflowCallback) OnStepPausedDueHumanSupportRequired(ctx context.Context, eventKey, messageID, clientID, step string, humanSupportType types.HumanSupportType, humanQaId string, humanSupportMessage types.HumanSupportMessage) {
	if c == nil || c.Cache == nil {
		return
	}

	event := models.WorkflowEvent{
		Key:       eventKey,
		Timestamp: time.Now(),
		EventType: models.WorkflowEventTypePausedDueHumanSupportRequired,
		MessageID: messageID,
		ClientID:  clientID,
		Step:      step,
		Rationale: fmt.Sprintf("Human support required. Type: %s, ID: %s, Message: %v", humanSupportType, humanQaId, humanSupportMessage),
	}

	if c.TaskService != nil {
		c.TaskService.ExpectHumanFeedback(ctx, messageID, clientID, eventKey, humanSupportType, humanQaId, humanSupportMessage)
	}
	c.Cache.AddEvent(event)
}

func (c *ExecutorAgenticWorkflowCallback) OnStepExecuted(ctx context.Context, eventKey, messageID string, clientID string, toolname string, result string, err error) {
	if c == nil || c.Cache == nil {
		return
	}

	errorStr := ""
	if err != nil {
		errorStr = err.Error()
	}

	outputMessages := strings.Split(result, "|||")

	event := models.WorkflowEvent{
		Key:       eventKey,
		Timestamp: time.Now(),
		EventType: models.WorkflowEventTypeExecuted,
		MessageID: messageID,
		ClientID:  clientID,
		Step:      toolname,
		Result:    outputMessages[0],
		Error:     errorStr,
	}

	if c.TaskService != nil {
		c.TaskService.MarkStepAsCompleted(ctx, messageID, eventKey, result)
	}
	c.Cache.AddEvent(event)
}

func (c *ExecutorAgenticWorkflowCallback) OnFunctionExecuted(ctx context.Context, eventKey, messageID string, clientID string, functionName, toolName, functionDescription string, inputs string, output string, err error, function *skill.Function) {
	if c == nil || c.Cache == nil {
		return
	}

	errorStr := ""
	if err != nil {
		errorStr = err.Error()
	}

	event := models.WorkflowEvent{
		Key:       eventKey,
		Timestamp: time.Now(),
		EventType: models.WorkflowEventTypeFunctionExecuted,
		MessageID: messageID,
		ClientID:  clientID,
		Step:      functionName,
		Result:    fmt.Sprintf("Tool: %s | Function: %s | Result: %s", eventKey, functionName, output),
		Inputs:    inputs,
		Error:     errorStr,
	}

	if c.TaskService != nil {
		c.TaskService.Aggregate(ctx, eventKey, messageID, toolName, inputs, output, string(models.WorkflowEventTypeFunctionExecuted))
	}
	c.Cache.AddEvent(event)

	// Generate and persist memory in background (if configured)
	go c.generateAndPersistMemory(ctx, eventKey, messageID, clientID, functionName, toolName, inputs, output, err)
}

func (c *ExecutorAgenticWorkflowCallback) OnDependencyExecuted(ctx context.Context, messageID string, clientID string, functionName, dependencyFunctionName string, result string, err error) {
	if c == nil || c.Cache == nil {
		return
	}

	errorStr := ""
	if err != nil {
		errorStr = err.Error()
	}

	event := models.WorkflowEvent{
		Timestamp: time.Now(),
		EventType: models.WorkflowEventTypeDependencyExecuted,
		MessageID: messageID,
		ClientID:  clientID,
		Step:      dependencyFunctionName,
		Result:    result,
		Error:     errorStr,
	}

	c.Cache.AddEvent(event)
}

func (c *ExecutorAgenticWorkflowCallback) OnInputFulfillDependencyExecuted(ctx context.Context, messageID string, clientID string, inputFieldName, inputFieldDescription, functionExecuted string, result string, err error) {
	if c == nil || c.Cache == nil {
		return
	}

	errorStr := ""
	if err != nil {
		errorStr = err.Error()
	}

	event := models.WorkflowEvent{
		Timestamp: time.Now(),
		EventType: models.WorkflowEventTypeInputFulfillDependencyExecuted,
		MessageID: messageID,
		ClientID:  clientID,
		Step:      functionExecuted,
		Result:    fmt.Sprintf("Input %s (%s) fulfilled: %s", inputFieldName, inputFieldDescription, result),
		Error:     errorStr,
	}

	c.Cache.AddEvent(event)
}

func (c *ExecutorAgenticWorkflowCallback) OnWorkflowCompleted(ctx context.Context, messageID string, clientID string, summary types.WorkflowSummary, stateStore models.StateStore, isNewMessage bool) {
	if c == nil || c.Cache == nil {
		return
	}

	summaryStr := fmt.Sprintf("Final Result: %s, Execution Time: %v, Completion Type: %s, Steps Executed: %d",
		summary.FinalResult, summary.ExecutionTime, summary.CompletionType, len(summary.ExecutedSteps))

	event := models.WorkflowEvent{
		Timestamp: time.Now(),
		EventType: models.WorkflowEventTypeCompleted,
		MessageID: messageID,
		ClientID:  clientID,
		Result:    summaryStr,
	}

	if c.TaskService != nil {
		c.TaskService.ConcludeWorkflow(ctx, messageID, clientID, summary, stateStore, isNewMessage)
	}
	c.Cache.AddEvent(event)
}

func (c *ExecutorAgenticWorkflowCallback) OnStepPausedDueApprovalRequired(ctx context.Context, stepKey, messageID, clientID, toolName, functionName string, checkpoint *types.ExecutionCheckpoint) {
	if c == nil || c.Cache == nil {
		return
	}

	event := models.WorkflowEvent{
		Key:          stepKey,
		Timestamp:    time.Now(),
		EventType:    models.WorkflowEventTypePausedDueApprovalRequired,
		MessageID:    messageID,
		ClientID:     clientID,
		Step:         stepKey,
		Result:       fmt.Sprintf("Execution paused - team approval required for %s.%s", toolName, functionName),
		CheckpointID: checkpoint.ID,
		PauseReason:  "team_approval_required",
		Details: map[string]interface{}{
			"tool_name":     toolName,
			"function_name": functionName,
			"checkpoint_id": checkpoint.ID,
			"session_id":    checkpoint.SessionID,
		},
	}

	c.Cache.AddEvent(event)
}

func (c *ExecutorAgenticWorkflowCallback) OnStepPausedDueMissingInput(ctx context.Context, stepKey, messageID, clientID, toolName, functionName string, checkpoint *types.ExecutionCheckpoint) {
	if c == nil || c.Cache == nil {
		return
	}

	event := models.WorkflowEvent{
		Key:          stepKey,
		Timestamp:    time.Now(),
		EventType:    models.WorkflowEventTypePausedDueMissingInput,
		MessageID:    messageID,
		ClientID:     clientID,
		Step:         stepKey,
		Result:       fmt.Sprintf("Execution paused - missing input required for %s.%s", toolName, functionName),
		CheckpointID: checkpoint.ID,
		PauseReason:  "missing_input",
		Details: map[string]interface{}{
			"tool_name":     toolName,
			"function_name": functionName,
			"checkpoint_id": checkpoint.ID,
			"session_id":    checkpoint.SessionID,
		},
	}

	c.Cache.AddEvent(event)
}

func (c *ExecutorAgenticWorkflowCallback) OnStepPausedDueRequiringUserConfirmation(ctx context.Context, stepKey, messageID, clientID, toolName, functionName string, checkpoint *types.ExecutionCheckpoint) {
	if c == nil || c.Cache == nil {
		return
	}

	event := models.WorkflowEvent{
		Key:          stepKey,
		Timestamp:    time.Now(),
		EventType:    models.WorkflowEventTypePausedDueMissingUserConfirmation,
		MessageID:    messageID,
		ClientID:     clientID,
		Step:         stepKey,
		Result:       fmt.Sprintf("Execution paused - requires user confirmation for %s.%s", toolName, functionName),
		CheckpointID: checkpoint.ID,
		PauseReason:  CompletionTypeUserConfirmation,
		Details: map[string]interface{}{
			"tool_name":     toolName,
			"function_name": functionName,
			"checkpoint_id": checkpoint.ID,
			"session_id":    checkpoint.SessionID,
		},
	}

	c.Cache.AddEvent(event)
}

func (c *ExecutorAgenticWorkflowCallback) OnWorkflowPaused(ctx context.Context, messageID, clientID string, currentStep string, reason string, checkpoint *types.ExecutionCheckpoint) {
	if c == nil || c.Cache == nil {
		return
	}

	event := models.WorkflowEvent{
		Timestamp:    time.Now(),
		EventType:    models.WorkflowEventTypePaused,
		MessageID:    messageID,
		ClientID:     clientID,
		Step:         currentStep,
		Result:       fmt.Sprintf("Workflow paused at step: %s", currentStep),
		CheckpointID: checkpoint.ID,
		PauseReason:  reason,
		Details: map[string]interface{}{
			"current_step":  currentStep,
			"pause_reason":  reason,
			"checkpoint_id": checkpoint.ID,
			"session_id":    checkpoint.SessionID,
		},
	}

	c.Cache.AddEvent(event)
}

func (c *ExecutorAgenticWorkflowCallback) OnWorkflowResumed(ctx context.Context, messageID, clientID string, checkpoint *types.ExecutionCheckpoint) {
	if c == nil || c.Cache == nil {
		return
	}

	event := models.WorkflowEvent{
		Timestamp:    time.Now(),
		EventType:    models.WorkflowEventTypeResumed,
		MessageID:    messageID,
		ClientID:     clientID,
		Step:         checkpoint.StepKey,
		Result:       fmt.Sprintf("Workflow resumed from checkpoint: %s", checkpoint.ID),
		CheckpointID: checkpoint.ID,
		Details: map[string]interface{}{
			"checkpoint_id": checkpoint.ID,
			"session_id":    checkpoint.SessionID,
		},
	}

	c.Cache.AddEvent(event)
}

func (c *ExecutorAgenticWorkflowCallback) OnStatePersisted(ctx context.Context, messageID, clientID string, checkpointID string) {
	if c == nil || c.Cache == nil {
		return
	}

	event := models.WorkflowEvent{
		Timestamp:    time.Now(),
		EventType:    models.WorkflowEventTypeStatePersisted,
		MessageID:    messageID,
		ClientID:     clientID,
		Result:       fmt.Sprintf("Execution state saved: %s", checkpointID),
		CheckpointID: checkpointID,
		Details: map[string]interface{}{
			"checkpoint_id": checkpointID,
		},
	}

	c.Cache.AddEvent(event)
}

func (c *ExecutorAgenticWorkflowCallback) OnStateRestored(ctx context.Context, messageID, clientID string, checkpointID string) {
	if c == nil || c.Cache == nil {
		return
	}

	event := models.WorkflowEvent{
		Timestamp:    time.Now(),
		EventType:    models.WorkflowEventTypeStateRestored,
		MessageID:    messageID,
		ClientID:     clientID,
		Result:       fmt.Sprintf("Execution state restored: %s", checkpointID),
		CheckpointID: checkpointID,
		Details: map[string]interface{}{
			"checkpoint_id": checkpointID,
		},
	}

	c.Cache.AddEvent(event)
}

func (c *ExecutorAgenticWorkflowCallback) OnScratchpadUpdated(ctx context.Context, messageID, clientID string, event models.WorkflowEvent) {
	if c == nil || c.Cache == nil {
		return
	}
	c.Cache.AddEvent(event)
}

// GetEvents retrieves events based on the grouping parameter.
// For clientID lookups, falls back to database if cache is empty.
func (c *ExecutorAgenticWorkflowCallback) GetEvents(id string, group models.CacheGroup) []models.WorkflowEvent {
	events := c.Cache.GetEvents(id, group)

	if len(events) == 0 && group == models.CACHE_GROUPED_BY_CLIENT_ID {
		events = c.getEventsFromDBByClientID(id)
	}

	return events
}

func (c *ExecutorAgenticWorkflowCallback) GetEventsString(ctx context.Context, id string, group models.CacheGroup, status models.WorkflowEventType) string {
	return c.Cache.GetEventsString(ctx, id, group, status)
}

func (c *ExecutorAgenticWorkflowCallback) GetEventsStringWithContext(ctx context.Context, id string, group models.CacheGroup, status models.WorkflowEventType) string {
	return c.Cache.GetEventsStringWithContext(ctx, id, group, status)
}

// RestoreEvents restores workflow events from serialized data.
func (c *ExecutorAgenticWorkflowCallback) RestoreEvents(ctx context.Context, events []interface{}) error {
	for _, eventInterface := range events {
		if eventMap, ok := eventInterface.(map[string]interface{}); ok {
			event := models.WorkflowEvent{}

			if key, ok := eventMap["Key"].(string); ok {
				event.Key = key
			}
			if timestamp, ok := eventMap["Timestamp"].(string); ok {
				if t, err := time.Parse(time.RFC3339, timestamp); err == nil {
					event.Timestamp = t
				}
			} else if timestamp, ok := eventMap["Timestamp"].(time.Time); ok {
				event.Timestamp = timestamp
			}
			if eventType, ok := eventMap["EventType"].(string); ok {
				event.EventType = models.WorkflowEventType(eventType)
			}
			if clientID, ok := eventMap["ClientID"].(string); ok {
				event.ClientID = clientID
			}
			if messageID, ok := eventMap["MessageID"].(string); ok {
				event.MessageID = messageID
			}
			if step, ok := eventMap["Step"].(string); ok {
				event.Step = step
			}
			if rationale, ok := eventMap["Rationale"].(string); ok {
				event.Rationale = rationale
			}
			if result, ok := eventMap["Result"].(string); ok {
				event.Result = result
			}
			if errorStr, ok := eventMap["Error"].(string); ok {
				event.Error = errorStr
			}
			if inputs, ok := eventMap["Inputs"].(string); ok {
				event.Inputs = inputs
			}
			if checkpointID, ok := eventMap["CheckpointID"].(string); ok {
				event.CheckpointID = checkpointID
			}
			if pauseReason, ok := eventMap["PauseReason"].(string); ok {
				event.PauseReason = pauseReason
			}
			if details, ok := eventMap["Details"].(map[string]interface{}); ok {
				event.Details = details
			}

			c.Cache.AddEvent(event)
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Inputs cache
// ---------------------------------------------------------------------------

func (c *ExecutorAgenticWorkflowCallback) OnInputsFulfilled(ctx context.Context, messageID, toolName, functionName string, inputs map[string]interface{}) {
	c.inputsMutex.Lock()
	defer c.inputsMutex.Unlock()

	key := c.buildInputKey(messageID, toolName, functionName)
	c.inputsCache[key] = inputs
}

func (c *ExecutorAgenticWorkflowCallback) GetFulfilledInputs(messageID, toolName, functionName string) (map[string]interface{}, bool) {
	c.inputsMutex.RLock()
	defer c.inputsMutex.RUnlock()

	key := c.buildInputKey(messageID, toolName, functionName)
	inputs, exists := c.inputsCache[key]
	return inputs, exists
}

func (c *ExecutorAgenticWorkflowCallback) buildInputKey(messageID, toolName, functionName string) string {
	return messageID + ":" + toolName + ":" + functionName
}

func (c *ExecutorAgenticWorkflowCallback) GetAllFulfilledInputsForMessage(messageID string) map[string]map[string]interface{} {
	c.inputsMutex.RLock()
	defer c.inputsMutex.RUnlock()

	result := make(map[string]map[string]interface{})
	prefix := messageID + ":"

	for key, inputs := range c.inputsCache {
		if len(key) > len(prefix) && key[:len(prefix)] == prefix {
			parts := key[len(prefix):]
			colonIdx := -1
			for i, ch := range parts {
				if ch == ':' {
					colonIdx = i
					break
				}
			}
			if colonIdx > 0 {
				toolName := parts[:colonIdx]
				functionName := parts[colonIdx+1:]
				functionKey := toolName + "." + functionName
				result[functionKey] = inputs
			}
		}
	}

	return result
}

// ---------------------------------------------------------------------------
// Background cleanup
// ---------------------------------------------------------------------------

func (c *ExecutorAgenticWorkflowCallback) startBackgroundCleanup() {
	go func() {
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			c.Cache.CleanupStaleEntries(5 * time.Minute)
		}
	}()
}

// ClearInputsCacheForMessage removes all inputs cache entries for a specific message ID.
func (c *ExecutorAgenticWorkflowCallback) ClearInputsCacheForMessage(messageID string) {
	c.inputsMutex.Lock()
	defer c.inputsMutex.Unlock()

	prefix := messageID + ":"
	for key := range c.inputsCache {
		if strings.HasPrefix(key, prefix) {
			delete(c.inputsCache, key)
		}
	}
}

// ClearAllCachesForMessage clears all in-memory caches for a specific message ID.
func (c *ExecutorAgenticWorkflowCallback) ClearAllCachesForMessage(messageID string) {
	c.ClearInputsCacheForMessage(messageID)

	if c.executionTracker != nil {
		c.executionTracker.ClearCacheForMessage(messageID)
	}
}

// ---------------------------------------------------------------------------
// DB fallback
// ---------------------------------------------------------------------------

func (c *ExecutorAgenticWorkflowCallback) getEventsFromDBByClientID(clientID string) []models.WorkflowEvent {
	if c.toolRepository == nil {
		return []models.WorkflowEvent{}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	executions, err := c.toolRepository.GetFunctionExecutionsByClientID(ctx, clientID)
	if err != nil {
		if logger != nil {
			logger.WithError(err).Warnf("Failed to get function executions from DB for cache fallback")
		}
		return []models.WorkflowEvent{}
	}
	if len(executions) == 0 {
		return []models.WorkflowEvent{}
	}

	events := make([]models.WorkflowEvent, 0, len(executions))
	for _, exec := range executions {
		events = append(events, models.WorkflowEvent{
			Timestamp: exec.ExecutedAt,
			EventType: models.WorkflowEventTypeExecuted,
			MessageID: exec.MessageID,
			ClientID:  exec.ClientID,
			Step:      exec.FunctionName,
			Result:    exec.Output,
			Inputs:    exec.Inputs,
		})
	}

	return events
}

// ---------------------------------------------------------------------------
// Memory generation (background)
// ---------------------------------------------------------------------------

func (c *ExecutorAgenticWorkflowCallback) generateAndPersistMemory(ctx context.Context, eventKey, messageID, clientID, functionName, toolName, inputs, output string, err error) {
	defer func() {
		if r := recover(); r != nil {
			if logger != nil {
				logger.Errorf("Panic in memory generation: %v", r)
			}
		}
	}()

	// Skip private functions
	if len(functionName) > 0 && functionName[0] >= 'a' && functionName[0] <= 'z' {
		return
	}

	if c.memoryRAGFactory == nil {
		return
	}

	// Extract step rationale from cache
	stepRationale := ""
	events := c.Cache.GetEvents(messageID, models.CACHE_GROUPED_BY_MESSAGE_ID)
	for _, event := range events {
		if event.Key == eventKey && event.EventType == models.WorkflowEventTypeProposed {
			stepRationale = event.Rationale
			break
		}
	}

	userMessage := ""
	contextForAgent := ""
	if val := ctx.Value(contextKeyUserMessage); val != nil {
		if str, ok := val.(string); ok {
			userMessage = str
		}
	}
	if val := ctx.Value(contextKeyContextForAgent); val != nil {
		if str, ok := val.(string); ok {
			contextForAgent = str
		}
	}

	executionData := ExecutionMemoryData{
		EventKey:        eventKey,
		MessageID:       messageID,
		ClientID:        clientID,
		FunctionName:    functionName,
		ToolName:        toolName,
		Inputs:          inputs,
		Output:          output,
		Error:           err,
		Timestamp:       time.Now(),
		StepRationale:   stepRationale,
		UserMessage:     userMessage,
		ContextForAgent: contextForAgent,
	}

	memoryGenerator := NewMemoryGenerator()
	memory, genErr := memoryGenerator.GenerateMemory(ctx, executionData)
	if genErr != nil {
		if logger != nil {
			logger.Errorf("Failed to generate memory for function %s: %v", functionName, genErr)
		}
		return
	}

	memory += fmt.Sprintf("\n related to the clientID %s, output by the function %s at %s", clientID, functionName, time.Now().String())

	metadata := map[string]interface{}{
		"client_id":     clientID,
		"message_id":    messageID,
		"event_key":     eventKey,
		"function_name": functionName,
		"tool_name":     toolName,
		"timestamp":     time.Now().Format(time.RFC3339),
		"user_message":  userMessage,
		"has_error":     err != nil,
	}
	if err != nil {
		metadata["error_message"] = err.Error()
	}

	memoryRAG, ragErr := c.memoryRAGFactory(ctx)
	if ragErr != nil {
		if logger != nil {
			logger.Errorf("Failed to create memory RAG: %v", ragErr)
		}
		return
	}

	persistErr := memoryRAG.PersistMemoryWithTopic(ctx, memory, "function_executed", metadata)
	if persistErr != nil {
		if logger != nil {
			logger.Errorf("Failed to persist memory for function %s: %v", functionName, persistErr)
		}
		return
	}

	if logger != nil {
		logger.Debugf("Successfully generated and persisted memory for function: %s", functionName)
	}
}
