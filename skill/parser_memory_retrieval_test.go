package skill

import (
	"strings"
	"testing"
)

func TestMemoryRetrievalMode_Success(t *testing.T) {
	testCases := []struct {
		name       string
		yamlInput  string
		validation func(*testing.T, CustomTool)
	}{
		{
			name: "Memory origin with latest mode",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "storeData"
        operation: "db"
        description: "Store data"
        successCriteria: "data stored successfully"
        triggers:
          - type: "always_on_user_message"
        input:
          - name: "data"
            description: "Data to store"
            origin: "chat"
            onError:
              strategy: "requestUserInput"
              message: "Please provide data"
        steps:
          - name: "store"
            action: "write"
            with:
              write: "INSERT INTO data VALUES ($data)"
      - name: "retrieveData"
        operation: "db"
        description: "Retrieve data"
        successCriteria: "data retrieved successfully"
        triggers:
          - type: "always_on_user_message"
        input:
          - name: "storedData"
            description: "Previously stored data"
            origin: "memory"
            from: "storeData"
            memoryRetrievalMode: "latest"
            successCriteria: "the latest stored data"
            onError:
              strategy: "requestUserInput"
              message: "No stored data found"
        steps:
          - name: "retrieve"
            action: "select"
            with:
              select: "SELECT * FROM data"
`,
			validation: func(t *testing.T, tool CustomTool) {
				if len(tool.Tools) != 1 {
					t.Fatalf("Expected 1 tool, got %d", len(tool.Tools))
				}
				if len(tool.Tools[0].Functions) != 2 {
					t.Fatalf("Expected 2 functions, got %d", len(tool.Tools[0].Functions))
				}

				retrieveFunc := tool.Tools[0].Functions[1]
				if retrieveFunc.Name != "retrieveData" {
					t.Errorf("Expected function name 'retrieveData', got %s", retrieveFunc.Name)
				}

				if len(retrieveFunc.Input) != 1 {
					t.Fatalf("Expected 1 input, got %d", len(retrieveFunc.Input))
				}

				input := retrieveFunc.Input[0]
				if input.Origin != DataOriginMemory {
					t.Errorf("Expected origin 'memory', got %s", input.Origin)
				}
				if input.MemoryRetrievalMode != "latest" {
					t.Errorf("Expected memoryRetrievalMode 'latest', got %s", input.MemoryRetrievalMode)
				}
			},
		},
		{
			name: "Memory origin with all mode",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "storeData"
        operation: "db"
        description: "Store data"
        successCriteria: "data stored successfully"
        triggers:
          - type: "always_on_user_message"
        input:
          - name: "data"
            description: "Data to store"
            origin: "chat"
            onError:
              strategy: "requestUserInput"
              message: "Please provide data"
        steps:
          - name: "store"
            action: "write"
            with:
              write: "INSERT INTO data VALUES ($data)"
      - name: "retrieveAllData"
        operation: "db"
        description: "Retrieve all data"
        successCriteria: "all data retrieved successfully"
        triggers:
          - type: "always_on_user_message"
        input:
          - name: "allStoredData"
            description: "All previously stored data"
            origin: "memory"
            from: "storeData"
            memoryRetrievalMode: "all"
            successCriteria: "all stored data records"
            onError:
              strategy: "requestUserInput"
              message: "No stored data found"
        steps:
          - name: "retrieve"
            action: "select"
            with:
              select: "SELECT * FROM data"
`,
			validation: func(t *testing.T, tool CustomTool) {
				if len(tool.Tools) != 1 {
					t.Fatalf("Expected 1 tool, got %d", len(tool.Tools))
				}
				if len(tool.Tools[0].Functions) != 2 {
					t.Fatalf("Expected 2 functions, got %d", len(tool.Tools[0].Functions))
				}

				retrieveFunc := tool.Tools[0].Functions[1]
				if retrieveFunc.Name != "retrieveAllData" {
					t.Errorf("Expected function name 'retrieveAllData', got %s", retrieveFunc.Name)
				}

				if len(retrieveFunc.Input) != 1 {
					t.Fatalf("Expected 1 input, got %d", len(retrieveFunc.Input))
				}

				input := retrieveFunc.Input[0]
				if input.Origin != DataOriginMemory {
					t.Errorf("Expected origin 'memory', got %s", input.Origin)
				}
				if input.MemoryRetrievalMode != "all" {
					t.Errorf("Expected memoryRetrievalMode 'all', got %s", input.MemoryRetrievalMode)
				}
			},
		},
		{
			name: "Memory origin without memoryRetrievalMode (defaults to latest)",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "storeData"
        operation: "db"
        description: "Store data"
        successCriteria: "data stored successfully"
        triggers:
          - type: "always_on_user_message"
        input:
          - name: "data"
            description: "Data to store"
            origin: "chat"
            onError:
              strategy: "requestUserInput"
              message: "Please provide data"
        steps:
          - name: "store"
            action: "write"
            with:
              write: "INSERT INTO data VALUES ($data)"
      - name: "retrieveData"
        operation: "db"
        description: "Retrieve data"
        successCriteria: "data retrieved successfully"
        triggers:
          - type: "always_on_user_message"
        input:
          - name: "storedData"
            description: "Previously stored data"
            origin: "memory"
            from: "storeData"
            successCriteria: "the stored data"
            onError:
              strategy: "requestUserInput"
              message: "No stored data found"
        steps:
          - name: "retrieve"
            action: "select"
            with:
              select: "SELECT * FROM data"
`,
			validation: func(t *testing.T, tool CustomTool) {
				if len(tool.Tools) != 1 {
					t.Fatalf("Expected 1 tool, got %d", len(tool.Tools))
				}
				if len(tool.Tools[0].Functions) != 2 {
					t.Fatalf("Expected 2 functions, got %d", len(tool.Tools[0].Functions))
				}

				retrieveFunc := tool.Tools[0].Functions[1]
				input := retrieveFunc.Input[0]

				if input.Origin != DataOriginMemory {
					t.Errorf("Expected origin 'memory', got %s", input.Origin)
				}
				// When not specified, should be empty string (defaults to "latest" at runtime)
				if input.MemoryRetrievalMode != "" {
					t.Errorf("Expected memoryRetrievalMode to be empty (defaults to latest), got %s", input.MemoryRetrievalMode)
				}
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tool, err := CreateTool(tc.yamlInput)
			if err != nil {
				t.Fatalf("Expected no error, got: %v", err)
			}
			tc.validation(t, tool)
		})
	}
}

func TestMemoryRetrievalMode_ValidationErrors(t *testing.T) {
	testCases := []struct {
		name          string
		yamlInput     string
		expectedError string
	}{
		{
			name: "Invalid memoryRetrievalMode value",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "storeData"
        operation: "db"
        description: "Store data"
        successCriteria: "data stored successfully"
        triggers:
          - type: "always_on_user_message"
        input:
          - name: "data"
            description: "Data to store"
            origin: "chat"
            onError:
              strategy: "requestUserInput"
              message: "Please provide data"
        steps:
          - name: "store"
            action: "write"
            with:
              write: "INSERT INTO data VALUES ($data)"
      - name: "retrieveData"
        operation: "db"
        description: "Retrieve data"
        successCriteria: "data retrieved successfully"
        triggers:
          - type: "always_on_user_message"
        input:
          - name: "storedData"
            description: "Previously stored data"
            origin: "memory"
            from: "storeData"
            memoryRetrievalMode: "invalid_mode"
            successCriteria: "the stored data"
            onError:
              strategy: "requestUserInput"
              message: "No stored data found"
        steps:
          - name: "retrieve"
            action: "select"
            with:
              select: "SELECT * FROM data"
`,
			expectedError: "has invalid memoryRetrievalMode 'invalid_mode'; must be either 'all' or 'latest'",
		},
		{
			name: "memoryRetrievalMode used with non-memory origin",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "TestFunction"
        operation: "db"
        description: "A test function"
        successCriteria: "success"
        triggers:
          - type: "always_on_user_message"
        input:
          - name: "query"
            description: "The query"
            origin: "chat"
            memoryRetrievalMode: "all"
            onError:
              strategy: "requestUserInput"
              message: "Please provide a query"
        steps:
          - name: "execute"
            action: "execute_sql"
            with:
              sql: "SELECT * FROM data"
`,
			expectedError: "has 'memoryRetrievalMode' set but origin is 'chat'; this field is only valid for inputs with origin 'memory'",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := CreateTool(tc.yamlInput)
			if err == nil {
				t.Fatalf("Expected error, got none")
			}
			if !strings.Contains(err.Error(), tc.expectedError) {
				t.Errorf("Expected error containing '%s', got: %v", tc.expectedError, err)
			}
		})
	}
}
