package skill

import (
	"strings"
	"testing"
)

func TestWorkflowValidation_Success(t *testing.T) {
	testCases := []struct {
		name       string
		yamlInput  string
		validation func(*testing.T, CustomTool)
	}{
		{
			name: "Valid workflow with PUBLIC function",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "PublicFunction"
        operation: "format"
        description: "A public function"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "message"
            description: "Message"
            origin: "inference"
            shouldBeHandledAsMessageToUser: true
            successCriteria: "Generate message"
            onError:
              strategy: "inference"
              message: "Failed"
        steps: []

workflows:
  - category: "test_workflow"
    human_category: "Test Workflow"
    description: "A test workflow"
    workflow_type: "user"
    steps:
      - order: 0
        action: "PublicFunction"
        human_action: "Execute Public Function"
        instructions: "When to execute this function"
        expected_outcome: "Function executed successfully"
`,
			validation: func(t *testing.T, tool CustomTool) {
				if len(tool.Workflows) != 1 {
					t.Errorf("Expected 1 workflow, got %d", len(tool.Workflows))
				}
				if tool.Workflows[0].CategoryName != "test_workflow" {
					t.Errorf("Expected category 'test_workflow', got %s", tool.Workflows[0].CategoryName)
				}
				if tool.Workflows[0].HumanReadableCategoryName != "Test Workflow" {
					t.Errorf("Expected human category 'Test Workflow', got %s", tool.Workflows[0].HumanReadableCategoryName)
				}
				if len(tool.Workflows[0].Steps) != 1 {
					t.Errorf("Expected 1 step, got %d", len(tool.Workflows[0].Steps))
				}
			},
		},
		{
			name: "Multiple workflows with multiple steps",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "StepOne"
        operation: "format"
        description: "First step"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "input1"
            description: "Input"
            origin: "inference"
            shouldBeHandledAsMessageToUser: true
            successCriteria: "Generate"
            onError:
              strategy: "inference"
              message: "Failed"
        steps: []

      - name: "StepTwo"
        operation: "format"
        description: "Second step"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "input2"
            description: "Input"
            origin: "inference"
            shouldBeHandledAsMessageToUser: true
            successCriteria: "Generate"
            onError:
              strategy: "inference"
              message: "Failed"
        steps: []

workflows:
  - category: "workflow_one"
    human_category: "Workflow One"
    description: "First workflow"
    workflow_type: "user"
    steps:
      - order: 0
        action: "StepOne"
        human_action: "First Step"
        instructions: "Execute first"

      - order: 1
        action: "StepTwo"
        human_action: "Second Step"
        instructions: "Execute second"

  - category: "workflow_two"
    human_category: "Workflow Two"
    description: "Second workflow"
    workflow_type: "user"
    steps:
      - order: 0
        action: "StepTwo"
        human_action: "Only Step"
        instructions: "Execute alone"
`,
			validation: func(t *testing.T, tool CustomTool) {
				if len(tool.Workflows) != 2 {
					t.Errorf("Expected 2 workflows, got %d", len(tool.Workflows))
				}
				if len(tool.Workflows[0].Steps) != 2 {
					t.Errorf("Expected 2 steps in first workflow, got %d", len(tool.Workflows[0].Steps))
				}
				if len(tool.Workflows[1].Steps) != 1 {
					t.Errorf("Expected 1 step in second workflow, got %d", len(tool.Workflows[1].Steps))
				}
			},
		},
		{
			name: "Workflow with system function",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "PublicFunction"
        operation: "format"
        description: "Public function"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "msg"
            description: "Message"
            origin: "inference"
            shouldBeHandledAsMessageToUser: true
            successCriteria: "Generate"
            onError:
              strategy: "inference"
              message: "Failed"
        steps: []

workflows:
  - category: "system_workflow"
    human_category: "System Workflow"
    description: "Workflow using system functions"
    workflow_type: "user"
    steps:
      - order: 0
        action: "PublicFunction"
        human_action: "Public Step"
        instructions: "Execute public function"

      - order: 1
        action: "requestInternalTeamInfo"
        human_action: "Get Team Info"
        instructions: "Request team information"

      - order: 2
        action: "askToTheConversationHistoryWithCustomer"
        human_action: "Check History"
        instructions: "Review conversation history"
`,
			validation: func(t *testing.T, tool CustomTool) {
				if len(tool.Workflows) != 1 {
					t.Errorf("Expected 1 workflow, got %d", len(tool.Workflows))
				}
				if len(tool.Workflows[0].Steps) != 3 {
					t.Errorf("Expected 3 steps, got %d", len(tool.Workflows[0].Steps))
				}
				// Verify system function step
				if tool.Workflows[0].Steps[1].Action != "requestInternalTeamInfo" {
					t.Errorf("Expected system function 'requestInternalTeamInfo', got %s", tool.Workflows[0].Steps[1].Action)
				}
			},
		},
		{
			name: "Workflow with optional expected_outcome",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "PublicFunc"
        operation: "format"
        description: "Public function"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "msg"
            description: "Message"
            origin: "inference"
            shouldBeHandledAsMessageToUser: true
            successCriteria: "Generate"
            onError:
              strategy: "inference"
              message: "Failed"
        steps: []

workflows:
  - category: "minimal_workflow"
    human_category: "Minimal Workflow"
    description: "Workflow without expected outcomes"
    workflow_type: "user"
    steps:
      - order: 0
        action: "PublicFunc"
        human_action: "Execute"
        instructions: "Execute this function"
`,
			validation: func(t *testing.T, tool CustomTool) {
				if len(tool.Workflows) != 1 {
					t.Errorf("Expected 1 workflow, got %d", len(tool.Workflows))
				}
				step := tool.Workflows[0].Steps[0]
				if step.ExpectedOutcomeDescription != "" {
					t.Errorf("Expected empty expected_outcome, got %s", step.ExpectedOutcomeDescription)
				}
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tool, err := CreateTool(tc.yamlInput)
			if err != nil {
				t.Fatalf("Expected no error, got: %v", err)
			}
			tc.validation(t, tool)
		})
	}
}

func TestWorkflowValidation_Failures(t *testing.T) {
	testCases := []struct {
		name          string
		yamlInput     string
		expectedError string
	}{
		{
			name: "Workflow references private function (lowercase)",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "privateFunction"
        operation: "format"
        description: "Private function"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "msg"
            description: "Message"
            origin: "inference"
            shouldBeHandledAsMessageToUser: true
            successCriteria: "Generate"
            onError:
              strategy: "inference"
              message: "Failed"
        steps: []

workflows:
  - category: "bad_workflow"
    human_category: "Bad Workflow"
    description: "Workflow using private function"
    workflow_type: "user"
    steps:
      - order: 0
        action: "privateFunction"
        human_action: "Private Step"
        instructions: "Try to use private function"
`,
			expectedError: "references unknown action 'privateFunction'",
		},
		{
			name: "Workflow with non-snake_case category",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "PublicFunc"
        operation: "format"
        description: "Public function"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "msg"
            description: "Message"
            origin: "inference"
            shouldBeHandledAsMessageToUser: true
            successCriteria: "Generate"
            onError:
              strategy: "inference"
              message: "Failed"
        steps: []

workflows:
  - category: "Bad-Workflow-Name"
    human_category: "Bad Workflow"
    description: "Workflow with invalid category name"
    workflow_type: "user"
    steps:
      - order: 0
        action: "PublicFunc"
        human_action: "Execute"
        instructions: "Execute function"
`,
			expectedError: "must be in snake_case format",
		},
		{
			name: "Workflow with duplicate category names",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "PublicFunc"
        operation: "format"
        description: "Public function"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "msg"
            description: "Message"
            origin: "inference"
            shouldBeHandledAsMessageToUser: true
            successCriteria: "Generate"
            onError:
              strategy: "inference"
              message: "Failed"
        steps: []

workflows:
  - category: "duplicate_name"
    human_category: "First"
    description: "First workflow"
    workflow_type: "user"
    steps:
      - order: 0
        action: "PublicFunc"
        human_action: "Step"
        instructions: "Execute"

  - category: "duplicate_name"
    human_category: "Second"
    description: "Second workflow"
    workflow_type: "user"
    steps:
      - order: 0
        action: "PublicFunc"
        human_action: "Step"
        instructions: "Execute"
`,
			expectedError: "duplicate workflow category name",
		},
		{
			name: "Workflow with empty category name",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "PublicFunc"
        operation: "format"
        description: "Public function"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "msg"
            description: "Message"
            origin: "inference"
            shouldBeHandledAsMessageToUser: true
            successCriteria: "Generate"
            onError:
              strategy: "inference"
              message: "Failed"
        steps: []

workflows:
  - category: ""
    human_category: "Empty Category"
    description: "Workflow with empty category"
    workflow_type: "user"
    steps:
      - order: 0
        action: "PublicFunc"
        human_action: "Step"
        instructions: "Execute"
`,
			expectedError: "has an empty category name",
		},
		{
			name: "Workflow with empty human_category",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "PublicFunc"
        operation: "format"
        description: "Public function"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "msg"
            description: "Message"
            origin: "inference"
            shouldBeHandledAsMessageToUser: true
            successCriteria: "Generate"
            onError:
              strategy: "inference"
              message: "Failed"
        steps: []

workflows:
  - category: "test_workflow"
    human_category: ""
    description: "Workflow with empty human category"
    workflow_type: "user"
    steps:
      - order: 0
        action: "PublicFunc"
        human_action: "Step"
        instructions: "Execute"
`,
			expectedError: "has an empty human_category",
		},
		{
			name: "Workflow with empty description",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "PublicFunc"
        operation: "format"
        description: "Public function"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "msg"
            description: "Message"
            origin: "inference"
            shouldBeHandledAsMessageToUser: true
            successCriteria: "Generate"
            onError:
              strategy: "inference"
              message: "Failed"
        steps: []

workflows:
  - category: "test_workflow"
    human_category: "Test Workflow"
    description: ""
    workflow_type: "user"
    steps:
      - order: 0
        action: "PublicFunc"
        human_action: "Step"
        instructions: "Execute"
`,
			expectedError: "has an empty description",
		},
		{
			name: "Workflow with no steps",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "PublicFunc"
        operation: "format"
        description: "Public function"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "msg"
            description: "Message"
            origin: "inference"
            shouldBeHandledAsMessageToUser: true
            successCriteria: "Generate"
            onError:
              strategy: "inference"
              message: "Failed"
        steps: []

workflows:
  - category: "empty_workflow"
    human_category: "Empty Workflow"
    description: "Workflow with no steps"
    workflow_type: "user"
    steps: []
`,
			expectedError: "must have at least one step",
		},
		{
			name: "Workflow with duplicate step order",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "PublicFunc"
        operation: "format"
        description: "Public function"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "msg"
            description: "Message"
            origin: "inference"
            shouldBeHandledAsMessageToUser: true
            successCriteria: "Generate"
            onError:
              strategy: "inference"
              message: "Failed"
        steps: []

workflows:
  - category: "duplicate_order"
    human_category: "Duplicate Order"
    description: "Workflow with duplicate step orders"
    workflow_type: "user"
    steps:
      - order: 0
        action: "PublicFunc"
        human_action: "First Step"
        instructions: "Execute first"

      - order: 0
        action: "PublicFunc"
        human_action: "Second Step"
        instructions: "Execute second"
`,
			expectedError: "duplicate step order",
		},
		{
			name: "Workflow step with empty action",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "PublicFunc"
        operation: "format"
        description: "Public function"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "msg"
            description: "Message"
            origin: "inference"
            shouldBeHandledAsMessageToUser: true
            successCriteria: "Generate"
            onError:
              strategy: "inference"
              message: "Failed"
        steps: []

workflows:
  - category: "empty_action"
    human_category: "Empty Action"
    description: "Workflow with empty action"
    workflow_type: "user"
    steps:
      - order: 0
        action: ""
        human_action: "Step"
        instructions: "Execute"
`,
			expectedError: "with an empty action",
		},
		{
			name: "Workflow step with empty human_action",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "PublicFunc"
        operation: "format"
        description: "Public function"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "msg"
            description: "Message"
            origin: "inference"
            shouldBeHandledAsMessageToUser: true
            successCriteria: "Generate"
            onError:
              strategy: "inference"
              message: "Failed"
        steps: []

workflows:
  - category: "empty_human_action"
    human_category: "Empty Human Action"
    description: "Workflow with empty human action"
    workflow_type: "user"
    steps:
      - order: 0
        action: "PublicFunc"
        human_action: ""
        instructions: "Execute"
`,
			expectedError: "has an empty human_action",
		},
		{
			name: "Workflow step with empty instructions",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "PublicFunc"
        operation: "format"
        description: "Public function"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "msg"
            description: "Message"
            origin: "inference"
            shouldBeHandledAsMessageToUser: true
            successCriteria: "Generate"
            onError:
              strategy: "inference"
              message: "Failed"
        steps: []

workflows:
  - category: "empty_instructions"
    human_category: "Empty Instructions"
    description: "Workflow with empty instructions"
    workflow_type: "user"
    steps:
      - order: 0
        action: "PublicFunc"
        human_action: "Step"
        instructions: ""
`,
			expectedError: "has empty instructions",
		},
		{
			name: "Workflow references non-existent function",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "PublicFunc"
        operation: "format"
        description: "Public function"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "msg"
            description: "Message"
            origin: "inference"
            shouldBeHandledAsMessageToUser: true
            successCriteria: "Generate"
            onError:
              strategy: "inference"
              message: "Failed"
        steps: []

workflows:
  - category: "missing_function"
    human_category: "Missing Function"
    description: "Workflow referencing non-existent function"
    workflow_type: "user"
    steps:
      - order: 0
        action: "NonExistentFunction"
        human_action: "Step"
        instructions: "Execute"
`,
			expectedError: "references unknown action 'NonExistentFunction'",
		},
		{
			name: "Workflow with negative step order",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "PublicFunc"
        operation: "format"
        description: "Public function"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "msg"
            description: "Message"
            origin: "inference"
            shouldBeHandledAsMessageToUser: true
            successCriteria: "Generate"
            onError:
              strategy: "inference"
              message: "Failed"
        steps: []

workflows:
  - category: "negative_order"
    human_category: "Negative Order"
    description: "Workflow with negative step order"
    workflow_type: "user"
    steps:
      - order: -1
        action: "PublicFunc"
        human_action: "Step"
        instructions: "Execute"
`,
			expectedError: "invalid order -1; order must be >= 0",
		},
		{
			name: "Workflow with missing workflow_type",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "PublicFunc"
        operation: "format"
        description: "Public function"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "msg"
            description: "Message"
            origin: "inference"
            shouldBeHandledAsMessageToUser: true
            successCriteria: "Generate"
            onError:
              strategy: "inference"
              message: "Failed"
        steps: []

workflows:
  - category: "missing_type"
    human_category: "Missing Type"
    description: "Workflow without workflow_type field"
    steps:
      - order: 0
        action: "PublicFunc"
        human_action: "Step"
        instructions: "Execute"
`,
			expectedError: "is missing required field 'workflow_type'",
		},
		{
			name: "Workflow with invalid workflow_type",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "PublicFunc"
        operation: "format"
        description: "Public function"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "msg"
            description: "Message"
            origin: "inference"
            shouldBeHandledAsMessageToUser: true
            successCriteria: "Generate"
            onError:
              strategy: "inference"
              message: "Failed"
        steps: []

workflows:
  - category: "invalid_type"
    human_category: "Invalid Type"
    description: "Workflow with invalid workflow_type value"
    workflow_type: "invalid"
    steps:
      - order: 0
        action: "PublicFunc"
        human_action: "Step"
        instructions: "Execute"
`,
			expectedError: "has invalid workflow_type 'invalid'",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := CreateTool(tc.yamlInput)
			if err == nil {
				t.Fatalf("Expected error containing '%s', got no error", tc.expectedError)
			}
			if !strings.Contains(err.Error(), tc.expectedError) {
				t.Errorf("Expected error containing '%s', got: %v", tc.expectedError, err)
			}
		})
	}
}

func TestWorkflowValidation_PublicVsPrivate(t *testing.T) {
	testCases := []struct {
		name         string
		functionName string
		shouldPass   bool
	}{
		// PUBLIC functions (uppercase first letter)
		{"Uppercase function", "PublicFunction", true},
		{"Uppercase with underscore", "Public_Function", true},
		{"Single uppercase letter", "P", true},
		{"Uppercase with numbers", "Function123", true},

		// Private functions (lowercase first letter)
		{"Lowercase function", "privateFunction", false},
		{"Lowercase with underscore", "private_function", false},
		{"Single lowercase letter", "p", false},
		{"Lowercase with numbers", "function123", false},
		{"camelCase", "functionName", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			yamlInput := `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "` + tc.functionName + `"
        operation: "format"
        description: "Test function"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "msg"
            description: "Message"
            origin: "inference"
            shouldBeHandledAsMessageToUser: true
            successCriteria: "Generate"
            onError:
              strategy: "inference"
              message: "Failed"
        steps: []

workflows:
  - category: "test_workflow"
    human_category: "Test Workflow"
    description: "Test"
    workflow_type: "user"
    steps:
      - order: 0
        action: "` + tc.functionName + `"
        human_action: "Step"
        instructions: "Execute"
`
			_, err := CreateTool(yamlInput)
			if tc.shouldPass && err != nil {
				t.Errorf("Expected function '%s' to be allowed in workflow, got error: %v", tc.functionName, err)
			}
			if !tc.shouldPass && err == nil {
				t.Errorf("Expected function '%s' to be rejected in workflow, but it was allowed", tc.functionName)
			}
			if !tc.shouldPass && err != nil {
				if !strings.Contains(err.Error(), "references unknown action") {
					t.Errorf("Expected 'references unknown action' error for '%s', got: %v", tc.functionName, err)
				}
			}
		})
	}
}

func TestWorkflowValidation_MisplacedWorkflowsInsideTool(t *testing.T) {
	// This test verifies that workflows defined inside a tool definition are silently ignored
	// and that a warning is generated for misplaced workflows
	yamlInput := `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "PublicFunc"
        operation: "format"
        description: "Public function"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "msg"
            description: "Message"
            origin: "inference"
            shouldBeHandledAsMessageToUser: true
            successCriteria: "Generate"
            onError:
              strategy: "inference"
              message: "Failed"
        steps: []
    workflows:  # This is WRONG - workflows inside tool will be ignored!
      - name: "test_workflow"
        description: "This workflow is incorrectly placed inside the tool"
        suggestedFunctions:
          - "PublicFunc"
`
	// The tool should parse without error (workflows field is silently ignored)
	result, err := CreateToolWithWarnings(yamlInput)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// But there should be no workflows parsed (they were ignored)
	if len(result.Tool.Workflows) != 0 {
		t.Errorf("Expected 0 workflows (misplaced ones should be ignored), got %d", len(result.Tool.Workflows))
	}

	// Check for warning about misplaced workflows
	hasWarning := false
	for _, w := range result.Warnings {
		if strings.Contains(w, "workflows") && strings.Contains(strings.ToLower(w), "tool") {
			hasWarning = true
			break
		}
	}
	if !hasWarning {
		t.Logf("Warnings: %v", result.Warnings)
		t.Error("Expected a warning about misplaced workflows inside tool definition")
	}
}

func TestWorkflowValidation_SnakeCase(t *testing.T) {
	testCases := []struct {
		name      string
		category  string
		shouldErr bool
	}{
		{"Valid snake_case", "valid_workflow_name", false},
		{"Valid single word", "workflow", false},
		{"Valid with numbers", "workflow_123", false},
		{"Invalid uppercase", "InvalidWorkflow", true},
		{"Invalid with dash", "invalid-workflow", true},
		{"Invalid with space", "invalid workflow", true},
		{"Invalid starting with number", "123_workflow", true},
		{"Invalid starting with underscore", "_workflow", true},
		{"Invalid camelCase", "workflowName", true},
		{"Invalid PascalCase", "WorkflowName", true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			yamlInput := `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "PublicFunc"
        operation: "format"
        description: "Public function"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "msg"
            description: "Message"
            origin: "inference"
            shouldBeHandledAsMessageToUser: true
            successCriteria: "Generate"
            onError:
              strategy: "inference"
              message: "Failed"
        steps: []

workflows:
  - category: "` + tc.category + `"
    human_category: "Test Workflow"
    description: "Test"
    workflow_type: "user"
    steps:
      - order: 0
        action: "PublicFunc"
        human_action: "Step"
        instructions: "Execute"
`
			_, err := CreateTool(yamlInput)
			if tc.shouldErr && err == nil {
				t.Errorf("Expected error for category '%s', got no error", tc.category)
			}
			if !tc.shouldErr && err != nil {
				t.Errorf("Expected no error for category '%s', got: %v", tc.category, err)
			}
		})
	}
}
