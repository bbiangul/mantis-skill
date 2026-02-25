package skill

import (
	"strings"
	"testing"
)

// TestSystemVariablesCOMPANY tests that all $COMPANY fields are properly recognized
func TestSystemVariablesCOMPANY(t *testing.T) {
	testCases := []struct {
		name       string
		field      string
		shouldPass bool
	}{
		{"COMPANY.id", "id", true},
		{"COMPANY.name", "name", true},
		{"COMPANY.fantasy_name", "fantasy_name", true},
		{"COMPANY.tax_code", "tax_code", true},
		{"COMPANY.industry", "industry", true},
		{"COMPANY.email", "email", true},
		{"COMPANY.instagram_profile", "instagram_profile", true},
		{"COMPANY.website", "website", true},
		{"COMPANY.ai_session_id", "ai_session_id", true},
		{"COMPANY alone", "", true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var valueExpr string
			if tc.field == "" {
				valueExpr = "$COMPANY"
			} else {
				valueExpr = "$COMPANY." + tc.field
			}

			yamlInput := `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "TestFunction"
        operation: "api_call"
        description: "Test function using COMPANY variable"
        triggers:
          - type: "always_on_user_message"
        input:
          - name: "companyData"
            description: "Company data from system variable"
            value: "` + valueExpr + `"
        steps:
          - name: "callAPI"
            action: "POST"
            with:
              url: "https://api.example.com/test"
              requestBody:
                type: "application/json"
                with:
                  data: "$companyData"
`
			_, err := CreateTool(yamlInput)

			if tc.shouldPass && err != nil {
				t.Errorf("Expected %s to be valid, but got error: %v", valueExpr, err)
			}
			if !tc.shouldPass && err == nil {
				t.Errorf("Expected %s to fail, but it passed", valueExpr)
			}
		})
	}
}

// TestSystemVariablesUSER tests that all $USER fields are properly recognized
func TestSystemVariablesUSER(t *testing.T) {
	testCases := []struct {
		name       string
		field      string
		shouldPass bool
	}{
		{"USER.id", "id", true},
		{"USER.first_name", "first_name", true},
		{"USER.last_name", "last_name", true},
		{"USER.email", "email", true},
		{"USER.phone", "phone", true},
		{"USER.gender", "gender", true},
		{"USER.birth_date", "birth_date", true},
		{"USER.address", "address", true},
		{"USER.company_id", "company_id", true},
		{"USER.company_name", "company_name", true},
		{"USER.language", "language", true},
		{"USER.messages_count", "messages_count", true},
		{"USER alone", "", true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var valueExpr string
			if tc.field == "" {
				valueExpr = "$USER"
			} else {
				valueExpr = "$USER." + tc.field
			}

			yamlInput := `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "TestFunction"
        operation: "api_call"
        description: "Test function using USER variable"
        triggers:
          - type: "always_on_user_message"
        input:
          - name: "userData"
            description: "User data from system variable"
            value: "` + valueExpr + `"
        steps:
          - name: "callAPI"
            action: "POST"
            with:
              url: "https://api.example.com/test"
              requestBody:
                type: "application/json"
                with:
                  data: "$userData"
`
			_, err := CreateTool(yamlInput)

			if tc.shouldPass && err != nil {
				t.Errorf("Expected %s to be valid, but got error: %v", valueExpr, err)
			}
			if !tc.shouldPass && err == nil {
				t.Errorf("Expected %s to fail, but it passed", valueExpr)
			}
		})
	}
}

// TestSystemVariablesNOW tests that all $NOW fields are properly recognized
func TestSystemVariablesNOW(t *testing.T) {
	testCases := []struct {
		name       string
		field      string
		shouldPass bool
	}{
		{"NOW.date", "date", true},
		{"NOW.time", "time", true},
		{"NOW.hour", "hour", true},
		{"NOW.unix", "unix", true},
		{"NOW.iso8601", "iso8601", true},
		{"NOW alone", "", true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var valueExpr string
			if tc.field == "" {
				valueExpr = "$NOW"
			} else {
				valueExpr = "$NOW." + tc.field
			}

			yamlInput := `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "TestFunction"
        operation: "api_call"
        description: "Test function using NOW variable"
        triggers:
          - type: "always_on_user_message"
        input:
          - name: "timeData"
            description: "Time data from system variable"
            value: "` + valueExpr + `"
        steps:
          - name: "callAPI"
            action: "POST"
            with:
              url: "https://api.example.com/test"
              requestBody:
                type: "application/json"
                with:
                  data: "$timeData"
`
			_, err := CreateTool(yamlInput)

			if tc.shouldPass && err != nil {
				t.Errorf("Expected %s to be valid, but got error: %v", valueExpr, err)
			}
			if !tc.shouldPass && err == nil {
				t.Errorf("Expected %s to fail, but it passed", valueExpr)
			}
		})
	}
}

// TestSystemVariablesMESSAGE tests that all $MESSAGE fields are properly recognized
func TestSystemVariablesMESSAGE(t *testing.T) {
	testCases := []struct {
		name       string
		field      string
		shouldPass bool
	}{
		{"MESSAGE.id", "id", true},
		{"MESSAGE.text", "text", true},
		{"MESSAGE.from", "from", true},
		{"MESSAGE.channel", "channel", true},
		{"MESSAGE.timestamp", "timestamp", true},
		{"MESSAGE.hasMedia", "hasMedia", true},
		{"MESSAGE alone", "", true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var valueExpr string
			if tc.field == "" {
				valueExpr = "$MESSAGE"
			} else {
				valueExpr = "$MESSAGE." + tc.field
			}

			yamlInput := `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "TestFunction"
        operation: "api_call"
        description: "Test function using MESSAGE variable"
        triggers:
          - type: "always_on_user_message"
        input:
          - name: "messageData"
            description: "Message data from system variable"
            value: "` + valueExpr + `"
        steps:
          - name: "callAPI"
            action: "POST"
            with:
              url: "https://api.example.com/test"
              requestBody:
                type: "application/json"
                with:
                  data: "$messageData"
`
			_, err := CreateTool(yamlInput)

			if tc.shouldPass && err != nil {
				t.Errorf("Expected %s to be valid, but got error: %v", valueExpr, err)
			}
			if !tc.shouldPass && err == nil {
				t.Errorf("Expected %s to fail, but it passed", valueExpr)
			}
		})
	}
}

// TestSystemVariablesADMIN tests that all $ADMIN fields are properly recognized
func TestSystemVariablesADMIN(t *testing.T) {
	testCases := []struct {
		name       string
		field      string
		shouldPass bool
	}{
		{"ADMIN.id", "id", true},
		{"ADMIN.first_name", "first_name", true},
		{"ADMIN.last_name", "last_name", true},
		{"ADMIN.email", "email", true},
		{"ADMIN.phone", "phone", true},
		{"ADMIN.gender", "gender", true},
		{"ADMIN.birth_date", "birth_date", true},
		{"ADMIN.address", "address", true},
		{"ADMIN.company_id", "company_id", true},
		{"ADMIN.company_name", "company_name", true},
		{"ADMIN.language", "language", true},
		{"ADMIN alone", "", true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var valueExpr string
			if tc.field == "" {
				valueExpr = "$ADMIN"
			} else {
				valueExpr = "$ADMIN." + tc.field
			}

			yamlInput := `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "TestFunction"
        operation: "api_call"
        description: "Test function using ADMIN variable"
        triggers:
          - type: "always_on_user_message"
        input:
          - name: "adminData"
            description: "Admin data from system variable"
            value: "` + valueExpr + `"
        steps:
          - name: "callAPI"
            action: "POST"
            with:
              url: "https://api.example.com/test"
              requestBody:
                type: "application/json"
                with:
                  data: "$adminData"
`
			_, err := CreateTool(yamlInput)

			if tc.shouldPass && err != nil {
				t.Errorf("Expected %s to be valid, but got error: %v", valueExpr, err)
			}
			if !tc.shouldPass && err == nil {
				t.Errorf("Expected %s to fail, but it passed", valueExpr)
			}
		})
	}
}

// TestSystemVariablesFILE tests that all $FILE fields are properly recognized
func TestSystemVariablesFILE(t *testing.T) {
	testCases := []struct {
		name       string
		field      string
		shouldPass bool
	}{
		{"FILE.url", "url", true},
		{"FILE.path", "path", true},
		{"FILE.mimetype", "mimetype", true},
		{"FILE.filename", "filename", true},
		{"FILE alone", "", true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var valueExpr string
			if tc.field == "" {
				valueExpr = "$FILE"
			} else {
				valueExpr = "$FILE." + tc.field
			}

			yamlInput := `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "TestFunction"
        operation: "api_call"
        description: "Test function using FILE variable"
        triggers:
          - type: "always_on_user_message"
        input:
          - name: "fileData"
            description: "File data from system variable"
            value: "` + valueExpr + `"
        steps:
          - name: "callAPI"
            action: "POST"
            with:
              url: "https://api.example.com/test"
              requestBody:
                type: "application/json"
                with:
                  data: "$fileData"
`
			_, err := CreateTool(yamlInput)

			if tc.shouldPass && err != nil {
				t.Errorf("Expected %s to be valid, but got error: %v", valueExpr, err)
			}
			if !tc.shouldPass && err == nil {
				t.Errorf("Expected %s to fail, but it passed", valueExpr)
			}
		})
	}
}

// TestSystemVariablesMEETING tests that all $MEETING fields are properly recognized
func TestSystemVariablesMEETING(t *testing.T) {
	testCases := []struct {
		name       string
		field      string
		shouldPass bool
	}{
		{"MEETING.bot_id", "bot_id", true},
		{"MEETING.event", "event", true},
		{"MEETING.timestamp", "timestamp", true},
		{"MEETING alone", "", true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var valueExpr string
			if tc.field == "" {
				valueExpr = "$MEETING"
			} else {
				valueExpr = "$MEETING." + tc.field
			}

			// Use on_meeting_start trigger for MEETING variable
			yamlInput := `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "TestFunction"
        operation: "api_call"
        description: "Test function using MEETING variable"
        triggers:
          - type: "on_meeting_start"
        input:
          - name: "meetingData"
            description: "Meeting data from system variable"
            value: "` + valueExpr + `"
        steps:
          - name: "callAPI"
            action: "POST"
            with:
              url: "https://api.example.com/test"
              requestBody:
                type: "application/json"
                with:
                  data: "$meetingData"
`
			_, err := CreateTool(yamlInput)

			if tc.shouldPass && err != nil {
				t.Errorf("Expected %s to be valid, but got error: %v", valueExpr, err)
			}
			if !tc.shouldPass && err == nil {
				t.Errorf("Expected %s to fail, but it passed", valueExpr)
			}
		})
	}
}

// TestSystemVariablesME tests that all $ME fields are properly recognized
func TestSystemVariablesME(t *testing.T) {
	testCases := []struct {
		name       string
		field      string
		shouldPass bool
	}{
		{"ME.name", "name", true},
		{"ME.version", "version", true},
		{"ME.description", "description", true},
		{"ME alone", "", true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var valueExpr string
			if tc.field == "" {
				valueExpr = "$ME"
			} else {
				valueExpr = "$ME." + tc.field
			}

			yamlInput := `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "TestFunction"
        operation: "api_call"
        description: "Test function using ME variable"
        triggers:
          - type: "always_on_user_message"
        input:
          - name: "meData"
            description: "ME data from system variable"
            value: "` + valueExpr + `"
        steps:
          - name: "callAPI"
            action: "POST"
            with:
              url: "https://api.example.com/test"
              requestBody:
                type: "application/json"
                with:
                  data: "$meData"
`
			_, err := CreateTool(yamlInput)

			if tc.shouldPass && err != nil {
				t.Errorf("Expected %s to be valid, but got error: %v", valueExpr, err)
			}
			if !tc.shouldPass && err == nil {
				t.Errorf("Expected %s to fail, but it passed", valueExpr)
			}
		})
	}
}

// TestSystemVariablesUUID tests that $UUID is properly recognized
func TestSystemVariablesUUID(t *testing.T) {
	yamlInput := `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "TestFunction"
        operation: "api_call"
        description: "Test function using UUID variable"
        triggers:
          - type: "always_on_user_message"
        input:
          - name: "uniqueId"
            description: "Unique ID from system variable"
            value: "$UUID"
        steps:
          - name: "callAPI"
            action: "POST"
            with:
              url: "https://api.example.com/test"
              requestBody:
                type: "application/json"
                with:
                  id: "$uniqueId"
`
	_, err := CreateTool(yamlInput)

	if err != nil {
		t.Errorf("Expected $UUID to be valid, but got error: %v", err)
	}
}

// TestSystemVariablesInDeterministicExpression tests runOnlyIf deterministic expressions
func TestSystemVariablesInDeterministicExpression(t *testing.T) {
	// This tests that deterministic expressions with function references are properly validated
	yamlInput := `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "getDealInfo"
        operation: "api_call"
        description: "Get deal info"
        triggers:
          - type: "always_on_user_message"
        steps:
          - name: "getDeal"
            action: "GET"
            with:
              url: "https://api.example.com/deal"
        output:
          type: "object"
          fields:
            - "status"
            - "amount"
      - name: "processHighValue"
        operation: "api_call"
        description: "Process high value deals"
        triggers:
          - type: "always_on_user_message"
        needs:
          - name: "getDealInfo"
        runOnlyIf:
          deterministic: "$getDealInfo.amount > 1000"
        steps:
          - name: "process"
            action: "POST"
            with:
              url: "https://api.example.com/process"
`
	_, err := CreateTool(yamlInput)

	if err != nil {
		t.Errorf("Expected tool with deterministic runOnlyIf to be valid, but got error: %v", err)
	}
}

// TestCOMPANYAiSessionIdInAPICall tests $COMPANY.ai_session_id specifically in an API call context
func TestCOMPANYAiSessionIdInAPICall(t *testing.T) {
	yamlInput := `
version: "v1"
author: "Test Author"
tools:
  - name: "SessionTool"
    description: "Tool that uses AI session ID"
    version: "1.0.0"
    functions:
      - name: "CallWithSession"
        operation: "api_call"
        description: "Make API call with AI session ID"
        triggers:
          - type: "always_on_user_message"
        input:
          - name: "sessionId"
            description: "AI Session ID for the company"
            value: "$COMPANY.ai_session_id"
          - name: "companyId"
            description: "Company ID"
            value: "$COMPANY.id"
          - name: "companyName"
            description: "Company name"
            value: "$COMPANY.name"
        steps:
          - name: "authenticatedCall"
            action: "POST"
            with:
              url: "https://api.example.com/session"
              headers:
                - key: "X-Session-ID"
                  value: "$sessionId"
                - key: "X-Company-ID"
                  value: "$companyId"
              requestBody:
                type: "application/json"
                with:
                  company: "$companyName"
                  session: "$sessionId"
        output:
          type: "object"
          fields:
            - "success"
            - "message"
`
	tool, err := CreateTool(yamlInput)

	if err != nil {
		t.Fatalf("Expected tool with $COMPANY.ai_session_id to be valid, but got error: %v", err)
	}

	// Verify the tool was parsed correctly
	if len(tool.Tools) != 1 {
		t.Errorf("Expected 1 tool, got %d", len(tool.Tools))
	}

	if len(tool.Tools[0].Functions) != 1 {
		t.Errorf("Expected 1 function, got %d", len(tool.Tools[0].Functions))
	}

	fn := tool.Tools[0].Functions[0]
	if len(fn.Input) != 3 {
		t.Errorf("Expected 3 inputs, got %d", len(fn.Input))
	}

	// Verify the sessionId input has the correct value
	var sessionInput *Input
	for i := range fn.Input {
		if fn.Input[i].Name == "sessionId" {
			sessionInput = &fn.Input[i]
			break
		}
	}

	if sessionInput == nil {
		t.Fatal("Expected to find sessionId input")
	}

	if sessionInput.Value != "$COMPANY.ai_session_id" {
		t.Errorf("Expected sessionId value to be '$COMPANY.ai_session_id', got '%s'", sessionInput.Value)
	}
}

// TestSystemVariablesMultipleInSameValue tests combining multiple system variables
func TestSystemVariablesMultipleInSameValue(t *testing.T) {
	yamlInput := `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "TestFunction"
        operation: "api_call"
        description: "Test function using multiple system variables"
        triggers:
          - type: "always_on_user_message"
        input:
          - name: "combinedData"
            description: "Combined data from multiple system variables"
            value: "$USER.first_name $USER.last_name works at $COMPANY.name"
          - name: "timestamp"
            description: "Current timestamp with company"
            value: "Session $COMPANY.ai_session_id at $NOW.iso8601"
        steps:
          - name: "callAPI"
            action: "POST"
            with:
              url: "https://api.example.com/test"
              requestBody:
                type: "application/json"
                with:
                  combined: "$combinedData"
                  timestamp: "$timestamp"
`
	_, err := CreateTool(yamlInput)

	if err != nil {
		t.Errorf("Expected tool with multiple system variables to be valid, but got error: %v", err)
	}
}

// TestSystemVariablesNoWarningsForValidUsage tests that valid system variable usage doesn't generate warnings
func TestSystemVariablesNoWarningsForValidUsage(t *testing.T) {
	yamlInput := `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "TestFunction"
        operation: "api_call"
        description: "Test function using system variables"
        triggers:
          - type: "always_on_user_message"
        input:
          - name: "userId"
            description: "User ID"
            value: "$USER.id"
          - name: "companySession"
            description: "Company AI session"
            value: "$COMPANY.ai_session_id"
          - name: "currentTime"
            description: "Current time"
            value: "$NOW.iso8601"
        steps:
          - name: "callAPI"
            action: "POST"
            with:
              url: "https://api.example.com/test"
              requestBody:
                type: "application/json"
                with:
                  userId: "$userId"
                  session: "$companySession"
`
	result, err := CreateToolWithWarnings(yamlInput)

	if err != nil {
		t.Fatalf("Expected tool to be valid, but got error: %v", err)
	}

	// Check that there are no warnings about invalid system variables
	for _, warning := range result.Warnings {
		if strings.Contains(warning, "system variable") && strings.Contains(warning, "invalid") {
			t.Errorf("Unexpected warning about system variables: %s", warning)
		}
	}
}

// TestSystemVariablesInvalidField tests that invalid system variable fields are rejected
func TestSystemVariablesInvalidField(t *testing.T) {
	testCases := []struct {
		name          string
		variableExpr  string
		expectedError string
	}{
		{
			name:          "Invalid COMPANY field",
			variableExpr:  "$COMPANY.invalid_field",
			expectedError: "invalid field 'invalid_field' for system variable '$COMPANY'",
		},
		{
			name:          "Invalid USER field",
			variableExpr:  "$USER.invalid_field",
			expectedError: "invalid field 'invalid_field' for system variable '$USER'",
		},
		{
			name:          "Invalid NOW field",
			variableExpr:  "$NOW.invalid_field",
			expectedError: "invalid field 'invalid_field' for system variable '$NOW'",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			yamlInput := `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "TestFunction"
        operation: "api_call"
        description: "Test function with invalid system variable field"
        triggers:
          - type: "always_on_user_message"
        input:
          - name: "testData"
            description: "Test data"
            value: "` + tc.variableExpr + `"
        steps:
          - name: "callAPI"
            action: "POST"
            with:
              url: "https://api.example.com/test"
              requestBody:
                type: "application/json"
                with:
                  data: "$testData"
`
			_, err := CreateTool(yamlInput)

			if err == nil {
				t.Errorf("Expected error for %s, but got none", tc.variableExpr)
				return
			}

			if !strings.Contains(err.Error(), tc.expectedError) {
				t.Errorf("Expected error to contain '%s', but got: %v", tc.expectedError, err)
			}
		})
	}
}

// TestAllCOMPANYFieldsComplete tests that all COMPANY fields from documentation are supported
func TestAllCOMPANYFieldsComplete(t *testing.T) {
	// These are all the fields that should be supported according to the documentation
	expectedFields := []string{
		"id",
		"name",
		"fantasy_name",
		"tax_code",
		"industry",
		"email",
		"instagram_profile",
		"website",
		"ai_session_id",
	}

	for _, field := range expectedFields {
		t.Run("COMPANY."+field, func(t *testing.T) {
			yamlInput := `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "TestFunction"
        operation: "api_call"
        description: "Test function"
        triggers:
          - type: "always_on_user_message"
        input:
          - name: "data"
            description: "Data"
            value: "$COMPANY.` + field + `"
        steps:
          - name: "callAPI"
            action: "GET"
            with:
              url: "https://api.example.com/test?data=$data"
`
			_, err := CreateTool(yamlInput)

			if err != nil {
				t.Errorf("Expected $COMPANY.%s to be valid, but got error: %v", field, err)
			}
		})
	}
}
