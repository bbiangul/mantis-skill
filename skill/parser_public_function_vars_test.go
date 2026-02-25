package skill

import (
	"strings"
	"testing"
)

func TestPublicFunctionWithVariableInputs(t *testing.T) {
	testCases := []struct {
		name            string
		yamlInput       string
		expectWarning   bool
		warningContains string
	}{
		{
			name: "Warning: Public function with $variable in input value",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "HandleRejectionGracefully"
        operation: "terminal"
        description: "Handle when lead explicitly says no - leave door open"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "companyName"
            description: "Lead's company name"
            value: "$companyName"
          - name: "dealId"
            description: "Deal ID"
            value: "$dealId"
        steps:
          - name: "handle"
            action: "bash"
            with:
              linux: |
                echo "Handling rejection for $companyName, deal $dealId"
              windows: |
                @echo off
                echo Handling rejection for %companyName%, deal %dealId%
`,
			expectWarning:   true,
			warningContains: "public function 'HandleRejectionGracefully' has input 'companyName' with workflow variable reference",
		},
		{
			name: "Warning: Public function with single $variable in input value",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "ProcessLead"
        operation: "terminal"
        description: "Process a lead"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "leadId"
            description: "The lead ID to process"
            value: "$leadId"
        steps:
          - name: "process"
            action: "bash"
            with:
              linux: |
                echo "Processing lead $leadId"
              windows: |
                @echo off
                echo Processing lead %leadId%
`,
			expectWarning:   true,
			warningContains: "public function 'ProcessLead' has input 'leadId'",
		},
		{
			name: "No warning: Private function with $variable in input value",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "PublicEntry"
        operation: "terminal"
        description: "Public entry point"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "companyName"
            description: "Company name"
            origin: "chat"
            onError:
              strategy: "requestUserInput"
              message: "Please provide company name"
        onSuccess:
          - name: "handleRejection"
            params:
              companyName: "$companyName"
        steps:
          - name: "entry"
            action: "bash"
            with:
              linux: |
                echo "Entry point for $companyName"
              windows: |
                @echo off
                echo Entry point for %companyName%
      - name: "handleRejection"
        operation: "terminal"
        description: "Handle rejection - private function called from workflow"
        input:
          - name: "companyName"
            description: "Lead's company name"
            value: "$companyName"
        steps:
          - name: "handle"
            action: "bash"
            with:
              linux: |
                echo "Handling rejection for $companyName"
              windows: |
                @echo off
                echo Handling rejection for %companyName%
`,
			expectWarning: false,
		},
		{
			name: "No warning: Public function with static value in input",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "GetDefaultConfig"
        operation: "terminal"
        description: "Get default configuration"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "configType"
            description: "Type of config to get"
            value: "default"
        steps:
          - name: "get"
            action: "bash"
            with:
              linux: |
                echo "Getting config: $configType"
              windows: |
                @echo off
                echo Getting config: %configType%
`,
			expectWarning: false,
		},
		{
			name: "No warning: Public function with chat origin input",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "ProcessUserInput"
        operation: "terminal"
        description: "Process user input"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "userInput"
            description: "Input from user"
            origin: "chat"
            onError:
              strategy: "requestUserInput"
              message: "Please provide input"
        steps:
          - name: "process"
            action: "bash"
            with:
              linux: |
                echo "Processing: $userInput"
              windows: |
                @echo off
                echo Processing: %userInput%
`,
			expectWarning: false,
		},
		{
			name: "No warning: Public function with inference origin input",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "AnalyzeMessage"
        operation: "format"
        description: "Analyze a message"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "sentiment"
            description: "Message sentiment"
            origin: "inference"
            successCriteria:
              condition: "Determine the sentiment of the user's message"
              allowedSystemFunctions: ["askToContext"]
            onError:
              strategy: "requestN1Support"
              message: "Failed to analyze sentiment"
`,
			expectWarning: false,
		},
		{
			name: "Warning: Public function with $var reference",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "FormatMessage"
        operation: "terminal"
        description: "Format a message"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "message"
            description: "Message to format"
            value: "$userMessage"
        steps:
          - name: "format"
            action: "bash"
            with:
              linux: |
                echo "Message: $message"
              windows: |
                @echo off
                echo Message: %message%
`,
			expectWarning:   true,
			warningContains: "public function 'FormatMessage' has input 'message' with workflow variable reference '$userMessage'",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := CreateToolWithWarnings(tc.yamlInput)

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			// Check for the specific warning about public functions with workflow vars
			hasTargetWarning := false
			for _, w := range result.Warnings {
				if strings.Contains(w, "public function") && strings.Contains(w, "workflow variable reference") {
					hasTargetWarning = true
					if tc.warningContains != "" && !strings.Contains(w, tc.warningContains) {
						t.Errorf("Warning does not contain expected text '%s', got: %s", tc.warningContains, w)
					}
					break
				}
			}

			if tc.expectWarning && !hasTargetWarning {
				t.Errorf("Expected warning about public function with workflow variable but got none. Warnings: %v", result.Warnings)
			}
			if !tc.expectWarning && hasTargetWarning {
				t.Errorf("Did not expect warning about public function with workflow variable but got one. Warnings: %v", result.Warnings)
			}
		})
	}
}

func TestPublicFunctionVarWarningMessageContent(t *testing.T) {
	yamlInput := `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "HandleRejection"
        operation: "terminal"
        description: "Handle rejection"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "companyName"
            description: "Company name"
            value: "$companyName"
        steps:
          - name: "handle"
            action: "bash"
            with:
              linux: |
                echo "$companyName"
              windows: |
                @echo off
                echo %companyName%
`

	result, err := CreateToolWithWarnings(yamlInput)

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Find the warning about public function
	var targetWarning string
	for _, w := range result.Warnings {
		if strings.Contains(w, "public function") && strings.Contains(w, "workflow variable reference") {
			targetWarning = w
			break
		}
	}

	if targetWarning == "" {
		t.Fatalf("Expected warning about public function with workflow variable but got none. Warnings: %v", result.Warnings)
	}

	// Verify the warning message contains all the important parts
	expectedParts := []string{
		"public function 'HandleRejection'",
		"input 'companyName'",
		"workflow variable reference '$companyName'",
		"agentic loop",
		"initiate_workflow",
		"context.params",
		"make the function private",
	}

	for _, part := range expectedParts {
		if !strings.Contains(targetWarning, part) {
			t.Errorf("Expected warning message to contain '%s', but got: %s", part, targetWarning)
		}
	}
}

func TestPublicFunctionMultipleWarnings(t *testing.T) {
	// Test that multiple inputs with workflow vars generate multiple warnings
	yamlInput := `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "ProcessDeal"
        operation: "terminal"
        description: "Process a deal"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "companyName"
            description: "Company name"
            value: "$companyName"
          - name: "dealId"
            description: "Deal ID"
            value: "$dealId"
          - name: "staticValue"
            description: "Static value - no warning"
            value: "static"
        steps:
          - name: "process"
            action: "bash"
            with:
              linux: |
                echo "Processing $companyName, $dealId"
              windows: |
                @echo off
                echo Processing
`

	result, err := CreateToolWithWarnings(yamlInput)

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Count warnings about public function workflow vars
	workflowVarWarnings := 0
	for _, w := range result.Warnings {
		if strings.Contains(w, "public function") && strings.Contains(w, "workflow variable reference") {
			workflowVarWarnings++
		}
	}

	// Should have warnings for companyName and dealId, but not staticValue
	if workflowVarWarnings != 2 {
		t.Errorf("Expected 2 warnings for workflow variable inputs, got %d. Warnings: %v", workflowVarWarnings, result.Warnings)
	}
}
