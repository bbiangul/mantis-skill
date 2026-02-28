package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	tool_engine_models "github.com/bbiangul/mantis-skill/engine/models"
	tool_protocol "github.com/bbiangul/mantis-skill/skill"
	"github.com/google/uuid"
)

// executeInitiateWorkflowOperation executes the initiate_workflow operation
func (t *YAMLDefinedTool) executeInitiateWorkflowOperation(
	ctx context.Context,
	messageID string,
	inputs map[string]interface{},
) (string, map[int]interface{}, error) {
	if logger != nil {
		logger.Debugf("Executing initiate_workflow operation for %s.%s", t.tool.Name, t.function.Name)
	}

	// Check for mock service in context (test mode only - zero-cost in production)
	// Production code NEVER sets TestMockServiceKey, so this check is always false in production
	if mockSvc, ok := ctx.Value(tool_engine_models.TestMockServiceKey).(tool_engine_models.IMockService); ok && mockSvc != nil {
		functionKey := GenerateFunctionKey(t.tool.Name, t.function.Name)
		// Check each step for mocks
		for _, step := range t.function.Steps {
			if mockSvc.ShouldMock("initiate_workflow", functionKey, step.Name) {
				if response, exists := mockSvc.GetMockResponseValue(functionKey, step.Name); exists {
					mockSvc.RecordCall(functionKey, step.Name, "initiate_workflow", inputs, response, "")
					if logger != nil {
						logger.Infof("[TEST MOCK] Returning mock for initiate_workflow %s.%s", functionKey, step.Name)
					}
					stepResults := make(map[int]interface{})
					stepResults[0] = response
					// Convert response to string
					var outputStr string
					switch v := response.(type) {
					case string:
						outputStr = v
					default:
						jsonBytes, _ := json.Marshal(v)
						outputStr = string(jsonBytes)
					}
					return outputStr, stepResults, nil
				}
			}
		}
	}

	// Check if workflowInitiator is available
	if t.workflowInitiator == nil {
		return "", nil, fmt.Errorf("workflowInitiator is not available for this operation")
	}

	stepResults := make(map[int]interface{})
	var output strings.Builder
	var successCount, failCount int

	for stepIndex, step := range t.function.Steps {
		if logger != nil {
			logger.Debugf("Processing step %d/%d: %s (ForEach: %t)",
				stepIndex+1, len(t.function.Steps), step.Name, step.ForEach != nil)
		}

		if step.ForEach != nil {
			// Handle foreach iteration
			err := t.executeForEachInitiateWorkflowStep(ctx, messageID, step, inputs, stepResults, &successCount, &failCount)
			if err != nil {
				return "", nil, fmt.Errorf("error executing foreach step '%s': %w", step.Name, err)
			}
		} else {
			// Handle single workflow initiation (no loop context)
			result, err := t.executeInitiateWorkflowStep(ctx, step, inputs, nil)
			if err != nil {
				if logger != nil {
					logger.Errorf("Failed to initiate workflow in step '%s': %v", step.Name, err)
				}
				failCount++
				output.WriteString(fmt.Sprintf("Failed to initiate workflow: %v\n", err))
				continue
			}

			successCount++
			output.WriteString(fmt.Sprintf("Initiated workflow for user: %s\n", result))

			if step.ResultIndex > 0 {
				stepResults[step.ResultIndex] = result
			}
		}
	}

	// Summary output
	summaryMsg := fmt.Sprintf("Workflow initiation complete: %d successful, %d failed", successCount, failCount)
	if logger != nil {
		logger.Infof("%s", summaryMsg)
	}
	output.WriteString(summaryMsg)

	return output.String(), stepResults, nil
}

// iterationError represents an error that occurred during a foreach iteration
type iterationError struct {
	index int
	err   error
}

// executeForEachInitiateWorkflowStep executes a step with foreach iteration
// This function is thread-safe and handles concurrent workflow initiations
func (t *YAMLDefinedTool) executeForEachInitiateWorkflowStep(
	ctx context.Context,
	messageID string,
	step tool_protocol.Step,
	inputs map[string]interface{},
	stepResults map[int]interface{},
	successCount *int,
	failCount *int,
) error {
	startTime := time.Now()

	// Get the variable path from the items field (e.g., "$conversations.conversations" -> "conversations.conversations")
	itemsPath := strings.TrimPrefix(step.ForEach.Items, "$")

	// Use NavigatePath to resolve nested paths like "conversations.conversations"
	rawItemsValue, err := t.variableReplacer.NavigatePath(inputs, itemsPath)
	if err != nil || rawItemsValue == nil {
		if logger != nil {
			logger.Errorf("variable '%s' not found in inputs: %v", step.ForEach.Items, err)
		}
		return fmt.Errorf("variable '%s' not found in inputs", step.ForEach.Items)
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

	// Early return for empty items
	if len(items) == 0 {
		if logger != nil {
			logger.Warnf("No items to process in foreach - skipping")
		}
		return nil
	}

	if logger != nil {
		logger.Infof("Initiating workflows for %d items", len(items))
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

	// If shouldSkip is present, execute sequentially (supports multiple conditions)
	shouldSkipConditions := step.ForEach.GetShouldSkipConditions()
	if len(shouldSkipConditions) > 0 {
		names := make([]string, len(shouldSkipConditions))
		for i, cond := range shouldSkipConditions {
			names[i] = cond.Name
		}
		if logger != nil {
			logger.Infof("Executing forEach with %d shouldSkip condition(s): %v sequentially", len(shouldSkipConditions), names)
		}
		return t.executeForEachInitiateWorkflowSequential(ctx, messageID, step, items, indexVar, itemVar, inputs, stepResults, successCount, failCount)
	}

	// Thread-safe counters and error collection
	var (
		mu           sync.Mutex
		localSuccess int
		localFail    int
		errors       []iterationError
		wg           sync.WaitGroup
	)

	// Only allocate results if we need to store them
	var results []interface{}
	if step.ResultIndex != 0 {
		results = make([]interface{}, len(items))
	}

	// Optimize worker count based on batch size
	workers := 2
	if len(items) > 100 {
		workers = 5
	}
	if len(items) > 1000 {
		workers = 10
	}

	// Create channel with optimal buffer size
	type workItem struct {
		index int
		item  interface{}
	}
	itemsChan := make(chan workItem, workers*2)

	// Fill the channel in a separate goroutine to avoid blocking
	go func() {
		for i, item := range items {
			select {
			case itemsChan <- workItem{index: i, item: item}:
			case <-ctx.Done():
				close(itemsChan)
				return
			}
		}
		close(itemsChan)
	}()

	// Start workers
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					// Context cancelled - stop processing
					if logger != nil {
						logger.Warnf("Worker %d stopped due to context cancellation", workerID)
					}
					return
				case work, ok := <-itemsChan:
					if !ok {
						// Channel closed - no more work
						return
					}

					loopContext := &tool_engine_models.LoopContext{
						IndexVar: indexVar,
						ItemVar:  itemVar,
						Index:    work.index,
						Item:     work.item,
					}

					// Process step parameters with loop context
					// Pass nil for stepResults since this is initiate workflow context
					processedParams, err := t.processStepParametersWithLoop(ctx, messageID, step.With, inputs, loopContext, nil)
					if err != nil {
						if logger != nil {
							logger.Errorf("Error processing step parameters for iteration %d: %v", work.index, err)
						}
						mu.Lock()
						localFail++
						errors = append(errors, iterationError{index: work.index, err: fmt.Errorf("parameter processing failed: %w", err)})
						mu.Unlock()
						continue
					}

					// Create a temporary step with processed parameters
					tempStep := step
					tempStep.With = processedParams

					// Pass loopContext for context.params variable resolution
					result, err := t.executeInitiateWorkflowStep(ctx, tempStep, inputs, loopContext)

					// Update counters and results with mutex protection
					mu.Lock()
					if err != nil {
						if logger != nil {
							logger.Errorf("Error initiating workflow for iteration %d: %v", work.index, err)
						}
						localFail++
						errors = append(errors, iterationError{index: work.index, err: err})
					} else {
						localSuccess++
						if results != nil {
							results[work.index] = result
						}
					}
					mu.Unlock()
				}
			}
		}(w)
	}

	// Wait for all workers with timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// All workers completed successfully
	case <-ctx.Done():
		// Context cancelled while waiting
		if logger != nil {
			logger.Warnf("Context cancelled while waiting for workers to complete")
		}
		return ctx.Err()
	}

	// Update the shared counters
	*successCount += localSuccess
	*failCount += localFail

	// Store the results if ResultIndex is specified
	if step.ResultIndex != 0 && results != nil {
		stepResults[step.ResultIndex] = results
	}

	duration := time.Since(startTime)
	if logger != nil {
		logger.Infof("Completed workflow initiation for %d items: %d succeeded, %d failed, duration: %v",
			len(items), localSuccess, localFail, duration)
	}

	// Return aggregated errors if any occurred
	if len(errors) > 0 {
		var errMessages []string
		for _, iterErr := range errors {
			errMessages = append(errMessages, fmt.Sprintf("iteration %d: %v", iterErr.index, iterErr.err))
		}
		return fmt.Errorf("encountered %d errors during foreach execution:\n  %s",
			len(errors), strings.Join(errMessages, "\n  "))
	}

	return nil
}

// executeInitiateWorkflowStep initiates a single workflow
// loopContext is optional and used for variable resolution in forEach loops
func (t *YAMLDefinedTool) executeInitiateWorkflowStep(
	ctx context.Context,
	step tool_protocol.Step,
	inputs map[string]interface{},
	loopContext *tool_engine_models.LoopContext,
) (string, error) {
	// Process step.With parameters with variable replacement
	// This resolves $variableName references using the fulfilled inputs
	// Example: "$workflowType" -> "user" when inputs contains {workflowType: "user"}
	processedWith := make(map[string]interface{})
	for key, value := range step.With {
		if strValue, ok := value.(string); ok {
			// Replace variables in string values
			resolved, err := t.variableReplacer.ReplaceVariables(ctx, strValue, inputs)
			if err != nil {
				if logger != nil {
					logger.Warnf("Failed to replace variables in step.With[%s]: %v", key, err)
				}
				processedWith[key] = value // Keep original on error
			} else {
				processedWith[key] = resolved
			}
		} else {
			// Non-string values pass through as-is
			processedWith[key] = value
		}
	}
	// Use processed values for the rest of the function
	step.With = processedWith

	// Extract workflow configuration
	workflowTypeRaw, ok := step.With["workflowType"]
	if !ok {
		return "", fmt.Errorf("workflowType is required in step configuration")
	}

	messageRaw, ok := step.With["message"]
	if !ok {
		return "", fmt.Errorf("message is required in step configuration")
	}

	workflowType, ok := workflowTypeRaw.(string)
	if !ok {
		return "", fmt.Errorf("workflowType must be a string, got %T", workflowTypeRaw)
	}

	message, ok := messageRaw.(string)
	if !ok {
		return "", fmt.Errorf("message must be a string, got %T", messageRaw)
	}

	// Validate workflowType
	if workflowType != "user" && workflowType != "team" {
		return "", fmt.Errorf("workflowType must be 'user' or 'team', got '%s'", workflowType)
	}

	// Determine userId - either from userId field or user object
	var userId string
	var channel string = "synthetic" // default channel
	var session string               // session for message sending (e.g., waha session)

	userIDRaw, hasUserId := step.With["userId"]
	userObjRaw, hasUserObj := step.With["user"]

	if hasUserId {
		// Option 1: userId is provided
		var ok bool
		userId, ok = userIDRaw.(string)
		if !ok {
			return "", fmt.Errorf("userId must be a string, got %T", userIDRaw)
		}

		// Check if user exists, if not, create them with additional fields
		if t.workflowInitiator != nil {
			exists, err := t.workflowInitiator.UserExists(ctx, userId)
			if err != nil {
				if logger != nil {
					logger.Errorf("Failed to check if user exists: %v", err)
				}
				t.recordFailedWorkflowExecution(ctx, userId, "user_validation_error", err.Error())
				return "", fmt.Errorf("failed to validate user existence: %w", err)
			}

			if !exists {
				// User doesn't exist - check if channel is provided for user creation
				channelRaw, hasChannel := step.With["channel"]
				if !hasChannel {
					// No channel provided - cannot create user
					if logger != nil {
						logger.Warnf("User '%s' does not exist and no channel provided for creation", userId)
					}
					t.recordFailedWorkflowExecution(ctx, userId, "user_not_found", fmt.Sprintf("user '%s' does not exist and no channel provided for creation", userId))
					return "", fmt.Errorf("user '%s' does not exist; provide 'channel' field to auto-create", userId)
				}

				// Set channel for the synthetic message
				if channelStr, ok := channelRaw.(string); ok {
					channel = channelStr
				}

				// Build and create user with provided fields
				newUser := t.buildUserFromStepConfig(userId, step.With)
				if logger != nil {
					logger.Infof("Creating new user '%s' for workflow initiation", userId)
				}
				if err := t.workflowInitiator.CreateUser(ctx, newUser); err != nil {
					if logger != nil {
						logger.Errorf("Failed to create user '%s': %v", userId, err)
					}
					t.recordFailedWorkflowExecution(ctx, userId, "user_creation_failed", err.Error())
					return "", fmt.Errorf("failed to create user '%s': %w", userId, err)
				}

				if logger != nil {
					logger.Infof("Successfully created user '%s' for workflow initiation", userId)
				}
			}
		}
	} else if hasUserObj {
		// Option 2: user object is provided - create user with generated UUID
		var userObj map[string]interface{}
		switch v := userObjRaw.(type) {
		case map[string]interface{}:
			userObj = v
		case string:
			// Handle case where variable replacement returned a JSON string
			if err := json.Unmarshal([]byte(v), &userObj); err != nil {
				return "", fmt.Errorf("user must be an object, got string that is not valid JSON: %v", err)
			}
		default:
			return "", fmt.Errorf("user must be an object, got %T", userObjRaw)
		}

		// Generate UUID for the new user
		userId = uuid.New().String()

		// Build user from object
		newUser := t.buildUserFromObject(userId, userObj)

		// Get channel from user object
		if channelVal, ok := userObj["channel"].(string); ok {
			channel = channelVal
		}

		if t.workflowInitiator != nil {
			if logger != nil {
				logger.Infof("Creating new user '%s' from user object for workflow initiation", userId)
			}
			if err := t.workflowInitiator.CreateUser(ctx, newUser); err != nil {
				if logger != nil {
					logger.Errorf("Failed to create user '%s': %v", userId, err)
				}
				t.recordFailedWorkflowExecution(ctx, userId, "user_creation_failed", err.Error())
				return "", fmt.Errorf("failed to create user: %w", err)
			}
			if logger != nil {
				logger.Infof("Successfully created user '%s' for workflow initiation", userId)
			}
		}
	} else {
		return "", fmt.Errorf("either userId or user object is required in step configuration")
	}

	// Parse context field (can be string or object with value+params)
	contextConfig, err := tool_protocol.ParseWorkflowContext(step.With["context"])
	if err != nil {
		return "", fmt.Errorf("failed to parse context: %w", err)
	}
	contextStr := contextConfig.Value

	// Process context params - validate, resolve variables, and build function-keyed map
	functionParams := make(map[string]map[string]interface{})
	for _, param := range contextConfig.Params {
		// First, resolve variables in the function name itself (e.g., "$targetFunction" -> "myTool.myFunction")
		resolvedFunctionName := param.Function
		if strings.HasPrefix(param.Function, "$") {
			var replaceErr error
			if loopContext != nil {
				resolvedFunctionName, replaceErr = t.variableReplacer.ReplaceVariablesWithLoop(ctx, param.Function, inputs, loopContext)
			} else {
				resolvedFunctionName, replaceErr = t.variableReplacer.ReplaceVariables(ctx, param.Function, inputs)
			}
			if replaceErr != nil {
				if logger != nil {
					logger.Warnf("Could not resolve function name variable '%s': %v (skipping this context param)", param.Function, replaceErr)
				}
				continue
			}
		}

		// Skip if the function name wasn't resolved (still has $ prefix or is empty)
		if strings.HasPrefix(resolvedFunctionName, "$") || resolvedFunctionName == "" {
			if logger != nil {
				logger.Debugf("Context param function '%s' resolved to empty or unresolved '%s' (skipping)", param.Function, resolvedFunctionName)
			}
			continue
		}

		// Collect input names for validation
		inputNames := make([]string, 0, len(param.Inputs))
		for inputName := range param.Inputs {
			inputNames = append(inputNames, inputName)
		}

		// Validate function exists and input names are valid, also resolves short names
		functionKey, validateErr := t.validateAndResolveFunctionParam(ctx, resolvedFunctionName, inputNames)
		if validateErr != nil {
			return "", fmt.Errorf("invalid context param for function '%s': %w", resolvedFunctionName, validateErr)
		}

		// Apply variable replacement to input values (same pattern as needs.params in tool_engine.go)
		// Use ReplaceVariablesWithLoop when loopContext is provided (for forEach iterations)
		resolvedInputs := make(map[string]interface{})
		for inputName, inputValue := range param.Inputs {
			// Handle string values with variable replacement
			if strVal, ok := inputValue.(string); ok {
				var resolved string
				var replaceErr error

				// Use ReplaceVariablesWithLoop if we have loop context (for forEach loop variables like $deal.field)
				if loopContext != nil {
					resolved, replaceErr = t.variableReplacer.ReplaceVariablesWithLoop(ctx, strVal, inputs, loopContext)
				} else {
					resolved, replaceErr = t.variableReplacer.ReplaceVariables(ctx, strVal, inputs)
				}

				if replaceErr != nil {
					// Resolution failed - skip this param and let the target function handle it normally
					if logger != nil {
						logger.Warnf("Failed to resolve context param '%s' for function '%s': %v (skipping pre-fill)", inputName, functionKey, replaceErr)
					}
					continue
				}

				// Check if the value was actually resolved (not left as $varName or resolved to empty)
				// With keepUnresolved: false (default), unresolved variables become empty strings
				// This matches the pattern in tool_engine.go ExecuteDependencies
				if (strings.HasPrefix(strVal, "$") && resolved == "") ||
					(strings.HasPrefix(resolved, "$") && resolved == strVal) {
					// Value wasn't resolved - skip this param
					if logger != nil {
						logger.Warnf("Context param '%s' for function '%s' references unavailable variable '%s' (skipping pre-fill)", inputName, functionKey, strVal)
					}
					continue
				}

				resolvedInputs[inputName] = resolved
			} else {
				// Non-string values pass through as-is
				resolvedInputs[inputName] = inputValue
			}
		}

		// Only add to functionParams if we have resolved inputs
		if len(resolvedInputs) > 0 {
			functionParams[functionKey] = resolvedInputs
			if logger != nil {
				logger.Debugf("Context param for function '%s': %v", functionKey, resolvedInputs)
			}
		} else {
			if logger != nil {
				logger.Debugf("No resolved inputs for function '%s' after variable resolution", functionKey)
			}
		}
	}

	// Optional shouldSend field
	shouldSend := true
	if shouldSendRaw, exists := step.With["shouldSend"]; exists {
		if shouldSendBool, ok := shouldSendRaw.(bool); ok {
			shouldSend = shouldSendBool
		}
	}

	// Override channel if explicitly set at step level (for userId path)
	if channelRaw, exists := step.With["channel"]; exists {
		if channelStr, ok := channelRaw.(string); ok {
			channel = channelStr
		}
	}

	// Extract session if provided (required for channels like waha to send messages)
	if sessionRaw, exists := step.With["session"]; exists {
		if sessionStr, ok := sessionRaw.(string); ok {
			session = sessionStr
		}
	}

	// If session not provided and channel is not synthetic, try to get from user's last message
	if session == "" && channel != "synthetic" && t.workflowInitiator != nil {
		lastSession, err := t.workflowInitiator.GetUserLastSession(ctx, userId)
		if err != nil {
			if logger != nil {
				logger.Warnf("Failed to get user's last session: %v", err)
			}
		} else if lastSession != "" {
			session = lastSession
			if logger != nil {
				logger.Debugf("Using session from user's last message: %s", session)
			}
		}
	}

	// Extract optional enforcedWorkflow field - bypasses LLM categorization when set
	var enforcedWorkflow string
	if workflowRaw, exists := step.With["workflow"]; exists {
		if workflowStr, ok := workflowRaw.(string); ok {
			enforcedWorkflow = workflowStr
		}
	}

	if logger != nil {
		logger.Infof("Initiating %s workflow for user %s with message: %s (context params: %d functions, channel: %s, session: %s, enforced workflow: %s)", workflowType, userId, message, len(functionParams), channel, session, enforcedWorkflow)
	}

	// Create synthetic message with context
	syntheticMsg := t.createSyntheticMessage(ctx, userId, message, workflowType, shouldSend, channel, session, contextStr)

	// Initiate the workflow through the workflow initiator
	err = t.workflowInitiator.InitiateWorkflow(ctx, syntheticMsg, contextStr, functionParams, enforcedWorkflow)
	if err != nil {
		return "", fmt.Errorf("failed to initiate workflow: %w", err)
	}

	if logger != nil {
		logger.Infof("Successfully initiated %s workflow for user %s", workflowType, userId)
	}
	return userId, nil
}

// buildUserFromStepConfig builds a User struct from step configuration fields
// Used when userId is provided but user doesn't exist
func (t *YAMLDefinedTool) buildUserFromStepConfig(userId string, stepWith map[string]interface{}) User {
	user := User{
		ID:        userId,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// Extract optional user creation fields
	if firstName, ok := stepWith["firstName"].(string); ok {
		user.FirstName = firstName
	}
	if lastName, ok := stepWith["lastName"].(string); ok {
		user.LastName = lastName
	}
	if email, ok := stepWith["email"].(string); ok {
		user.Email = email
	}
	if phoneNumber, ok := stepWith["phoneNumber"].(string); ok {
		user.PhoneNumber = phoneNumber
	}

	return user
}

// buildUserFromObject builds a User struct from a user object
// Used when user object is provided instead of userId
func (t *YAMLDefinedTool) buildUserFromObject(userId string, userObj map[string]interface{}) User {
	user := User{
		ID:        userId,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// Extract fields from user object
	if firstName, ok := userObj["firstName"].(string); ok {
		user.FirstName = firstName
	}
	if lastName, ok := userObj["lastName"].(string); ok {
		user.LastName = lastName
	}
	if email, ok := userObj["email"].(string); ok {
		user.Email = email
	}
	if phoneNumber, ok := userObj["phoneNumber"].(string); ok {
		user.PhoneNumber = phoneNumber
	}
	// Note: channel is used for the synthetic message, not stored in user

	return user
}

// createSyntheticMessage creates a synthetic message to trigger a workflow
// Note: This message is NOT persisted to the database as per user's requirement
func (t *YAMLDefinedTool) createSyntheticMessage(
	ctx context.Context,
	userId, message, workflowType string,
	shouldReply bool,
	channel string,
	session string,
	contextForAgent string,
) Message {
	// Session priority:
	// 1. Explicitly provided session (from YAML config or user's last message)
	// 2. For "team" workflow type, use company's AI session
	// 3. Otherwise, empty string
	if session == "" && workflowType == "team" {
		if company, ok := ctx.Value(CompanyInContextKey).(*CompanyInfo); ok && company != nil {
			session = company.AISessionID
		}
	}

	return Message{
		Id:              uuid.New().String(),
		ClientID:        userId,
		Timestamp:       int(time.Now().Unix()),
		Body:            message,
		From:            userId,
		FromMe:          false,
		Channel:         channel,
		Session:         session,
		ShouldReply:     shouldReply,
		HasMedia:        false,
		IsSynthetic:     true, // Mark as synthetic to prevent DB persistence
		ContextForAgent: contextForAgent,
		WorkflowType:    workflowType, // "user" or "team" - controls workflow routing in AsyncProcessing
	}
}

// recordFailedWorkflowExecution records a failed workflow execution to function_executions table
func (t *YAMLDefinedTool) recordFailedWorkflowExecution(ctx context.Context, userId, reason, errorDetails string) {
	if t.executionTracker == nil {
		return
	}

	inputs := map[string]interface{}{
		"userId": userId,
		"reason": reason,
	}

	err := t.executionTracker.RecordExecution(
		ctx,
		"",     // messageID - empty for failed validation
		userId, // clientID
		t.tool.Name,
		t.function.Name,
		t.function.Description,
		inputs,
		"", // inputsHash - empty for failed validation
		fmt.Sprintf("Workflow initiation failed: %s - %s", reason, errorDetails),
		nil, // originalOutput
		time.Now(),
		"failed",
		t.function,
	)
	if err != nil {
		if logger != nil {
			logger.Errorf("Failed to record failed workflow execution: %v", err)
		}
	}
}

// resolveFunctionKey resolves a short function name to full toolName.functionName format
// If the function name already contains a dot, it's assumed to be a full key and returned as-is
// Otherwise, searches all loaded tools to find a matching function
// Returns error if function is not found or if it's ambiguous (exists in multiple tools)
func (t *YAMLDefinedTool) resolveFunctionKey(ctx context.Context, shortName string) (string, error) {
	if t.toolEngine == nil {
		return "", fmt.Errorf("toolEngine is not available for function key resolution")
	}

	// Search through all loaded tools
	allTools := t.toolEngine.GetAllTools()
	var matches []string

	for _, tool := range allTools {
		for _, fn := range tool.Functions {
			if fn.Name == shortName {
				matches = append(matches, tool.Name+"."+fn.Name)
			}
		}
	}

	if len(matches) == 0 {
		return "", fmt.Errorf("function '%s' not found in any loaded tool", shortName)
	}

	if len(matches) > 1 {
		return "", fmt.Errorf("function '%s' is ambiguous, found in multiple tools: %v. Use full key (toolName.functionName)", shortName, matches)
	}

	return matches[0], nil
}

// validateAndResolveFunctionParam validates a function param and returns the resolved function key
// Validates:
// 1. Function exists (resolves short names to full keys)
// 2. Input names exist in the target function's input definition
func (t *YAMLDefinedTool) validateAndResolveFunctionParam(ctx context.Context, functionRef string, inputNames []string) (string, error) {
	if t.toolEngine == nil {
		return "", fmt.Errorf("toolEngine is not available for function validation")
	}

	allTools := t.toolEngine.GetAllTools()

	var resolvedKey string
	var targetTool *tool_protocol.Tool
	var targetFunc *tool_protocol.Function

	if strings.Contains(functionRef, ".") {
		// Full key format: toolName.functionName
		parts := strings.SplitN(functionRef, ".", 2)
		toolName, funcName := parts[0], parts[1]

		// Find the tool
		for i := range allTools {
			if allTools[i].Name == toolName {
				targetTool = &allTools[i]
				break
			}
		}
		if targetTool == nil {
			return "", fmt.Errorf("tool '%s' not found for context param function '%s'", toolName, functionRef)
		}

		// Find the function
		for i := range targetTool.Functions {
			if targetTool.Functions[i].Name == funcName {
				targetFunc = &targetTool.Functions[i]
				break
			}
		}
		if targetFunc == nil {
			return "", fmt.Errorf("function '%s' not found in tool '%s'", funcName, toolName)
		}

		resolvedKey = functionRef
	} else {
		// Short name - search all tools
		var matches []struct {
			tool *tool_protocol.Tool
			fn   *tool_protocol.Function
			key  string
		}

		for i := range allTools {
			for j := range allTools[i].Functions {
				if allTools[i].Functions[j].Name == functionRef {
					matches = append(matches, struct {
						tool *tool_protocol.Tool
						fn   *tool_protocol.Function
						key  string
					}{
						tool: &allTools[i],
						fn:   &allTools[i].Functions[j],
						key:  allTools[i].Name + "." + functionRef,
					})
				}
			}
		}

		if len(matches) == 0 {
			return "", fmt.Errorf("function '%s' not found in any loaded tool", functionRef)
		}
		if len(matches) > 1 {
			toolNames := make([]string, len(matches))
			for i, m := range matches {
				toolNames[i] = m.tool.Name
			}
			return "", fmt.Errorf("function '%s' is ambiguous, found in tools: %v. Use full key (toolName.functionName)", functionRef, toolNames)
		}

		targetTool = matches[0].tool
		targetFunc = matches[0].fn
		resolvedKey = matches[0].key
	}

	// Validate input names exist in the target function
	validInputs := make(map[string]bool)
	for _, inp := range targetFunc.Input {
		validInputs[inp.Name] = true
	}

	for _, inputName := range inputNames {
		if !validInputs[inputName] {
			// Collect valid input names for error message
			validNames := make([]string, 0, len(validInputs))
			for name := range validInputs {
				validNames = append(validNames, name)
			}
			return "", fmt.Errorf("input '%s' not found in function '%s.%s'. Valid inputs: %v", inputName, targetTool.Name, targetFunc.Name, validNames)
		}
	}

	return resolvedKey, nil
}

// executeForEachInitiateWorkflowSequential executes forEach workflow initiation sequentially
// Used when shouldSkip is present, as we need to evaluate shouldSkip before each iteration
func (t *YAMLDefinedTool) executeForEachInitiateWorkflowSequential(
	ctx context.Context,
	messageID string,
	step tool_protocol.Step,
	items []interface{},
	indexVar, itemVar string,
	inputs map[string]interface{},
	stepResults map[int]interface{},
	successCount *int,
	failCount *int,
) error {
	startTime := time.Now()

	var results []interface{}
	if step.ResultIndex != 0 {
		results = make([]interface{}, 0, len(items))
	}

	localSuccess := 0
	localFail := 0

	// Get shouldSkip conditions once outside the loop for efficiency
	shouldSkipConditions := step.ForEach.GetShouldSkipConditions()

	// Log shouldSkip configuration if present
	if len(shouldSkipConditions) > 0 {
		names := make([]string, len(shouldSkipConditions))
		for i, cond := range shouldSkipConditions {
			names[i] = cond.Name
		}
		if logger != nil {
			logger.Debugf("Executing forEach workflow with %d shouldSkip condition(s): %v", len(shouldSkipConditions), names)
		}
	}

	for i, item := range items {
		select {
		case <-ctx.Done():
			if logger != nil {
				logger.Warnf("Context cancelled during forEach workflow initiation at index %d/%d", i, len(items))
			}
			return ctx.Err()
		default:
		}

		if logger != nil {
			logger.Debugf("Processing forEach workflow item %d/%d", i+1, len(items))
		}

		// Check shouldSkip conditions at the START of each iteration
		// Uses OR logic: if ANY condition returns true, skip the iteration
		if len(shouldSkipConditions) > 0 {
			skip, err := t.shouldSkipIterationMultiple(ctx, messageID, shouldSkipConditions, i, item, indexVar, itemVar, inputs)
			if err != nil {
				// Error already logged in shouldSkipIterationMultiple, skip this iteration
				if logger != nil {
					logger.Debugf("Skipping workflow item %d/%d due to shouldSkip error", i+1, len(items))
				}
				localFail++
				continue
			}
			if skip {
				if logger != nil {
					logger.Debugf("Skipping workflow item %d/%d (shouldSkip returned true)", i+1, len(items))
				}
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
		// Pass nil for stepResults since this is initiate workflow context
		processedParams, err := t.processStepParametersWithLoop(ctx, messageID, step.With, inputs, loopContext, nil)
		if err != nil {
			if logger != nil {
				logger.Errorf("Error processing step parameters for iteration %d: %v", i, err)
			}
			localFail++
			continue
		}

		// Create a temporary step with processed parameters
		tempStep := step
		tempStep.With = processedParams

		// Pass loopContext for context.params variable resolution
		result, err := t.executeInitiateWorkflowStep(ctx, tempStep, inputs, loopContext)

		if err != nil {
			if logger != nil {
				logger.Errorf("Error initiating workflow for iteration %d: %v", i, err)
			}
			localFail++
		} else {
			localSuccess++
			if results != nil {
				results = append(results, result)
			}
		}
	}

	// Update the shared counters
	*successCount += localSuccess
	*failCount += localFail

	// Store the results if ResultIndex is specified
	if step.ResultIndex != 0 && results != nil {
		stepResults[step.ResultIndex] = results
	}

	duration := time.Since(startTime)
	if logger != nil {
		logger.Infof("Completed sequential workflow initiation for %d items: %d succeeded, %d failed, duration: %v",
			len(items), localSuccess, localFail, duration)
	}

	return nil
}
