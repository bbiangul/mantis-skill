package engine

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/bbiangul/mantis-skill/skill"
)

// ---------------------------------------------------------------------------
// Sentinel errors
// ---------------------------------------------------------------------------

// ErrNotFound is returned by repository methods when a record does not exist.
var ErrNotFound = errors.New("record not found")

// ---------------------------------------------------------------------------
// Database model types used by the repository layer
// ---------------------------------------------------------------------------

// DBFunctionExecution is the database-layer representation of a function execution record.
type DBFunctionExecution struct {
	ID             int64     `json:"id"`
	MessageID      string    `json:"message_id"`
	ClientID       string    `json:"client_id"`
	ToolName       string    `json:"tool_name"`
	FunctionName   string    `json:"function_name"`
	Inputs         string    `json:"inputs"`
	InputsHash     string    `json:"inputs_hash"`
	Output         string    `json:"output"`
	OriginalOutput *string   `json:"original_output,omitempty"`
	ExecutedAt     time.Time `json:"executed_at"`
	ExecutionTime  int64     `json:"execution_time_ms"`
	Status         string    `json:"status"`
	CreatedAt      time.Time `json:"created_at"`
}

// DBStepResult is the database-layer representation of a step result record.
type DBStepResult struct {
	ID           int64     `json:"id"`
	MessageID    string    `json:"message_id"`
	FunctionName string    `json:"function_name"`
	ResultIndex  int       `json:"result_index"`
	ResultData   string    `json:"result_data"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// ---------------------------------------------------------------------------
// Extended ToolRepository — covers all DB access the tracker needs
// ---------------------------------------------------------------------------

// ExecutionRepository extends ToolRepository with the full set of queries the
// execution tracker requires. Host applications implement this interface.
type ExecutionRepository interface {
	// --- function executions ---
	CreateFunctionExecution(ctx context.Context, execution DBFunctionExecution) (int64, error)
	GetFunctionExecution(ctx context.Context, messageID, functionName string) (DBFunctionExecution, error)
	GetFunctionExecutionsByMessage(ctx context.Context, messageID string) ([]*DBFunctionExecution, error)
	GetFunctionExecutionsByClientAndFunction(ctx context.Context, clientID, functionName, toolName string, statusFilter []string) ([]*DBFunctionExecution, error)
	GetFunctionExecutionsByFunction(ctx context.Context, functionName, toolName string, statusFilter []string) ([]*DBFunctionExecution, error)
	GetFunctionExecutionsWithSameInputs(ctx context.Context, clientID, messageID, functionName, toolName, inputsHash string, statusFilter []string) ([]*DBFunctionExecution, error)
	GetLatestFunctionExecutionByClient(ctx context.Context, clientID, functionName, toolName string, statusFilter []string) (*DBFunctionExecution, error)

	// --- step results ---
	CreateStepResult(ctx context.Context, result DBStepResult) (int64, error)
	UpdateStepResult(ctx context.Context, result DBStepResult) error
	GetStepResult(ctx context.Context, messageID string, resultIndex int) (*DBStepResult, error)
	GetStepResultsByMessage(ctx context.Context, messageID string) ([]*DBStepResult, error)
}

// ---------------------------------------------------------------------------
// FunctionExecution — in-memory cache type (distinct from DB type)
// ---------------------------------------------------------------------------

// FunctionExecution represents a single function execution within a cycle.
type FunctionExecution struct {
	ID            int64     `json:"id"`
	MessageID     string    `json:"message_id"`
	ToolName      string    `json:"tool_name"`
	FunctionName  string    `json:"function_name"`
	Inputs        string    `json:"inputs"`
	Output        string    `json:"output"`
	ExecutedAt    time.Time `json:"executed_at"`
	ExecutionTime int64     `json:"execution_time_ms"`
	Status        string    `json:"status"`
}

// StepResult represents the result of a specific step with a result index.
// This mirrors engine/models.StepResult to avoid an import cycle (engine/models imports engine).
type StepResult struct {
	ID           int64     `json:"id"`
	MessageID    string    `json:"message_id"`
	FunctionName string    `json:"function_name"`
	ResultIndex  int       `json:"result_index"`
	ResultData   string    `json:"result_data"`
	CreatedAt    time.Time `json:"created_at"`
}

// ExecutionStatus constants
const (
	ExecutionStatusCompleted = "completed"
	ExecutionStatusFailed    = "failed"
	ExecutionStatusPending   = "pending"
)

// ---------------------------------------------------------------------------
// FunctionExecutionTracker
// ---------------------------------------------------------------------------

// FunctionExecutionTracker manages and queries function executions.
type FunctionExecutionTracker struct {
	repository ExecutionRepository
	cache      map[string]map[string]*FunctionExecution // messageID -> functionName -> execution
	// cache for step results: messageID -> resultIndex -> result
	stepResultsCache map[string]map[int]*StepResult
	mutex            sync.RWMutex
	stepMutex        sync.RWMutex // Separate mutex for step results to avoid contention

	// global / scoped cache provider (replaces BadgerDB)
	cacheProvider CacheProvider

	// gatherInfo cache for shouldBeHandledAsMessageToUser functionality
	gatherInfoCache map[string][]string
	gatherInfoMutex sync.Mutex

	// filesToShare cache for shouldBeHandledAsMessageToUser functionality with file outputs
	filesToShareCache map[string][]*MediaType
	filesToShareMutex sync.Mutex

	// responseLanguage override set by YAML functions via responseLanguage field
	responseLanguageCache map[string]string
	responseLanguageMutex sync.Mutex
}

// NewFunctionExecutionTracker creates a new tracker.
// The returned *FunctionExecutionTracker satisfies models.IFunctionExecutionTracker.
func NewFunctionExecutionTracker(repository ExecutionRepository, cacheProvider CacheProvider) (*FunctionExecutionTracker, error) {
	if repository == nil {
		return nil, errors.New("repository cannot be nil")
	}

	return &FunctionExecutionTracker{
		repository:            repository,
		cache:                 make(map[string]map[string]*FunctionExecution),
		stepResultsCache:      make(map[string]map[int]*StepResult),
		cacheProvider:         cacheProvider,
		gatherInfoCache:       make(map[string][]string),
		filesToShareCache:     make(map[string][]*MediaType),
		responseLanguageCache: make(map[string]string),
	}, nil
}

// RecordExecution records a function execution.
func (t *FunctionExecutionTracker) RecordExecution(
	ctx context.Context,
	messageID,
	clientID,
	toolName,
	functionName,
	functionDescription string,
	inputs map[string]interface{},
	inputsHash string,
	output string,
	originalOutput *string,
	startTime time.Time,
	status string,
	function *skill.Function,
) error {
	inputsJSON, err := json.Marshal(inputs)
	if err != nil {
		return fmt.Errorf("failed to marshal inputs: %w", err)
	}

	// If hash was not provided, calculate it now
	if inputsHash == "" {
		inputsHash, err = t.calculateInputsHash(inputs)
		if err != nil {
			return fmt.Errorf("failed to calculate inputs hash: %w", err)
		}
	}

	// Notify callback if present in context (uses context keys from triggers.go).
	// We use a local interface to avoid importing engine/models (which imports engine -> cycle).
	type functionExecutedNotifier interface {
		OnFunctionExecuted(ctx context.Context, stepKey, messageID, clientID, functionName, toolName, functionDescription, inputs, output string, err error, function *skill.Function)
	}
	if cb, ok := ctx.Value(CallbackInContextKey).(functionExecutedNotifier); ok && cb != nil {
		stepKey, _ := ctx.Value(StepKeyInContextKey).(string)
		cb.OnFunctionExecuted(ctx, stepKey, messageID, clientID, functionName, toolName, functionDescription, string(inputsJSON), strings.TrimSpace(output), nil, function)
	}

	// Trim whitespace from output to ensure clean data everywhere
	trimmedOutput := strings.TrimSpace(output)

	executionTime := time.Since(startTime).Milliseconds()

	execution := DBFunctionExecution{
		MessageID:      messageID,
		ClientID:       clientID,
		ToolName:       toolName,
		FunctionName:   functionName,
		Inputs:         string(inputsJSON),
		InputsHash:     inputsHash,
		Output:         trimmedOutput,
		OriginalOutput: originalOutput,
		ExecutedAt:     startTime,
		ExecutionTime:  executionTime,
		Status:         status,
	}

	id, err := t.repository.CreateFunctionExecution(ctx, execution)
	if err != nil {
		return fmt.Errorf("failed to create function execution record: %w", err)
	}

	execution.ID = id

	// Update cache
	t.mutex.Lock()
	defer t.mutex.Unlock()

	if _, ok := t.cache[messageID]; !ok {
		t.cache[messageID] = make(map[string]*FunctionExecution)
	}

	t.cache[messageID][functionName] = mapDBFunctionExecutionToFunction(execution)

	return nil
}

// calculateInputsHash computes SHA256 hash of inputs for unique checking.
func (t *FunctionExecutionTracker) calculateInputsHash(inputs map[string]interface{}) (string, error) {
	inputsJSON, err := json.Marshal(inputs)
	if err != nil {
		return "", fmt.Errorf("failed to marshal inputs for hash: %w", err)
	}
	hash := sha256.Sum256(inputsJSON)
	return fmt.Sprintf("%x", hash), nil
}

// CheckCallRuleViolationWithoutInputs checks if executing a function would violate a "once" type call rule.
// This is optimized for pre-fulfillment checking and does NOT require inputs.
func (t *FunctionExecutionTracker) CheckCallRuleViolationWithoutInputs(
	ctx context.Context,
	messageID, clientID, toolName, functionName string,
	callRule *skill.CallRule,
) (bool, string, error) {
	if callRule == nil || callRule.Type == skill.CallRuleTypeMultiple {
		return false, "", nil
	}

	if callRule.Type != skill.CallRuleTypeOnce {
		return false, "", nil
	}

	return t.checkOnceRule(ctx, messageID, clientID, toolName, functionName, callRule)
}

// CheckCallRuleViolation checks if executing a function would violate its call rule.
// Returns: (violates bool, message string, inputsHash string, error).
func (t *FunctionExecutionTracker) CheckCallRuleViolation(
	ctx context.Context,
	messageID, clientID, toolName, functionName string,
	inputs map[string]interface{},
	callRule *skill.CallRule,
) (bool, string, string, error) {
	inputsHash, err := t.calculateInputsHash(inputs)
	if err != nil {
		return false, "", "", err
	}

	if callRule == nil || callRule.Type == skill.CallRuleTypeMultiple {
		return false, "", inputsHash, nil
	}

	switch callRule.Type {
	case skill.CallRuleTypeOnce:
		violates, message, err := t.checkOnceRule(ctx, messageID, clientID, toolName, functionName, callRule)
		return violates, message, inputsHash, err
	case skill.CallRuleTypeUnique:
		violates, message, err := t.checkUniqueRule(ctx, messageID, clientID, toolName, functionName, inputsHash, callRule)
		return violates, message, inputsHash, err
	default:
		return false, "", inputsHash, fmt.Errorf("unknown call rule type: %s", callRule.Type)
	}
}

// shouldCountExecutionStatus checks if an execution status should be counted based on statusFilter.
func shouldCountExecutionStatus(status string, statusFilter []string) bool {
	if len(statusFilter) == 0 {
		return true
	}
	for _, s := range statusFilter {
		if s == skill.CallRuleStatusFilterAll {
			return true
		}
		if s == status {
			return true
		}
	}
	return false
}

// checkOnceRule checks if a function with "once" rule has already been executed.
func (t *FunctionExecutionTracker) checkOnceRule(
	ctx context.Context,
	messageID, clientID, toolName, functionName string,
	callRule *skill.CallRule,
) (bool, string, error) {
	switch callRule.Scope {
	case skill.CallRuleScopeMessage:
		hasMatchingExecution, err := t.hasExecutionWithMatchingStatus(ctx, messageID, toolName, functionName, callRule.StatusFilter)
		if err != nil {
			return false, "", err
		}
		if hasMatchingExecution {
			return true, fmt.Sprintf("Function '%s' can only be executed once per message and has already been executed", functionName), nil
		}

	case skill.CallRuleScopeUser:
		executions, err := t.repository.GetFunctionExecutionsByClientAndFunction(ctx, clientID, functionName, toolName, callRule.StatusFilter)
		if err != nil {
			return false, "", fmt.Errorf("failed to check user executions: %w", err)
		}
		if len(executions) > 0 {
			return true, fmt.Sprintf("Function '%s' can only be executed once per user and has already been executed", functionName), nil
		}

	case skill.CallRuleScopeMinimumInterval:
		lastExec, err := t.repository.GetLatestFunctionExecutionByClient(ctx, clientID, functionName, toolName, callRule.StatusFilter)
		if err != nil {
			return false, "", fmt.Errorf("failed to get latest execution: %w", err)
		}
		if lastExec != nil {
			timeSinceExec := time.Since(lastExec.ExecutedAt).Seconds()
			if timeSinceExec < float64(callRule.MinimumInterval) {
				remainingTime := float64(callRule.MinimumInterval) - timeSinceExec
				return true, fmt.Sprintf("Function '%s' can only be executed once every %d seconds. Please wait %.0f more seconds",
					functionName, callRule.MinimumInterval, remainingTime), nil
			}
		}

	case skill.CallRuleScopeCompany:
		executions, err := t.repository.GetFunctionExecutionsByFunction(ctx, functionName, toolName, callRule.StatusFilter)
		if err != nil {
			return false, "", fmt.Errorf("failed to check company executions: %w", err)
		}
		if len(executions) > 0 {
			return true, fmt.Sprintf("Function '%s' can only be executed once per company and has already been executed", functionName), nil
		}
	}

	return false, "", nil
}

// hasExecutionWithMatchingStatus checks if ANY execution for the function in this message matches the statusFilter.
func (t *FunctionExecutionTracker) hasExecutionWithMatchingStatus(ctx context.Context, messageID, toolName, functionName string, statusFilter []string) (bool, error) {
	executions, err := t.repository.GetFunctionExecutionsByMessage(ctx, messageID)
	if err != nil {
		return false, fmt.Errorf("failed to get executions for message: %w", err)
	}

	for _, exec := range executions {
		if exec.ToolName == toolName && exec.FunctionName == functionName && shouldCountExecutionStatus(exec.Status, statusFilter) {
			return true, nil
		}
	}

	return false, nil
}

// checkUniqueRule checks if a function with "unique" rule has been executed with the same inputs.
func (t *FunctionExecutionTracker) checkUniqueRule(
	ctx context.Context,
	messageID, clientID, toolName, functionName, inputsHash string,
	callRule *skill.CallRule,
) (bool, string, error) {
	var scopeClientID, scopeMessageID string

	switch callRule.Scope {
	case skill.CallRuleScopeMessage:
		scopeMessageID = messageID
	case skill.CallRuleScopeUser:
		scopeClientID = clientID
	case skill.CallRuleScopeMinimumInterval:
		scopeClientID = clientID
	case skill.CallRuleScopeCompany:
		// Company scope: no client/message filter
	}

	executions, err := t.repository.GetFunctionExecutionsWithSameInputs(
		ctx, scopeClientID, scopeMessageID, functionName, toolName, inputsHash, callRule.StatusFilter,
	)
	if err != nil {
		return false, "", fmt.Errorf("failed to check unique inputs: %w", err)
	}

	// For minimum interval scope, filter by time
	if callRule.Scope == skill.CallRuleScopeMinimumInterval {
		cutoffTime := time.Now().Add(-time.Duration(callRule.MinimumInterval) * time.Second)
		var recentExecutions []*DBFunctionExecution
		for _, exec := range executions {
			if exec.ExecutedAt.After(cutoffTime) {
				recentExecutions = append(recentExecutions, exec)
			}
		}
		executions = recentExecutions
	}

	if len(executions) > 0 {
		scopeDesc := ""
		switch callRule.Scope {
		case skill.CallRuleScopeMessage:
			scopeDesc = "in this message"
		case skill.CallRuleScopeUser:
			scopeDesc = "for this user"
		case skill.CallRuleScopeMinimumInterval:
			scopeDesc = fmt.Sprintf("within the last %d seconds", callRule.MinimumInterval)
		case skill.CallRuleScopeCompany:
			scopeDesc = "for this company"
		}
		return true, fmt.Sprintf("Function '%s' has already been executed with the same inputs %s", functionName, scopeDesc), nil
	}

	return false, "", nil
}

// RecordStepResult records the result of a step with a result index.
// Uses UPSERT logic: updates if exists, creates if not.
func (t *FunctionExecutionTracker) RecordStepResult(
	ctx context.Context,
	messageID string,
	functionName string,
	resultIndex int,
	resultData interface{},
) error {
	var resultJSON []byte
	var err error

	// Check if resultData is already a string that looks like valid JSON.
	// This avoids double-encoding when API responses are already JSON strings.
	if strData, ok := resultData.(string); ok {
		trimmed := strings.TrimSpace(strData)
		if (strings.HasPrefix(trimmed, "{") && strings.HasSuffix(trimmed, "}")) ||
			(strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]")) {
			var jsonTest interface{}
			if json.Unmarshal([]byte(strData), &jsonTest) == nil {
				resultJSON = []byte(strData)
			} else {
				resultJSON, err = jsonMarshalNoEscape(resultData)
				if err != nil {
					return fmt.Errorf("failed to marshal step result data: %w", err)
				}
			}
		} else {
			resultJSON, err = jsonMarshalNoEscape(resultData)
			if err != nil {
				return fmt.Errorf("failed to marshal step result data: %w", err)
			}
		}
	} else {
		resultJSON, err = jsonMarshalNoEscape(resultData)
		if err != nil {
			return fmt.Errorf("failed to marshal step result data: %w", err)
		}
	}

	stepResult := DBStepResult{
		MessageID:    messageID,
		FunctionName: functionName,
		ResultIndex:  resultIndex,
		ResultData:   string(resultJSON),
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	// Try to get existing step result first (UPSERT)
	existingResult, err := t.repository.GetStepResult(ctx, messageID, resultIndex)
	if err == nil && existingResult != nil {
		stepResult.ID = existingResult.ID
		stepResult.CreatedAt = existingResult.CreatedAt // Preserve original creation time
		err = t.repository.UpdateStepResult(ctx, stepResult)
		if err != nil {
			return fmt.Errorf("failed to update step result record: %w", err)
		}
	} else {
		id, createErr := t.repository.CreateStepResult(ctx, stepResult)
		if createErr != nil {
			return fmt.Errorf("failed to create step result record: %w", createErr)
		}
		stepResult.ID = id
	}

	// Update step results cache
	t.stepMutex.Lock()
	defer t.stepMutex.Unlock()

	if _, ok := t.stepResultsCache[messageID]; !ok {
		t.stepResultsCache[messageID] = make(map[int]*StepResult)
	}

	t.stepResultsCache[messageID][resultIndex] = mapDBStepResultToStepResult(stepResult)

	return nil
}

// GetStepResult retrieves a step result by message ID and result index.
func (t *FunctionExecutionTracker) GetStepResult(
	ctx context.Context,
	messageID string,
	resultIndex int,
) (*StepResult, error) {
	// First check cache
	t.stepMutex.RLock()
	if messageResults, ok := t.stepResultsCache[messageID]; ok {
		if result, ok := messageResults[resultIndex]; ok {
			t.stepMutex.RUnlock()
			return result, nil
		}
	}
	t.stepMutex.RUnlock()

	// Then check database
	dbResult, err := t.repository.GetStepResult(ctx, messageID, resultIndex)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, fmt.Errorf("no step result found for message %s with index %d", messageID, resultIndex)
		}
		return nil, fmt.Errorf("failed to get step result: %w", err)
	}

	// Update cache
	result := mapDBStepResultToStepResult(*dbResult)

	t.stepMutex.Lock()
	if _, ok := t.stepResultsCache[messageID]; !ok {
		t.stepResultsCache[messageID] = make(map[int]*StepResult)
	}
	t.stepResultsCache[messageID][resultIndex] = result
	t.stepMutex.Unlock()

	return result, nil
}

// GetStepResultField extracts a specific field from a step result.
func (t *FunctionExecutionTracker) GetStepResultField(
	ctx context.Context,
	messageID string,
	resultIndex int,
	fieldPath string,
) (interface{}, error) {
	result, err := t.GetStepResult(ctx, messageID, resultIndex)
	if err != nil {
		return nil, err
	}

	var resultData map[string]interface{}
	err = json.Unmarshal([]byte(result.ResultData), &resultData)
	if err != nil {
		return nil, fmt.Errorf("failed to parse result data as JSON: %w", err)
	}

	if fieldPath == "" {
		return resultData, nil
	}

	if value, ok := resultData[fieldPath]; ok {
		return value, nil
	}

	return nil, fmt.Errorf("field '%s' not found in result data", fieldPath)
}

// HasExecuted checks if a function has been executed for a message.
func (t *FunctionExecutionTracker) HasExecuted(ctx context.Context, messageID, functionName string) (bool, string, error) {
	// First check cache
	t.mutex.RLock()
	if messageFuncs, ok := t.cache[messageID]; ok {
		if execution, ok := messageFuncs[functionName]; ok {
			t.mutex.RUnlock()
			return true, execution.Output, nil
		}
	}
	t.mutex.RUnlock()

	// Then check database
	execution, err := t.repository.GetFunctionExecution(ctx, messageID, functionName)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return false, "", nil
		}
		return false, "", fmt.Errorf("failed to get function execution: %w", err)
	}

	// Update cache
	t.mutex.Lock()
	if _, ok := t.cache[messageID]; !ok {
		t.cache[messageID] = make(map[string]*FunctionExecution)
	}
	t.cache[messageID][functionName] = mapDBFunctionExecutionToFunction(execution)
	t.mutex.Unlock()

	return true, execution.Output, nil
}

// LoadFunctionsForMessage preloads all functions executed for a message into the cache.
func (t *FunctionExecutionTracker) LoadFunctionsForMessage(ctx context.Context, messageID string) error {
	executions, err := t.repository.GetFunctionExecutionsByMessage(ctx, messageID)
	if err != nil {
		return fmt.Errorf("failed to get function executions: %w", err)
	}

	t.mutex.Lock()
	defer t.mutex.Unlock()

	if _, ok := t.cache[messageID]; !ok {
		t.cache[messageID] = make(map[string]*FunctionExecution)
	}

	for _, execution := range executions {
		t.cache[messageID][execution.FunctionName] = mapDBFunctionExecutionToFunction(*execution)
	}

	return nil
}

// LoadStepResultsForMessage preloads all step results for a message into the cache.
func (t *FunctionExecutionTracker) LoadStepResultsForMessage(ctx context.Context, messageID string) error {
	stepResults, err := t.repository.GetStepResultsByMessage(ctx, messageID)
	if err != nil {
		return fmt.Errorf("failed to get step results: %w", err)
	}

	t.stepMutex.Lock()
	defer t.stepMutex.Unlock()

	if _, ok := t.stepResultsCache[messageID]; !ok {
		t.stepResultsCache[messageID] = make(map[int]*StepResult)
	}

	for _, result := range stepResults {
		t.stepResultsCache[messageID][result.ResultIndex] = mapDBStepResultToStepResult(*result)
	}

	return nil
}

// CheckNeeds verifies that all needed functions have been executed.
func (t *FunctionExecutionTracker) CheckNeeds(
	ctx context.Context,
	messageID string,
	needs []string,
) (allMet bool, missingNeeds []string, executionResults []string, err error) {
	if len(needs) == 0 {
		return true, nil, nil, nil
	}

	// Preload executions for this message
	err = t.LoadFunctionsForMessage(ctx, messageID)
	if err != nil {
		return false, nil, nil, fmt.Errorf("failed to load function executions: %w", err)
	}

	for _, neededFunc := range needs {
		executed, res, execErr := t.HasExecuted(ctx, messageID, neededFunc)
		if execErr != nil {
			return false, nil, nil, fmt.Errorf("error checking function execution: %w", execErr)
		}

		if !executed {
			missingNeeds = append(missingNeeds, neededFunc)
		} else {
			executionResults = append(executionResults, fmt.Sprintf("%s: %s", neededFunc, res))
		}
	}

	return len(missingNeeds) == 0, missingNeeds, executionResults, nil
}

// GetExecutionResult retrieves the result of a previous function execution.
func (t *FunctionExecutionTracker) GetExecutionResult(
	ctx context.Context,
	messageID string,
	functionName string,
) (string, error) {
	_, output, err := t.HasExecuted(ctx, messageID, functionName)
	if err != nil {
		return "", err
	}

	if output == "" {
		return "", fmt.Errorf("no output found for function %s", functionName)
	}

	return output, nil
}

// ClearCache clears all caches.
func (t *FunctionExecutionTracker) ClearCache() {
	t.mutex.Lock()
	t.cache = make(map[string]map[string]*FunctionExecution)
	t.mutex.Unlock()

	t.stepMutex.Lock()
	t.stepResultsCache = make(map[string]map[int]*StepResult)
	t.stepMutex.Unlock()
}

// ClearCacheForMessage clears in-memory caches for a specific message ID.
func (t *FunctionExecutionTracker) ClearCacheForMessage(messageID string) {
	t.mutex.Lock()
	delete(t.cache, messageID)
	t.mutex.Unlock()

	t.stepMutex.Lock()
	delete(t.stepResultsCache, messageID)
	t.stepMutex.Unlock()
}

// ---------------------------------------------------------------------------
// Global / scoped cache — delegates to CacheProvider
// ---------------------------------------------------------------------------

// generateGlobalCacheKey builds a unique cache key from tool, function and inputs.
func generateGlobalCacheKey(toolName, functionName string, inputs map[string]interface{}) (string, error) {
	inputsJSON, err := json.Marshal(inputs)
	if err != nil {
		return "", fmt.Errorf("failed to marshal inputs for cache key: %w", err)
	}

	h := sha256.New()
	h.Write([]byte(fmt.Sprintf("%s:%s:%s", toolName, functionName, string(inputsJSON))))
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

// GetFromGlobalCache checks if a result exists in the global cache and is still valid.
func (t *FunctionExecutionTracker) GetFromGlobalCache(ctx context.Context, toolName, functionName string, inputs map[string]interface{}) (string, bool, error) {
	if t.cacheProvider == nil {
		return "", false, nil
	}

	cacheKey, err := generateGlobalCacheKey(toolName, functionName, inputs)
	if err != nil {
		return "", false, err
	}

	output, found, err := t.cacheProvider.Get(ctx, cacheKey)
	if err != nil || !found {
		return "", false, nil
	}

	return output, true, nil
}

// GetSummaryFromGlobalCache retrieves a summary from the global cache.
func (t *FunctionExecutionTracker) GetSummaryFromGlobalCache(ctx context.Context, toolName, functionName string, inputs map[string]interface{}) (string, bool, error) {
	if t.cacheProvider == nil {
		return "", false, nil
	}

	cacheKey, err := generateGlobalCacheKey(toolName, functionName, inputs)
	if err != nil {
		return "", false, err
	}

	summaryKey := cacheKey + ":summary"
	summary, found, err := t.cacheProvider.Get(ctx, summaryKey)
	if err != nil || !found || summary == "" {
		return "", false, nil
	}

	return summary, true, nil
}

// AddToGlobalCache stores a function result in the global cache.
func (t *FunctionExecutionTracker) AddToGlobalCache(ctx context.Context, toolName, functionName string, inputs map[string]interface{}, output, summary string, ttl int) error {
	if ttl <= 0 || t.cacheProvider == nil {
		return nil
	}

	cacheKey, err := generateGlobalCacheKey(toolName, functionName, inputs)
	if err != nil {
		return err
	}

	ttlDuration := time.Duration(ttl) * time.Second

	if err := t.cacheProvider.Set(ctx, cacheKey, output, ttlDuration); err != nil {
		return err
	}

	// Store summary separately if provided
	if summary != "" {
		summaryKey := cacheKey + ":summary"
		if err := t.cacheProvider.Set(ctx, summaryKey, summary, ttlDuration); err != nil {
			// Log but don't fail — summary is best-effort
			if logger != nil {
				logger.Warnf("failed to cache summary for %s.%s: %v", toolName, functionName, err)
			}
		}
	}

	return nil
}

// generateScopedCacheKey generates a cache key based on scope, scopeID, and optionally inputs.
func generateScopedCacheKey(toolName, functionName string, scope skill.CacheScope, scopeID string, inputs map[string]interface{}) (string, error) {
	var keyParts []string
	keyParts = append(keyParts, toolName, functionName)

	if scope != skill.CacheScopeGlobal && scope != "" {
		keyParts = append(keyParts, string(scope), scopeID)
	}

	if inputs != nil && len(inputs) > 0 {
		inputsJSON, err := json.Marshal(inputs)
		if err != nil {
			return "", fmt.Errorf("failed to marshal inputs for cache key: %w", err)
		}
		keyParts = append(keyParts, string(inputsJSON))
	}

	h := sha256.New()
	h.Write([]byte(strings.Join(keyParts, ":")))
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

// GetFromScopedCache retrieves a cached result using scoped caching.
func (t *FunctionExecutionTracker) GetFromScopedCache(ctx context.Context, toolName, functionName string, scope skill.CacheScope, scopeID string, inputs map[string]interface{}) (string, bool, error) {
	if t.cacheProvider == nil {
		return "", false, nil
	}

	cacheKey, err := generateScopedCacheKey(toolName, functionName, scope, scopeID, inputs)
	if err != nil {
		return "", false, err
	}

	output, found, err := t.cacheProvider.Get(ctx, cacheKey)
	if err != nil || !found {
		return "", false, nil
	}

	if logger != nil {
		logger.Infof("Scoped cache HIT (scope: %s) for %s.%s", scope, toolName, functionName)
	}
	return output, true, nil
}

// AddToScopedCache stores a function result in the scoped cache.
func (t *FunctionExecutionTracker) AddToScopedCache(ctx context.Context, toolName, functionName string, scope skill.CacheScope, scopeID string, inputs map[string]interface{}, output string, ttl int) error {
	if ttl <= 0 || t.cacheProvider == nil {
		return nil
	}

	cacheKey, err := generateScopedCacheKey(toolName, functionName, scope, scopeID, inputs)
	if err != nil {
		return err
	}

	ttlDuration := time.Duration(ttl) * time.Second

	if err := t.cacheProvider.Set(ctx, cacheKey, output, ttlDuration); err != nil {
		return err
	}

	if logger != nil {
		logger.Infof("Added to scoped cache (scope: %s, scopeID: %s) for %s.%s (TTL: %d seconds)",
			scope, scopeID, toolName, functionName, ttl)
	}
	return nil
}

// StartCacheCleanup is a no-op — cache provider handles its own cleanup.
func (t *FunctionExecutionTracker) StartCacheCleanup(interval time.Duration) {
	// CacheProvider implementations manage their own expiration/cleanup.
}

// ---------------------------------------------------------------------------
// GatherInfo cache
// ---------------------------------------------------------------------------

// AddGatherInfo stores information that should be surfaced to the user.
func (t *FunctionExecutionTracker) AddGatherInfo(messageID string, info string) {
	if info == "" {
		return
	}
	t.gatherInfoMutex.Lock()
	defer t.gatherInfoMutex.Unlock()
	t.gatherInfoCache[messageID] = append(t.gatherInfoCache[messageID], info)
}

// GetAndClearGatherInfo retrieves all gathered info for a message and clears the cache.
func (t *FunctionExecutionTracker) GetAndClearGatherInfo(messageID string) []string {
	t.gatherInfoMutex.Lock()
	defer t.gatherInfoMutex.Unlock()
	items := t.gatherInfoCache[messageID]
	delete(t.gatherInfoCache, messageID)
	return items
}

// ---------------------------------------------------------------------------
// FilesToShare cache
// ---------------------------------------------------------------------------

// AddFileToShare stores a file that should be attached to the user response.
func (t *FunctionExecutionTracker) AddFileToShare(messageID string, file *MediaType) {
	if file == nil || messageID == "" {
		return
	}
	t.filesToShareMutex.Lock()
	defer t.filesToShareMutex.Unlock()
	t.filesToShareCache[messageID] = append(t.filesToShareCache[messageID], file)
}

// GetAndClearFilesToShare retrieves all files to share for a message and clears the cache.
func (t *FunctionExecutionTracker) GetAndClearFilesToShare(messageID string) []*MediaType {
	t.filesToShareMutex.Lock()
	defer t.filesToShareMutex.Unlock()
	files := t.filesToShareCache[messageID]
	delete(t.filesToShareCache, messageID)
	return files
}

// ---------------------------------------------------------------------------
// Response language cache
// ---------------------------------------------------------------------------

// SetResponseLanguage stores the response language override for a message.
func (t *FunctionExecutionTracker) SetResponseLanguage(messageID string, language string) {
	if messageID == "" || language == "" {
		return
	}
	t.responseLanguageMutex.Lock()
	defer t.responseLanguageMutex.Unlock()
	t.responseLanguageCache[messageID] = language
}

// GetResponseLanguage retrieves the response language override for a message.
// Returns empty string if no override was set.
func (t *FunctionExecutionTracker) GetResponseLanguage(messageID string) string {
	t.responseLanguageMutex.Lock()
	defer t.responseLanguageMutex.Unlock()
	lang := t.responseLanguageCache[messageID]
	delete(t.responseLanguageCache, messageID)
	return lang
}

// ---------------------------------------------------------------------------
// Mapping helpers
// ---------------------------------------------------------------------------

func mapDBFunctionExecutionToFunction(db DBFunctionExecution) *FunctionExecution {
	return &FunctionExecution{
		ID:            db.ID,
		MessageID:     db.MessageID,
		ToolName:      db.ToolName,
		FunctionName:  db.FunctionName,
		Inputs:        db.Inputs,
		Output:        db.Output,
		ExecutedAt:    db.ExecutedAt,
		ExecutionTime: db.ExecutionTime,
		Status:        db.Status,
	}
}

func mapDBStepResultToStepResult(db DBStepResult) *StepResult {
	return &StepResult{
		ID:           db.ID,
		MessageID:    db.MessageID,
		FunctionName: db.FunctionName,
		ResultIndex:  db.ResultIndex,
		ResultData:   db.ResultData,
		CreatedAt:    db.CreatedAt,
	}
}

// ---------------------------------------------------------------------------
// Inlined utilities
// ---------------------------------------------------------------------------

// jsonMarshalNoEscape marshals data to JSON without escaping HTML characters.
// Standard json.Marshal escapes &, <, > to \u0026, \u003c, \u003e for HTML safety.
// This function preserves these characters as-is, which is necessary for URLs
// containing query parameters with & that should not be escaped.
func jsonMarshalNoEscape(data interface{}) ([]byte, error) {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	err := encoder.Encode(data)
	if err != nil {
		return nil, err
	}
	result := buf.Bytes()
	if len(result) > 0 && result[len(result)-1] == '\n' {
		result = result[:len(result)-1]
	}
	return result, nil
}
