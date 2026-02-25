package skill

import (
	"strings"
	"testing"
)

// TestValidateStepRunOnlyIf_ValidCases tests valid step-level runOnlyIf configurations
func TestValidateStepRunOnlyIf_ValidCases(t *testing.T) {
	tests := []struct {
		name      string
		yaml      string
		wantErr   bool
		wantWarns bool
	}{
		{
			name: "valid deterministic runOnlyIf in api_call step",
			yaml: `
version: v1
author: "test"
tools:
  - name: "TestTool"
    description: "Test tool"
    version: "1.0.0"
    functions:
      - name: "testFunc"
        operation: "api_call"
        description: "Test function"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "personId"
            description: "Person ID"
            origin: "chat"
            onError:
              strategy: "requestUserInput"
              message: "Please provide person ID"
        steps:
          - name: "step1"
            action: "GET"
            resultIndex: 1
            with:
              url: "https://api.example.com/test"
          - name: "step2"
            action: "POST"
            resultIndex: 2
            runOnlyIf:
              deterministic: "len($personId) == 0"
            with:
              url: "https://api.example.com/create"
        output:
          type: "string"
          value: "result[2]"
`,
			wantErr:   false,
			wantWarns: true, // Should warn about result[2] from step with runOnlyIf
		},
		{
			name: "valid deterministic runOnlyIf in terminal step",
			yaml: `
version: v1
author: "test"
tools:
  - name: "TestTool"
    description: "Test tool"
    version: "1.0.0"
    functions:
      - name: "testFunc"
        operation: "terminal"
        description: "Test function"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "skip"
            description: "Skip flag"
            origin: "chat"
            onError:
              strategy: "requestUserInput"
              message: "Please provide skip flag"
        steps:
          - name: "step1"
            action: "bash"
            resultIndex: 1
            runOnlyIf:
              deterministic: "$skip != 'true'"
            with:
              linux: "echo 'hello'"
              windows: "echo hello"
        output:
          type: "string"
          value: "result[1]"
`,
			wantErr:   false,
			wantWarns: true,
		},
		{
			name: "valid deterministic runOnlyIf in db step",
			yaml: `
version: v1
author: "test"
tools:
  - name: "TestTool"
    description: "Test tool"
    version: "1.0.0"
    functions:
      - name: "testFunc"
        operation: "db"
        description: "Test function"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "userId"
            description: "User ID"
            origin: "chat"
            onError:
              strategy: "requestUserInput"
              message: "Please provide user ID"
        steps:
          - name: "step1"
            action: "select"
            resultIndex: 1
            runOnlyIf:
              deterministic: "len($userId) > 0"
            with:
              select: "SELECT * FROM users WHERE id = $userId"
        output:
          type: "string"
          value: "result[1]"
`,
			wantErr:   false,
			wantWarns: true,
		},
		{
			name: "step without runOnlyIf - backward compatibility",
			yaml: `
version: v1
author: "test"
tools:
  - name: "TestTool"
    description: "Test tool"
    version: "1.0.0"
    functions:
      - name: "testFunc"
        operation: "api_call"
        description: "Test function"
        triggers:
          - type: "flex_for_user"
        steps:
          - name: "step1"
            action: "GET"
            resultIndex: 1
            with:
              url: "https://api.example.com/test"
        output:
          type: "string"
          value: "result[1]"
`,
			wantErr:   false,
			wantWarns: false,
		},
		{
			name: "runOnlyIf with comparison operators",
			yaml: `
version: v1
author: "test"
tools:
  - name: "TestTool"
    description: "Test tool"
    version: "1.0.0"
    functions:
      - name: "testFunc"
        operation: "api_call"
        description: "Test function"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "count"
            description: "Count"
            origin: "chat"
            onError:
              strategy: "requestUserInput"
              message: "Please provide count"
        steps:
          - name: "step1"
            action: "GET"
            resultIndex: 1
            runOnlyIf:
              deterministic: "$count > 5 && $count < 100"
            with:
              url: "https://api.example.com/test"
        output:
          type: "string"
          value: "result[1]"
`,
			wantErr:   false,
			wantWarns: true,
		},
		{
			name: "runOnlyIf with isEmpty function",
			yaml: `
version: v1
author: "test"
tools:
  - name: "TestTool"
    description: "Test tool"
    version: "1.0.0"
    functions:
      - name: "testFunc"
        operation: "api_call"
        description: "Test function"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "data"
            description: "Data"
            origin: "chat"
            onError:
              strategy: "requestUserInput"
              message: "Please provide data"
        steps:
          - name: "step1"
            action: "POST"
            resultIndex: 1
            runOnlyIf:
              deterministic: "isEmpty($data)"
            with:
              url: "https://api.example.com/create"
        output:
          type: "string"
          value: "result[1]"
`,
			wantErr:   false,
			wantWarns: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := CreateToolWithWarnings(tt.yaml)
			if (err != nil) != tt.wantErr {
				t.Errorf("CreateToolWithWarnings() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantWarns && len(result.Warnings) == 0 {
				t.Errorf("CreateToolWithWarnings() expected warnings but got none")
			}
		})
	}
}

// TestValidateStepRunOnlyIf_InvalidCases tests invalid step-level runOnlyIf configurations
func TestValidateStepRunOnlyIf_InvalidCases(t *testing.T) {
	tests := []struct {
		name        string
		yaml        string
		errContains string
	}{
		{
			name: "runOnlyIf with inference condition not allowed",
			yaml: `
version: v1
author: "test"
tools:
  - name: "TestTool"
    description: "Test tool"
    version: "1.0.0"
    functions:
      - name: "testFunc"
        operation: "api_call"
        description: "Test function"
        triggers:
          - type: "flex_for_user"
        steps:
          - name: "step1"
            action: "GET"
            runOnlyIf:
              condition: "should this run?"
            with:
              url: "https://api.example.com/test"
`,
			errContains: "only supports 'deterministic' mode",
		},
		{
			name: "runOnlyIf with inference object not allowed",
			yaml: `
version: v1
author: "test"
tools:
  - name: "TestTool"
    description: "Test tool"
    version: "1.0.0"
    functions:
      - name: "testFunc"
        operation: "api_call"
        description: "Test function"
        triggers:
          - type: "flex_for_user"
        steps:
          - name: "step1"
            action: "GET"
            runOnlyIf:
              inference:
                condition: "should this run?"
            with:
              url: "https://api.example.com/test"
`,
			errContains: "only supports 'deterministic' mode",
		},
		{
			name: "runOnlyIf not allowed in web_browse operation",
			yaml: `
version: v1
author: "test"
tools:
  - name: "TestTool"
    description: "Test tool"
    version: "1.0.0"
    functions:
      - name: "testFunc"
        operation: "web_browse"
        description: "Test function"
        triggers:
          - type: "flex_for_user"
        successCriteria: "Find the information"
        steps:
          - name: "step1"
            action: "open_url"
            runOnlyIf:
              deterministic: "true == true"
            with:
              url: "https://example.com"
`,
			errContains: "not supported",
		},
		{
			name: "runOnlyIf without deterministic field",
			yaml: `
version: v1
author: "test"
tools:
  - name: "TestTool"
    description: "Test tool"
    version: "1.0.0"
    functions:
      - name: "testFunc"
        operation: "api_call"
        description: "Test function"
        triggers:
          - type: "flex_for_user"
        steps:
          - name: "step1"
            action: "GET"
            runOnlyIf:
              combineWith: "AND"
            with:
              url: "https://api.example.com/test"
`,
			errContains: "must specify at least one of",
		},
		{
			name: "runOnlyIf with invalid expression - no operator",
			yaml: `
version: v1
author: "test"
tools:
  - name: "TestTool"
    description: "Test tool"
    version: "1.0.0"
    functions:
      - name: "testFunc"
        operation: "api_call"
        description: "Test function"
        triggers:
          - type: "flex_for_user"
        steps:
          - name: "step1"
            action: "GET"
            runOnlyIf:
              deterministic: "$personId"
            with:
              url: "https://api.example.com/test"
`,
			errContains: "must contain a comparison operator",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := CreateTool(tt.yaml)
			if err == nil {
				t.Errorf("CreateTool() expected error but got none")
				return
			}
			if !strings.Contains(err.Error(), tt.errContains) {
				t.Errorf("CreateTool() error = %v, expected to contain %q", err, tt.errContains)
			}
		})
	}
}

// TestCheckOutputReferencesToRunOnlyIfSteps tests warning generation for output references
func TestCheckOutputReferencesToRunOnlyIfSteps(t *testing.T) {
	tests := []struct {
		name         string
		yaml         string
		wantWarning  bool
		warnContains string
	}{
		{
			name: "output references step with runOnlyIf - should warn",
			yaml: `
version: v1
author: "test"
tools:
  - name: "TestTool"
    description: "Test tool"
    version: "1.0.0"
    functions:
      - name: "testFunc"
        operation: "api_call"
        description: "Test function"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "skip"
            description: "Skip"
            origin: "chat"
            onError:
              strategy: "requestUserInput"
              message: "Please provide skip value"
        steps:
          - name: "step1"
            action: "GET"
            resultIndex: 1
            runOnlyIf:
              deterministic: "$skip == 'true'"
            with:
              url: "https://api.example.com/test"
        output:
          type: "string"
          value: "result[1].data"
`,
			wantWarning:  true,
			warnContains: "coalesce",
		},
		{
			name: "output references step without runOnlyIf - no warning",
			yaml: `
version: v1
author: "test"
tools:
  - name: "TestTool"
    description: "Test tool"
    version: "1.0.0"
    functions:
      - name: "testFunc"
        operation: "api_call"
        description: "Test function"
        triggers:
          - type: "flex_for_user"
        steps:
          - name: "step1"
            action: "GET"
            resultIndex: 1
            with:
              url: "https://api.example.com/test"
        output:
          type: "string"
          value: "result[1].data"
`,
			wantWarning: false,
		},
		{
			name: "output references different step than one with runOnlyIf - no warning",
			yaml: `
version: v1
author: "test"
tools:
  - name: "TestTool"
    description: "Test tool"
    version: "1.0.0"
    functions:
      - name: "testFunc"
        operation: "api_call"
        description: "Test function"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "skip"
            description: "Skip"
            origin: "chat"
            onError:
              strategy: "requestUserInput"
              message: "Please provide skip value"
        steps:
          - name: "step1"
            action: "GET"
            resultIndex: 1
            with:
              url: "https://api.example.com/first"
          - name: "step2"
            action: "POST"
            resultIndex: 2
            runOnlyIf:
              deterministic: "$skip == 'true'"
            with:
              url: "https://api.example.com/second"
        output:
          type: "string"
          value: "result[1].data"
`,
			wantWarning: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := CreateToolWithWarnings(tt.yaml)
			if err != nil {
				t.Errorf("CreateToolWithWarnings() unexpected error = %v", err)
				return
			}

			hasWarning := len(result.Warnings) > 0
			if hasWarning != tt.wantWarning {
				t.Errorf("CreateToolWithWarnings() hasWarning = %v, wantWarning %v, warnings: %v",
					hasWarning, tt.wantWarning, result.Warnings)
				return
			}

			if tt.wantWarning && tt.warnContains != "" {
				found := false
				for _, w := range result.Warnings {
					if strings.Contains(w, tt.warnContains) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("CreateToolWithWarnings() warnings = %v, expected to contain %q",
						result.Warnings, tt.warnContains)
				}
			}
		})
	}
}
