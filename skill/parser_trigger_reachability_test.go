package skill

import (
	"strings"
	"testing"
)

// TestTriggerReachability_FunctionWithoutTriggerReachableViaNeeds tests that functions without triggers
// are allowed if they are reachable via the 'needs' field from a triggered function
func TestTriggerReachability_FunctionWithoutTriggerReachableViaNeeds(t *testing.T) {
	yamlInput := `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "Test tool with helper function"
    version: "1.0.0"
    functions:
      - name: "mainFunction"
        operation: "api_call"
        description: "Main function with trigger"
        triggers:
          - type: "always_on_user_message"
        needs:
          - name: "helperFunction"
        steps:
          - name: "call api"
            action: "GET"
            with:
              url: "https://example.com"
        output:
          type: "string"
          value: "done"
      - name: "helperFunction"
        operation: "api_call"
        description: "Helper function without trigger, reachable via needs"
        steps:
          - name: "call api"
            action: "GET"
            with:
              url: "https://api.example.com"
        output:
          type: "string"
          value: "helper done"
`
	_, err := CreateTool(yamlInput)
	if err != nil {
		t.Errorf("Expected no error for function reachable via needs, got: %v", err)
	}
}

// TestTriggerReachability_FunctionWithoutTriggerReachableViaOnSuccess tests that functions without triggers
// are allowed if they are reachable via the 'onSuccess' callback from a triggered function
func TestTriggerReachability_FunctionWithoutTriggerReachableViaOnSuccess(t *testing.T) {
	yamlInput := `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "Test tool with onSuccess callback"
    version: "1.0.0"
    functions:
      - name: "mainFunction"
        operation: "api_call"
        description: "Main function with trigger"
        triggers:
          - type: "always_on_user_message"
        onSuccess:
          - name: "successHandler"
        steps:
          - name: "call api"
            action: "GET"
            with:
              url: "https://example.com"
        output:
          type: "string"
          value: "done"
      - name: "successHandler"
        operation: "api_call"
        description: "Success handler without trigger, reachable via onSuccess"
        steps:
          - name: "call api"
            action: "GET"
            with:
              url: "https://api.example.com"
        output:
          type: "string"
          value: "success handled"
`
	_, err := CreateTool(yamlInput)
	if err != nil {
		t.Errorf("Expected no error for function reachable via onSuccess, got: %v", err)
	}
}

// TestTriggerReachability_FunctionWithoutTriggerReachableViaOnFailure tests that functions without triggers
// are allowed if they are reachable via the 'onFailure' callback from a triggered function
func TestTriggerReachability_FunctionWithoutTriggerReachableViaOnFailure(t *testing.T) {
	yamlInput := `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "Test tool with onFailure callback"
    version: "1.0.0"
    functions:
      - name: "mainFunction"
        operation: "api_call"
        description: "Main function with trigger"
        triggers:
          - type: "always_on_user_message"
        onFailure:
          - name: "failureHandler"
        steps:
          - name: "call api"
            action: "GET"
            with:
              url: "https://example.com"
        output:
          type: "string"
          value: "done"
      - name: "failureHandler"
        operation: "api_call"
        description: "Failure handler without trigger, reachable via onFailure"
        steps:
          - name: "call api"
            action: "GET"
            with:
              url: "https://api.example.com"
        output:
          type: "string"
          value: "failure handled"
`
	_, err := CreateTool(yamlInput)
	if err != nil {
		t.Errorf("Expected no error for function reachable via onFailure, got: %v", err)
	}
}

// TestTriggerReachability_FunctionWithoutTriggerReachableViaInputFrom tests that functions without triggers
// are allowed if they are reachable via the 'input.from' with origin "function" from a triggered function
func TestTriggerReachability_FunctionWithoutTriggerReachableViaInputFrom(t *testing.T) {
	yamlInput := `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "Test tool with input from function"
    version: "1.0.0"
    functions:
      - name: "dataProvider"
        operation: "api_call"
        description: "Data provider function without trigger, reachable via input.from"
        steps:
          - name: "fetch data"
            action: "GET"
            resultIndex: 1
            with:
              url: "https://api.example.com/data"
        output:
          type: "object"
          fields:
            - name: "data"
              type: "string"
      - name: "mainFunction"
        operation: "api_call"
        description: "Main function that uses data from provider"
        triggers:
          - type: "always_on_user_message"
        needs:
          - name: "dataProvider"
        input:
          - name: "dataValue"
            description: "Data from provider"
            origin: "function"
            from: "dataProvider"
            onError:
              strategy: "requestUserInput"
              message: "Please provide data"
        steps:
          - name: "use data"
            action: "GET"
            with:
              url: "https://example.com/api?data=$dataValue"
        output:
          type: "string"
          value: "done"
`
	_, err := CreateTool(yamlInput)
	if err != nil {
		t.Errorf("Expected no error for function reachable via input.from, got: %v", err)
	}
}

// TestTriggerReachability_TransitiveReachability tests that functions are allowed if they are
// transitively reachable from a triggered function (e.g., A triggers -> B needs -> C needs)
func TestTriggerReachability_TransitiveReachability(t *testing.T) {
	yamlInput := `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "Test tool with transitive reachability"
    version: "1.0.0"
    functions:
      - name: "mainFunction"
        operation: "api_call"
        description: "Main function with trigger"
        triggers:
          - type: "always_on_user_message"
        needs:
          - name: "middleFunction"
        steps:
          - name: "call api"
            action: "GET"
            with:
              url: "https://example.com"
        output:
          type: "string"
          value: "done"
      - name: "middleFunction"
        operation: "api_call"
        description: "Middle function reachable from main"
        needs:
          - name: "leafFunction"
        steps:
          - name: "call api"
            action: "GET"
            with:
              url: "https://api.example.com/middle"
        output:
          type: "string"
          value: "middle done"
      - name: "leafFunction"
        operation: "api_call"
        description: "Leaf function transitively reachable from main"
        steps:
          - name: "call api"
            action: "GET"
            with:
              url: "https://api.example.com/leaf"
        output:
          type: "string"
          value: "leaf done"
`
	_, err := CreateTool(yamlInput)
	if err != nil {
		t.Errorf("Expected no error for transitive reachability, got: %v", err)
	}
}

// TestTriggerReachability_DeepTransitiveChain tests 5 levels of transitive reachability
// Triggered -> Level1 -> Level2 -> Level3 -> Level4 -> Level5
func TestTriggerReachability_DeepTransitiveChain(t *testing.T) {
	yamlInput := `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "Test tool with 5-level deep chain"
    version: "1.0.0"
    functions:
      - name: "triggeredFunction"
        operation: "api_call"
        description: "Entry point with trigger"
        triggers:
          - type: "always_on_user_message"
        needs:
          - name: "level1"
        steps:
          - name: "call api"
            action: "GET"
            with:
              url: "https://example.com"
        output:
          type: "string"
          value: "done"
      - name: "level1"
        operation: "api_call"
        description: "Level 1 - reachable via needs"
        needs:
          - name: "level2"
        steps:
          - name: "call api"
            action: "GET"
            with:
              url: "https://api.example.com/1"
        output:
          type: "string"
          value: "level1"
      - name: "level2"
        operation: "api_call"
        description: "Level 2 - reachable via level1"
        needs:
          - name: "level3"
        steps:
          - name: "call api"
            action: "GET"
            with:
              url: "https://api.example.com/2"
        output:
          type: "string"
          value: "level2"
      - name: "level3"
        operation: "api_call"
        description: "Level 3 - reachable via level2"
        needs:
          - name: "level4"
        steps:
          - name: "call api"
            action: "GET"
            with:
              url: "https://api.example.com/3"
        output:
          type: "string"
          value: "level3"
      - name: "level4"
        operation: "api_call"
        description: "Level 4 - reachable via level3"
        needs:
          - name: "level5"
        steps:
          - name: "call api"
            action: "GET"
            with:
              url: "https://api.example.com/4"
        output:
          type: "string"
          value: "level4"
      - name: "level5"
        operation: "api_call"
        description: "Level 5 - deepest level, reachable via level4"
        steps:
          - name: "call api"
            action: "GET"
            with:
              url: "https://api.example.com/5"
        output:
          type: "string"
          value: "level5"
`
	_, err := CreateTool(yamlInput)
	if err != nil {
		t.Errorf("Expected no error for 5-level deep chain, got: %v", err)
	}
}

// TestTriggerReachability_MixedDependencyTypesDeep tests mixed dependency types across multiple levels
// Triggered --(needs)--> A --(onSuccess)--> B --(needs)--> C --(onFailure)--> D
func TestTriggerReachability_MixedDependencyTypesDeep(t *testing.T) {
	yamlInput := `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "Test tool with mixed dependency types across levels"
    version: "1.0.0"
    functions:
      - name: "triggeredFunction"
        operation: "api_call"
        description: "Entry point with trigger"
        triggers:
          - type: "time_based"
            cron: "0 * * * *"
        needs:
          - name: "functionA"
        steps:
          - name: "call api"
            action: "GET"
            with:
              url: "https://example.com"
        output:
          type: "string"
          value: "done"
      - name: "functionA"
        operation: "api_call"
        description: "Reachable via needs, calls B on success"
        onSuccess:
          - name: "functionB"
        steps:
          - name: "call api"
            action: "GET"
            with:
              url: "https://api.example.com/a"
        output:
          type: "string"
          value: "a done"
      - name: "functionB"
        operation: "api_call"
        description: "Reachable via onSuccess from A, needs C"
        needs:
          - name: "functionC"
        steps:
          - name: "call api"
            action: "GET"
            with:
              url: "https://api.example.com/b"
        output:
          type: "string"
          value: "b done"
      - name: "functionC"
        operation: "api_call"
        description: "Reachable via needs from B, calls D on failure"
        onFailure:
          - name: "functionD"
        steps:
          - name: "call api"
            action: "GET"
            with:
              url: "https://api.example.com/c"
        output:
          type: "string"
          value: "c done"
      - name: "functionD"
        operation: "api_call"
        description: "Deepest level, reachable via onFailure from C"
        steps:
          - name: "call api"
            action: "GET"
            with:
              url: "https://api.example.com/d"
        output:
          type: "string"
          value: "d done"
`
	_, err := CreateTool(yamlInput)
	if err != nil {
		t.Errorf("Expected no error for mixed dependency types deep chain, got: %v", err)
	}
}

// TestTriggerReachability_DiamondPattern tests a diamond-shaped dependency graph
// Triggered -> A -> B and C -> D (where D is reached from both B and C)
func TestTriggerReachability_DiamondPattern(t *testing.T) {
	yamlInput := `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "Test tool with diamond dependency pattern"
    version: "1.0.0"
    functions:
      - name: "triggeredFunction"
        operation: "api_call"
        description: "Entry point"
        triggers:
          - type: "always_on_user_message"
        needs:
          - name: "functionA"
        steps:
          - name: "call api"
            action: "GET"
            with:
              url: "https://example.com"
        output:
          type: "string"
          value: "done"
      - name: "functionA"
        operation: "api_call"
        description: "Splits into B and C"
        needs:
          - name: "functionB"
          - name: "functionC"
        steps:
          - name: "call api"
            action: "GET"
            with:
              url: "https://api.example.com/a"
        output:
          type: "string"
          value: "a done"
      - name: "functionB"
        operation: "api_call"
        description: "Left branch, needs D"
        needs:
          - name: "functionD"
        steps:
          - name: "call api"
            action: "GET"
            with:
              url: "https://api.example.com/b"
        output:
          type: "string"
          value: "b done"
      - name: "functionC"
        operation: "api_call"
        description: "Right branch, also needs D"
        needs:
          - name: "functionD"
        steps:
          - name: "call api"
            action: "GET"
            with:
              url: "https://api.example.com/c"
        output:
          type: "string"
          value: "c done"
      - name: "functionD"
        operation: "api_call"
        description: "Bottom of diamond, reachable from both B and C"
        steps:
          - name: "call api"
            action: "GET"
            with:
              url: "https://api.example.com/d"
        output:
          type: "string"
          value: "d done"
`
	_, err := CreateTool(yamlInput)
	if err != nil {
		t.Errorf("Expected no error for diamond pattern, got: %v", err)
	}
}

// TestTriggerReachability_PartiallyUnreachableChain tests that if a function in the middle
// of a chain is unreachable, functions below it are also unreachable
func TestTriggerReachability_PartiallyUnreachableChain(t *testing.T) {
	yamlInput := `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "Test tool with partially unreachable chain"
    version: "1.0.0"
    functions:
      - name: "triggeredFunction"
        operation: "api_call"
        description: "Entry point"
        triggers:
          - type: "always_on_user_message"
        needs:
          - name: "functionA"
        steps:
          - name: "call api"
            action: "GET"
            with:
              url: "https://example.com"
        output:
          type: "string"
          value: "done"
      - name: "functionA"
        operation: "api_call"
        description: "Reachable"
        steps:
          - name: "call api"
            action: "GET"
            with:
              url: "https://api.example.com/a"
        output:
          type: "string"
          value: "a done"
      - name: "orphanB"
        operation: "api_call"
        description: "NOT connected to triggered function"
        needs:
          - name: "orphanC"
        steps:
          - name: "call api"
            action: "GET"
            with:
              url: "https://api.example.com/orphan-b"
        output:
          type: "string"
          value: "orphan b"
      - name: "orphanC"
        operation: "api_call"
        description: "Only reachable from orphanB which is itself unreachable"
        steps:
          - name: "call api"
            action: "GET"
            with:
              url: "https://api.example.com/orphan-c"
        output:
          type: "string"
          value: "orphan c"
`
	_, err := CreateTool(yamlInput)
	if err == nil {
		t.Error("Expected error for partially unreachable chain")
	} else if !strings.Contains(err.Error(), "orphanB") && !strings.Contains(err.Error(), "orphanC") {
		t.Errorf("Expected error about orphanB or orphanC, got: %v", err)
	}
}

// TestTriggerReachability_UnreachableFunctionFails tests that functions without triggers
// that are NOT reachable from any triggered function should fail validation
func TestTriggerReachability_UnreachableFunctionFails(t *testing.T) {
	yamlInput := `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "Test tool with unreachable function"
    version: "1.0.0"
    functions:
      - name: "mainFunction"
        operation: "api_call"
        description: "Main function with trigger"
        triggers:
          - type: "always_on_user_message"
        steps:
          - name: "call api"
            action: "GET"
            with:
              url: "https://example.com"
        output:
          type: "string"
          value: "done"
      - name: "orphanFunction"
        operation: "api_call"
        description: "Orphan function without trigger and not reachable"
        steps:
          - name: "call api"
            action: "GET"
            with:
              url: "https://api.example.com"
        output:
          type: "string"
          value: "orphan done"
`
	_, err := CreateTool(yamlInput)
	if err == nil {
		t.Error("Expected error for unreachable function without trigger")
	} else if !strings.Contains(err.Error(), "orphanFunction") || !strings.Contains(err.Error(), "must have at least one trigger or be reachable") {
		t.Errorf("Expected error about unreachable function, got: %v", err)
	}
}

// TestTriggerReachability_NoTriggeredFunctionsFails tests that when no functions have triggers,
// validation should fail for any function
func TestTriggerReachability_NoTriggeredFunctionsFails(t *testing.T) {
	yamlInput := `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "Test tool with no triggered functions"
    version: "1.0.0"
    functions:
      - name: "functionA"
        operation: "api_call"
        description: "Function A without trigger"
        needs:
          - name: "functionB"
        steps:
          - name: "call api"
            action: "GET"
            with:
              url: "https://example.com"
        output:
          type: "string"
          value: "done"
      - name: "functionB"
        operation: "api_call"
        description: "Function B without trigger"
        steps:
          - name: "call api"
            action: "GET"
            with:
              url: "https://api.example.com"
        output:
          type: "string"
          value: "done"
`
	_, err := CreateTool(yamlInput)
	if err == nil {
		t.Error("Expected error when no functions have triggers")
	} else if !strings.Contains(err.Error(), "must have at least one trigger") {
		t.Errorf("Expected error about missing trigger, got: %v", err)
	}
}

// TestTriggerReachability_MultipleTriggeredFunctions tests that multiple functions with triggers
// can each have their own dependency chains
func TestTriggerReachability_MultipleTriggeredFunctions(t *testing.T) {
	yamlInput := `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "Test tool with multiple triggered functions"
    version: "1.0.0"
    functions:
      - name: "triggeredA"
        operation: "api_call"
        description: "Triggered function A"
        triggers:
          - type: "always_on_user_message"
        needs:
          - name: "helperA"
        steps:
          - name: "call api"
            action: "GET"
            with:
              url: "https://example.com/a"
        output:
          type: "string"
          value: "done A"
      - name: "triggeredB"
        operation: "api_call"
        description: "Triggered function B"
        triggers:
          - type: "time_based"
            cron: "0 * * * *"
        needs:
          - name: "helperB"
        steps:
          - name: "call api"
            action: "GET"
            with:
              url: "https://example.com/b"
        output:
          type: "string"
          value: "done B"
      - name: "helperA"
        operation: "api_call"
        description: "Helper A reachable from triggeredA"
        steps:
          - name: "call api"
            action: "GET"
            with:
              url: "https://api.example.com/a"
        output:
          type: "string"
          value: "helper A done"
      - name: "helperB"
        operation: "api_call"
        description: "Helper B reachable from triggeredB"
        steps:
          - name: "call api"
            action: "GET"
            with:
              url: "https://api.example.com/b"
        output:
          type: "string"
          value: "helper B done"
`
	_, err := CreateTool(yamlInput)
	if err != nil {
		t.Errorf("Expected no error for multiple triggered functions, got: %v", err)
	}
}

// TestTriggerReachability_SharedHelper tests that a helper function can be reachable from
// multiple triggered functions
func TestTriggerReachability_SharedHelper(t *testing.T) {
	yamlInput := `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "Test tool with shared helper"
    version: "1.0.0"
    functions:
      - name: "triggeredA"
        operation: "api_call"
        description: "Triggered function A"
        triggers:
          - type: "always_on_user_message"
        needs:
          - name: "sharedHelper"
        steps:
          - name: "call api"
            action: "GET"
            with:
              url: "https://example.com/a"
        output:
          type: "string"
          value: "done A"
      - name: "triggeredB"
        operation: "api_call"
        description: "Triggered function B"
        triggers:
          - type: "always_on_team_message"
        needs:
          - name: "sharedHelper"
        steps:
          - name: "call api"
            action: "GET"
            with:
              url: "https://example.com/b"
        output:
          type: "string"
          value: "done B"
      - name: "sharedHelper"
        operation: "api_call"
        description: "Shared helper reachable from both triggered functions"
        steps:
          - name: "call api"
            action: "GET"
            with:
              url: "https://api.example.com/shared"
        output:
          type: "string"
          value: "shared helper done"
`
	_, err := CreateTool(yamlInput)
	if err != nil {
		t.Errorf("Expected no error for shared helper, got: %v", err)
	}
}

// TestTriggerReachability_ComplexDependencyGraph tests a more complex dependency graph
// with multiple paths and mixed dependency types
func TestTriggerReachability_ComplexDependencyGraph(t *testing.T) {
	yamlInput := `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "Test tool with complex dependency graph"
    version: "1.0.0"
    functions:
      - name: "mainFunction"
        operation: "api_call"
        description: "Main function with trigger"
        triggers:
          - type: "always_on_user_message"
        needs:
          - name: "dataFetcher"
        onSuccess:
          - name: "notificationSender"
        onFailure:
          - name: "errorLogger"
        steps:
          - name: "process data"
            action: "GET"
            with:
              url: "https://example.com"
        output:
          type: "string"
          value: "done"
      - name: "dataFetcher"
        operation: "api_call"
        description: "Fetches data - reachable via needs"
        needs:
          - name: "authProvider"
        steps:
          - name: "fetch"
            action: "GET"
            resultIndex: 1
            with:
              url: "https://api.example.com/data"
        output:
          type: "object"
          fields:
            - name: "data"
              type: "string"
      - name: "authProvider"
        operation: "api_call"
        description: "Provides auth token - transitively reachable"
        steps:
          - name: "get token"
            action: "GET"
            resultIndex: 1
            with:
              url: "https://auth.example.com"
        output:
          type: "object"
          fields:
            - name: "token"
              type: "string"
      - name: "notificationSender"
        operation: "api_call"
        description: "Sends notification - reachable via onSuccess"
        steps:
          - name: "send"
            action: "POST"
            with:
              url: "https://notifications.example.com"
        output:
          type: "string"
          value: "notification sent"
      - name: "errorLogger"
        operation: "api_call"
        description: "Logs error - reachable via onFailure"
        steps:
          - name: "log"
            action: "POST"
            with:
              url: "https://logs.example.com"
        output:
          type: "string"
          value: "error logged"
`
	_, err := CreateTool(yamlInput)
	if err != nil {
		t.Errorf("Expected no error for complex dependency graph, got: %v", err)
	}
}

// Note: TestTriggerReachability_InputFromWithFieldPath and TestTriggerReachability_InputFromWithArrayIndex
// were removed because dot notation in the 'from' field is not supported by the parser.
// The ExtractFunctionNameFromFrom helper function is tested directly via TestExtractFunctionNameFromFrom.

// TestExtractFunctionNameFromFrom tests the helper function that extracts function names
// from the 'from' field
func TestExtractFunctionNameFromFrom(t *testing.T) {
	tests := []struct {
		name     string
		from     string
		expected string
	}{
		{"simple function name", "myFunction", "myFunction"},
		{"with dot notation", "myFunction.field", "myFunction"},
		{"with nested fields", "myFunction.nested.field", "myFunction"},
		{"with array index", "myFunction[0]", "myFunction"},
		{"with array and field", "myFunction[0].field", "myFunction"},
		{"with complex path", "myFunction[0].nested[1].value", "myFunction"},
		{"empty string", "", ""},
		{"underscore in name", "my_function.field", "my_function"},
		{"camelCase", "myDataFunction.value", "myDataFunction"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractFunctionNameFromFrom(tt.from)
			if result != tt.expected {
				t.Errorf("ExtractFunctionNameFromFrom(%q) = %q, want %q", tt.from, result, tt.expected)
			}
		})
	}
}

// TestBuildForwardDependencyGraph tests the graph building function
func TestBuildForwardDependencyGraph(t *testing.T) {
	functions := []Function{
		{
			Name: "funcA",
			Needs: []NeedItem{
				{Name: "funcB"},
				{Name: "funcC"},
			},
		},
		{
			Name: "funcB",
			OnSuccess: []FunctionCall{
				{Name: "funcD"},
			},
		},
		{
			Name: "funcC",
			OnFailure: []FunctionCall{
				{Name: "funcE"},
			},
		},
		{
			Name: "funcD",
			Input: []Input{
				{
					Name:   "data",
					Origin: "function",
					From:   "funcF.value",
				},
			},
		},
		{Name: "funcE"},
		{Name: "funcF"},
	}

	graph := BuildForwardDependencyGraph(functions)

	// Verify edges
	expectedEdges := map[string][]string{
		"funcA": {"funcB", "funcC"},
		"funcB": {"funcD"},
		"funcC": {"funcE"},
		"funcD": {"funcF"},
		"funcE": {},
		"funcF": {},
	}

	for fn, expected := range expectedEdges {
		actual := graph[fn]
		if len(actual) != len(expected) {
			t.Errorf("BuildForwardDependencyGraph: %s has %d edges, want %d", fn, len(actual), len(expected))
			continue
		}
		for _, expectedDep := range expected {
			found := false
			for _, actualDep := range actual {
				if actualDep == expectedDep {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("BuildForwardDependencyGraph: %s missing edge to %s", fn, expectedDep)
			}
		}
	}
}
