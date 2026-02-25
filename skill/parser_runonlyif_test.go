package skill

import (
	"strings"
	"testing"
)

func TestValidateRunOnlyIfVariables(t *testing.T) {
	tests := []struct {
		name          string
		runOnlyIf     string
		function      Function
		envVars       []EnvVar
		expectError   bool
		errorContains string
	}{
		{
			name:      "Valid - system variable USER",
			runOnlyIf: "$USER.preferred_language is Spanish",
			function: Function{
				Name: "TestFunc",
			},
			expectError: false,
		},
		{
			name:      "Valid - system variable COMPANY",
			runOnlyIf: "$COMPANY.industry is technology",
			function: Function{
				Name: "TestFunc",
			},
			expectError: false,
		},
		{
			name:      "Valid - environment variable",
			runOnlyIf: "$API_KEY exists",
			function: Function{
				Name: "TestFunc",
			},
			envVars: []EnvVar{
				{Name: "API_KEY", Value: "test"},
			},
			expectError: false,
		},
		{
			name:      "Invalid - input variable",
			runOnlyIf: "$userType is premium",
			function: Function{
				Name: "TestFunc",
				Input: []Input{
					{Name: "userType", Description: "Type of user"},
				},
			},
			expectError:   true,
			errorContains: "references input 'userType' in runOnlyIf",
		},
		{
			name:      "Invalid - input variable with field",
			runOnlyIf: "$userData.type is premium",
			function: Function{
				Name: "TestFunc",
				Input: []Input{
					{Name: "userData", Description: "User data"},
				},
			},
			expectError:   true,
			errorContains: "references input 'userData' in runOnlyIf",
		},
		{
			name:      "Valid - no variables",
			runOnlyIf: "the user has requested detailed analysis",
			function: Function{
				Name: "TestFunc",
			},
			expectError: false,
		},
		{
			name:      "Valid - mixed system and text",
			runOnlyIf: "process if $NOW is after business hours and user wants it",
			function: Function{
				Name: "TestFunc",
			},
			expectError: false,
		},
		{
			name:      "Invalid - multiple inputs referenced",
			runOnlyIf: "process if $category is electronics and $price is low",
			function: Function{
				Name: "TestFunc",
				Input: []Input{
					{Name: "category", Description: "Product category"},
					{Name: "price", Description: "Product price"},
				},
			},
			expectError:   true,
			errorContains: "references input 'category' in runOnlyIf",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateRunOnlyIfVariables(tt.runOnlyIf, tt.function, "TestTool", tt.envVars)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				} else if tt.errorContains != "" && !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("Expected error containing '%s', got '%s'", tt.errorContains, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error but got: %v", err)
				}
			}
		})
	}
}

func TestRunOnlyIfValidationIntegration(t *testing.T) {
	yamlDef := `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "Test tool"
    version: "1.0.0"
    functions:
      - name: "TestFunction"
        operation: "api_call"
        description: "Test function"
        runOnlyIf: "$userInput is valid"
        triggers:
          - type: "always_on_user_message"
        input:
          - name: "userInput"
            description: "User input"
            origin: "chat"
            onError:
              strategy: "requestUserInput"
              message: "Please provide input"
        steps:
          - name: "test step"
            action: "GET"
            with:
              url: "https://example.com"
`

	_, err := CreateTool(yamlDef)

	if err == nil {
		t.Error("Expected validation error for input reference in runOnlyIf")
	}

	if !strings.Contains(err.Error(), "runOnlyIf") {
		t.Errorf("Expected error to mention runOnlyIf, got: %v", err)
	}

	if !strings.Contains(err.Error(), "userInput") {
		t.Errorf("Expected error to mention the input name 'userInput', got: %v", err)
	}
}

func TestRunOnlyIfWithFormatOperationWorkaround(t *testing.T) {
	yamlDef := `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "Test tool"
    version: "1.0.0"
    functions:
      - name: "checkConditions"
        operation: "format"
        description: "Check conditions"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "userType"
            description: "Type of user"
            origin: "inference"
            successCriteria: "Determine user type"
            onError:
              strategy: "requestUserInput"
              message: "What type of user?"
        output:
          type: "object"
          fields:
            - "shouldProceed"
            - "userType"
      
      - name: "ProcessData"
        operation: "api_call"
        description: "Process data conditionally"
        needs: ["checkConditions"]
        runOnlyIf: "the context contains shouldProceed=true"
        triggers:
          - type: "always_on_user_message"
        steps:
          - name: "process"
            action: "GET"
            with:
              url: "https://example.com/process"
`

	tool, err := CreateTool(yamlDef)

	if err != nil {
		t.Fatalf("Expected no error for valid workaround pattern, got: %v", err)
	}

	if len(tool.Tools) != 1 {
		t.Errorf("Expected 1 tool, got %d", len(tool.Tools))
	}

	if len(tool.Tools[0].Functions) != 2 {
		t.Errorf("Expected 2 functions, got %d", len(tool.Tools[0].Functions))
	}

	processFunc := tool.Tools[0].Functions[1]
	if processFunc.RunOnlyIf != "the context contains shouldProceed=true" {
		t.Errorf("Expected runOnlyIf to be preserved, got: %s", processFunc.RunOnlyIf)
	}

	if len(processFunc.Needs) != 1 || processFunc.Needs[0].Name != "checkConditions" {
		t.Errorf("Expected needs to contain checkConditions")
	}
}

func TestRunOnlyIfAndSuccessCriteriaRequired(t *testing.T) {
	yamlDef := `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "Test tool"
    version: "1.0.0"
    functions:
      - name: "TestFunction"
        operation: "web_browse"
        description: "Test function with both runOnlyIf and successCriteria"
        runOnlyIf: "$USER.preferred_language is Spanish"
        successCriteria: "Successfully extracted the product information"
        triggers:
          - type: "always_on_user_message"
        input:
          - name: "searchQuery"
            description: "What to search for"
            origin: "chat"
            onError:
              strategy: "requestUserInput"
              message: "What would you like to search?"
        steps:
          - name: "open site"
            action: "open_url"
            with:
              url: "https://example.com/search?q=$searchQuery"
          - name: "extract results"
            action: "extract_text"
            goal: "Extract search results"
            with:
              findBy: "semantic_context"
              findValue: "search results"
`

	tool, err := CreateTool(yamlDef)

	if err != nil {
		t.Fatalf("Expected no error for function with both runOnlyIf and successCriteria, got: %v", err)
	}

	function := tool.Tools[0].Functions[0]

	if GetRunOnlyIfCondition(function.RunOnlyIf) == "" {
		t.Error("Expected runOnlyIf to be defined")
	}

	if GetSuccessCriteriaCondition(function.SuccessCriteria) == "" {
		t.Error("Expected successCriteria to be defined")
	}

	if function.RunOnlyIf != "$USER.preferred_language is Spanish" {
		t.Errorf("Expected runOnlyIf to match, got: %s", function.RunOnlyIf)
	}

	if function.SuccessCriteria != "Successfully extracted the product information" {
		t.Errorf("Expected successCriteria to match, got: %s", function.SuccessCriteria)
	}
}
