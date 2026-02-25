package skill

import (
	"strings"
	"testing"
)

func TestJSONSchemaValidation(t *testing.T) {
	testCases := []struct {
		name          string
		yamlInput     string
		expectError   bool
		errorContains string
	}{
		{
			name: "Valid JSON schema validator - simple object",
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
          - name: "user"
            description: "User information object"
            origin: "chat"
            jsonSchemaValidator: '{"type": "object", "properties": {"name": {"type": "string"}, "age": {"type": "number"}}, "required": ["name"]}'
            onError:
              strategy: "requestUserInput"
              message: "Please provide valid user information"
        steps:
          - name: "api call"
            action: "GET"
            with:
              url: "https://api.example.com"
`,
			expectError: false,
		},
		{
			name: "Valid JSON schema validator - array of numbers",
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
          - name: "scores"
            description: "Array of scores"
            origin: "chat"
            jsonSchemaValidator: '{"type": "array", "items": {"type": "number", "minimum": 0, "maximum": 100}}'
            onError:
              strategy: "requestUserInput"
              message: "Please provide valid scores"
        steps:
          - name: "api call"
            action: "GET"
            with:
              url: "https://api.example.com"
`,
			expectError: false,
		},
		{
			name: "Valid JSON schema validator - enum values",
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
          - name: "status"
            description: "Status value"
            origin: "chat"
            jsonSchemaValidator: '{"type": "string", "enum": ["active", "inactive", "pending"]}'
            onError:
              strategy: "requestUserInput"
              message: "Please provide a valid status"
        steps:
          - name: "api call"
            action: "GET"
            with:
              url: "https://api.example.com"
`,
			expectError: false,
		},
		{
			name: "Valid JSON schema validator - complex nested object",
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
          - name: "order"
            description: "Order information"
            origin: "chat"
            jsonSchemaValidator: '{"type": "object", "properties": {"id": {"type": "string"}, "items": {"type": "array", "items": {"type": "object", "properties": {"name": {"type": "string"}, "quantity": {"type": "number", "minimum": 1}}, "required": ["name", "quantity"]}}, "total": {"type": "number", "minimum": 0}}, "required": ["id", "items", "total"]}'
            onError:
              strategy: "requestUserInput"
              message: "Please provide valid order information"
        steps:
          - name: "api call"
            action: "GET"
            with:
              url: "https://api.example.com"
`,
			expectError: false,
		},
		{
			name: "Invalid JSON schema - malformed JSON",
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
            jsonSchemaValidator: '{"type": "object", "properties": {"name": {"type": "string"'
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
			errorContains: "invalid JSON schema validator",
		},
		{
			name: "Invalid JSON schema - invalid schema syntax",
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
            jsonSchemaValidator: '{"type": "invalid_type"}'
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
			errorContains: "invalid JSON schema validator",
		},
		{
			name: "Both regex and JSON schema validators (should fail)",
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
            regexValidator: "^\\d+$"
            jsonSchemaValidator: '{"type": "number"}'
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
			errorContains: "cannot have both regexValidator and jsonSchemaValidator specified",
		},
		{
			name: "Optional input with JSON schema validator",
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
          - name: "metadata"
            description: "Optional metadata object"
            origin: "chat"
            isOptional: true
            jsonSchemaValidator: '{"type": "object", "properties": {"tags": {"type": "array", "items": {"type": "string"}}, "category": {"type": "string"}}, "additionalProperties": false}'
        steps:
          - name: "api call"
            action: "GET"
            with:
              url: "https://api.example.com"
`,
			expectError: false,
		},
		{
			name: "JSON schema with inference origin",
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
          - name: "contact"
            description: "Contact information extracted from message"
            origin: "inference"
            successCriteria: "extract contact information from the user message"
            jsonSchemaValidator: '{"type": "object", "properties": {"name": {"type": "string"}, "email": {"type": "string", "format": "email"}, "phone": {"type": "string"}}, "required": ["name"]}'
            onError:
              strategy: "requestUserInput"
              message: "Please provide contact information"
        steps:
          - name: "api call"
            action: "GET"
            with:
              url: "https://api.example.com"
`,
			expectError: false,
		},
		{
			name: "JSON schema with date format validation",
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
          - name: "event"
            description: "Event with date information"
            origin: "chat"
            jsonSchemaValidator: '{"type": "object", "properties": {"title": {"type": "string"}, "date": {"type": "string", "format": "date"}, "time": {"type": "string", "pattern": "^([01]?[0-9]|2[0-3]):[0-5][0-9]$"}}, "required": ["title", "date"]}'
            onError:
              strategy: "requestUserInput"
              message: "Please provide valid event information"
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
