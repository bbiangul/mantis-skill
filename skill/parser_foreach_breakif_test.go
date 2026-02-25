package skill

import (
	"strings"
	"testing"
)

func TestForEachBreakIfValidation(t *testing.T) {
	testCases := []struct {
		name          string
		yamlInput     string
		expectError   bool
		errorContains string
	}{
		{
			name: "Valid: forEach with deterministic breakIf using item field",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "processItems"
        operation: "terminal"
        description: "Process items"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "itemList"
            description: "List of items"
            origin: "chat"
            onError:
              strategy: "requestUserInput"
              message: "Please provide items"
        steps:
          - name: "process_each"
            action: "bash"
            with:
              linux: echo "Processing"
              windows: echo Processing
            foreach:
              items: "$itemList"
              itemVar: "item"
              indexVar: "index"
              breakIf: "$item.status == 'completed'"
            resultIndex: 1
`,
			expectError: false,
		},
		{
			name: "Valid: forEach with deterministic breakIf using index",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "processItems"
        operation: "terminal"
        description: "Process items"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "items"
            description: "Items list"
            origin: "chat"
            onError:
              strategy: "requestUserInput"
              message: "Provide items"
        steps:
          - name: "process"
            action: "bash"
            with:
              linux: echo "test"
              windows: echo test
            foreach:
              items: "$items"
              itemVar: "item"
              indexVar: "idx"
              breakIf: "$idx >= 5"
            resultIndex: 1
`,
			expectError: false,
		},
		{
			name: "Invalid: forEach breakIf without comparison operator",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "processItems"
        operation: "terminal"
        description: "Process items"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "items"
            description: "Items list"
            origin: "chat"
            onError:
              strategy: "requestUserInput"
              message: "Provide items"
        steps:
          - name: "process"
            action: "bash"
            with:
              linux: echo "test"
              windows: echo test
            foreach:
              items: "$items"
              itemVar: "item"
              indexVar: "index"
              breakIf: "$item.status"
            resultIndex: 1
`,
			expectError:   true,
			errorContains: "invalid syntax",
		},
		{
			name: "Invalid: forEach breakIf using undefined variable",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "processItems"
        operation: "terminal"
        description: "Process items"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "items"
            description: "Items list"
            origin: "chat"
            onError:
              strategy: "requestUserInput"
              message: "Provide items"
        steps:
          - name: "process"
            action: "bash"
            with:
              linux: echo "test"
              windows: echo test
            foreach:
              items: "$items"
              itemVar: "item"
              indexVar: "idx"
              breakIf: "$someOtherVar == 'done'"
            resultIndex: 1
`,
			expectError:   true,
			errorContains: "invalid variable",
		},
		{
			name: "Valid: forEach with inference breakIf",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "getDentists"
        operation: "terminal"
        description: "Get all dentists"
        triggers:
          - type: "flex_for_user"
        steps:
          - name: "fetch"
            action: "bash"
            with:
              linux: |
                echo '{"data": [{"id": 1, "name": "Dr. Smith"}]}'
              windows: |
                @echo off
                echo {"data": [{"id": 1, "name": "Dr. Smith"}]}
            resultIndex: 1
      - name: "findPreferredDentist"
        operation: "terminal"
        description: "Find preferred dentist"
        triggers:
          - type: "flex_for_user"
        needs: ["getDentists"]
        input:
          - name: "dentists"
            description: "List of dentists"
            origin: "function"
            from: "getDentists"
            onError:
              strategy: "requestN1Support"
              message: "Failed to get dentists"
          - name: "preferredName"
            description: "Preferred dentist name"
            origin: "chat"
            onError:
              strategy: "requestUserInput"
              message: "Which dentist?"
        steps:
          - name: "check_each"
            action: "bash"
            with:
              linux: echo "checking"
              windows: echo checking
            foreach:
              items: "$dentists.data"
              itemVar: "dentist"
              breakIf:
                condition: "Check if this dentist matches: $preferredName"
                from: ["getDentists"]
                allowedSystemFunctions: ["askToContext"]
            resultIndex: 1
`,
			expectError: false,
		},
		{
			name: "Invalid: forEach breakIf inference with empty condition",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "processItems"
        operation: "terminal"
        description: "Process items"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "items"
            description: "Items list"
            origin: "chat"
            onError:
              strategy: "requestUserInput"
              message: "Provide items"
        steps:
          - name: "process"
            action: "bash"
            with:
              linux: echo "test"
              windows: echo test
            foreach:
              items: "$items"
              itemVar: "item"
              breakIf:
                condition: ""
                from: []
            resultIndex: 1
`,
			expectError:   true,
			errorContains: "empty condition",
		},
		{
			name: "Valid: forEach breakIf with nested field access",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "processItems"
        operation: "terminal"
        description: "Process items"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "items"
            description: "Items list"
            origin: "chat"
            onError:
              strategy: "requestUserInput"
              message: "Provide items"
        steps:
          - name: "process"
            action: "bash"
            with:
              linux: echo "test"
              windows: echo test
            foreach:
              items: "$items"
              itemVar: "item"
              breakIf: "$item.metadata.priority == 'high'"
            resultIndex: 1
`,
			expectError: false,
		},
		{
			name: "Valid: forEach breakIf with multiple comparison operators",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "processNumbers"
        operation: "terminal"
        description: "Process numbers"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "numbers"
            description: "Number list"
            origin: "chat"
            onError:
              strategy: "requestUserInput"
              message: "Provide numbers"
        steps:
          - name: "process"
            action: "bash"
            with:
              linux: echo "test"
              windows: echo test
            foreach:
              items: "$numbers"
              itemVar: "num"
              indexVar: "i"
              breakIf: "$num > 100"
            resultIndex: 1
`,
			expectError: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := CreateTool(tc.yamlInput)

			if tc.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				} else if tc.errorContains != "" && !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tc.errorContains)) {
					t.Errorf("Expected error containing '%s', but got: %s", tc.errorContains, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %s", err.Error())
				}
			}
		})
	}
}
