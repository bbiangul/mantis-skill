package skill

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateFunctionOnMissingUserInfo(t *testing.T) {
	testCases := []struct {
		name     string
		yamlData string
		wantErr  bool
		errMsg   string
	}{
		{
			name: "Valid onMissingUserInfo - Tool function",
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
        onMissingUserInfo: ["notifyTeam", "logEvent"]
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

      - name: "notifyTeam"
        operation: "api_call"
        description: "Notify team about missing info"
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "notify"
            action: "POST"
            with:
              url: "https://api.example.com/notify"

      - name: "logEvent"
        operation: "db"
        description: "Log missing info event"
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "log"
            action: "write"
            with:
              write: "INSERT INTO logs VALUES ('missing_info')"
`,
			wantErr: false,
		},
		{
			name: "Valid onMissingUserInfo - System function",
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
        onMissingUserInfo: ["NotifyHuman"]
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
			name: "Invalid onMissingUserInfo - Non-existent function",
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
        description: "Function with invalid callback"
        onMissingUserInfo: ["nonExistentFunc"]
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
			name: "Invalid onMissingUserInfo - Self reference",
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
        description: "Function that calls itself"
        onMissingUserInfo: ["selfRefFunc"]
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
			errMsg:  "cannot include itself in its 'onMissingUserInfo' list",
		},
		{
			name: "Valid onMissingUserInfo - multiple callbacks",
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
        onMissingUserInfo: ["logEvent", "notifyTeam"]
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "step1"
            action: "GET"
            with:
              url: "https://api.example.com"

      - name: "logEvent"
        operation: "api_call"
        description: "Log event"
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "log"
            action: "POST"
            with:
              url: "https://api.example.com/log"

      - name: "notifyTeam"
        operation: "api_call"
        description: "Notify team"
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "notify"
            action: "POST"
            with:
              url: "https://api.example.com/notify"
`,
			wantErr: false,
		},
		{
			name: "Valid onMissingUserInfo with shouldBeHandledAsMessageToUser",
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
        onMissingUserInfo:
          - name: "notifyUser"
            shouldBeHandledAsMessageToUser: true
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "step1"
            action: "GET"
            with:
              url: "https://api.example.com"

      - name: "notifyUser"
        operation: "api_call"
        description: "Notify user"
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "notify"
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

func TestValidateFunctionOnUserConfirmationRequest(t *testing.T) {
	testCases := []struct {
		name     string
		yamlData string
		wantErr  bool
		errMsg   string
	}{
		{
			name: "Valid onUserConfirmationRequest - Tool function",
			yamlData: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "Tool for testing"
    version: "1.0.0"
    functions:
      - name: "scheduleAppointment"
        operation: "api_call"
        description: "Schedule an appointment"
        requiresUserConfirmation: true
        onUserConfirmationRequest: ["logConfirmationRequest"]
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "schedule"
            action: "POST"
            with:
              url: "https://api.example.com/schedule"

      - name: "logConfirmationRequest"
        operation: "api_call"
        description: "Log confirmation request"
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
			name: "Invalid onUserConfirmationRequest - Non-existent function",
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
        description: "Function with invalid callback"
        onUserConfirmationRequest: ["nonExistentFunc"]
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
			name: "Invalid onUserConfirmationRequest - Self reference",
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
        description: "Function that calls itself"
        onUserConfirmationRequest: ["selfRefFunc"]
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
			errMsg:  "cannot include itself in its 'onUserConfirmationRequest' list",
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

func TestValidateFunctionOnTeamApprovalRequest(t *testing.T) {
	testCases := []struct {
		name     string
		yamlData string
		wantErr  bool
		errMsg   string
	}{
		{
			name: "Valid onTeamApprovalRequest - Tool function",
			yamlData: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "Tool for testing"
    version: "1.0.0"
    functions:
      - name: "processHighValueTransaction"
        operation: "api_call"
        description: "Process high value transaction"
        requiresTeamApproval: true
        onTeamApprovalRequest: ["sendApprovalNotification", "createJiraTicket"]
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "process"
            action: "POST"
            with:
              url: "https://api.example.com/process"

      - name: "sendApprovalNotification"
        operation: "api_call"
        description: "Send approval notification"
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "notify"
            action: "POST"
            with:
              url: "https://api.example.com/notify"

      - name: "createJiraTicket"
        operation: "api_call"
        description: "Create JIRA ticket"
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "create"
            action: "POST"
            with:
              url: "https://api.example.com/jira"
`,
			wantErr: false,
		},
		{
			name: "Invalid onTeamApprovalRequest - Non-existent function",
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
        description: "Function with invalid callback"
        onTeamApprovalRequest: ["nonExistentFunc"]
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
			name: "Invalid onTeamApprovalRequest - Self reference",
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
        description: "Function that calls itself"
        onTeamApprovalRequest: ["selfRefFunc"]
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
			errMsg:  "cannot include itself in its 'onTeamApprovalRequest' list",
		},
		{
			name: "Valid onTeamApprovalRequest - multiple callbacks",
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
        onTeamApprovalRequest: ["notifyApprovers", "createTicket"]
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "step1"
            action: "GET"
            with:
              url: "https://api.example.com"

      - name: "notifyApprovers"
        operation: "api_call"
        description: "Notify approvers"
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "notify"
            action: "POST"
            with:
              url: "https://api.example.com/notify"

      - name: "createTicket"
        operation: "api_call"
        description: "Create ticket"
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "create"
            action: "POST"
            with:
              url: "https://api.example.com/ticket"
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

func TestMagicStringCallbackFieldsInFunction(t *testing.T) {
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
        description: "Main function with all callback types"
        onMissingUserInfo: ["missingInfoHandler"]
        onUserConfirmationRequest: ["confirmationHandler"]
        onTeamApprovalRequest: ["approvalHandler"]
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "execute"
            action: "GET"
            with:
              url: "https://api.example.com"

      - name: "missingInfoHandler"
        operation: "api_call"
        description: "Handle missing info"
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "handle"
            action: "POST"
            with:
              url: "https://api.example.com/missing-info"

      - name: "confirmationHandler"
        operation: "api_call"
        description: "Handle confirmation"
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "handle"
            action: "POST"
            with:
              url: "https://api.example.com/confirmation"

      - name: "approvalHandler"
        operation: "api_call"
        description: "Handle approval"
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "handle"
            action: "POST"
            with:
              url: "https://api.example.com/approval"
`

	tool, err := CreateTool(yamlData)
	assert.NoError(t, err)
	assert.NotNil(t, tool)

	// Verify the callback fields are properly parsed
	assert.Len(t, tool.Tools, 1)
	assert.Len(t, tool.Tools[0].Functions, 4)

	mainFunc := tool.Tools[0].Functions[0]
	assert.Equal(t, "mainFunction", mainFunc.Name)

	// Verify onMissingUserInfo
	assert.Len(t, mainFunc.OnMissingUserInfo, 1)
	assert.Equal(t, "missingInfoHandler", mainFunc.OnMissingUserInfo[0].Name)

	// Verify onUserConfirmationRequest
	assert.Len(t, mainFunc.OnUserConfirmationRequest, 1)
	assert.Equal(t, "confirmationHandler", mainFunc.OnUserConfirmationRequest[0].Name)

	// Verify onTeamApprovalRequest
	assert.Len(t, mainFunc.OnTeamApprovalRequest, 1)
	assert.Equal(t, "approvalHandler", mainFunc.OnTeamApprovalRequest[0].Name)
}

func TestMagicStringCallbackWithShouldBeHandledAsMessageToUser(t *testing.T) {
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
        onMissingUserInfo:
          - name: "logEvent"
            shouldBeHandledAsMessageToUser: true
          - name: "notifyTeam"
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "execute"
            action: "GET"
            with:
              url: "https://api.example.com"

      - name: "logEvent"
        operation: "api_call"
        description: "Log event"
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "log"
            action: "POST"
            with:
              url: "https://api.example.com/log"

      - name: "notifyTeam"
        operation: "api_call"
        description: "Notify team"
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "notify"
            action: "POST"
            with:
              url: "https://api.example.com/notify"
`

	tool, err := CreateTool(yamlData)
	assert.NoError(t, err)
	assert.NotNil(t, tool)

	mainFunc := tool.Tools[0].Functions[0]
	assert.Len(t, mainFunc.OnMissingUserInfo, 2)

	// First callback - logEvent with shouldBeHandledAsMessageToUser
	logCallback := mainFunc.OnMissingUserInfo[0]
	assert.Equal(t, "logEvent", logCallback.Name)
	assert.NotNil(t, logCallback.ShouldBeHandledAsMessageToUser)
	assert.True(t, *logCallback.ShouldBeHandledAsMessageToUser)

	// Second callback - notifyTeam without shouldBeHandledAsMessageToUser
	notifyCallback := mainFunc.OnMissingUserInfo[1]
	assert.Equal(t, "notifyTeam", notifyCallback.Name)
	assert.Nil(t, notifyCallback.ShouldBeHandledAsMessageToUser)
}
