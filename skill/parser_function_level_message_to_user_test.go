package skill

import (
	"strings"
	"testing"
)

func TestFunctionLevelShouldBeHandledAsMessageToUser(t *testing.T) {
	// Test that function-level shouldBeHandledAsMessageToUser is parsed correctly
	yamlDef := `
version: v1
author: test

tools:
  - name: testTool
    description: Test tool with function-level shouldBeHandledAsMessageToUser
    version: 1.0.0
    functions:
      - name: getCustomerInfo
        operation: api_call
        description: Fetch customer info
        shouldBeHandledAsMessageToUser: true
        triggers:
          - type: flex_for_user
        input:
          - name: customerId
            description: Customer ID
            origin: chat
            onError:
              strategy: requestUserInput
              message: Please provide customer ID
        steps:
          - name: fetch
            action: GET
            with:
              url: "https://api.example.com/customers/$customerId"
            resultIndex: 1
        output:
          type: string
          value: "done"
`

	result, err := CreateToolWithWarnings(yamlDef)
	if err != nil {
		t.Fatalf("CreateToolWithWarnings() error = %v", err)
	}

	if len(result.Tool.Tools) != 1 {
		t.Fatalf("Expected 1 tool, got %d", len(result.Tool.Tools))
	}

	tool := result.Tool.Tools[0]
	if len(tool.Functions) != 1 {
		t.Fatalf("Expected 1 function, got %d", len(tool.Functions))
	}

	fn := tool.Functions[0]
	if !fn.ShouldBeHandledAsMessageToUser {
		t.Error("Expected ShouldBeHandledAsMessageToUser to be true")
	}
}

func TestFunctionLevelShouldBeHandledAsMessageToUserDefault(t *testing.T) {
	// Test that function-level shouldBeHandledAsMessageToUser defaults to false
	yamlDef := `
version: v1
author: test

tools:
  - name: testTool
    description: Test tool without shouldBeHandledAsMessageToUser
    version: 1.0.0
    functions:
      - name: getCustomerInfo
        operation: api_call
        description: Fetch customer info
        triggers:
          - type: flex_for_user
        input:
          - name: customerId
            description: Customer ID
            origin: chat
            onError:
              strategy: requestUserInput
              message: Please provide customer ID
        steps:
          - name: fetch
            action: GET
            with:
              url: "https://api.example.com/customers/$customerId"
            resultIndex: 1
        output:
          type: string
          value: "done"
`

	result, err := CreateToolWithWarnings(yamlDef)
	if err != nil {
		t.Fatalf("CreateToolWithWarnings() error = %v", err)
	}

	fn := result.Tool.Tools[0].Functions[0]
	if fn.ShouldBeHandledAsMessageToUser {
		t.Error("Expected ShouldBeHandledAsMessageToUser to default to false")
	}
}

func TestFunctionLevelShouldBeHandledAsMessageToUserAllOperations(t *testing.T) {
	// Test that shouldBeHandledAsMessageToUser works with api_call and db operations
	// (terminal requires both linux/windows, format requires inference input - tested separately)
	operations := []struct {
		operation string
		steps     string
	}{
		{
			operation: "api_call",
			steps: `
        steps:
          - name: fetch
            action: GET
            with:
              url: "https://api.example.com/test"
            resultIndex: 1`,
		},
		{
			operation: "db",
			steps: `
        steps:
          - name: query
            action: select
            with:
              select: "SELECT 1"
            resultIndex: 1`,
		},
	}

	for _, op := range operations {
		t.Run(op.operation, func(t *testing.T) {
			yamlDef := `
version: v1
author: test

tools:
  - name: testTool
    description: Test tool
    version: 1.0.0
    functions:
      - name: testFunc
        operation: ` + op.operation + `
        description: Test function
        shouldBeHandledAsMessageToUser: true
        triggers:
          - type: flex_for_user
        input:
          - name: testInput
            description: Test input
            origin: chat
            onError:
              strategy: requestUserInput
              message: Please provide input` + op.steps + `
        output:
          type: string
          value: "test"
`

			result, err := CreateToolWithWarnings(yamlDef)
			if err != nil {
				t.Fatalf("CreateToolWithWarnings() error for operation %s = %v", op.operation, err)
			}

			fn := result.Tool.Tools[0].Functions[0]
			if !fn.ShouldBeHandledAsMessageToUser {
				t.Errorf("Expected ShouldBeHandledAsMessageToUser to be true for operation %s", op.operation)
			}
		})
	}
}

func TestFunctionLevelShouldBeHandledAsMessageToUserConflictWithInputLevel(t *testing.T) {
	// Test that using both function-level AND input-level shouldBeHandledAsMessageToUser fails validation
	yamlDef := `
version: v1
author: test

tools:
  - name: testTool
    description: Test tool with conflicting flags
    version: 1.0.0
    functions:
      - name: conflictingFunc
        operation: format
        description: Function with both function-level and input-level flags
        shouldBeHandledAsMessageToUser: true
        triggers:
          - type: flex_for_user
        input:
          - name: testInput
            description: Test input
            origin: inference
            shouldBeHandledAsMessageToUser: true
            successCriteria: "Generate test output"
            onError:
              strategy: inference
              message: Please provide input
        steps: []
        output:
          type: string
          value: "$testInput"
`

	_, err := CreateToolWithWarnings(yamlDef)
	if err == nil {
		t.Fatal("Expected error for conflicting function-level and input-level shouldBeHandledAsMessageToUser, got none")
	}

	if !strings.Contains(err.Error(), "shouldBeHandledAsMessageToUser at both function and input level") {
		t.Errorf("Expected error about conflicting flags, got: %v", err)
	}
}

func TestFunctionLevelShouldBeHandledAsMessageToUserWithInputLevelFalse(t *testing.T) {
	// Test that function-level true + input-level false is allowed (input level is default false)
	yamlDef := `
version: v1
author: test

tools:
  - name: testTool
    description: Test tool with function-level flag only
    version: 1.0.0
    functions:
      - name: validFunc
        operation: format
        description: Function with function-level flag only
        shouldBeHandledAsMessageToUser: true
        triggers:
          - type: flex_for_user
        input:
          - name: testInput
            description: Test input
            origin: inference
            successCriteria: "Generate test output"
            onError:
              strategy: inference
              message: Please provide input
        steps: []
        output:
          type: string
          value: "$testInput"
`

	result, err := CreateToolWithWarnings(yamlDef)
	if err != nil {
		t.Fatalf("CreateToolWithWarnings() error = %v", err)
	}

	fn := result.Tool.Tools[0].Functions[0]
	if !fn.ShouldBeHandledAsMessageToUser {
		t.Error("Expected ShouldBeHandledAsMessageToUser to be true")
	}

	// Input-level should be false (default)
	if fn.Input[0].ShouldBeHandledAsMessageToUser {
		t.Error("Expected input-level ShouldBeHandledAsMessageToUser to be false")
	}
}

func TestInputLevelShouldBeHandledAsMessageToUserStillWorks(t *testing.T) {
	// Test that input-level shouldBeHandledAsMessageToUser still works when function-level is not set
	yamlDef := `
version: v1
author: test

tools:
  - name: testTool
    description: Test tool with input-level flag
    version: 1.0.0
    functions:
      - name: inputLevelFunc
        operation: format
        description: Function with input-level flag only
        triggers:
          - type: flex_for_user
        input:
          - name: testInput
            description: Test input
            origin: inference
            shouldBeHandledAsMessageToUser: true
            successCriteria: "Generate test output"
            onError:
              strategy: inference
              message: Please provide input
        steps: []
        output:
          type: string
          value: "$testInput"
`

	result, err := CreateToolWithWarnings(yamlDef)
	if err != nil {
		t.Fatalf("CreateToolWithWarnings() error = %v", err)
	}

	fn := result.Tool.Tools[0].Functions[0]

	// Function-level should be false (default)
	if fn.ShouldBeHandledAsMessageToUser {
		t.Error("Expected function-level ShouldBeHandledAsMessageToUser to be false")
	}

	// Input-level should be true
	if !fn.Input[0].ShouldBeHandledAsMessageToUser {
		t.Error("Expected input-level ShouldBeHandledAsMessageToUser to be true")
	}
}
