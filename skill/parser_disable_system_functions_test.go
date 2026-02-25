package skill

import (
	"strings"
	"testing"
)

func TestCreateTool_DisableAllSystemFunctions_Success(t *testing.T) {
	testCases := []struct {
		name       string
		yamlInput  string
		validation func(*testing.T, CustomTool)
	}{
		{
			name: "RunOnlyIf with disableAllSystemFunctions true",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "TestFunction"
        operation: "web_browse"
        description: "A test function"
        successCriteria: "Find the information"
        runOnlyIf:
          condition: "check if deterministic condition is met"
          disableAllSystemFunctions: true
        triggers:
          - type: "always_on_user_message"
        input:
          - name: "query"
            description: "The search query"
            origin: "chat"
            onError:
              strategy: "requestUserInput"
              message: "Please provide a search query"
        steps:
          - name: "open browser"
            action: "open_url"
            with:
              url: "https://example.com"
`,
			validation: func(t *testing.T, tool CustomTool) {
				function := tool.Tools[0].Functions[0]

				runOnlyIfObj, err := ParseRunOnlyIf(function.RunOnlyIf)
				if err != nil {
					t.Errorf("Failed to parse runOnlyIf: %v", err)
				}
				if !runOnlyIfObj.DisableAllSystemFunctions {
					t.Errorf("Expected disableAllSystemFunctions to be true, got false")
				}
				if runOnlyIfObj.Condition != "check if deterministic condition is met" {
					t.Errorf("Expected condition 'check if deterministic condition is met', got %s", runOnlyIfObj.Condition)
				}
			},
		},
		{
			name: "RunOnlyIf with disableAllSystemFunctions false (explicit)",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "TestFunction"
        operation: "web_browse"
        description: "A test function"
        successCriteria: "Find the information"
        runOnlyIf:
          condition: "check permissions"
          disableAllSystemFunctions: false
        triggers:
          - type: "always_on_user_message"
        input:
          - name: "query"
            description: "The search query"
            origin: "chat"
            onError:
              strategy: "requestUserInput"
              message: "Please provide a search query"
        steps:
          - name: "open browser"
            action: "open_url"
            with:
              url: "https://example.com"
`,
			validation: func(t *testing.T, tool CustomTool) {
				function := tool.Tools[0].Functions[0]

				runOnlyIfObj, err := ParseRunOnlyIf(function.RunOnlyIf)
				if err != nil {
					t.Errorf("Failed to parse runOnlyIf: %v", err)
				}
				if runOnlyIfObj.DisableAllSystemFunctions {
					t.Errorf("Expected disableAllSystemFunctions to be false, got true")
				}
			},
		},
		{
			name: "SuccessCriteria with disableAllSystemFunctions true",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "TestFunction"
        operation: "format"
        description: "A test function"
        triggers:
          - type: "always_on_user_message"
        input:
          - name: "userInfo"
            description: "Extract user information"
            origin: "inference"
            successCriteria:
              condition: "Extract from the immediate context only"
              disableAllSystemFunctions: true
            onError:
              strategy: "requestUserInput"
              message: "Cannot extract user info"
        output:
          type: "string"
          value: "processed"
`,
			validation: func(t *testing.T, tool CustomTool) {
				function := tool.Tools[0].Functions[0]
				input := function.Input[0]

				successCriteriaObj, err := ParseSuccessCriteria(input.SuccessCriteria)
				if err != nil {
					t.Errorf("Failed to parse successCriteria: %v", err)
				}
				if !successCriteriaObj.DisableAllSystemFunctions {
					t.Errorf("Expected disableAllSystemFunctions to be true, got false")
				}
				if successCriteriaObj.Condition != "Extract from the immediate context only" {
					t.Errorf("Expected condition 'Extract from the immediate context only', got %s", successCriteriaObj.Condition)
				}
			},
		},
		{
			name: "SuccessCriteria with disableAllSystemFunctions false (explicit)",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "TestFunction"
        operation: "format"
        description: "A test function"
        triggers:
          - type: "always_on_user_message"
        input:
          - name: "userInfo"
            description: "Extract user information"
            origin: "inference"
            successCriteria:
              condition: "Extract user info"
              disableAllSystemFunctions: false
            onError:
              strategy: "requestUserInput"
              message: "Cannot extract user info"
        output:
          type: "string"
          value: "processed"
`,
			validation: func(t *testing.T, tool CustomTool) {
				function := tool.Tools[0].Functions[0]
				input := function.Input[0]

				successCriteriaObj, err := ParseSuccessCriteria(input.SuccessCriteria)
				if err != nil {
					t.Errorf("Failed to parse successCriteria: %v", err)
				}
				if successCriteriaObj.DisableAllSystemFunctions {
					t.Errorf("Expected disableAllSystemFunctions to be false, got true")
				}
			},
		},
		{
			name: "RunOnlyIf with disableAllSystemFunctions true and empty allowedSystemFunctions (disable wins)",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "TestFunction"
        operation: "web_browse"
        description: "A test function"
        successCriteria: "Find the information"
        runOnlyIf:
          condition: "check permissions"
          allowedSystemFunctions: []
          disableAllSystemFunctions: true
        triggers:
          - type: "always_on_user_message"
        input:
          - name: "query"
            description: "The search query"
            origin: "chat"
            onError:
              strategy: "requestUserInput"
              message: "Please provide a search query"
        steps:
          - name: "open browser"
            action: "open_url"
            with:
              url: "https://example.com"
`,
			validation: func(t *testing.T, tool CustomTool) {
				function := tool.Tools[0].Functions[0]

				runOnlyIfObj, err := ParseRunOnlyIf(function.RunOnlyIf)
				if err != nil {
					t.Errorf("Failed to parse runOnlyIf: %v", err)
				}
				if !runOnlyIfObj.DisableAllSystemFunctions {
					t.Errorf("Expected disableAllSystemFunctions to be true, got false")
				}
				if len(runOnlyIfObj.AllowedSystemFunctions) != 0 {
					t.Errorf("Expected empty allowedSystemFunctions, got %v", runOnlyIfObj.AllowedSystemFunctions)
				}
			},
		},
		{
			name: "SuccessCriteria with from field and disableAllSystemFunctions true (valid)",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "getDependency"
        operation: "format"
        description: "Get dependency data"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "data"
            description: "Some data"
            origin: "inference"
            successCriteria: "Get data"
            onError:
              strategy: "requestUserInput"
              message: "Cannot get data"
        output:
          type: "string"
          value: "dependency_result"
      - name: "TestFunction"
        operation: "format"
        description: "A test function"
        needs: ["getDependency"]
        triggers:
          - type: "always_on_user_message"
        input:
          - name: "userInfo"
            description: "Extract user information"
            origin: "inference"
            successCriteria:
              condition: "Extract from dependency context only"
              from: ["getDependency"]
              disableAllSystemFunctions: true
            onError:
              strategy: "requestUserInput"
              message: "Cannot extract user info"
        output:
          type: "string"
          value: "processed"
`,
			validation: func(t *testing.T, tool CustomTool) {
				function := tool.Tools[0].Functions[1] // Second function
				input := function.Input[0]

				successCriteriaObj, err := ParseSuccessCriteria(input.SuccessCriteria)
				if err != nil {
					t.Errorf("Failed to parse successCriteria: %v", err)
				}
				if !successCriteriaObj.DisableAllSystemFunctions {
					t.Errorf("Expected disableAllSystemFunctions to be true, got false")
				}
				if len(successCriteriaObj.From) != 1 {
					t.Errorf("Expected 1 item in From array, got %d", len(successCriteriaObj.From))
				}
				if successCriteriaObj.From[0] != "getDependency" {
					t.Errorf("Expected 'getDependency' in From array, got %s", successCriteriaObj.From[0])
				}
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tool, err := CreateTool(tc.yamlInput)
			if err != nil {
				t.Fatalf("Failed to create tool: %v", err)
			}
			tc.validation(t, tool)
		})
	}
}

func TestCreateTool_DisableAllSystemFunctions_ValidationErrors(t *testing.T) {
	testCases := []struct {
		name          string
		yamlInput     string
		expectedError string
	}{
		{
			name: "RunOnlyIf with disableAllSystemFunctions as string (invalid)",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "TestFunction"
        operation: "web_browse"
        description: "A test function"
        successCriteria: "Find info"
        runOnlyIf:
          condition: "check permissions"
          disableAllSystemFunctions: "true"
        triggers:
          - type: "always_on_user_message"
        input:
          - name: "query"
            description: "The search query"
            origin: "chat"
            onError:
              strategy: "requestUserInput"
              message: "Please provide a search query"
        steps:
          - name: "open browser"
            action: "open_url"
            with:
              url: "https://example.com"
`,
			expectedError: "runOnlyIf.disableAllSystemFunctions must be a boolean",
		},
		{
			name: "RunOnlyIf with both disableAllSystemFunctions=true and allowedSystemFunctions (contradictory)",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "TestFunction"
        operation: "web_browse"
        description: "A test function"
        successCriteria: "Find info"
        runOnlyIf:
          condition: "check permissions"
          disableAllSystemFunctions: true
          allowedSystemFunctions: ["askToContext"]
        triggers:
          - type: "always_on_user_message"
        input:
          - name: "query"
            description: "The search query"
            origin: "chat"
            onError:
              strategy: "requestUserInput"
              message: "Please provide a search query"
        steps:
          - name: "open browser"
            action: "open_url"
            with:
              url: "https://example.com"
`,
			expectedError: "runOnlyIf cannot have both disableAllSystemFunctions=true and allowedSystemFunctions specified",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := CreateTool(tc.yamlInput)
			if err == nil {
				t.Fatalf("Expected error but got none")
			}
			if !strings.Contains(err.Error(), tc.expectedError) {
				t.Errorf("Expected error containing '%s', got '%s'", tc.expectedError, err.Error())
			}
		})
	}
}

func TestCreateTool_DisableAllSystemFunctions_BackwardCompatibility(t *testing.T) {
	// Test that omitting disableAllSystemFunctions field defaults to false
	yamlInput := `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "TestFunction"
        operation: "web_browse"
        description: "A test function"
        successCriteria: "Find the information"
        runOnlyIf:
          condition: "check permissions"
        triggers:
          - type: "always_on_user_message"
        input:
          - name: "query"
            description: "The search query"
            origin: "chat"
            onError:
              strategy: "requestUserInput"
              message: "Please provide a search query"
          - name: "userIntent"
            description: "User intention"
            origin: "inference"
            successCriteria:
              condition: "Determine what the user wants to do"
            onError:
              strategy: "requestUserInput"
              message: "Cannot determine intent"
        steps:
          - name: "open browser"
            action: "open_url"
            with:
              url: "https://example.com"
`

	tool, err := CreateTool(yamlInput)
	if err != nil {
		t.Fatalf("Failed to create tool with backward compatible YAML: %v", err)
	}

	function := tool.Tools[0].Functions[0]

	// Test runOnlyIf backward compatibility
	runOnlyIfObj, err := ParseRunOnlyIf(function.RunOnlyIf)
	if err != nil {
		t.Errorf("Failed to parse runOnlyIf: %v", err)
	}
	if runOnlyIfObj.DisableAllSystemFunctions {
		t.Errorf("Expected disableAllSystemFunctions to be false (default), got true")
	}

	// Test successCriteria backward compatibility
	input := function.Input[1] // Second input (inference one)
	successCriteriaObj, err := ParseSuccessCriteria(input.SuccessCriteria)
	if err != nil {
		t.Errorf("Failed to parse successCriteria: %v", err)
	}
	if successCriteriaObj.DisableAllSystemFunctions {
		t.Errorf("Expected disableAllSystemFunctions to be false (default), got true")
	}
}

func TestParseHelpers_DisableAllSystemFunctions_Contradiction(t *testing.T) {
	t.Run("RunOnlyIf with contradictory settings", func(t *testing.T) {
		input := map[string]interface{}{
			"condition":                 "check something",
			"disableAllSystemFunctions": true,
			"allowedSystemFunctions":    []string{"askToContext"},
		}

		_, err := ParseRunOnlyIf(input)
		if err == nil {
			t.Fatalf("Expected error for contradictory settings, got none")
		}
		if !strings.Contains(err.Error(), "cannot have both disableAllSystemFunctions=true and allowedSystemFunctions specified") {
			t.Errorf("Expected contradiction error, got: %v", err)
		}
	})

	t.Run("SuccessCriteria with contradictory settings", func(t *testing.T) {
		input := map[string]interface{}{
			"condition":                 "extract something",
			"disableAllSystemFunctions": true,
			"allowedSystemFunctions":    []string{"askToContext", "queryMemories"},
		}

		_, err := ParseSuccessCriteria(input)
		if err == nil {
			t.Fatalf("Expected error for contradictory settings, got none")
		}
		if !strings.Contains(err.Error(), "cannot have both disableAllSystemFunctions=true and allowedSystemFunctions specified") {
			t.Errorf("Expected contradiction error, got: %v", err)
		}
	})

	t.Run("RunOnlyIf with disableAllSystemFunctions and empty allowedSystemFunctions is OK", func(t *testing.T) {
		input := map[string]interface{}{
			"condition":                 "check something",
			"disableAllSystemFunctions": true,
			"allowedSystemFunctions":    []string{},
		}

		result, err := ParseRunOnlyIf(input)
		if err != nil {
			t.Fatalf("Should not error with empty allowedSystemFunctions: %v", err)
		}
		if !result.DisableAllSystemFunctions {
			t.Errorf("Expected disableAllSystemFunctions to be true")
		}
	})
}
