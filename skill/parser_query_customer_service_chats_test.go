package skill

import (
	"strings"
	"testing"
)

// TestCreateTool_QueryCustomerServiceChats_ClientIdsRequired tests that clientIds is required
// when queryCustomerServiceChats is in allowedSystemFunctions
func TestCreateTool_QueryCustomerServiceChats_ClientIdsRequired(t *testing.T) {
	testCases := []struct {
		name          string
		yamlInput     string
		expectError   bool
		errorContains string
	}{
		{
			name: "RunOnlyIf with queryCustomerServiceChats and clientIds - valid",
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
          condition: "analyze these customer chats"
          allowedSystemFunctions: ["queryCustomerServiceChats"]
          clientIds: "$targetClientIds"
        triggers:
          - type: "always_on_team_message"
        input:
          - name: "targetClientIds"
            description: "Client IDs to analyze"
            origin: "chat"
            onError:
              strategy: "requestUserInput"
              message: "Please provide client IDs"
          - name: "inferredValue"
            description: "Some inferred value"
            origin: "inference"
            successCriteria: "Extract the value"
            onError:
              strategy: "requestUserInput"
              message: "Cannot extract"
        output:
          type: "string"
          value: "done"
`,
			expectError: false,
		},
		{
			name: "RunOnlyIf with queryCustomerServiceChats missing clientIds - error",
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
          condition: "analyze these customer chats"
          allowedSystemFunctions: ["queryCustomerServiceChats"]
        triggers:
          - type: "always_on_team_message"
        input:
          - name: "query"
            description: "Query to analyze"
            origin: "chat"
            onError:
              strategy: "requestUserInput"
              message: "Please provide a query"
          - name: "inferredValue"
            description: "Some inferred value"
            origin: "inference"
            successCriteria: "Extract the value"
            onError:
              strategy: "requestUserInput"
              message: "Cannot extract"
        output:
          type: "string"
          value: "done"
`,
			expectError:   true,
			errorContains: "queryCustomerServiceChats",
		},
		{
			name: "RunOnlyIf inference mode with queryCustomerServiceChats and clientIds - valid",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "hasPermission"
        operation: "format"
        description: "Check permission"
        triggers:
          - type: "flex_for_team"
        input:
          - name: "check"
            description: "Permission check"
            origin: "inference"
            successCriteria: "Check if user has permission"
            onError:
              strategy: "requestUserInput"
              message: "Error"
        output:
          type: "string"
          value: "true"
      - name: "TestFunction"
        operation: "format"
        description: "A test function"
        runOnlyIf:
          deterministic: "$hasPermission == 'true'"
          inference:
            condition: "analyze these customer chats"
            allowedSystemFunctions: ["queryCustomerServiceChats"]
            clientIds: "$targetClientIds"
          combineWith: "AND"
        needs: ["hasPermission"]
        triggers:
          - type: "always_on_team_message"
        input:
          - name: "targetClientIds"
            description: "Client IDs to analyze"
            origin: "chat"
            onError:
              strategy: "requestUserInput"
              message: "Please provide client IDs"
          - name: "inferredValue"
            description: "Some inferred value"
            origin: "inference"
            successCriteria: "Extract the value"
            onError:
              strategy: "requestUserInput"
              message: "Cannot extract"
        output:
          type: "string"
          value: "done"
`,
			expectError: false,
		},
		{
			name: "RunOnlyIf inference mode with queryCustomerServiceChats missing clientIds - error",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "hasPermission"
        operation: "format"
        description: "Check permission"
        triggers:
          - type: "flex_for_team"
        input:
          - name: "check"
            description: "Permission check"
            origin: "inference"
            successCriteria: "Check if user has permission"
            onError:
              strategy: "requestUserInput"
              message: "Error"
        output:
          type: "string"
          value: "true"
      - name: "TestFunction"
        operation: "format"
        description: "A test function"
        runOnlyIf:
          deterministic: "$hasPermission == 'true'"
          inference:
            condition: "analyze these customer chats"
            allowedSystemFunctions: ["queryCustomerServiceChats"]
          combineWith: "AND"
        needs: ["hasPermission"]
        triggers:
          - type: "always_on_team_message"
        input:
          - name: "query"
            description: "Query to analyze"
            origin: "chat"
            onError:
              strategy: "requestUserInput"
              message: "Please provide a query"
          - name: "inferredValue"
            description: "Some inferred value"
            origin: "inference"
            successCriteria: "Extract the value"
            onError:
              strategy: "requestUserInput"
              message: "Cannot extract"
        output:
          type: "string"
          value: "done"
`,
			expectError:   true,
			errorContains: "inference.allowedSystemFunctions includes 'queryCustomerServiceChats' but 'clientIds' field is not specified",
		},
		{
			name: "SuccessCriteria with queryCustomerServiceChats and clientIds - valid",
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
          - type: "always_on_team_message"
        input:
          - name: "targetClientIds"
            description: "Client IDs to analyze"
            origin: "chat"
            onError:
              strategy: "requestUserInput"
              message: "Please provide client IDs"
          - name: "analysis"
            description: "Analyze customer conversations"
            origin: "inference"
            successCriteria:
              condition: "Find patterns in customer conversations"
              allowedSystemFunctions: ["queryCustomerServiceChats"]
              clientIds: "$targetClientIds"
            onError:
              strategy: "requestUserInput"
              message: "Cannot analyze"
        output:
          type: "string"
          value: "done"
`,
			expectError: false,
		},
		{
			name: "SuccessCriteria with queryCustomerServiceChats missing clientIds - error",
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
          - type: "always_on_team_message"
        input:
          - name: "analysis"
            description: "Analyze customer conversations"
            origin: "inference"
            successCriteria:
              condition: "Find patterns in customer conversations"
              allowedSystemFunctions: ["queryCustomerServiceChats"]
            onError:
              strategy: "requestUserInput"
              message: "Cannot analyze"
        output:
          type: "string"
          value: "done"
`,
			expectError:   true,
			errorContains: "queryCustomerServiceChats",
		},
		{
			name: "Multiple allowedSystemFunctions with queryCustomerServiceChats and clientIds - valid",
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
          - type: "always_on_team_message"
        input:
          - name: "targetClientIds"
            description: "Client IDs to analyze"
            origin: "chat"
            onError:
              strategy: "requestUserInput"
              message: "Please provide client IDs"
          - name: "analysis"
            description: "Analyze customer conversations"
            origin: "inference"
            successCriteria:
              condition: "Find patterns in customer conversations"
              allowedSystemFunctions: ["askToKnowledgeBase", "queryCustomerServiceChats", "askToContext"]
              clientIds: "$targetClientIds"
            onError:
              strategy: "requestUserInput"
              message: "Cannot analyze"
        output:
          type: "string"
          value: "done"
`,
			expectError: false,
		},
		{
			name: "allowedSystemFunctions without queryCustomerServiceChats - clientIds not required",
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
          - type: "always_on_team_message"
        input:
          - name: "analysis"
            description: "Analyze something"
            origin: "inference"
            successCriteria:
              condition: "Find patterns"
              allowedSystemFunctions: ["askToKnowledgeBase", "askToContext"]
            onError:
              strategy: "requestUserInput"
              message: "Cannot analyze"
        output:
          type: "string"
          value: "done"
`,
			expectError: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := CreateTool(tc.yamlInput)
			if tc.expectError {
				if err == nil {
					t.Fatalf("Expected error but got none")
				}
				if !strings.Contains(err.Error(), tc.errorContains) {
					t.Errorf("Expected error containing '%s', got '%s'", tc.errorContains, err.Error())
				}
			} else {
				if err != nil {
					t.Fatalf("Did not expect error but got: %v", err)
				}
			}
		})
	}
}

// TestParseHelpers_QueryCustomerServiceChats_ClientIds tests the parsing helpers
func TestParseHelpers_QueryCustomerServiceChats_ClientIds(t *testing.T) {
	t.Run("ParseRunOnlyIf with clientIds", func(t *testing.T) {
		input := map[string]interface{}{
			"condition":              "analyze chats",
			"allowedSystemFunctions": []interface{}{"queryCustomerServiceChats"},
			"clientIds":              "$targetClientIds",
		}

		result, err := ParseRunOnlyIf(input)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if result.ClientIds != "$targetClientIds" {
			t.Errorf("Expected clientIds '$targetClientIds', got '%s'", result.ClientIds)
		}
	})

	t.Run("ParseRunOnlyIf with inference.clientIds", func(t *testing.T) {
		input := map[string]interface{}{
			"deterministic": "$check == 'true'",
			"inference": map[string]interface{}{
				"condition":              "analyze chats",
				"allowedSystemFunctions": []interface{}{"queryCustomerServiceChats"},
				"clientIds":              "$inferenceClientIds",
			},
			"combineWith": "AND",
		}

		result, err := ParseRunOnlyIf(input)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if result.Inference == nil {
			t.Fatal("Expected Inference to be set")
		}
		if result.Inference.ClientIds != "$inferenceClientIds" {
			t.Errorf("Expected inference.clientIds '$inferenceClientIds', got '%s'", result.Inference.ClientIds)
		}
	})

	t.Run("ParseSuccessCriteria with clientIds", func(t *testing.T) {
		input := map[string]interface{}{
			"condition":              "analyze chats",
			"allowedSystemFunctions": []interface{}{"queryCustomerServiceChats"},
			"clientIds":              "$myClientIds",
		}

		result, err := ParseSuccessCriteria(input)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if result.ClientIds != "$myClientIds" {
			t.Errorf("Expected clientIds '$myClientIds', got '%s'", result.ClientIds)
		}
	})

	t.Run("ParseRunOnlyIf clientIds must be string", func(t *testing.T) {
		input := map[string]interface{}{
			"condition":              "analyze chats",
			"allowedSystemFunctions": []interface{}{"queryCustomerServiceChats"},
			"clientIds":              123, // Invalid type
		}

		_, err := ParseRunOnlyIf(input)
		if err == nil {
			t.Fatal("Expected error for non-string clientIds")
		}
		if !strings.Contains(err.Error(), "clientIds must be a string") {
			t.Errorf("Expected clientIds type error, got: %v", err)
		}
	})

	t.Run("ParseSuccessCriteria clientIds must be string", func(t *testing.T) {
		input := map[string]interface{}{
			"condition":              "analyze chats",
			"allowedSystemFunctions": []interface{}{"queryCustomerServiceChats"},
			"clientIds":              []string{"id1", "id2"}, // Invalid type - should be a variable reference string
		}

		_, err := ParseSuccessCriteria(input)
		if err == nil {
			t.Fatal("Expected error for non-string clientIds")
		}
		if !strings.Contains(err.Error(), "clientIds must be a string") {
			t.Errorf("Expected clientIds type error, got: %v", err)
		}
	})
}

// TestSystemFunctionQueryCustomerServiceChats_Constant tests the constant is defined correctly
func TestSystemFunctionQueryCustomerServiceChats_Constant(t *testing.T) {
	if SystemFunctionQueryCustomerServiceChats != "queryCustomerServiceChats" {
		t.Errorf("Expected SystemFunctionQueryCustomerServiceChats to be 'queryCustomerServiceChats', got '%s'",
			SystemFunctionQueryCustomerServiceChats)
	}
}
