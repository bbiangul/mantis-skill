package models

import (
	"context"
	"fmt"
	"time"

	"github.com/bbiangul/mantis-skill/types"
	"github.com/bbiangul/mantis-skill/engine/utils"
)

// WorkflowManager handles the execution of agentic workflows
type WorkflowManager struct {
	Repository     types.ToolRepository
	ToolEngine     IToolEngine
	InputFulfiller IInputFulfiller
	Coordinator    IAgenticCoordinator
}

// ExecuteAgenticWorkflow processes a user request through the agentic framework
func (m *WorkflowManager) ExecuteAgenticWorkflow(ctx context.Context, userMessage, contextForAgent string, toolsAndFunctions map[string][]string) (string, error) {
	startTime := time.Now()

	if logger != nil {
		logger.Infof("Starting agentic workflow for message: %s", userMessage)
	}

	result, err := m.Coordinator.ExecuteWorkflow(ctx, userMessage, contextForAgent, toolsAndFunctions)
	if err != nil {
		if logger != nil {
			logger.Errorf("Workflow execution failed: %v", err)
		}
		return "", err
	}

	executionTime := time.Since(startTime)
	if logger != nil {
		logger.Infof("Workflow execution completed in %v with result: %s",
			executionTime, utils.TruncateForLogging(result))
	}

	return result, nil
}

// SetWorkflowOptions configures options for workflow execution
func (m *WorkflowManager) SetWorkflowOptions(maxTurns int) {
	m.Coordinator.SetMaxTurns(maxTurns)
}

// GetCoordinator returns the underlying coordinator
func (m *WorkflowManager) GetCoordinator() IAgenticCoordinator {
	return m.Coordinator
}
func (m *WorkflowManager) SetCallback(callback AgenticWorkflowCallback) {
	m.Coordinator.SetCallback(callback)
}

// GetActiveWorkflows returns all active workflow message IDs
func (m *WorkflowManager) GetActiveWorkflows() []string {
	return (*m.Coordinator.StateStore()).GetActiveWorkflows()
}

func (m *WorkflowManager) MarkAsApproved(ctx context.Context, sessionID string) error {
	if logger != nil {
		logger.Infof("Marking workflow as approved for session: %s", sessionID)
	}

	// Update the coordinator state to mark as approved
	err := m.Coordinator.ApproveCheckPoint(ctx, sessionID)
	if err != nil {
		if logger != nil {
			logger.Errorf("Failed to mark workflow as approved for session %s: %v", sessionID, err)
		}
		return err
	}

	if logger != nil {
		logger.Infof("Workflow marked as approved for session: %s", sessionID)
	}
	return nil
}

func (m *WorkflowManager) MarkAsResumed(ctx context.Context, sessionID string) error {
	err := m.Coordinator.MarkCheckPointAsResumed(ctx, sessionID)
	if err != nil {
		if logger != nil {
			logger.Errorf("Failed to mark workflow as resumed for session %s: %v", sessionID, err)
		}
		return err
	}

	return nil
}

func (m *WorkflowManager) MarkAsComplete(ctx context.Context, sessionID string) error {
	err := m.Coordinator.MarkCheckPointAsComplete(ctx, sessionID)
	if err != nil {
		if logger != nil {
			logger.Errorf("Failed to mark workflow as complete for session %s: %v", sessionID, err)
		}
		return err
	}

	return nil
}

// ResumeExecuteAgenticWorkflow resumes a paused workflow execution from a checkpoint
func (m *WorkflowManager) ResumeExecuteAgenticWorkflow(ctx context.Context, sessionID string) (string, string, error) {
	startTime := time.Now()

	if logger != nil {
		logger.Infof("Resuming agentic workflow for session: %s", sessionID)
	}

	result, clientID, err := m.Coordinator.ContinueExecution(ctx, sessionID)
	if err != nil {
		if logger != nil {
			logger.Errorf("Workflow resume failed for session %s: %v", sessionID, err)
		}
		return "", "", err
	}

	err = m.MarkAsComplete(ctx, sessionID)
	if err != nil {
		if logger != nil {
			logger.WithError(err).Errorf("Failed to mark step as complete")
		}
		return "", "", fmt.Errorf("failed to mark step as complete: %w", err)
	}

	executionTime := time.Since(startTime)
	if logger != nil {
		logger.Infof("Workflow resume completed in %v for session %s with result: %s",
			executionTime, sessionID, utils.TruncateForLogging(result))
	}

	return result, clientID, nil
}
