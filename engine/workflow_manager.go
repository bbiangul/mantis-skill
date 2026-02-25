package engine

import (
	"errors"

	"github.com/bbiangul/mantis-skill/engine/models"
)

// NewWorkflowManager creates a new WorkflowManager with the given dependencies.
//
// Unlike the original which internally constructed an AgenticCoordinator,
// this version accepts a pre-built coordinator to avoid pulling in the
// massive coordinator construction dependencies at this layer.
func NewWorkflowManager(
	repository ToolRepository,
	toolEngine models.IToolEngine,
	inputFulfiller models.IInputFulfiller,
	coordinator models.IAgenticCoordinator,
) (*models.WorkflowManager, error) {
	if repository == nil {
		return nil, errors.New("repository cannot be nil")
	}
	if toolEngine == nil {
		return nil, errors.New("toolEngine cannot be nil")
	}
	if inputFulfiller == nil {
		return nil, errors.New("inputFulfiller cannot be nil")
	}
	if coordinator == nil {
		return nil, errors.New("coordinator cannot be nil")
	}

	return &models.WorkflowManager{
		Repository:     repository,
		ToolEngine:     toolEngine,
		InputFulfiller: inputFulfiller,
		Coordinator:    coordinator,
	}, nil
}
