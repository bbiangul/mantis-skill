package engine

// -----------------------------------------------------------------------
// Stubs for functions that will be provided by yaml_defined_tool.go and
// callback.go once they are ported. These allow triggers.go and
// triggers_handler.go to compile in the meantime.
// -----------------------------------------------------------------------

import (
	"context"
	"fmt"

	"github.com/bbiangul/mantis-skill/engine/models"
	"github.com/bbiangul/mantis-skill/skill"
)

// YAMLDefinedTool is a stub for the YAML-defined tool executor.
// It will be replaced by the full implementation from yaml_defined_tool.go.
type YAMLDefinedTool struct {
	function *skill.Function
}

// NewYAMLDefinedTool creates a new YAMLDefinedTool stub.
func NewYAMLDefinedTool(
	inputFulfiller models.IInputFulfiller,
	tool *skill.Tool,
	function *skill.Function,
	executionTracker models.IFunctionExecutionTracker,
	variableReplacer models.IVariableReplacer,
	toolEngine models.IToolEngine,
	workflowInitiator models.IWorkflowInitiator,
) *YAMLDefinedTool {
	return &YAMLDefinedTool{function: function}
}

// Call executes the YAML-defined tool. Stub returns empty.
func (y *YAMLDefinedTool) Call(ctx context.Context, input string) (string, error) {
	return "", fmt.Errorf("YAMLDefinedTool.Call not implemented (stub)")
}

// Name returns the tool name.
func (y *YAMLDefinedTool) Name() string {
	if y.function != nil {
		return y.function.Name
	}
	return ""
}

// Description returns the tool description.
func (y *YAMLDefinedTool) Description() string {
	if y.function != nil {
		return y.function.Description
	}
	return ""
}

// GetExecutorAgenticWorkflowCallback returns the singleton callback instance.
// Stub implementation -- will be replaced by callback.go.
func GetExecutorAgenticWorkflowCallback() (models.AgenticWorkflowCallback, error) {
	return nil, fmt.Errorf("GetExecutorAgenticWorkflowCallback not implemented (stub)")
}
