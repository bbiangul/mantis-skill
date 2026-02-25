package skill

import (
	"strings"
	"testing"
)

func TestDotNotationValidation(t *testing.T) {
	testCases := []struct {
		name          string
		yamlInput     string
		expectError   bool
		errorContains string
	}{
		{
			name: "Valid: No dot notation usage",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "getUser"
        operation: "terminal"
        description: "Get user data"
        triggers:
          - type: "flex_for_user"
        steps:
          - name: "fetch"
            action: "bash"
            with:
              linux: |
                echo '{"id": 123, "email": "test@example.com"}'
              windows: |
                @echo off
                echo {"id": 123, "email": "test@example.com"}
            resultIndex: 1
      - name: "sendEmail"
        operation: "api_call"
        description: "Send email"
        triggers:
          - type: "flex_for_user"
        needs: ["getUser"]
        input:
          - name: "userData"
            description: "User data from previous function"
            origin: "function"
            from: "getUser"
            onError:
              strategy: "requestN1Support"
              message: "Failed to get user data"
        steps:
          - name: "call api"
            action: "POST"
            with:
              url: "https://api.example.com"
              requestBody:
                type: "application/json"
                with:
                  data: "$userData"
`,
			expectError: false,
		},
		{
			name: "Valid: Dot notation on function origin input in requestBody",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "getUser"
        operation: "terminal"
        description: "Get user data"
        triggers:
          - type: "flex_for_user"
        steps:
          - name: "fetch"
            action: "bash"
            with:
              linux: |
                echo '{"id": 123, "email": "test@example.com"}'
              windows: |
                @echo off
                echo {"id": 123, "email": "test@example.com"}
            resultIndex: 1
      - name: "sendEmail"
        operation: "api_call"
        description: "Send email"
        triggers:
          - type: "flex_for_user"
        needs: ["getUser"]
        input:
          - name: "userData"
            description: "User data from previous function"
            origin: "function"
            from: "getUser"
            onError:
              strategy: "requestN1Support"
              message: "Failed to get user data"
        steps:
          - name: "call api"
            action: "POST"
            with:
              url: "https://api.example.com"
              requestBody:
                type: "application/json"
                with:
                  email: "$userData.email"
`,
			expectError: false,
		},
		{
			name: "Valid: Dot notation on function origin input in step onError message",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "getConfig"
        operation: "terminal"
        description: "Get config"
        triggers:
          - type: "flex_for_user"
        steps:
          - name: "fetch"
            action: "bash"
            with:
              linux: |
                echo '{"apiKey": "secret", "endpoint": "https://api.example.com"}'
              windows: |
                @echo off
                echo {"apiKey": "secret", "endpoint": "https://api.example.com"}
            resultIndex: 1
      - name: "callApi"
        operation: "api_call"
        description: "Call API"
        triggers:
          - type: "flex_for_user"
        needs: ["getConfig"]
        input:
          - name: "config"
            description: "Configuration"
            origin: "function"
            from: "getConfig"
            onError:
              strategy: "requestN1Support"
              message: "Failed to get config"
        steps:
          - name: "call"
            action: "GET"
            with:
              url: "https://api.example.com"
            onError:
              strategy: "retry"
              message: "Failed to call API with endpoint $config.endpoint"
`,
			expectError: false,
		},
		{
			name: "Valid: Dot notation on function origin input in another input's onError message",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "getAccount"
        operation: "terminal"
        description: "Get account"
        triggers:
          - type: "flex_for_user"
        steps:
          - name: "fetch"
            action: "bash"
            with:
              linux: |
                echo '{"accountId": "12345", "status": "active"}'
              windows: |
                @echo off
                echo {"accountId": "12345", "status": "active"}
            resultIndex: 1
      - name: "validateAccount"
        operation: "format"
        description: "Validate account"
        triggers:
          - type: "flex_for_user"
        needs: ["getAccount"]
        input:
          - name: "accountData"
            description: "Account data"
            origin: "function"
            from: "getAccount"
            onError:
              strategy: "requestN1Support"
              message: "Failed to get account data"
          - name: "validationResult"
            description: "Validation result"
            origin: "inference"
            successCriteria:
              condition: "Validate the account"
              from: ["getAccount"]
              allowedSystemFunctions: ["askToContext"]
            onError:
              strategy: "requestUserInput"
              message: "Failed to validate account $accountData.accountId"
`,
			expectError: false,
		},
		{
			name: "Valid: Dot notation on inference origin input",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "getUser"
        operation: "terminal"
        description: "Get user data"
        triggers:
          - type: "flex_for_user"
        steps:
          - name: "fetch"
            action: "bash"
            with:
              linux: |
                echo '{"id": 123, "email": "test@example.com"}'
              windows: |
                @echo off
                echo {"id": 123, "email": "test@example.com"}
            resultIndex: 1
      - name: "sendEmail"
        operation: "api_call"
        description: "Send email"
        triggers:
          - type: "flex_for_user"
        needs: ["getUser"]
        input:
          - name: "userEmail"
            description: "User email"
            origin: "inference"
            successCriteria:
              condition: "Extract the email field"
              from: ["getUser"]
              allowedSystemFunctions: ["askToContext"]
            onError:
              strategy: "requestN1Support"
              message: "Failed to extract email"
        steps:
          - name: "call api"
            action: "POST"
            with:
              url: "https://api.example.com"
              requestBody:
                type: "application/json"
                with:
                  email: "$userEmail"
`,
			expectError: false,
		},
		{
			name: "Valid: Dot notation on chat origin input",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "TestFunction"
        operation: "terminal"
        description: "Test function"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "userInput"
            description: "User input"
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
			expectError: false,
		},
		{
			name: "Valid: System variable with dot notation",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "TestFunction"
        operation: "terminal"
        description: "Test function"
        triggers:
          - type: "flex_for_user"
        steps:
          - name: "use system var"
            action: "bash"
            with:
              linux: |
                echo "User ID: $USER.id"
              windows: |
                @echo off
                echo User ID: %USER.id%
`,
			expectError: false,
		},
		{
			name: "Valid: Dot notation on function origin input with multiple fields",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "getRecipient"
        operation: "terminal"
        description: "Get recipient data"
        triggers:
          - type: "flex_for_user"
        steps:
          - name: "fetch"
            action: "bash"
            with:
              linux: |
                echo '{"email": "john@example.com", "name": "John"}'
              windows: |
                @echo off
                echo {"email": "john@example.com", "name": "John"}
            resultIndex: 1
      - name: "sendMessage"
        operation: "api_call"
        description: "Send message"
        triggers:
          - type: "flex_for_user"
        needs: ["getRecipient"]
        input:
          - name: "recipient"
            description: "Recipient information"
            origin: "function"
            from: "getRecipient"
            onError:
              strategy: "requestN1Support"
              message: "Failed to get recipient"
        steps:
          - name: "send"
            action: "POST"
            with:
              url: "https://api.example.com/send"
              requestBody:
                type: "application/json"
                with:
                  to: "$recipient.email"
                  name: "$recipient.name"
`,
			expectError: false,
		},
		{
			name: "Valid: Proper use of inference to extract field",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "formatEmail"
        operation: "terminal"
        description: "Format email"
        triggers:
          - type: "flex_for_user"
        steps:
          - name: "format"
            action: "bash"
            with:
              linux: |
                echo '{"raw": "base64encodedstring"}'
              windows: |
                @echo off
                echo {"raw": "base64encodedstring"}
            resultIndex: 1
      - name: "sendEmail"
        operation: "api_call"
        description: "Send email via API"
        triggers:
          - type: "flex_for_user"
        needs: ["formatEmail"]
        input:
          - name: "encodedEmail"
            description: "Base64 encoded email"
            origin: "inference"
            cache: 1800
            successCriteria:
              condition: "Extract ONLY the base64 string from the formatEmail output"
              from: ["formatEmail"]
              allowedSystemFunctions: ["askToContext"]
            onError:
              strategy: "requestN1Support"
              message: "Failed to format email"
        steps:
          - name: "send"
            action: "POST"
            with:
              url: "https://gmail.googleapis.com/gmail/v1/users/me/messages/send"
              requestBody:
                type: "application/json"
                with:
                  raw: "$encodedEmail"
`,
			expectError: false,
		},
		{
			name: "Valid: Array access on function origin input with nested objects",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "getUsers"
        operation: "terminal"
        description: "Get users list"
        triggers:
          - type: "flex_for_user"
        steps:
          - name: "fetch"
            action: "bash"
            with:
              linux: |
                echo '{"users": [{"id": 1, "email": "alice@example.com"}, {"id": 2, "email": "bob@example.com"}], "total": 2}'
              windows: |
                @echo off
                echo {"users": [{"id": 1, "email": "alice@example.com"}, {"id": 2, "email": "bob@example.com"}], "total": 2}
            resultIndex: 1
      - name: "sendEmail"
        operation: "api_call"
        description: "Send email to first user"
        triggers:
          - type: "flex_for_user"
        needs: ["getUsers"]
        input:
          - name: "usersData"
            description: "Users data from database"
            origin: "function"
            from: "getUsers"
            onError:
              strategy: "requestN1Support"
              message: "Failed to get users"
        steps:
          - name: "send"
            action: "POST"
            with:
              url: "https://api.example.com/send"
              requestBody:
                type: "application/json"
                with:
                  to: "$usersData.users[0].email"
                  userId: "$usersData.users[0].id"
`,
			expectError: false,
		},
		{
			name: "Invalid: Direct array access without dot notation",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "getItems"
        operation: "terminal"
        description: "Get items list"
        triggers:
          - type: "flex_for_user"
        steps:
          - name: "fetch"
            action: "bash"
            with:
              linux: |
                echo '[{"id": 1, "name": "Item 1"}, {"id": 2, "name": "Item 2"}]'
              windows: |
                @echo off
                echo [{"id": 1, "name": "Item 1"}, {"id": 2, "name": "Item 2"}]
            resultIndex: 1
      - name: "processItem"
        operation: "api_call"
        description: "Process first item"
        triggers:
          - type: "flex_for_user"
        needs: ["getItems"]
        input:
          - name: "items"
            description: "Items array"
            origin: "function"
            from: "getItems"
            onError:
              strategy: "requestN1Support"
              message: "Failed to get items"
        steps:
          - name: "process"
            action: "POST"
            with:
              url: "https://api.example.com/process"
              requestBody:
                type: "application/json"
                with:
                  itemId: "$items[0].id"
`,
			expectError:   true,
			errorContains: "uses unsupported pattern '$items[0].id'. Direct array access on variable 'items' is not supported",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := CreateTool(tc.yamlInput)

			if tc.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				} else if tc.errorContains != "" && !strings.Contains(err.Error(), tc.errorContains) {
					t.Errorf("Expected error to contain '%s', but got: %s", tc.errorContains, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error but got: %v", err)
				}
			}
		})
	}
}

func TestDotNotationErrorMessage(t *testing.T) {
	yamlInput := `
version: "v1"
author: "Test Author"
tools:
  - name: "EmailTool"
    description: "Email tool"
    version: "1.0.0"
    functions:
      - name: "getRecipient"
        operation: "terminal"
        description: "Get recipient data"
        triggers:
          - type: "flex_for_user"
        steps:
          - name: "fetch"
            action: "bash"
            with:
              linux: |
                echo '{"email": "john@example.com", "name": "John"}'
              windows: |
                @echo off
                echo {"email": "john@example.com", "name": "John"}
            resultIndex: 1
      - name: "sendMessage"
        operation: "api_call"
        description: "Send message"
        triggers:
          - type: "flex_for_user"
        needs: ["getRecipient"]
        input:
          - name: "recipient"
            description: "Recipient information"
            origin: "function"
            from: "getRecipient"
            onError:
              strategy: "requestN1Support"
              message: "Failed to get recipient"
        steps:
          - name: "send"
            action: "POST"
            with:
              url: "https://api.example.com/send"
              requestBody:
                type: "application/json"
                with:
                  to: "$recipient.email"
`

	_, err := CreateTool(yamlInput)

	if err == nil {
		t.Fatal("Expected error but got none")
	}

	// Verify the error message contains all the important parts
	errorMessage := err.Error()
	expectedParts := []string{
		"uses dot notation '$recipient.email'",
		"on input 'recipient'",
		"which has origin 'function'",
		"Tip: Use origin 'inference' instead",
		"successCriteria",
		"from: [\"getRecipient\"]",
		"allowedSystemFunctions: [\"askToContext\"]",
		"Extract the 'email' field",
	}

	for _, part := range expectedParts {
		if !strings.Contains(errorMessage, part) {
			t.Errorf("Expected error message to contain '%s', but got: %s", part, errorMessage)
		}
	}
}
