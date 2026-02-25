package skill

import (
	"strings"
	"testing"
)

func TestValidateForEach(t *testing.T) {
	tests := []struct {
		name        string
		forEach     *ForEach
		stepName    string
		funcName    string
		toolName    string
		wantErr     bool
		expectedErr string
	}{
		{
			name: "valid foreach with defaults",
			forEach: &ForEach{
				Items: "$itemList",
			},
			stepName: "testStep",
			funcName: "testFunc",
			toolName: "testTool",
			wantErr:  false,
		},
		{
			name: "valid foreach with custom values",
			forEach: &ForEach{
				Items:     "$myItems",
				Separator: ";",
				IndexVar:  "idx",
				ItemVar:   "element",
			},
			stepName: "testStep",
			funcName: "testFunc",
			toolName: "testTool",
			wantErr:  false,
		},
		{
			name: "empty items field",
			forEach: &ForEach{
				Items: "",
			},
			stepName:    "testStep",
			funcName:    "testFunc",
			toolName:    "testTool",
			wantErr:     true,
			expectedErr: "has foreach with empty items field",
		},
		{
			name: "same indexVar and itemVar",
			forEach: &ForEach{
				Items:    "$items",
				IndexVar: "same",
				ItemVar:  "same",
			},
			stepName:    "testStep",
			funcName:    "testFunc",
			toolName:    "testTool",
			wantErr:     true,
			expectedErr: "has foreach with same indexVar and itemVar 'same'",
		},
		{
			name: "invalid indexVar",
			forEach: &ForEach{
				Items:    "$items",
				IndexVar: "123invalid",
			},
			stepName:    "testStep",
			funcName:    "testFunc",
			toolName:    "testTool",
			wantErr:     true,
			expectedErr: "has foreach with invalid indexVar '123invalid'",
		},
		{
			name: "invalid itemVar",
			forEach: &ForEach{
				Items:   "$items",
				ItemVar: "invalid-name",
			},
			stepName:    "testStep",
			funcName:    "testFunc",
			toolName:    "testTool",
			wantErr:     true,
			expectedErr: "has foreach with invalid itemVar 'invalid-name'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateForEach(tt.forEach, tt.stepName, tt.funcName, tt.toolName)

			if tt.wantErr {
				if err == nil {
					t.Errorf("validateForEach() expected error but got none")
					return
				}
				if tt.expectedErr != "" && !strings.Contains(err.Error(), tt.expectedErr) {
					t.Errorf("validateForEach() error = %v, want error containing %v", err, tt.expectedErr)
				}
			} else {
				if err != nil {
					t.Errorf("validateForEach() error = %v, want nil", err)
				}
			}

			if !tt.wantErr && tt.forEach != nil {
				if tt.forEach.Separator == "" && tt.forEach.Separator != DefaultForEachSeparator {
					t.Errorf("validateForEach() should set default separator")
				}
				if tt.forEach.IndexVar == "" && tt.forEach.IndexVar != DefaultForEachIndexVar {
					t.Errorf("validateForEach() should set default indexVar")
				}
				if tt.forEach.ItemVar == "" && tt.forEach.ItemVar != DefaultForEachItemVar {
					t.Errorf("validateForEach() should set default itemVar")
				}
			}
		})
	}
}

func TestForEachVariableValidation(t *testing.T) {
	tests := []struct {
		name        string
		yamlInput   string
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid foreach with default itemVar",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "Test tool"
    version: "1.0.0"
    functions:
      - name: "TestFunction"
        operation: "api_call"
        description: "Test function"
        triggers:
          - type: "always_on_user_message"
        input:
          - name: "items"
            description: "List of items"
            origin: "chat"
            isOptional: true
        steps:
          - name: "ProcessItems"
            action: "GET"
            foreach:
              items: "$items"
            with:
              url: "https://api.example.com/process/$item"
        successCriteria: "Successfully processed all items"
`,
			expectError: false,
		},
		{
			name: "valid foreach with custom itemVar",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "Test tool"
    version: "1.0.0"
    functions:
      - name: "TestFunction"
        operation: "api_call"
        description: "Test function"
        triggers:
          - type: "always_on_user_message"
        input:
          - name: "professionals"
            description: "List of professionals"
            origin: "chat"
            isOptional: true
        steps:
          - name: "ProcessProfessionals"
            action: "GET"
            foreach:
              items: "$professionals"
              itemVar: "professionalID"
            with:
              url: "https://api.example.com/professional/$professionalID"
        successCriteria: "Successfully processed all professionals"
`,
			expectError: false,
		},
		{
			name: "invalid foreach - undefined variable in foreach context",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "Test tool"
    version: "1.0.0"
    functions:
      - name: "TestFunction"
        operation: "api_call"
        description: "Test function"
        triggers:
          - type: "always_on_user_message"
        input:
          - name: "items"
            description: "List of items"
            origin: "chat"
            isOptional: true
        steps:
          - name: "ProcessItems"
            action: "GET"
            foreach:
              items: "$items"
            with:
              url: "https://api.example.com/process/$undefinedVar"
        successCriteria: "Successfully processed all items"
`,
			expectError: true,
			errorMsg:    "references undefined variable '$undefinedVar'",
		},
		{
			name: "valid foreach with indexVar",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "Test tool"
    version: "1.0.0"
    functions:
      - name: "TestFunction"
        operation: "api_call"
        description: "Test function"
        triggers:
          - type: "always_on_user_message"
        input:
          - name: "items"
            description: "List of items"
            origin: "chat"
            isOptional: true
        steps:
          - name: "ProcessItems"
            action: "GET"
            foreach:
              items: "$items"
              indexVar: "i"
              itemVar: "element"
            with:
              url: "https://api.example.com/process/$element?index=$i"
        successCriteria: "Successfully processed all items"
`,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := CreateTool(tt.yamlInput)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				} else if tt.errorMsg != "" && !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("Expected error containing '%s', but got: %v", tt.errorMsg, err)
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error but got: %v", err)
				}
			}
		})
	}
}
