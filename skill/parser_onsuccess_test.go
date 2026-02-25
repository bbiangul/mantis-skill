package skill

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateFunctionOnSuccess(t *testing.T) {
	testCases := []struct {
		name     string
		yamlData string
		wantErr  bool
		errMsg   string
	}{
		{
			name: "Valid onSuccess - Tool function",
			yamlData: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "Tool for testing"
    version: "1.0.0"
    functions:
      - name: "createBooking"
        operation: "api_call"
        description: "Create a booking"
        onSuccess: ["sendEmail", "logToDatabase"]
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "book"
            action: "POST"
            with:
              url: "https://api.example.com/book"
              requestBody:
                type: "application/json"
                with:
                  data: "test"

      - name: "sendEmail"
        operation: "api_call"
        description: "Send confirmation email"
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "send"
            action: "POST"
            with:
              url: "https://api.example.com/email"

      - name: "logToDatabase"
        operation: "db"
        description: "Log booking to database"
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "log"
            action: "write"
            with:
              write: "INSERT INTO logs VALUES ('booked')"
`,
			wantErr: false,
		},
		{
			name: "Valid onSuccess - System function",
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
        description: "Process data and notify"
        onSuccess: ["NotifyHuman"]
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "process"
            action: "GET"
            with:
              url: "https://api.example.com/data"
`,
			wantErr: false,
		},
		{
			name: "Invalid onSuccess - Non-existent function",
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
        description: "Function with invalid onSuccess"
        onSuccess: ["nonExistentFunc"]
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "step1"
            action: "GET"
            with:
              url: "https://api.example.com"
`,
			wantErr: true,
			errMsg:  "is not available in the tool or as a system function",
		},
		{
			name: "Invalid onSuccess - Self reference",
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
        description: "Function that calls itself on success"
        onSuccess: ["selfRefFunc"]
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "step1"
            action: "GET"
            with:
              url: "https://api.example.com"
`,
			wantErr: true,
			errMsg:  "cannot include itself in its 'onSuccess' list",
		},
		{
			name: "Invalid onSuccess - Circular dependency",
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
        onSuccess: ["funcB"]
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "step1"
            action: "GET"
            with:
              url: "https://api.example.com"

      - name: "funcB"
        operation: "api_call"
        description: "Function B"
        onSuccess: ["funcA"]
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "step1"
            action: "GET"
            with:
              url: "https://api.example.com"
`,
			wantErr: true,
			errMsg:  "circular dependency detected",
		},
		{
			name: "Valid onSuccess - Multiple functions",
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
        onSuccess: ["cleanup", "notify", "log"]
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "process"
            action: "POST"
            with:
              url: "https://api.example.com/process"

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
              write: "INSERT INTO logs VALUES ('processed')"
`,
			wantErr: false,
		},
		{
			name: "Valid onSuccess - Chain of functions",
			yamlData: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "Tool for testing"
    version: "1.0.0"
    functions:
      - name: "step1"
        operation: "api_call"
        description: "First step"
        onSuccess: ["step2"]
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "execute"
            action: "GET"
            with:
              url: "https://api.example.com/step1"

      - name: "step2"
        operation: "api_call"
        description: "Second step"
        onSuccess: ["step3"]
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "execute"
            action: "GET"
            with:
              url: "https://api.example.com/step2"

      - name: "step3"
        operation: "api_call"
        description: "Third step"
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "execute"
            action: "GET"
            with:
              url: "https://api.example.com/step3"
`,
			wantErr: false,
		},
		{
			name: "Mixed needs and onSuccess",
			yamlData: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "Tool for testing"
    version: "1.0.0"
    functions:
      - name: "prepare"
        operation: "api_call"
        description: "Prepare data"
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "prep"
            action: "GET"
            with:
              url: "https://api.example.com/prepare"

      - name: "process"
        operation: "api_call"
        description: "Process with dependencies"
        needs: ["prepare"]
        onSuccess: ["cleanup", "notify"]
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "exec"
            action: "POST"
            with:
              url: "https://api.example.com/process"

      - name: "cleanup"
        operation: "api_call"
        description: "Cleanup"
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "clean"
            action: "DELETE"
            with:
              url: "https://api.example.com/cleanup"

      - name: "notify"
        operation: "api_call"
        description: "Notify"
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "send"
            action: "POST"
            with:
              url: "https://api.example.com/notify"
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

func TestOnSuccessFieldInFunction(t *testing.T) {
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
        onSuccess: ["successHandler"]
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "execute"
            action: "GET"
            with:
              url: "https://api.example.com"

      - name: "successHandler"
        operation: "api_call"
        description: "Success handler"
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "handle"
            action: "POST"
            with:
              url: "https://api.example.com/success"
`

	tool, err := CreateTool(yamlData)
	assert.NoError(t, err)
	assert.NotNil(t, tool)

	// Verify the onSuccess field is properly parsed
	assert.Len(t, tool.Tools, 1)
	assert.Len(t, tool.Tools[0].Functions, 2)

	mainFunc := tool.Tools[0].Functions[0]
	assert.Equal(t, "mainFunction", mainFunc.Name)
	assert.Len(t, mainFunc.OnSuccess, 1)
	assert.Equal(t, "successHandler", mainFunc.OnSuccess[0].Name)
}

func TestMarkOnSuccessReferencedFunctions(t *testing.T) {
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
        onSuccess: ["successHandler", "logger"]
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "execute"
            action: "GET"
            with:
              url: "https://api.example.com"

      - name: "successHandler"
        operation: "api_call"
        description: "Success handler"
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "handle"
            action: "POST"
            with:
              url: "https://api.example.com/success"

      - name: "logger"
        operation: "api_call"
        description: "Logger function"
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "log"
            action: "POST"
            with:
              url: "https://api.example.com/log"

      - name: "independentFunction"
        operation: "api_call"
        description: "Independent function not referenced in onSuccess"
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "execute"
            action: "GET"
            with:
              url: "https://api.example.com/independent"
`

	tool, err := CreateTool(yamlData)
	assert.NoError(t, err)
	assert.NotNil(t, tool)

	// Verify the functions have correct SkipInputPreEvaluation flag
	assert.Len(t, tool.Tools, 1)
	assert.Len(t, tool.Tools[0].Functions, 4)

	// All functions should default to false (allow pre-evaluation)
	mainFunc := tool.Tools[0].Functions[0]
	assert.Equal(t, "mainFunction", mainFunc.Name)
	assert.False(t, mainFunc.SkipInputPreEvaluation, "mainFunction should allow input pre-evaluation (default false)")

	// successHandler: default is false even though it's in onSuccess
	successHandler := tool.Tools[0].Functions[1]
	assert.Equal(t, "successHandler", successHandler.Name)
	assert.False(t, successHandler.SkipInputPreEvaluation, "successHandler should allow input pre-evaluation (default false)")

	// logger: default is false even though it's in onSuccess
	logger := tool.Tools[0].Functions[2]
	assert.Equal(t, "logger", logger.Name)
	assert.False(t, logger.SkipInputPreEvaluation, "logger should allow input pre-evaluation (default false)")

	// independentFunction: default is false
	independentFunc := tool.Tools[0].Functions[3]
	assert.Equal(t, "independentFunction", independentFunc.Name)
	assert.False(t, independentFunc.SkipInputPreEvaluation, "independentFunction should allow input pre-evaluation (default false)")
}

func TestMarkOnSuccessReferencedFunctionsChained(t *testing.T) {
	yamlData := `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "Tool for testing"
    version: "1.0.0"
    functions:
      - name: "step1"
        operation: "api_call"
        description: "First step"
        onSuccess: ["step2"]
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "execute"
            action: "GET"
            with:
              url: "https://api.example.com/step1"

      - name: "step2"
        operation: "api_call"
        description: "Second step"
        onSuccess: ["step3"]
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "execute"
            action: "GET"
            with:
              url: "https://api.example.com/step2"

      - name: "step3"
        operation: "api_call"
        description: "Third step"
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "execute"
            action: "GET"
            with:
              url: "https://api.example.com/step3"
`

	tool, err := CreateTool(yamlData)
	assert.NoError(t, err)
	assert.NotNil(t, tool)

	// Verify all functions default to false
	assert.Len(t, tool.Tools[0].Functions, 3)

	// step1: default is false
	step1 := tool.Tools[0].Functions[0]
	assert.Equal(t, "step1", step1.Name)
	assert.False(t, step1.SkipInputPreEvaluation, "step1 should allow input pre-evaluation (default false)")

	// step2: default is false even though referenced by step1
	step2 := tool.Tools[0].Functions[1]
	assert.Equal(t, "step2", step2.Name)
	assert.False(t, step2.SkipInputPreEvaluation, "step2 should allow input pre-evaluation (default false)")

	// step3: default is false even though referenced by step2
	step3 := tool.Tools[0].Functions[2]
	assert.Equal(t, "step3", step3.Name)
	assert.False(t, step3.SkipInputPreEvaluation, "step3 should allow input pre-evaluation (default false)")
}

func TestMarkOnSuccessReferencedFunctionsSystemFunction(t *testing.T) {
	yamlData := `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "Tool for testing"
    version: "1.0.0"
    functions:
      - name: "processData"
        operation: "api_call"
        description: "Process data and notify"
        onSuccess: ["NotifyHuman", "localLogger"]
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "process"
            action: "GET"
            with:
              url: "https://api.example.com/data"

      - name: "localLogger"
        operation: "api_call"
        description: "Local logger"
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "log"
            action: "POST"
            with:
              url: "https://api.example.com/log"
`

	tool, err := CreateTool(yamlData)
	assert.NoError(t, err)
	assert.NotNil(t, tool)

	// Verify system functions don't cause issues
	assert.Len(t, tool.Tools[0].Functions, 2)

	// processData: default is false
	processData := tool.Tools[0].Functions[0]
	assert.Equal(t, "processData", processData.Name)
	assert.False(t, processData.SkipInputPreEvaluation, "processData should allow input pre-evaluation (default false)")

	// localLogger: default is false even though referenced in onSuccess
	localLogger := tool.Tools[0].Functions[1]
	assert.Equal(t, "localLogger", localLogger.Name)
	assert.False(t, localLogger.SkipInputPreEvaluation, "localLogger should allow input pre-evaluation (default false)")
}

func TestSkipInputPreEvaluationYAMLField(t *testing.T) {
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
        onSuccess: ["callbackWithSkip", "callbackWithoutSkip", "callbackDefault"]
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "execute"
            action: "GET"
            with:
              url: "https://api.example.com"

      - name: "callbackWithSkip"
        operation: "api_call"
        description: "Callback with explicit skipInputPreEvaluation: true"
        skipInputPreEvaluation: true
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "handle"
            action: "POST"
            with:
              url: "https://api.example.com/callback1"

      - name: "callbackWithoutSkip"
        operation: "api_call"
        description: "Callback with explicit skipInputPreEvaluation: false"
        skipInputPreEvaluation: false
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "handle"
            action: "POST"
            with:
              url: "https://api.example.com/callback2"

      - name: "callbackDefault"
        operation: "api_call"
        description: "Callback without explicit skipInputPreEvaluation"
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "handle"
            action: "POST"
            with:
              url: "https://api.example.com/callback3"

      - name: "independentFunction"
        operation: "api_call"
        description: "Independent function with explicit skipInputPreEvaluation: true"
        skipInputPreEvaluation: true
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "execute"
            action: "GET"
            with:
              url: "https://api.example.com/independent"
`

	tool, err := CreateTool(yamlData)
	assert.NoError(t, err)
	assert.NotNil(t, tool)

	assert.Len(t, tool.Tools[0].Functions, 5)

	// mainFunction: default is false
	mainFunc := tool.Tools[0].Functions[0]
	assert.Equal(t, "mainFunction", mainFunc.Name)
	assert.False(t, mainFunc.SkipInputPreEvaluation, "mainFunction should allow input pre-evaluation (default false)")

	// callbackWithSkip: explicitly set to true in YAML
	callbackWithSkip := tool.Tools[0].Functions[1]
	assert.Equal(t, "callbackWithSkip", callbackWithSkip.Name)
	assert.True(t, callbackWithSkip.SkipInputPreEvaluation, "callbackWithSkip has explicit skipInputPreEvaluation: true")

	// callbackWithoutSkip: explicitly set to false in YAML
	callbackWithoutSkip := tool.Tools[0].Functions[2]
	assert.Equal(t, "callbackWithoutSkip", callbackWithoutSkip.Name)
	assert.False(t, callbackWithoutSkip.SkipInputPreEvaluation, "callbackWithoutSkip has explicit skipInputPreEvaluation: false")

	// callbackDefault: no explicit value, defaults to false
	callbackDefault := tool.Tools[0].Functions[3]
	assert.Equal(t, "callbackDefault", callbackDefault.Name)
	assert.False(t, callbackDefault.SkipInputPreEvaluation, "callbackDefault should default to false")

	// independentFunction: explicitly set to true
	independentFunc := tool.Tools[0].Functions[4]
	assert.Equal(t, "independentFunction", independentFunc.Name)
	assert.True(t, independentFunc.SkipInputPreEvaluation, "independentFunction has explicit skipInputPreEvaluation: true")
}

func TestOnSuccessSiblingChaining(t *testing.T) {
	testCases := []struct {
		name     string
		yamlData string
		wantErr  bool
		errMsg   string
	}{
		{
			name: "Valid sibling chaining - second function references first function output",
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
          - name: "firstCallback"
          - name: "secondCallback"
            params:
              dataFromFirst: "$firstCallback.result"
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "execute"
            action: "GET"
            with:
              url: "https://api.example.com"

      - name: "firstCallback"
        operation: "api_call"
        description: "First callback"
        triggers:
          - type: "flex_for_user"
        input: []
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
        input:
          - name: "dataFromFirst"
            description: "Data from first callback"
            value: "$dataFromFirst"
        steps:
          - name: "step1"
            action: "POST"
            with:
              url: "https://api.example.com/second"
              requestBody:
                type: "application/json"
                with:
                  data: "$dataFromFirst"
`,
			wantErr: false,
		},
		{
			name: "Valid sibling chaining - third function references both first and second sibling outputs",
			yamlData: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "Tool for testing"
    version: "1.0.0"
    functions:
      - name: "MainProcess"
        operation: "api_call"
        description: "Main process"
        onSuccess:
          - name: "stepOne"
          - name: "stepTwo"
            params:
              inputFromOne: "$stepOne.output"
          - name: "stepThree"
            params:
              inputFromOne: "$stepOne.output"
              inputFromTwo: "$stepTwo.output"
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "process"
            action: "POST"
            with:
              url: "https://api.example.com/process"

      - name: "stepOne"
        operation: "api_call"
        description: "Step one"
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "run"
            action: "GET"
            with:
              url: "https://api.example.com/one"

      - name: "stepTwo"
        operation: "api_call"
        description: "Step two"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "inputFromOne"
            description: "Input from step one"
            origin: "chat"
            isOptional: true
        steps:
          - name: "run"
            action: "GET"
            with:
              url: "https://api.example.com/two"

      - name: "stepThree"
        operation: "api_call"
        description: "Step three"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "inputFromOne"
            description: "Input from step one"
            origin: "chat"
            isOptional: true
          - name: "inputFromTwo"
            description: "Input from step two"
            origin: "chat"
            isOptional: true
        steps:
          - name: "run"
            action: "GET"
            with:
              url: "https://api.example.com/three"
`,
			wantErr: false,
		},
		{
			name: "Invalid - references sibling that comes AFTER (order matters)",
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
          - name: "firstCallback"
            params:
              dataFromSecond: "$secondCallback.result"
          - name: "secondCallback"
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "execute"
            action: "GET"
            with:
              url: "https://api.example.com"

      - name: "firstCallback"
        operation: "api_call"
        description: "First callback"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "dataFromSecond"
            description: "Data from second callback"
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
			name: "Valid - multiple siblings chaining",
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
          - name: "funcA"
          - name: "funcB"
            params:
              fromA: "$funcA.output"
          - name: "funcC"
            params:
              fromA: "$funcA.output"
              fromB: "$funcB.output"
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "execute"
            action: "GET"
            with:
              url: "https://api.example.com"

      - name: "funcA"
        operation: "api_call"
        description: "Function A"
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "step"
            action: "GET"
            with:
              url: "https://api.example.com/a"

      - name: "funcB"
        operation: "api_call"
        description: "Function B"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "fromA"
            description: "Output from funcA"
            value: "$fromA"
        steps:
          - name: "step"
            action: "GET"
            with:
              url: "https://api.example.com/b"

      - name: "funcC"
        operation: "api_call"
        description: "Function C"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "fromA"
            description: "Output from funcA"
            value: "$fromA"
          - name: "fromB"
            description: "Output from funcB"
            value: "$fromB"
        steps:
          - name: "step"
            action: "GET"
            with:
              url: "https://api.example.com/c"
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

func TestOnSuccessCircularDependencyDetection(t *testing.T) {
	testCases := []struct {
		name     string
		yamlData string
		wantErr  bool
	}{
		{
			name: "Direct circular dependency via onSuccess",
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
        onSuccess: ["funcB"]
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "step"
            action: "GET"
            with:
              url: "https://api.example.com"

      - name: "funcB"
        operation: "api_call"
        description: "Function B"
        onSuccess: ["funcA"]
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "step"
            action: "GET"
            with:
              url: "https://api.example.com"
`,
			wantErr: true,
		},
		{
			name: "Indirect circular dependency via onSuccess",
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
        onSuccess: ["funcB"]
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "step"
            action: "GET"
            with:
              url: "https://api.example.com"

      - name: "funcB"
        operation: "api_call"
        description: "Function B"
        onSuccess: ["funcC"]
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "step"
            action: "GET"
            with:
              url: "https://api.example.com"

      - name: "funcC"
        operation: "api_call"
        description: "Function C"
        onSuccess: ["funcA"]
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "step"
            action: "GET"
            with:
              url: "https://api.example.com"
`,
			wantErr: true,
		},
		{
			name: "No circular dependency with onSuccess",
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
        onSuccess: ["funcB"]
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "step"
            action: "GET"
            with:
              url: "https://api.example.com"

      - name: "funcB"
        operation: "api_call"
        description: "Function B"
        onSuccess: ["funcC"]
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "step"
            action: "GET"
            with:
              url: "https://api.example.com"

      - name: "funcC"
        operation: "api_call"
        description: "Function C"
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "step"
            action: "GET"
            with:
              url: "https://api.example.com"
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
