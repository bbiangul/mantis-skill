package skill

import (
	"strings"
	"testing"
)

// TestInitiateWorkflowValidation tests the validation rules for initiate_workflow operations
func TestInitiateWorkflowValidation(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid initiate_workflow with userId",
			yaml: `
version: v1
author: test
tools:
  - name: TestTool
    description: Test tool
    version: "1.0.0"
    functions:
      - name: TriggerWorkflow
        description: Triggers a workflow for a user
        operation: initiate_workflow
        triggers:
          - type: time_based
            cron: "0 0 9 * * *"
        steps:
          - name: start_workflow
            action: start_workflow
            with:
              userId: "user123"
              workflowType: "user"
              message: "Hello, time to check in!"
              context: "Daily check-in workflow"
`,
			wantErr: false,
		},
		{
			name: "valid initiate_workflow with user object",
			yaml: `
version: v1
author: test
tools:
  - name: TestTool
    description: Test tool
    version: "1.0.0"
    functions:
      - name: TriggerWorkflow
        description: Triggers a workflow for a new user
        operation: initiate_workflow
        triggers:
          - type: time_based
            cron: "0 0 9 * * *"
        steps:
          - name: start_workflow
            action: start_workflow
            with:
              user:
                channel: "Waha"
                phoneNumber: "+5511999999999"
                firstName: "John"
              workflowType: "user"
              message: "Welcome new user!"
              context: "Onboarding workflow"
`,
			wantErr: false,
		},
		{
			name: "valid initiate_workflow with user object - all optional fields",
			yaml: `
version: v1
author: test
tools:
  - name: TestTool
    description: Test tool
    version: "1.0.0"
    functions:
      - name: TriggerWorkflow
        description: Triggers a workflow for a new user
        operation: initiate_workflow
        triggers:
          - type: time_based
            cron: "0 0 9 * * *"
        steps:
          - name: start_workflow
            action: start_workflow
            with:
              user:
                channel: "email"
                email: "user@example.com"
                phoneNumber: "+5511999999999"
                firstName: "John"
                lastName: "Doe"
                sessionId: "session-123"
              workflowType: "user"
              message: "Welcome!"
              context: "Full user creation workflow"
`,
			wantErr: false,
		},
		{
			name: "valid initiate_workflow with user object - minimal",
			yaml: `
version: v1
author: test
tools:
  - name: TestTool
    description: Test tool
    version: "1.0.0"
    functions:
      - name: TriggerWorkflow
        description: Triggers a workflow for users
        operation: initiate_workflow
        triggers:
          - type: time_based
            cron: "0 0 9 * * *"
        steps:
          - name: start_workflow
            action: start_workflow
            with:
              user:
                channel: "synthetic"
              workflowType: "user"
              message: "Hello!"
              context: "Batch workflow"
`,
			wantErr: false,
		},
		{
			name: "valid initiate_workflow with userId and optional user creation fields",
			yaml: `
version: v1
author: test
tools:
  - name: TestTool
    description: Test tool
    version: "1.0.0"
    functions:
      - name: TriggerWorkflow
        description: Triggers a workflow
        operation: initiate_workflow
        triggers:
          - type: time_based
            cron: "0 0 9 * * *"
        steps:
          - name: start_workflow
            action: start_workflow
            with:
              userId: "user123"
              channel: "Waha"
              phoneNumber: "+5511999999999"
              firstName: "John"
              workflowType: "user"
              message: "Hello!"
              context: "Workflow context"
`,
			wantErr: false,
		},
		{
			name: "invalid - no userId or user object",
			yaml: `
version: v1
author: test
tools:
  - name: TestTool
    description: Test tool
    version: "1.0.0"
    functions:
      - name: TriggerWorkflow
        description: Triggers a workflow
        operation: initiate_workflow
        triggers:
          - type: time_based
            cron: "0 0 9 * * *"
        steps:
          - name: start_workflow
            action: start_workflow
            with:
              workflowType: "user"
              message: "Hello!"
              context: "No user identifier"
`,
			wantErr: true,
			errMsg:  "must have either 'userId' or 'user' field",
		},
		{
			name: "invalid - both userId and user object",
			yaml: `
version: v1
author: test
tools:
  - name: TestTool
    description: Test tool
    version: "1.0.0"
    functions:
      - name: TriggerWorkflow
        description: Triggers a workflow
        operation: initiate_workflow
        triggers:
          - type: time_based
            cron: "0 0 9 * * *"
        steps:
          - name: start_workflow
            action: start_workflow
            with:
              userId: "user123"
              user:
                channel: "Waha"
              workflowType: "user"
              message: "Hello!"
              context: "Both specified"
`,
			wantErr: true,
			errMsg:  "cannot have both 'userId' and 'user' fields",
		},
		{
			name: "invalid - user object without channel",
			yaml: `
version: v1
author: test
tools:
  - name: TestTool
    description: Test tool
    version: "1.0.0"
    functions:
      - name: TriggerWorkflow
        description: Triggers a workflow
        operation: initiate_workflow
        triggers:
          - type: time_based
            cron: "0 0 9 * * *"
        steps:
          - name: start_workflow
            action: start_workflow
            with:
              user:
                email: "test@example.com"
              workflowType: "user"
              message: "Hello!"
              context: "Missing channel"
`,
			wantErr: true,
			errMsg:  "user object missing required field: 'channel'",
		},
		{
			name: "invalid - user object with invalid channel",
			yaml: `
version: v1
author: test
tools:
  - name: TestTool
    description: Test tool
    version: "1.0.0"
    functions:
      - name: TriggerWorkflow
        description: Triggers a workflow
        operation: initiate_workflow
        triggers:
          - type: time_based
            cron: "0 0 9 * * *"
        steps:
          - name: start_workflow
            action: start_workflow
            with:
              user:
                channel: "invalid_channel"
              workflowType: "user"
              message: "Hello!"
              context: "Invalid channel"
`,
			wantErr: true,
			errMsg:  "invalid channel 'invalid_channel'",
		},
		{
			name: "invalid - user value is plain string (not variable)",
			yaml: `
version: v1
author: test
tools:
  - name: TestTool
    description: Test tool
    version: "1.0.0"
    functions:
      - name: TriggerWorkflow
        description: Triggers a workflow
        operation: initiate_workflow
        triggers:
          - type: time_based
            cron: "0 0 9 * * *"
        steps:
          - name: start_workflow
            action: start_workflow
            with:
              user: "not_a_variable"
              workflowType: "user"
              message: "Hello!"
              context: "Plain string user"
`,
			wantErr: true,
			errMsg:  "must be an object or variable reference",
		},
		{
			name: "valid - flex_for_user trigger",
			yaml: `
version: v1
author: test
tools:
  - name: TestTool
    description: Test tool
    version: "1.0.0"
    functions:
      - name: TriggerWorkflow
        description: Triggers a workflow
        operation: initiate_workflow
        triggers:
          - type: flex_for_user
        steps:
          - name: start_workflow
            action: start_workflow
            with:
              userId: "user123"
              workflowType: "user"
              message: "Hello!"
              context: "Flex for user trigger"
`,
			wantErr: false,
		},
		{
			name: "valid - flex_for_team trigger",
			yaml: `
version: v1
author: test
tools:
  - name: TestTool
    description: Test tool
    version: "1.0.0"
    functions:
      - name: TriggerTeamWorkflow
        description: Triggers a team workflow
        operation: initiate_workflow
        triggers:
          - type: flex_for_team
        steps:
          - name: start_workflow
            action: start_workflow
            with:
              userId: "team123"
              workflowType: "team"
              message: "Team notification!"
              context: "Flex for team trigger"
`,
			wantErr: false,
		},
		{
			name: "invalid - always_on_user_message trigger (not allowed for initiate_workflow)",
			yaml: `
version: v1
author: test
tools:
  - name: TestTool
    description: Test tool
    version: "1.0.0"
    functions:
      - name: TriggerWorkflow
        description: Triggers a workflow
        operation: initiate_workflow
        triggers:
          - type: always_on_user_message
        steps:
          - name: start_workflow
            action: start_workflow
            with:
              userId: "user123"
              workflowType: "user"
              message: "Hello!"
              context: "Invalid trigger type"
`,
			wantErr: true,
			errMsg:  "can only be used with time_based, flex_for_user, or flex_for_team triggers",
		},
		{
			name: "invalid - missing workflowType",
			yaml: `
version: v1
author: test
tools:
  - name: TestTool
    description: Test tool
    version: "1.0.0"
    functions:
      - name: TriggerWorkflow
        description: Triggers a workflow
        operation: initiate_workflow
        triggers:
          - type: time_based
            cron: "0 0 9 * * *"
        steps:
          - name: start_workflow
            action: start_workflow
            with:
              userId: "user123"
              message: "Hello!"
              context: "Missing workflowType"
`,
			wantErr: true,
			errMsg:  "missing required field: 'workflowType'",
		},
		{
			name: "invalid - missing message",
			yaml: `
version: v1
author: test
tools:
  - name: TestTool
    description: Test tool
    version: "1.0.0"
    functions:
      - name: TriggerWorkflow
        description: Triggers a workflow
        operation: initiate_workflow
        triggers:
          - type: time_based
            cron: "0 0 9 * * *"
        steps:
          - name: start_workflow
            action: start_workflow
            with:
              userId: "user123"
              workflowType: "user"
              context: "Missing message"
`,
			wantErr: true,
			errMsg:  "missing required field: 'message'",
		},
		{
			name: "invalid - missing context",
			yaml: `
version: v1
author: test
tools:
  - name: TestTool
    description: Test tool
    version: "1.0.0"
    functions:
      - name: TriggerWorkflow
        description: Triggers a workflow
        operation: initiate_workflow
        triggers:
          - type: time_based
            cron: "0 0 9 * * *"
        steps:
          - name: start_workflow
            action: start_workflow
            with:
              userId: "user123"
              workflowType: "user"
              message: "Hello!"
`,
			wantErr: true,
			errMsg:  "missing required field: 'context'",
		},
		{
			name: "invalid - wrong step action",
			yaml: `
version: v1
author: test
tools:
  - name: TestTool
    description: Test tool
    version: "1.0.0"
    functions:
      - name: TriggerWorkflow
        description: Triggers a workflow
        operation: initiate_workflow
        triggers:
          - type: time_based
            cron: "0 0 9 * * *"
        steps:
          - name: start_workflow
            action: http
            with:
              userId: "user123"
              workflowType: "user"
              message: "Hello!"
              context: "Wrong action"
`,
			wantErr: true,
			errMsg:  "invalid step action 'http'; must be 'start_workflow'",
		},
		{
			name: "invalid - invalid workflowType value",
			yaml: `
version: v1
author: test
tools:
  - name: TestTool
    description: Test tool
    version: "1.0.0"
    functions:
      - name: TriggerWorkflow
        description: Triggers a workflow
        operation: initiate_workflow
        triggers:
          - type: time_based
            cron: "0 0 9 * * *"
        steps:
          - name: start_workflow
            action: start_workflow
            with:
              userId: "user123"
              workflowType: "invalid"
              message: "Hello!"
              context: "Invalid workflow type"
`,
			wantErr: true,
			errMsg:  "invalid workflowType 'invalid'; must be 'user' or 'team'",
		},
		{
			name: "valid - all channel types",
			yaml: `
version: v1
author: test
tools:
  - name: TestTool
    description: Test tool
    version: "1.0.0"
    functions:
      - name: TriggerWhatsapp
        description: Triggers workflow via Waha WhatsApp
        operation: initiate_workflow
        triggers:
          - type: time_based
            cron: "0 0 9 * * *"
        steps:
          - name: start_workflow
            action: start_workflow
            with:
              user:
                channel: "Waha"
              workflowType: "user"
              message: "Hello!"
              context: "Whatsapp"
      - name: TriggerWaha
        description: Triggers workflow via Waha
        operation: initiate_workflow
        triggers:
          - type: time_based
            cron: "0 0 10 * * *"
        steps:
          - name: start_workflow
            action: start_workflow
            with:
              user:
                channel: "Waha"
              workflowType: "user"
              message: "Hello!"
              context: "Waha"
      - name: TriggerEmail
        description: Triggers workflow via email
        operation: initiate_workflow
        triggers:
          - type: time_based
            cron: "0 0 11 * * *"
        steps:
          - name: start_workflow
            action: start_workflow
            with:
              user:
                channel: "email"
              workflowType: "user"
              message: "Hello!"
              context: "Email"
      - name: TriggerWebchat
        description: Triggers workflow via webchat
        operation: initiate_workflow
        triggers:
          - type: time_based
            cron: "0 0 12 * * *"
        steps:
          - name: start_workflow
            action: start_workflow
            with:
              user:
                channel: "webchat"
              workflowType: "user"
              message: "Hello!"
              context: "Webchat"
      - name: TriggerSynthetic
        description: Triggers workflow via synthetic
        operation: initiate_workflow
        triggers:
          - type: time_based
            cron: "0 0 13 * * *"
        steps:
          - name: start_workflow
            action: start_workflow
            with:
              user:
                channel: "synthetic"
              workflowType: "user"
              message: "Hello!"
              context: "Synthetic"
`,
			wantErr: false,
		},
		{
			name: "valid - workflowType team",
			yaml: `
version: v1
author: test
tools:
  - name: TestTool
    description: Test tool
    version: "1.0.0"
    functions:
      - name: TriggerTeamWorkflow
        description: Triggers a team workflow
        operation: initiate_workflow
        triggers:
          - type: time_based
            cron: "0 0 9 * * *"
        steps:
          - name: start_workflow
            action: start_workflow
            with:
              userId: "team123"
              workflowType: "team"
              message: "Team notification!"
              context: "Team workflow"
`,
			wantErr: false,
		},
		{
			name: "valid - shouldSend boolean",
			yaml: `
version: v1
author: test
tools:
  - name: TestTool
    description: Test tool
    version: "1.0.0"
    functions:
      - name: TriggerWorkflow
        description: Triggers a workflow
        operation: initiate_workflow
        triggers:
          - type: time_based
            cron: "0 0 9 * * *"
        steps:
          - name: start_workflow
            action: start_workflow
            with:
              userId: "user123"
              workflowType: "user"
              message: "Hello!"
              context: "With shouldSend"
              shouldSend: false
`,
			wantErr: false,
		},
		{
			name: "valid - shouldSend true literal",
			yaml: `
version: v1
author: test
tools:
  - name: TestTool
    description: Test tool
    version: "1.0.0"
    functions:
      - name: TriggerWorkflow
        description: Triggers a workflow
        operation: initiate_workflow
        triggers:
          - type: time_based
            cron: "0 0 9 * * *"
        steps:
          - name: start_workflow
            action: start_workflow
            with:
              userId: "user123"
              workflowType: "user"
              message: "Hello!"
              context: "With shouldSend true"
              shouldSend: true
`,
			wantErr: false,
		},
		{
			name: "valid - user object with email",
			yaml: `
version: v1
author: test
tools:
  - name: TestTool
    description: Test tool
    version: "1.0.0"
    functions:
      - name: TriggerWorkflow
        description: Triggers a workflow
        operation: initiate_workflow
        triggers:
          - type: time_based
            cron: "0 0 9 * * *"
        steps:
          - name: start_workflow
            action: start_workflow
            with:
              user:
                channel: "email"
                email: "test@example.com"
              workflowType: "user"
              message: "Hello!"
              context: "Email channel"
`,
			wantErr: false,
		},
		{
			name: "invalid - user object field with non-string type",
			yaml: `
version: v1
author: test
tools:
  - name: TestTool
    description: Test tool
    version: "1.0.0"
    functions:
      - name: TriggerWorkflow
        description: Triggers a workflow
        operation: initiate_workflow
        triggers:
          - type: time_based
            cron: "0 0 9 * * *"
        steps:
          - name: start_workflow
            action: start_workflow
            with:
              user:
                channel: "Waha"
                phoneNumber: 12345
              workflowType: "user"
              message: "Hello!"
              context: "Invalid phone type"
`,
			wantErr: true,
			errMsg:  "field 'phoneNumber' must be a string",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := CreateTool(tt.yaml)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error containing %q, but got no error", tt.errMsg)
					return
				}
				if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("error message = %q, want it to contain %q", err.Error(), tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

// TestInitiateWorkflowUserObjectValidation tests specific user object validation
func TestInitiateWorkflowUserObjectValidation(t *testing.T) {
	validChannels := []string{
		"Waha",
		"Widget",
		"API_OFFICIAL",
		"instagram",
		"Teams",
		"whatsapp_cloud",
		"messenger",
		"telegram",
		"email",
		"webchat",
		"slack",
		"facebook",
		"none",
		"WebClient",
		"local_agent_chat",
		"synthetic",
	}

	for _, channel := range validChannels {
		t.Run("valid channel: "+channel, func(t *testing.T) {
			yaml := `
version: v1
author: test
tools:
  - name: TestTool
    description: Test tool
    version: "1.0.0"
    functions:
      - name: TriggerWorkflow
        description: Triggers a workflow
        operation: initiate_workflow
        triggers:
          - type: time_based
            cron: "0 0 9 * * *"
        steps:
          - name: start_workflow
            action: start_workflow
            with:
              user:
                channel: "` + channel + `"
              workflowType: "user"
              message: "Hello!"
              context: "Testing channel"
`
			_, err := CreateTool(yaml)
			if err != nil {
				t.Errorf("channel %q should be valid, but got error: %v", channel, err)
			}
		})
	}
}

// TestInitiateWorkflowChannelCaseSensitivity tests that channel validation is case-sensitive
// and provides helpful error messages for common casing mistakes
func TestInitiateWorkflowChannelCaseSensitivity(t *testing.T) {
	tests := []struct {
		name      string
		channel   string
		wantErr   bool
		errSubstr string
	}{
		{
			name:    "valid - Waha with correct casing",
			channel: "Waha",
			wantErr: false,
		},
		{
			name:      "invalid - waha lowercase should suggest Waha",
			channel:   "waha",
			wantErr:   true,
			errSubstr: "did you mean 'Waha'",
		},
		{
			name:      "invalid - WAHA uppercase should suggest Waha",
			channel:   "WAHA",
			wantErr:   true,
			errSubstr: "did you mean 'Waha'",
		},
		{
			name:    "valid - Teams with correct casing",
			channel: "Teams",
			wantErr: false,
		},
		{
			name:      "invalid - teams lowercase should suggest Teams",
			channel:   "teams",
			wantErr:   true,
			errSubstr: "did you mean 'Teams'",
		},
		{
			name:    "valid - Widget with correct casing",
			channel: "Widget",
			wantErr: false,
		},
		{
			name:      "invalid - widget lowercase should suggest Widget",
			channel:   "widget",
			wantErr:   true,
			errSubstr: "did you mean 'Widget'",
		},
		{
			name:      "invalid - webclient wrong casing should suggest WebClient",
			channel:   "webclient",
			wantErr:   true,
			errSubstr: "did you mean 'WebClient'",
		},
		{
			name:      "invalid - completely unknown channel",
			channel:   "unknown_channel",
			wantErr:   true,
			errSubstr: "invalid channel 'unknown_channel'",
		},
		{
			name:      "invalid - waha_whatsapp is not a valid channel",
			channel:   "waha_whatsapp",
			wantErr:   true,
			errSubstr: "invalid channel 'waha_whatsapp'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			yaml := `
version: v1
author: test
tools:
  - name: TestTool
    description: Test tool
    version: "1.0.0"
    functions:
      - name: TriggerWorkflow
        description: Triggers a workflow
        operation: initiate_workflow
        triggers:
          - type: time_based
            cron: "0 0 9 * * *"
        steps:
          - name: start_workflow
            action: start_workflow
            with:
              user:
                channel: "` + tt.channel + `"
              workflowType: "user"
              message: "Hello!"
              context: "Testing"
`
			_, err := CreateTool(yaml)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error for channel %q, but got nil", tt.channel)
					return
				}
				if !strings.Contains(err.Error(), tt.errSubstr) {
					t.Errorf("expected error containing %q, got: %v", tt.errSubstr, err)
				}
			} else {
				if err != nil {
					t.Errorf("expected no error for channel %q, but got: %v", tt.channel, err)
				}
			}
		})
	}
}

// TestInitiateWorkflowWithVariableReferences tests user field with variable references
func TestInitiateWorkflowWithVariableReferences(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid - multiple steps",
			yaml: `
version: v1
author: test
tools:
  - name: TestTool
    description: Test tool
    version: "1.0.0"
    functions:
      - name: TriggerWorkflow
        description: Triggers multiple workflows
        operation: initiate_workflow
        triggers:
          - type: time_based
            cron: "0 0 9 * * *"
        steps:
          - name: start_workflow_1
            action: start_workflow
            with:
              userId: "user1"
              workflowType: "user"
              message: "Hello user 1!"
              context: "Step 1"
          - name: start_workflow_2
            action: start_workflow
            with:
              userId: "user2"
              workflowType: "team"
              message: "Hello user 2!"
              context: "Step 2"
`,
			wantErr: false,
		},
		{
			name: "valid - foreach with userId from items",
			yaml: `
version: v1
author: test
tools:
  - name: TestTool
    description: Test tool
    version: "1.0.0"
    functions:
      - name: GetUsers
        description: Get users from database
        operation: db
        triggers:
          - type: time_based
            cron: "0 0 9 * * *"
        steps:
          - name: query
            action: select
            with:
              select: "SELECT id, name FROM users"
              resultIndex: 1
        output:
          type: "list[object]"
          fields:
            - value: id
            - value: name
      - name: TriggerWorkflows
        description: Triggers workflows for users
        operation: initiate_workflow
        triggers:
          - type: time_based
            cron: "0 0 9 * * *"
        needs:
          - GetUsers
        input:
          - name: "users"
            type: "list[object]"
            description: "List of users"
            origin: "function"
            from: "GetUsers"
            onError:
              strategy: "requestN1Support"
              message: "Failed to get users"
        steps:
          - name: start_workflow
            action: start_workflow
            foreach:
              items: "$users"
              itemVar: "user"
            with:
              userId: "$user.id"
              workflowType: "user"
              message: "Hello $user.name!"
              context: "Batch workflow"
`,
			wantErr: false,
		},
		{
			name: "valid - foreach with user object variable",
			yaml: `
version: v1
author: test
tools:
  - name: TestTool
    description: Test tool
    version: "1.0.0"
    functions:
      - name: GetLeads
        description: Get leads from database
        operation: db
        triggers:
          - type: time_based
            cron: "0 0 9 * * *"
        steps:
          - name: query
            action: select
            with:
              select: "SELECT channel, email, name FROM leads"
              resultIndex: 1
        output:
          type: "list[object]"
          fields:
            - value: channel
            - value: email
            - value: name
      - name: TriggerWorkflows
        description: Triggers workflows for leads
        operation: initiate_workflow
        triggers:
          - type: time_based
            cron: "0 0 9 * * *"
        needs:
          - GetLeads
        input:
          - name: "leads"
            type: "list[object]"
            description: "List of leads"
            origin: "function"
            from: "GetLeads"
            onError:
              strategy: "requestN1Support"
              message: "Failed to get leads"
        steps:
          - name: start_workflow
            action: start_workflow
            foreach:
              items: "$leads"
              itemVar: "lead"
            with:
              user: "$lead"
              workflowType: "user"
              message: "Hello!"
              context: "Lead outreach"
`,
			wantErr: false,
		},
		{
			name: "valid - user object with variable channel field",
			yaml: `
version: v1
author: test
tools:
  - name: TestTool
    description: Test tool
    version: "1.0.0"
    functions:
      - name: GetContacts
        description: Get contacts from database
        operation: db
        triggers:
          - type: time_based
            cron: "0 0 9 * * *"
        steps:
          - name: query
            action: select
            with:
              select: "SELECT channel, email, name, phone FROM contacts"
              resultIndex: 1
        output:
          type: "list[object]"
          fields:
            - value: channel
            - value: email
            - value: name
            - value: phone
      - name: TriggerWorkflows
        description: Triggers workflows for contacts
        operation: initiate_workflow
        triggers:
          - type: time_based
            cron: "0 0 9 * * *"
        needs:
          - GetContacts
        input:
          - name: "contacts"
            type: "list[object]"
            description: "List of contacts"
            origin: "function"
            from: "GetContacts"
            onError:
              strategy: "requestN1Support"
              message: "Failed to get contacts"
        steps:
          - name: start_workflow
            action: start_workflow
            foreach:
              items: "$contacts"
              itemVar: "contact"
            with:
              user:
                channel: "$contact.channel"
                email: "$contact.email"
                firstName: "$contact.name"
                phoneNumber: "$contact.phone"
              workflowType: "user"
              message: "Hi $contact.name!"
              context: "Contact outreach"
`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := CreateTool(tt.yaml)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error containing %q, but got no error", tt.errMsg)
					return
				}
				if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("error message = %q, want it to contain %q", err.Error(), tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

// TestInitiateWorkflowContextParamsValidation tests the validation of context params in initiate_workflow
func TestInitiateWorkflowContextParamsValidation(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid context with params - full function key",
			yaml: `
version: v1
author: test
tools:
  - name: TestTool
    description: Test tool
    version: "1.0.0"
    functions:
      - name: TriggerWorkflow
        description: Triggers a workflow
        operation: initiate_workflow
        triggers:
          - type: time_based
            cron: "0 0 9 * * *"
        steps:
          - name: start_workflow
            action: start_workflow
            with:
              userId: "user123"
              workflowType: "user"
              message: "Hello!"
              context:
                value: "Appointment reminder"
                params:
                  - function: "appointments.getDetails"
                    inputs:
                      appointmentId: "123"
`,
			wantErr: false,
		},
		{
			name: "valid context with params - short function name (cross-tool reference)",
			yaml: `
version: v1
author: test
tools:
  - name: TestTool
    description: Test tool
    version: "1.0.0"
    functions:
      - name: TriggerWorkflow
        description: Triggers a workflow
        operation: initiate_workflow
        triggers:
          - type: time_based
            cron: "0 0 9 * * *"
        steps:
          - name: start_workflow
            action: start_workflow
            with:
              userId: "user123"
              workflowType: "user"
              message: "Hello!"
              context:
                value: "Reminder context"
                params:
                  - function: "otherTool.getDetails"
                    inputs:
                      id: "literal-value"
`,
			wantErr: false,
		},
		{
			name: "valid context with multiple params",
			yaml: `
version: v1
author: test
tools:
  - name: TestTool
    description: Test tool
    version: "1.0.0"
    functions:
      - name: TriggerWorkflow
        description: Triggers a workflow
        operation: initiate_workflow
        triggers:
          - type: time_based
            cron: "0 0 9 * * *"
        steps:
          - name: start_workflow
            action: start_workflow
            with:
              userId: "user123"
              workflowType: "user"
              message: "Hello!"
              context:
                value: "Multi-function context"
                params:
                  - function: "tool1.func1"
                    inputs:
                      input1: "value1"
                  - function: "tool2.func2"
                    inputs:
                      input2: "value2"
                      input3: "value3"
`,
			wantErr: false,
		},
		{
			name: "valid context with params using variable replacement",
			yaml: `
version: v1
author: test
tools:
  - name: TestTool
    description: Test tool
    version: "1.0.0"
    functions:
      - name: GetAppointments
        description: Get appointments
        operation: api_call
        triggers:
          - type: time_based
            cron: "0 0 8 * * *"
        private: true
        steps:
          - name: get
            action: GET
            with:
              url: "http://api.example.com/appointments"
        output:
          type: list[object]
          fields:
            - name: id
              type: string
            - name: userId
              type: string
            - name: patientName
              type: string
            - name: date
              type: string
      - name: TriggerReminders
        description: Triggers reminders
        operation: initiate_workflow
        triggers:
          - type: time_based
            cron: "0 0 9 * * *"
        needs:
          - GetAppointments
        input:
          - name: "appointments"
            type: "list[object]"
            description: "List of appointments"
            origin: "function"
            from: "GetAppointments"
            onError:
              strategy: "requestN1Support"
              message: "Failed to get appointments"
        steps:
          - name: start_workflow
            action: start_workflow
            foreach:
              items: "$appointments"
              itemVar: "appt"
            with:
              userId: "$appt.userId"
              workflowType: "user"
              message: "Reminder for $appt.date"
              context:
                value: "Appointment reminder for $appt.patientName"
                params:
                  - function: "appointments.confirm"
                    inputs:
                      appointmentId: "$appt.id"
                      appointmentDate: "$appt.date"
`,
			wantErr: false,
		},
		{
			name: "invalid context - object without value",
			yaml: `
version: v1
author: test
tools:
  - name: TestTool
    description: Test tool
    version: "1.0.0"
    functions:
      - name: TriggerWorkflow
        description: Triggers a workflow
        operation: initiate_workflow
        triggers:
          - type: time_based
            cron: "0 0 9 * * *"
        steps:
          - name: start_workflow
            action: start_workflow
            with:
              userId: "user123"
              workflowType: "user"
              message: "Hello!"
              context:
                params:
                  - function: "test.func"
                    inputs:
                      id: "123"
`,
			wantErr: true,
			errMsg:  "context object must have a 'value' field",
		},
		{
			name: "invalid context - params not array",
			yaml: `
version: v1
author: test
tools:
  - name: TestTool
    description: Test tool
    version: "1.0.0"
    functions:
      - name: TriggerWorkflow
        description: Triggers a workflow
        operation: initiate_workflow
        triggers:
          - type: time_based
            cron: "0 0 9 * * *"
        steps:
          - name: start_workflow
            action: start_workflow
            with:
              userId: "user123"
              workflowType: "user"
              message: "Hello!"
              context:
                value: "Context"
                params: "not-an-array"
`,
			wantErr: true,
			errMsg:  "context.params must be an array",
		},
		{
			name: "invalid context - param without function",
			yaml: `
version: v1
author: test
tools:
  - name: TestTool
    description: Test tool
    version: "1.0.0"
    functions:
      - name: TriggerWorkflow
        description: Triggers a workflow
        operation: initiate_workflow
        triggers:
          - type: time_based
            cron: "0 0 9 * * *"
        steps:
          - name: start_workflow
            action: start_workflow
            with:
              userId: "user123"
              workflowType: "user"
              message: "Hello!"
              context:
                value: "Context"
                params:
                  - inputs:
                      id: "123"
`,
			wantErr: true,
			errMsg:  "must have a 'function' field",
		},
		{
			name: "invalid context - param without inputs",
			yaml: `
version: v1
author: test
tools:
  - name: TestTool
    description: Test tool
    version: "1.0.0"
    functions:
      - name: TriggerWorkflow
        description: Triggers a workflow
        operation: initiate_workflow
        triggers:
          - type: time_based
            cron: "0 0 9 * * *"
        steps:
          - name: start_workflow
            action: start_workflow
            with:
              userId: "user123"
              workflowType: "user"
              message: "Hello!"
              context:
                value: "Context"
                params:
                  - function: "test.func"
`,
			wantErr: true,
			errMsg:  "must have an 'inputs' field",
		},
		{
			name: "invalid context - empty function name",
			yaml: `
version: v1
author: test
tools:
  - name: TestTool
    description: Test tool
    version: "1.0.0"
    functions:
      - name: TriggerWorkflow
        description: Triggers a workflow
        operation: initiate_workflow
        triggers:
          - type: time_based
            cron: "0 0 9 * * *"
        steps:
          - name: start_workflow
            action: start_workflow
            with:
              userId: "user123"
              workflowType: "user"
              message: "Hello!"
              context:
                value: "Context"
                params:
                  - function: ""
                    inputs:
                      id: "123"
`,
			wantErr: true,
			errMsg:  "must be a non-empty string",
		},
		{
			name: "invalid context - inputs not an object",
			yaml: `
version: v1
author: test
tools:
  - name: TestTool
    description: Test tool
    version: "1.0.0"
    functions:
      - name: TriggerWorkflow
        description: Triggers a workflow
        operation: initiate_workflow
        triggers:
          - type: time_based
            cron: "0 0 9 * * *"
        steps:
          - name: start_workflow
            action: start_workflow
            with:
              userId: "user123"
              workflowType: "user"
              message: "Hello!"
              context:
                value: "Context"
                params:
                  - function: "test.func"
                    inputs: "not-an-object"
`,
			wantErr: true,
			errMsg:  "inputs must be an object",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := CreateTool(tt.yaml)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error containing %q, but got no error", tt.errMsg)
					return
				}
				if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("error message = %q, want it to contain %q", err.Error(), tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

// TestParseWorkflowContext tests the ParseWorkflowContext function
func TestParseWorkflowContext(t *testing.T) {
	tests := []struct {
		name       string
		input      interface{}
		wantErr    bool
		errMsg     string
		wantValue  string
		wantParams int
	}{
		{
			name:       "simple string context",
			input:      "This is a simple context",
			wantErr:    false,
			wantValue:  "This is a simple context",
			wantParams: 0,
		},
		{
			name:       "empty string context",
			input:      "",
			wantErr:    false,
			wantValue:  "",
			wantParams: 0,
		},
		{
			name: "object context with value only",
			input: map[string]interface{}{
				"value": "Context with value",
			},
			wantErr:    false,
			wantValue:  "Context with value",
			wantParams: 0,
		},
		{
			name: "object context with value and empty params",
			input: map[string]interface{}{
				"value":  "Context",
				"params": []interface{}{},
			},
			wantErr:    false,
			wantValue:  "Context",
			wantParams: 0,
		},
		{
			name: "object context with params",
			input: map[string]interface{}{
				"value": "Context with params",
				"params": []interface{}{
					map[string]interface{}{
						"function": "tool.func",
						"inputs": map[string]interface{}{
							"input1": "value1",
						},
					},
				},
			},
			wantErr:    false,
			wantValue:  "Context with params",
			wantParams: 1,
		},
		{
			name: "object context with multiple params",
			input: map[string]interface{}{
				"value": "Multi-param context",
				"params": []interface{}{
					map[string]interface{}{
						"function": "tool1.func1",
						"inputs": map[string]interface{}{
							"a": "1",
						},
					},
					map[string]interface{}{
						"function": "tool2.func2",
						"inputs": map[string]interface{}{
							"b": "2",
							"c": "3",
						},
					},
				},
			},
			wantErr:    false,
			wantValue:  "Multi-param context",
			wantParams: 2,
		},
		{
			name:    "nil input",
			input:   nil,
			wantErr: true,
			errMsg:  "context is required",
		},
		{
			name: "object without value",
			input: map[string]interface{}{
				"params": []interface{}{},
			},
			wantErr: true,
			errMsg:  "context.value is required",
		},
		{
			name: "object with non-string value",
			input: map[string]interface{}{
				"value": 123,
			},
			wantErr: true,
			errMsg:  "context.value is required and must be a string",
		},
		{
			name: "params not an array",
			input: map[string]interface{}{
				"value":  "Context",
				"params": "not-array",
			},
			wantErr: true,
			errMsg:  "context.params must be an array",
		},
		{
			name: "param without function",
			input: map[string]interface{}{
				"value": "Context",
				"params": []interface{}{
					map[string]interface{}{
						"inputs": map[string]interface{}{},
					},
				},
			},
			wantErr: true,
			errMsg:  "context.params[0].function is required",
		},
		{
			name: "param without inputs",
			input: map[string]interface{}{
				"value": "Context",
				"params": []interface{}{
					map[string]interface{}{
						"function": "test.func",
					},
				},
			},
			wantErr: true,
			errMsg:  "context.params[0].inputs is required",
		},
		{
			name:    "invalid type",
			input:   123,
			wantErr: true,
			errMsg:  "context must be a string or an object",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseWorkflowContext(tt.input)

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error containing %q, but got no error", tt.errMsg)
					return
				}
				if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("error message = %q, want it to contain %q", err.Error(), tt.errMsg)
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if result == nil {
				t.Error("expected non-nil result")
				return
			}

			if result.Value != tt.wantValue {
				t.Errorf("Value = %q, want %q", result.Value, tt.wantValue)
			}

			if len(result.Params) != tt.wantParams {
				t.Errorf("len(Params) = %d, want %d", len(result.Params), tt.wantParams)
			}
		})
	}
}

// TestParseWorkflowContextParamDetails tests detailed parsing of context params
func TestParseWorkflowContextParamDetails(t *testing.T) {
	input := map[string]interface{}{
		"value": "Test context",
		"params": []interface{}{
			map[string]interface{}{
				"function": "myTool.myFunction",
				"inputs": map[string]interface{}{
					"appointmentId": "$appt.id",
					"patientName":   "John Doe",
					"numericValue":  42,
					"booleanValue":  true,
				},
			},
		},
	}

	result, err := ParseWorkflowContext(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Params) != 1 {
		t.Fatalf("expected 1 param, got %d", len(result.Params))
	}

	param := result.Params[0]

	if param.Function != "myTool.myFunction" {
		t.Errorf("Function = %q, want %q", param.Function, "myTool.myFunction")
	}

	if len(param.Inputs) != 4 {
		t.Errorf("len(Inputs) = %d, want 4", len(param.Inputs))
	}

	// Check string input
	if v, ok := param.Inputs["appointmentId"].(string); !ok || v != "$appt.id" {
		t.Errorf("appointmentId = %v, want %q", param.Inputs["appointmentId"], "$appt.id")
	}

	// Check literal string
	if v, ok := param.Inputs["patientName"].(string); !ok || v != "John Doe" {
		t.Errorf("patientName = %v, want %q", param.Inputs["patientName"], "John Doe")
	}

	// Check numeric value preserved
	if v, ok := param.Inputs["numericValue"].(int); !ok || v != 42 {
		t.Errorf("numericValue = %v (type %T), want 42", param.Inputs["numericValue"], param.Inputs["numericValue"])
	}

	// Check boolean value preserved
	if v, ok := param.Inputs["booleanValue"].(bool); !ok || v != true {
		t.Errorf("booleanValue = %v (type %T), want true", param.Inputs["booleanValue"], param.Inputs["booleanValue"])
	}
}

// TestInitiateWorkflowContextParamsFunctionReferences tests validation of function references
// in context.params, ensuring functions exist and have compatible triggers for the workflow type.
func TestInitiateWorkflowContextParamsFunctionReferences(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid - user workflow with flex_for_user trigger",
			yaml: `
version: v1
author: test
tools:
  - name: TestTool
    description: Test tool
    version: "1.0.0"
    functions:
      - name: SendMessage
        description: Send a message
        operation: api_call
        triggers:
          - type: flex_for_user
        input:
          - name: messageContent
            type: string
            description: The message content
            isOptional: true
        steps:
          - name: post
            action: POST
            with:
              url: "http://api.example.com/send"
        output:
          type: string
          value: "sent"
      - name: TriggerWorkflow
        description: Triggers a workflow
        operation: initiate_workflow
        triggers:
          - type: time_based
            cron: "0 0 9 * * *"
        steps:
          - name: start_workflow
            action: start_workflow
            with:
              userId: "user123"
              workflowType: "user"
              message: "Hello!"
              context:
                value: "Reminder context"
                params:
                  - function: "TestTool.SendMessage"
                    inputs:
                      messageContent: "Hello from workflow"
`,
			wantErr: false,
		},
		{
			name: "valid - team workflow with flex_for_team trigger",
			yaml: `
version: v1
author: test
tools:
  - name: TestTool
    description: Test tool
    version: "1.0.0"
    functions:
      - name: SendTeamNotification
        description: Send team notification
        operation: api_call
        triggers:
          - type: flex_for_team
        input:
          - name: notificationContent
            type: string
            description: The notification content
            isOptional: true
        steps:
          - name: post
            action: POST
            with:
              url: "http://api.example.com/notify"
        output:
          type: string
          value: "notified"
      - name: TriggerTeamWorkflow
        description: Triggers a team workflow
        operation: initiate_workflow
        triggers:
          - type: time_based
            cron: "0 0 9 * * *"
        steps:
          - name: start_workflow
            action: start_workflow
            with:
              userId: "team123"
              workflowType: "team"
              message: "Team notification"
              context:
                value: "Team context"
                params:
                  - function: "TestTool.SendTeamNotification"
                    inputs:
                      notificationContent: "Hello team"
`,
			wantErr: false,
		},
		{
			name: "valid - user workflow with always_on_user_message trigger",
			yaml: `
version: v1
author: test
tools:
  - name: TestTool
    description: Test tool
    version: "1.0.0"
    functions:
      - name: ProcessMessage
        description: Process user message
        operation: api_call
        triggers:
          - type: always_on_user_message
        input:
          - name: content
            type: string
            description: Content
            isOptional: true
        steps:
          - name: get
            action: GET
            with:
              url: "http://api.example.com/process"
        output:
          type: string
          value: "processed"
      - name: TriggerWorkflow
        description: Triggers a workflow
        operation: initiate_workflow
        triggers:
          - type: time_based
            cron: "0 0 9 * * *"
        steps:
          - name: start_workflow
            action: start_workflow
            with:
              userId: "user123"
              workflowType: "user"
              message: "Hello!"
              context:
                value: "Context"
                params:
                  - function: "TestTool.ProcessMessage"
                    inputs:
                      content: "test"
`,
			wantErr: false,
		},
		{
			name: "valid - team workflow with always_on_team_message trigger",
			yaml: `
version: v1
author: test
tools:
  - name: TestTool
    description: Test tool
    version: "1.0.0"
    functions:
      - name: ProcessTeamMessage
        description: Process team message
        operation: api_call
        triggers:
          - type: always_on_team_message
        input:
          - name: content
            type: string
            description: Content
            isOptional: true
        steps:
          - name: get
            action: GET
            with:
              url: "http://api.example.com/team"
        output:
          type: string
          value: "done"
      - name: TriggerWorkflow
        description: Triggers a workflow
        operation: initiate_workflow
        triggers:
          - type: time_based
            cron: "0 0 9 * * *"
        steps:
          - name: start_workflow
            action: start_workflow
            with:
              userId: "team123"
              workflowType: "team"
              message: "Team!"
              context:
                value: "Context"
                params:
                  - function: "TestTool.ProcessTeamMessage"
                    inputs:
                      content: "test"
`,
			wantErr: false,
		},
		{
			name: "valid - function with both user and team triggers",
			yaml: `
version: v1
author: test
tools:
  - name: TestTool
    description: Test tool
    version: "1.0.0"
    functions:
      - name: UniversalMessage
        description: Universal message function
        operation: api_call
        triggers:
          - type: flex_for_user
          - type: flex_for_team
        input:
          - name: content
            type: string
            description: Content
            isOptional: true
        steps:
          - name: post
            action: POST
            with:
              url: "http://api.example.com/universal"
        output:
          type: string
          value: "sent"
      - name: TriggerUserWorkflow
        description: Triggers a user workflow
        operation: initiate_workflow
        triggers:
          - type: time_based
            cron: "0 0 9 * * *"
        steps:
          - name: start_workflow
            action: start_workflow
            with:
              userId: "user123"
              workflowType: "user"
              message: "Hello!"
              context:
                value: "Context"
                params:
                  - function: "TestTool.UniversalMessage"
                    inputs:
                      content: "test"
      - name: TriggerTeamWorkflow
        description: Triggers a team workflow
        operation: initiate_workflow
        triggers:
          - type: time_based
            cron: "0 0 10 * * *"
        steps:
          - name: start_workflow
            action: start_workflow
            with:
              userId: "team123"
              workflowType: "team"
              message: "Team!"
              context:
                value: "Context"
                params:
                  - function: "TestTool.UniversalMessage"
                    inputs:
                      content: "test"
`,
			wantErr: false,
		},
		{
			name: "valid - short function name (same tool)",
			yaml: `
version: v1
author: test
tools:
  - name: TestTool
    description: Test tool
    version: "1.0.0"
    functions:
      - name: LocalFunction
        description: A local function
        operation: api_call
        triggers:
          - type: flex_for_user
        input:
          - name: value
            type: string
            description: Value
            isOptional: true
        steps:
          - name: get
            action: GET
            with:
              url: "http://api.example.com/local"
        output:
          type: string
          value: "done"
      - name: TriggerWorkflow
        description: Triggers a workflow
        operation: initiate_workflow
        triggers:
          - type: time_based
            cron: "0 0 9 * * *"
        steps:
          - name: start_workflow
            action: start_workflow
            with:
              userId: "user123"
              workflowType: "user"
              message: "Hello!"
              context:
                value: "Context"
                params:
                  - function: "LocalFunction"
                    inputs:
                      value: "test"
`,
			wantErr: false,
		},
		{
			name: "valid - reference to another tool (not validated)",
			yaml: `
version: v1
author: test
tools:
  - name: TestTool
    description: Test tool
    version: "1.0.0"
    functions:
      - name: TriggerWorkflow
        description: Triggers a workflow
        operation: initiate_workflow
        triggers:
          - type: time_based
            cron: "0 0 9 * * *"
        steps:
          - name: start_workflow
            action: start_workflow
            with:
              userId: "user123"
              workflowType: "user"
              message: "Hello!"
              context:
                value: "Context"
                params:
                  - function: "OtherTool.SomeFunction"
                    inputs:
                      value: "test"
`,
			wantErr: false, // Cannot validate cross-tool references at parse time
		},
		{
			name: "invalid - user workflow with flex_for_team trigger only",
			yaml: `
version: v1
author: test
tools:
  - name: TestTool
    description: Test tool
    version: "1.0.0"
    functions:
      - name: TeamOnlyFunction
        description: Team only function
        operation: api_call
        triggers:
          - type: flex_for_team
        input:
          - name: content
            type: string
            description: Content
            isOptional: true
        steps:
          - name: get
            action: GET
            with:
              url: "http://api.example.com/team"
        output:
          type: string
          value: "done"
      - name: TriggerWorkflow
        description: Triggers a workflow
        operation: initiate_workflow
        triggers:
          - type: time_based
            cron: "0 0 9 * * *"
        steps:
          - name: start_workflow
            action: start_workflow
            with:
              userId: "user123"
              workflowType: "user"
              message: "Hello!"
              context:
                value: "Context"
                params:
                  - function: "TestTool.TeamOnlyFunction"
                    inputs:
                      content: "test"
`,
			wantErr: true,
			errMsg:  "has triggers [flex_for_team], but workflowType 'user' requires one of: flex_for_user or always_on_user_message",
		},
		{
			name: "invalid - team workflow with flex_for_user trigger only",
			yaml: `
version: v1
author: test
tools:
  - name: TestTool
    description: Test tool
    version: "1.0.0"
    functions:
      - name: UserOnlyFunction
        description: User only function
        operation: api_call
        triggers:
          - type: flex_for_user
        input:
          - name: content
            type: string
            description: Content
            isOptional: true
        steps:
          - name: get
            action: GET
            with:
              url: "http://api.example.com/user"
        output:
          type: string
          value: "done"
      - name: TriggerWorkflow
        description: Triggers a workflow
        operation: initiate_workflow
        triggers:
          - type: time_based
            cron: "0 0 9 * * *"
        steps:
          - name: start_workflow
            action: start_workflow
            with:
              userId: "team123"
              workflowType: "team"
              message: "Team!"
              context:
                value: "Context"
                params:
                  - function: "TestTool.UserOnlyFunction"
                    inputs:
                      content: "test"
`,
			wantErr: true,
			errMsg:  "has triggers [flex_for_user], but workflowType 'team' requires one of: flex_for_team or always_on_team_message",
		},
		{
			name: "invalid - function does not exist in tool",
			yaml: `
version: v1
author: test
tools:
  - name: TestTool
    description: Test tool
    version: "1.0.0"
    functions:
      - name: TriggerWorkflow
        description: Triggers a workflow
        operation: initiate_workflow
        triggers:
          - type: time_based
            cron: "0 0 9 * * *"
        steps:
          - name: start_workflow
            action: start_workflow
            with:
              userId: "user123"
              workflowType: "user"
              message: "Hello!"
              context:
                value: "Context"
                params:
                  - function: "TestTool.NonExistentFunction"
                    inputs:
                      content: "test"
`,
			wantErr: true,
			errMsg:  "references function 'NonExistentFunction' which does not exist in tool 'TestTool'",
		},
		{
			name: "invalid - short function name does not exist",
			yaml: `
version: v1
author: test
tools:
  - name: TestTool
    description: Test tool
    version: "1.0.0"
    functions:
      - name: TriggerWorkflow
        description: Triggers a workflow
        operation: initiate_workflow
        triggers:
          - type: time_based
            cron: "0 0 9 * * *"
        steps:
          - name: start_workflow
            action: start_workflow
            with:
              userId: "user123"
              workflowType: "user"
              message: "Hello!"
              context:
                value: "Context"
                params:
                  - function: "DoesNotExist"
                    inputs:
                      content: "test"
`,
			wantErr: true,
			errMsg:  "references function 'DoesNotExist' which does not exist",
		},
		{
			name: "invalid - function with time_based trigger only (not available for user workflows)",
			yaml: `
version: v1
author: test
tools:
  - name: TestTool
    description: Test tool
    version: "1.0.0"
    functions:
      - name: ScheduledFunction
        description: A scheduled function
        operation: api_call
        triggers:
          - type: time_based
            cron: "0 0 8 * * *"
        steps:
          - name: get
            action: GET
            with:
              url: "http://api.example.com/data"
        output:
          type: string
          value: "done"
      - name: TriggerWorkflow
        description: Triggers a workflow
        operation: initiate_workflow
        triggers:
          - type: time_based
            cron: "0 0 9 * * *"
        steps:
          - name: start_workflow
            action: start_workflow
            with:
              userId: "user123"
              workflowType: "user"
              message: "Hello!"
              context:
                value: "Context"
                params:
                  - function: "TestTool.ScheduledFunction"
                    inputs:
                      dummy: "value"
`,
			wantErr: true,
			errMsg:  "has triggers [time_based], but workflowType 'user' requires one of: flex_for_user or always_on_user_message",
		},
		{
			name: "valid - no context params (simple string context)",
			yaml: `
version: v1
author: test
tools:
  - name: TestTool
    description: Test tool
    version: "1.0.0"
    functions:
      - name: TriggerWorkflow
        description: Triggers a workflow
        operation: initiate_workflow
        triggers:
          - type: time_based
            cron: "0 0 9 * * *"
        steps:
          - name: start_workflow
            action: start_workflow
            with:
              userId: "user123"
              workflowType: "user"
              message: "Hello!"
              context: "Simple string context"
`,
			wantErr: false,
		},
		{
			name: "valid - context object with empty params",
			yaml: `
version: v1
author: test
tools:
  - name: TestTool
    description: Test tool
    version: "1.0.0"
    functions:
      - name: TriggerWorkflow
        description: Triggers a workflow
        operation: initiate_workflow
        triggers:
          - type: time_based
            cron: "0 0 9 * * *"
        steps:
          - name: start_workflow
            action: start_workflow
            with:
              userId: "user123"
              workflowType: "user"
              message: "Hello!"
              context:
                value: "Context with no params"
`,
			wantErr: false,
		},
		{
			name: "valid - multiple params all with compatible triggers",
			yaml: `
version: v1
author: test
tools:
  - name: TestTool
    description: Test tool
    version: "1.0.0"
    functions:
      - name: Function1
        description: Function 1
        operation: api_call
        triggers:
          - type: flex_for_user
        input:
          - name: a
            type: string
            description: A
            isOptional: true
        steps:
          - name: get
            action: GET
            with:
              url: "http://api.example.com/func1"
        output:
          type: string
          value: "done1"
      - name: Function2
        description: Function 2
        operation: api_call
        triggers:
          - type: always_on_user_message
        input:
          - name: b
            type: string
            description: B
            isOptional: true
        steps:
          - name: get
            action: GET
            with:
              url: "http://api.example.com/func2"
        output:
          type: string
          value: "done2"
      - name: TriggerWorkflow
        description: Triggers a workflow
        operation: initiate_workflow
        triggers:
          - type: time_based
            cron: "0 0 9 * * *"
        steps:
          - name: start_workflow
            action: start_workflow
            with:
              userId: "user123"
              workflowType: "user"
              message: "Hello!"
              context:
                value: "Multi-function context"
                params:
                  - function: "TestTool.Function1"
                    inputs:
                      a: "1"
                  - function: "TestTool.Function2"
                    inputs:
                      b: "2"
`,
			wantErr: false,
		},
		{
			name: "invalid - one of multiple params has incompatible trigger",
			yaml: `
version: v1
author: test
tools:
  - name: TestTool
    description: Test tool
    version: "1.0.0"
    functions:
      - name: UserFunction
        description: User function
        operation: api_call
        triggers:
          - type: flex_for_user
        input:
          - name: a
            type: string
            description: A
            isOptional: true
        steps:
          - name: get
            action: GET
            with:
              url: "http://api.example.com/user"
        output:
          type: string
          value: "user"
      - name: TeamFunction
        description: Team function
        operation: api_call
        triggers:
          - type: flex_for_team
        input:
          - name: b
            type: string
            description: B
            isOptional: true
        steps:
          - name: get
            action: GET
            with:
              url: "http://api.example.com/team"
        output:
          type: string
          value: "team"
      - name: TriggerWorkflow
        description: Triggers a workflow
        operation: initiate_workflow
        triggers:
          - type: time_based
            cron: "0 0 9 * * *"
        steps:
          - name: start_workflow
            action: start_workflow
            with:
              userId: "user123"
              workflowType: "user"
              message: "Hello!"
              context:
                value: "Multi-function context"
                params:
                  - function: "TestTool.UserFunction"
                    inputs:
                      a: "1"
                  - function: "TestTool.TeamFunction"
                    inputs:
                      b: "2"
`,
			wantErr: true,
			errMsg:  "references function 'TeamFunction' which has triggers [flex_for_team], but workflowType 'user' requires",
		},
		{
			name: "valid - function with no triggers but reachable from triggered function (edge case)",
			yaml: `
version: v1
author: test
tools:
  - name: TestTool
    description: Test tool
    version: "1.0.0"
    functions:
      - name: TriggeredFunction
        description: A triggered function
        operation: api_call
        triggers:
          - type: flex_for_user
        needs:
          - helperFunction
        input:
          - name: data
            type: string
            description: Data
            isOptional: true
        steps:
          - name: get
            action: GET
            with:
              url: "http://api.example.com/triggered"
        output:
          type: string
          value: "done"
      - name: helperFunction
        description: A helper function without triggers
        operation: api_call
        input:
          - name: input
            type: string
            description: Input
            isOptional: true
        steps:
          - name: get
            action: GET
            with:
              url: "http://api.example.com/helper"
        output:
          type: string
          value: "helped"
      - name: TriggerWorkflow
        description: Triggers a workflow
        operation: initiate_workflow
        triggers:
          - type: time_based
            cron: "0 0 9 * * *"
        steps:
          - name: start_workflow
            action: start_workflow
            with:
              userId: "user123"
              workflowType: "user"
              message: "Hello!"
              context:
                value: "Context"
                params:
                  - function: "TestTool.TriggeredFunction"
                    inputs:
                      data: "test"
`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := CreateTool(tt.yaml)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error containing %q, but got no error", tt.errMsg)
					return
				}
				if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("error message = %q, want it to contain %q", err.Error(), tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

// TestParseContextParamFunctionKey tests the function key parsing
func TestParseContextParamFunctionKey(t *testing.T) {
	tests := []struct {
		input        string
		wantTool     string
		wantFunction string
	}{
		{"ToolName.FunctionName", "ToolName", "FunctionName"},
		{"FunctionName", "", "FunctionName"},
		{"Tool.Func.Extra", "Tool", "Func.Extra"}, // SplitN with limit 2
		{"", "", ""},
		{".", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			gotTool, gotFunc := ParseContextParamFunctionKey(tt.input)
			if gotTool != tt.wantTool {
				t.Errorf("toolName = %q, want %q", gotTool, tt.wantTool)
			}
			if gotFunc != tt.wantFunction {
				t.Errorf("functionName = %q, want %q", gotFunc, tt.wantFunction)
			}
		})
	}
}

// TestInitiateWorkflowWorkflowFieldValidation tests the workflow field validation in initiate_workflow
func TestInitiateWorkflowWorkflowFieldValidation(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid - workflow field matches defined workflow",
			yaml: `
version: v1
author: test
tools:
  - name: TestTool
    description: Test tool
    version: "1.0.0"
    functions:
      - name: GreetUser
        description: Greets the user
        operation: policy
        triggers:
          - type: flex_for_user
        output:
          type: string
          value: "Hello!"
      - name: TriggerWorkflow
        description: Triggers a workflow
        operation: initiate_workflow
        triggers:
          - type: time_based
            cron: "0 0 9 * * *"
        steps:
          - name: start_workflow
            action: start_workflow
            with:
              userId: "user123"
              workflowType: "user"
              message: "Hello!"
              context: "Daily check-in"
              workflow: "initial_engagement"
workflows:
  - category: initial_engagement
    human_category: "Initial Engagement"
    description: "Initial engagement workflow"
    workflow_type: "user"
    steps:
      - action: "GreetUser"
        human_action: "Greet"
        instructions: "Greet the user"
`,
			wantErr: false,
		},
		{
			name: "valid - workflow field with multiple workflows defined",
			yaml: `
version: v1
author: test
tools:
  - name: TestTool
    description: Test tool
    version: "1.0.0"
    functions:
      - name: GreetUser
        description: Greets the user
        operation: policy
        triggers:
          - type: flex_for_user
        output:
          type: string
          value: "Hello!"
      - name: SendFollowUp
        description: Sends follow up
        operation: policy
        triggers:
          - type: flex_for_user
        output:
          type: string
          value: "Follow up!"
      - name: TriggerWorkflow
        description: Triggers a workflow
        operation: initiate_workflow
        triggers:
          - type: time_based
            cron: "0 0 9 * * *"
        steps:
          - name: start_workflow
            action: start_workflow
            with:
              userId: "user123"
              workflowType: "user"
              message: "Hello!"
              context: "Follow up"
              workflow: "follow_up"
workflows:
  - category: initial_engagement
    human_category: "Initial Engagement"
    description: "Initial engagement"
    workflow_type: "user"
    steps:
      - action: "GreetUser"
        human_action: "Greet"
        instructions: "Greet the user"
  - category: follow_up
    human_category: "Follow Up"
    description: "Follow up workflow"
    workflow_type: "user"
    steps:
      - action: "SendFollowUp"
        human_action: "Follow Up"
        instructions: "Send follow up message"
`,
			wantErr: false,
		},
		{
			name: "valid - no workflow field (optional)",
			yaml: `
version: v1
author: test
tools:
  - name: TestTool
    description: Test tool
    version: "1.0.0"
    functions:
      - name: GreetUser
        description: Greets the user
        operation: policy
        triggers:
          - type: flex_for_user
        output:
          type: string
          value: "Hello!"
      - name: TriggerWorkflow
        description: Triggers a workflow
        operation: initiate_workflow
        triggers:
          - type: time_based
            cron: "0 0 9 * * *"
        steps:
          - name: start_workflow
            action: start_workflow
            with:
              userId: "user123"
              workflowType: "user"
              message: "Hello!"
              context: "No forced workflow"
workflows:
  - category: initial_engagement
    human_category: "Initial Engagement"
    description: "Initial engagement"
    workflow_type: "user"
    steps:
      - action: "GreetUser"
        human_action: "Greet"
        instructions: "Greet the user"
`,
			wantErr: false,
		},
		{
			name: "valid - workflow field with variable reference in foreach",
			yaml: `
version: v1
author: test
tools:
  - name: TestTool
    description: Test tool
    version: "1.0.0"
    functions:
      - name: GreetUser
        description: Greets the user
        operation: policy
        triggers:
          - type: flex_for_user
        output:
          type: string
          value: "Hello!"
      - name: GetUsers
        description: Get users
        operation: db
        triggers:
          - type: time_based
            cron: "0 0 8 * * *"
        steps:
          - name: query
            action: select
            with:
              select: "SELECT id, workflow FROM users"
              resultIndex: 1
        output:
          type: "list[object]"
          fields:
            - value: id
            - value: workflow
      - name: TriggerWorkflow
        description: Triggers a workflow
        operation: initiate_workflow
        triggers:
          - type: time_based
            cron: "0 0 9 * * *"
        needs:
          - GetUsers
        input:
          - name: "users"
            type: "list[object]"
            description: "List of users"
            origin: "function"
            from: "GetUsers"
            onError:
              strategy: "requestN1Support"
              message: "Failed to get users"
        steps:
          - name: start_workflow
            action: start_workflow
            foreach:
              items: "$users"
              itemVar: "item"
            with:
              userId: "$item.id"
              workflowType: "user"
              message: "Hello!"
              context: "Dynamic workflow"
              workflow: "$item.workflow"
workflows:
  - category: initial_engagement
    human_category: "Initial Engagement"
    description: "Initial engagement"
    workflow_type: "user"
    steps:
      - action: "GreetUser"
        human_action: "Greet"
        instructions: "Greet the user"
`,
			wantErr: false,
		},
		{
			name: "invalid - workflow field with unknown workflow",
			yaml: `
version: v1
author: test
tools:
  - name: TestTool
    description: Test tool
    version: "1.0.0"
    functions:
      - name: GreetUser
        description: Greets the user
        operation: policy
        triggers:
          - type: flex_for_user
        output:
          type: string
          value: "Hello!"
      - name: TriggerWorkflow
        description: Triggers a workflow
        operation: initiate_workflow
        triggers:
          - type: time_based
            cron: "0 0 9 * * *"
        steps:
          - name: start_workflow
            action: start_workflow
            with:
              userId: "user123"
              workflowType: "user"
              message: "Hello!"
              context: "Unknown workflow"
              workflow: "non_existent_workflow"
workflows:
  - category: initial_engagement
    human_category: "Initial Engagement"
    description: "Initial engagement"
    workflow_type: "user"
    steps:
      - action: "GreetUser"
        human_action: "Greet"
        instructions: "Greet the user"
`,
			wantErr: true,
			errMsg:  "specifies unknown workflow 'non_existent_workflow'",
		},
		{
			name: "invalid - workflow field when no workflows defined",
			yaml: `
version: v1
author: test
tools:
  - name: TestTool
    description: Test tool
    version: "1.0.0"
    functions:
      - name: TriggerWorkflow
        description: Triggers a workflow
        operation: initiate_workflow
        triggers:
          - type: time_based
            cron: "0 0 9 * * *"
        steps:
          - name: start_workflow
            action: start_workflow
            with:
              userId: "user123"
              workflowType: "user"
              message: "Hello!"
              context: "No workflows defined"
              workflow: "some_workflow"
`,
			wantErr: true,
			errMsg:  "but no workflows are defined in the YAML file",
		},
		{
			name: "invalid - workflow field with non-string type",
			yaml: `
version: v1
author: test
tools:
  - name: TestTool
    description: Test tool
    version: "1.0.0"
    functions:
      - name: GreetUser
        description: Greets the user
        operation: policy
        triggers:
          - type: flex_for_user
        output:
          type: string
          value: "Hello!"
      - name: TriggerWorkflow
        description: Triggers a workflow
        operation: initiate_workflow
        triggers:
          - type: time_based
            cron: "0 0 9 * * *"
        steps:
          - name: start_workflow
            action: start_workflow
            with:
              userId: "user123"
              workflowType: "user"
              message: "Hello!"
              context: "Invalid workflow type"
              workflow: 123
workflows:
  - category: initial_engagement
    human_category: "Initial Engagement"
    description: "Initial engagement"
    workflow_type: "user"
    steps:
      - action: "GreetUser"
        human_action: "Greet"
        instructions: "Greet the user"
`,
			wantErr: true,
			errMsg:  "invalid workflow value; must be a string or variable reference",
		},
		{
			name: "valid - workflow field in foreach",
			yaml: `
version: v1
author: test
tools:
  - name: TestTool
    description: Test tool
    version: "1.0.0"
    functions:
      - name: SendMessage
        description: Sends a message
        operation: policy
        triggers:
          - type: flex_for_user
        output:
          type: string
          value: "Message sent!"
      - name: GetDeals
        description: Get deals from database
        operation: db
        triggers:
          - type: time_based
            cron: "0 0 8 * * *"
        steps:
          - name: query
            action: select
            with:
              select: "SELECT id, user_id FROM deals"
              resultIndex: 1
        output:
          type: "list[object]"
          fields:
            - value: id
            - value: user_id
      - name: ProcessDeals
        description: Process deals
        operation: initiate_workflow
        triggers:
          - type: time_based
            cron: "0 0 9 * * *"
        needs:
          - GetDeals
        input:
          - name: "deals"
            type: "list[object]"
            description: "List of deals"
            origin: "function"
            from: "GetDeals"
            onError:
              strategy: "requestN1Support"
              message: "Failed to get deals"
        steps:
          - name: start_workflow
            action: start_workflow
            foreach:
              items: "$deals"
              itemVar: "deal"
            with:
              userId: "$deal.user_id"
              workflowType: "user"
              message: "Deal notification"
              context: "Deal processing"
              workflow: "outbound_engagement"
workflows:
  - category: outbound_engagement
    human_category: "Outbound Engagement"
    description: "Outbound engagement workflow"
    workflow_type: "user"
    steps:
      - action: "SendMessage"
        human_action: "Send"
        instructions: "Send a message"
`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := CreateTool(tt.yaml)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error containing %q, but got no error", tt.errMsg)
					return
				}
				if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("error message = %q, want it to contain %q", err.Error(), tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}
