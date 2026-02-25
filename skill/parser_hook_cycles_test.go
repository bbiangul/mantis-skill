package skill

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckStaticHookCycles_NoCycle(t *testing.T) {
	yaml := `
version: "v1"
author: "test"
tools:
  - name: "test_tool"
    description: "Test tool"
    version: "1.0.0"
    functions:
      - name: "FunctionA"
        operation: "policy"
        description: "Function A"
        triggers:
          - type: "flex_for_user"
        onSuccess:
          - name: "registerHook"
            params:
              toolName: "test_tool"
              functionName: "FunctionB"
              callbackFunction: "LogActivity"
              triggerType: "afterSuccess"
              ttl: "3600"
        output:
          type: "string"
          value: "done"
      - name: "FunctionB"
        operation: "policy"
        description: "Function B"
        triggers:
          - type: "flex_for_team"
        output:
          type: "string"
          value: "done"
      - name: "LogActivity"
        operation: "policy"
        description: "Log activity"
        triggers:
          - type: "flex_for_team"
        output:
          type: "string"
          value: "logged"
`
	result, err := CreateToolWithWarnings(yaml)
	require.NoError(t, err)

	// No warnings about hook cycles expected
	for _, w := range result.Warnings {
		assert.NotContains(t, w, "hook cycle", "Unexpected hook cycle warning: %s", w)
	}
}

func TestCheckStaticHookCycles_SimpleCycle(t *testing.T) {
	yaml := `
version: "v1"
author: "test"
tools:
  - name: "test_tool"
    description: "Test tool"
    version: "1.0.0"
    functions:
      - name: "SetupHooks"
        operation: "policy"
        description: "Setup hooks with cycle"
        triggers:
          - type: "flex_for_user"
        onSuccess:
          - name: "registerHook"
            params:
              toolName: "test_tool"
              functionName: "FunctionA"
              callbackFunction: "FunctionB"
              triggerType: "afterSuccess"
              ttl: "3600"
          - name: "registerHook"
            params:
              toolName: "test_tool"
              functionName: "FunctionB"
              callbackFunction: "FunctionA"
              triggerType: "afterSuccess"
              ttl: "3600"
        output:
          type: "string"
          value: "done"
      - name: "FunctionA"
        operation: "policy"
        description: "Function A"
        triggers:
          - type: "flex_for_team"
        output:
          type: "string"
          value: "done"
      - name: "FunctionB"
        operation: "policy"
        description: "Function B"
        triggers:
          - type: "flex_for_team"
        output:
          type: "string"
          value: "done"
`
	result, err := CreateToolWithWarnings(yaml)
	require.NoError(t, err)

	// Should have a warning about hook cycle
	hasHookCycleWarning := false
	for _, w := range result.Warnings {
		if strings.Contains(w, "hook cycle") {
			hasHookCycleWarning = true
			// Verify it mentions the cycle
			assert.Contains(t, w, "FunctionA")
			assert.Contains(t, w, "FunctionB")
			break
		}
	}
	assert.True(t, hasHookCycleWarning, "Expected warning about hook cycle, got warnings: %v", result.Warnings)
}

func TestCheckStaticHookCycles_SelfCycle(t *testing.T) {
	yaml := `
version: "v1"
author: "test"
tools:
  - name: "test_tool"
    description: "Test tool"
    version: "1.0.0"
    functions:
      - name: "SetupHooks"
        operation: "policy"
        description: "Setup hooks with self-cycle"
        triggers:
          - type: "flex_for_user"
        onSuccess:
          - name: "registerHook"
            params:
              toolName: "test_tool"
              functionName: "FunctionA"
              callbackFunction: "FunctionA"
              triggerType: "afterSuccess"
              ttl: "3600"
        output:
          type: "string"
          value: "done"
      - name: "FunctionA"
        operation: "policy"
        description: "Function A"
        triggers:
          - type: "flex_for_team"
        output:
          type: "string"
          value: "done"
`
	result, err := CreateToolWithWarnings(yaml)
	require.NoError(t, err)

	// Should have a warning about hook cycle (self-referencing)
	hasHookCycleWarning := false
	for _, w := range result.Warnings {
		if strings.Contains(w, "hook cycle") {
			hasHookCycleWarning = true
			assert.Contains(t, w, "FunctionA")
			break
		}
	}
	assert.True(t, hasHookCycleWarning, "Expected warning about self-referencing hook cycle, got warnings: %v", result.Warnings)
}

func TestCheckStaticHookCycles_VariableParams_NoCycle(t *testing.T) {
	// When params are variables, we can't detect cycles at parse time
	yaml := `
version: "v1"
author: "test"
tools:
  - name: "test_tool"
    description: "Test tool"
    version: "1.0.0"
    functions:
      - name: "SetupHooks"
        operation: "policy"
        description: "Setup hooks with variable params"
        triggers:
          - type: "flex_for_user"
        onSuccess:
          - name: "registerHook"
            params:
              toolName: "test_tool"
              functionName: "$targetFunction"
              callbackFunction: "$callbackFunction"
              triggerType: "afterSuccess"
              ttl: "3600"
        output:
          type: "string"
          value: "done"
      - name: "FunctionA"
        operation: "policy"
        description: "Function A"
        triggers:
          - type: "flex_for_team"
        output:
          type: "string"
          value: "done"
`
	result, err := CreateToolWithWarnings(yaml)
	require.NoError(t, err)

	// Should NOT have a warning about hook cycle (can't detect with variables)
	for _, w := range result.Warnings {
		assert.NotContains(t, w, "hook cycle", "Should not detect cycle with variable params: %s", w)
	}
}

func TestCheckStaticHookCycles_ThreeNodeCycle(t *testing.T) {
	yaml := `
version: "v1"
author: "test"
tools:
  - name: "test_tool"
    description: "Test tool"
    version: "1.0.0"
    functions:
      - name: "SetupHooks"
        operation: "policy"
        description: "Setup hooks with 3-node cycle"
        triggers:
          - type: "flex_for_user"
        onSuccess:
          - name: "registerHook"
            params:
              toolName: "test_tool"
              functionName: "FunctionA"
              callbackFunction: "FunctionB"
              triggerType: "afterSuccess"
              ttl: "3600"
          - name: "registerHook"
            params:
              toolName: "test_tool"
              functionName: "FunctionB"
              callbackFunction: "FunctionC"
              triggerType: "afterSuccess"
              ttl: "3600"
          - name: "registerHook"
            params:
              toolName: "test_tool"
              functionName: "FunctionC"
              callbackFunction: "FunctionA"
              triggerType: "afterSuccess"
              ttl: "3600"
        output:
          type: "string"
          value: "done"
      - name: "FunctionA"
        operation: "policy"
        description: "Function A"
        triggers:
          - type: "flex_for_team"
        output:
          type: "string"
          value: "done"
      - name: "FunctionB"
        operation: "policy"
        description: "Function B"
        triggers:
          - type: "flex_for_team"
        output:
          type: "string"
          value: "done"
      - name: "FunctionC"
        operation: "policy"
        description: "Function C"
        triggers:
          - type: "flex_for_team"
        output:
          type: "string"
          value: "done"
`
	result, err := CreateToolWithWarnings(yaml)
	require.NoError(t, err)

	// Should have a warning about hook cycle
	hasHookCycleWarning := false
	for _, w := range result.Warnings {
		if strings.Contains(w, "hook cycle") {
			hasHookCycleWarning = true
			break
		}
	}
	assert.True(t, hasHookCycleWarning, "Expected warning about 3-node hook cycle, got warnings: %v", result.Warnings)
}

func TestCheckStaticHookCycles_InNeeds(t *testing.T) {
	yaml := `
version: "v1"
author: "test"
tools:
  - name: "test_tool"
    description: "Test tool"
    version: "1.0.0"
    functions:
      - name: "FunctionA"
        operation: "policy"
        description: "Function A with hook cycle in needs"
        triggers:
          - type: "flex_for_user"
        needs:
          - name: "registerHook"
            params:
              toolName: "test_tool"
              functionName: "FunctionB"
              callbackFunction: "FunctionA"
              triggerType: "afterSuccess"
              ttl: "3600"
          - name: "registerHook"
            params:
              toolName: "test_tool"
              functionName: "FunctionA"
              callbackFunction: "FunctionB"
              triggerType: "afterSuccess"
              ttl: "3600"
        output:
          type: "string"
          value: "done"
      - name: "FunctionB"
        operation: "policy"
        description: "Function B"
        triggers:
          - type: "flex_for_team"
        output:
          type: "string"
          value: "done"
`
	result, err := CreateToolWithWarnings(yaml)
	require.NoError(t, err)

	// Should have a warning about hook cycle (FunctionA -> FunctionB -> FunctionA)
	hasHookCycleWarning := false
	for _, w := range result.Warnings {
		if strings.Contains(w, "hook cycle") {
			hasHookCycleWarning = true
			break
		}
	}
	assert.True(t, hasHookCycleWarning, "Expected warning about hook cycle in needs, got warnings: %v", result.Warnings)
}

func TestCheckStaticHookCycles_DotNotation(t *testing.T) {
	yaml := `
version: "v1"
author: "test"
tools:
  - name: "test_tool"
    description: "Test tool"
    version: "1.0.0"
    functions:
      - name: "SetupHooks"
        operation: "policy"
        description: "Setup hooks with dot notation"
        triggers:
          - type: "flex_for_user"
        onSuccess:
          - name: "registerHook"
            params:
              toolName: "test_tool"
              functionName: "FunctionA"
              callbackFunction: "test_tool.FunctionB"
              triggerType: "afterSuccess"
              ttl: "3600"
          - name: "registerHook"
            params:
              toolName: "test_tool"
              functionName: "FunctionB"
              callbackFunction: "test_tool.FunctionA"
              triggerType: "afterSuccess"
              ttl: "3600"
        output:
          type: "string"
          value: "done"
      - name: "FunctionA"
        operation: "policy"
        description: "Function A"
        triggers:
          - type: "flex_for_team"
        output:
          type: "string"
          value: "done"
      - name: "FunctionB"
        operation: "policy"
        description: "Function B"
        triggers:
          - type: "flex_for_team"
        output:
          type: "string"
          value: "done"
`
	result, err := CreateToolWithWarnings(yaml)
	require.NoError(t, err)

	// Should have a warning about hook cycle with dot notation
	hasHookCycleWarning := false
	for _, w := range result.Warnings {
		if strings.Contains(w, "hook cycle") {
			hasHookCycleWarning = true
			break
		}
	}
	assert.True(t, hasHookCycleWarning, "Expected warning about hook cycle with dot notation, got warnings: %v", result.Warnings)
}

func TestCheckStaticHookCycles_NoDuplicateWarnings(t *testing.T) {
	// Test that a simple cycle (A→B→A) only produces ONE warning, not two
	yaml := `
version: "v1"
author: "test"
tools:
  - name: "test_tool"
    description: "Test tool"
    version: "1.0.0"
    functions:
      - name: "SetupHooks"
        operation: "policy"
        description: "Setup hooks with cycle"
        triggers:
          - type: "flex_for_user"
        onSuccess:
          - name: "registerHook"
            params:
              toolName: "test_tool"
              functionName: "FunctionA"
              callbackFunction: "FunctionB"
              triggerType: "afterSuccess"
              ttl: "3600"
          - name: "registerHook"
            params:
              toolName: "test_tool"
              functionName: "FunctionB"
              callbackFunction: "FunctionA"
              triggerType: "afterSuccess"
              ttl: "3600"
        output:
          type: "string"
          value: "done"
      - name: "FunctionA"
        operation: "policy"
        description: "Function A"
        triggers:
          - type: "flex_for_team"
        output:
          type: "string"
          value: "done"
      - name: "FunctionB"
        operation: "policy"
        description: "Function B"
        triggers:
          - type: "flex_for_team"
        output:
          type: "string"
          value: "done"
`
	result, err := CreateToolWithWarnings(yaml)
	require.NoError(t, err)

	// Count hook cycle warnings - should be exactly 1, not 2
	hookCycleWarningCount := 0
	for _, w := range result.Warnings {
		if strings.Contains(w, "hook cycle") {
			hookCycleWarningCount++
		}
	}
	assert.Equal(t, 1, hookCycleWarningCount, "Expected exactly 1 hook cycle warning (no duplicates), got %d: %v", hookCycleWarningCount, result.Warnings)
}
