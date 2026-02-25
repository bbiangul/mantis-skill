package engine

import (
	"context"
	"fmt"
	"runtime/debug"

	"github.com/bbiangul/mantis-skill/skill"
	"github.com/google/uuid"
)

// -----------------------------------------------------------------------
// MessageContext & TriggerHandler interface
// -----------------------------------------------------------------------

// MessageContext contains information about a message for trigger evaluation.
type MessageContext struct {
	MessageID     string
	UserID        string
	TeamID        string
	Text          string
	IsUserMessage bool
	IsTeamMessage bool
}

// TriggerHandler defines an interface for handling different types of triggers.
type TriggerHandler interface {
	CanHandle(triggerType string) bool
	Handle(ctx context.Context, inputFulfiller TriggerInputFulfiller, trigger *skill.Trigger, function *skill.Function, tool *skill.Tool, replacer TriggerVariableReplacer) error
}

// -----------------------------------------------------------------------
// helper: executeTriggerHandler is the shared logic for all message-based handlers.
// -----------------------------------------------------------------------

func executeTriggerHandler(
	ctx context.Context,
	inputFulfiller TriggerInputFulfiller,
	trigger *skill.Trigger,
	function *skill.Function,
	tool *skill.Tool,
	replacer TriggerVariableReplacer,
	ts *TriggerSystem,
	logPrefix string,
) error {
	// TODO: Requires YAMLDefinedTool — will be enabled once yaml_defined_tool.go is ported.
	if logger != nil {
		logger.Warnf("%s trigger handler for %s.%s called but YAMLDefinedTool not yet available", logPrefix, tool.Name, function.Name)
	}
	return fmt.Errorf("%s trigger handler: YAMLDefinedTool not yet implemented", logPrefix)
}

// -----------------------------------------------------------------------
// helper: executeSyntheticTriggerHandler is the shared logic for time-based / meeting handlers.
// These handlers inject a synthetic message, callback, and step key into the context.
// -----------------------------------------------------------------------

func executeSyntheticTriggerHandler(
	ctx context.Context,
	inputFulfiller TriggerInputFulfiller,
	trigger *skill.Trigger,
	function *skill.Function,
	tool *skill.Tool,
	replacer TriggerVariableReplacer,
	ts *TriggerSystem,
	logPrefix string,
	syntheticClientID string,
	syntheticBody string,
) error {
	// Check if context is already cancelled (concurrency control)
	select {
	case <-ctx.Done():
		if logger != nil {
			logger.Infof("%s execution cancelled before start", logPrefix)
		}
		return ctx.Err()
	default:
	}

	// Inject synthetic message context for triggers that don't have a real message.
	syntheticMsg := Message{
		Id:          uuid.New().String(),
		ClientID:    syntheticClientID,
		From:        "system",
		Body:        syntheticBody,
		IsSynthetic: true,
	}
	ctx = context.WithValue(ctx, MessageInContextKey, syntheticMsg)

	// Inject callback for synthetic triggers.
	callback, err := GetExecutorAgenticWorkflowCallback()
	if err != nil {
		if logger != nil {
			logger.Errorf("Failed to get callback for %s trigger: %v", logPrefix, err)
		}
		return fmt.Errorf("callback not available for %s trigger: %w", logPrefix, err)
	}
	ctx = context.WithValue(ctx, CallbackInContextKey, callback)

	// Generate and set step key for execution tracking.
	stepKey := uuid.New().String()
	ctx = context.WithValue(ctx, StepKeyInContextKey, stepKey)

	// TODO: Requires YAMLDefinedTool — will be enabled once yaml_defined_tool.go is ported.
	_ = stepKey // suppress unused warning
	if logger != nil {
		logger.Warnf("%s synthetic trigger handler for %s.%s called but YAMLDefinedTool not yet available", logPrefix, tool.Name, function.Name)
	}
	return fmt.Errorf("%s synthetic trigger handler: YAMLDefinedTool not yet implemented", logPrefix)
}

// -----------------------------------------------------------------------
// UserMessageTriggerHandler — always_on_user_message
// -----------------------------------------------------------------------

type UserMessageTriggerHandler struct {
	triggerSystem *TriggerSystem
}

func NewUserMessageTriggerHandler(triggerSystem *TriggerSystem) *UserMessageTriggerHandler {
	return &UserMessageTriggerHandler{triggerSystem: triggerSystem}
}

func (h *UserMessageTriggerHandler) CanHandle(triggerType string) bool {
	return triggerType == skill.TriggerOnUserMessage
}

func (h *UserMessageTriggerHandler) Handle(
	ctx context.Context,
	inputFulfiller TriggerInputFulfiller,
	trigger *skill.Trigger,
	function *skill.Function,
	tool *skill.Tool,
	replacer TriggerVariableReplacer,
) error {
	return executeTriggerHandler(ctx, inputFulfiller, trigger, function, tool, replacer, h.triggerSystem, "User message")
}

// -----------------------------------------------------------------------
// TeamMessageTriggerHandler — always_on_team_message
// -----------------------------------------------------------------------

type TeamMessageTriggerHandler struct {
	triggerSystem *TriggerSystem
}

func NewTeamMessageTriggerHandler(triggerSystem *TriggerSystem) *TeamMessageTriggerHandler {
	return &TeamMessageTriggerHandler{triggerSystem: triggerSystem}
}

func (h *TeamMessageTriggerHandler) CanHandle(triggerType string) bool {
	return triggerType == skill.TriggerOnTeamMessage
}

func (h *TeamMessageTriggerHandler) Handle(
	ctx context.Context,
	inputFulfiller TriggerInputFulfiller,
	trigger *skill.Trigger,
	function *skill.Function,
	tool *skill.Tool,
	replacer TriggerVariableReplacer,
) error {
	return executeTriggerHandler(ctx, inputFulfiller, trigger, function, tool, replacer, h.triggerSystem, "Team message")
}

// -----------------------------------------------------------------------
// CompletedUserMessageTriggerHandler — always_on_completed_user_message
// -----------------------------------------------------------------------

type CompletedUserMessageTriggerHandler struct {
	triggerSystem *TriggerSystem
}

func NewCompletedUserMessageTriggerHandler(triggerSystem *TriggerSystem) *CompletedUserMessageTriggerHandler {
	return &CompletedUserMessageTriggerHandler{triggerSystem: triggerSystem}
}

func (h *CompletedUserMessageTriggerHandler) CanHandle(triggerType string) bool {
	return triggerType == skill.TriggerOnCompletedUserMessage
}

func (h *CompletedUserMessageTriggerHandler) Handle(
	ctx context.Context,
	inputFulfiller TriggerInputFulfiller,
	trigger *skill.Trigger,
	function *skill.Function,
	tool *skill.Tool,
	replacer TriggerVariableReplacer,
) error {
	return executeTriggerHandler(ctx, inputFulfiller, trigger, function, tool, replacer, h.triggerSystem, "Completed user message")
}

// -----------------------------------------------------------------------
// CompletedTeamMessageTriggerHandler — always_on_completed_team_message
// -----------------------------------------------------------------------

type CompletedTeamMessageTriggerHandler struct {
	triggerSystem *TriggerSystem
}

func NewCompletedTeamMessageTriggerHandler(triggerSystem *TriggerSystem) *CompletedTeamMessageTriggerHandler {
	return &CompletedTeamMessageTriggerHandler{triggerSystem: triggerSystem}
}

func (h *CompletedTeamMessageTriggerHandler) CanHandle(triggerType string) bool {
	return triggerType == skill.TriggerOnCompletedTeamMessage
}

func (h *CompletedTeamMessageTriggerHandler) Handle(
	ctx context.Context,
	inputFulfiller TriggerInputFulfiller,
	trigger *skill.Trigger,
	function *skill.Function,
	tool *skill.Tool,
	replacer TriggerVariableReplacer,
) error {
	return executeTriggerHandler(ctx, inputFulfiller, trigger, function, tool, replacer, h.triggerSystem, "Completed team message")
}

// -----------------------------------------------------------------------
// TimeBasedTriggerHandler — time_based
// -----------------------------------------------------------------------

type TimeBasedTriggerHandler struct {
	triggerSystem *TriggerSystem
}

func NewTimeBasedTriggerHandler(triggerSystem *TriggerSystem) *TimeBasedTriggerHandler {
	return &TimeBasedTriggerHandler{triggerSystem: triggerSystem}
}

func (h *TimeBasedTriggerHandler) CanHandle(triggerType string) bool {
	return triggerType == skill.TriggerTime
}

func (h *TimeBasedTriggerHandler) Handle(
	ctx context.Context,
	inputFulfiller TriggerInputFulfiller,
	trigger *skill.Trigger,
	function *skill.Function,
	tool *skill.Tool,
	replacer TriggerVariableReplacer,
) error {
	return executeSyntheticTriggerHandler(
		ctx, inputFulfiller, trigger, function, tool, replacer,
		h.triggerSystem, "Time-based",
		fmt.Sprintf("cron:%s.%s", tool.Name, function.Name),
		fmt.Sprintf("Cron trigger: %s.%s", tool.Name, function.Name),
	)
}

// -----------------------------------------------------------------------
// MeetingTriggerHandler — on_meeting_start / on_meeting_end
// -----------------------------------------------------------------------

type MeetingTriggerHandler struct {
	triggerSystem *TriggerSystem
}

func NewMeetingTriggerHandler(triggerSystem *TriggerSystem) *MeetingTriggerHandler {
	return &MeetingTriggerHandler{triggerSystem: triggerSystem}
}

func (h *MeetingTriggerHandler) CanHandle(triggerType string) bool {
	return triggerType == skill.TriggerOnMeetingStart ||
		triggerType == skill.TriggerOnMeetingEnd
}

func (h *MeetingTriggerHandler) Handle(
	ctx context.Context,
	inputFulfiller TriggerInputFulfiller,
	trigger *skill.Trigger,
	function *skill.Function,
	tool *skill.Tool,
	replacer TriggerVariableReplacer,
) error {
	return executeSyntheticTriggerHandler(
		ctx, inputFulfiller, trigger, function, tool, replacer,
		h.triggerSystem, "Meeting",
		fmt.Sprintf("meeting:%s.%s", tool.Name, function.Name),
		fmt.Sprintf("Meeting trigger: %s.%s (%s)", tool.Name, function.Name, trigger.Type),
	)
}

// -----------------------------------------------------------------------
// UserEmailTriggerHandler — always_on_user_email
// -----------------------------------------------------------------------

type UserEmailTriggerHandler struct {
	triggerSystem *TriggerSystem
}

func NewUserEmailTriggerHandler(triggerSystem *TriggerSystem) *UserEmailTriggerHandler {
	return &UserEmailTriggerHandler{triggerSystem: triggerSystem}
}

func (h *UserEmailTriggerHandler) CanHandle(triggerType string) bool {
	return triggerType == skill.TriggerOnUserEmail
}

func (h *UserEmailTriggerHandler) Handle(
	ctx context.Context,
	inputFulfiller TriggerInputFulfiller,
	trigger *skill.Trigger,
	function *skill.Function,
	tool *skill.Tool,
	replacer TriggerVariableReplacer,
) error {
	return executeTriggerHandler(ctx, inputFulfiller, trigger, function, tool, replacer, h.triggerSystem, "User email")
}

// -----------------------------------------------------------------------
// TeamEmailTriggerHandler — always_on_team_email
// -----------------------------------------------------------------------

type TeamEmailTriggerHandler struct {
	triggerSystem *TriggerSystem
}

func NewTeamEmailTriggerHandler(triggerSystem *TriggerSystem) *TeamEmailTriggerHandler {
	return &TeamEmailTriggerHandler{triggerSystem: triggerSystem}
}

func (h *TeamEmailTriggerHandler) CanHandle(triggerType string) bool {
	return triggerType == skill.TriggerOnTeamEmail
}

func (h *TeamEmailTriggerHandler) Handle(
	ctx context.Context,
	inputFulfiller TriggerInputFulfiller,
	trigger *skill.Trigger,
	function *skill.Function,
	tool *skill.Tool,
	replacer TriggerVariableReplacer,
) error {
	return executeTriggerHandler(ctx, inputFulfiller, trigger, function, tool, replacer, h.triggerSystem, "Team email")
}

// -----------------------------------------------------------------------
// CompletedUserEmailTriggerHandler — always_on_completed_user_email
// -----------------------------------------------------------------------

type CompletedUserEmailTriggerHandler struct {
	triggerSystem *TriggerSystem
}

func NewCompletedUserEmailTriggerHandler(triggerSystem *TriggerSystem) *CompletedUserEmailTriggerHandler {
	return &CompletedUserEmailTriggerHandler{triggerSystem: triggerSystem}
}

func (h *CompletedUserEmailTriggerHandler) CanHandle(triggerType string) bool {
	return triggerType == skill.TriggerOnCompletedUserEmail
}

func (h *CompletedUserEmailTriggerHandler) Handle(
	ctx context.Context,
	inputFulfiller TriggerInputFulfiller,
	trigger *skill.Trigger,
	function *skill.Function,
	tool *skill.Tool,
	replacer TriggerVariableReplacer,
) error {
	return executeTriggerHandler(ctx, inputFulfiller, trigger, function, tool, replacer, h.triggerSystem, "Completed user email")
}

// -----------------------------------------------------------------------
// CompletedTeamEmailTriggerHandler — always_on_completed_team_email
// -----------------------------------------------------------------------

type CompletedTeamEmailTriggerHandler struct {
	triggerSystem *TriggerSystem
}

func NewCompletedTeamEmailTriggerHandler(triggerSystem *TriggerSystem) *CompletedTeamEmailTriggerHandler {
	return &CompletedTeamEmailTriggerHandler{triggerSystem: triggerSystem}
}

func (h *CompletedTeamEmailTriggerHandler) CanHandle(triggerType string) bool {
	return triggerType == skill.TriggerOnCompletedTeamEmail
}

func (h *CompletedTeamEmailTriggerHandler) Handle(
	ctx context.Context,
	inputFulfiller TriggerInputFulfiller,
	trigger *skill.Trigger,
	function *skill.Function,
	tool *skill.Tool,
	replacer TriggerVariableReplacer,
) error {
	return executeTriggerHandler(ctx, inputFulfiller, trigger, function, tool, replacer, h.triggerSystem, "Completed team email")
}

// -----------------------------------------------------------------------
// Callback — fire-and-forget notification of trigger results.
// -----------------------------------------------------------------------

// Callback dispatches execution results to the AgenticWorkflowCallback in a goroutine.
func Callback(ctx context.Context, toolName, result string, err error) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				stackTrace := string(debug.Stack())
				panicErr := fmt.Errorf("panic occurred: %v\nStack Trace:\n%s", r, stackTrace)
				if logger != nil {
					logger.WithError(panicErr).Errorf("Panic occurred at callback")
				}
			}
		}()

		callback, ok := ctx.Value(CallbackInContextKey).(TriggerAgenticCallback)
		if !ok {
			return
		}

		stepKey, ok := ctx.Value(StepKeyInContextKey).(string)
		if !ok {
			if logger != nil {
				logger.Errorf("Error getting step key from ctx")
			}
		}

		retrievedMsg, ok := ctx.Value(MessageInContextKey).(Message)
		if !ok {
			return
		}

		clientID := retrievedMsg.ClientID

		callback.OnStepExecuted(ctx, stepKey, retrievedMsg.Id, clientID, toolName, result, err)
	}()
}
