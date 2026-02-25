package skill

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateFunctionOnSkip(t *testing.T) {
	testCases := []struct {
		name     string
		yamlData string
		wantErr  bool
		errMsg   string
	}{
		{
			name: "Valid onSkip - Tool function",
			yamlData: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "Tool for testing"
    version: "1.0.0"
    functions:
      - name: "processPayment"
        operation: "api_call"
        description: "Process a payment"
        runOnlyIf:
          deterministic: "$getStatus != 'already_paid'"
        onSkip: ["logSkippedPayment"]
        triggers:
          - type: "flex_for_user"
        input:
          - name: "status"
            description: "Payment status"
            origin: "function"
            from: "getStatus"
            isOptional: true
        needs:
          - name: "getStatus"
        steps:
          - name: "process"
            action: "POST"
            with:
              url: "https://api.example.com/payment"
              requestBody:
                type: "application/json"
                with:
                  status: "$status"

      - name: "getStatus"
        operation: "api_call"
        description: "Get status"
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "get"
            action: "GET"
            with:
              url: "https://api.example.com/status"

      - name: "logSkippedPayment"
        operation: "api_call"
        description: "Log when payment was skipped"
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "log"
            action: "POST"
            with:
              url: "https://api.example.com/log"
`,
			wantErr: false,
		},
		{
			name: "Valid onSkip - System function",
			yamlData: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "Tool for testing"
    version: "1.0.0"
    functions:
      - name: "processData"
        operation: "api_call"
        description: "Process data"
        runOnlyIf:
          deterministic: "$checkFlag == true"
        onSkip: ["NotifyHuman"]
        triggers:
          - type: "flex_for_user"
        input:
          - name: "shouldProcess"
            description: "Whether to process"
            origin: "function"
            from: "checkFlag"
            isOptional: true
        needs:
          - name: "checkFlag"
        steps:
          - name: "process"
            action: "GET"
            with:
              url: "https://api.example.com/data"

      - name: "checkFlag"
        operation: "api_call"
        description: "Check flag"
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "check"
            action: "GET"
            with:
              url: "https://api.example.com/flag"
`,
			wantErr: false,
		},
		{
			name: "Invalid onSkip - Non-existent function",
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
        runOnlyIf:
          deterministic: "$getFlag == true"
        onSkip: ["nonExistentFunc"]
        triggers:
          - type: "flex_for_user"
        input:
          - name: "flag"
            description: "A flag"
            origin: "function"
            from: "getFlag"
            isOptional: true
        needs:
          - name: "getFlag"
        steps:
          - name: "step1"
            action: "GET"
            with:
              url: "https://api.example.com"

      - name: "getFlag"
        operation: "api_call"
        description: "Get flag"
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "get"
            action: "GET"
            with:
              url: "https://api.example.com/flag"
`,
			wantErr: true,
			errMsg:  "is not available in the tool",
		},
		{
			name: "Invalid onSkip - Self reference",
			yamlData: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "Tool for testing"
    version: "1.0.0"
    functions:
      - name: "selfRefFunc"
        operation: "api_call"
        description: "Function that calls itself on skip"
        runOnlyIf:
          deterministic: "$getFlag == true"
        onSkip: ["selfRefFunc"]
        triggers:
          - type: "flex_for_user"
        input:
          - name: "flag"
            description: "A flag"
            origin: "function"
            from: "getFlag"
            isOptional: true
        needs:
          - name: "getFlag"
        steps:
          - name: "step1"
            action: "GET"
            with:
              url: "https://api.example.com"

      - name: "getFlag"
        operation: "api_call"
        description: "Get flag"
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "get"
            action: "GET"
            with:
              url: "https://api.example.com/flag"
`,
			wantErr: true,
			errMsg:  "cannot include itself in its 'onSkip' list",
		},
		{
			name: "Valid onSkip with $skipReason parameter",
			yamlData: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "Tool for testing"
    version: "1.0.0"
    functions:
      - name: "processPayment"
        operation: "api_call"
        description: "Process a payment"
        runOnlyIf:
          deterministic: "$getPaymentStatus != 'already_paid'"
        onSkip:
          - name: "logSkippedPayment"
            params:
              reason: "$skipReason"
              paymentId: "$paymentId"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "paymentId"
            description: "Payment ID"
            origin: "chat"
            isOptional: true
          - name: "paymentStatus"
            description: "Payment status"
            origin: "function"
            from: "getPaymentStatus"
            isOptional: true
        needs:
          - name: "getPaymentStatus"
        steps:
          - name: "process"
            action: "POST"
            with:
              url: "https://api.example.com/payment"
              requestBody:
                type: "application/json"
                with:
                  id: "$paymentId"

      - name: "getPaymentStatus"
        operation: "api_call"
        description: "Get payment status"
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "get"
            action: "GET"
            with:
              url: "https://api.example.com/status"

      - name: "logSkippedPayment"
        operation: "api_call"
        description: "Log skipped payment"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "reason"
            description: "Skip reason"
            origin: "chat"
            isOptional: true
          - name: "paymentId"
            description: "Payment ID"
            origin: "chat"
            isOptional: true
        steps:
          - name: "log"
            action: "POST"
            with:
              url: "https://api.example.com/log"
              requestBody:
                type: "application/json"
                with:
                  reason: "$reason"
                  paymentId: "$paymentId"
`,
			wantErr: false,
		},
		{
			name: "Valid onSkip with sibling chaining",
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
        runOnlyIf:
          deterministic: "$getFlag == true"
        onSkip:
          - name: "firstCallback"
            params:
              reason: "$skipReason"
          - name: "secondCallback"
            params:
              dataFromFirst: "$firstCallback.result"
              originalReason: "$skipReason"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "flag"
            description: "A flag"
            origin: "function"
            from: "getFlag"
            isOptional: true
        needs:
          - name: "getFlag"
        steps:
          - name: "execute"
            action: "GET"
            with:
              url: "https://api.example.com"

      - name: "getFlag"
        operation: "api_call"
        description: "Get flag"
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "get"
            action: "GET"
            with:
              url: "https://api.example.com/flag"

      - name: "firstCallback"
        operation: "api_call"
        description: "First callback"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "reason"
            description: "Skip reason"
            origin: "chat"
            isOptional: true
        steps:
          - name: "step1"
            action: "POST"
            with:
              url: "https://api.example.com/first"
              requestBody:
                type: "application/json"
                with:
                  reason: "$reason"

      - name: "secondCallback"
        operation: "api_call"
        description: "Second callback"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "dataFromFirst"
            description: "Data from first callback"
            origin: "chat"
            isOptional: true
          - name: "originalReason"
            description: "Original skip reason"
            origin: "chat"
            isOptional: true
        steps:
          - name: "step1"
            action: "POST"
            with:
              url: "https://api.example.com/second"
              requestBody:
                type: "application/json"
                with:
                  data: "$dataFromFirst"
                  reason: "$originalReason"
`,
			wantErr: false,
		},
		{
			name: "Invalid onSkip - references sibling that comes AFTER",
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
        runOnlyIf:
          deterministic: "$getFlag == true"
        onSkip:
          - name: "firstCallback"
            params:
              dataFromSecond: "$secondCallback.result"
          - name: "secondCallback"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "flag"
            description: "A flag"
            origin: "function"
            from: "getFlag"
            isOptional: true
        needs:
          - name: "getFlag"
        steps:
          - name: "execute"
            action: "GET"
            with:
              url: "https://api.example.com"

      - name: "getFlag"
        operation: "api_call"
        description: "Get flag"
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "get"
            action: "GET"
            with:
              url: "https://api.example.com/flag"

      - name: "firstCallback"
        operation: "api_call"
        description: "First callback"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "dataFromSecond"
            description: "Data from second callback"
            origin: "chat"
            isOptional: true
        steps:
          - name: "step1"
            action: "GET"
            with:
              url: "https://api.example.com/first"

      - name: "secondCallback"
        operation: "api_call"
        description: "Second callback"
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "step1"
            action: "POST"
            with:
              url: "https://api.example.com/second"
`,
			wantErr: true,
			errMsg:  "references '$secondCallback' which is not available",
		},
		{
			name: "Valid onSkip - Multiple functions",
			yamlData: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "Tool for testing"
    version: "1.0.0"
    functions:
      - name: "mainProcess"
        operation: "api_call"
        description: "Main process"
        runOnlyIf:
          deterministic: "$checkRun == true"
        onSkip: ["cleanup", "notify", "log"]
        triggers:
          - type: "flex_for_user"
        input:
          - name: "shouldRun"
            description: "Whether to run"
            origin: "function"
            from: "checkRun"
            isOptional: true
        needs:
          - name: "checkRun"
        steps:
          - name: "process"
            action: "POST"
            with:
              url: "https://api.example.com/process"

      - name: "checkRun"
        operation: "api_call"
        description: "Check if should run"
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "check"
            action: "GET"
            with:
              url: "https://api.example.com/check"

      - name: "cleanup"
        operation: "api_call"
        description: "Cleanup resources"
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "clean"
            action: "DELETE"
            with:
              url: "https://api.example.com/temp"

      - name: "notify"
        operation: "api_call"
        description: "Send notification"
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "send"
            action: "POST"
            with:
              url: "https://api.example.com/notify"

      - name: "log"
        operation: "db"
        description: "Log to database"
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "write"
            action: "write"
            with:
              write: "INSERT INTO logs VALUES ('skipped')"
`,
			wantErr: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := CreateTool(tc.yamlData)

			if tc.wantErr {
				assert.Error(t, err)
				if tc.errMsg != "" {
					assert.Contains(t, err.Error(), tc.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestOnSkipFieldInFunction(t *testing.T) {
	yamlData := `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "Tool for testing"
    version: "1.0.0"
    functions:
      - name: "mainFunction"
        operation: "api_call"
        description: "Main function"
        runOnlyIf:
          deterministic: "$getFlag == true"
        onSkip: ["skipHandler"]
        triggers:
          - type: "flex_for_user"
        input:
          - name: "flag"
            description: "A flag"
            origin: "function"
            from: "getFlag"
            isOptional: true
        needs:
          - name: "getFlag"
        steps:
          - name: "execute"
            action: "GET"
            with:
              url: "https://api.example.com"

      - name: "getFlag"
        operation: "api_call"
        description: "Get flag"
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "get"
            action: "GET"
            with:
              url: "https://api.example.com/flag"

      - name: "skipHandler"
        operation: "api_call"
        description: "Skip handler"
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "handle"
            action: "POST"
            with:
              url: "https://api.example.com/skip"
`

	tool, err := CreateTool(yamlData)
	assert.NoError(t, err)
	assert.NotNil(t, tool)

	// Verify the onSkip field is properly parsed
	assert.Len(t, tool.Tools, 1)
	assert.Len(t, tool.Tools[0].Functions, 3)

	mainFunc := tool.Tools[0].Functions[0]
	assert.Equal(t, "mainFunction", mainFunc.Name)
	assert.Len(t, mainFunc.OnSkip, 1)
	assert.Equal(t, "skipHandler", mainFunc.OnSkip[0].Name)
}

func TestOnSkipWithRunOnlyIf(t *testing.T) {
	yamlData := `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "Tool for testing"
    version: "1.0.0"
    functions:
      - name: "mainFunction"
        operation: "api_call"
        description: "Main function"
        runOnlyIf:
          deterministic: "$checkShouldRun == true"
        onSkip:
          - name: "conditionalSkipHandler"
            runOnlyIf:
              deterministic: "$notifyOnSkip == true"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "shouldRun"
            description: "Whether to run"
            origin: "function"
            from: "checkShouldRun"
            isOptional: true
          - name: "notifyOnSkip"
            description: "Whether to notify on skip"
            origin: "chat"
            isOptional: true
        needs:
          - name: "checkShouldRun"
        steps:
          - name: "execute"
            action: "GET"
            with:
              url: "https://api.example.com"

      - name: "checkShouldRun"
        operation: "api_call"
        description: "Check should run"
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "check"
            action: "GET"
            with:
              url: "https://api.example.com/check"

      - name: "conditionalSkipHandler"
        operation: "api_call"
        description: "Conditional skip handler"
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "handle"
            action: "POST"
            with:
              url: "https://api.example.com/skip"
`

	tool, err := CreateTool(yamlData)
	assert.NoError(t, err)
	assert.NotNil(t, tool)

	mainFunc := tool.Tools[0].Functions[0]
	assert.Len(t, mainFunc.OnSkip, 1)
	assert.NotNil(t, mainFunc.OnSkip[0].RunOnlyIf)
}

func TestOnSkipCircularDependencyDetection(t *testing.T) {
	testCases := []struct {
		name     string
		yamlData string
		wantErr  bool
	}{
		{
			name: "Direct circular dependency via onSkip",
			yamlData: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "Tool for testing"
    version: "1.0.0"
    functions:
      - name: "funcA"
        operation: "api_call"
        description: "Function A"
        runOnlyIf:
          deterministic: "$checkA == true"
        onSkip: ["funcB"]
        triggers:
          - type: "flex_for_user"
        input:
          - name: "checkA"
            description: "Check A result"
            origin: "function"
            from: "checkA"
            isOptional: true
        needs:
          - name: "checkA"
        steps:
          - name: "stepA"
            action: "GET"
            with:
              url: "https://api.example.com/a"

      - name: "funcB"
        operation: "api_call"
        description: "Function B"
        runOnlyIf:
          deterministic: "$checkB == true"
        onSkip: ["funcA"]
        triggers:
          - type: "flex_for_user"
        input:
          - name: "checkB"
            description: "Check B result"
            origin: "function"
            from: "checkB"
            isOptional: true
        needs:
          - name: "checkB"
        steps:
          - name: "stepB"
            action: "GET"
            with:
              url: "https://api.example.com/b"

      - name: "checkA"
        operation: "api_call"
        description: "Check A"
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "check"
            action: "GET"
            with:
              url: "https://api.example.com/checkA"

      - name: "checkB"
        operation: "api_call"
        description: "Check B"
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "check"
            action: "GET"
            with:
              url: "https://api.example.com/checkB"
`,
			wantErr: true,
		},
		{
			name: "Indirect circular dependency via onSkip",
			yamlData: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "Tool for testing"
    version: "1.0.0"
    functions:
      - name: "funcA"
        operation: "api_call"
        description: "Function A"
        runOnlyIf:
          deterministic: "$checkA == true"
        onSkip: ["funcB"]
        triggers:
          - type: "flex_for_user"
        input:
          - name: "checkA"
            description: "Check A result"
            origin: "function"
            from: "checkA"
            isOptional: true
        needs:
          - name: "checkA"
        steps:
          - name: "stepA"
            action: "GET"
            with:
              url: "https://api.example.com/a"

      - name: "funcB"
        operation: "api_call"
        description: "Function B"
        runOnlyIf:
          deterministic: "$checkB == true"
        onSkip: ["funcC"]
        triggers:
          - type: "flex_for_user"
        input:
          - name: "checkB"
            description: "Check B result"
            origin: "function"
            from: "checkB"
            isOptional: true
        needs:
          - name: "checkB"
        steps:
          - name: "stepB"
            action: "GET"
            with:
              url: "https://api.example.com/b"

      - name: "funcC"
        operation: "api_call"
        description: "Function C"
        runOnlyIf:
          deterministic: "$checkC == true"
        onSkip: ["funcA"]
        triggers:
          - type: "flex_for_user"
        input:
          - name: "checkC"
            description: "Check C result"
            origin: "function"
            from: "checkC"
            isOptional: true
        needs:
          - name: "checkC"
        steps:
          - name: "stepC"
            action: "GET"
            with:
              url: "https://api.example.com/c"

      - name: "checkA"
        operation: "api_call"
        description: "Check A"
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "check"
            action: "GET"
            with:
              url: "https://api.example.com/checkA"

      - name: "checkB"
        operation: "api_call"
        description: "Check B"
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "check"
            action: "GET"
            with:
              url: "https://api.example.com/checkB"

      - name: "checkC"
        operation: "api_call"
        description: "Check C"
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "check"
            action: "GET"
            with:
              url: "https://api.example.com/checkC"
`,
			wantErr: true,
		},
		{
			name: "No circular dependency with onSkip",
			yamlData: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "Tool for testing"
    version: "1.0.0"
    functions:
      - name: "funcA"
        operation: "api_call"
        description: "Function A"
        runOnlyIf:
          deterministic: "$checkA == true"
        onSkip: ["funcB"]
        triggers:
          - type: "flex_for_user"
        input:
          - name: "checkA"
            description: "Check A result"
            origin: "function"
            from: "checkA"
            isOptional: true
        needs:
          - name: "checkA"
        steps:
          - name: "stepA"
            action: "GET"
            with:
              url: "https://api.example.com/a"

      - name: "funcB"
        operation: "api_call"
        description: "Function B"
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "stepB"
            action: "GET"
            with:
              url: "https://api.example.com/b"

      - name: "checkA"
        operation: "api_call"
        description: "Check A"
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "check"
            action: "GET"
            with:
              url: "https://api.example.com/checkA"
`,
			wantErr: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := CreateTool(tc.yamlData)

			if tc.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "circular dependency")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
