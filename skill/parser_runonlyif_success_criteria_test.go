package skill

import (
	"fmt"
	"strings"
	"testing"
)

func TestCreateTool_RunOnlyIf_Success(t *testing.T) {
	testCases := []struct {
		name       string
		yamlInput  string
		validation func(*testing.T, CustomTool)
	}{
		{
			name: "RunOnlyIf as string (backward compatibility)",
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
        runOnlyIf: "user has admin privileges"
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
				if len(tool.Tools) != 1 {
					t.Errorf("Expected 1 tool, got %d", len(tool.Tools))
				}
				function := tool.Tools[0].Functions[0]

				// Test that runOnlyIf is stored as interface{} but can be parsed
				runOnlyIfObj, err := ParseRunOnlyIf(function.RunOnlyIf)
				if err != nil {
					t.Errorf("Failed to parse runOnlyIf: %v", err)
				}
				if runOnlyIfObj.Condition != "user has admin privileges" {
					t.Errorf("Expected condition 'user has admin privileges', got %s", runOnlyIfObj.Condition)
				}
				if len(runOnlyIfObj.From) != 0 {
					t.Errorf("Expected empty From array, got %v", runOnlyIfObj.From)
				}
				if runOnlyIfObj.OnError != nil {
					t.Errorf("Expected nil OnError, got %v", runOnlyIfObj.OnError)
				}
			},
		},
		{
			name: "RunOnlyIf as object with condition only",
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
          condition: "check if user is authenticated"
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
				if runOnlyIfObj.Condition != "check if user is authenticated" {
					t.Errorf("Expected condition 'check if user is authenticated', got %s", runOnlyIfObj.Condition)
				}
			},
		},
		{
			name: "RunOnlyIf as object with condition and from",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "checkAuth"
        operation: "format"
        description: "Check authentication"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "token"
            description: "Auth token"
            origin: "inference"
            successCriteria: "Extract token from context"
            onError:
              strategy: "requestUserInput"
              message: "Cannot find auth token"
        output:
          type: "string"
          value: "authenticated"
      - name: "TestFunction"
        operation: "web_browse"
        description: "A test function"
        successCriteria: "Find the information"
        needs: ["checkAuth"]
        runOnlyIf:
          condition: "check if user has valid permissions"
          from: ["checkAuth"]
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
				function := tool.Tools[0].Functions[1] // Second function

				runOnlyIfObj, err := ParseRunOnlyIf(function.RunOnlyIf)
				if err != nil {
					t.Errorf("Failed to parse runOnlyIf: %v", err)
				}
				if runOnlyIfObj.Condition != "check if user has valid permissions" {
					t.Errorf("Expected condition 'check if user has valid permissions', got %s", runOnlyIfObj.Condition)
				}
				if len(runOnlyIfObj.From) != 1 {
					t.Errorf("Expected 1 item in From array, got %d", len(runOnlyIfObj.From))
				}
				if runOnlyIfObj.From[0] != "checkAuth" {
					t.Errorf("Expected 'checkAuth' in From array, got %s", runOnlyIfObj.From[0])
				}
			},
		},
		{
			name: "RunOnlyIf as object with onError",
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
          condition: "verify user authorization"
          onError:
            strategy: "requestUserInput"
            message: "Cannot verify authorization. Continue anyway?"
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
				if runOnlyIfObj.Condition != "verify user authorization" {
					t.Errorf("Expected condition 'verify user authorization', got %s", runOnlyIfObj.Condition)
				}
				if runOnlyIfObj.OnError == nil {
					t.Errorf("Expected OnError to be non-nil")
				} else {
					if runOnlyIfObj.OnError.Strategy != "requestUserInput" {
						t.Errorf("Expected OnError strategy 'requestUserInput', got %s", runOnlyIfObj.OnError.Strategy)
					}
					if runOnlyIfObj.OnError.Message != "Cannot verify authorization. Continue anyway?" {
						t.Errorf("Expected OnError message 'Cannot verify authorization. Continue anyway?', got %s", runOnlyIfObj.OnError.Message)
					}
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

func TestCreateTool_SuccessCriteria_Success(t *testing.T) {
	testCases := []struct {
		name       string
		yamlInput  string
		validation func(*testing.T, CustomTool)
	}{
		{
			name: "SuccessCriteria as string (backward compatibility)",
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
            successCriteria: "Extract user name and email from the conversation"
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
				if successCriteriaObj.Condition != "Extract user name and email from the conversation" {
					t.Errorf("Expected condition 'Extract user name and email from the conversation', got %s", successCriteriaObj.Condition)
				}
				if len(successCriteriaObj.From) != 0 {
					t.Errorf("Expected empty From array, got %v", successCriteriaObj.From)
				}
			},
		},
		{
			name: "SuccessCriteria as object with condition only",
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
              condition: "Extract detailed user profile information"
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
				if successCriteriaObj.Condition != "Extract detailed user profile information" {
					t.Errorf("Expected condition 'Extract detailed user profile information', got %s", successCriteriaObj.Condition)
				}
			},
		},
		{
			name: "SuccessCriteria as object with condition and from",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "getUserData"
        operation: "format"
        description: "Get user data"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "userId"
            description: "User ID"
            origin: "inference"
            successCriteria: "Extract user ID"
            onError:
              strategy: "requestUserInput"
              message: "Cannot find user ID"
        output:
          type: "string"
          value: "user_data"
      - name: "enrichUser"
        operation: "format"
        description: "Enrich user data"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "basicData"
            description: "Basic user data"
            origin: "inference"
            successCriteria: "Get basic data"
            onError:
              strategy: "requestUserInput"
              message: "Cannot get basic data"
        output:
          type: "string"
          value: "enriched_data"
      - name: "TestFunction"
        operation: "format"
        description: "A test function"
        needs: ["getUserData", "enrichUser"]
        triggers:
          - type: "always_on_user_message"
        input:
          - name: "userInfo"
            description: "Extract comprehensive user information"
            origin: "inference"
            successCriteria:
              condition: "Extract user profile with contact details and preferences"
              from: ["getUserData", "enrichUser"]
            onError:
              strategy: "requestUserInput"
              message: "Cannot extract user info"
        output:
          type: "string"
          value: "processed"
`,
			validation: func(t *testing.T, tool CustomTool) {
				function := tool.Tools[0].Functions[2] // Third function
				input := function.Input[0]

				successCriteriaObj, err := ParseSuccessCriteria(input.SuccessCriteria)
				if err != nil {
					t.Errorf("Failed to parse successCriteria: %v", err)
				}
				if successCriteriaObj.Condition != "Extract user profile with contact details and preferences" {
					t.Errorf("Expected condition 'Extract user profile with contact details and preferences', got %s", successCriteriaObj.Condition)
				}
				if len(successCriteriaObj.From) != 2 {
					t.Errorf("Expected 2 items in From array, got %d", len(successCriteriaObj.From))
				}
				expectedFrom := []string{"getUserData", "enrichUser"}
				for i, expected := range expectedFrom {
					if successCriteriaObj.From[i] != expected {
						t.Errorf("Expected '%s' at index %d in From array, got %s", expected, i, successCriteriaObj.From[i])
					}
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

func TestCreateTool_RunOnlyIf_ValidationErrors(t *testing.T) {
	testCases := []struct {
		name          string
		yamlInput     string
		expectedError string // empty string means test should pass
	}{
		{
			name: "RunOnlyIf object without condition",
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
          from: ["someFunction"]
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
			expectedError: "runOnlyIf object must have a 'condition' field",
		},
		{
			name: "RunOnlyIf with invalid function in from",
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
          from: ["nonExistentFunction"]
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
			expectedError: "not transitively reachable through the dependency chain",
		},
		{
			name: "RunOnlyIf with self-reference in from",
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
          from: ["TestFunction"]
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
			expectedError: "function cannot reference itself in runOnlyIf.from",
		},
		{
			name: "RunOnlyIf with invalid onError strategy",
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
          onError:
            strategy: "invalidStrategy"
            message: "Error message"
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
			expectedError: "invalid onError strategy: invalidStrategy",
		},
		{
			name: "RunOnlyIf with system function in from",
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
          from: ["Ask", "Learn"]
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
			expectedError: "", // Should pass - system functions are valid
		},
		{
			name: "RunOnlyIf with needs dependency in from",
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
        operation: "web_browse"
        description: "A test function"
        successCriteria: "Find info"
        needs: ["getDependency"]
        runOnlyIf:
          condition: "check permissions"
          from: ["getDependency"]
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
			expectedError: "", // Should pass - needs dependency is valid
		},
		{
			name: "RunOnlyIf with empty from array",
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
          from: []
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
			expectedError: "", // Should pass - empty from array is valid
		},
		{
			name: "RunOnlyIf with mixed valid and invalid functions in from",
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
          from: ["Ask", "nonExistentFunction"]
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
			expectedError: "not transitively reachable through the dependency chain",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := CreateTool(tc.yamlInput)
			if tc.expectedError == "" {
				// This should pass
				if err != nil {
					t.Fatalf("Expected no error but got: %v", err)
				}
			} else {
				// This should fail with the expected error
				if err == nil {
					t.Fatalf("Expected error but got none")
				}
				if !strings.Contains(err.Error(), tc.expectedError) {
					t.Errorf("Expected error containing '%s', got '%s'", tc.expectedError, err.Error())
				}
			}
		})
	}
}

func TestCreateTool_SuccessCriteria_ValidationErrors(t *testing.T) {
	testCases := []struct {
		name          string
		yamlInput     string
		expectedError string // empty string means test should pass
	}{
		{
			name: "SuccessCriteria object without condition",
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
              from: ["someFunction"]
        output:
          type: "string"
          value: "processed"
`,
			expectedError: "must have a success criteria",
		},
		{
			name: "SuccessCriteria with invalid function in from",
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
              condition: "extract info"
              from: ["nonExistentFunction"]
        output:
          type: "string"
          value: "processed"
`,
			expectedError: "not transitively reachable through the dependency chain",
		},
		{
			name: "SuccessCriteria with self-reference in from",
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
              condition: "extract info"
              from: ["TestFunction"]
        output:
          type: "string"
          value: "processed"
`,
			expectedError: "function cannot reference itself in successCriteria.from",
		},
		{
			name: "SuccessCriteria with system function in from",
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
              condition: "extract info"
              from: ["AskHuman", "NotifyHuman"]
            onError:
              strategy: "requestUserInput"
              message: "Cannot extract user info"
        output:
          type: "string"
          value: "processed"
`,
			expectedError: "", // Should pass - system functions are valid
		},
		{
			name: "SuccessCriteria with needs dependency in from",
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
              condition: "extract info"
              from: ["getDependency"]
            onError:
              strategy: "requestUserInput"
              message: "Cannot extract user info"
        output:
          type: "string"
          value: "processed"
`,
			expectedError: "", // Should pass - needs dependency is valid
		},
		{
			name: "SuccessCriteria with empty from array",
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
              condition: "extract info"
              from: []
            onError:
              strategy: "requestUserInput"
              message: "Cannot extract user info"
        output:
          type: "string"
          value: "processed"
`,
			expectedError: "", // Should pass - empty from array is valid
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := CreateTool(tc.yamlInput)
			if tc.expectedError == "" {
				// This should pass
				if err != nil {
					t.Fatalf("Expected no error but got: %v", err)
				}
			} else {
				// This should fail with the expected error
				if err == nil {
					t.Fatalf("Expected error but got none")
				}
				if !strings.Contains(err.Error(), tc.expectedError) {
					t.Errorf("Expected error containing '%s', got '%s'", tc.expectedError, err.Error())
				}
			}
		})
	}
}

func TestCreateTool_AllValidOnErrorStrategies(t *testing.T) {
	validStrategies := []string{
		"requestUserInput",
		"requestN1Support",
		"requestN2Support",
		"requestN3Support",
		"requestApplicationSupport",
		"search",
		"inference",
	}

	for _, strategy := range validStrategies {
		t.Run("RunOnlyIf with "+strategy+" strategy", func(t *testing.T) {
			yamlInput := fmt.Sprintf(`
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
          onError:
            strategy: "%s"
            message: "Error with %s strategy"
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
`, strategy, strategy)

			_, err := CreateTool(yamlInput)
			if err != nil {
				t.Fatalf("Strategy '%s' should be valid but got error: %v", strategy, err)
			}
		})
	}
}

func TestCreateTool_InvalidInputTypes(t *testing.T) {
	testCases := []struct {
		name          string
		yamlInput     string
		expectedError string
	}{
		{
			name: "RunOnlyIf as number",
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
        runOnlyIf: 123
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
			expectedError: "runOnlyIf must be a string or object",
		},
		{
			name: "SuccessCriteria as boolean",
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
            successCriteria: true
            onError:
              strategy: "requestUserInput"
              message: "Cannot extract user info"
        output:
          type: "string"
          value: "processed"
`,
			expectedError: "must have a success criteria",
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

func TestCreateTool_TransitiveDependencyValidation(t *testing.T) {
	testCases := []struct {
		name          string
		yamlInput     string
		expectedError string // empty means should pass
	}{
		{
			name: "Valid direct dependency in from",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "auth"
        operation: "format"
        description: "Authenticate user"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "token"
            description: "Auth token"
            origin: "inference"
            successCriteria: "Extract token"
            onError:
              strategy: "requestUserInput"
              message: "Cannot find token"
        output:
          type: "string"
          value: "authenticated"
      - name: "processData"
        operation: "format"
        description: "Process data"
        needs: ["auth"]
        runOnlyIf:
          condition: "check if user is authenticated"
          from: ["auth"]
        triggers:
          - type: "always_on_user_message"
        input:
          - name: "data"
            description: "Process data"
            origin: "inference"
            successCriteria: "Get data"
            onError:
              strategy: "requestUserInput"
              message: "Cannot get data"
        output:
          type: "string"
          value: "processed"
`,
			expectedError: "", // Should pass - direct dependency
		},
		{
			name: "Valid transitive dependency in from (2 levels)",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "auth"
        operation: "format"
        description: "Authenticate user"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "token"
            description: "Auth token"
            origin: "inference"
            successCriteria: "Extract token"
            onError:
              strategy: "requestUserInput"
              message: "Cannot find token"
        output:
          type: "string"
          value: "authenticated"
      - name: "getProfile"
        operation: "format"
        description: "Get user profile"
        needs: ["auth"]
        triggers:
          - type: "flex_for_user"
        input:
          - name: "userId"
            description: "User ID"
            origin: "inference"
            successCriteria: "Extract user ID"
            onError:
              strategy: "requestUserInput"
              message: "Cannot find user ID"
        output:
          type: "string"
          value: "profile_data"
      - name: "processData"
        operation: "format"
        description: "Process data"
        needs: ["getProfile"]
        runOnlyIf:
          condition: "check if user has valid auth and profile"
          from: ["auth", "getProfile"]  # auth is transitive dependency through getProfile
        triggers:
          - type: "always_on_user_message"
        input:
          - name: "data"
            description: "Process data"
            origin: "inference"
            successCriteria:
              condition: "Combine auth and profile data"
              from: ["auth", "getProfile"]  # Both should be valid
            onError:
              strategy: "requestUserInput"
              message: "Cannot get data"
        output:
          type: "string"
          value: "processed"
`,
			expectedError: "", // Should pass - transitive dependency
		},
		{
			name: "Valid transitive dependency in from (3 levels)",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "auth"
        operation: "format"
        description: "Authenticate user"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "token"
            description: "Auth token"
            origin: "inference"
            successCriteria: "Extract token"
            onError:
              strategy: "requestUserInput"
              message: "Cannot find token"
        output:
          type: "string"
          value: "authenticated"
      - name: "getProfile"
        operation: "format"
        description: "Get user profile"
        needs: ["auth"]
        triggers:
          - type: "flex_for_user"
        input:
          - name: "userId"
            description: "User ID"
            origin: "inference"
            successCriteria: "Extract user ID"
            onError:
              strategy: "requestUserInput"
              message: "Cannot find user ID"
        output:
          type: "string"
          value: "profile_data"
      - name: "enrichProfile"
        operation: "format"
        description: "Enrich profile data"
        needs: ["getProfile"]
        triggers:
          - type: "flex_for_user"
        input:
          - name: "basicProfile"
            description: "Basic profile"
            origin: "inference"
            successCriteria: "Get basic profile"
            onError:
              strategy: "requestUserInput"
              message: "Cannot get profile"
        output:
          type: "string"
          value: "enriched_profile"
      - name: "finalProcess"
        operation: "format"
        description: "Final processing"
        needs: ["enrichProfile"]
        runOnlyIf:
          condition: "check if all data is available"
          from: ["auth", "getProfile", "enrichProfile"]  # auth is 3-level transitive
        triggers:
          - type: "always_on_user_message"
        input:
          - name: "data"
            description: "Final data"
            origin: "inference"
            successCriteria:
              condition: "Combine all data"
              from: ["auth", "getProfile", "enrichProfile"]
            onError:
              strategy: "requestUserInput"
              message: "Cannot get final data"
        output:
          type: "string"
          value: "final_result"
`,
			expectedError: "", // Should pass - 3-level transitive dependency
		},
		{
			name: "Invalid - function not in dependency chain",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "auth"
        operation: "format"
        description: "Authenticate user"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "token"
            description: "Auth token"
            origin: "inference"
            successCriteria: "Extract token"
            onError:
              strategy: "requestUserInput"
              message: "Cannot find token"
        output:
          type: "string"
          value: "authenticated"
      - name: "unrelatedFunction"
        operation: "format"
        description: "Unrelated function"
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
          value: "unrelated_result"
      - name: "processData"
        operation: "format"
        description: "Process data"
        needs: ["auth"]
        runOnlyIf:
          condition: "check if user is authenticated"
          from: ["auth", "unrelatedFunction"]  # unrelatedFunction not in dependency chain
        triggers:
          - type: "always_on_user_message"
        input:
          - name: "data"
            description: "Process data"
            origin: "inference"
            successCriteria: "Get data"
            onError:
              strategy: "requestUserInput"
              message: "Cannot get data"
        output:
          type: "string"
          value: "processed"
`,
			expectedError: "not transitively reachable through the dependency chain",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := CreateTool(tc.yamlInput)
			if tc.expectedError == "" {
				// This should pass
				if err != nil {
					t.Fatalf("Expected no error but got: %v", err)
				}
			} else {
				// This should fail with the expected error
				if err == nil {
					t.Fatalf("Expected error but got none")
				}
				if !strings.Contains(err.Error(), tc.expectedError) {
					t.Errorf("Expected error containing '%s', got '%s'", tc.expectedError, err.Error())
				}
			}
		})
	}
}

func TestCreateTool_ComplexIntegrationScenarios(t *testing.T) {
	testCases := []struct {
		name          string
		yamlInput     string
		expectedError string // empty means should pass
	}{
		{
			name: "Complex dependency chain with mixed formats",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "auth"
        operation: "format"
        description: "Authenticate user"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "token"
            description: "Auth token"
            origin: "inference"
            successCriteria: "Extract token"
            onError:
              strategy: "requestUserInput"
              message: "Cannot find token"
        output:
          type: "string"
          value: "auth_result"
      - name: "getProfile"
        operation: "format"
        description: "Get user profile"
        needs: ["auth"]
        runOnlyIf: "user is authenticated"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "userId"
            description: "User ID"
            origin: "inference"
            successCriteria:
              condition: "Extract user ID from context"
              from: ["auth"]
            onError:
              strategy: "requestUserInput"
              message: "Cannot find user ID"
        output:
          type: "object"
          fields:
            - name: "name"
            - name: "email"
      - name: "processData"
        operation: "format"
        description: "Process user data"
        needs: ["auth", "getProfile"]
        runOnlyIf:
          condition: "user has valid profile and auth"
          from: ["auth", "getProfile"]
          onError:
            strategy: "inference"
            message: "Cannot verify user status"
        triggers:
          - type: "always_on_user_message"
        input:
          - name: "data"
            description: "Process data"
            origin: "inference"
            successCriteria:
              condition: "Combine auth and profile data"
              from: ["auth", "getProfile"]
            onError:
              strategy: "search"
              message: "Cannot process data"
        output:
          type: "string"
          value: "processed_result"
`,
			expectedError: "", // Should pass
		},
		{
			name: "Circular dependency attempt",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "funcA"
        operation: "format"
        description: "Function A"
        needs: ["funcB"]
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
          value: "result_a"
      - name: "funcB"
        operation: "format"
        description: "Function B"
        needs: ["funcA"]
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
          value: "result_b"
`,
			expectedError: "circular dependency detected",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := CreateTool(tc.yamlInput)
			if tc.expectedError == "" {
				// This should pass
				if err != nil {
					t.Fatalf("Expected no error but got: %v", err)
				}
			} else {
				// This should fail with the expected error
				if err == nil {
					t.Fatalf("Expected error but got none")
				}
				if !strings.Contains(err.Error(), tc.expectedError) {
					t.Errorf("Expected error containing '%s', got '%s'", tc.expectedError, err.Error())
				}
			}
		})
	}
}

func TestCreateTool_BackwardCompatibility(t *testing.T) {
	// Test that existing YAML definitions still work
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
        runOnlyIf: "user has admin privileges"
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
            successCriteria: "Determine what the user wants to do"
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
	runOnlyIfCondition := GetRunOnlyIfCondition(function.RunOnlyIf)
	if runOnlyIfCondition != "user has admin privileges" {
		t.Errorf("Expected runOnlyIf condition 'user has admin privileges', got %s", runOnlyIfCondition)
	}

	// Test successCriteria backward compatibility
	input := function.Input[1] // Second input (inference one)
	successCriteriaCondition := GetSuccessCriteriaCondition(input.SuccessCriteria)
	if successCriteriaCondition != "Determine what the user wants to do" {
		t.Errorf("Expected successCriteria condition 'Determine what the user wants to do', got %s", successCriteriaCondition)
	}
}

func TestCreateTool_AllowedSystemFunctions_Success(t *testing.T) {
	testCases := []struct {
		name       string
		yamlInput  string
		validation func(*testing.T, CustomTool)
	}{
		{
			name: "RunOnlyIf with valid allowedSystemFunctions",
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
          condition: "check if user mentioned their language preference"
          allowedSystemFunctions: ["askToTheConversationHistoryWithCustomer", "askToContext"]
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
				if len(runOnlyIfObj.AllowedSystemFunctions) != 2 {
					t.Errorf("Expected 2 allowed system functions, got %d", len(runOnlyIfObj.AllowedSystemFunctions))
				}
				expectedFunctions := []string{"askToTheConversationHistoryWithCustomer", "askToContext"}
				for i, expected := range expectedFunctions {
					if runOnlyIfObj.AllowedSystemFunctions[i] != expected {
						t.Errorf("Expected '%s' at index %d, got '%s'", expected, i, runOnlyIfObj.AllowedSystemFunctions[i])
					}
				}
			},
		},
		{
			name: "SuccessCriteria with valid allowedSystemFunctions",
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
              condition: "Extract user preferences from history"
              allowedSystemFunctions: ["askToTheConversationHistoryWithCustomer", "queryMemories"]
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
				if len(successCriteriaObj.AllowedSystemFunctions) != 2 {
					t.Errorf("Expected 2 allowed system functions, got %d", len(successCriteriaObj.AllowedSystemFunctions))
				}
				expectedFunctions := []string{"askToTheConversationHistoryWithCustomer", "queryMemories"}
				for i, expected := range expectedFunctions {
					if successCriteriaObj.AllowedSystemFunctions[i] != expected {
						t.Errorf("Expected '%s' at index %d, got '%s'", expected, i, successCriteriaObj.AllowedSystemFunctions[i])
					}
				}
			},
		},
		{
			name: "RunOnlyIf with all valid system functions",
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
          condition: "comprehensive check"
          allowedSystemFunctions:
            - "askToTheConversationHistoryWithCustomer"
            - "askToKnowledgeBase"
            - "askToContext"
            - "doDeepWebResearch"
            - "doSimpleWebSearch"
            - "getWeekdayFromDate"
            - "queryMemories"
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
				if len(runOnlyIfObj.AllowedSystemFunctions) != 7 {
					t.Errorf("Expected 7 allowed system functions, got %d", len(runOnlyIfObj.AllowedSystemFunctions))
				}
			},
		},
		{
			name: "RunOnlyIf with empty allowedSystemFunctions (all allowed)",
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
				if len(runOnlyIfObj.AllowedSystemFunctions) != 0 {
					t.Errorf("Expected 0 allowed system functions (meaning all are allowed), got %d", len(runOnlyIfObj.AllowedSystemFunctions))
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

func TestCreateTool_ScratchpadFilter_Success(t *testing.T) {
	testCases := []struct {
		name       string
		yamlInput  string
		validation func(*testing.T, CustomTool)
	}{
		{
			name: "RunOnlyIf with scratchpad in from (no dependency validation)",
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
        runOnlyIf:
          condition: "check gathered workflow context"
          from: ["scratchpad"]
        triggers:
          - type: "always_on_user_message"
        input:
          - name: "data"
            description: "Some data"
            origin: "inference"
            successCriteria: "Extract data"
            onError:
              strategy: "requestUserInput"
              message: "Cannot get data"
        output:
          type: "string"
          value: "processed"
`,
			validation: func(t *testing.T, tool CustomTool) {
				function := tool.Tools[0].Functions[0]

				runOnlyIfObj, err := ParseRunOnlyIf(function.RunOnlyIf)
				if err != nil {
					t.Errorf("Failed to parse runOnlyIf: %v", err)
				}
				if len(runOnlyIfObj.From) != 1 {
					t.Errorf("Expected 1 item in From array, got %d", len(runOnlyIfObj.From))
				}
				if runOnlyIfObj.From[0] != "scratchpad" {
					t.Errorf("Expected 'scratchpad' in From array, got %s", runOnlyIfObj.From[0])
				}
			},
		},
		{
			name: "SuccessCriteria with scratchpad in from (no dependency validation)",
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
          - name: "analysis"
            description: "Generate analysis using workflow context"
            origin: "inference"
            successCriteria:
              condition: "Generate analysis from gathered information"
              from: ["scratchpad"]
            onError:
              strategy: "requestUserInput"
              message: "Cannot generate analysis"
        output:
          type: "string"
          value: "$analysis"
`,
			validation: func(t *testing.T, tool CustomTool) {
				function := tool.Tools[0].Functions[0]
				input := function.Input[0]

				successCriteriaObj, err := ParseSuccessCriteria(input.SuccessCriteria)
				if err != nil {
					t.Errorf("Failed to parse successCriteria: %v", err)
				}
				if len(successCriteriaObj.From) != 1 {
					t.Errorf("Expected 1 item in From array, got %d", len(successCriteriaObj.From))
				}
				if successCriteriaObj.From[0] != "scratchpad" {
					t.Errorf("Expected 'scratchpad' in From array, got %s", successCriteriaObj.From[0])
				}
			},
		},
		{
			name: "Mixed scratchpad and regular function in from",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "searchPatient"
        operation: "format"
        description: "Search for patient"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "patientName"
            description: "Patient name"
            origin: "inference"
            successCriteria: "Extract patient name"
            onError:
              strategy: "requestUserInput"
              message: "Cannot find patient name"
        output:
          type: "string"
          value: "patient_data"
      - name: "generateReport"
        operation: "format"
        description: "Generate report"
        needs: ["searchPatient"]
        triggers:
          - type: "always_on_user_message"
        input:
          - name: "report"
            description: "Generate report from patient data and workflow context"
            origin: "inference"
            successCriteria:
              condition: "Generate comprehensive report"
              from: ["searchPatient", "scratchpad"]
            onError:
              strategy: "requestUserInput"
              message: "Cannot generate report"
        output:
          type: "string"
          value: "$report"
`,
			validation: func(t *testing.T, tool CustomTool) {
				function := tool.Tools[0].Functions[1] // generateReport
				input := function.Input[0]

				successCriteriaObj, err := ParseSuccessCriteria(input.SuccessCriteria)
				if err != nil {
					t.Errorf("Failed to parse successCriteria: %v", err)
				}
				if len(successCriteriaObj.From) != 2 {
					t.Errorf("Expected 2 items in From array, got %d", len(successCriteriaObj.From))
				}
				// Verify both values exist
				foundSearchPatient := false
				foundScratchpad := false
				for _, f := range successCriteriaObj.From {
					if f == "searchPatient" {
						foundSearchPatient = true
					}
					if f == "scratchpad" {
						foundScratchpad = true
					}
				}
				if !foundSearchPatient {
					t.Error("Expected 'searchPatient' in From array")
				}
				if !foundScratchpad {
					t.Error("Expected 'scratchpad' in From array")
				}
			},
		},
		{
			name: "Scratchpad with allowedSystemFunctions including askToContext",
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
          - name: "analysis"
            description: "Generate analysis"
            origin: "inference"
            successCriteria:
              condition: "Analyze workflow context"
              from: ["scratchpad"]
              allowedSystemFunctions: ["askToContext"]
            onError:
              strategy: "requestUserInput"
              message: "Cannot generate analysis"
        output:
          type: "string"
          value: "$analysis"
`,
			validation: func(t *testing.T, tool CustomTool) {
				function := tool.Tools[0].Functions[0]
				input := function.Input[0]

				successCriteriaObj, err := ParseSuccessCriteria(input.SuccessCriteria)
				if err != nil {
					t.Errorf("Failed to parse successCriteria: %v", err)
				}
				if len(successCriteriaObj.From) != 1 || successCriteriaObj.From[0] != "scratchpad" {
					t.Errorf("Expected ['scratchpad'] in From, got %v", successCriteriaObj.From)
				}
				if len(successCriteriaObj.AllowedSystemFunctions) != 1 || successCriteriaObj.AllowedSystemFunctions[0] != "askToContext" {
					t.Errorf("Expected ['askToContext'] in AllowedSystemFunctions, got %v", successCriteriaObj.AllowedSystemFunctions)
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

func TestCreateTool_AllowedSystemFunctions_ValidationErrors(t *testing.T) {
	testCases := []struct {
		name          string
		yamlInput     string
		expectedError string
	}{
		{
			name: "RunOnlyIf with invalid system function",
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
          allowedSystemFunctions: ["askToTheConversationHistoryWithCustomer", "invalidFunction"]
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
			expectedError: "invalid system function 'invalidFunction' in runOnlyIf.allowedSystemFunctions",
		},
		{
			name: "SuccessCriteria with invalid system function",
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
              condition: "Extract info"
              allowedSystemFunctions: ["askToContext", "notARealFunction"]
            onError:
              strategy: "requestUserInput"
              message: "Cannot extract user info"
        output:
          type: "string"
          value: "processed"
`,
			expectedError: "invalid system function 'notARealFunction' in successCriteria.allowedSystemFunctions",
		},
		{
			name: "RunOnlyIf with typo in system function name",
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
          allowedSystemFunctions: ["askToTheConversationHistory"]
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
			expectedError: "invalid system function 'askToTheConversationHistory' in runOnlyIf.allowedSystemFunctions",
		},
		{
			name: "SuccessCriteria with multiple invalid functions",
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
              condition: "Extract info"
              allowedSystemFunctions: ["invalidFunc1", "invalidFunc2", "askToContext"]
            onError:
              strategy: "requestUserInput"
              message: "Cannot extract user info"
        output:
          type: "string"
          value: "processed"
`,
			expectedError: "invalid system function 'invalidFunc1' in successCriteria.allowedSystemFunctions",
		},
		{
			name: "RunOnlyIf with from but askToContext not in allowedSystemFunctions",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "parent"
        operation: "format"
        description: "Parent function"
        triggers:
          - type: "always_on_user_message"
        input:
          - name: "data"
            description: "Some data"
            origin: "inference"
            successCriteria: "Extract data from context"
            onError:
              strategy: "inference"
              message: "Provide data"
        output:
          type: "string"
          value: "$data"
      - name: "child"
        operation: "web_browse"
        description: "Child function"
        successCriteria: "Find info"
        needs: ["parent"]
        runOnlyIf:
          condition: "check parent results"
          from: ["parent"]
          allowedSystemFunctions: ["askToTheConversationHistoryWithCustomer", "queryMemories"]
        triggers:
          - type: "always_on_user_message"
        input:
          - name: "query"
            description: "Search query"
            origin: "chat"
            onError:
              strategy: "requestUserInput"
              message: "Provide query"
        steps:
          - name: "search"
            action: "open_url"
            with:
              url: "https://example.com"
`,
			expectedError: "runOnlyIf.allowedSystemFunctions must include 'askToContext' when 'from' field is specified",
		},
		{
			name: "SuccessCriteria with from but askToContext not in allowedSystemFunctions",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "parent"
        operation: "format"
        description: "Parent function"
        triggers:
          - type: "always_on_user_message"
        input:
          - name: "data"
            description: "Some data"
            origin: "inference"
            successCriteria: "Extract data from context"
            onError:
              strategy: "inference"
              message: "Provide data"
        output:
          type: "string"
          value: "$data"
      - name: "child"
        operation: "format"
        description: "Child function"
        needs: ["parent"]
        triggers:
          - type: "always_on_user_message"
        input:
          - name: "info"
            description: "Extract info based on parent"
            origin: "inference"
            successCriteria:
              condition: "Extract info from parent results"
              from: ["parent"]
              allowedSystemFunctions: ["askToTheConversationHistoryWithCustomer"]
            onError:
              strategy: "inference"
              message: "Failed to extract"
        output:
          type: "string"
          value: "$info"
`,
			expectedError: "successCriteria.allowedSystemFunctions must include 'askToContext' when 'from' field is specified",
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
