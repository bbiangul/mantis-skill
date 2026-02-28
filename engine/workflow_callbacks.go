package engine

import "github.com/bbiangul/mantis-skill/engine/models"

// NewNullAgenticWorkflowCallback returns a no-op implementation of AgenticWorkflowCallback.
// Useful as a default when no real callback is needed.
func NewNullAgenticWorkflowCallback() models.AgenticWorkflowCallback {
	return &models.NullAgenticWorkflowCallback{}
}
