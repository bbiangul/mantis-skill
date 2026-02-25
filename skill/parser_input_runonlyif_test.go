package skill

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInputRunOnlyIfValidation(t *testing.T) {
	testCases := []struct {
		name          string
		yamlData      string
		wantErr       bool
		errorContains string
	}{
		{
			name: "Valid - input runOnlyIf referencing previous input",
			yamlData: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "Tool for testing"
    version: "1.0.0"
    functions:
      - name: "testFunc"
        operation: "api_call"
        description: "Test function"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "hasCompletePreference"
            description: "Has complete preference flag"
            origin: "chat"
            onError:
              strategy: "requestUserInput"
              message: "Does user have complete preference?"
          - name: "suggestionMessage"
            description: "Suggestion message"
            isOptional: true
            origin: "inference"
            successCriteria: "a non-empty message string"
            runOnlyIf:
              deterministic: "$hasCompletePreference == false"
        steps:
          - name: "fetch"
            action: "GET"
            with:
              url: "https://api.example.com"
`,
			wantErr: false,
		},
		{
			name: "Valid - input runOnlyIf referencing needs function",
			yamlData: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "Tool for testing"
    version: "1.0.0"
    functions:
      - name: "checkPreference"
        operation: "api_call"
        description: "Check preference"
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "check"
            action: "GET"
            with:
              url: "https://api.example.com/check"

      - name: "testFunc"
        operation: "api_call"
        description: "Test function"
        needs: ["checkPreference"]
        triggers:
          - type: "flex_for_user"
        input:
          - name: "hasCompletePreferenceFlag"
            description: "Flag from dependency"
            isOptional: true
            origin: "function"
            from: "checkPreference"
          - name: "suggestionMessage"
            description: "Suggestion message"
            isOptional: true
            origin: "inference"
            successCriteria: "a valid suggestion message"
            runOnlyIf:
              deterministic: "$hasCompletePreferenceFlag == false"
        steps:
          - name: "fetch"
            action: "GET"
            with:
              url: "https://api.example.com"
`,
			wantErr: false,
		},
		{
			name: "Invalid - input runOnlyIf with condition (inference) not allowed",
			yamlData: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "Tool for testing"
    version: "1.0.0"
    functions:
      - name: "testFunc"
        operation: "api_call"
        description: "Test function"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "checkValue"
            description: "Some value"
            origin: "chat"
            onError:
              strategy: "requestUserInput"
              message: "Please provide value"
          - name: "conditionalInput"
            description: "Conditional input"
            isOptional: true
            origin: "inference"
            successCriteria: "some criteria"
            runOnlyIf:
              condition: "is the user happy?"
        steps:
          - name: "fetch"
            action: "GET"
            with:
              url: "https://api.example.com"
`,
			wantErr:       true,
			errorContains: "Input-level runOnlyIf only supports deterministic evaluation",
		},
		{
			name: "Invalid - input runOnlyIf with inference not allowed",
			yamlData: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "Tool for testing"
    version: "1.0.0"
    functions:
      - name: "testFunc"
        operation: "api_call"
        description: "Test function"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "checkValue"
            description: "Some value"
            origin: "chat"
            onError:
              strategy: "requestUserInput"
              message: "Please provide value"
          - name: "conditionalInput"
            description: "Conditional input"
            isOptional: true
            origin: "inference"
            successCriteria: "some criteria"
            runOnlyIf:
              inference:
                condition: "is the user happy?"
        steps:
          - name: "fetch"
            action: "GET"
            with:
              url: "https://api.example.com"
`,
			wantErr:       true,
			errorContains: "Input-level runOnlyIf only supports deterministic evaluation",
		},
		{
			name: "Invalid - input runOnlyIf references later input (forward reference)",
			yamlData: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "Tool for testing"
    version: "1.0.0"
    functions:
      - name: "testFunc"
        operation: "api_call"
        description: "Test function"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "firstInput"
            description: "First input"
            isOptional: true
            origin: "inference"
            successCriteria: "some criteria"
            runOnlyIf:
              deterministic: "$secondInput == true"
          - name: "secondInput"
            description: "Second input"
            origin: "chat"
            onError:
              strategy: "requestUserInput"
              message: "Please provide value"
        steps:
          - name: "fetch"
            action: "GET"
            with:
              url: "https://api.example.com"
`,
			wantErr:       true,
			errorContains: "runOnlyIf that references '$secondInput' which is defined later in the input list",
		},
		{
			name: "Invalid - input runOnlyIf references unknown variable",
			yamlData: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "Tool for testing"
    version: "1.0.0"
    functions:
      - name: "testFunc"
        operation: "api_call"
        description: "Test function"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "conditionalInput"
            description: "Conditional input"
            isOptional: true
            origin: "inference"
            successCriteria: "some criteria"
            runOnlyIf:
              deterministic: "$unknownVariable == true"
        steps:
          - name: "fetch"
            action: "GET"
            with:
              url: "https://api.example.com"
`,
			wantErr:       true,
			errorContains: "runOnlyIf that references unknown variable '$unknownVariable'",
		},
		{
			name: "Invalid - input runOnlyIf empty deterministic",
			yamlData: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "Tool for testing"
    version: "1.0.0"
    functions:
      - name: "testFunc"
        operation: "api_call"
        description: "Test function"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "checkValue"
            description: "Some value"
            origin: "chat"
            onError:
              strategy: "requestUserInput"
              message: "Please provide value"
          - name: "conditionalInput"
            description: "Conditional input"
            isOptional: true
            origin: "inference"
            successCriteria: "some criteria"
            runOnlyIf:
              deterministic: ""
        steps:
          - name: "fetch"
            action: "GET"
            with:
              url: "https://api.example.com"
`,
			wantErr:       true,
			errorContains: "runOnlyIf must specify at least one of",
		},
		{
			name: "Valid - input runOnlyIf with system variable",
			yamlData: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "Tool for testing"
    version: "1.0.0"
    functions:
      - name: "testFunc"
        operation: "api_call"
        description: "Test function"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "conditionalInput"
            description: "Conditional input"
            isOptional: true
            origin: "inference"
            successCriteria: "some criteria"
            runOnlyIf:
              deterministic: "$NOW != ''"
        steps:
          - name: "fetch"
            action: "GET"
            with:
              url: "https://api.example.com"
`,
			wantErr: false,
		},
		{
			name: "Valid - input runOnlyIf referencing direct dependency",
			yamlData: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "Tool for testing"
    version: "1.0.0"
    functions:
      - name: "baseFunc"
        operation: "api_call"
        description: "Base function"
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "base"
            action: "GET"
            with:
              url: "https://api.example.com/base"

      - name: "middleFunc"
        operation: "api_call"
        description: "Middle function"
        needs: ["baseFunc"]
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "middle"
            action: "GET"
            with:
              url: "https://api.example.com/middle"

      - name: "testFunc"
        operation: "api_call"
        description: "Test function"
        needs: ["middleFunc"]
        triggers:
          - type: "flex_for_user"
        input:
          - name: "conditionalInput"
            description: "Conditional input"
            isOptional: true
            origin: "inference"
            successCriteria: "some criteria"
            runOnlyIf:
              deterministic: "$middleFunc != null"
        steps:
          - name: "fetch"
            action: "GET"
            with:
              url: "https://api.example.com"
`,
			wantErr: false,
		},
		{
			name: "Valid - input runOnlyIf with multiple previous inputs",
			yamlData: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "Tool for testing"
    version: "1.0.0"
    functions:
      - name: "testFunc"
        operation: "api_call"
        description: "Test function"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "flagA"
            description: "Flag A"
            origin: "chat"
            onError:
              strategy: "requestUserInput"
              message: "Provide flag A"
          - name: "flagB"
            description: "Flag B"
            origin: "chat"
            onError:
              strategy: "requestUserInput"
              message: "Provide flag B"
          - name: "conditionalInput"
            description: "Conditional input"
            isOptional: true
            origin: "inference"
            successCriteria: "some criteria"
            runOnlyIf:
              deterministic: "$flagA == true && $flagB == false"
        steps:
          - name: "fetch"
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
				assert.Error(t, err, "Expected an error for test case: %s", tc.name)
				if tc.errorContains != "" {
					assert.True(t, strings.Contains(err.Error(), tc.errorContains),
						"Error should contain '%s', but got: %s", tc.errorContains, err.Error())
				}
			} else {
				assert.NoError(t, err, "Expected no error for test case: %s", tc.name)
			}
		})
	}
}
