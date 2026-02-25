package skill

import (
	"testing"
)

func TestRunOnlyIfAndSuccessCriteriaIntegration(t *testing.T) {
	// Test a complex YAML that uses both runOnlyIf and successCriteria objects
	yamlInput := `
version: "v1"
author: "Integration Test Author"
tools:
  - name: "IntegrationTool"
    description: "Integration test tool"
    version: "1.0.0"
    functions:
      # Helper function for authentication
      - name: "authenticate"
        operation: "api_call"
        description: "Authenticate user"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "credentials"
            description: "User credentials"
            origin: "chat"
            onError:
              strategy: "requestUserInput"
              message: "Please provide credentials"
        steps:
          - name: "call auth api"
            action: "POST"
            with:
              url: "https://api.example.com/auth"
        output:
          type: "string"
          value: "auth_token"

      # Helper function for getting user profile
      - name: "getUserProfile"
        operation: "api_call"
        description: "Get user profile"
        needs: ["authenticate"]
        triggers:
          - type: "flex_for_user"
        input:
          - name: "userId"
            description: "User ID"
            origin: "inference"
            successCriteria:
              condition: "Extract user ID from conversation"
              from: ["authenticate"]
            onError:
              strategy: "requestUserInput"
              message: "Cannot find user ID"
        steps:
          - name: "get profile"
            action: "GET"
            with:
              url: "https://api.example.com/users/{userId}"
        output:
          type: "object"
          fields:
            - name: "name"
            - name: "email"

      # Main function with complex runOnlyIf
      - name: "ProcessUserData"
        operation: "format"
        description: "Process user data based on authentication and profile"
        needs: ["authenticate", "getUserProfile"]
        runOnlyIf:
          condition: "user is authenticated and has valid profile"
          from: ["authenticate", "getUserProfile"]
          onError:
            strategy: "requestUserInput"
            message: "Cannot verify authentication status. Continue anyway?"
        triggers:
          - type: "always_on_user_message"
        input:
          - name: "processedData"
            description: "Extract and process user information"
            origin: "inference"
            successCriteria:
              condition: "Combine user authentication and profile data"
              from: ["authenticate", "getUserProfile"]
            onError:
              strategy: "requestUserInput"
              message: "Cannot process user data"
        output:
          type: "string"
          value: "processed_user_data"
`

	tool, err := CreateTool(yamlInput)
	if err != nil {
		t.Fatalf("Failed to create integration tool: %v", err)
	}

	// Verify the tool structure
	if len(tool.Tools) != 1 {
		t.Errorf("Expected 1 tool, got %d", len(tool.Tools))
	}

	if len(tool.Tools[0].Functions) != 3 {
		t.Errorf("Expected 3 functions, got %d", len(tool.Tools[0].Functions))
	}

	// Test the main function (ProcessUserData)
	mainFunction := tool.Tools[0].Functions[2]

	// Test runOnlyIf object parsing
	runOnlyIfObj, err := ParseRunOnlyIf(mainFunction.RunOnlyIf)
	if err != nil {
		t.Errorf("Failed to parse runOnlyIf: %v", err)
	}

	if runOnlyIfObj.Condition != "user is authenticated and has valid profile" {
		t.Errorf("Expected runOnlyIf condition 'user is authenticated and has valid profile', got %s", runOnlyIfObj.Condition)
	}

	if len(runOnlyIfObj.From) != 2 {
		t.Errorf("Expected 2 functions in runOnlyIf.from, got %d", len(runOnlyIfObj.From))
	}

	expectedFrom := []string{"authenticate", "getUserProfile"}
	for i, expected := range expectedFrom {
		if runOnlyIfObj.From[i] != expected {
			t.Errorf("Expected runOnlyIf.from[%d] = %s, got %s", i, expected, runOnlyIfObj.From[i])
		}
	}

	if runOnlyIfObj.OnError == nil {
		t.Errorf("Expected runOnlyIf.onError to be non-nil")
	} else {
		if runOnlyIfObj.OnError.Strategy != "requestUserInput" {
			t.Errorf("Expected runOnlyIf.onError.strategy 'requestUserInput', got %s", runOnlyIfObj.OnError.Strategy)
		}
	}

	// Test successCriteria object parsing
	input := mainFunction.Input[0]
	successCriteriaObj, err := ParseSuccessCriteria(input.SuccessCriteria)
	if err != nil {
		t.Errorf("Failed to parse successCriteria: %v", err)
	}

	if successCriteriaObj.Condition != "Combine user authentication and profile data" {
		t.Errorf("Expected successCriteria condition 'Combine user authentication and profile data', got %s", successCriteriaObj.Condition)
	}

	if len(successCriteriaObj.From) != 2 {
		t.Errorf("Expected 2 functions in successCriteria.from, got %d", len(successCriteriaObj.From))
	}

	for i, expected := range expectedFrom {
		if successCriteriaObj.From[i] != expected {
			t.Errorf("Expected successCriteria.from[%d] = %s, got %s", i, expected, successCriteriaObj.From[i])
		}
	}

	// Test helper function (getUserProfile) with successCriteria object
	helperFunction := tool.Tools[0].Functions[1]
	helperInput := helperFunction.Input[0]
	helperSuccessCriteriaObj, err := ParseSuccessCriteria(helperInput.SuccessCriteria)
	if err != nil {
		t.Errorf("Failed to parse helper successCriteria: %v", err)
	}

	if helperSuccessCriteriaObj.Condition != "Extract user ID from conversation" {
		t.Errorf("Expected helper successCriteria condition 'Extract user ID from conversation', got %s", helperSuccessCriteriaObj.Condition)
	}

	if len(helperSuccessCriteriaObj.From) != 1 {
		t.Errorf("Expected 1 function in helper successCriteria.from, got %d", len(helperSuccessCriteriaObj.From))
	}

	if helperSuccessCriteriaObj.From[0] != "authenticate" {
		t.Errorf("Expected helper successCriteria.from[0] = 'authenticate', got %s", helperSuccessCriteriaObj.From[0])
	}
}

func TestMixedObjectAndStringFormats(t *testing.T) {
	// Test that mixing object and string formats works correctly
	yamlInput := `
version: "v1"
author: "Mixed Format Test Author"
tools:
  - name: "MixedTool"
    description: "Tool with mixed format usage"
    version: "1.0.0"
    functions:
      - name: "StringFormatFunction"
        operation: "format"
        description: "Function using string formats"
        runOnlyIf: "simple string condition"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "data"
            description: "Some data"
            origin: "inference"
            successCriteria: "simple string criteria"
            onError:
              strategy: "requestUserInput"
              message: "Cannot get data"
        output:
          type: "string"
          value: "result"

      - name: "ObjectFormatFunction"
        operation: "format"
        description: "Function using object formats"
        needs: ["StringFormatFunction"]
        runOnlyIf:
          condition: "complex object condition"
          from: ["StringFormatFunction"]
        triggers:
          - type: "always_on_user_message"
        input:
          - name: "complexData"
            description: "Complex data extraction"
            origin: "inference"
            successCriteria:
              condition: "complex object criteria"
              from: ["StringFormatFunction"]
            onError:
              strategy: "requestUserInput"
              message: "Cannot get complex data"
        output:
          type: "string"
          value: "complex_result"
`

	tool, err := CreateTool(yamlInput)
	if err != nil {
		t.Fatalf("Failed to create mixed format tool: %v", err)
	}

	// Test string format function
	stringFunction := tool.Tools[0].Functions[0]

	// Test string runOnlyIf
	stringRunOnlyIfCondition := GetRunOnlyIfCondition(stringFunction.RunOnlyIf)
	if stringRunOnlyIfCondition != "simple string condition" {
		t.Errorf("Expected string runOnlyIf 'simple string condition', got %s", stringRunOnlyIfCondition)
	}

	// Test string successCriteria
	stringInput := stringFunction.Input[0]
	stringSuccessCriteriaCondition := GetSuccessCriteriaCondition(stringInput.SuccessCriteria)
	if stringSuccessCriteriaCondition != "simple string criteria" {
		t.Errorf("Expected string successCriteria 'simple string criteria', got %s", stringSuccessCriteriaCondition)
	}

	// Test object format function
	objectFunction := tool.Tools[0].Functions[1]

	// Test object runOnlyIf
	objectRunOnlyIfObj, err := ParseRunOnlyIf(objectFunction.RunOnlyIf)
	if err != nil {
		t.Errorf("Failed to parse object runOnlyIf: %v", err)
	}
	if objectRunOnlyIfObj.Condition != "complex object condition" {
		t.Errorf("Expected object runOnlyIf condition 'complex object condition', got %s", objectRunOnlyIfObj.Condition)
	}
	if len(objectRunOnlyIfObj.From) != 1 || objectRunOnlyIfObj.From[0] != "StringFormatFunction" {
		t.Errorf("Expected object runOnlyIf.from ['StringFormatFunction'], got %v", objectRunOnlyIfObj.From)
	}

	// Test object successCriteria
	objectInput := objectFunction.Input[0]
	objectSuccessCriteriaObj, err := ParseSuccessCriteria(objectInput.SuccessCriteria)
	if err != nil {
		t.Errorf("Failed to parse object successCriteria: %v", err)
	}
	if objectSuccessCriteriaObj.Condition != "complex object criteria" {
		t.Errorf("Expected object successCriteria condition 'complex object criteria', got %s", objectSuccessCriteriaObj.Condition)
	}
	if len(objectSuccessCriteriaObj.From) != 1 || objectSuccessCriteriaObj.From[0] != "StringFormatFunction" {
		t.Errorf("Expected object successCriteria.from ['StringFormatFunction'], got %v", objectSuccessCriteriaObj.From)
	}
}
