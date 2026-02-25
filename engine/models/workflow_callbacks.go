package models

import (
	"context"

	"github.com/bbiangul/mantis-skill/types"
	"github.com/bbiangul/mantis-skill/skill"
)

// NullAgenticWorkflowCallback provides an empty implementation of AgenticWorkflowCallback
type NullAgenticWorkflowCallback struct{}

func (c *NullAgenticWorkflowCallback) OnStepPausedDueRequiringUserConfirmation(ctx context.Context, stepKey, messageID, clientID, toolName, functionName string, checkpoint *types.ExecutionCheckpoint) {
}

func (c *NullAgenticWorkflowCallback) SetTaskService(taskService interface{}) {

}

func (c *NullAgenticWorkflowCallback) OnFunctionExecuted(ctx context.Context, stepId string, messageID string, clientID string, functionName, toolName, functionDescription string, inputs, result string, err error, function *skill.Function) {
}

func (c *NullAgenticWorkflowCallback) OnWorkflowPlanned(ctx context.Context, messageID string, clientID string, state *CoordinatorState, isNew bool, isNewMessage bool) (interface{}, error) {
	return nil, nil
}

func (c *NullAgenticWorkflowCallback) OnStepProposed(ctx context.Context, stepKey string, messageID string, clientID string, toolName string, rationale string) {
}

func (c *NullAgenticWorkflowCallback) OnStepPausedDueHumanSupportRequired(ctx context.Context, eventKey, messageID, clientID, step string, humanSupportType types.HumanSupportType, humanQaId string, humanSupportMessage types.HumanSupportMessage) {
}

func (c *NullAgenticWorkflowCallback) OnStepExecuted(ctx context.Context, messageID string, clientID string, step string, toolname string, output string, error error) {
}

func (c *NullAgenticWorkflowCallback) OnWorkflowCompleted(ctx context.Context, messageID string, clientID string, summary types.WorkflowSummary, stateStore StateStore, isNewMessage bool) {
}

func (c *NullAgenticWorkflowCallback) OnDependencyExecuted(ctx context.Context, messageID string, clientID string, functionName, dependencyFunctionName string, result string, err error) {
}

func (c *NullAgenticWorkflowCallback) OnInputFulfillDependencyExecuted(ctx context.Context, messageID string, clientID string, inputFieldName, inputFieldDescription, functionExecuted string, result string, err error) {
}

func (c *NullAgenticWorkflowCallback) OnStepPausedDueApprovalRequired(ctx context.Context, stepKey, messageID, clientID, toolName, functionName string, checkpoint *types.ExecutionCheckpoint) {
}

func (c *NullAgenticWorkflowCallback) OnStepPausedDueMissingInput(ctx context.Context, stepKey, messageID, clientID, toolName, functionName string, checkpoint *types.ExecutionCheckpoint) {
}

func (c *NullAgenticWorkflowCallback) OnWorkflowPaused(ctx context.Context, messageID, clientID string, currentStep string, reason string, checkpoint *types.ExecutionCheckpoint) {
}

func (c *NullAgenticWorkflowCallback) OnWorkflowResumed(ctx context.Context, messageID, clientID string, checkpoint *types.ExecutionCheckpoint) {
}

func (c *NullAgenticWorkflowCallback) OnStatePersisted(ctx context.Context, messageID, clientID string, checkpointID string) {
}

func (c *NullAgenticWorkflowCallback) OnStateRestored(ctx context.Context, messageID, clientID string, checkpointID string) {
}

func (c *NullAgenticWorkflowCallback) GetEvents(id string, group CacheGroup) []WorkflowEvent {
	return []WorkflowEvent{}
}

func (c *NullAgenticWorkflowCallback) GetEventsString(ctx context.Context, id string, group CacheGroup, status WorkflowEventType) string {
	return ""
}

func (c *NullAgenticWorkflowCallback) GetEventsStringWithContext(ctx context.Context, id string, group CacheGroup, status WorkflowEventType) string {
	return ""
}

func (c *NullAgenticWorkflowCallback) RestoreEvents(ctx context.Context, events []interface{}) error {
	return nil
}

func (c *NullAgenticWorkflowCallback) OnInputsFulfilled(ctx context.Context, messageID, toolName, functionName string, inputs map[string]interface{}) {
}

func (c *NullAgenticWorkflowCallback) GetFulfilledInputs(messageID, toolName, functionName string) (map[string]interface{}, bool) {
	return nil, false
}

func (c *NullAgenticWorkflowCallback) GetAllFulfilledInputsForMessage(messageID string) map[string]map[string]interface{} {
	return make(map[string]map[string]interface{})
}

func (c *NullAgenticWorkflowCallback) OnScratchpadUpdated(ctx context.Context, messageID, clientID string, event WorkflowEvent) {
}
