package skill

import (
	"strings"
	"testing"
)

func TestRegexValidation(t *testing.T) {
	testCases := []struct {
		name          string
		yamlInput     string
		expectError   bool
		errorContains string
	}{
		{
			name: "Valid regex validator",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "TestFunction"
        operation: "api_call"
        description: "A test function"
        triggers:
          - type: "always_on_user_message"
        input:
          - name: "date"
            description: "Date in YYYY-MM-DD format"
            origin: "chat"
            regexValidator: "^\\d{4}-\\d{2}-\\d{2}$"
            onError:
              strategy: "requestUserInput"
              message: "Please provide a valid date"
        steps:
          - name: "api call"
            action: "GET"
            with:
              url: "https://api.example.com"
`,
			expectError: false,
		},
		{
			name: "Invalid regex pattern",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "TestFunction"
        operation: "api_call"
        description: "A test function"
        triggers:
          - type: "always_on_user_message"
        input:
          - name: "value"
            description: "Some value"
            origin: "chat"
            regexValidator: "[invalid(regex"
            onError:
              strategy: "requestUserInput"
              message: "Please provide a valid value"
        steps:
          - name: "api call"
            action: "GET"
            with:
              url: "https://api.example.com"
`,
			expectError:   true,
			errorContains: "invalid regex validator",
		},
		{
			name: "Complex regex validator for phone number",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "TestFunction"
        operation: "api_call"
        description: "A test function"
        triggers:
          - type: "always_on_user_message"
        input:
          - name: "phone"
            description: "Phone number"
            origin: "chat"
            regexValidator: "^\\+?[1-9]\\d{1,14}$"
            onError:
              strategy: "requestUserInput"
              message: "Please provide a valid phone number"
        steps:
          - name: "api call"
            action: "GET"
            with:
              url: "https://api.example.com"
`,
			expectError: false,
		},
		{
			name: "Optional input with regex validator",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "TestFunction"
        operation: "api_call"
        description: "A test function"
        triggers:
          - type: "always_on_user_message"
        input:
          - name: "email"
            description: "Email address"
            origin: "chat"
            isOptional: true
            regexValidator: "^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\\.[a-zA-Z]{2,}$"
        steps:
          - name: "api call"
            action: "GET"
            with:
              url: "https://api.example.com"
`,
			expectError: false,
		},
		{
			name: "Regex validator for decimal number",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "TestFunction"
        operation: "api_call"
        description: "A test function"
        triggers:
          - type: "always_on_user_message"
        input:
          - name: "price"
            description: "Price with exactly 2 decimal places"
            origin: "chat"
            regexValidator: "^\\d+\\.\\d{2}$"
            onError:
              strategy: "requestUserInput"
              message: "Please provide a valid price (e.g., 10.99)"
        steps:
          - name: "api call"
            action: "GET"
            with:
              url: "https://api.example.com"
`,
			expectError: false,
		},
		{
			name: "Time format regex validator",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "TestFunction"
        operation: "api_call"
        description: "A test function"
        triggers:
          - type: "always_on_user_message"
        input:
          - name: "time"
            description: "Time in HH:MM format"
            origin: "inference"
            successCriteria: "extract the time from the message"
            regexValidator: "^([01]?[0-9]|2[0-3]):[0-5][0-9]$"
            onError:
              strategy: "requestUserInput"
              message: "Please provide a valid time in HH:MM format"
        steps:
          - name: "api call"
            action: "GET"
            with:
              url: "https://api.example.com"
`,
			expectError: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := CreateTool(tc.yamlInput)

			if tc.expectError {
				if err == nil {
					t.Errorf("Expected error containing '%s', but got no error", tc.errorContains)
				} else if !strings.Contains(err.Error(), tc.errorContains) {
					t.Errorf("Expected error containing '%s', but got: %v", tc.errorContains, err)
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error, but got: %v", err)
				}
			}
		})
	}
}
