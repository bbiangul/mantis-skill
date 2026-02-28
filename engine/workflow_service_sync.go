package engine

import (
	"context"

	"github.com/bbiangul/mantis-skill/engine/models"
	"github.com/bbiangul/mantis-skill/types"
)

// IWorkflowMapper defines the interface for synchronizing workflows to a bridge/external schema.
// This replaces connect-ai/internal/services.IWorkflowMapper.
type IWorkflowMapper interface {
	SyncWorkflowFromMainSchema(ctx context.Context, workflow *types.Workflow) error
	SyncWorkflowStepFromMainSchema(ctx context.Context, step *types.WorkflowStep) error
	RemoveWorkflowStepFromBridgeSchema(ctx context.Context, stepID int64) error
}

// SyncEnabledWorkflowService wraps the workflow service with bridge schema synchronization
type SyncEnabledWorkflowService struct {
	*WorkflowService
	workflowMapper IWorkflowMapper
}

// NewSyncEnabledWorkflowService creates a new workflow service with automatic bridge schema sync
func NewSyncEnabledWorkflowService(
	workflowRepo types.WorkflowRepository,
	toolEngine models.IToolEngine,
	inputFulfiller models.IInputFulfiller,
	llm LLMProvider,
	workflowMapper IWorkflowMapper,
) *SyncEnabledWorkflowService {
	return &SyncEnabledWorkflowService{
		WorkflowService: NewWorkflowService(workflowRepo, toolEngine, inputFulfiller, llm),
		workflowMapper:  workflowMapper,
	}
}

// Ensure SyncEnabledWorkflowService implements the IWorkflowService interface
var _ models.IWorkflowService = (*SyncEnabledWorkflowService)(nil)

// SaveWorkflow saves a new workflow to the database and syncs to bridge schema
func (s *SyncEnabledWorkflowService) SaveWorkflow(ctx context.Context, workflow *types.Workflow) error {
	// Save to main schema first
	if err := s.WorkflowService.SaveWorkflow(ctx, workflow); err != nil {
		return err
	}

	// Sync to bridge schema and agent-proxy
	if err := s.syncWorkflowToBridge(ctx, workflow); err != nil {
		// Log but don't fail the operation - main schema save succeeded
		if logger != nil {
			logger.WithError(err).Warnf("Failed to sync workflow %d to bridge schema after save", workflow.ID)
		}
	}

	return nil
}

// UpdateWorkflow updates an existing workflow and syncs to bridge schema
func (s *SyncEnabledWorkflowService) UpdateWorkflow(ctx context.Context, workflow *types.Workflow) error {
	// Update main schema first
	if err := s.WorkflowService.UpdateWorkflow(ctx, workflow); err != nil {
		return err
	}

	// Sync to bridge schema and agent-proxy
	if err := s.syncWorkflowToBridge(ctx, workflow); err != nil {
		// Log but don't fail the operation - main schema update succeeded
		if logger != nil {
			logger.WithError(err).Warnf("Failed to sync workflow %d to bridge schema after update", workflow.ID)
		}
	}

	return nil
}

// UpdateWorkflowStep updates an existing workflow step and syncs to bridge schema
func (s *SyncEnabledWorkflowService) UpdateWorkflowStep(ctx context.Context, step *types.WorkflowStep) error {
	// Update main schema first
	if err := s.WorkflowService.UpdateWorkflowStep(ctx, step); err != nil {
		return err
	}

	// Sync step to bridge schema and agent-proxy
	if err := s.syncStepToBridge(ctx, step); err != nil {
		// Log but don't fail the operation - main schema update succeeded
		if logger != nil {
			logger.WithError(err).Warnf("Failed to sync workflow step %d to bridge schema after update", step.ID)
		}
	}

	return nil
}

// AddStepToWorkflow adds a new step to a workflow and syncs to bridge schema
func (s *SyncEnabledWorkflowService) AddStepToWorkflow(ctx context.Context, workflowID int64, step *types.WorkflowStep) error {
	// Add to main schema first
	if err := s.WorkflowService.AddStepToWorkflow(ctx, workflowID, step); err != nil {
		return err
	}

	// Sync to bridge schema
	if err := s.syncStepToBridge(ctx, step); err != nil {
		// Log but don't fail the operation - main schema add succeeded
		if logger != nil {
			logger.WithError(err).Warnf("Failed to sync workflow step %d to bridge schema after add", step.ID)
		}
	}

	return nil
}

// DeleteWorkflowStep deletes a workflow step and removes from bridge schema
func (s *SyncEnabledWorkflowService) DeleteWorkflowStep(ctx context.Context, stepID int64) error {
	// Delete from main schema first
	if err := s.WorkflowService.DeleteWorkflowStep(ctx, stepID); err != nil {
		return err
	}

	// Remove from bridge schema
	if err := s.removeStepFromBridge(ctx, stepID); err != nil {
		// Log but don't fail the operation - main schema delete succeeded
		if logger != nil {
			logger.WithError(err).Warnf("Failed to remove workflow step %d from bridge schema after delete", stepID)
		}
	}

	return nil
}

// UpdateWorkflowFromFeedback updates a workflow based on human feedback and syncs to bridge schema
func (s *SyncEnabledWorkflowService) UpdateWorkflowFromFeedback(ctx context.Context, categoryName, feedback string) (*types.Workflow, error) {
	// Update workflow in main schema
	updatedWorkflow, err := s.WorkflowService.UpdateWorkflowFromFeedback(ctx, categoryName, feedback)
	if err != nil {
		return nil, err
	}

	// Sync updated workflow to bridge schema
	if err := s.syncWorkflowToBridge(ctx, updatedWorkflow); err != nil {
		// Log but don't fail the operation - main schema update succeeded
		if logger != nil {
			logger.WithError(err).Warnf("Failed to sync workflow to bridge schema after feedback update for category %s", categoryName)
		}
	}

	return updatedWorkflow, nil
}

// Helper methods for synchronization

func (s *SyncEnabledWorkflowService) syncWorkflowToBridge(ctx context.Context, workflow *types.Workflow) error {
	if s.workflowMapper == nil {
		if logger != nil {
			logger.Warnf("Workflow mapper not available for sync")
		}
		return nil
	}

	return s.workflowMapper.SyncWorkflowFromMainSchema(ctx, workflow)
}

func (s *SyncEnabledWorkflowService) syncStepToBridge(ctx context.Context, step *types.WorkflowStep) error {
	if s.workflowMapper == nil {
		if logger != nil {
			logger.Warnf("Workflow mapper not available for step sync")
		}
		return nil
	}

	return s.workflowMapper.SyncWorkflowStepFromMainSchema(ctx, step)
}

func (s *SyncEnabledWorkflowService) removeStepFromBridge(ctx context.Context, stepID int64) error {
	if s.workflowMapper == nil {
		if logger != nil {
			logger.Warnf("Workflow mapper not available for step removal")
		}
		return nil
	}

	return s.workflowMapper.RemoveWorkflowStepFromBridgeSchema(ctx, stepID)
}
