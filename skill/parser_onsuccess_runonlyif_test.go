package skill

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOnSuccessRunOnlyIfValidation(t *testing.T) {
	testCases := []struct {
		name          string
		yamlData      string
		wantErr       bool
		errorContains string
	}{
		{
			name: "Valid - onSuccess with deterministic runOnlyIf referencing needs",
			yamlData: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "Tool for testing"
    version: "1.0.0"
    functions:
      - name: "getDealInfo"
        operation: "api_call"
        description: "Get deal information"
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "fetch"
            action: "GET"
            with:
              url: "https://api.example.com/deal"

      - name: "processDeal"
        operation: "api_call"
        description: "Process deal"
        needs: ["getDealInfo"]
        onSuccess:
          - name: "callback"
            runOnlyIf:
              deterministic: "$getDealInfo.stage == 'D'"
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "process"
            action: "POST"
            with:
              url: "https://api.example.com/process"

      - name: "callback"
        operation: "api_call"
        description: "Callback"
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "call"
            action: "POST"
            with:
              url: "https://api.example.com/callback"
`,
			wantErr: false,
		},
		{
			name: "Valid - onSuccess runOnlyIf referencing parent input",
			yamlData: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "Tool for testing"
    version: "1.0.0"
    functions:
      - name: "mainFunc"
        operation: "api_call"
        description: "Main function"
        onSuccess:
          - name: "callback"
            runOnlyIf:
              deterministic: "$amount > 1000"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "amount"
            description: "Amount"
            origin: "chat"
            onError:
              strategy: "requestUserInput"
              message: "Provide amount"
        steps:
          - name: "step1"
            action: "GET"
            with:
              url: "https://api.example.com"

      - name: "callback"
        operation: "api_call"
        description: "Callback"
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "step1"
            action: "GET"
            with:
              url: "https://api.example.com/callback"
`,
			wantErr: false,
		},
		{
			name: "Valid - onSuccess runOnlyIf referencing result",
			yamlData: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "Tool for testing"
    version: "1.0.0"
    functions:
      - name: "mainFunc"
        operation: "api_call"
        description: "Main function"
        onSuccess:
          - name: "callback"
            runOnlyIf:
              deterministic: "$result.success == true"
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "step1"
            action: "GET"
            with:
              url: "https://api.example.com"

      - name: "callback"
        operation: "api_call"
        description: "Callback"
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "step1"
            action: "GET"
            with:
              url: "https://api.example.com/callback"
`,
			wantErr: false,
		},
		{
			name: "Valid - onFailure with deterministic runOnlyIf",
			yamlData: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "Tool for testing"
    version: "1.0.0"
    functions:
      - name: "mainFunc"
        operation: "api_call"
        description: "Main function"
        onFailure:
          - name: "fallback"
            runOnlyIf:
              deterministic: "$errorCode == 500"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "errorCode"
            description: "Error code"
            origin: "chat"
            onError:
              strategy: "requestUserInput"
              message: "Provide error code"
        steps:
          - name: "step1"
            action: "GET"
            with:
              url: "https://api.example.com"

      - name: "fallback"
        operation: "api_call"
        description: "Fallback"
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "step1"
            action: "GET"
            with:
              url: "https://api.example.com/fallback"
`,
			wantErr: false,
		},
		{
			name: "Valid - onSuccess without runOnlyIf (backward compat simple string)",
			yamlData: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "Tool for testing"
    version: "1.0.0"
    functions:
      - name: "mainFunc"
        operation: "api_call"
        description: "Main function"
        onSuccess:
          - "callback"
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "step1"
            action: "GET"
            with:
              url: "https://api.example.com"

      - name: "callback"
        operation: "api_call"
        description: "Callback"
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "step1"
            action: "GET"
            with:
              url: "https://api.example.com/callback"
`,
			wantErr: false,
		},
		{
			name: "Invalid - onSuccess runOnlyIf with condition (inference)",
			yamlData: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "Tool for testing"
    version: "1.0.0"
    functions:
      - name: "mainFunc"
        operation: "api_call"
        description: "Main function"
        onSuccess:
          - name: "callback"
            runOnlyIf:
              condition: "the user wants to proceed"
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "step1"
            action: "GET"
            with:
              url: "https://api.example.com"

      - name: "callback"
        operation: "api_call"
        description: "Callback"
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "step1"
            action: "GET"
            with:
              url: "https://api.example.com/callback"
`,
			wantErr:       true,
			errorContains: "deterministic",
		},
		{
			name: "Invalid - onSuccess runOnlyIf with inference block",
			yamlData: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "Tool for testing"
    version: "1.0.0"
    functions:
      - name: "mainFunc"
        operation: "api_call"
        description: "Main function"
        onSuccess:
          - name: "callback"
            runOnlyIf:
              inference:
                condition: "check if retry needed"
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "step1"
            action: "GET"
            with:
              url: "https://api.example.com"

      - name: "callback"
        operation: "api_call"
        description: "Callback"
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "step1"
            action: "GET"
            with:
              url: "https://api.example.com/callback"
`,
			wantErr:       true,
			errorContains: "deterministic",
		},
		{
			name: "Invalid - onSuccess runOnlyIf references unavailable function",
			yamlData: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "Tool for testing"
    version: "1.0.0"
    functions:
      - name: "mainFunc"
        operation: "api_call"
        description: "Main function"
        onSuccess:
          - name: "callback"
            runOnlyIf:
              deterministic: "$unknownFunc.result == 'test'"
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "step1"
            action: "GET"
            with:
              url: "https://api.example.com"

      - name: "callback"
        operation: "api_call"
        description: "Callback"
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "step1"
            action: "GET"
            with:
              url: "https://api.example.com/callback"
`,
			wantErr:       true,
			errorContains: "not available",
		},
		{
			name: "Invalid - onFailure runOnlyIf with inference",
			yamlData: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "Tool for testing"
    version: "1.0.0"
    functions:
      - name: "mainFunc"
        operation: "api_call"
        description: "Main function"
        onFailure:
          - name: "fallback"
            runOnlyIf:
              inference:
                condition: "check if retry is needed"
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "step1"
            action: "GET"
            with:
              url: "https://api.example.com"

      - name: "fallback"
        operation: "api_call"
        description: "Fallback"
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "step1"
            action: "GET"
            with:
              url: "https://api.example.com/fallback"
`,
			wantErr:       true,
			errorContains: "deterministic",
		},
		{
			name: "Invalid - onSuccess runOnlyIf with unsupported function (upper)",
			yamlData: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "Tool for testing"
    version: "1.0.0"
    functions:
      - name: "mainFunc"
        operation: "api_call"
        description: "Main function"
        onSuccess:
          - name: "callback"
            runOnlyIf:
              deterministic: "upper($status) == 'ACTIVE'"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "status"
            description: "Status"
            origin: "chat"
            onError:
              strategy: "requestUserInput"
              message: "Provide status"
        steps:
          - name: "step1"
            action: "GET"
            with:
              url: "https://api.example.com"

      - name: "callback"
        operation: "api_call"
        description: "Callback"
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "step1"
            action: "GET"
            with:
              url: "https://api.example.com/callback"
`,
			wantErr:       true,
			errorContains: "unsupported function",
		},
		{
			name: "Invalid - onSuccess runOnlyIf with unsupported function (startsWith)",
			yamlData: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "Tool for testing"
    version: "1.0.0"
    functions:
      - name: "mainFunc"
        operation: "api_call"
        description: "Main function"
        onSuccess:
          - name: "callback"
            runOnlyIf:
              deterministic: "startsWith($name, 'John') == true"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "name"
            description: "Name"
            origin: "chat"
            onError:
              strategy: "requestUserInput"
              message: "Provide name"
        steps:
          - name: "step1"
            action: "GET"
            with:
              url: "https://api.example.com"

      - name: "callback"
        operation: "api_call"
        description: "Callback"
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "step1"
            action: "GET"
            with:
              url: "https://api.example.com/callback"
`,
			wantErr:       true,
			errorContains: "unsupported function",
		},
		{
			name: "Valid - onSuccess runOnlyIf with supported function (len)",
			yamlData: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "Tool for testing"
    version: "1.0.0"
    functions:
      - name: "getWorkflows"
        operation: "api_call"
        description: "Get workflows"
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "fetch"
            action: "GET"
            with:
              url: "https://api.example.com/workflows"

      - name: "processWorkflows"
        operation: "api_call"
        description: "Process workflows"
        needs: ["getWorkflows"]
        onSuccess:
          - name: "callback"
            runOnlyIf:
              deterministic: "len($getWorkflows) > 0"
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "process"
            action: "POST"
            with:
              url: "https://api.example.com/process"

      - name: "callback"
        operation: "api_call"
        description: "Callback"
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "step1"
            action: "GET"
            with:
              url: "https://api.example.com/callback"
`,
			wantErr: false,
		},
		{
			name: "Valid - onSuccess runOnlyIf with supported function (contains)",
			yamlData: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "Tool for testing"
    version: "1.0.0"
    functions:
      - name: "mainFunc"
        operation: "api_call"
        description: "Main function"
        onSuccess:
          - name: "callback"
            runOnlyIf:
              deterministic: "contains($status, 'active') == true"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "status"
            description: "Status"
            origin: "chat"
            onError:
              strategy: "requestUserInput"
              message: "Provide status"
        steps:
          - name: "step1"
            action: "GET"
            with:
              url: "https://api.example.com"

      - name: "callback"
        operation: "api_call"
        description: "Callback"
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "step1"
            action: "GET"
            with:
              url: "https://api.example.com/callback"
`,
			wantErr: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := CreateTool(tc.yamlData)

			if tc.wantErr {
				assert.Error(t, err)
				if tc.errorContains != "" {
					assert.True(t, strings.Contains(err.Error(), tc.errorContains),
						"Expected error to contain %q, got: %s", tc.errorContains, err.Error())
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestFunctionCallRunOnlyIfParsing(t *testing.T) {
	// Test that FunctionCall correctly parses runOnlyIf field
	yamlData := `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "Tool for testing"
    version: "1.0.0"
    functions:
      - name: "mainFunc"
        operation: "api_call"
        description: "Main function"
        onSuccess:
          - name: "callback"
            runOnlyIf:
              deterministic: "$amount > 100"
            params:
              dealId: "$dealId"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "amount"
            description: "Amount"
            origin: "chat"
            onError:
              strategy: "requestUserInput"
              message: "Amount"
          - name: "dealId"
            description: "Deal ID"
            origin: "chat"
            onError:
              strategy: "requestUserInput"
              message: "Deal ID"
        steps:
          - name: "step1"
            action: "GET"
            with:
              url: "https://api.example.com"

      - name: "callback"
        operation: "api_call"
        description: "Callback"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "dealId"
            description: "Deal ID"
            origin: "chat"
            onError:
              strategy: "requestUserInput"
              message: "Deal ID"
        steps:
          - name: "step1"
            action: "GET"
            with:
              url: "https://api.example.com/callback"
`

	customTool, err := CreateTool(yamlData)
	assert.NoError(t, err)
	assert.Len(t, customTool.Tools, 1)

	tool := customTool.Tools[0]
	mainFunc := findFunctionByName("mainFunc", tool.Functions)
	assert.NotNil(t, mainFunc)
	assert.Len(t, mainFunc.OnSuccess, 1)

	successCall := mainFunc.OnSuccess[0]
	assert.Equal(t, "callback", successCall.Name)
	assert.NotNil(t, successCall.RunOnlyIf)
	assert.NotNil(t, successCall.Params)
	assert.Equal(t, "$dealId", successCall.Params["dealId"])
}
