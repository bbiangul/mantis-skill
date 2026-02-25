package skill

import (
	"strings"
	"testing"
)

// TestPolicyOperation_Validation tests the validation rules for policy operations
func TestPolicyOperation_Validation(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid policy operation with output.value",
			yaml: `
version: v1
author: test
tools:
  - name: TestTool
    description: Test tool
    version: "1.0.0"
    functions:
      - name: GetPolicy
        description: Returns a fixed policy value
        operation: policy
        triggers:
          - type: flex_for_user
        output:
          type: string
          value: "This is our refund policy: returns within 30 days."
`,
			wantErr: false,
		},
		{
			name: "valid policy operation with variable substitution from input",
			yaml: `
version: v1
author: test
tools:
  - name: TestTool
    description: Test tool
    version: "1.0.0"
    functions:
      - name: GetGreeting
        description: Returns a greeting with user name
        operation: policy
        triggers:
          - type: flex_for_user
        input:
          - name: userName
            description: User name
            origin: inference
            successCriteria: Get the user name from context
            onError:
              strategy: requestUserInput
              message: Please provide your name
        output:
          type: string
          value: "Hello $userName, welcome!"
`,
			wantErr: false,
		},
		{
			name: "invalid policy operation without output",
			yaml: `
version: v1
author: test
tools:
  - name: TestTool
    description: Test tool
    version: "1.0.0"
    functions:
      - name: GetPolicy
        description: Returns a fixed policy value
        operation: policy
        triggers:
          - type: flex_for_user
`,
			wantErr: true,
			errMsg:  "must have output.value defined",
		},
		{
			name: "invalid policy operation with empty output.value",
			yaml: `
version: v1
author: test
tools:
  - name: TestTool
    description: Test tool
    version: "1.0.0"
    functions:
      - name: GetPolicy
        description: Returns a fixed policy value
        operation: policy
        triggers:
          - type: flex_for_user
        output:
          type: string
          value: ""
`,
			wantErr: true,
			errMsg:  "must have output.value defined",
		},
		{
			name: "invalid policy operation with output but no value field",
			yaml: `
version: v1
author: test
tools:
  - name: TestTool
    description: Test tool
    version: "1.0.0"
    functions:
      - name: GetPolicy
        description: Returns a fixed policy value
        operation: policy
        triggers:
          - type: flex_for_user
        output:
          type: object
          fields:
            - name
`,
			wantErr: true,
			errMsg:  "must have output.value defined",
		},
		{
			name: "policy operation does not require inference input",
			yaml: `
version: v1
author: test
tools:
  - name: TestTool
    description: Test tool
    version: "1.0.0"
    functions:
      - name: GetPolicy
        description: Returns a fixed policy value
        operation: policy
        triggers:
          - type: flex_for_user
        input:
          - name: userId
            description: User ID
            origin: chat
            onError:
              strategy: requestUserInput
              message: Please provide the user ID
        output:
          type: string
          value: "Policy for user $userId"
`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := CreateTool(tt.yaml)
			if tt.wantErr {
				if err == nil {
					t.Errorf("CreateTool() expected error containing %q, got nil", tt.errMsg)
					return
				}
				if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("CreateTool() error = %v, should contain %q", err.Error(), tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("CreateTool() unexpected error = %v", err)
				}
			}
		})
	}
}

// TestPolicyOperation_InValidOperationsList tests that policy is recognized as a valid operation
func TestPolicyOperation_InValidOperationsList(t *testing.T) {
	yaml := `
version: v1
author: test
tools:
  - name: TestTool
    description: Test tool
    version: "1.0.0"
    functions:
      - name: GetPolicy
        description: Returns a fixed policy value
        operation: policy
        triggers:
          - type: flex_for_user
        output:
          type: string
          value: "Fixed value"
`
	tool, err := CreateTool(yaml)
	if err != nil {
		t.Fatalf("CreateTool() unexpected error = %v", err)
	}

	if len(tool.Tools) == 0 || len(tool.Tools[0].Functions) == 0 {
		t.Fatal("Expected tool with at least one function")
	}

	fn := tool.Tools[0].Functions[0]
	if fn.Operation != OperationPolicy {
		t.Errorf("Expected operation %q, got %q", OperationPolicy, fn.Operation)
	}
}
