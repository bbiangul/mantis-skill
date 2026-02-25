package engine

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/bbiangul/mantis-skill/skill"
	"github.com/robfig/cron/v3"
)

// -----------------------------------------------------------------------
// Narrow interfaces — only the methods the trigger system actually needs.
// These are structurally compatible with engine/models.IToolEngine etc.,
// so any concrete type that satisfies the full interface also satisfies these.
// Defined here to avoid an import cycle (engine -> engine/models -> engine).
// -----------------------------------------------------------------------

// TriggerToolEngine is the subset of IToolEngine used by the trigger system.
type TriggerToolEngine interface {
	GetAllTools() []skill.Tool
	ExecutionTracker() TriggerExecutionTracker
	GetWorkflowInitiator() TriggerWorkflowInitiator
}

// TriggerExecutionTracker is the subset of IFunctionExecutionTracker needed by triggers.
type TriggerExecutionTracker interface{}

// TriggerWorkflowInitiator is the subset of IWorkflowInitiator needed by triggers.
type TriggerWorkflowInitiator interface{}

// TriggerVariableReplacer is the subset of IVariableReplacer used by triggers.
type TriggerVariableReplacer interface{}

// TriggerInputFulfiller is the subset of IInputFulfiller used by triggers.
type TriggerInputFulfiller interface{}

// TriggerAgenticCallback is the subset of AgenticWorkflowCallback used by triggers.
type TriggerAgenticCallback interface {
	OnStepExecuted(ctx context.Context, stepKey string, messageID string, clientID string, toolname string, output string, error error)
}

// -----------------------------------------------------------------------
// Context keys — locally defined, replacing connect-ai/pkg/domain/agents/prompts
// -----------------------------------------------------------------------

// contextKeyType prevents collisions with other packages using plain strings.
type contextKeyType string

const (
	// ChannelInContextKey identifies the communication channel (e.g. "email", "whatsapp").
	ChannelInContextKey contextKeyType = "channelInContextKey"

	// MessageInContextKey carries the engine.Message through the context.
	MessageInContextKey contextKeyType = "message"

	// CallbackInContextKey carries the AgenticWorkflowCallback through the context.
	CallbackInContextKey contextKeyType = "callbackInContextKey"

	// StepKeyInContextKey carries the unique step key for execution tracking.
	StepKeyInContextKey contextKeyType = "stepKeyInContextKey"
)

// -----------------------------------------------------------------------
// Meeting context — locally defined, replacing variable_replacer.go types.
// -----------------------------------------------------------------------

// meetingContextKeyType is an unexported type for the meeting context key.
type meetingContextKeyType struct{}

// MeetingContextKey is the context key for meeting information.
var MeetingContextKey = meetingContextKeyType{}

// MeetingContext holds meeting event information for $MEETING variable.
type MeetingContext struct {
	BotID     string
	Event     string    // "start" or "end"
	Timestamp time.Time
}

// -----------------------------------------------------------------------
// Trigger queue
// -----------------------------------------------------------------------

// TriggerQueue holds messages waiting to be processed by triggers.
type TriggerQueue struct {
	items    []TriggerQueueItem
	mu       sync.Mutex
	notifyCh chan struct{} // Notification channel for new items
}

// TriggerQueueItem represents a message waiting to be processed.
type TriggerQueueItem struct {
	TriggerType string
	MessageID   string
	Message     string
	UserID      string
	TeamID      string
	Timestamp   time.Time
	ctx         context.Context
}

// NewTriggerQueue creates a new trigger queue.
func NewTriggerQueue() *TriggerQueue {
	return &TriggerQueue{
		items:    make([]TriggerQueueItem, 0),
		notifyCh: make(chan struct{}, 1), // Buffered to avoid blocking on send
	}
}

// Add adds a message to the queue.
func (q *TriggerQueue) Add(item TriggerQueueItem) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.items = append(q.items, item)

	// Notify waiting goroutine that a new item is available (non-blocking send)
	select {
	case q.notifyCh <- struct{}{}:
	default:
		// Channel already has a notification pending, no need to send another
	}
}

// GetNext gets the next message from the queue, or nil if empty.
func (q *TriggerQueue) GetNext() *TriggerQueueItem {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.items) == 0 {
		return nil
	}

	item := q.items[0]
	q.items = q.items[1:]
	return &item
}

// -----------------------------------------------------------------------
// Trigger system
// -----------------------------------------------------------------------

// TriggerSystem manages the triggering of tools based on their trigger definitions.
type TriggerSystem struct {
	toolEngine       TriggerToolEngine
	cronScheduler    *cron.Cron
	triggerHandlers  map[string]TriggerHandler
	queue            *TriggerQueue
	variableReplacer TriggerVariableReplacer
	inputFulfiller   TriggerInputFulfiller
	context          context.Context
	cancel           context.CancelFunc

	// Concurrency control for time-based triggers
	activeExecutions map[string][]*ActiveExecution
	executionMutex   sync.RWMutex
}

// ActiveExecution represents a running cron execution.
type ActiveExecution struct {
	ExecutionID string
	Context     context.Context
	Cancel      context.CancelFunc
	StartTime   time.Time
	FunctionKey string        // "toolName.functionName"
	DoneCh      chan struct{} // Closed when execution completes
}

// NewTriggerSystem creates a new TriggerSystem.
func NewTriggerSystem(toolEngine TriggerToolEngine, variableReplacer TriggerVariableReplacer, inputFulfiller TriggerInputFulfiller) (*TriggerSystem, error) {
	if &toolEngine == nil {
		return nil, fmt.Errorf("toolEngine cannot be nil")
	}
	if variableReplacer == nil {
		return nil, fmt.Errorf("variableReplacer cannot be nil")
	}
	if &inputFulfiller == nil {
		return nil, fmt.Errorf("inputFulfiller cannot be nil")
	}

	cronScheduler := cron.New(cron.WithSeconds())
	ctx, cancel := context.WithCancel(context.Background())

	system := &TriggerSystem{
		toolEngine:       toolEngine,
		cronScheduler:    cronScheduler,
		triggerHandlers:  make(map[string]TriggerHandler),
		queue:            NewTriggerQueue(),
		variableReplacer: variableReplacer,
		inputFulfiller:   inputFulfiller,
		context:          ctx,
		cancel:           cancel,
		activeExecutions: make(map[string][]*ActiveExecution),
	}

	// Register default handlers
	system.RegisterHandler(NewUserMessageTriggerHandler(system))
	system.RegisterHandler(NewTeamMessageTriggerHandler(system))
	system.RegisterHandler(NewCompletedUserMessageTriggerHandler(system))
	system.RegisterHandler(NewCompletedTeamMessageTriggerHandler(system))
	system.RegisterHandler(NewTimeBasedTriggerHandler(system))
	system.RegisterHandler(NewMeetingTriggerHandler(system))

	// Register email handlers
	system.RegisterHandler(NewUserEmailTriggerHandler(system))
	system.RegisterHandler(NewTeamEmailTriggerHandler(system))
	system.RegisterHandler(NewCompletedUserEmailTriggerHandler(system))
	system.RegisterHandler(NewCompletedTeamEmailTriggerHandler(system))

	return system, nil
}

// Start initializes the trigger system and starts processing.
func (t *TriggerSystem) Start(ctx context.Context) error {
	// Start cron scheduler
	t.cronScheduler.Start()

	// Initialize time-based triggers
	if err := t.InitializeTimeBasedTriggers(ctx); err != nil {
		return fmt.Errorf("failed to initialize time-based triggers: %w", err)
	}

	// Start queue processing goroutine
	go t.ProcessMessageQueue(ctx)

	return nil
}

// Stop stops the trigger system.
func (t *TriggerSystem) Stop() {
	t.cancel()
	if t.cronScheduler != nil {
		t.cronScheduler.Stop()
	}
}

func (t *TriggerSystem) RegisterHandler(handler TriggerHandler) {
	for _, triggerType := range getSupportedTriggerTypes(handler) {
		t.triggerHandlers[triggerType] = handler
	}
}

// getSupportedTriggerTypes is a helper function to get all trigger types a handler supports.
func getSupportedTriggerTypes(handler TriggerHandler) []string {
	var types []string

	if handler.CanHandle(skill.TriggerOnUserMessage) {
		types = append(types, skill.TriggerOnUserMessage)
	}
	if handler.CanHandle(skill.TriggerOnTeamMessage) {
		types = append(types, skill.TriggerOnTeamMessage)
	}
	if handler.CanHandle(skill.TriggerOnCompletedUserMessage) {
		types = append(types, skill.TriggerOnCompletedUserMessage)
	}
	if handler.CanHandle(skill.TriggerOnCompletedTeamMessage) {
		types = append(types, skill.TriggerOnCompletedTeamMessage)
	}
	if handler.CanHandle(skill.TriggerTime) {
		types = append(types, skill.TriggerTime)
	}
	if handler.CanHandle(skill.TriggerFlexForUser) {
		types = append(types, skill.TriggerFlexForUser)
	}
	if handler.CanHandle(skill.TriggerFlexForTeam) {
		types = append(types, skill.TriggerFlexForTeam)
	}
	if handler.CanHandle(skill.TriggerOnMeetingStart) {
		types = append(types, skill.TriggerOnMeetingStart)
	}
	if handler.CanHandle(skill.TriggerOnMeetingEnd) {
		types = append(types, skill.TriggerOnMeetingEnd)
	}
	// Email triggers
	if handler.CanHandle(skill.TriggerOnUserEmail) {
		types = append(types, skill.TriggerOnUserEmail)
	}
	if handler.CanHandle(skill.TriggerOnTeamEmail) {
		types = append(types, skill.TriggerOnTeamEmail)
	}
	if handler.CanHandle(skill.TriggerOnCompletedUserEmail) {
		types = append(types, skill.TriggerOnCompletedUserEmail)
	}
	if handler.CanHandle(skill.TriggerOnCompletedTeamEmail) {
		types = append(types, skill.TriggerOnCompletedTeamEmail)
	}
	if handler.CanHandle(skill.TriggerFlexForUserEmail) {
		types = append(types, skill.TriggerFlexForUserEmail)
	}
	if handler.CanHandle(skill.TriggerFlexForTeamEmail) {
		types = append(types, skill.TriggerFlexForTeamEmail)
	}

	return types
}

// -----------------------------------------------------------------------
// Concurrency control methods for time-based triggers
// -----------------------------------------------------------------------

// shouldExecute checks if function should execute based on concurrency config.
func (t *TriggerSystem) shouldExecute(functionKey string, cc *skill.ConcurrencyControl) (bool, *ActiveExecution) {
	t.executionMutex.RLock()
	defer t.executionMutex.RUnlock()

	executions := t.activeExecutions[functionKey]
	runningCount := len(executions)

	if runningCount == 0 {
		return true, nil
	}

	// Apply defaults if not set
	strategy := skill.DefaultConcurrencyStrategy
	maxParallel := skill.DefaultMaxParallel
	if cc != nil {
		if cc.Strategy != "" {
			strategy = cc.Strategy
		}
		if cc.MaxParallel > 0 {
			maxParallel = cc.MaxParallel
		}
	}

	switch strategy {
	case skill.ConcurrencyStrategySkip:
		// Skip if any execution is running
		return false, nil

	case skill.ConcurrencyStrategyKill:
		// Always kill oldest execution (we already checked runningCount > 0 above)
		return true, executions[0] // Oldest execution

	case skill.ConcurrencyStrategyParallel:
		// Kill oldest if limit reached
		if runningCount >= maxParallel {
			return true, executions[0] // Oldest execution
		}
		return true, nil

	default:
		return true, nil
	}
}

// registerExecution adds execution to tracking.
func (t *TriggerSystem) registerExecution(exec *ActiveExecution) {
	t.executionMutex.Lock()
	defer t.executionMutex.Unlock()

	t.activeExecutions[exec.FunctionKey] = append(t.activeExecutions[exec.FunctionKey], exec)
}

// unregisterExecution removes execution from tracking.
func (t *TriggerSystem) unregisterExecution(functionKey, executionID string) {
	t.executionMutex.Lock()
	defer t.executionMutex.Unlock()

	executions := t.activeExecutions[functionKey]
	for i, exec := range executions {
		if exec.ExecutionID == executionID {
			t.activeExecutions[functionKey] = append(executions[:i], executions[i+1:]...)
			break
		}
	}

	// Clean up empty entries
	if len(t.activeExecutions[functionKey]) == 0 {
		delete(t.activeExecutions, functionKey)
	}
}

// killExecution cancels execution with timeout (event-driven, no polling).
func (t *TriggerSystem) killExecution(exec *ActiveExecution, timeout time.Duration) {
	if logger != nil {
		logger.Infof("Killing execution %s (running for %v)",
			exec.ExecutionID, time.Since(exec.StartTime))
	}

	// Cancel context to signal execution to stop
	exec.Cancel()

	// Wait for execution to complete or timeout (event-driven)
	select {
	case <-exec.DoneCh:
		// Execution completed gracefully
		if logger != nil {
			logger.Infof("Execution killed gracefully")
		}
	case <-time.After(timeout):
		// Timeout reached, force cleanup
		if logger != nil {
			logger.Warnf("Execution kill timeout - forcing cleanup")
		}
		// Force unregister (execution may still be running but we move on)
		// Note: The old execution's defer will also try to unregister when it eventually
		// completes, but unregisterExecution safely handles double unregister.
		t.unregisterExecution(exec.FunctionKey, exec.ExecutionID)
	}
}

// findExecution helper (caller must hold lock).
func (t *TriggerSystem) findExecution(functionKey, executionID string) (*ActiveExecution, bool) {
	for _, exec := range t.activeExecutions[functionKey] {
		if exec.ExecutionID == executionID {
			return exec, true
		}
	}
	return nil, false
}

// ProcessMessageQueue runs as a goroutine to process queued messages.
func (t *TriggerSystem) ProcessMessageQueue(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return

		case <-t.queue.notifyCh:
			// Notification received: new message(s) available in queue
			item := t.queue.GetNext()
			if item == nil {
				// False alarm or another goroutine got it, continue waiting
				continue
			}

			// Create message context
			msgCtx := MessageContext{
				MessageID:     item.MessageID,
				UserID:        item.UserID,
				TeamID:        item.TeamID,
				Text:          item.Message,
				IsUserMessage: item.TriggerType == skill.TriggerOnUserMessage || item.TriggerType == skill.TriggerOnCompletedUserMessage,
				IsTeamMessage: item.TriggerType == skill.TriggerOnTeamMessage || item.TriggerType == skill.TriggerOnCompletedTeamMessage,
			}

			// Process the message
			err := t.ProcessMessage(item.ctx, msgCtx)
			if err != nil {
				fmt.Printf("Error processing message: %v\n", err)
			}

		case <-time.After(5 * time.Second):
			// Periodic check as safety fallback (in case notification was missed)
			item := t.queue.GetNext()
			if item == nil {
				// No messages available, continue waiting
				continue
			}

			// Create message context
			msgCtx := MessageContext{
				MessageID:     item.MessageID,
				UserID:        item.UserID,
				TeamID:        item.TeamID,
				Text:          item.Message,
				IsUserMessage: item.TriggerType == skill.TriggerOnUserMessage || item.TriggerType == skill.TriggerOnCompletedUserMessage,
				IsTeamMessage: item.TriggerType == skill.TriggerOnTeamMessage || item.TriggerType == skill.TriggerOnCompletedTeamMessage,
			}

			// Process the message
			err := t.ProcessMessage(item.ctx, msgCtx)
			if err != nil {
				fmt.Printf("Error processing message: %v\n", err)
			}
		}
	}
}

// AddMessageToQueue adds a message to the processing queue.
func (t *TriggerSystem) AddMessageToQueue(ctx context.Context, triggerType, messageID, message, userID string, teamID string) {
	t.queue.Add(TriggerQueueItem{
		TriggerType: triggerType,
		MessageID:   messageID,
		Message:     message,
		UserID:      userID,
		TeamID:      teamID,
		Timestamp:   time.Now(),
		ctx:         ctx,
	})
}

// ProcessMessage evaluates all tools with message-based triggers.
func (t *TriggerSystem) ProcessMessage(ctx context.Context, msgCtx MessageContext) error {
	allTools := t.toolEngine.GetAllTools()

	// Get channel from context to determine if this is an email
	channel, _ := ctx.Value(ChannelInContextKey).(string)
	isEmailChannel := channel == "email"

	// Check each tool's functions for matching triggers
	for _, tool := range allTools {
		for _, function := range tool.Functions {
			// Check each trigger
			for _, trigger := range function.Triggers {
				// Find the appropriate handler
				handler, exists := t.triggerHandlers[trigger.Type]
				if !exists {
					// No handler for this trigger type
					continue
				}

				// Check if this is a message trigger and if the message type matches
				// For non-email channel: use message triggers
				if !isEmailChannel {
					if (trigger.Type == skill.TriggerOnUserMessage && !msgCtx.IsUserMessage) ||
						(trigger.Type == skill.TriggerOnTeamMessage && !msgCtx.IsTeamMessage) ||
						(trigger.Type == skill.TriggerOnCompletedUserMessage && !msgCtx.IsUserMessage) ||
						(trigger.Type == skill.TriggerOnCompletedTeamMessage && !msgCtx.IsTeamMessage) {
						continue
					}
					// Skip email triggers for non-email channels
					if trigger.Type == skill.TriggerOnUserEmail ||
						trigger.Type == skill.TriggerOnTeamEmail ||
						trigger.Type == skill.TriggerOnCompletedUserEmail ||
						trigger.Type == skill.TriggerOnCompletedTeamEmail {
						continue
					}
				}

				// For email channel: use email triggers instead of message triggers
				if isEmailChannel {
					if (trigger.Type == skill.TriggerOnUserEmail && !msgCtx.IsUserMessage) ||
						(trigger.Type == skill.TriggerOnTeamEmail && !msgCtx.IsTeamMessage) ||
						(trigger.Type == skill.TriggerOnCompletedUserEmail && !msgCtx.IsUserMessage) ||
						(trigger.Type == skill.TriggerOnCompletedTeamEmail && !msgCtx.IsTeamMessage) {
						continue
					}
					// Skip message triggers for email channel
					if trigger.Type == skill.TriggerOnUserMessage ||
						trigger.Type == skill.TriggerOnTeamMessage ||
						trigger.Type == skill.TriggerOnCompletedUserMessage ||
						trigger.Type == skill.TriggerOnCompletedTeamMessage {
						continue
					}
				}

				// Handle the trigger
				err := handler.Handle(ctx, t.inputFulfiller, &trigger, &function, &tool, t.variableReplacer)
				if err != nil {
					if logger != nil {
						logger.WithError(err).Errorf("Error handling trigger")
					}
				}
			}
		}
	}

	return nil
}

// InitializeTimeBasedTriggers sets up all cron-based triggers.
func (t *TriggerSystem) InitializeTimeBasedTriggers(ctx context.Context) error {
	allTools := t.toolEngine.GetAllTools()

	// Check each tool's functions for time-based triggers
	for _, tool := range allTools {
		for _, function := range tool.Functions {
			for _, trigger := range function.Triggers {
				if trigger.Type == skill.TriggerTime && trigger.Cron != "" {
					// Find the time-based trigger handler
					handler, exists := t.triggerHandlers[skill.TriggerTime]
					if !exists {
						continue
					}

					// Capture variables for closure
					toolCopy := tool
					functionCopy := function
					triggerCopy := trigger

					// Schedule with concurrency control wrapper
					_, err := t.cronScheduler.AddFunc(trigger.Cron, func() {
						t.executeCronWithConcurrencyControl(
							ctx,
							handler,
							&triggerCopy,
							&functionCopy,
							&toolCopy,
						)
					})

					if err != nil {
						return fmt.Errorf("failed to schedule cron job for function %s: %w",
							function.Name, err)
					}

					if logger != nil {
						logger.Infof("Scheduled time-based trigger for %s.%s with cron: %s",
							tool.Name, function.Name, trigger.Cron)
					}
				}
			}
		}
	}

	return nil
}

// executeCronWithConcurrencyControl wraps cron execution with concurrency control.
func (t *TriggerSystem) executeCronWithConcurrencyControl(
	ctx context.Context,
	handler TriggerHandler,
	trigger *skill.Trigger,
	function *skill.Function,
	tool *skill.Tool,
) {
	functionKey := fmt.Sprintf("%s.%s", tool.Name, function.Name)
	executionID := fmt.Sprintf("%s-%d", functionKey, time.Now().UnixNano())

	// Check if should execute based on concurrency config
	shouldExecute, executionToKill := t.shouldExecute(functionKey, trigger.ConcurrencyControl)

	if !shouldExecute {
		if logger != nil {
			logger.Infof("Skipping cron execution for %s - concurrency limit reached (strategy: skip)",
				functionKey)
		}
		return
	}

	// Kill old execution if needed
	if executionToKill != nil {
		// Verify execution still exists before killing
		t.executionMutex.RLock()
		_, stillExists := t.findExecution(executionToKill.FunctionKey, executionToKill.ExecutionID)
		t.executionMutex.RUnlock()

		if stillExists {
			killTimeout := time.Duration(skill.DefaultKillTimeoutSeconds) * time.Second
			if trigger.ConcurrencyControl != nil && trigger.ConcurrencyControl.KillTimeout > 0 {
				killTimeout = time.Duration(trigger.ConcurrencyControl.KillTimeout) * time.Second
			}

			t.killExecution(executionToKill, killTimeout)
		} else {
			if logger != nil {
				logger.Debugf("Execution %s already completed, no need to kill", executionToKill.ExecutionID)
			}
		}
	}

	// Create cancellable context for this execution
	execCtx, cancel := context.WithCancel(ctx)

	// Register execution
	activeExec := &ActiveExecution{
		ExecutionID: executionID,
		Context:     execCtx,
		Cancel:      cancel,
		StartTime:   time.Now(),
		FunctionKey: functionKey,
		DoneCh:      make(chan struct{}),
	}
	t.registerExecution(activeExec)

	// Ensure cleanup on completion
	defer func() {
		close(activeExec.DoneCh) // Signal completion (event-driven)
		t.unregisterExecution(functionKey, executionID)
		cancel() // Clean up context
	}()

	// Execute the function
	if logger != nil {
		logger.Infof("Executing cron function %s (execution: %s)", functionKey, executionID)
	}

	err := handler.Handle(execCtx, t.inputFulfiller, trigger, function, tool, t.variableReplacer)
	if err != nil {
		if logger != nil {
			logger.Errorf("Error handling time-based trigger for function %s: %v",
				function.Name, err)
		}
	}

	if logger != nil {
		logger.Infof("Completed cron execution for %s (duration: %v)",
			functionKey, time.Since(activeExec.StartTime))
	}
}

// ProcessMeetingEvent processes functions with meeting-based triggers.
// botID is the Recall.ai bot ID, eventType is "in_call_recording" or "call_ended".
// This method implements the ITriggerSystem interface.
func (t *TriggerSystem) ProcessMeetingEvent(ctx context.Context, botID string, eventType string) error {
	// Map event types to trigger types
	var triggerType string
	var meetingEvent string
	switch eventType {
	case "in_call_recording":
		triggerType = skill.TriggerOnMeetingStart
		meetingEvent = "start"
	case "call_ended":
		triggerType = skill.TriggerOnMeetingEnd
		meetingEvent = "end"
	default:
		if logger != nil {
			logger.Debugf("Unknown meeting event type: %s, ignoring", eventType)
		}
		return nil // Unknown event, ignore
	}

	if logger != nil {
		logger.WithFields(map[string]interface{}{
			"botId":       botID,
			"eventType":   eventType,
			"triggerType": triggerType,
		}).Infof("Processing meeting event for triggers")
	}

	// Inject meeting context for $MEETING variable resolution
	meetingCtx := MeetingContext{
		BotID:     botID,
		Event:     meetingEvent,
		Timestamp: time.Now(),
	}
	ctx = context.WithValue(ctx, MeetingContextKey, meetingCtx)

	// Find the handler for meeting triggers
	handler, exists := t.triggerHandlers[triggerType]
	if !exists {
		if logger != nil {
			logger.Warnf("No handler registered for trigger type: %s", triggerType)
		}
		return nil
	}

	// Get all tools and check for matching triggers
	allTools := t.toolEngine.GetAllTools()
	triggeredCount := 0

	for _, tool := range allTools {
		for _, function := range tool.Functions {
			for _, trigger := range function.Triggers {
				if trigger.Type == triggerType {
					triggeredCount++
					if logger != nil {
						logger.Infof("Executing meeting trigger for %s.%s (%s)",
							tool.Name, function.Name, triggerType)
					}

					// Execute the function - capture variables for proper closure
					toolCopy := tool
					functionCopy := function
					triggerCopy := trigger

					// Execute synchronously for now (can be made async if needed)
					err := handler.Handle(ctx, t.inputFulfiller, &triggerCopy, &functionCopy, &toolCopy, t.variableReplacer)
					if err != nil {
						if logger != nil {
							logger.WithError(err).Errorf("Error handling meeting trigger for %s.%s",
								tool.Name, function.Name)
						}
						// Continue processing other triggers even if one fails
					}
				}
			}
		}
	}

	if logger != nil {
		logger.Infof("Processed %d functions with %s trigger for botID %s",
			triggeredCount, triggerType, botID)
	}

	return nil
}

// TimeBasedTriggerInfo contains information about a time-based trigger for API responses.
type TimeBasedTriggerInfo struct {
	ToolName            string `json:"tool_name"`
	FunctionName        string `json:"function_name"`
	Description         string `json:"description"`
	Cron                string `json:"cron"`
	ConcurrencyStrategy string `json:"concurrency_strategy"`
	MaxParallel         int    `json:"max_parallel"`
}

// ListTimeBasedTriggers returns all functions that have time-based triggers.
func (t *TriggerSystem) ListTimeBasedTriggers() []TimeBasedTriggerInfo {
	var triggers []TimeBasedTriggerInfo
	allTools := t.toolEngine.GetAllTools()

	for _, tool := range allTools {
		for _, function := range tool.Functions {
			for _, trigger := range function.Triggers {
				if trigger.Type == skill.TriggerTime && trigger.Cron != "" {
					// Determine concurrency settings
					strategy := skill.DefaultConcurrencyStrategy
					maxParallel := skill.DefaultMaxParallel
					if trigger.ConcurrencyControl != nil {
						if trigger.ConcurrencyControl.Strategy != "" {
							strategy = trigger.ConcurrencyControl.Strategy
						}
						if trigger.ConcurrencyControl.MaxParallel > 0 {
							maxParallel = trigger.ConcurrencyControl.MaxParallel
						}
					}

					triggers = append(triggers, TimeBasedTriggerInfo{
						ToolName:            tool.Name,
						FunctionName:        function.Name,
						Description:         function.Description,
						Cron:                trigger.Cron,
						ConcurrencyStrategy: strategy,
						MaxParallel:         maxParallel,
					})
				}
			}
		}
	}

	return triggers
}

// ExecuteTimeBasedTriggerManually executes a time-based trigger manually.
// It uses the same execution path as automatic cron execution, including concurrency control.
// Returns an error if the trigger is not found or if it was skipped due to concurrency control.
func (t *TriggerSystem) ExecuteTimeBasedTriggerManually(ctx context.Context, toolName, functionName string) (string, error) {
	allTools := t.toolEngine.GetAllTools()

	// Find the tool and function
	for _, tool := range allTools {
		if tool.Name != toolName {
			continue
		}
		for _, function := range tool.Functions {
			if function.Name != functionName {
				continue
			}
			// Find a time-based trigger for this function
			for _, trigger := range function.Triggers {
				if trigger.Type == skill.TriggerTime {
					// Find the time-based trigger handler
					handler, exists := t.triggerHandlers[skill.TriggerTime]
					if !exists {
						return "", fmt.Errorf("no handler registered for time-based triggers")
					}

					// Check concurrency before executing
					functionKey := fmt.Sprintf("%s.%s", tool.Name, function.Name)
					shouldExec, _ := t.shouldExecute(functionKey, trigger.ConcurrencyControl)
					if !shouldExec {
						strategy := skill.DefaultConcurrencyStrategy
						if trigger.ConcurrencyControl != nil && trigger.ConcurrencyControl.Strategy != "" {
							strategy = trigger.ConcurrencyControl.Strategy
						}
						return "", fmt.Errorf("trigger skipped due to concurrency control (strategy: %s)", strategy)
					}

					// Capture variables for execution
					toolCopy := tool
					functionCopy := function
					triggerCopy := trigger

					// Generate execution ID for tracking
					executionID := fmt.Sprintf("%s-%d", functionKey, time.Now().UnixNano())

					// Execute using the same path as automatic cron (runs synchronously)
					go t.executeCronWithConcurrencyControl(
						ctx,
						handler,
						&triggerCopy,
						&functionCopy,
						&toolCopy,
					)

					if logger != nil {
						logger.Infof("Manual trigger execution started for %s.%s", toolName, functionName)
					}
					return executionID, nil
				}
			}
			return "", fmt.Errorf("function %s.%s has no time-based trigger", toolName, functionName)
		}
	}

	return "", fmt.Errorf("tool or function not found: %s.%s", toolName, functionName)
}

// TestModeFunctionInfo represents function info for test mode.
type TestModeFunctionInfo struct {
	ToolName     string                  `json:"tool_name"`
	FunctionName string                  `json:"function_name"`
	Description  string                  `json:"description"`
	FunctionKey  string                  `json:"function_key"`
	Inputs       []TestModeFunctionInput `json:"inputs"`
	Needs        []string                `json:"needs,omitempty"`
	OnSuccess    []string                `json:"on_success,omitempty"`
	OnFailure    []string                `json:"on_failure,omitempty"`
	TriggerTypes []string                `json:"trigger_types,omitempty"`
}

// TestModeFunctionInput represents an input parameter for test mode.
type TestModeFunctionInput struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	IsOptional  bool   `json:"is_optional"`
	Origin      string `json:"origin,omitempty"`
}

// GetTestModeFunctions returns all functions with their inputs and dependencies
// for test mode execution planning.
func (t *TriggerSystem) GetTestModeFunctions(workflowType string) (startTriggers, completionTriggers, availableFunctions []TestModeFunctionInfo) {
	allTools := t.toolEngine.GetAllTools()

	// Determine trigger types based on workflow type
	var startTriggerType, completeTriggerType string
	if workflowType == "team" {
		startTriggerType = skill.TriggerOnTeamMessage
		completeTriggerType = skill.TriggerOnCompletedTeamMessage
	} else {
		startTriggerType = skill.TriggerOnUserMessage
		completeTriggerType = skill.TriggerOnCompletedUserMessage
	}

	for _, tool := range allTools {
		for _, function := range tool.Functions {
			fnInfo := TestModeFunctionInfo{
				ToolName:     tool.Name,
				FunctionName: function.Name,
				Description:  function.Description,
				FunctionKey:  fmt.Sprintf("%s.%s", tool.Name, function.Name),
				Inputs:       make([]TestModeFunctionInput, 0),
				TriggerTypes: make([]string, 0),
			}

			// Extract inputs
			for _, input := range function.Input {
				fnInfo.Inputs = append(fnInfo.Inputs, TestModeFunctionInput{
					Name:        input.Name,
					Description: input.Description,
					IsOptional:  input.IsOptional,
					Origin:      input.Origin,
				})
			}

			// Extract needs (dependencies)
			if function.Needs != nil {
				for _, need := range function.Needs {
					fnInfo.Needs = append(fnInfo.Needs, need.Name)
				}
			}

			// Extract onSuccess dependencies
			if function.OnSuccess != nil {
				for _, onSuccess := range function.OnSuccess {
					fnInfo.OnSuccess = append(fnInfo.OnSuccess, onSuccess.Name)
				}
			}

			// Extract onFailure dependencies
			if function.OnFailure != nil {
				for _, onFailure := range function.OnFailure {
					fnInfo.OnFailure = append(fnInfo.OnFailure, onFailure.Name)
				}
			}

			// Check trigger types and categorize
			isStartTrigger := false
			isCompletionTrigger := false
			isFlexFunction := false
			for _, trigger := range function.Triggers {
				fnInfo.TriggerTypes = append(fnInfo.TriggerTypes, trigger.Type)
				if trigger.Type == startTriggerType {
					isStartTrigger = true
				}
				if trigger.Type == completeTriggerType {
					isCompletionTrigger = true
				}
				// Check if function is flex (available for workflow execution)
				if trigger.Type == skill.TriggerFlexForUser || trigger.Type == skill.TriggerFlexForTeam {
					isFlexFunction = true
				}
			}

			// Categorize function
			if isStartTrigger {
				startTriggers = append(startTriggers, fnInfo)
			}
			if isCompletionTrigger {
				completionTriggers = append(completionTriggers, fnInfo)
			}
			// Add to available functions if it's flex (available for workflow execution)
			if isFlexFunction {
				availableFunctions = append(availableFunctions, fnInfo)
			}
		}
	}

	return startTriggers, completionTriggers, availableFunctions
}
