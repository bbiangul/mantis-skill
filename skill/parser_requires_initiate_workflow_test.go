package skill

import (
	"strings"
	"testing"
)

func TestRequiresInitiateWorkflow_ValidPublicFunctionReferenced(t *testing.T) {
	// Test: public function with requiresInitiateWorkflow that IS referenced by initiate_workflow
	// Expected: no error
	yamlDef := `
version: v1
author: test

tools:
  - name: TestTool
    description: Test tool for requiresInitiateWorkflow
    version: 1.0.0
    functions:
      - name: processFollowUps
        operation: initiate_workflow
        description: Process scheduled follow-ups
        triggers:
          - type: time_based
            cron: "0 0 10 * * *"
        steps:
          - name: sendFollowUps
            action: start_workflow
            with:
              userId: "user123"
              workflowType: user
              message: "Hello, follow-up time!"
              context:
                value: "Follow-up context"
                params:
                  - function: "TestTool.GenerateFollowUp"
                    inputs:
                      companyName: "Test Company"
                      dealId: "123"

      - name: GenerateFollowUp
        operation: api_call
        description: Generate follow-up message
        requiresInitiateWorkflow: true
        triggers:
          - type: flex_for_user
        input:
          - name: companyName
            description: Company name
            value: "$companyName"
          - name: dealId
            description: Deal ID
            value: "$dealId"
        steps:
          - name: generate
            action: GET
            with:
              url: "https://api.example.com/generate?company=$companyName"
            resultIndex: 1
        output:
          type: string
          value: "done"
`

	result, err := CreateToolWithWarnings(yamlDef)
	if err != nil {
		t.Fatalf("CreateToolWithWarnings() error = %v", err)
	}

	// Find the GenerateFollowUp function
	var fn *Function
	for i := range result.Tool.Tools[0].Functions {
		if result.Tool.Tools[0].Functions[i].Name == "GenerateFollowUp" {
			fn = &result.Tool.Tools[0].Functions[i]
			break
		}
	}

	if fn == nil {
		t.Fatal("GenerateFollowUp function not found")
	}

	if !fn.RequiresInitiateWorkflow {
		t.Error("Expected RequiresInitiateWorkflow to be true")
	}
}

func TestRequiresInitiateWorkflow_PrivateFunctionFails(t *testing.T) {
	// Test: private function (lowercase) with requiresInitiateWorkflow
	// Expected: error "can only be used on public functions"
	yamlDef := `
version: v1
author: test

tools:
  - name: TestTool
    description: Test tool for requiresInitiateWorkflow
    version: 1.0.0
    functions:
      - name: processFollowUps
        operation: initiate_workflow
        description: Process scheduled follow-ups
        triggers:
          - type: time_based
            cron: "0 0 10 * * *"
        steps:
          - name: sendFollowUps
            action: start_workflow
            with:
              userId: "user123"
              workflowType: user
              message: "Hello, follow-up time!"
              context:
                value: "Follow-up context"
                params:
                  - function: "TestTool.generateFollowUp"
                    inputs:
                      companyName: "Test Company"

      - name: generateFollowUp
        operation: api_call
        description: Generate follow-up message - private function
        requiresInitiateWorkflow: true
        triggers:
          - type: flex_for_user
        input:
          - name: companyName
            description: Company name
            value: "$companyName"
        steps:
          - name: generate
            action: GET
            with:
              url: "https://api.example.com/generate?company=$companyName"
            resultIndex: 1
        output:
          type: string
          value: "done"
`

	_, err := CreateToolWithWarnings(yamlDef)
	if err == nil {
		t.Fatal("Expected error for private function with requiresInitiateWorkflow, got none")
	}

	if !strings.Contains(err.Error(), "private") && !strings.Contains(err.Error(), "lowercase") {
		t.Errorf("Expected error about private function, got: %v", err)
	}
}

func TestRequiresInitiateWorkflow_NotReferencedFails(t *testing.T) {
	// Test: public function with requiresInitiateWorkflow NOT referenced by any initiate_workflow
	// Expected: error "not referenced by any initiate_workflow function"
	yamlDef := `
version: v1
author: test

tools:
  - name: TestTool
    description: Test tool for requiresInitiateWorkflow
    version: 1.0.0
    functions:
      - name: ProcessData
        operation: api_call
        description: Some other function
        triggers:
          - type: flex_for_user
        input:
          - name: data
            description: Data
            origin: chat
            onError:
              strategy: requestUserInput
              message: Please provide data
        steps:
          - name: call
            action: GET
            with:
              url: "https://example.com/api"
            resultIndex: 1
        output:
          type: string
          value: "done"

      - name: GenerateFollowUp
        operation: api_call
        description: Generate follow-up message - NOT referenced by initiate_workflow
        requiresInitiateWorkflow: true
        triggers:
          - type: flex_for_user
        input:
          - name: companyName
            description: Company name
            origin: chat
            onError:
              strategy: requestUserInput
              message: Please provide company name
        steps:
          - name: generate
            action: GET
            with:
              url: "https://api.example.com/generate?company=$companyName"
            resultIndex: 1
        output:
          type: string
          value: "done"
`

	_, err := CreateToolWithWarnings(yamlDef)
	if err == nil {
		t.Fatal("Expected error for function not referenced by initiate_workflow, got none")
	}

	if !strings.Contains(err.Error(), "not referenced by any initiate_workflow") {
		t.Errorf("Expected error about not being referenced, got: %v", err)
	}
}

func TestRequiresInitiateWorkflow_ReferencedWithDotNotation(t *testing.T) {
	// Test: function referenced as "ToolName.FunctionName" in context.params
	// Expected: no error
	yamlDef := `
version: v1
author: test

tools:
  - name: TestTool
    description: Test tool for requiresInitiateWorkflow
    version: 1.0.0
    functions:
      - name: processFollowUps
        operation: initiate_workflow
        description: Process scheduled follow-ups
        triggers:
          - type: time_based
            cron: "0 0 10 * * *"
        steps:
          - name: sendFollowUps
            action: start_workflow
            with:
              userId: "user123"
              workflowType: user
              message: "Hello, follow-up time!"
              context:
                value: "Follow-up context"
                params:
                  - function: "TestTool.GenerateFollowUp"
                    inputs:
                      companyName: "Test Company"

      - name: GenerateFollowUp
        operation: api_call
        description: Generate follow-up message
        requiresInitiateWorkflow: true
        triggers:
          - type: flex_for_user
        input:
          - name: companyName
            description: Company name
            value: "$companyName"
        steps:
          - name: generate
            action: GET
            with:
              url: "https://api.example.com/generate?company=$companyName"
            resultIndex: 1
        output:
          type: string
          value: "done"
`

	_, err := CreateToolWithWarnings(yamlDef)
	if err != nil {
		t.Fatalf("CreateToolWithWarnings() error = %v", err)
	}
}

func TestRequiresInitiateWorkflow_ReferencedWithoutDotNotation(t *testing.T) {
	// Test: function referenced as just "FunctionName" in context.params (local reference)
	// Expected: no error
	yamlDef := `
version: v1
author: test

tools:
  - name: TestTool
    description: Test tool for requiresInitiateWorkflow
    version: 1.0.0
    functions:
      - name: processFollowUps
        operation: initiate_workflow
        description: Process scheduled follow-ups
        triggers:
          - type: time_based
            cron: "0 0 10 * * *"
        steps:
          - name: sendFollowUps
            action: start_workflow
            with:
              userId: "user123"
              workflowType: user
              message: "Hello, follow-up time!"
              context:
                value: "Follow-up context"
                params:
                  - function: "GenerateFollowUp"
                    inputs:
                      companyName: "Test Company"

      - name: GenerateFollowUp
        operation: api_call
        description: Generate follow-up message
        requiresInitiateWorkflow: true
        triggers:
          - type: flex_for_user
        input:
          - name: companyName
            description: Company name
            value: "$companyName"
        steps:
          - name: generate
            action: GET
            with:
              url: "https://api.example.com/generate?company=$companyName"
            resultIndex: 1
        output:
          type: string
          value: "done"
`

	_, err := CreateToolWithWarnings(yamlDef)
	if err != nil {
		t.Fatalf("CreateToolWithWarnings() error = %v", err)
	}
}

func TestRequiresInitiateWorkflow_DefaultFalse(t *testing.T) {
	// Test that requiresInitiateWorkflow defaults to false
	yamlDef := `
version: v1
author: test

tools:
  - name: TestTool
    description: Test tool without requiresInitiateWorkflow
    version: 1.0.0
    functions:
      - name: SomeFunction
        operation: api_call
        description: A function without requiresInitiateWorkflow
        triggers:
          - type: flex_for_user
        input:
          - name: data
            description: Data
            origin: chat
            onError:
              strategy: requestUserInput
              message: Please provide data
        steps:
          - name: call
            action: GET
            with:
              url: "https://example.com/api"
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
	if fn.RequiresInitiateWorkflow {
		t.Error("Expected RequiresInitiateWorkflow to default to false")
	}
}

func TestRequiresInitiateWorkflow_MultipleReferencingWorkflows(t *testing.T) {
	// Test: function referenced by multiple initiate_workflow functions
	// Expected: no error
	yamlDef := `
version: v1
author: test

tools:
  - name: TestTool
    description: Test tool for requiresInitiateWorkflow
    version: 1.0.0
    functions:
      - name: processFollowUps
        operation: initiate_workflow
        description: Process scheduled follow-ups
        triggers:
          - type: time_based
            cron: "0 0 10 * * *"
        steps:
          - name: sendFollowUps
            action: start_workflow
            with:
              userId: "user123"
              workflowType: user
              message: "Hello, follow-up time!"
              context:
                value: "Follow-up context"
                params:
                  - function: "TestTool.GenerateFollowUp"
                    inputs:
                      companyName: "Test Company"

      - name: processUrgentFollowUps
        operation: initiate_workflow
        description: Process urgent follow-ups
        triggers:
          - type: time_based
            cron: "0 0 12 * * *"
        steps:
          - name: sendUrgentFollowUps
            action: start_workflow
            with:
              userId: "user123"
              workflowType: user
              message: "Hello, follow-up time!"
              context:
                value: "Follow-up context"
                params:
                  - function: "TestTool.GenerateFollowUp"
                    inputs:
                      companyName: "Urgent Company"

      - name: GenerateFollowUp
        operation: api_call
        description: Generate follow-up message
        requiresInitiateWorkflow: true
        triggers:
          - type: flex_for_user
        input:
          - name: companyName
            description: Company name
            value: "$companyName"
        steps:
          - name: generate
            action: GET
            with:
              url: "https://api.example.com/generate?company=$companyName"
            resultIndex: 1
        output:
          type: string
          value: "done"
`

	_, err := CreateToolWithWarnings(yamlDef)
	if err != nil {
		t.Fatalf("CreateToolWithWarnings() error = %v", err)
	}
}

func TestExtractContextParamsFunctionRefs(t *testing.T) {
	// Test the extractContextParamsFunctionRefs helper function
	tests := []struct {
		name       string
		contextRaw interface{}
		toolName   string
		expected   []string
	}{
		{
			name:       "nil context",
			contextRaw: nil,
			toolName:   "TestTool",
			expected:   nil,
		},
		{
			name:       "string context",
			contextRaw: "some string context",
			toolName:   "TestTool",
			expected:   nil,
		},
		{
			name: "context with params - dot notation",
			contextRaw: map[string]interface{}{
				"params": []interface{}{
					map[string]interface{}{
						"function": "TestTool.MyFunction",
						"inputs":   map[string]interface{}{},
					},
				},
			},
			toolName: "TestTool",
			expected: []string{"MyFunction"},
		},
		{
			name: "context with params - local reference",
			contextRaw: map[string]interface{}{
				"params": []interface{}{
					map[string]interface{}{
						"function": "MyFunction",
						"inputs":   map[string]interface{}{},
					},
				},
			},
			toolName: "TestTool",
			expected: []string{"MyFunction"},
		},
		{
			name: "context with params - different tool (should be excluded)",
			contextRaw: map[string]interface{}{
				"params": []interface{}{
					map[string]interface{}{
						"function": "OtherTool.MyFunction",
						"inputs":   map[string]interface{}{},
					},
				},
			},
			toolName: "TestTool",
			expected: nil,
		},
		{
			name: "context with multiple params",
			contextRaw: map[string]interface{}{
				"params": []interface{}{
					map[string]interface{}{
						"function": "TestTool.Function1",
						"inputs":   map[string]interface{}{},
					},
					map[string]interface{}{
						"function": "Function2",
						"inputs":   map[string]interface{}{},
					},
				},
			},
			toolName: "TestTool",
			expected: []string{"Function1", "Function2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractContextParamsFunctionRefs(tt.contextRaw, tt.toolName)

			if len(result) != len(tt.expected) {
				t.Errorf("extractContextParamsFunctionRefs() = %v, want %v", result, tt.expected)
				return
			}

			for i, ref := range result {
				if ref != tt.expected[i] {
					t.Errorf("extractContextParamsFunctionRefs()[%d] = %v, want %v", i, ref, tt.expected[i])
				}
			}
		})
	}
}
