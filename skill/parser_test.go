package skill

import (
	"gopkg.in/yaml.v2"
	"strings"
	"testing"
)

func TestCreateTool_Success(t *testing.T) {
	testCases := []struct {
		name       string
		yamlInput  string
		validation func(*testing.T, CustomTool)
	}{
		{
			name: "Minimal valid configuration",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "TestFunction"
        operation: "web_browse"
        description: "A test function"
        successCriteria: "inference"
        triggers:
          - type: "always_on_user_message"
        input:
          - name: "query"
            description: "The search query"
            origin: "chat"
            onError:
              strategy: "requestUserInput"
              message: "Please provide a search query"
        steps:
          - name: "open browser"
            action: "open_url"
            with:
              url: "https://example.com"
`,
			validation: func(t *testing.T, tool CustomTool) {
				if tool.Version != "v1" {
					t.Errorf("Expected version v1, got %s", tool.Version)
				}
				if tool.Author != "Test Author" {
					t.Errorf("Expected author 'Test Author', got %s", tool.Author)
				}
				if len(tool.Tools) != 1 {
					t.Errorf("Expected 1 tool, got %d", len(tool.Tools))
				}
				if tool.Tools[0].Name != "TestTool" {
					t.Errorf("Expected tool name 'TestTool', got %s", tool.Tools[0].Name)
				}
			},
		},
		{
			name: "Configuration with environment variables",
			yamlInput: `
version: "v1"
author: "Test Author"
env:
  - name: "API_KEY"
    value: "test-key-12345"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "TestFunction"
        operation: "web_browse"
        description: "A test function"
        successCriteria: "inference"
        triggers:
          - type: "always_on_user_message"
        input:
          - name: "query"
            description: "The search query"
            origin: "chat"
            onError:
              strategy: "requestUserInput"
              message: "Please provide a search query"
        steps:
          - name: "open browser"
            action: "open_url"
            with:
              url: "https://example.com"
`,
			validation: func(t *testing.T, tool CustomTool) {
				if len(tool.Env) != 1 {
					t.Errorf("Expected 1 environment variable, got %d", len(tool.Env))
				}
				if tool.Env[0].Name != "API_KEY" {
					t.Errorf("Expected env name 'API_KEY', got %s", tool.Env[0].Name)
				}
				if tool.Env[0].Value != "test-key-12345" {
					t.Errorf("Expected env value 'test-key-12345', got %s", tool.Env[0].Value)
				}
			},
		},
		{
			name: "Configuration with api_call operation",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "APITool"
    description: "An API tool"
    version: "1.0.0"
    functions:
      - name: "FetchData"
        operation: "api_call"
        description: "Fetch data from API"
        triggers:
          - type: "always_on_user_message"
        input:
          - name: "userId"
            description: "User ID to fetch"
            origin: "chat"
            onError:
              strategy: "requestUserInput"
              message: "Please provide a user ID"
        steps:
          - name: "fetch user"
            action: "GET"
            with:
              url: "https://api.example.com/users/123"
              headers:
                - key: "Content-Type"
                  value: "application/json"
        output:
          type: "object"
          fields:
            - "userId"
            - "name"
            - "email"
`,
			validation: func(t *testing.T, tool CustomTool) {
				fn := tool.Tools[0].Functions[0]
				if fn.Operation != OperationAPI {
					t.Errorf("Expected operation 'api_call', got %s", fn.Operation)
				}
				if fn.Steps[0].Action != StepActionGET {
					t.Errorf("Expected action 'GET', got %s", fn.Steps[0].Action)
				}
				if fn.Output == nil {
					t.Error("Expected output to be defined")
				}
				if fn.Output.Type != StepOutputObject {
					t.Errorf("Expected output type 'object', got %s", fn.Output.Type)
				}
			},
		},
		{
			name: "Configuration with desktop_use operation",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "DesktopTool"
    description: "A desktop tool"
    version: "1.0.0"
    functions:
      - name: "OpenNotepad"
        operation: "desktop_use"
        description: "Open notepad and type text"
        triggers:
          - type: "always_on_user_message"
        input:
          - name: "text"
            description: "Text to type"
            origin: "chat"
            onError:
              strategy: "requestUserInput"
              message: "Please provide text to type"
        steps:
          - name: "open notepad"
            action: "open_app"
            with:
              app_name: "notepad"
          - name: "type text"
            action: "fill"
            with:
              fillValue: "Hello World"
`,
			validation: func(t *testing.T, tool CustomTool) {
				fn := tool.Tools[0].Functions[0]
				if fn.Operation != OperationDesktop {
					t.Errorf("Expected operation 'desktop_use', got %s", fn.Operation)
				}
				if fn.Steps[0].Action != StepActionApp {
					t.Errorf("Expected action 'open_app', got %s", fn.Steps[0].Action)
				}
				appName, ok := fn.Steps[0].With[StepWithApp]
				if !ok || appName != "notepad" {
					t.Errorf("Expected app_name 'notepad', got %v", fn.Steps[0].With[StepWithApp])
				}
			},
		},
		{
			name: "Function with time-based trigger",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "ScheduledTool"
    description: "A scheduled tool"
    version: "1.0.0"
    functions:
      - name: "DailyCheck"
        operation: "web_browse"
        description: "Daily check"
        triggers:
          - type: "time_based"
            cron: "0 0 * * *"
        steps:
          - name: "open browser"
            action: "open_url"
            with:
              url: "https://example.com"
`,
			validation: func(t *testing.T, tool CustomTool) {
				trigger := tool.Tools[0].Functions[0].Triggers[0]
				if trigger.Type != TriggerTime {
					t.Errorf("Expected trigger type 'time_based', got %s", trigger.Type)
				}
				if trigger.Cron != "0 0 * * *" {
					t.Errorf("Expected cron '0 0 * * *', got %s", trigger.Cron)
				}
			},
		},
		{
			name: "Complex output structure",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "OutputTool"
    description: "A tool with complex output"
    version: "1.0.0"
    functions:
      - name: "GetProductInfo"
        operation: "api_call"
        description: "Get product information"
        triggers:
          - type: "always_on_user_message"
        steps:
          - name: "fetch product"
            action: "GET"
            with:
              url: "https://api.example.com/products/123"
        output:
          type: "object"
          fields:
            - name: "product"
              type: "object"
              fields:
                - "name"
                - "description"
                - "price"
            - name: "categories"
              type: "list[string]"
            - name: "relatedProducts"
              type: "list[object]"
              fields:
                - "id"
                - "name"
                - "price"
            - "simpleField"
`,
			validation: func(t *testing.T, tool CustomTool) {
				output := tool.Tools[0].Functions[0].Output
				if output == nil {
					t.Error("Expected output to be defined")
					return
				}

				if len(output.Fields) != 4 {
					t.Errorf("Expected 4 output fields, got %d", len(output.Fields))
					return
				}

				// Check complex object field
				productField := output.Fields[0]
				if productField.Name != "product" || productField.Type != StepOutputObject {
					t.Errorf("Expected product field with object type, got name=%s, type=%s",
						productField.Name, productField.Type)
				}
				if len(productField.Fields) != 3 {
					t.Errorf("Expected 3 nested fields in product, got %d", len(productField.Fields))
				}

				// Check list[string] field
				categoriesField := output.Fields[1]
				if categoriesField.Name != "categories" || categoriesField.Type != StepOutputListString {
					t.Errorf("Expected categories field with list[string] type, got name=%s, type=%s",
						categoriesField.Name, categoriesField.Type)
				}

				// Check list[object] field
				relatedField := output.Fields[2]
				if relatedField.Name != "relatedProducts" || relatedField.Type != StepOutputListOfObject {
					t.Errorf("Expected relatedProducts field with list[object] type, got name=%s, type=%s",
						relatedField.Name, relatedField.Type)
				}
				if len(relatedField.Fields) != 3 {
					t.Errorf("Expected 3 nested fields in relatedProducts, got %d", len(relatedField.Fields))
				}

				// Check simple field
				simpleField := output.Fields[3]
				if simpleField.Value != "simpleField" {
					t.Errorf("Expected simpleField value, got %s", simpleField.Value)
				}
			},
		},
		{
			name: "List string output",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "StringListTool"
    description: "A tool returning string list"
    version: "1.0.0"
    functions:
      - name: "GetTags"
        operation: "api_call"
        description: "Get tags list"
        triggers:
          - type: "always_on_user_message"
        steps:
          - name: "fetch tags"
            action: "GET"
            with:
              url: "https://api.example.com/tags"
        output:
          type: "list[string]"
`,
			validation: func(t *testing.T, tool CustomTool) {
				output := tool.Tools[0].Functions[0].Output
				if output == nil {
					t.Error("Expected output to be defined")
					return
				}

				if output.Type != StepOutputListString {
					t.Errorf("Expected output type list[string], got %s", output.Type)
				}

				if len(output.Fields) != 0 {
					t.Errorf("Expected 0 fields for list[string] output, got %d", len(output.Fields))
				}
			},
		},
		{
			name: "List number output",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "NumberListTool"
    description: "A tool returning number list"
    version: "1.0.0"
    functions:
      - name: "GetPrices"
        operation: "api_call"
        description: "Get prices list"
        triggers:
          - type: "always_on_user_message"
        steps:
          - name: "fetch prices"
            action: "GET"
            with:
              url: "https://api.example.com/prices"
        output:
          type: "list[number]"
`,
			validation: func(t *testing.T, tool CustomTool) {
				output := tool.Tools[0].Functions[0].Output
				if output == nil {
					t.Error("Expected output to be defined")
					return
				}

				if output.Type != StepOutputListNumber {
					t.Errorf("Expected output type list[number], got %s", output.Type)
				}

				if len(output.Fields) != 0 {
					t.Errorf("Expected 0 fields for list[number] output, got %d", len(output.Fields))
				}
			},
		},
		{
			name: "Function with function inputs",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "FunctionInputTool"
    description: "A tool with function inputs"
    version: "1.0.0"
    functions:
      - name: "getData"
        operation: "api_call"
        description: "Private function to get data"
        triggers:
          - type: "flex_for_user"
        steps:
          - name: "fetch data"
            action: "GET"
            with:
              url: "https://api.example.com/data"
        output:
          type: "object"
          fields:
            - "id"
            - "name"
      - name: "ProcessData"
        operation: "web_browse"
        description: "Process data from another function"
        triggers:
          - type: "always_on_user_message"
        input:
          - name: "dataItem"
            description: "Data to process"
            origin: "function"
            from: "getData"
            onError:
              strategy: "requestUserInput"
              message: "Failed to get data"
        steps:
          - name: "open browser"
            action: "open_url"
            with:
              url: "https://example.com"
`,
			validation: func(t *testing.T, tool CustomTool) {
				// Check if the first function exists and is private
				privateFunc := tool.Tools[0].Functions[0]
				if privateFunc.Name != "getData" {
					t.Errorf("Expected private function name 'getData', got %s", privateFunc.Name)
				}

				// Check the input with function origin
				input := tool.Tools[0].Functions[1].Input[0]
				if input.Origin != DataOriginFunction {
					t.Errorf("Expected origin 'function', got %s", input.Origin)
				}
				if input.From != "getData" {
					t.Errorf("Expected from 'getData', got %s", input.From)
				}
			},
		},
		{
			name: "Function with zero-state",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "ZeroStateTool"
    description: "A tool with zero-state"
    version: "1.0.0"
    functions:
      - name: "BrowseWithZeroState"
        operation: "web_browse"
        description: "Browse with zero state"
        triggers:
          - type: "always_on_user_message"
        zero-state:
          steps:
            - name: "init browser"
              action: "open_url"
              with:
                url: "https://example.com"
        steps:
          - name: "search"
            action: "find_and_click"
            with:
              findBy: "id"
              findValue: "search-button"
`,
			validation: func(t *testing.T, tool CustomTool) {
				zeroState := tool.Tools[0].Functions[0].ZeroState
				if len(zeroState.Steps) != 1 {
					t.Errorf("Expected 1 zero-state step, got %d", len(zeroState.Steps))
					return
				}

				if zeroState.Steps[0].Name != "init browser" {
					t.Errorf("Expected zero-state step name 'init browser', got %s", zeroState.Steps[0].Name)
				}

				if zeroState.Steps[0].Action != StepActionURL {
					t.Errorf("Expected zero-state action 'open_url', got %s", zeroState.Steps[0].Action)
				}
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := CreateTool(tc.yamlInput)
			if err != nil {
				t.Fatalf("Failed to parse valid YAML: %v", err)
			}

			tc.validation(t, result)
		})
	}
}

func TestCreateTool_Errors(t *testing.T) {
	testCases := []struct {
		name          string
		yamlInput     string
		expectedError string
	}{
		{
			name: "Missing version",
			yamlInput: `
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "TestFunction"
        operation: "web_browse"
        description: "A test function"
        triggers:
          - type: "always_on_user_message"
        steps:
          - name: "open browser"
            action: "open_url"
            with:
              url: "https://example.com"
`,
			expectedError: "global 'version' is required",
		},
		{
			name: "Unsupported version",
			yamlInput: `
version: "v2"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "TestFunction"
        operation: "web_browse"
        description: "A test function"
        triggers:
          - type: "always_on_user_message"
        steps:
          - name: "open browser"
            action: "open_url"
            with:
              url: "https://example.com"
`,
			expectedError: "v1' is the only yaml definition version",
		},
		{
			name: "Missing author",
			yamlInput: `
version: "v1"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "TestFunction"
        operation: "web_browse"
        description: "A test function"
        triggers:
          - type: "always_on_user_message"
        steps:
          - name: "open browser"
            action: "open_url"
            with:
              url: "https://example.com"
`,
			expectedError: "global 'author' is required",
		},
		{
			name: "Empty env variable name",
			yamlInput: `
version: "v1"
author: "Test Author"
env:
  - name: ""
    value: "test-value"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "TestFunction"
        operation: "web_browse"
        description: "A test function"
        triggers:
          - type: "always_on_user_message"
        steps:
          - name: "open browser"
            action: "open_url"
            with:
              url: "https://example.com"
`,
			expectedError: "empty name",
		},
		{
			name: "Invalid env variable name",
			yamlInput: `
version: "v1"
author: "Test Author"
env:
  - name: "123-INVALID"
    value: "test-value"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "TestFunction"
        operation: "web_browse"
        description: "A test function"
        triggers:
          - type: "always_on_user_message"
        steps:
          - name: "open browser"
            action: "open_url"
            with:
              url: "https://example.com"
`,
			expectedError: "not a valid environment variable name",
		},
		{
			name: "Duplicate env variable name",
			yamlInput: `
version: "v1"
author: "Test Author"
env:
  - name: "API_KEY"
    value: "value1"
  - name: "API_KEY"
    value: "value2"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "TestFunction"
        operation: "web_browse"
        description: "A test function"
        triggers:
          - type: "always_on_user_message"
        steps:
          - name: "open browser"
            action: "open_url"
            with:
              url: "https://example.com"
`,
			expectedError: "duplicate environment variable name",
		},
		{
			name: "No tools defined",
			yamlInput: `
version: "v1"
author: "Test Author"
`,
			expectedError: "at least one tool must be defined",
		},
		{
			name: "Empty tool name",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: ""
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "TestFunction"
        operation: "web_browse"
        description: "A test function"
        triggers:
          - type: "always_on_user_message"
        steps:
          - name: "open browser"
            action: "open_url"
            with:
              url: "https://example.com"
`,
			expectedError: "empty name",
		},
		{
			name: "Duplicate tool name",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "TestFunction"
        operation: "web_browse"
        description: "A test function"
        triggers:
          - type: "always_on_user_message"
        steps:
          - name: "open browser"
            action: "open_url"
            with:
              url: "https://example.com"
  - name: "TestTool"
    description: "Another test tool"
    version: "1.0.0"
    functions:
      - name: "AnotherFunction"
        operation: "web_browse"
        description: "Another function"
        triggers:
          - type: "always_on_user_message"
        steps:
          - name: "open browser"
            action: "open_url"
            with:
              url: "https://example.com"
`,
			expectedError: "duplicate tool name",
		},
		{
			name: "Invalid semantic version",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0"
    functions:
      - name: "TestFunction"
        operation: "web_browse"
        description: "A test function"
        triggers:
          - type: "always_on_user_message"
        steps:
          - name: "open browser"
            action: "open_url"
            with:
              url: "https://example.com"
`,
			expectedError: "invalid; must match semantic versioning",
		},
		{
			name: "Empty function name",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: ""
        operation: "web_browse"
        description: "A test function"
        triggers:
          - type: "always_on_user_message"
        steps:
          - name: "open browser"
            action: "open_url"
            with:
              url: "https://example.com"
`,
			expectedError: "empty name",
		},
		{
			name: "Invalid operation",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "TestFunction"
        operation: "invalid_operation"
        description: "A test function"
        triggers:
          - type: "always_on_user_message"
        steps:
          - name: "open browser"
            action: "open_url"
            with:
              url: "https://example.com"
`,
			expectedError: "invalid operation",
		},
		{
			name: "No triggers defined",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "TestFunction"
        operation: "web_browse"
        description: "A test function"
        steps:
          - name: "open browser"
            action: "open_url"
            with:
              url: "https://example.com"
`,
			expectedError: "must have at least one trigger",
		},
		{
			name: "Invalid trigger type",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "TestFunction"
        operation: "web_browse"
        description: "A test function"
        triggers:
          - type: "invalid_trigger"
        steps:
          - name: "open browser"
            action: "open_url"
            with:
              url: "https://example.com"
`,
			expectedError: "invalid type",
		},
		{
			name: "Missing cron for time-based trigger",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "TestFunction"
        operation: "web_browse"
        description: "A test function"
        triggers:
          - type: "time_based"
        steps:
          - name: "open browser"
            action: "open_url"
            with:
              url: "https://example.com"
`,
			expectedError: "must have a cron expression",
		},
		{
			name: "Invalid cron expression",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "TestFunction"
        operation: "web_browse"
        description: "A test function"
        triggers:
          - type: "time_based"
            cron: "invalid cron"
        steps:
          - name: "open browser"
            action: "open_url"
            with:
              url: "https://example.com"
`,
			expectedError: "invalid cron expression",
		},
		{
			name: "Missing input description",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "TestFunction"
        operation: "web_browse"
        description: "A test function"
        triggers:
          - type: "always_on_user_message"
        input:
          - name: "query"
            origin: "chat"
        steps:
          - name: "open browser"
            action: "open_url"
            with:
              url: "https://example.com"
`,
			expectedError: "must have a description",
		},
		{
			name: "Missing input origin and value",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "TestFunction"
        operation: "web_browse"
        description: "A test function"
        triggers:
          - type: "always_on_user_message"
        input:
          - name: "query"
            description: "The search query"
        steps:
          - name: "open browser"
            action: "open_url"
            with:
              url: "https://example.com"
`,
			expectedError: "must have an origin or a static value",
		},
		{
			name: "Invalid input origin",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "TestFunction"
        operation: "web_browse"
        description: "A test function"
        triggers:
          - type: "always_on_user_message"
        input:
          - name: "query"
            description: "The search query"
            origin: "invalid_origin"
        steps:
          - name: "open browser"
            action: "open_url"
            with:
              url: "https://example.com"
`,
			expectedError: "invalid origin",
		},
		{
			name: "Missing from for function origin",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "TestFunction"
        operation: "web_browse"
        description: "A test function"
        triggers:
          - type: "always_on_user_message"
        input:
          - name: "query"
            description: "The search query"
            origin: "function"
        steps:
          - name: "open browser"
            action: "open_url"
            with:
              url: "https://example.com"
`,
			expectedError: "missing the 'from' field",
		},
		{
			name: "Non-private function reference",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "privateFunction"
        operation: "api_call"
        description: "Private function"
        triggers:
          - type: "flex_for_user"
        steps:
          - name: "call api"
            action: "GET"
            with:
              url: "https://example.com/api"
      - name: "TestFunction"
        operation: "web_browse"
        description: "A test function"
        triggers:
          - type: "always_on_user_message"
        input:
          - name: "query"
            description: "The search query"
            origin: "function"
            from: "NonPrivateFunction"
        steps:
          - name: "open browser"
            action: "open_url"
            with:
              url: "https://example.com"
`,
			expectedError: "must refer to a private function",
		},
		{
			name: "Function reference not found",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "TestFunction"
        operation: "web_browse"
        description: "A test function"
        triggers:
          - type: "always_on_user_message"
        input:
          - name: "query"
            description: "The search query"
            origin: "function"
            from: "nonExistentFunction"
        steps:
          - name: "open browser"
            action: "open_url"
            with:
              url: "https://example.com"
`,
			expectedError: "was not found",
		},
		{
			name: "Missing onError for required input",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "TestFunction"
        operation: "web_browse"
        description: "A test function"
        triggers:
          - type: "always_on_user_message"
        input:
          - name: "query"
            description: "The search query"
            origin: "chat"
            isOptional: false
        steps:
          - name: "open browser"
            action: "open_url"
            with:
              url: "https://example.com"
`,
			expectedError: "missing onError definition",
		},
		{
			name: "Invalid onError strategy",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "TestFunction"
        operation: "web_browse"
        description: "A test function"
        triggers:
          - type: "always_on_user_message"
        input:
          - name: "query"
            description: "The search query"
            origin: "chat"
            onError:
              strategy: "invalidStrategy"
              message: "Please provide a search query"
        steps:
          - name: "open browser"
            action: "open_url"
            with:
              url: "https://example.com"
`,
			expectedError: "invalid onError strategy",
		},
		{
			name: "Missing onError message - now valid for chat/inference origins",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "TestFunction"
        operation: "web_browse"
        description: "A test function"
        triggers:
          - type: "always_on_user_message"
        input:
          - name: "query"
            description: "The search query"
            origin: "chat"
            onError:
              strategy: "requestUserInput"
        steps:
          - name: "open browser"
            action: "open_url"
            with:
              url: "https://example.com"
`,
			expectedError: "", // onError.message is now optional for origin: chat/inference - auto-generated from extraction rationale
		},
		{
			name: "Empty step name",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "TestFunction"
        operation: "web_browse"
        description: "A test function"
        triggers:
          - type: "always_on_user_message"
        steps:
          - name: ""
            action: "open_url"
            with:
              url: "https://example.com"
`,
			expectedError: "empty name",
		},
		{
			name: "Empty step action",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "TestFunction"
        operation: "web_browse"
        description: "A test function"
        triggers:
          - type: "always_on_user_message"
        steps:
          - name: "open browser"
            action: ""
            with:
              url: "https://example.com"
`,
			expectedError: "empty action",
		},
		{
			name: "Invalid web_browse action",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "TestFunction"
        operation: "web_browse"
        description: "A test function"
        triggers:
          - type: "always_on_user_message"
        steps:
          - name: "invalid action"
            action: "invalid_action"
`,
			expectedError: "unsupported action",
		},
		{
			name: "Missing with block for open_url",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "TestFunction"
        operation: "web_browse"
        description: "A test function"
        triggers:
          - type: "always_on_user_message"
        steps:
          - name: "open browser"
            action: "open_url"
`,
			expectedError: "must have a 'with' block",
		},
		{
			name: "Missing url for open_url",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "TestFunction"
        operation: "web_browse"
        description: "A test function"
        triggers:
          - type: "always_on_user_message"
        steps:
          - name: "open browser"
            action: "open_url"
            with:
              incognitoMode: true
`,
			expectedError: "must provide a 'url' parameter",
		},
		{
			name: "Missing fillValue for fill action",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "TestFunction"
        operation: "web_browse"
        description: "A test function"
        triggers:
          - type: "always_on_user_message"
        steps:
          - name: "fill form"
            action: "fill"
            with: {}
`,
			expectedError: "must provide a 'fillValue' parameter",
		},
		{
			name: "Missing findBy for find_and_click",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "TestFunction"
        operation: "web_browse"
        description: "A test function"
        triggers:
          - type: "always_on_user_message"
        steps:
          - name: "click button"
            action: "find_and_click"
            with:
              findValue: "submit-button"
`,
			expectedError: "must provide a 'findBy' parameter",
		},
		{
			name: "Invalid findBy value",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "TestFunction"
        operation: "web_browse"
        description: "A test function"
        triggers:
          - type: "always_on_user_message"
        steps:
          - name: "click button"
            action: "find_and_click"
            with:
              findBy: "invalid_find_by"
              findValue: "submit-button"
`,
			expectedError: "invalid 'findBy' value",
		},
		{
			name: "Invalid element type for findBy=type",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "TestFunction"
        operation: "web_browse"
        description: "A test function"
        triggers:
          - type: "always_on_user_message"
        steps:
          - name: "click button"
            action: "find_and_click"
            with:
              findBy: "type"
              findValue: "invalid_element_type"
`,
			expectedError: "invalid element type",
		},
		{
			name: "Missing fillValue for find_fill_and_tab",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "TestFunction"
        operation: "web_browse"
        description: "A test function"
        triggers:
          - type: "always_on_user_message"
        steps:
          - name: "fill and tab"
            action: "find_fill_and_tab"
            with:
              findBy: "id"
              findValue: "username"
`,
			expectedError: "must provide a 'fillValue' parameter",
		},
		{
			name: "Missing app_name for open_app",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "TestFunction"
        operation: "desktop_use"
        description: "A test function"
        triggers:
          - type: "always_on_user_message"
        steps:
          - name: "open app"
            action: "open_app"
            with: {}
`,
			expectedError: "must provide an 'app_name' parameter",
		},
		{
			name: "Invalid HTTP method for api_call",
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
        steps:
          - name: "invalid method"
            action: "INVALID_METHOD"
`,
			expectedError: "invalid HTTP method",
		},
		{
			name: "Invalid requestBody format",
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
        steps:
          - name: "post data"
            action: "POST"
            with:
              url: "https://api.example.com"
              requestBody: "invalid-format"
`,
			expectedError: "invalid requestBody format",
		},
		{
			name: "Invalid content type",
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
        steps:
          - name: "post data"
            action: "POST"
            with:
              url: "https://api.example.com"
              requestBody:
                type: "invalid_content_type"
                with:
                  name: "test"
`,
			expectedError: "invalid content type",
		},
		{
			name: "Invalid output type",
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
        steps:
          - name: "get data"
            action: "GET"
            with:
              url: "https://api.example.com"
        output:
          type: "invalid_type"
          fields:
            - "name"
            - "description"
`,
			expectedError: "invalid output type",
		},
		{
			name: "Fields with list[string] output",
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
        steps:
          - name: "get data"
            action: "GET"
            with:
              url: "https://api.example.com"
        output:
          type: "list[string]"
          fields:
            - "name"
`,
			expectedError: "should not have fields",
		},
		{
			name: "No fields for object output",
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
        steps:
          - name: "get data"
            action: "GET"
            with:
              url: "https://api.example.com"
        output:
          type: "object"
          fields: []
`,
			expectedError: "must have at least one field",
		},
		{
			name: "Invalid field type in output",
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
        steps:
          - name: "get data"
            action: "GET"
            with:
              url: "https://api.example.com"
        output:
          type: "object"
          fields:
            - name: "product"
              type: "invalid_type"
`,
			expectedError: "invalid type",
		},
		{
			name: "Exceeding maximum nesting depth",
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
        steps:
          - name: "get data"
            action: "GET"
            with:
              url: "https://api.example.com"
        output:
          type: "object"
          fields:
            - name: "level1"
              type: "object"
              fields:
                - name: "level2"
                  type: "object"
                  fields:
                    - name: "level3"
                      type: "object"
                      fields:
                        - "tooDeep"
`,
			expectedError: "exceeds maximum nesting depth",
		},
		{
			name: "Missing nested fields for complex type",
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
        steps:
          - name: "get data"
            action: "GET"
            with:
              url: "https://api.example.com"
        output:
          type: "object"
          fields:
            - name: "product"
              type: "object"
              fields: []
`,
			expectedError: "must define nested fields",
		},
		{
			name: "Circular dependency in functions",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "functionA"
        operation: "api_call"
        description: "Function A"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "dataFromB"
            description: "Data from B"
            origin: "function"
            from: "functionB"
            onError:
              strategy: "requestUserInput"
              message: "Error getting data from B"
        steps:
          - name: "call api"
            action: "GET"
            with:
              url: "https://api.example.com"
      - name: "functionB"
        operation: "api_call"
        description: "Function B"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "dataFromA"
            description: "Data from A"
            origin: "function"
            from: "functionA"
            onError:
              strategy: "requestUserInput"
              message: "Error getting data from A"
        steps:
          - name: "call api"
            action: "GET"
            with:
              url: "https://api.example.com"
`,
			expectedError: "circular dependency",
		},
		{
			name: "Invalid step in zero-state",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "TestFunction"
        operation: "web_browse"
        description: "A test function"
        triggers:
          - type: "always_on_user_message"
        zero-state:
          steps:
            - name: "open browser"
              action: "open_url"
              # Missing with block
        steps:
          - name: "click button"
            action: "find_and_click"
            with:
              findBy: "id"
              findValue: "search-button"
`,
			expectedError: "in zero-state",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := CreateTool(tc.yamlInput)
			if err == nil {
				t.Fatalf("Expected error containing '%s', but got no error", tc.expectedError)
			}

			if !strings.Contains(err.Error(), tc.expectedError) {
				t.Errorf("Expected error containing '%s', but got: %v", tc.expectedError, err)
			}
		})
	}
}

// Test the custom UnmarshalYAML method for OutputField
func TestOutputField_UnmarshalYAML(t *testing.T) {
	testCases := []struct {
		name          string
		yamlInput     string
		expectedField OutputField
		expectError   bool
	}{
		{
			name:      "Simple string field",
			yamlInput: `"simpleField"`,
			expectedField: OutputField{
				Value: "simpleField",
			},
			expectError: false,
		},
		{
			name: "Complex object field",
			yamlInput: `
name: "complexField"
type: "object"
fields:
  - "nestedField1"
  - "nestedField2"
`,
			expectedField: OutputField{
				Name: "complexField",
				Type: "object",
				Fields: []OutputField{
					{Value: "nestedField1"},
					{Value: "nestedField2"},
				},
			},
			expectError: false,
		},
		{
			name: "List field",
			yamlInput: `
name: "listField"
type: "list[object]"
fields:
  - "item1"
  - "item2"
`,
			expectedField: OutputField{
				Name: "listField",
				Type: "list[object]",
				Fields: []OutputField{
					{Value: "item1"},
					{Value: "item2"},
				},
			},
			expectError: false,
		},
		{
			name: "Invalid YAML",
			yamlInput: `
name: "invalidField
type: "object"
`,
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var field OutputField
			err := yaml.Unmarshal([]byte(tc.yamlInput), &field)

			if tc.expectError {
				if err == nil {
					t.Errorf("Expected error unmarshaling YAML, but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("Failed to unmarshal YAML: %v", err)
			}

			// Compare fields
			if field.Name != tc.expectedField.Name {
				t.Errorf("Expected Name %q, got %q", tc.expectedField.Name, field.Name)
			}

			if field.Value != tc.expectedField.Value {
				t.Errorf("Expected Value %q, got %q", tc.expectedField.Value, field.Value)
			}

			if field.Type != tc.expectedField.Type {
				t.Errorf("Expected Type %q, got %q", tc.expectedField.Type, field.Type)
			}

			if len(field.Fields) != len(tc.expectedField.Fields) {
				t.Errorf("Expected %d Fields, got %d", len(tc.expectedField.Fields), len(field.Fields))
				return
			}

			for i, expectedNestedField := range tc.expectedField.Fields {
				actualNestedField := field.Fields[i]
				if actualNestedField.Value != expectedNestedField.Value {
					t.Errorf("Expected nested field %d Value to be %q, got %q", i, expectedNestedField.Value, actualNestedField.Value)
				}
			}
		})
	}
}

// Test the full YAML parsing for specific output structures
func TestOutputParsing(t *testing.T) {
	yamlInput := `
version: "v1"
author: "Test Author"
tools:
  - name: "OutputTool"
    description: "A tool with various outputs"
    version: "1.0.0"
    functions:
      - name: "SimpleObject"
        operation: "api_call"
        description: "Returns a simple object"
        triggers:
          - type: "always_on_user_message"
        steps:
          - name: "get data"
            action: "GET"
            with:
              url: "https://api.example.com"
        output:
          type: "object"
          fields:
            - "name"
            - "description"
            - "price"

      - name: "ListOfStrings"
        operation: "api_call"
        description: "Returns a list of strings"
        triggers:
          - type: "always_on_user_message"
        steps:
          - name: "get data"
            action: "GET"
            with:
              url: "https://api.example.com/tags"
        output:
          type: "list[string]"

      - name: "ListOfNumbers"
        operation: "api_call"
        description: "Returns a list of numbers"
        triggers:
          - type: "always_on_user_message"
        steps:
          - name: "get data"
            action: "GET"
            with:
              url: "https://api.example.com/prices"
        output:
          type: "list[number]"

      - name: "ComplexNestedOutput"
        operation: "api_call"
        description: "Returns a complex nested structure"
        triggers:
          - type: "always_on_user_message"
        steps:
          - name: "get data"
            action: "GET"
            with:
              url: "https://api.example.com/products"
        output:
          type: "object"
          fields:
            - name: "product"
              type: "object"
              fields:
                - "id"
                - "name"
                - "description"
            - name: "variants"
              type: "list[object]"
              fields:
                - "color"
                - "size"
                - "price"
            - name: "categories"
              type: "list[string]"
            - "updatedAt"
`

	tool, err := CreateTool(yamlInput)
	if err != nil {
		t.Fatalf("Failed to parse YAML: %v", err)
	}

	// Test SimpleObject output
	simpleObjectFn := tool.Tools[0].Functions[0]
	if simpleObjectFn.Name != "SimpleObject" {
		t.Errorf("Expected function name 'SimpleObject', got %s", simpleObjectFn.Name)
	}

	if simpleObjectFn.Output.Type != StepOutputObject {
		t.Errorf("Expected output type 'object', got %s", simpleObjectFn.Output.Type)
	}

	if len(simpleObjectFn.Output.Fields) != 3 {
		t.Errorf("Expected 3 fields, got %d", len(simpleObjectFn.Output.Fields))
	} else {
		expectedFields := []string{"name", "description", "price"}
		for i, expected := range expectedFields {
			if simpleObjectFn.Output.Fields[i].Value != expected {
				t.Errorf("Expected field %d to be %s, got %s", i, expected, simpleObjectFn.Output.Fields[i].Value)
			}
		}
	}

	// Test ListOfStrings output
	listStringsFn := tool.Tools[0].Functions[1]
	if listStringsFn.Name != "ListOfStrings" {
		t.Errorf("Expected function name 'ListOfStrings', got %s", listStringsFn.Name)
	}

	if listStringsFn.Output.Type != StepOutputListString {
		t.Errorf("Expected output type 'list[string]', got %s", listStringsFn.Output.Type)
	}

	if len(listStringsFn.Output.Fields) != 0 {
		t.Errorf("Expected 0 fields for list[string], got %d", len(listStringsFn.Output.Fields))
	}

	// Test ListOfNumbers output
	listNumbersFn := tool.Tools[0].Functions[2]
	if listNumbersFn.Output.Type != StepOutputListNumber {
		t.Errorf("Expected output type 'list[number]', got %s", listNumbersFn.Output.Type)
	}

	// Test ComplexNestedOutput
	complexFn := tool.Tools[0].Functions[3]
	if len(complexFn.Output.Fields) != 4 {
		t.Errorf("Expected 4 fields in complex output, got %d", len(complexFn.Output.Fields))
	} else {
		// Check product object field
		productField := complexFn.Output.Fields[0]
		if productField.Name != "product" || productField.Type != StepOutputObject {
			t.Errorf("Expected product field with object type, got name=%s, type=%s",
				productField.Name, productField.Type)
		}

		if len(productField.Fields) != 3 {
			t.Errorf("Expected 3 nested fields in product, got %d", len(productField.Fields))
		}

		// Check variants list[object] field
		variantsField := complexFn.Output.Fields[1]
		if variantsField.Name != "variants" || variantsField.Type != StepOutputListOfObject {
			t.Errorf("Expected variants field with list[object] type, got name=%s, type=%s",
				variantsField.Name, variantsField.Type)
		}

		// Check categories list[string] field
		categoriesField := complexFn.Output.Fields[2]
		if categoriesField.Name != "categories" || categoriesField.Type != StepOutputListString {
			t.Errorf("Expected categories field with list[string] type, got name=%s, type=%s",
				categoriesField.Name, categoriesField.Type)
		}

		// Check simple field
		simpleField := complexFn.Output.Fields[3]
		if simpleField.Value != "updatedAt" {
			t.Errorf("Expected simple field value 'updatedAt', got %s", simpleField.Value)
		}
	}
}

func TestValidateSteps(t *testing.T) {
	createBaseFunction := func(operation string) Function {
		return Function{
			Name:      "test_function",
			Operation: operation,
			Steps:     []Step{},
			ZeroState: ZeroState{Steps: []Step{}},
		}
	}

	t.Run("Valid web_browse steps", func(t *testing.T) {
		fn := createBaseFunction(OperationWeb)
		fn.Steps = []Step{
			{
				Name:   "open browser",
				Action: StepActionURL,
				With: map[string]interface{}{
					StepWithURL:           "https://example.com",
					StepWithIncognitoMode: true,
				},
			},
			{
				Name:   "read page",
				Action: StepActionExtractText,
				With:   map[string]interface{}{},
				Goal:   "",
			},
			{
				Name:   "fill form",
				Action: StepActionFill,
				With: map[string]interface{}{
					StepWithFillValue: "test input",
				},
			},
			{
				Name:   "click button",
				Action: StepActionFindClick,
				With: map[string]interface{}{
					StepWithFindBy:    StepWithFindById,
					StepWithFindValue: "submit-button",
				},
			},
			{
				Name:   "fill and tab",
				Action: StepActionFindFillTab,
				With: map[string]interface{}{
					StepWithFindBy:    StepWithFindByLabel,
					StepWithFindValue: "Username",
					StepWithFillValue: "testuser",
				},
			},
		}

		err := ValidateSteps(fn, Tool{Name: "web_tool"}, []EnvVar{})
		if err != nil {
			t.Errorf("Expected valid web_browse steps to pass validation, got error: %v", err)
		}
	})

	t.Run("Valid desktop_use steps", func(t *testing.T) {
		fn := createBaseFunction(OperationDesktop)
		fn.Steps = []Step{
			{
				Name:   "open app",
				Action: StepActionApp,
				With: map[string]interface{}{
					StepWithApp: "Notepad",
				},
			},
			{
				Name:   "type text",
				Action: StepActionFill,
				With: map[string]interface{}{
					StepWithFillValue: "Hello World",
				},
			},
		}

		err := ValidateSteps(fn, Tool{Name: "desktop_tool"}, []EnvVar{})
		if err != nil {
			t.Errorf("Expected valid desktop_use steps to pass validation, got error: %v", err)
		}
	})

	t.Run("Valid api_call steps", func(t *testing.T) {
		fn := createBaseFunction(OperationAPI)
		fn.Steps = []Step{
			{
				Name:   "make get request",
				Action: StepActionGET,
				With: map[string]interface{}{
					StepWithURL: "https://api.example.com/data",
					Headers: []interface{}{
						map[string]interface{}{
							Key:   "Authorization",
							Value: "Bearer token123",
						},
					},
				},
			},
			{
				Name:   "make post request",
				Action: StepActionPOST,
				With: map[string]interface{}{
					StepWithURL: "https://api.example.com/data",
					StepWithRequestBody: map[string]interface{}{
						StepWithFindByType: StepBodyJSON,
						With: map[string]interface{}{
							"name": "John Doe",
							"age":  30,
						},
					},
				},
			},
		}

		err := ValidateSteps(fn, Tool{Name: "api_tool"}, []EnvVar{})
		if err != nil {
			t.Errorf("Expected valid api_call steps to pass validation, got error: %v", err)
		}
	})

	t.Run("Valid zero-state steps", func(t *testing.T) {
		fn := createBaseFunction(OperationWeb)
		fn.ZeroState = ZeroState{
			Steps: []Step{
				{
					Name:   "init browser",
					Action: StepActionURL,
					With: map[string]interface{}{
						StepWithURL: "https://example.com",
					},
				},
			},
		}

		err := ValidateSteps(fn, Tool{Name: "web_tool"}, []EnvVar{})
		if err != nil {
			t.Errorf("Expected valid zero-state steps to pass validation, got error: %v", err)
		}
	})

	// Error cases
	t.Run("Invalid operation", func(t *testing.T) {
		fn := createBaseFunction("invalid_operation")
		fn.Steps = []Step{
			{
				Name:   "open browser",
				Action: StepActionURL,
				With: map[string]interface{}{
					StepWithURL: "https://example.com",
				},
			},
		}

		err := ValidateSteps(fn, Tool{Name: "invalid_tool"}, []EnvVar{})
		if err == nil {
			t.Error("Expected error for invalid operation, got none")
		} else if !strings.Contains(err.Error(), "unsupported operation") {
			t.Errorf("Expected 'unsupported operation' error, got: %v", err)
		}
	})

	t.Run("Empty step name", func(t *testing.T) {
		fn := createBaseFunction(OperationWeb)
		fn.Steps = []Step{
			{
				Name:   "",
				Action: StepActionURL,
				With: map[string]interface{}{
					StepWithURL: "https://example.com",
				},
			},
		}

		err := ValidateSteps(fn, Tool{Name: "web_tool"}, []EnvVar{})
		if err == nil {
			t.Error("Expected error for empty step name, got none")
		} else if !strings.Contains(err.Error(), "empty name") {
			t.Errorf("Expected 'empty name' error, got: %v", err)
		}
	})

	t.Run("Empty action", func(t *testing.T) {
		fn := createBaseFunction(OperationWeb)
		fn.Steps = []Step{
			{
				Name:   "step one",
				Action: "",
			},
		}

		err := ValidateSteps(fn, Tool{Name: "web_tool"}, []EnvVar{})
		if err == nil {
			t.Error("Expected error for empty action, got none")
		} else if !strings.Contains(err.Error(), "empty action") {
			t.Errorf("Expected 'empty action' error, got: %v", err)
		}
	})

	t.Run("Missing with block for open_url", func(t *testing.T) {
		fn := createBaseFunction(OperationWeb)
		fn.Steps = []Step{
			{
				Name:   "open browser",
				Action: StepActionURL,
			},
		}

		err := ValidateSteps(fn, Tool{Name: "web_tool"}, []EnvVar{})
		if err == nil {
			t.Error("Expected error for missing with block, got none")
		} else if !strings.Contains(err.Error(), "must have a 'with' block") {
			t.Errorf("Expected 'must have a with block' error, got: %v", err)
		}
	})

	t.Run("Missing url for open_url", func(t *testing.T) {
		fn := createBaseFunction(OperationWeb)
		fn.Steps = []Step{
			{
				Name:   "open browser",
				Action: StepActionURL,
				With:   map[string]interface{}{},
			},
		}

		err := ValidateSteps(fn, Tool{Name: "web_tool"}, []EnvVar{})
		if err == nil {
			t.Error("Expected error for missing url, got none")
		} else if !strings.Contains(err.Error(), "must provide a 'url' parameter") {
			t.Errorf("Expected 'must provide a url parameter' error, got: %v", err)
		}
	})

	t.Run("Missing fillValue for fill action", func(t *testing.T) {
		fn := createBaseFunction(OperationWeb)
		fn.Steps = []Step{
			{
				Name:   "fill form",
				Action: StepActionFill,
				With:   map[string]interface{}{},
			},
		}

		err := ValidateSteps(fn, Tool{Name: "web_tool"}, []EnvVar{})
		if err == nil {
			t.Error("Expected error for missing fillValue, got none")
		} else if !strings.Contains(err.Error(), "must provide a 'fillValue' parameter") {
			t.Errorf("Expected 'must provide a fillValue parameter' error, got: %v", err)
		}
	})

	t.Run("Invalid action for web_browse", func(t *testing.T) {
		fn := createBaseFunction(OperationWeb)
		fn.Steps = []Step{
			{
				Name:   "invalid action",
				Action: "invalid_action",
			},
		}
		err := ValidateSteps(fn, Tool{Name: "web_tool"}, []EnvVar{})
		if err == nil {
			t.Error("Expected error for invalid action, got none")
		} else if !strings.Contains(err.Error(), "unsupported action") {
			t.Errorf("Expected 'unsupported action' error, got: %v", err)
		}
	})

	t.Run("Missing with block for find_and_click", func(t *testing.T) {
		fn := createBaseFunction(OperationWeb)
		fn.Steps = []Step{
			{
				Name:   "click button",
				Action: StepActionFindClick,
			},
		}

		err := ValidateSteps(fn, Tool{Name: "web_tool"}, []EnvVar{})
		if err == nil {
			t.Error("Expected error for missing with block, got none")
		} else if !strings.Contains(err.Error(), "must have a 'with' block") {
			t.Errorf("Expected 'must have a with block' error, got: %v", err)
		}
	})

	t.Run("Missing findBy for find_and_click", func(t *testing.T) {
		fn := createBaseFunction(OperationWeb)
		fn.Steps = []Step{
			{
				Name:   "click button",
				Action: StepActionFindClick,
				With:   map[string]interface{}{},
			},
		}

		err := ValidateSteps(fn, Tool{Name: "web_tool"}, []EnvVar{})
		if err == nil {
			t.Error("Expected error for missing findBy, got none")
		} else if !strings.Contains(err.Error(), "must provide a 'findBy' parameter") {
			t.Errorf("Expected 'must provide a findBy parameter' error, got: %v", err)
		}
	})

	t.Run("Invalid findBy value", func(t *testing.T) {
		fn := createBaseFunction(OperationWeb)
		fn.Steps = []Step{
			{
				Name:   "click button",
				Action: StepActionFindClick,
				With: map[string]interface{}{
					StepWithFindBy: "invalid_find_by",
				},
			},
		}

		err := ValidateSteps(fn, Tool{Name: "web_tool"}, []EnvVar{})
		if err == nil {
			t.Error("Expected error for invalid findBy value, got none")
		} else if !strings.Contains(err.Error(), "invalid 'findBy' value") {
			t.Errorf("Expected 'invalid findBy value' error, got: %v", err)
		}
	})

	t.Run("Missing findValue", func(t *testing.T) {
		fn := createBaseFunction(OperationWeb)
		fn.Steps = []Step{
			{
				Name:   "click button",
				Action: StepActionFindClick,
				With: map[string]interface{}{
					StepWithFindBy: StepWithFindById,
				},
			},
		}

		err := ValidateSteps(fn, Tool{Name: "web_tool"}, []EnvVar{})
		if err == nil {
			t.Error("Expected error for missing findValue, got none")
		} else if !strings.Contains(err.Error(), "must provide a 'findValue' parameter") {
			t.Errorf("Expected 'must provide a findValue parameter' error, got: %v", err)
		}
	})

	t.Run("Invalid element type for findBy=type", func(t *testing.T) {
		fn := createBaseFunction(OperationWeb)
		fn.Steps = []Step{
			{
				Name:   "click button",
				Action: StepActionFindClick,
				With: map[string]interface{}{
					StepWithFindBy:    StepWithFindByType,
					StepWithFindValue: "invalid_element_type",
				},
			},
		}

		err := ValidateSteps(fn, Tool{Name: "web_tool"}, []EnvVar{})
		if err == nil {
			t.Error("Expected error for invalid element type, got none")
		} else if !strings.Contains(err.Error(), "invalid element type") {
			t.Errorf("Expected 'invalid element type' error, got: %v", err)
		}
	})

	t.Run("Missing fillValue for find_fill_and_tab", func(t *testing.T) {
		fn := createBaseFunction(OperationWeb)
		fn.Steps = []Step{
			{
				Name:   "fill and tab",
				Action: StepActionFindFillTab,
				With: map[string]interface{}{
					StepWithFindBy:    StepWithFindById,
					StepWithFindValue: "username",
				},
			},
		}

		err := ValidateSteps(fn, Tool{Name: "web_tool"}, []EnvVar{})
		if err == nil {
			t.Error("Expected error for missing fillValue, got none")
		} else if !strings.Contains(err.Error(), "must provide a 'fillValue' parameter") {
			t.Errorf("Expected 'must provide a fillValue parameter' error, got: %v", err)
		}
	})

	t.Run("Missing app_name for open_app in desktop_use", func(t *testing.T) {
		fn := createBaseFunction(OperationDesktop)
		fn.Steps = []Step{
			{
				Name:   "open app",
				Action: StepActionApp,
				With:   map[string]interface{}{},
			},
		}

		err := ValidateSteps(fn, Tool{Name: "desktop_tool"}, []EnvVar{})
		if err == nil {
			t.Error("Expected error for missing app_name, got none")
		} else if !strings.Contains(err.Error(), "must provide an 'app_name' parameter") {
			t.Errorf("Expected 'must provide an app_name parameter' error, got: %v", err)
		}
	})

	t.Run("Invalid action for desktop_use", func(t *testing.T) {
		fn := createBaseFunction(OperationDesktop)
		fn.Steps = []Step{
			{
				Name:   "invalid action",
				Action: "invalid_action",
			},
		}

		err := ValidateSteps(fn, Tool{Name: "desktop_tool"}, []EnvVar{})
		if err == nil {
			t.Error("Expected error for invalid action, got none")
		} else if !strings.Contains(err.Error(), "unsupported action") {
			t.Errorf("Expected 'unsupported action' error, got: %v", err)
		}
	})

	t.Run("Invalid HTTP method for api_call", func(t *testing.T) {
		fn := createBaseFunction(OperationAPI)
		fn.Steps = []Step{
			{
				Name:   "invalid method",
				Action: "INVALID_METHOD",
			},
		}

		err := ValidateSteps(fn, Tool{Name: "api_tool"}, []EnvVar{})
		if err == nil {
			t.Error("Expected error for invalid HTTP method, got none")
		} else if !strings.Contains(err.Error(), "invalid HTTP method") {
			t.Errorf("Expected 'invalid HTTP method' error, got: %v", err)
		}
	})

	t.Run("Invalid requestBody format", func(t *testing.T) {
		fn := createBaseFunction(OperationAPI)
		fn.Steps = []Step{
			{
				Name:   "post request",
				Action: StepActionPOST,
				With: map[string]interface{}{
					"url":         "https://api.example.com",
					"requestBody": "invalid-format", // Should be a map
				},
			},
		}

		err := ValidateSteps(fn, Tool{Name: "api_tool"}, []EnvVar{})
		if err == nil {
			t.Error("Expected error for invalid requestBody format, got none")
		} else if !strings.Contains(err.Error(), "invalid requestBody format") {
			t.Errorf("Expected 'invalid requestBody format' error, got: %v", err)
		}
	})

	t.Run("Missing requestBody type", func(t *testing.T) {
		fn := createBaseFunction(OperationAPI)
		fn.Steps = []Step{
			{
				Name:   "post request",
				Action: StepActionPOST,
				With: map[string]interface{}{
					"url": "https://api.example.com",
					"requestBody": map[string]interface{}{
						"with": map[string]interface{}{
							"data": "value",
						},
					},
				},
			},
		}

		err := ValidateSteps(fn, Tool{Name: "api_tool"}, []EnvVar{})
		if err == nil {
			t.Error("Expected error for missing requestBody type, got none")
		} else if !strings.Contains(err.Error(), "must have a 'type' field") {
			t.Errorf("Expected 'must have a type field' error, got: %v", err)
		}
	})

	t.Run("Invalid content type", func(t *testing.T) {
		fn := createBaseFunction(OperationAPI)
		fn.Steps = []Step{
			{
				Name:   "post request",
				Action: StepActionPOST,
				With: map[string]interface{}{
					"url": "https://api.example.com",
					"requestBody": map[string]interface{}{
						"type": "invalid_content_type",
						"with": map[string]interface{}{
							"data": "value",
						},
					},
				},
			},
		}

		err := ValidateSteps(fn, Tool{Name: "api_tool"}, []EnvVar{})
		if err == nil {
			t.Error("Expected error for invalid content type, got none")
		} else if !strings.Contains(err.Error(), "invalid content type") {
			t.Errorf("Expected 'invalid content type' error, got: %v", err)
		}
	})

	t.Run("Missing 'with' field in requestBody", func(t *testing.T) {
		fn := createBaseFunction(OperationAPI)
		fn.Steps = []Step{
			{
				Name:   "post request",
				Action: StepActionPOST,
				With: map[string]interface{}{
					"url": "https://api.example.com",
					"requestBody": map[string]interface{}{
						"type": StepBodyJSON,
						// missing "with" field
					},
				},
			},
		}

		err := ValidateSteps(fn, Tool{Name: "api_tool"}, []EnvVar{})
		if err == nil {
			t.Error("Expected error for missing 'with' field in requestBody, got none")
		} else if !strings.Contains(err.Error(), "must have a 'with' field") {
			t.Errorf("Expected 'must have a with field' error, got: %v", err)
		}
	})

	t.Run("Invalid headers format", func(t *testing.T) {
		fn := createBaseFunction(OperationAPI)
		fn.Steps = []Step{
			{
				Name:   "get request",
				Action: StepActionGET,
				With: map[string]interface{}{
					"url":     "https://api.example.com",
					"headers": "invalid-headers-format", // Should be an array
				},
			},
		}

		err := ValidateSteps(fn, Tool{Name: "api_tool"}, []EnvVar{})
		if err == nil {
			t.Error("Expected error for invalid headers format, got none")
		} else if !strings.Contains(err.Error(), "invalid headers format") {
			t.Errorf("Expected 'invalid headers format' error, got: %v", err)
		}
	})

	t.Run("Invalid header item format", func(t *testing.T) {
		fn := createBaseFunction(OperationAPI)
		fn.Steps = []Step{
			{
				Name:   "get request",
				Action: StepActionGET,
				With: map[string]interface{}{
					"url": "https://api.example.com",
					"headers": []interface{}{
						"invalid-header-item", // Should be a map
					},
				},
			},
		}

		err := ValidateSteps(fn, Tool{Name: "api_tool"}, []EnvVar{})
		if err == nil {
			t.Error("Expected error for invalid header item format, got none")
		} else if !strings.Contains(err.Error(), "invalid header at index") {
			t.Errorf("Expected 'invalid header at index' error, got: %v", err)
		}
	})

	t.Run("Missing key in header", func(t *testing.T) {
		fn := createBaseFunction(OperationAPI)
		fn.Steps = []Step{
			{
				Name:   "get request",
				Action: StepActionGET,
				With: map[string]interface{}{
					"url": "https://api.example.com",
					"headers": []interface{}{
						map[string]interface{}{
							// Missing "key" field
							"value": "application/json",
						},
					},
				},
			},
		}

		err := ValidateSteps(fn, Tool{Name: "api_tool"}, []EnvVar{})
		if err == nil {
			t.Error("Expected error for missing key in header, got none")
		} else if !strings.Contains(err.Error(), "must have a 'key' field") {
			t.Errorf("Expected 'must have a key field' error, got: %v", err)
		}
	})

	t.Run("Missing value in header", func(t *testing.T) {
		fn := createBaseFunction(OperationAPI)
		fn.Steps = []Step{
			{
				Name:   "get request",
				Action: StepActionGET,
				With: map[string]interface{}{
					"url": "https://api.example.com",
					"headers": []interface{}{
						map[string]interface{}{
							"key": "Content-Type",
							// Missing "value" field
						},
					},
				},
			},
		}

		err := ValidateSteps(fn, Tool{Name: "api_tool"}, []EnvVar{})
		if err == nil {
			t.Error("Expected error for missing value in header, got none")
		} else if !strings.Contains(err.Error(), "must have a 'value' field") {
			t.Errorf("Expected 'must have a value field' error, got: %v", err)
		}
	})

	t.Run("Invalid zero-state step", func(t *testing.T) {
		fn := createBaseFunction(OperationWeb)
		fn.ZeroState = ZeroState{
			Steps: []Step{
				{
					Name:   "init browser",
					Action: StepActionURL,
					// Missing "with" block
				},
			},
		}

		err := ValidateSteps(fn, Tool{Name: "web_tool"}, []EnvVar{})
		if err == nil {
			t.Error("Expected error for invalid zero-state step, got none")
		} else if !strings.Contains(err.Error(), "in zero-state") {
			t.Errorf("Expected error message to contain 'in zero-state', got: %v", err)
		}
	})

	t.Run("Non-string findBy parameter", func(t *testing.T) {
		fn := createBaseFunction(OperationWeb)
		fn.Steps = []Step{
			{
				Name:   "click button",
				Action: StepActionFindClick,
				With: map[string]interface{}{
					StepWithFindBy:    123, // Should be a string
					StepWithFindValue: "submit-button",
				},
			},
		}

		err := ValidateSteps(fn, Tool{Name: "web_tool"}, []EnvVar{})
		if err == nil {
			t.Error("Expected error for non-string findBy parameter, got none")
		} else if !strings.Contains(err.Error(), "invalid 'findBy' format") {
			t.Errorf("Expected 'invalid findBy format' error, got: %v", err)
		}
	})

	t.Run("Non-string findValue for findBy=type", func(t *testing.T) {
		fn := createBaseFunction(OperationWeb)
		fn.Steps = []Step{
			{
				Name:   "click button",
				Action: StepActionFindClick,
				With: map[string]interface{}{
					StepWithFindBy:    StepWithFindByType,
					StepWithFindValue: 123, // Should be a string
				},
			},
		}

		err := ValidateSteps(fn, Tool{Name: "web_tool"}, []EnvVar{})
		if err == nil {
			t.Error("Expected error for non-string findValue, got none")
		} else if !strings.Contains(err.Error(), "invalid 'findValue' format") {
			t.Errorf("Expected 'invalid findValue format' error, got: %v", err)
		}
	})
}

func TestSystemVariablesAndFunctions(t *testing.T) {
	testCases := []struct {
		name          string
		yamlInput     string
		expectError   bool
		errorContains string
	}{
		{
			name: "Valid system variable usage",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "TestFunction"
        operation: "web_browse"
        description: "A test function"
        triggers:
          - type: "always_on_user_message"
        steps:
          - name: "open browser"
            action: "open_url"
            with:
              url: "https://example.com/user/$USER.id"
            onError:
              strategy: "requestUserInput"
              message: "Failed to open URL for $USER.first_name"
`,
			expectError: false,
		},
		{
			name: "Valid system function usage",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "TestFunction"
        operation: "web_browse"
        description: "A test function"
        triggers:
          - type: "always_on_user_message"
        steps:
          - name: "process data"
            action: "extract_text"
            goal: "Use Ask('What is this page about?') to understand the content"
          - name: "notify"
            action: "find_and_click"
            with:
              findBy: "id"
              findValue: "button"
            goal: "If needed, use NotifyHuman('Found an issue', 'admin')"
`,
			expectError: true,
		},
		{
			name: "InValid system variable usage for user field",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "TestFunction"
        operation: "web_browse"
        description: "A test function"
        triggers:
          - type: "always_on_user_message"
        steps:
          - name: "open browser"
            action: "open_url"
            with:
              url: "https://example.com/user/$USER.id"
            onError:
              strategy: "requestUserInput"
              message: "Failed to open URL for $USER.name"
`,
			expectError: true,
		},
		{
			name: "Invalid system variable for trigger type",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "ScheduledTask"
        operation: "web_browse"
        description: "A scheduled task"
        triggers:
          - type: "time_based"
            cron: "0 0 * * *"
        steps:
          - name: "open browser"
            action: "open_url"
            with:
              url: "https://example.com/user/$USER.id"  # $USER not available in time_based trigger
`,
			expectError:   true,
			errorContains: "uses system variable '$USER' which is not available",
		},
		{
			name: "Invalid system function for trigger type",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "ScheduledTask"
        operation: "web_browse"
        description: "A scheduled task"
        triggers:
          - type: "time_based"
            cron: "0 0 * * *"
        steps:
          - name: "process data"
            action: "extract_text"
            goal: "Use AskUser('What should I do next?') to get instructions"  # AskUser not available in time_based trigger
            with: 
`,
			expectError:   true,
			errorContains: "uses system function 'AskUser' which is not available",
		},
		{
			name: "Undefined variable",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "TestFunction"
        operation: "web_browse"
        description: "A test function"
        triggers:
          - type: "always_on_user_message"
        steps:
          - name: "open browser"
            action: "open_url"
            with:
              url: "https://example.com/search?q=$nonexistent"  # Undefined lowercase variable
`,
			expectError:   true,
			errorContains: "references undefined variable '$nonexistent'",
		},
		{
			name: "Environment variable usage",
			yamlInput: `
version: "v1"
author: "Test Author"
env:
  - name: "API_KEY"
    value: "test-key-123"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "TestFunction"
        operation: "api_call"
        description: "An API call"
        triggers:
          - type: "always_on_user_message"
        steps:
          - name: "call api"
            action: "GET"
            with:
              url: "https://api.example.com/data?key=$API_KEY"
`,
			expectError: false,
		},
		{
			name: "Valid input variable reference",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "TestFunction"
        operation: "web_browse"
        description: "A test function"
        triggers:
          - type: "always_on_user_message"
        input:
          - name: "searchTerm"
            description: "Search term"
            origin: "chat"
            onError:
              strategy: "requestUserInput"
              message: "Please provide a search term"
        steps:
          - name: "open browser"
            action: "open_url"
            with:
              url: "https://example.com/search?q=$searchTerm"
`,
			expectError: false,
		},
		{
			name: "Missing input variable (undefined)",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "TestFunction"
        operation: "web_browse"
        description: "A test function"
        triggers:
          - type: "always_on_user_message"
        input:
          - name: "searchTerm"
            description: "Search term"
            origin: "chat"
            onError:
              strategy: "requestUserInput"
              message: "Please provide a search term"
        steps:
          - name: "open browser"
            action: "open_url"
            with:
              url: "https://example.com/search?q=$searchTerm&filter=$missingInput"
`,
			expectError:   true,
			errorContains: "references undefined variable '$missingInput'",
		},
		{
			name: "Mixed valid variables",
			yamlInput: `
version: "v1"
author: "Test Author"
env:
  - name: "API_KEY"
    value: "test-key-123"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "TestFunction"
        operation: "web_browse"
        description: "A test function"
        triggers:
          - type: "always_on_user_message"
        input:
          - name: "query"
            description: "Search query"
            origin: "chat"
            onError:
              strategy: "requestUserInput"
              message: "Please provide a search query"
        steps:
          - name: "open browser"
            action: "open_url"
            with:
              url: "https://example.com/search?q=$query&user=$USER.id&key=$API_KEY"
`,
			expectError: false,
		},
		{
			name: "Valid $FILE system variable usage with all fields",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "UploadFile"
        operation: "api_call"
        description: "Upload file to external API"
        triggers:
          - type: "always_on_user_message"
        steps:
          - name: "upload"
            action: "POST"
            with:
              url: "https://api.example.com/upload"
              requestBody:
                type: "multipart/form-data"
                with:
                  file: "$FILE.path"
                  url: "$FILE.url"
                  mimetype: "$FILE.mimetype"
                  filename: "$FILE.filename"
`,
			expectError: false,
		},
		{
			name: "Invalid $FILE field",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "UploadFile"
        operation: "api_call"
        description: "Upload file"
        triggers:
          - type: "always_on_user_message"
        steps:
          - name: "upload"
            action: "POST"
            with:
              url: "https://api.example.com/upload?file=$FILE.invalidfield"
`,
			expectError:   true,
			errorContains: "invalid field 'invalidfield' for system variable '$FILE'",
		},
		{
			name: "$FILE available in time_based trigger",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "ScheduledUpload"
        operation: "api_call"
        description: "Scheduled file upload"
        triggers:
          - type: "time_based"
            cron: "0 0 * * *"
        steps:
          - name: "upload"
            action: "POST"
            with:
              url: "https://api.example.com/upload"
              requestBody:
                type: "multipart/form-data"
                with:
                  file: "$FILE.path"
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
				} else if !strings.Contains(err.Error(), tc.errorContains) {
					t.Errorf("Expected error containing '%s', but got: %v", tc.errorContains, err)
				}
			} else if err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}
		})
	}
}

// Testing nested fields and complex cases
func TestSystemVariablesAdvanced(t *testing.T) {
	testCases := []struct {
		name          string
		yamlInput     string
		expectError   bool
		errorContains string
	}{
		{
			name: "System variables with nested fields",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "TestFunction"
        operation: "web_browse"
        description: "A test function"
        triggers:
          - type: "always_on_user_message"
        steps:
          - name: "process message"
            action: "extract_text"
            goal: "Analyze message from $MESSAGE.from with humor level $MESSAGE.humor"
`,
			expectError: true,
		},
		{
			name: "Multiple variable types in one string",
			yamlInput: `
version: "v1"
author: "Test Author"
env:
  - name: "COMPANY_NAME"
    value: "Acme Corp"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "TestFunction"
        operation: "web_browse"
        description: "A test function"
        triggers:
          - type: "always_on_user_message"
        input:
          - name: "product"
            description: "Product to search for"
            origin: "chat"
            onError:
              strategy: "requestUserInput"
              message: "Please specify a product"
        steps:
          - name: "open browser"
            action: "open_url"
            with:
              url: "https://example.com/search?q=$product&company=$COMPANY_NAME&user=$USER.id"
`,
			expectError: false,
		},
		{
			name: "RunOnlyIf condition with variables",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "TestFunction"
        operation: "web_browse"
        description: "A test function"
        triggers:
          - type: "always_on_user_message"
        runOnlyIf: "$MESSAGE.hasMedia == true && $USER.preferred_language == 'en'"
        steps:
          - name: "open browser"
            action: "open_url"
            with:
              url: "https://example.com"
`,
			expectError: false,
		},
		{
			name: "Function parameters with variables",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "TestFunction"
        operation: "web_browse"
        description: "A test function"
        triggers:
          - type: "always_on_user_message"
        steps:
          - name: "notify admins"
            action: "extract_text"
            with:
              with:
            goal: "Use NotifyHuman('User $USER.id has sent a message: $MESSAGE.text', 'admin')"
`,
			expectError: true,
		},
		{
			name: "Invalid variable reference format",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "TestFunction"
        operation: "web_browse"
        description: "A test function"
        triggers:
          - type: "always_on_user_message"
        steps:
          - name: "process data"
            action: "extract_text"
            goal: "Analyze $USER..id" # Invalid double dot
`,
			expectError: true,
		},
		{
			name: "System variable with unknown field",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "TestFunction"
        operation: "web_browse"
        description: "A test function"
        triggers:
          - type: "always_on_user_message"
        steps:
          - name: "process data"
            action: "extract_text"
            goal: "Analyze $USER.nonexistentfield" # Unknown field
`,
			expectError: true,
		},
		{
			name: "UUID variable usage without fields",
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
        steps:
          - name: "create request"
            action: "POST"
            with:
              url: "https://api.example.com/resource"
              requestBody:
                type: "application/json"
                with:
                  - key: "id"
                    value: "$UUID"
                  - key: "user"
                    value: "$USER.id"
`,
			expectError: false,
		},
		{
			name: "UUID variable with invalid field",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "TestFunction"
        operation: "web_browse"
        description: "A test function"
        triggers:
          - type: "always_on_user_message"
        successCriteria: "Page loaded successfully"
        steps:
          - name: "process data"
            action: "extract_text"
            goal: "Generate ID $UUID.field" # UUID doesn't support fields
`,
			expectError:   true,
			errorContains: "references invalid field 'field' for system variable '$UUID'",
		},
		{
			name: "UUID in time-based trigger",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "ScheduledTask"
        operation: "api_call"
        description: "A scheduled task"
        triggers:
          - type: "time_based"
            cron: "0 0 * * *"
        steps:
          - name: "create request"
            action: "POST"
            with:
              url: "https://api.example.com/batch"
              requestBody:
                type: "application/json"
                with:
                  - key: "batch_id"
                    value: "$UUID"
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
				} else if !strings.Contains(err.Error(), tc.errorContains) {
					t.Errorf("Expected error containing '%s', but got: %v", tc.errorContains, err)
				}
			} else if err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}
		})
	}
}

// Test custom validation for environment variables
func TestEnvironmentVariableValidation(t *testing.T) {
	testCases := []struct {
		name          string
		yamlInput     string
		expectError   bool
		errorContains string
	}{
		{
			name: "Valid environment variables",
			yamlInput: `
version: "v1"
author: "Test Author"
env:
  - name: "API_KEY"
    value: "test-key-123"
  - name: "BASE_URL"
    value: "https://api.example.com"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "TestFunction"
        operation: "api_call"
        description: "An API call"
        triggers:
          - type: "always_on_user_message"
        steps:
          - name: "call api"
            action: "GET"
            with:
              url: "$BASE_URL/data?key=$API_KEY"
`,
			expectError: false,
		},
		{
			name: "Valid environment variables",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "TestFunction"
        operation: "format"
        description: "An API call"
        triggers:
          - type: "always_on_user_message"

        input:
          - name: "dataFromA"
            description: "Data from A"
            origin: "chat"
            onError:
              strategy: "requestUserInput"
              message: "Error getting data from A"
          - name: "dataFromAb"
            description: "Data from A"
            origin: "inference"
            successCriteria: "inference"
            onError:
              strategy: "requestUserInput"
              message: "Error getting data from A"
`,
			expectError: false,
		},
		{
			name: "Undefined environment variable",
			yamlInput: `
version: "v1"
author: "Test Author"
env:
  - name: "API_KEY"
    value: "test-key-123"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "TestFunction"
        operation: "api_call"
        description: "An API call"
        triggers:
          - type: "always_on_user_message"
        steps:
          - name: "call api"
            action: "GET"
            with:
              url: "https://api.example.com/data?key=$API_KEY&region=$REGION" # REGION is undefined
`,
			expectError:   true,
			errorContains: "is not available",
		},
		{
			name: "Invalid environment variable name",
			yamlInput: `
version: "v1"
author: "Test Author"
env:
  - name: "123invalid"
    value: "test-value"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "TestFunction"
        operation: "web_browse"
        description: "A test function"
        triggers:
          - type: "always_on_user_message"
        steps:
          - name: "open browser"
            action: "open_url"
            with:
              url: "https://example.com"
`,
			expectError:   true,
			errorContains: "not a valid environment variable name",
		},
		{
			name: "All valid fields for USER",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "TestFunction"
        operation: "web_browse"
        description: "A test function"
        triggers:
          - type: "always_on_user_message"
        steps:
          - name: "log user"
            action: "extract_text"
            with:
            goal: "Log $USER.id, $USER.first_name, $USER.last_name, $USER.email, $USER.phone, $USER.gender, $USER.birthday_date, $USER.interests, $USER.preferred_language"
`,
			expectError: false,
		},
		{
			name: "All valid fields for COMPANY",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "TestFunction"
        operation: "web_browse"
        description: "A test function"
        triggers:
          - type: "always_on_user_message"
        steps:
          - name: "log company"
            action: "extract_text"
            goal: "Log $COMPANY.name, $COMPANY.fantasy_name, $COMPANY.tax_code, $COMPANY.industry, $COMPANY.email, $COMPANY.instagram_profile, $COMPANY.website"
`,
			expectError: false,
		},
		{
			name: "Invalid field for MESSAGE",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "TestFunction"
        operation: "web_browse"
        description: "A test function"
        triggers:
          - type: "always_on_user_message"
        steps:
          - name: "analyze message"
            action: "extract_text"
            goal: "Analyze $MESSAGE.content"
`,
			expectError:   true,
			errorContains: "invalid field 'content'",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := CreateTool(tc.yamlInput)

			if tc.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				} else if !strings.Contains(err.Error(), tc.errorContains) {
					t.Errorf("Expected error containing '%s', but got: %v", tc.errorContains, err)
				}
			} else if err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}
		})
	}
}

func TestVariableSyntaxEdgeCases(t *testing.T) {
	testCases := []struct {
		name          string
		yamlInput     string
		expectError   bool
		errorContains string
	}{
		{
			name: "Variable at beginning of string",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "TestFunction"
        operation: "web_browse"
        description: "A test function"
        triggers:
          - type: "always_on_user_message"
        input:
          - name: "query"
            description: "Search query"
            origin: "chat"
        steps:
          - name: "search"
            action: "open_url"
            with:
              url: "$querytest.com" // Should be caught as undefined
`,
			expectError: true,
		},
		{
			name: "Variable next to special characters",
			yamlInput: `
version: "v1"
author: "Test Author"
env:
  - name: "API_KEY"
    value: "secret"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "TestFunction"
        operation: "api_call"
        description: "An API call"
        triggers:
          - type: "always_on_user_message"
        steps:
          - name: "call api"
            action: "GET"
            with:
              url: "https://api.example.com?key=($API_KEY)"
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
				} else if !strings.Contains(err.Error(), tc.errorContains) {
					t.Errorf("Expected error containing '%s', but got: %v", tc.errorContains, err)
				}
			} else if err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}
		})
	}
}

func TestFunctionDependencyValidation(t *testing.T) {
	testCases := []struct {
		name          string
		yamlInput     string
		expectError   bool
		errorContains string
	}{
		{
			name: "Long dependency chain",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "funcA"
        operation: "api_call"
        description: "Function A"
        triggers:
          - type: "flex_for_user"
        steps:
          - name: "step"
            action: "GET"
            with:
              url: "https://example.com"
        output:
          type: "object"
          fields:
            - "dataA"
      - name: "funcB"
        operation: "api_call"
        description: "Function B"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "dataFromA"
            description: "Data from A"
            origin: "function"
            from: "funcA"
            onError:
              strategy: "requestUserInput"
              message: "Error getting data from A"
        steps:
          - name: "step"
            action: "GET"
            with:
              url: "https://example.com"
        output:
          type: "object"
          fields:
            - "dataB"
      - name: "funcC"
        operation: "web_browse"
        description: "Function C"
        successCriteria: "inference"
        triggers:
          - type: "always_on_user_message"
        input:
          - name: "dataFromB"
            description: "Data from B"
            origin: "function"
            from: "funcB"
            onError:
              strategy: "requestUserInput"
              message: "Error getting data from B"
        steps:
          - name: "open browser"
            action: "open_url"
            with:
              url: "https://example.com"
`,
			expectError: false, // Long chains are okay
		},
		{
			name: "Complex circular dependency",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "funcA"
        operation: "api_call"
        description: "Function A"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "dataFromC"
            description: "Data from C"
            origin: "function"
            from: "funcC"
            onError:
              strategy: "requestUserInput"
              message: "Error getting data from C"
        steps:
          - name: "step"
            action: "GET"
            with:
              url: "https://example.com"
        output:
          type: "object"
          fields:
            - "dataA"
      - name: "funcB"
        operation: "api_call"
        description: "Function B"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "dataFromA"
            description: "Data from A"
            origin: "function"
            from: "funcA"
            onError:
              strategy: "requestUserInput"
              message: "Error getting data from A"
        steps:
          - name: "step"
            action: "GET"
            with:
              url: "https://example.com"
        output:
          type: "object"
          fields:
            - "dataB"
      - name: "funcC"
        operation: "api_call"
        description: "Function C"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "dataFromB"
            description: "Data from B"
            origin: "function"
            from: "funcB"
            onError:
              strategy: "requestUserInput"
              message: "Error getting data from B"
        steps:
          - name: "step"
            action: "GET"
            with:
              url: "https://example.com"
        output:
          type: "object"
          fields:
            - "dataC"
`,
			expectError:   true,
			errorContains: "circular dependency",
		},
		{
			name: "Self-referential function",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "selfRef"
        operation: "api_call"
        description: "Self-referential function"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "dataFromSelf"
            description: "Data from self"
            origin: "function"
            from: "selfRef"
            onError:
              strategy: "requestUserInput"
              message: "Error getting data from self"
        steps:
          - name: "step"
            action: "GET"
            with:
              url: "https://example.com"
`,
			expectError:   true,
			errorContains: "circular dependency",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := CreateTool(tc.yamlInput)

			if tc.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				} else if !strings.Contains(err.Error(), tc.errorContains) {
					t.Errorf("Expected error containing '%s', but got: %v", tc.errorContains, err)
				}
			} else if err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}
		})
	}
}

func TestResultIndexValidation(t *testing.T) {
	testCases := []struct {
		name          string
		yamlInput     string
		expectError   bool
		errorContains string
	}{
		{
			name: "Valid result index for API GET",
			yamlInput: `
version: "v1"
author: "Test Author"
env:
  - name: "API_KEY"
    value: "test-key-123"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "TestFunction"
        operation: "api_call"
        description: "An API call with result index"
        triggers:
          - type: "always_on_user_message"
        steps:
          - name: "get orders"
            action: "GET"
            with:
              url: "https://api.example.com/orders"
              headers:
                - key: "Authorization"
                  value: "Bearer $API_KEY"
            resultIndex: 1
          - name: "get details"
            action: "GET"
            with:
              url: "https://api.example.com/orders/details"
              requestBody:
                type: "application/json"
                with:
                  orderId: "result[1].id"
`,
			expectError: false,
		},
		{
			name: "Valid result index for web_browse read",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "TestFunction"
        operation: "web_browse"
        description: "Web browsing with result storage"
        successCriteria: "inference"
        triggers:
          - type: "always_on_user_message"
        steps:
          - name: "open page"
            action: "open_url"
            with:
              url: "https://example.com/products"
          - name: "read products"
            goal: "extract_text"
            action: "extract_text"
            with:
              findBy: "semantic_context"
              findValue: "latest news"
              limit: "3"
            resultIndex: 1
          - name: "click product"
            action: "find_and_click"
            with:
              findBy: "id"
              findValue: "result[1].productId"
`,
			expectError: false,
		},
		{
			name: "Invalid action for result index",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "TestFunction"
        operation: "web_browse"
        description: "Invalid result index"
        successCriteria: "inference"
        triggers:
          - type: "always_on_user_message"
        steps:
          - name: "open page"
            action: "open_url"
            with:
              url: "https://example.com"
            resultIndex: 1
`,
			expectError:   true,
			errorContains: "action 'open_url' cannot store results",
		},
		{
			name: "Invalid result index for POST",
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
        description: "Invalid result index for POST"
        triggers:
          - type: "always_on_user_message"
        steps:
          - name: "create order"
            action: "POST"
            with:
              url: "https://api.example.com/orders"
              requestBody:
                type: "application/json"
                with:
                  productId: "123"
            resultIndex: 1
`,
			expectError:   true,
			errorContains: "action 'POST' cannot store results",
		},
		{
			name: "Duplicate result indices",
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
        description: "Duplicate result indices"
        triggers:
          - type: "always_on_user_message"
        steps:
          - name: "get orders"
            action: "GET"
            with:
              url: "https://api.example.com/orders"
            resultIndex: 1
          - name: "get products"
            action: "GET"
            with:
              url: "https://api.example.com/products"
            resultIndex: 1
`,
			expectError:   true,
			errorContains: "use the same resultIndex",
		},
		{
			name: "Reference to non-existent result",
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
        description: "Reference non-existent result"
        triggers:
          - type: "always_on_user_message"
        steps:
          - name: "get order"
            action: "GET"
            with:
              url: "https://api.example.com/orders/details"
              requestBody:
                type: "application/json"
                with:
                  orderId: "result[2].id"
`,
			expectError:   true,
			errorContains: "references resultIndex 2 which is not defined",
		},
		{
			name: "Negative result index",
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
        description: "Negative result index"
        triggers:
          - type: "always_on_user_message"
        steps:
          - name: "get orders"
            action: "GET"
            with:
              url: "https://api.example.com/orders"
            resultIndex: -1
`,
			expectError:   true,
			errorContains: "has negative resultIndex",
		},
		{
			name: "Complex chain of result references",
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
        description: "Chain of result references"
        triggers:
          - type: "always_on_user_message"
        steps:
          - name: "get users"
            action: "GET"
            with:
              url: "https://api.example.com/users"
            resultIndex: 1
          - name: "get orders"
            action: "GET"
            with:
              url: "https://api.example.com/users/result[1].id/orders"
            resultIndex: 2
          - name: "get order details"
            action: "GET"
            with:
              url: "https://api.example.com/orders/result[2].orderId/details"
            resultIndex: 3
          - name: "get related products"
            action: "GET"
            with:
              url: "https://api.example.com/products?category=result[3].category"
            resultIndex: 4
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
				} else if !strings.Contains(err.Error(), tc.errorContains) {
					t.Errorf("Expected error containing '%s', but got: %v", tc.errorContains, err)
				}
			} else if err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}
		})
	}
}

func TestValidateFunctionNeeds(t *testing.T) {
	testCases := []struct {
		name     string
		yamlData string
		wantErr  bool
		errMsg   string
	}{
		{
			name: "Valid needs - Tool function",
			yamlData: `
version: "v1"
author: "Test Author"
env:
  - name: "API_KEY"
    value: "test-key"
tools:
  - name: "TestTool"
    description: "Tool for testing"
    version: "1.0.0"
    functions:
      - name: "helperFunc"
        operation: "api_call"
        description: "Helper function"
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "Step1"
            action: "GET"
            with:
              url: "https://api.example.com/data"
        output:
          type: "object"
          fields:
            - "result"
            
      - name: "mainFunc"
        operation: "web_browse"
        description: "Main function that needs helperFunc"
        successCriteria: "inference"
        needs: ["helperFunc"]
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "Step1"
            action: "open_url"
            with:
              url: "https://example.com"
        output:
          type: "object"
          fields:
            - "status"
`,
			wantErr: false,
		},
		{
			name: "Invalid needs - Non-existent function",
			yamlData: `
version: "v1"
author: "Test Author"
env:
  - name: "API_KEY"
    value: "test-key"
tools:
  - name: "TestTool"
    description: "Tool for testing"
    version: "1.0.0"
    functions:
      - name: "mainFunc"
        operation: "web_browse"
        description: "Main function with invalid dependency"
        needs: ["nonExistentFunc"]
        successCriteria: "inference"
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "Step1"
            action: "open_url"
            with:
              url: "https://example.com"
        output:
          type: "object"
          fields:
            - "status"
`,
			wantErr: true,
			errMsg:  "is not available in the tool or as a system function",
		},
		{
			name: "Invalid needs - Self dependency",
			yamlData: `
version: "v1"
author: "Test Author"
env:
  - name: "API_KEY"
    value: "test-key"
tools:
  - name: "TestTool"
    description: "Tool for testing"
    version: "1.0.0"
    functions:
      - name: "selfRefFunc"
        operation: "web_browse"
        description: "Function that references itself"
        successCriteria: "inference"
        needs: ["selfRefFunc"]
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "Step1"
            action: "open_url"
            with:
              url: "https://example.com"
        output:
          type: "object"
          fields:
            - "status"
`,
			wantErr: true,
			errMsg:  "cannot include itself in its 'needs' list",
		},
		{
			name: "Invalid needs - Circular dependency",
			yamlData: `
version: "v1"
author: "Test Author"
env:
  - name: "API_KEY"
    value: "test-key"
tools:
  - name: "TestTool"
    description: "Tool for testing"
    version: "1.0.0"
    functions:
      - name: "funcA"
        operation: "web_browse"
        description: "Function A"
        successCriteria: "inference"
        needs: ["funcB"]
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "Step1"
            action: "open_url"
            with:
              url: "https://example.com"
        output:
          type: "object"
          fields:
            - "status"

      - name: "funcB"
        operation: "web_browse"
        description: "Function B"
        successCriteria: "inference"
        needs: ["funcA"]
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "Step1"
            action: "open_url"
            with:
              url: "https://example.com"
        output:
          type: "object"
          fields:
            - "status"
`,
			wantErr: true,
			errMsg:  "circular dependency detected",
		},
		{
			name: "Valid needs - askToKnowledgeBase with query",
			yamlData: `
version: "v1"
author: "Test Author"
env:
  - name: "API_KEY"
    value: "test-key"
tools:
  - name: "TestTool"
    description: "Tool for testing"
    version: "1.0.0"
    functions:
      - name: "mainFunc"
        operation: "api_call"
        description: "Main function that needs knowledge base"
        successCriteria: "inference"
        needs:
          - name: "askToKnowledgeBase"
            query: "what are the company policies?"
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "Step1"
            action: "GET"
            with:
              url: "https://api.example.com/data"
        output:
          type: "object"
          fields:
            - "status"
`,
			wantErr: false,
		},
		{
			name: "Valid needs - Mixed format with askToKnowledgeBase and regular function",
			yamlData: `
version: "v1"
author: "Test Author"
env:
  - name: "API_KEY"
    value: "test-key"
tools:
  - name: "TestTool"
    description: "Tool for testing"
    version: "1.0.0"
    functions:
      - name: "helperFunc"
        operation: "api_call"
        description: "Helper function"
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "Step1"
            action: "GET"
            with:
              url: "https://api.example.com/data"
        output:
          type: "object"
          fields:
            - "result"

      - name: "mainFunc"
        operation: "api_call"
        description: "Main function with mixed needs"
        successCriteria: "inference"
        needs:
          - name: "askToKnowledgeBase"
            query: "what are the policies?"
          - "helperFunc"
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "Step1"
            action: "GET"
            with:
              url: "https://api.example.com/data"
        output:
          type: "object"
          fields:
            - "status"
`,
			wantErr: false,
		},
		{
			name: "Invalid needs - askToKnowledgeBase without query",
			yamlData: `
version: "v1"
author: "Test Author"
env:
  - name: "API_KEY"
    value: "test-key"
tools:
  - name: "TestTool"
    description: "Tool for testing"
    version: "1.0.0"
    functions:
      - name: "mainFunc"
        operation: "api_call"
        description: "Main function with invalid askToKnowledgeBase"
        successCriteria: "inference"
        needs:
          - name: "askToKnowledgeBase"
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "Step1"
            action: "GET"
            with:
              url: "https://api.example.com/data"
        output:
          type: "object"
          fields:
            - "status"
`,
			wantErr: true,
			errMsg:  "must have a query parameter",
		},
		{
			name: "Invalid needs - System function Ask in needs",
			yamlData: `
version: "v1"
author: "Test Author"
env:
  - name: "API_KEY"
    value: "test-key"
tools:
  - name: "TestTool"
    description: "Tool for testing"
    version: "1.0.0"
    functions:
      - name: "mainFunc"
        operation: "api_call"
        description: "Main function with Ask in needs"
        successCriteria: "inference"
        needs:
          - "Ask"
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "Step1"
            action: "GET"
            with:
              url: "https://api.example.com/data"
        output:
          type: "object"
          fields:
            - "status"
`,
			wantErr: true,
			errMsg:  "is not allowed in 'needs', only 'askToKnowledgeBase' and system functions with params support are allowed",
		},
		{
			name: "Invalid needs - Regular function with query parameter",
			yamlData: `
version: "v1"
author: "Test Author"
env:
  - name: "API_KEY"
    value: "test-key"
tools:
  - name: "TestTool"
    description: "Tool for testing"
    version: "1.0.0"
    functions:
      - name: "helperFunc"
        operation: "api_call"
        description: "Helper function"
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "Step1"
            action: "GET"
            with:
              url: "https://api.example.com/data"
        output:
          type: "object"
          fields:
            - "result"

      - name: "mainFunc"
        operation: "api_call"
        description: "Main function with invalid needs format"
        successCriteria: "inference"
        needs:
          - name: "helperFunc"
            query: "some query"
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "Step1"
            action: "GET"
            with:
              url: "https://api.example.com/data"
        output:
          type: "object"
          fields:
            - "status"
`,
			wantErr: true,
			errMsg:  "cannot have a query parameter in 'needs'",
		},
		{
			name: "Invalid needs - AskHuman with query parameter",
			yamlData: `
version: "v1"
author: "Test Author"
env:
  - name: "API_KEY"
    value: "test-key"
tools:
  - name: "TestTool"
    description: "Tool for testing"
    version: "1.0.0"
    functions:
      - name: "mainFunc"
        operation: "api_call"
        description: "Main function with AskHuman in needs"
        successCriteria: "inference"
        needs:
          - name: "AskHuman"
            query: "some query"
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "Step1"
            action: "GET"
            with:
              url: "https://api.example.com/data"
        output:
          type: "object"
          fields:
            - "status"
`,
			wantErr: true,
			errMsg:  "currently only 'askToKnowledgeBase' and 'queryMemories' are allowed in 'needs' with a query parameter",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := CreateTool(tc.yamlData)

			if tc.wantErr {
				if err == nil {
					t.Errorf("Expected error but got none")
				} else if tc.errMsg != "" && !strings.Contains(err.Error(), tc.errMsg) {
					t.Errorf("Expected error to contain '%s', got '%s'", tc.errMsg, err.Error())
				}
			} else if err != nil {
				t.Errorf("Did not expect error but got: %v", err)
			}
		})
	}
}

func TestStringOutputType(t *testing.T) {
	testCases := []struct {
		name     string
		yamlData string
		wantErr  bool
		errMsg   string
	}{
		{
			name: "Valid string output type with simple value",
			yamlData: `
version: "v1"
author: "Test Author"
env:
  - name: "API_KEY"
    value: "test-key"
tools:
  - name: "TestTool"
    description: "Tool for testing"
    version: "1.0.0"
    functions:
      - name: "StringFunction"
        operation: "api_call"
        description: "Function with string output"
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "Step1"
            action: "GET"
            with:
              url: "https://api.example.com/data"
        output:
          type: "string"
          value: "This is a simple string response"
`,
			wantErr: false,
		},
		{
			name: "Valid string output with environment variables",
			yamlData: `
version: "v1"
author: "Test Author"
env:
  - name: "API_KEY"
    value: "test-key"
  - name: "USERNAME"
    value: "testuser"
tools:
  - name: "TestTool"
    description: "Tool for testing"
    version: "1.0.0"
    functions:
      - name: "StringFunction"
        operation: "api_call"
        description: "Function with string output using env vars"
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "Step1"
            action: "GET"
            with:
              url: "https://api.example.com/data"
        output:
          type: "string"
          value: "Hello $USERNAME, your API key is $API_KEY"
`,
			wantErr: false,
		},
		{
			name: "Invalid string output - missing value",
			yamlData: `
version: "v1"
author: "Test Author"
env:
  - name: "API_KEY"
    value: "test-key"
tools:
  - name: "TestTool"
    description: "Tool for testing"
    version: "1.0.0"
    functions:
      - name: "StringFunction"
        operation: "api_call"
        description: "Function with string output but missing value"
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "Step1"
            action: "GET"
            with:
              url: "https://api.example.com/data"
        output:
          type: "string"
`,
			wantErr: true,
			errMsg:  "must have a 'value' property",
		},
		{
			name: "Invalid string output - with fields",
			yamlData: `
version: "v1"
author: "Test Author"
env:
  - name: "API_KEY"
    value: "test-key"
tools:
  - name: "TestTool"
    description: "Tool for testing"
    version: "1.0.0"
    functions:
      - name: "StringFunction"
        operation: "api_call"
        description: "Function with string output incorrectly using fields"
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "Step1"
            action: "GET"
            with:
              url: "https://api.example.com/data"
        output:
          type: "string"
          value: "Hello user"
          fields:
            - "status"
`,
			wantErr: true,
			errMsg:  "should not have fields",
		},
		{
			name: "String output with system variables",
			yamlData: `
version: "v1"
author: "Test Author"
env:
  - name: "API_KEY"
    value: "test-key"
tools:
  - name: "TestTool"
    description: "Tool for testing"
    version: "1.0.0"
    functions:
      - name: "StringFunction"
        operation: "api_call"
        description: "Function with string output using system vars"
        triggers:
          - type: "always_on_user_message"
        input: []
        steps:
          - name: "Step1"
            action: "GET"
            with:
              url: "https://api.example.com/data"
        output:
          type: "string"
          value: "Hello $USER.first_name, the time is $NOW"
`,
			wantErr: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := CreateTool(tc.yamlData)

			if tc.wantErr {
				if err == nil {
					t.Errorf("Expected error but got none")
				} else if tc.errMsg != "" && !strings.Contains(err.Error(), tc.errMsg) {
					t.Errorf("Expected error to contain '%s', got '%s'", tc.errMsg, err.Error())
				}
			} else if err != nil {
				t.Errorf("Did not expect error but got: %v", err)
			}
		})
	}
}

func TestValidateMCPOperation(t *testing.T) {
	testCases := []struct {
		name     string
		yamlData string
		wantErr  bool
		errMsg   string
	}{
		{
			name: "Valid MCP operation with stdio protocol",
			yamlData: `
version: "v1"
author: "Test Author"
env:
  - name: "API_KEY"
    value: "test-key"
tools:
  - name: "MCPTool"
    description: "Tool for MCP operations"
    version: "1.0.0"
    functions:
      - name: "getUserProfile"
        operation: "mcp"
        description: "Get user profile from MCP server"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "userId"
            isOptional: false
            description: "The user ID to fetch profile for"
            origin: "chat"
        mcp:
          protocol: "stdio"
          stdio:
            command: "python"
            args: ["./server.py", "--debug"]
            env:
              LOG_LEVEL: "debug"
          function: "getUserById"
`,
			wantErr: false,
		},
		{
			name: "Valid MCP operation with stdio protocol and origin inference",
			yamlData: `
version: "v1"
author: "Test Author"
env:
  - name: "API_KEY"
    value: "test-key"
tools:
  - name: "MCPTool"
    description: "Tool for MCP operations"
    version: "1.0.0"
    functions:
      - name: "getUserProfile"
        operation: "mcp"
        description: "Get user profile from MCP server"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "userId"
            isOptional: false
            description: "The user ID to fetch profile for"
            origin: "inference"
            successCriteria: "inference"
        mcp:
          protocol: "stdio"
          stdio:
            command: "python"
            args: ["./server.py", "--debug"]
            env:
              LOG_LEVEL: "debug"
          function: "getUserById"
`,
			wantErr: false,
		},
		{
			name: "Valid MCP operation with stdio protocol and origin search",
			yamlData: `
version: "v1"
author: "Test Author"
env:
  - name: "API_KEY"
    value: "test-key"
tools:
  - name: "MCPTool"
    description: "Tool for MCP operations"
    version: "1.0.0"
    functions:
      - name: "getUserProfile"
        operation: "mcp"
        description: "Get user profile from MCP server"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "userId"
            isOptional: false
            description: "The user ID to fetch profile for"
            origin: "search"
        mcp:
          protocol: "stdio"
          stdio:
            command: "python"
            args: ["./server.py", "--debug"]
            env:
              LOG_LEVEL: "debug"
          function: "getUserById"
`,
			wantErr: false,
		},
		{
			name: "Valid MCP operation with sse protocol",
			yamlData: `
version: "v1"
author: "Test Author"
env:
  - name: "API_KEY"
    value: "test-key"
tools:
  - name: "MCPTool"
    description: "Tool for MCP operations"
    version: "1.0.0"
    functions:
      - name: "getUserActivity"
        operation: "mcp"
        description: "Monitor user activity via SSE"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "userId"
            isOptional: false
            description: "The user ID to monitor"
            origin: "chat"
        mcp:
          protocol: "sse"
          sse:
            url: "http://localhost:8000/sse"
          function: "monitorUserActivity"
`,
			wantErr: false,
		},
		{
			name: "MCP operation - missing protocol",
			yamlData: `
version: "v1"
author: "Test Author"
env:
  - name: "API_KEY"
    value: "test-key"
tools:
  - name: "MCPTool"
    description: "Tool for MCP operations"
    version: "1.0.0"
    functions:
      - name: "getUserProfile"
        operation: "mcp"
        description: "Get user profile from MCP server"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "userId"
            isOptional: false
            description: "The user ID to fetch profile for"
            origin: "chat"
        mcp:
          function: "getUserById"
`,
			wantErr: true,
			errMsg:  "invalid MCP protocol",
		},
		{
			name: "MCP operation - stdio missing command",
			yamlData: `
version: "v1"
author: "Test Author"
env:
  - name: "API_KEY"
    value: "test-key"
tools:
  - name: "MCPTool"
    description: "Tool for MCP operations"
    version: "1.0.0"
    functions:
      - name: "getUserProfile"
        operation: "mcp"
        description: "Get user profile from MCP server"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "userId"
            isOptional: false
            description: "The user ID to fetch profile for"
            origin: "chat"
        mcp:
          protocol: "stdio"
          stdio:
            args: ["./server.py"]
          function: "getUserById"
`,
			wantErr: true,
			errMsg:  "missing required 'command' field",
		},
		{
			name: "MCP operation - sse missing url",
			yamlData: `
version: "v1"
author: "Test Author"
env:
  - name: "API_KEY"
    value: "test-key"
tools:
  - name: "MCPTool"
    description: "Tool for MCP operations"
    version: "1.0.0"
    functions:
      - name: "getUserActivity"
        operation: "mcp"
        description: "Monitor user activity via SSE"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "userId"
            isOptional: false
            description: "The user ID to monitor"
            origin: "chat"
        mcp:
          protocol: "sse"
          sse: {}
          function: "monitorUserActivity"
`,
			wantErr: true,
			errMsg:  "missing required 'url' field",
		},
		{
			name: "MCP operation - missing function name",
			yamlData: `
version: "v1"
author: "Test Author"
env:
  - name: "API_KEY"
    value: "test-key"
tools:
  - name: "MCPTool"
    description: "Tool for MCP operations"
    version: "1.0.0"
    functions:
      - name: "getUserProfile"
        operation: "mcp"
        description: "Get user profile from MCP server"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "userId"
            isOptional: false
            description: "The user ID to fetch profile for"
            origin: "chat"
        mcp:
          protocol: "stdio"
          stdio:
            command: "python"
            args: ["./server.py"]
`,
			wantErr: true,
			errMsg:  "missing required 'function' field",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := CreateTool(tc.yamlData)

			if tc.wantErr {
				if err == nil {
					t.Errorf("Expected error but got none")
				} else if tc.errMsg != "" && !strings.Contains(err.Error(), tc.errMsg) {
					t.Errorf("Expected error to contain '%s', got '%s'", tc.errMsg, err.Error())
				}
			} else if err != nil {
				t.Errorf("Did not expect error but got: %v", err)
			}
		})
	}
}

func BASE(t *testing.T) {
	testCases := []struct {
		name          string
		yamlInput     string
		expectError   bool
		errorContains string
	}{}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := CreateTool(tc.yamlInput)

			if tc.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				} else if !strings.Contains(err.Error(), tc.errorContains) {
					t.Errorf("Expected error containing '%s', but got: %v", tc.errorContains, err)
				}
			} else if err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}
		})
	}
}

func TestCreateTool_DBOperation(t *testing.T) {
	testCases := []struct {
		name      string
		yamlInput string
		wantErr   bool
		errMsg    string
	}{
		{
			name: "Valid DB operation with init and steps",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "UserDB"
    description: "User database operations"
    version: "1.0.0"
    functions:
      - name: "ManageUsers"
        operation: "db"
        description: "Manage user data"
        successCriteria: "inference"
        triggers:
          - type: "always_on_user_message"
        input:
          - name: "userId"
            description: "The user ID"
            origin: "chat"
            onError:
              strategy: "requestUserInput"
              message: "Please provide a user ID"
        with:
          init: |
            CREATE TABLE IF NOT EXISTS users (
              id INTEGER PRIMARY KEY,
              name TEXT NOT NULL,
              email TEXT UNIQUE
            )
        steps:
          - name: "insertUser"
            action: "write"
            with:
              write: "INSERT INTO users (id, name, email) VALUES ($userId, 'John Doe', 'john@example.com')"
          - name: "getUser"
            action: "select"
            resultIndex: 1
            with:
              select: "SELECT * FROM users WHERE id = $userId"
`,
			wantErr: false,
		},
		{
			name: "DB operation with init including INSERT statements",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "ConfigDB"
    description: "Configuration database"
    version: "1.0.0"
    functions:
      - name: "InitConfig"
        operation: "db"
        description: "Initialize configuration"
        successCriteria: "inference"
        triggers:
          - type: "always_on_user_message"
        input:
          - name: "action"
            description: "Action to perform"
            origin: "chat"
            onError:
              strategy: "requestUserInput"
              message: "What action?"
        with:
          init: |
            CREATE TABLE IF NOT EXISTS config (
              key TEXT PRIMARY KEY,
              value TEXT NOT NULL,
              updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
            );
            INSERT OR IGNORE INTO config (key, value) VALUES 
              ('theme', 'light'),
              ('language', 'en');
        steps:
          - name: "getConfig"
            action: "select"
            with:
              select: "SELECT * FROM config"
`,
			wantErr: false,
		},
		{
			name: "DB operation with non-idempotent INSERT",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestDB"
    description: "Test database"
    version: "1.0.0"
    functions:
      - name: "TestFunc"
        operation: "db"
        description: "Test function"
        successCriteria: "inference"
        triggers:
          - type: "always_on_user_message"
        input: []
        with:
          init: |
            CREATE TABLE IF NOT EXISTS test_data (
              id INTEGER PRIMARY KEY,
              value TEXT
            );
            INSERT INTO test_data (id, value) VALUES (1, 'test');
        steps:
          - name: "test"
            action: "select"
            with:
              select: "SELECT * FROM test_data"
`,
			wantErr: true,
			errMsg:  "not idempotent",
		},
		{
			name: "DB operation with invalid step action",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestDB"
    description: "Test database"
    version: "1.0.0"
    functions:
      - name: "TestFunc"
        operation: "db"
        description: "Test function"
        successCriteria: "inference"
        triggers:
          - type: "always_on_user_message"
        input: []
        steps:
          - name: "test"
            action: "invalid_action"
            with:
              invalid_action: "SELECT * FROM users"
`,
			wantErr: true,
			errMsg:  "invalid action",
		},
		{
			name: "DB operation querying uninitialized table",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestDB"
    description: "Test database"
    version: "1.0.0"
    functions:
      - name: "TestFunc"
        operation: "db"
        description: "Test function"
        successCriteria: "inference"
        triggers:
          - type: "always_on_user_message"
        input: []
        with:
          init: "CREATE TABLE IF NOT EXISTS users (id INTEGER PRIMARY KEY)"
        steps:
          - name: "test"
            action: "select"
            with:
              select: "SELECT * FROM products"
`,
			wantErr: true,
			errMsg:  "queries table 'PRODUCTS' which was not initialized",
		},
		{
			name: "DB step missing SQL query",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestDB"
    description: "Test database"
    version: "1.0.0"
    functions:
      - name: "TestFunc"
        operation: "db"
        description: "Test function"
        successCriteria: "inference"
        triggers:
          - type: "always_on_user_message"
        input: []
        steps:
          - name: "test"
            action: "select"
`,
			wantErr: true,
			errMsg:  "must have a 'with' block",
		},
		{
			name: "DB operation with invalid SQL syntax",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestDB"
    description: "Test database"
    version: "1.0.0"
    functions:
      - name: "TestFunc"
        operation: "db"
        description: "Test function"
        successCriteria: "inference"
        triggers:
          - type: "always_on_user_message"
        input: []
        with:
          init: "CREATE TABL users (id INTEGER PRIMARY KEY)"
        steps:
          - name: "test"
            action: "select"
            with:
              select: "SELECT * FROM users"
`,
			wantErr: true,
			errMsg:  "invalid init SQL",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := CreateTool(tc.yamlInput)

			if tc.wantErr {
				if err == nil {
					t.Errorf("Expected error containing '%s', but got no error", tc.errMsg)
				} else if !strings.Contains(err.Error(), tc.errMsg) {
					t.Errorf("Expected error containing '%s', but got: %v", tc.errMsg, err)
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error, but got: %v", err)
				}
			}
		})
	}
}

func TestIsAuthenticationValidation(t *testing.T) {
	t.Run("isAuthentication with api_call operation should be valid", func(t *testing.T) {
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
        description: "A test function"
        triggers:
          - type: "always_on_user_message"
        steps:
          - name: "authenticate"
            action: "POST"
            isAuthentication: true
            with:
              url: "https://api.example.com/auth"
          - name: "api call"
            action: "GET"
            with:
              url: "https://api.example.com/data"
`
		_, err := CreateTool(yamlInput)
		if err != nil {
			t.Errorf("Expected no error for isAuthentication with api_call operation, got: %v", err)
		}
	})

	t.Run("isAuthentication with web_browse operation should fail", func(t *testing.T) {
		yamlInput := `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "TestFunction"
        operation: "web_browse"
        description: "A test function"
        successCriteria: "test criteria"
        triggers:
          - type: "always_on_user_message"
        steps:
          - name: "open browser"
            action: "open_url"
            isAuthentication: true
            with:
              url: "https://example.com"
`
		_, err := CreateTool(yamlInput)
		if err == nil {
			t.Error("Expected error for isAuthentication with web_browse operation, got none")
		} else if !strings.Contains(err.Error(), "isAuthentication") ||
			!strings.Contains(err.Error(), "only applicable to 'api_call' operations") {
			t.Errorf("Expected error about isAuthentication being only for api_call, got: %v", err)
		}
	})

	t.Run("isAuthentication with format operation should fail", func(t *testing.T) {
		yamlInput := `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "TestFunction"
        operation: "format"
        description: "A test function"
        triggers:
          - type: "always_on_user_message"
        input:
          - name: "data"
            description: "Input data"
            origin: "inference"
            successCriteria: "test"
            onError:
              strategy: "requestUserInput"
              message: "Please provide data"
        steps:
          - name: "format data"
            action: "format"
            isAuthentication: true
            with:
              template: "formatted: {{data}}"
`
		_, err := CreateTool(yamlInput)
		if err == nil {
			t.Error("Expected error for isAuthentication with format operation, got none")
		} else if !strings.Contains(err.Error(), "isAuthentication") ||
			!strings.Contains(err.Error(), "only applicable to 'api_call' operations") {
			t.Errorf("Expected error about isAuthentication being only for api_call, got: %v", err)
		}
	})
}

func TestExtractAuthTokenValidation(t *testing.T) {
	t.Run("extractAuthToken with valid header config should be valid", func(t *testing.T) {
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
        description: "A test function"
        triggers:
          - type: "always_on_user_message"
        steps:
          - name: "authenticate"
            action: "POST"
            isAuthentication: true
            with:
              url: "https://api.example.com/auth"
              extractAuthToken:
                from: "header"
                key: "Authorization"
                cache: 3600
          - name: "api call"
            action: "GET"
            with:
              url: "https://api.example.com/data"
`
		_, err := CreateTool(yamlInput)
		if err != nil {
			t.Errorf("Expected no error for valid extractAuthToken config, got: %v", err)
		}
	})

	t.Run("extractAuthToken with responseBody should be valid", func(t *testing.T) {
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
        description: "A test function"
        triggers:
          - type: "always_on_user_message"
        steps:
          - name: "authenticate"
            action: "POST"
            isAuthentication: true
            with:
              url: "https://api.example.com/auth"
              extractAuthToken:
                from: "responseBody"
                key: "data.access_token"
          - name: "api call"
            action: "GET"
            with:
              url: "https://api.example.com/data"
`
		_, err := CreateTool(yamlInput)
		if err != nil {
			t.Errorf("Expected no error for extractAuthToken with responseBody, got: %v", err)
		}
	})

	t.Run("isAuthentication not at index 0 should fail", func(t *testing.T) {
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
        description: "A test function"
        triggers:
          - type: "always_on_user_message"
        steps:
          - name: "get data"
            action: "GET"
            with:
              url: "https://api.example.com/data"
          - name: "authenticate"
            action: "POST"
            isAuthentication: true
            with:
              url: "https://api.example.com/auth"
`
		_, err := CreateTool(yamlInput)
		if err == nil {
			t.Error("Expected error for isAuthentication not at index 0, got none")
		} else if !strings.Contains(err.Error(), "not the first step") ||
			!strings.Contains(err.Error(), "index 0") {
			t.Errorf("Expected error about authentication step needing to be at index 0, got: %v", err)
		}
	})

	t.Run("extractAuthToken without isAuthentication should fail", func(t *testing.T) {
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
        description: "A test function"
        triggers:
          - type: "always_on_user_message"
        steps:
          - name: "api call"
            action: "GET"
            with:
              url: "https://api.example.com/data"
              extractAuthToken:
                from: "header"
                key: "Authorization"
`
		_, err := CreateTool(yamlInput)
		if err == nil {
			t.Error("Expected error for extractAuthToken without isAuthentication, got none")
		} else if !strings.Contains(err.Error(), "extractAuthToken") ||
			!strings.Contains(err.Error(), "isAuthentication") {
			t.Errorf("Expected error about extractAuthToken requiring isAuthentication, got: %v", err)
		}
	})

	t.Run("extractAuthToken with invalid from value should fail", func(t *testing.T) {
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
        description: "A test function"
        triggers:
          - type: "always_on_user_message"
        steps:
          - name: "authenticate"
            action: "POST"
            isAuthentication: true
            with:
              url: "https://api.example.com/auth"
              extractAuthToken:
                from: "invalid"
                key: "Authorization"
          - name: "api call"
            action: "GET"
            with:
              url: "https://api.example.com/data"
`
		_, err := CreateTool(yamlInput)
		if err == nil {
			t.Error("Expected error for invalid extractAuthToken from value, got none")
		} else if !strings.Contains(err.Error(), "from") ||
			!strings.Contains(err.Error(), "header") {
			t.Errorf("Expected error about invalid 'from' value, got: %v", err)
		}
	})

	t.Run("extractAuthToken with missing key should fail", func(t *testing.T) {
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
        description: "A test function"
        triggers:
          - type: "always_on_user_message"
        steps:
          - name: "authenticate"
            action: "POST"
            isAuthentication: true
            with:
              url: "https://api.example.com/auth"
              extractAuthToken:
                from: "header"
          - name: "api call"
            action: "GET"
            with:
              url: "https://api.example.com/data"
`
		_, err := CreateTool(yamlInput)
		if err == nil {
			t.Error("Expected error for extractAuthToken with missing key, got none")
		} else if !strings.Contains(err.Error(), "key") {
			t.Errorf("Expected error about missing 'key' field, got: %v", err)
		}
	})

	t.Run("extractAuthToken with empty key should fail", func(t *testing.T) {
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
        description: "A test function"
        triggers:
          - type: "always_on_user_message"
        steps:
          - name: "authenticate"
            action: "POST"
            isAuthentication: true
            with:
              url: "https://api.example.com/auth"
              extractAuthToken:
                from: "header"
                key: "   "
          - name: "api call"
            action: "GET"
            with:
              url: "https://api.example.com/data"
`
		_, err := CreateTool(yamlInput)
		if err == nil {
			t.Error("Expected error for extractAuthToken with empty key, got none")
		} else if !strings.Contains(err.Error(), "key") ||
			!strings.Contains(err.Error(), "empty") {
			t.Errorf("Expected error about empty 'key' field, got: %v", err)
		}
	})

	t.Run("extractAuthToken with negative cache should fail", func(t *testing.T) {
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
        description: "A test function"
        triggers:
          - type: "always_on_user_message"
        steps:
          - name: "authenticate"
            action: "POST"
            isAuthentication: true
            with:
              url: "https://api.example.com/auth"
              extractAuthToken:
                from: "header"
                key: "Authorization"
                cache: -100
          - name: "api call"
            action: "GET"
            with:
              url: "https://api.example.com/data"
`
		_, err := CreateTool(yamlInput)
		if err == nil {
			t.Error("Expected error for extractAuthToken with negative cache, got none")
		} else if !strings.Contains(err.Error(), "cache") ||
			!strings.Contains(err.Error(), ">= 0") {
			t.Errorf("Expected error about negative cache value, got: %v", err)
		}
	})

	t.Run("extractAuthToken with missing from field should fail", func(t *testing.T) {
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
        description: "A test function"
        triggers:
          - type: "always_on_user_message"
        steps:
          - name: "authenticate"
            action: "POST"
            isAuthentication: true
            with:
              url: "https://api.example.com/auth"
              extractAuthToken:
                key: "Authorization"
          - name: "api call"
            action: "GET"
            with:
              url: "https://api.example.com/data"
`
		_, err := CreateTool(yamlInput)
		if err == nil {
			t.Error("Expected error for extractAuthToken with missing from field, got none")
		} else if !strings.Contains(err.Error(), "from") {
			t.Errorf("Expected error about missing 'from' field, got: %v", err)
		}
	})
}

// TestNotOneOfValidation tests the NotOneOf constraint validation
func TestNotOneOfValidation(t *testing.T) {
	testCases := []struct {
		name          string
		yamlInput     string
		expectedError string
		validation    func(*testing.T, CustomTool)
	}{
		{
			name: "Valid NotOneOf constraint in input",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "getRestrictedLocations"
        operation: "api_call"
        description: "Get restricted server locations"
        triggers:
          - type: "flex_for_user"
        steps:
          - name: "get locations"
            action: "GET"
            with:
              url: "https://api.example.com/restricted-locations"
        output:
          type: "string"
          value: "us-east-1,eu-west-1,ap-south-1"
      - name: "DeployApplication"
        operation: "api_call"
        description: "Deploy application to server"
        triggers:
          - type: "always_on_user_message"
        input:
          - name: "location"
            description: "Server location for deployment"
            origin: "chat"
            with:
              notOneOf: "getRestrictedLocations"
              ttl: 30
            onError:
              strategy: "requestUserInput"
              message: "Please provide a deployment location"
        steps:
          - name: "deploy"
            action: "POST"
            with:
              url: "https://api.example.com/deploy"
`,
			expectedError: "",
			validation: func(t *testing.T, tool CustomTool) {
				deployFunc := tool.Tools[0].Functions[1]
				locationInput := deployFunc.Input[0]

				if locationInput.With == nil {
					t.Error("Expected input to have 'with' options")
					return
				}

				if locationInput.With.NotOneOf != "getRestrictedLocations" {
					t.Errorf("Expected notOneOf to be 'getRestrictedLocations', got '%s'", locationInput.With.NotOneOf)
				}

				if locationInput.With.TTL != 30 {
					t.Errorf("Expected TTL to be 30, got %d", locationInput.With.TTL)
				}
			},
		},
		{
			name: "Valid NotOneOf constraint in onError",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "getForbiddenCategories"
        operation: "api_call"
        description: "Get forbidden categories"
        triggers:
          - type: "flex_for_user"
        steps:
          - name: "get categories"
            action: "GET"
            with:
              url: "https://api.example.com/forbidden-categories"
        output:
          type: "string"
          value: "adult,illegal,dangerous"
      - name: "CreateContent"
        operation: "api_call"
        description: "Create content in category"
        triggers:
          - type: "always_on_user_message"
        input:
          - name: "category"
            description: "Content category"
            origin: "chat"
            onError:
              strategy: "requestUserInput"
              message: "Please provide a content category"
              with:
                notOneOf: "getForbiddenCategories"
                ttl: 60
        steps:
          - name: "create"
            action: "POST"
            with:
              url: "https://api.example.com/content"
`,
			expectedError: "",
			validation: func(t *testing.T, tool CustomTool) {
				createFunc := tool.Tools[0].Functions[1]
				categoryInput := createFunc.Input[0]

				if categoryInput.OnError == nil {
					t.Error("Expected input to have onError")
					return
				}

				if categoryInput.OnError.With == nil {
					t.Error("Expected onError to have 'with' options")
					return
				}

				if categoryInput.OnError.With.NotOneOf != "getForbiddenCategories" {
					t.Errorf("Expected onError notOneOf to be 'getForbiddenCategories', got '%s'", categoryInput.OnError.With.NotOneOf)
				}
			},
		},
		{
			name: "Invalid - Multiple constraint options",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "getOptions"
        operation: "api_call"
        description: "Get options"
        triggers:
          - type: "flex_for_user"
        steps:
          - name: "get options"
            action: "GET"
            with:
              url: "https://api.example.com/options"
        output:
          type: "string"
          value: "option1,option2,option3"
      - name: "TestFunction"
        operation: "api_call"
        description: "Test function"
        triggers:
          - type: "always_on_user_message"
        input:
          - name: "choice"
            description: "User choice"
            origin: "chat"
            with:
              oneOf: "getOptions"
              notOneOf: "getOptions"
            onError:
              strategy: "requestUserInput"
              message: "Please provide a choice"
        steps:
          - name: "test"
            action: "GET"
            with:
              url: "https://api.example.com/test"
`,
			expectedError: "cannot have multiple constraint options",
		},
		{
			name: "Invalid - NotOneOf function not found",
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
        description: "Test function"
        triggers:
          - type: "always_on_user_message"
        input:
          - name: "choice"
            description: "User choice"
            origin: "chat"
            with:
              notOneOf: "nonExistentFunction"
            onError:
              strategy: "requestUserInput"
              message: "Please provide a choice"
        steps:
          - name: "test"
            action: "GET"
            with:
              url: "https://api.example.com/test"
`,
			expectedError: "was not found",
		},
		{
			name: "Invalid - NotOneOf with non-chat origin",
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
        description: "Test function"
        triggers:
          - type: "always_on_user_message"
        input:
          - name: "choice"
            description: "User choice"
            origin: "inference"
            successCriteria: "test criteria"
            with:
              notOneOf: "someFunction"
            onError:
              strategy: "requestUserInput"
              message: "Please provide a choice"
        steps:
          - name: "test"
            action: "GET"
            with:
              url: "https://api.example.com/test"
`,
			expectedError: "with 'with' field must have origin 'chat'",
		},
		{
			name: "Invalid - NotOneOf in onError with wrong strategy",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "getOptions"
        operation: "api_call"
        description: "Get options"
        triggers:
          - type: "flex_for_user"
        steps:
          - name: "get options"
            action: "GET"
            with:
              url: "https://api.example.com/options"
        output:
          type: "string"
          value: "option1,option2,option3"
      - name: "TestFunction"
        operation: "api_call"
        description: "Test function"
        triggers:
          - type: "always_on_user_message"
        input:
          - name: "choice"
            description: "User choice"
            origin: "chat"
            onError:
              strategy: "requestN1Support"
              message: "Please provide a choice"
              with:
                notOneOf: "getOptions"
        steps:
          - name: "test"
            action: "GET"
            with:
              url: "https://api.example.com/test"
`,
			expectedError: "strategy is not 'requestUserInput'",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tool, err := CreateTool(tc.yamlInput)

			if tc.expectedError != "" {
				if err == nil {
					t.Errorf("Expected error containing '%s', but got none", tc.expectedError)
				} else if !strings.Contains(err.Error(), tc.expectedError) {
					t.Errorf("Expected error containing '%s', but got: %v", tc.expectedError, err)
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error, but got: %v", err)
				} else if tc.validation != nil {
					tc.validation(t, tool)
				}
			}
		})
	}
}

func TestCreateTool_GDriveOperation(t *testing.T) {
	testCases := []struct {
		name      string
		yamlInput string
		wantErr   bool
		errMsg    string
	}{
		{
			name: "Valid gdrive list operation",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "DriveManager"
    description: "Google Drive manager"
    version: "1.0.0"
    functions:
      - name: "ListFiles"
        operation: "gdrive"
        description: "List files in a folder"
        successCriteria: "inference"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "folderId"
            description: "The folder ID"
            origin: "chat"
            onError:
              strategy: "requestUserInput"
              message: "Please provide a folder ID"
        steps:
          - name: "listFiles"
            action: "list"
            resultIndex: 1
            with:
              folderId: "$folderId"
              pageSize: 50
        output:
          type: "list[object]"
          fields:
            - name: "fileId"
              type: "string"
            - name: "name"
              type: "string"
`,
			wantErr: false,
		},
		{
			name: "Valid gdrive upload operation",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "DriveUploader"
    description: "Google Drive uploader"
    version: "1.0.0"
    functions:
      - name: "UploadFile"
        operation: "gdrive"
        description: "Upload a file"
        successCriteria: "inference"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "content"
            description: "File content"
            origin: "chat"
            onError:
              strategy: "requestUserInput"
              message: "Please provide file content"
        steps:
          - name: "upload"
            action: "upload"
            resultIndex: 1
            with:
              fileName: "report.pdf"
              content: "$content"
              mimeType: "application/pdf"
        output:
          type: "object"
          fields:
            - name: "fileId"
              type: "string"
            - name: "webViewLink"
              type: "string"
`,
			wantErr: false,
		},
		{
			name: "Valid gdrive download operation with file output",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "DriveDownloader"
    description: "Google Drive downloader"
    version: "1.0.0"
    functions:
      - name: "DownloadFile"
        operation: "gdrive"
        description: "Download a file"
        successCriteria: "inference"
        shouldBeHandledAsMessageToUser: true
        triggers:
          - type: "flex_for_user"
        input:
          - name: "fileId"
            description: "The file ID"
            origin: "chat"
            onError:
              strategy: "requestUserInput"
              message: "Please provide a file ID"
        steps:
          - name: "download"
            action: "download"
            resultIndex: 1
            with:
              fileId: "$fileId"
        output:
          type: "file"
          upload: true
`,
			wantErr: false,
		},
		{
			name: "Valid gdrive multi-step with runOnlyIf",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "DriveOps"
    description: "Google Drive operations"
    version: "1.0.0"
    functions:
      - name: "SearchAndDownload"
        operation: "gdrive"
        description: "Search and download"
        successCriteria: "inference"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "searchTerm"
            description: "Search term"
            origin: "chat"
            onError:
              strategy: "requestUserInput"
              message: "What to search?"
          - name: "fileId"
            description: "File ID to download"
            origin: "chat"
            onError:
              strategy: "requestUserInput"
              message: "What file?"
        steps:
          - name: "search"
            action: "search"
            resultIndex: 1
            with:
              query: "name contains '$searchTerm'"
          - name: "download"
            action: "download"
            resultIndex: 2
            runOnlyIf:
              deterministic: "$fileId != ''"
            with:
              fileId: "$fileId"
        output:
          type: "file"
          upload: true
`,
			wantErr: false,
		},
		{
			name: "Invalid gdrive action rejected",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "DriveTest"
    description: "Test"
    version: "1.0.0"
    functions:
      - name: "BadAction"
        operation: "gdrive"
        description: "Bad action"
        successCriteria: "inference"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "x"
            description: "x"
            origin: "chat"
            onError:
              strategy: "requestUserInput"
              message: "x"
        steps:
          - name: "bad"
            action: "invalid_action"
            with:
              fileId: "abc"
`,
			wantErr: true,
			errMsg:  "invalid gdrive action 'invalid_action'",
		},
		{
			name: "Missing required fileId for download",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "DriveTest"
    description: "Test"
    version: "1.0.0"
    functions:
      - name: "BadDownload"
        operation: "gdrive"
        description: "Download without fileId"
        successCriteria: "inference"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "x"
            description: "x"
            origin: "chat"
            onError:
              strategy: "requestUserInput"
              message: "x"
        steps:
          - name: "dl"
            action: "download"
            with: {}
`,
			wantErr: true,
			errMsg:  "missing required 'with.fileId'",
		},
		{
			name: "Missing required fileName for upload",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "DriveTest"
    description: "Test"
    version: "1.0.0"
    functions:
      - name: "BadUpload"
        operation: "gdrive"
        description: "Upload without fileName"
        successCriteria: "inference"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "x"
            description: "x"
            origin: "chat"
            onError:
              strategy: "requestUserInput"
              message: "x"
        steps:
          - name: "up"
            action: "upload"
            with:
              content: "some content"
`,
			wantErr: true,
			errMsg:  "missing required 'with.fileName'",
		},
		{
			name: "Missing required content for upload",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "DriveTest"
    description: "Test"
    version: "1.0.0"
    functions:
      - name: "BadUpload2"
        operation: "gdrive"
        description: "Upload without content"
        successCriteria: "inference"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "x"
            description: "x"
            origin: "chat"
            onError:
              strategy: "requestUserInput"
              message: "x"
        steps:
          - name: "up"
            action: "upload"
            with:
              fileName: "test.txt"
`,
			wantErr: true,
			errMsg:  "missing required 'with.content'",
		},
		{
			name: "Missing required query for search",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "DriveTest"
    description: "Test"
    version: "1.0.0"
    functions:
      - name: "BadSearch"
        operation: "gdrive"
        description: "Search without query"
        successCriteria: "inference"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "x"
            description: "x"
            origin: "chat"
            onError:
              strategy: "requestUserInput"
              message: "x"
        steps:
          - name: "s"
            action: "search"
            with: {}
`,
			wantErr: true,
			errMsg:  "missing required 'with.query'",
		},
		{
			name: "Missing required targetFolderId for move",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "DriveTest"
    description: "Test"
    version: "1.0.0"
    functions:
      - name: "BadMove"
        operation: "gdrive"
        description: "Move without targetFolderId"
        successCriteria: "inference"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "x"
            description: "x"
            origin: "chat"
            onError:
              strategy: "requestUserInput"
              message: "x"
        steps:
          - name: "m"
            action: "move"
            with:
              fileId: "abc"
`,
			wantErr: true,
			errMsg:  "missing required 'with.targetFolderId'",
		},
		{
			name: "Missing required mimeType for export",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "DriveTest"
    description: "Test"
    version: "1.0.0"
    functions:
      - name: "BadExport"
        operation: "gdrive"
        description: "Export without mimeType"
        successCriteria: "inference"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "x"
            description: "x"
            origin: "chat"
            onError:
              strategy: "requestUserInput"
              message: "x"
        steps:
          - name: "e"
            action: "export"
            with:
              fileId: "abc"
`,
			wantErr: true,
			errMsg:  "missing required 'with.mimeType'",
		},
		{
			name: "Invalid pageSize rejected",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "DriveTest"
    description: "Test"
    version: "1.0.0"
    functions:
      - name: "BadPageSize"
        operation: "gdrive"
        description: "List with invalid pageSize"
        successCriteria: "inference"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "x"
            description: "x"
            origin: "chat"
            onError:
              strategy: "requestUserInput"
              message: "x"
        steps:
          - name: "l"
            action: "list"
            with:
              pageSize: 5000
`,
			wantErr: true,
			errMsg:  "invalid pageSize",
		},
		{
			name: "gdrive with no steps rejected",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "DriveTest"
    description: "Test"
    version: "1.0.0"
    functions:
      - name: "NoSteps"
        operation: "gdrive"
        description: "No steps"
        successCriteria: "inference"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "x"
            description: "x"
            origin: "chat"
            onError:
              strategy: "requestUserInput"
              message: "x"
`,
			wantErr: true,
			errMsg:  "no steps defined",
		},
		{
			name: "Valid gdrive create_folder",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "DriveFolder"
    description: "Folder manager"
    version: "1.0.0"
    functions:
      - name: "CreateFolder"
        operation: "gdrive"
        description: "Create a folder"
        successCriteria: "inference"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "folderName"
            description: "Folder name"
            origin: "chat"
            onError:
              strategy: "requestUserInput"
              message: "Folder name?"
        steps:
          - name: "create"
            action: "create_folder"
            resultIndex: 1
            with:
              fileName: "$folderName"
        output:
          type: "object"
          fields:
            - name: "fileId"
              type: "string"
`,
			wantErr: false,
		},
		{
			name: "Valid gdrive export",
			yamlInput: `
version: "v1"
author: "Test Author"
tools:
  - name: "DriveExporter"
    description: "Doc exporter"
    version: "1.0.0"
    functions:
      - name: "ExportDoc"
        operation: "gdrive"
        description: "Export a Google Doc as PDF"
        successCriteria: "inference"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "docId"
            description: "Doc ID"
            origin: "chat"
            onError:
              strategy: "requestUserInput"
              message: "Doc ID?"
        steps:
          - name: "export"
            action: "export"
            resultIndex: 1
            with:
              fileId: "$docId"
              mimeType: "application/pdf"
              fileName: "exported.pdf"
        output:
          type: "file"
          upload: true
`,
			wantErr: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := CreateTool(tc.yamlInput)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("Expected error containing '%s', but got nil", tc.errMsg)
				}
				if !strings.Contains(err.Error(), tc.errMsg) {
					t.Errorf("Expected error containing '%s', but got: %v", tc.errMsg, err)
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error, but got: %v", err)
				}
			}
		})
	}
}

// =============================================
// KPI Definition Validation Tests
// =============================================

func TestKPIDefinition_ValidParsesCorrectly(t *testing.T) {
	yamlInput := `
version: "v1"
author: "Test"
tools:
  - name: "my_tool"
    description: "A tool with KPIs"
    version: "1.0.0"
    kpis:
      - id: "messages_handled"
        label:
          en: "Messages Handled"
          pt: "Mensagens Atendidas"
        category: "communication"
        category_label:
          en: "Communication"
          pt: "Comunicação"
        event_type: "message_handled"
        aggregation: "count"
        value_type: "number"
        positive_direction: "up"
        comparison: "previous_day"
        featured: true
        order: 1
    functions:
      - name: "DoSomething"
        operation: "web_browse"
        description: "does something"
        successCriteria: "inference"
        triggers:
          - type: "always_on_user_message"
        input:
          - name: "query"
            description: "the query"
            origin: "chat"
            onError:
              strategy: "requestUserInput"
              message: "Please provide input"
        steps:
          - name: "browse"
            action: "open_url"
            with:
              url: "https://example.com"
`
	result, err := CreateTool(yamlInput)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if len(result.Tools[0].Kpis) != 1 {
		t.Fatalf("Expected 1 KPI, got %d", len(result.Tools[0].Kpis))
	}

	kpi := result.Tools[0].Kpis[0]
	if kpi.ID != "messages_handled" {
		t.Errorf("Expected ID 'messages_handled', got '%s'", kpi.ID)
	}
	if kpi.Label.En != "Messages Handled" {
		t.Errorf("Expected label.en 'Messages Handled', got '%s'", kpi.Label.En)
	}
	if kpi.Label.Pt != "Mensagens Atendidas" {
		t.Errorf("Expected label.pt 'Mensagens Atendidas', got '%s'", kpi.Label.Pt)
	}
	if kpi.Aggregation != "count" {
		t.Errorf("Expected aggregation 'count', got '%s'", kpi.Aggregation)
	}
	if !kpi.Featured {
		t.Error("Expected featured to be true")
	}
}

func TestKPIDefinition_MissingID(t *testing.T) {
	yamlInput := `
version: "v1"
author: "Test"
tools:
  - name: "my_tool"
    description: "A tool"
    version: "1.0.0"
    kpis:
      - label: "Messages"
        category: "comm"
        event_type: "msg"
        aggregation: "count"
        value_type: "number"
    functions:
      - name: "DoSomething"
        operation: "web_browse"
        description: "does something"
        successCriteria: "inference"
        triggers:
          - type: "always_on_user_message"
        input:
          - name: "query"
            description: "the query"
            origin: "chat"
            onError:
              strategy: "requestUserInput"
              message: "Please provide input"
        steps:
          - name: "browse"
            action: "open_url"
            with:
              url: "https://example.com"
`
	_, err := CreateTool(yamlInput)
	if err == nil {
		t.Fatal("Expected error for missing KPI id")
	}
	if !strings.Contains(err.Error(), "missing required field 'id'") {
		t.Errorf("Expected 'missing required field id' error, got: %v", err)
	}
}

func TestKPIDefinition_MissingLabel(t *testing.T) {
	yamlInput := `
version: "v1"
author: "Test"
tools:
  - name: "my_tool"
    description: "A tool"
    version: "1.0.0"
    kpis:
      - id: "test_kpi"
        category: "comm"
        event_type: "msg"
        aggregation: "count"
        value_type: "number"
    functions:
      - name: "DoSomething"
        operation: "web_browse"
        description: "does something"
        successCriteria: "inference"
        triggers:
          - type: "always_on_user_message"
        input:
          - name: "query"
            description: "the query"
            origin: "chat"
            onError:
              strategy: "requestUserInput"
              message: "Please provide input"
        steps:
          - name: "browse"
            action: "open_url"
            with:
              url: "https://example.com"
`
	_, err := CreateTool(yamlInput)
	if err == nil {
		t.Fatal("Expected error for missing KPI label")
	}
	if !strings.Contains(err.Error(), "missing required field 'label'") {
		t.Errorf("Expected 'missing required field label' error, got: %v", err)
	}
}

func TestKPIDefinition_InvalidAggregation(t *testing.T) {
	yamlInput := `
version: "v1"
author: "Test"
tools:
  - name: "my_tool"
    description: "A tool"
    version: "1.0.0"
    kpis:
      - id: "test_kpi"
        label: "Test"
        category: "comm"
        event_type: "msg"
        aggregation: "median"
        value_type: "number"
    functions:
      - name: "DoSomething"
        operation: "web_browse"
        description: "does something"
        successCriteria: "inference"
        triggers:
          - type: "always_on_user_message"
        input:
          - name: "query"
            description: "the query"
            origin: "chat"
            onError:
              strategy: "requestUserInput"
              message: "Please provide input"
        steps:
          - name: "browse"
            action: "open_url"
            with:
              url: "https://example.com"
`
	_, err := CreateTool(yamlInput)
	if err == nil {
		t.Fatal("Expected error for invalid aggregation")
	}
	if !strings.Contains(err.Error(), "invalid aggregation") {
		t.Errorf("Expected 'invalid aggregation' error, got: %v", err)
	}
}

func TestKPIDefinition_RatioWithoutNumerator(t *testing.T) {
	yamlInput := `
version: "v1"
author: "Test"
tools:
  - name: "my_tool"
    description: "A tool"
    version: "1.0.0"
    kpis:
      - id: "test_ratio"
        label: "Test Ratio"
        category: "perf"
        aggregation: "ratio"
        value_type: "percentage"
        denominator: "total_events"
    functions:
      - name: "DoSomething"
        operation: "web_browse"
        description: "does something"
        successCriteria: "inference"
        triggers:
          - type: "always_on_user_message"
        input:
          - name: "query"
            description: "the query"
            origin: "chat"
            onError:
              strategy: "requestUserInput"
              message: "Please provide input"
        steps:
          - name: "browse"
            action: "open_url"
            with:
              url: "https://example.com"
`
	_, err := CreateTool(yamlInput)
	if err == nil {
		t.Fatal("Expected error for ratio without numerator")
	}
	if !strings.Contains(err.Error(), "requires both 'numerator' and 'denominator'") {
		t.Errorf("Expected numerator/denominator error, got: %v", err)
	}
}

func TestKPIDefinition_DuplicateIDs(t *testing.T) {
	yamlInput := `
version: "v1"
author: "Test"
tools:
  - name: "my_tool"
    description: "A tool"
    version: "1.0.0"
    kpis:
      - id: "same_id"
        label: "First"
        category: "comm"
        event_type: "msg"
        aggregation: "count"
        value_type: "number"
      - id: "same_id"
        label: "Second"
        category: "comm"
        event_type: "other"
        aggregation: "count"
        value_type: "number"
    functions:
      - name: "DoSomething"
        operation: "web_browse"
        description: "does something"
        successCriteria: "inference"
        triggers:
          - type: "always_on_user_message"
        input:
          - name: "query"
            description: "the query"
            origin: "chat"
            onError:
              strategy: "requestUserInput"
              message: "Please provide input"
        steps:
          - name: "browse"
            action: "open_url"
            with:
              url: "https://example.com"
`
	_, err := CreateTool(yamlInput)
	if err == nil {
		t.Fatal("Expected error for duplicate KPI IDs")
	}
	if !strings.Contains(err.Error(), "duplicate kpi id") {
		t.Errorf("Expected 'duplicate kpi id' error, got: %v", err)
	}
}

func TestKPIDefinition_ToolWithoutKPIsStillWorks(t *testing.T) {
	yamlInput := `
version: "v1"
author: "Test"
tools:
  - name: "no_kpis"
    description: "A tool without KPIs"
    version: "1.0.0"
    functions:
      - name: "DoSomething"
        operation: "web_browse"
        description: "does something"
        successCriteria: "inference"
        triggers:
          - type: "always_on_user_message"
        input:
          - name: "query"
            description: "the query"
            origin: "chat"
            onError:
              strategy: "requestUserInput"
              message: "Please provide input"
        steps:
          - name: "browse"
            action: "open_url"
            with:
              url: "https://example.com"
`
	result, err := CreateTool(yamlInput)
	if err != nil {
		t.Fatalf("Expected no error for tool without KPIs, got: %v", err)
	}
	if len(result.Tools[0].Kpis) != 0 {
		t.Errorf("Expected 0 KPIs, got %d", len(result.Tools[0].Kpis))
	}
}

func TestI18nString_PlainString(t *testing.T) {
	yamlStr := `label: "Hello"`
	type testStruct struct {
		Label I18nString `yaml:"label"`
	}
	var s testStruct
	err := yaml.Unmarshal([]byte(yamlStr), &s)
	if err != nil {
		t.Fatalf("Failed to unmarshal plain string: %v", err)
	}
	if s.Label.En != "Hello" {
		t.Errorf("Expected En='Hello', got '%s'", s.Label.En)
	}
	if s.Label.Pt != "" {
		t.Errorf("Expected Pt='', got '%s'", s.Label.Pt)
	}
}

func TestI18nString_ObjectFormat(t *testing.T) {
	yamlStr := `label:
  en: "Hello"
  pt: "Olá"
  es: "Hola"`
	type testStruct struct {
		Label I18nString `yaml:"label"`
	}
	var s testStruct
	err := yaml.Unmarshal([]byte(yamlStr), &s)
	if err != nil {
		t.Fatalf("Failed to unmarshal object: %v", err)
	}
	if s.Label.En != "Hello" {
		t.Errorf("Expected En='Hello', got '%s'", s.Label.En)
	}
	if s.Label.Pt != "Olá" {
		t.Errorf("Expected Pt='Olá', got '%s'", s.Label.Pt)
	}
	if s.Label.Es != "Hola" {
		t.Errorf("Expected Es='Hola', got '%s'", s.Label.Es)
	}
}

func TestI18nString_Resolve(t *testing.T) {
	s := I18nString{En: "Hello", Pt: "Olá", Es: "Hola"}
	if s.Resolve("en") != "Hello" {
		t.Error("Resolve('en') should return English")
	}
	if s.Resolve("pt") != "Olá" {
		t.Error("Resolve('pt') should return Portuguese")
	}
	if s.Resolve("es") != "Hola" {
		t.Error("Resolve('es') should return Spanish")
	}
	if s.Resolve("fr") != "Hello" {
		t.Error("Resolve('fr') should fall back to English")
	}
	// Test fallback when pt is empty
	s2 := I18nString{En: "Hello"}
	if s2.Resolve("pt") != "Hello" {
		t.Error("Resolve('pt') with empty Pt should fall back to English")
	}
}
