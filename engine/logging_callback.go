package engine

import (
	"context"

	"github.com/bbiangul/mantis-skill/engine/models"
	"github.com/bbiangul/mantis-skill/types"
)

// LoggingWorkflowCallback wraps the null callback and logs key workflow events
// using the package-level logger. If the logger is nil, all calls are no-ops
// (inherited from NullAgenticWorkflowCallback).
type LoggingWorkflowCallback struct {
	models.NullAgenticWorkflowCallback // Inherit empty implementations
}

// NewLoggingWorkflowCallback creates a LoggingWorkflowCallback that logs
// workflow events through the package-level logger (set via SetLogger).
func NewLoggingWorkflowCallback() *LoggingWorkflowCallback {
	return &LoggingWorkflowCallback{}
}

func (c *LoggingWorkflowCallback) OnStepProposed(ctx context.Context, stepKey string, messageID string, clientID string, toolName string, rationale string) {
	if logger != nil {
		logger.Infof("[%s] Proposed step: %s - Reason: %s", messageID, toolName, rationale)
	}
}

func (c *LoggingWorkflowCallback) OnStepExecuted(ctx context.Context, stepKey string, messageID string, clientID string, toolname string, output string, err error) {
	if logger == nil {
		return
	}
	if err != nil {
		logger.Warnf("[%s] Step %s execution failed: %v", messageID, toolname, err)
	} else {
		logger.Infof("[%s] Step %s executed successfully", messageID, toolname)
	}
}

func (c *LoggingWorkflowCallback) OnWorkflowCompleted(ctx context.Context, messageID string, clientID string, summary types.WorkflowSummary, stateStore models.StateStore, isNewMessage bool) {
	if logger == nil {
		return
	}
	logger.Infof("[%s] Workflow completed: %s - Ran %d steps in %v",
		messageID, summary.CompletionType, len(summary.ExecutedSteps), summary.ExecutionTime)

	for i, step := range summary.ExecutedSteps {
		logger.Infof("[%s] Step %d: %s - %s", messageID, i+1, step.ToolName, step.FunctionName)
	}

	logger.Infof("[%s] Final result: %s", messageID, summary.FinalResult)
}
