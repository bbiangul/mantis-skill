package skill

import (
	"strings"
	"testing"
)

// Test Group 1: checkPublicFunctionsNeedsParamsFromInputs
// Tests that public functions cannot have needs params that reference their own input variables
func TestPublicFunctionNeedsParamsFromInputs(t *testing.T) {
	testCases := []struct {
		name          string
		yamlInput     string
		expectError   bool
		errorContains string
	}{
		// Case 1: Error - Public function $var in needs params
		{
			name: "Error: Public function $var in needs params",
			yamlInput: `
version: "v1"
author: "Test"
tools:
  - name: "TestTool"
    description: "Test tool"
    version: "1.0.0"
    functions:
      - name: "GetDealInfo"
        operation: "terminal"
        description: "Get deal info"
        triggers:
          - type: "flex_for_team"
        needs:
          - name: "getDealDetails"
            params:
              dealId: "$dealId"
        input:
          - name: "dealId"
            description: "Deal ID"
            value: "12345"
        steps:
          - name: "process"
            action: "bash"
            with:
              linux: "echo ok"
              windows: "echo ok"
        output:
          type: "string"
          value: "result[1]"
      - name: "getDealDetails"
        operation: "terminal"
        description: "Get deal details"
        input:
          - name: "dealId"
            description: "Deal ID"
            value: "default-id"
        steps:
          - name: "fetch"
            action: "bash"
            with:
              linux: "echo $dealId"
              windows: "echo %dealId%"
        output:
          type: "string"
          value: "result[1]"
`,
			expectError:   true,
			errorContains: "public function 'GetDealInfo' has needs 'getDealDetails' with param 'dealId' referencing input variable",
		},

		// Case 2: Error - Public function ${var} in needs params
		{
			name: "Error: Public function ${var} in needs params",
			yamlInput: `
version: "v1"
author: "Test"
tools:
  - name: "TestTool"
    description: "Test tool"
    version: "1.0.0"
    functions:
      - name: "GetDealInfo"
        operation: "terminal"
        description: "Get deal info"
        triggers:
          - type: "flex_for_team"
        needs:
          - name: "getDealDetails"
            params:
              dealId: "${dealId}"
        input:
          - name: "dealId"
            description: "Deal ID"
            value: "12345"
        steps:
          - name: "process"
            action: "bash"
            with:
              linux: "echo ok"
              windows: "echo ok"
        output:
          type: "string"
          value: "result[1]"
      - name: "getDealDetails"
        operation: "terminal"
        description: "Get deal details"
        input:
          - name: "dealId"
            description: "Deal ID"
            value: "default-id"
        steps:
          - name: "fetch"
            action: "bash"
            with:
              linux: "echo $dealId"
              windows: "echo %dealId%"
        output:
          type: "string"
          value: "result[1]"
`,
			expectError:   true,
			errorContains: "public function 'GetDealInfo' has needs 'getDealDetails' with param 'dealId' referencing input variable",
		},

		// Case 3: Error - Public function $var.field in needs params
		{
			name: "Error: Public function $var.field in needs params",
			yamlInput: `
version: "v1"
author: "Test"
tools:
  - name: "TestTool"
    description: "Test tool"
    version: "1.0.0"
    functions:
      - name: "GetDealInfo"
        operation: "terminal"
        description: "Get deal info"
        triggers:
          - type: "flex_for_team"
        needs:
          - name: "getDealDetails"
            params:
              dealId: "$deal.id"
        input:
          - name: "deal"
            description: "Deal object"
            value: "{}"
        steps:
          - name: "process"
            action: "bash"
            with:
              linux: "echo ok"
              windows: "echo ok"
        output:
          type: "string"
          value: "result[1]"
      - name: "getDealDetails"
        operation: "terminal"
        description: "Get deal details"
        input:
          - name: "dealId"
            description: "Deal ID"
            value: "default-id"
        steps:
          - name: "fetch"
            action: "bash"
            with:
              linux: "echo $dealId"
              windows: "echo %dealId%"
        output:
          type: "string"
          value: "result[1]"
`,
			expectError:   true,
			errorContains: "public function 'GetDealInfo' has needs 'getDealDetails' with param 'dealId' referencing input variable",
		},

		// Case 4: Pass - Private function $var in needs params (called via onSuccess from public function)
		{
			name: "Pass: Private function $var in needs params",
			yamlInput: `
version: "v1"
author: "Test"
tools:
  - name: "TestTool"
    description: "Test tool"
    version: "1.0.0"
    functions:
      - name: "PublicEntry"
        operation: "terminal"
        description: "Public entry point"
        triggers:
          - type: "flex_for_team"
        input:
          - name: "dealId"
            description: "Deal ID"
            value: "12345"
        onSuccess:
          - name: "getDealInfo"
            params:
              dealId: "$dealId"
        steps:
          - name: "entry"
            action: "bash"
            with:
              linux: "echo entry"
              windows: "echo entry"
        output:
          type: "string"
          value: "result[1]"
      - name: "getDealInfo"
        operation: "terminal"
        description: "Get deal info"
        needs:
          - name: "getDealDetails"
            params:
              dealId: "$dealId"
        input:
          - name: "dealId"
            description: "Deal ID"
            value: "default-id"
        steps:
          - name: "process"
            action: "bash"
            with:
              linux: "echo ok"
              windows: "echo ok"
        output:
          type: "string"
          value: "result[1]"
      - name: "getDealDetails"
        operation: "terminal"
        description: "Get deal details"
        input:
          - name: "dealId"
            description: "Deal ID"
            value: "default-id"
        steps:
          - name: "fetch"
            action: "bash"
            with:
              linux: "echo $dealId"
              windows: "echo %dealId%"
        output:
          type: "string"
          value: "result[1]"
`,
			expectError: false,
		},

		// Case 5: Pass - Public function with ENV var in needs params
		{
			name: "Pass: Public function with ENV var in needs params",
			yamlInput: `
version: "v1"
author: "Test"
env:
  - name: "API_KEY"
    value: "secret123"
tools:
  - name: "TestTool"
    description: "Test tool"
    version: "1.0.0"
    functions:
      - name: "GetData"
        operation: "terminal"
        description: "Get data"
        triggers:
          - type: "flex_for_team"
        needs:
          - name: "fetchData"
            params:
              apiKey: "$API_KEY"
        steps:
          - name: "process"
            action: "bash"
            with:
              linux: "echo ok"
              windows: "echo ok"
        output:
          type: "string"
          value: "result[1]"
      - name: "fetchData"
        operation: "terminal"
        description: "Fetch data"
        input:
          - name: "apiKey"
            description: "API Key"
            value: "default-key"
        steps:
          - name: "fetch"
            action: "bash"
            with:
              linux: "echo $apiKey"
              windows: "echo %apiKey%"
        output:
          type: "string"
          value: "result[1]"
`,
			expectError: false,
		},

		// Case 6: Pass - Public function with system var $NOW in needs params
		{
			name: "Pass: Public function with system var $NOW in needs params",
			yamlInput: `
version: "v1"
author: "Test"
tools:
  - name: "TestTool"
    description: "Test tool"
    version: "1.0.0"
    functions:
      - name: "GetData"
        operation: "terminal"
        description: "Get data"
        triggers:
          - type: "flex_for_team"
        needs:
          - name: "fetchData"
            params:
              timestamp: "$NOW"
        steps:
          - name: "process"
            action: "bash"
            with:
              linux: "echo ok"
              windows: "echo ok"
        output:
          type: "string"
          value: "result[1]"
      - name: "fetchData"
        operation: "terminal"
        description: "Fetch data"
        input:
          - name: "timestamp"
            description: "Timestamp"
            value: "2024-01-01"
        steps:
          - name: "fetch"
            action: "bash"
            with:
              linux: "echo $timestamp"
              windows: "echo %timestamp%"
        output:
          type: "string"
          value: "result[1]"
`,
			expectError: false,
		},

		// Case 7: Pass - Public function with needs result in needs params
		{
			name: "Pass: Public function with needs result in needs params",
			yamlInput: `
version: "v1"
author: "Test"
tools:
  - name: "TestTool"
    description: "Test tool"
    version: "1.0.0"
    functions:
      - name: "ProcessData"
        operation: "terminal"
        description: "Process data"
        triggers:
          - type: "flex_for_team"
        needs:
          - name: "fetchConfig"
          - name: "processWithConfig"
            params:
              config: "$fetchConfig"
        steps:
          - name: "process"
            action: "bash"
            with:
              linux: "echo ok"
              windows: "echo ok"
        output:
          type: "string"
          value: "result[1]"
      - name: "fetchConfig"
        operation: "terminal"
        description: "Fetch config"
        steps:
          - name: "fetch"
            action: "bash"
            with:
              linux: "echo config"
              windows: "echo config"
        output:
          type: "string"
          value: "result[1]"
      - name: "processWithConfig"
        operation: "terminal"
        description: "Process with config"
        input:
          - name: "config"
            description: "Config"
            value: "default-config"
        steps:
          - name: "process"
            action: "bash"
            with:
              linux: "echo $config"
              windows: "echo %config%"
        output:
          type: "string"
          value: "result[1]"
`,
			expectError: false,
		},

		// Case 8: Pass - Public function with fixed string value in needs params
		{
			name: "Pass: Public function with fixed string value in needs params",
			yamlInput: `
version: "v1"
author: "Test"
tools:
  - name: "TestTool"
    description: "Test tool"
    version: "1.0.0"
    functions:
      - name: "GetData"
        operation: "terminal"
        description: "Get data"
        triggers:
          - type: "flex_for_team"
        needs:
          - name: "fetchData"
            params:
              status: "active"
        steps:
          - name: "process"
            action: "bash"
            with:
              linux: "echo ok"
              windows: "echo ok"
        output:
          type: "string"
          value: "result[1]"
      - name: "fetchData"
        operation: "terminal"
        description: "Fetch data"
        input:
          - name: "status"
            description: "Status"
            value: "pending"
        steps:
          - name: "fetch"
            action: "bash"
            with:
              linux: "echo $status"
              windows: "echo %status%"
        output:
          type: "string"
          value: "result[1]"
`,
			expectError: false,
		},

		// Case 9: Error - Mixed valid and invalid vars in needs params
		{
			name: "Error: Mixed valid and invalid vars in needs params",
			yamlInput: `
version: "v1"
author: "Test"
tools:
  - name: "TestTool"
    description: "Test tool"
    version: "1.0.0"
    functions:
      - name: "GetData"
        operation: "terminal"
        description: "Get data"
        triggers:
          - type: "flex_for_team"
        needs:
          - name: "fetchData"
            params:
              msg: "$NOW - $dealId"
        input:
          - name: "dealId"
            description: "Deal ID"
            value: "12345"
        steps:
          - name: "process"
            action: "bash"
            with:
              linux: "echo ok"
              windows: "echo ok"
        output:
          type: "string"
          value: "result[1]"
      - name: "fetchData"
        operation: "terminal"
        description: "Fetch data"
        input:
          - name: "msg"
            description: "Message"
            value: "default-msg"
        steps:
          - name: "fetch"
            action: "bash"
            with:
              linux: "echo $msg"
              windows: "echo %msg%"
        output:
          type: "string"
          value: "result[1]"
`,
			expectError:   true,
			errorContains: "public function 'GetData' has needs 'fetchData' with param 'msg' referencing input variable",
		},

		// Case 10: Pass - Public function without needs block
		{
			name: "Pass: Public function without needs block",
			yamlInput: `
version: "v1"
author: "Test"
tools:
  - name: "TestTool"
    description: "Test tool"
    version: "1.0.0"
    functions:
      - name: "GetData"
        operation: "terminal"
        description: "Get data"
        triggers:
          - type: "flex_for_team"
        input:
          - name: "dealId"
            description: "Deal ID"
            value: "12345"
        steps:
          - name: "fetch"
            action: "bash"
            with:
              linux: "echo $dealId"
              windows: "echo %dealId%"
        output:
          type: "string"
          value: "result[1]"
`,
			expectError: false,
		},

		// Case 11: Pass - Public function with needs but without params
		{
			name: "Pass: Public function with needs but without params",
			yamlInput: `
version: "v1"
author: "Test"
tools:
  - name: "TestTool"
    description: "Test tool"
    version: "1.0.0"
    functions:
      - name: "GetData"
        operation: "terminal"
        description: "Get data"
        triggers:
          - type: "flex_for_team"
        needs:
          - name: "fetchConfig"
        input:
          - name: "dealId"
            description: "Deal ID"
            value: "12345"
        steps:
          - name: "process"
            action: "bash"
            with:
              linux: "echo ok"
              windows: "echo ok"
        output:
          type: "string"
          value: "result[1]"
      - name: "fetchConfig"
        operation: "terminal"
        description: "Fetch config"
        steps:
          - name: "fetch"
            action: "bash"
            with:
              linux: "echo config"
              windows: "echo config"
        output:
          type: "string"
          value: "result[1]"
`,
			expectError: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := CreateToolWithWarnings(tc.yamlInput)
			hasError := err != nil

			if tc.expectError && !hasError {
				t.Errorf("expected error but got none. Warnings: %v", result.Warnings)
			}
			if !tc.expectError && hasError {
				t.Errorf("unexpected error: %v", err)
			}
			if tc.expectError && tc.errorContains != "" && err != nil {
				if !strings.Contains(err.Error(), tc.errorContains) {
					t.Errorf("expected error containing '%s', got: %v", tc.errorContains, err)
				}
			}
		})
	}
}

// Test Group 2: checkInputVariableReferencesAccumulated
// Tests that input variables can only reference accumulated (previous) inputs, ENV vars, or system vars
func TestInputVariableReferencesAccumulated(t *testing.T) {
	testCases := []struct {
		name          string
		yamlInput     string
		expectError   bool
		errorContains string
	}{
		// Case 1: Pass - Input references previous input
		{
			name: "Pass: Input references previous input",
			yamlInput: `
version: "v1"
author: "Test"
tools:
  - name: "TestTool"
    description: "Test tool"
    version: "1.0.0"
    functions:
      - name: "ProcessData"
        operation: "terminal"
        description: "Process data"
        triggers:
          - type: "flex_for_team"
        input:
          - name: "input1"
            description: "First input"
            value: "first-value"
          - name: "input2"
            description: "Second input"
            value: "$input1"
        steps:
          - name: "process"
            action: "bash"
            with:
              linux: "echo $input2"
              windows: "echo %input2%"
        output:
          type: "string"
          value: "result[1]"
`,
			expectError: false,
		},

		// Case 2: Pass - Input references previous input ${}
		{
			name: "Pass: Input references previous input ${}",
			yamlInput: `
version: "v1"
author: "Test"
tools:
  - name: "TestTool"
    description: "Test tool"
    version: "1.0.0"
    functions:
      - name: "ProcessData"
        operation: "terminal"
        description: "Process data"
        triggers:
          - type: "flex_for_team"
        input:
          - name: "input1"
            description: "First input"
            value: "first-value"
          - name: "input2"
            description: "Second input"
            value: "${input1}"
        steps:
          - name: "process"
            action: "bash"
            with:
              linux: "echo $input2"
              windows: "echo %input2%"
        output:
          type: "string"
          value: "result[1]"
`,
			expectError: false,
		},

		// Case 3: Pass - Input references ENV var
		{
			name: "Pass: Input references ENV var",
			yamlInput: `
version: "v1"
author: "Test"
env:
  - name: "API_KEY"
    value: "secret123"
tools:
  - name: "TestTool"
    description: "Test tool"
    version: "1.0.0"
    functions:
      - name: "ProcessData"
        operation: "terminal"
        description: "Process data"
        triggers:
          - type: "flex_for_team"
        input:
          - name: "token"
            description: "Token"
            value: "$API_KEY"
        steps:
          - name: "process"
            action: "bash"
            with:
              linux: "echo $token"
              windows: "echo %token%"
        output:
          type: "string"
          value: "result[1]"
`,
			expectError: false,
		},

		// Case 4: Pass - Input references system var $NOW
		{
			name: "Pass: Input references system var $NOW",
			yamlInput: `
version: "v1"
author: "Test"
tools:
  - name: "TestTool"
    description: "Test tool"
    version: "1.0.0"
    functions:
      - name: "ProcessData"
        operation: "terminal"
        description: "Process data"
        triggers:
          - type: "flex_for_team"
        input:
          - name: "timestamp"
            description: "Timestamp"
            value: "$NOW"
        steps:
          - name: "process"
            action: "bash"
            with:
              linux: "echo $timestamp"
              windows: "echo %timestamp%"
        output:
          type: "string"
          value: "result[1]"
`,
			expectError: false,
		},

		// Case 5: Error - Input references later input
		{
			name: "Error: Input references later input",
			yamlInput: `
version: "v1"
author: "Test"
tools:
  - name: "TestTool"
    description: "Test tool"
    version: "1.0.0"
    functions:
      - name: "ProcessData"
        operation: "terminal"
        description: "Process data"
        triggers:
          - type: "flex_for_team"
        input:
          - name: "input1"
            description: "First input"
            value: "$input2"
          - name: "input2"
            description: "Second input"
            value: "second-value"
        steps:
          - name: "process"
            action: "bash"
            with:
              linux: "echo $input1"
              windows: "echo %input1%"
        output:
          type: "string"
          value: "result[1]"
`,
			expectError:   true,
			errorContains: "function 'ProcessData' input 'input1' references '$input2' which is not available",
		},

		// Case 6: Error - Input references non-existent var
		{
			name: "Error: Input references non-existent var",
			yamlInput: `
version: "v1"
author: "Test"
tools:
  - name: "TestTool"
    description: "Test tool"
    version: "1.0.0"
    functions:
      - name: "ProcessData"
        operation: "terminal"
        description: "Process data"
        triggers:
          - type: "flex_for_team"
        input:
          - name: "input1"
            description: "First input"
            value: "$nonExistent"
        steps:
          - name: "process"
            action: "bash"
            with:
              linux: "echo $input1"
              windows: "echo %input1%"
        output:
          type: "string"
          value: "result[1]"
`,
			expectError:   true,
			errorContains: "function 'ProcessData' input 'input1' references '$nonExistent' which is not available",
		},

		// Case 7: Pass - Input with fixed string value
		{
			name: "Pass: Input with fixed string value",
			yamlInput: `
version: "v1"
author: "Test"
tools:
  - name: "TestTool"
    description: "Test tool"
    version: "1.0.0"
    functions:
      - name: "ProcessData"
        operation: "terminal"
        description: "Process data"
        triggers:
          - type: "flex_for_team"
        input:
          - name: "status"
            description: "Status"
            value: "active"
        steps:
          - name: "process"
            action: "bash"
            with:
              linux: "echo $status"
              windows: "echo %status%"
        output:
          type: "string"
          value: "result[1]"
`,
			expectError: false,
		},

		// Case 8: Error - Input references itself
		{
			name: "Error: Input references itself",
			yamlInput: `
version: "v1"
author: "Test"
tools:
  - name: "TestTool"
    description: "Test tool"
    version: "1.0.0"
    functions:
      - name: "ProcessData"
        operation: "terminal"
        description: "Process data"
        triggers:
          - type: "flex_for_team"
        input:
          - name: "input1"
            description: "First input"
            value: "$input1"
        steps:
          - name: "process"
            action: "bash"
            with:
              linux: "echo $input1"
              windows: "echo %input1%"
        output:
          type: "string"
          value: "result[1]"
`,
			expectError:   true,
			errorContains: "function 'ProcessData' input 'input1' references '$input1' which is not available",
		},

		// Case 9: Pass - Multiple accumulated refs
		{
			name: "Pass: Multiple accumulated refs",
			yamlInput: `
version: "v1"
author: "Test"
tools:
  - name: "TestTool"
    description: "Test tool"
    version: "1.0.0"
    functions:
      - name: "ProcessData"
        operation: "terminal"
        description: "Process data"
        triggers:
          - type: "flex_for_team"
        input:
          - name: "input1"
            description: "First input"
            value: "first"
          - name: "input2"
            description: "Second input"
            value: "second"
          - name: "input3"
            description: "Third input"
            value: "$input1 $input2"
        steps:
          - name: "process"
            action: "bash"
            with:
              linux: "echo $input3"
              windows: "echo %input3%"
        output:
          type: "string"
          value: "result[1]"
`,
			expectError: false,
		},

		// Case 10: Error - Mixed valid and invalid refs in input
		{
			name: "Error: Mixed valid and invalid refs in input",
			yamlInput: `
version: "v1"
author: "Test"
tools:
  - name: "TestTool"
    description: "Test tool"
    version: "1.0.0"
    functions:
      - name: "ProcessData"
        operation: "terminal"
        description: "Process data"
        triggers:
          - type: "flex_for_team"
        input:
          - name: "input1"
            description: "First input"
            value: "first"
          - name: "input2"
            description: "Second input"
            value: "$input1 $input3"
          - name: "input3"
            description: "Third input"
            value: "third"
        steps:
          - name: "process"
            action: "bash"
            with:
              linux: "echo $input2"
              windows: "echo %input2%"
        output:
          type: "string"
          value: "result[1]"
`,
			expectError:   true,
			errorContains: "function 'ProcessData' input 'input2' references '$input3' which is not available",
		},

		// Case 11: Pass - Field access on accumulated input
		{
			name: "Pass: Field access on accumulated input",
			yamlInput: `
version: "v1"
author: "Test"
tools:
  - name: "TestTool"
    description: "Test tool"
    version: "1.0.0"
    functions:
      - name: "ProcessData"
        operation: "terminal"
        description: "Process data"
        triggers:
          - type: "flex_for_team"
        input:
          - name: "input1"
            description: "First input"
            value: "{\"field\": \"value\"}"
          - name: "input2"
            description: "Second input"
            value: "$input1.field"
        steps:
          - name: "process"
            action: "bash"
            with:
              linux: "echo $input2"
              windows: "echo %input2%"
        output:
          type: "string"
          value: "result[1]"
`,
			expectError: false,
		},

		// Case 12: Pass - All system vars
		{
			name: "Pass: All system vars",
			yamlInput: `
version: "v1"
author: "Test"
tools:
  - name: "TestTool"
    description: "Test tool"
    version: "1.0.0"
    functions:
      - name: "ProcessData"
        operation: "terminal"
        description: "Process data"
        triggers:
          - type: "flex_for_team"
        input:
          - name: "user"
            description: "User"
            value: "$USER"
          - name: "now"
            description: "Now"
            value: "$NOW"
          - name: "message"
            description: "Message"
            value: "$MESSAGE"
          - name: "admin"
            description: "Admin"
            value: "$ADMIN"
          - name: "company"
            description: "Company"
            value: "$COMPANY"
          - name: "uuid"
            description: "UUID"
            value: "$UUID"
          - name: "file"
            description: "File"
            value: "$FILE"
          - name: "me"
            description: "Me"
            value: "$ME"
        steps:
          - name: "process"
            action: "bash"
            with:
              linux: "echo ok"
              windows: "echo ok"
        output:
          type: "string"
          value: "result[1]"
`,
			expectError: false,
		},

		// Case 13: Error - Unknown uppercase var (not a known system var)
		{
			name: "Error: Unknown uppercase var (not a known system var)",
			yamlInput: `
version: "v1"
author: "Test"
tools:
  - name: "TestTool"
    description: "Test tool"
    version: "1.0.0"
    functions:
      - name: "ProcessData"
        operation: "terminal"
        description: "Process data"
        triggers:
          - type: "flex_for_team"
        input:
          - name: "value"
            description: "Value"
            value: "$UNKNOWN"
        steps:
          - name: "process"
            action: "bash"
            with:
              linux: "echo $value"
              windows: "echo %value%"
        output:
          type: "string"
          value: "result[1]"
`,
			expectError:   true,
			errorContains: "$UNKNOWN",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := CreateToolWithWarnings(tc.yamlInput)
			hasError := err != nil

			if tc.expectError && !hasError {
				t.Errorf("expected error but got none. Warnings: %v", result.Warnings)
			}
			if !tc.expectError && hasError {
				t.Errorf("unexpected error: %v", err)
			}
			if tc.expectError && tc.errorContains != "" && err != nil {
				if !strings.Contains(err.Error(), tc.errorContains) {
					t.Errorf("expected error containing '%s', got: %v", tc.errorContains, err)
				}
			}
		})
	}
}
