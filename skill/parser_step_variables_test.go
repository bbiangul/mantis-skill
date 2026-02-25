package skill

import (
	"strings"
	"testing"
)

// TestValidateStepVariableReferences_Terminal tests that terminal operations
// SKIP variable validation because shell scripts use $var for both shell variables and YAML inputs
func TestValidateStepVariableReferences_Terminal(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr bool
		errMsg  string
	}{
		{
			name: "terminal step referencing function without input definition should PASS (validation skipped)",
			yaml: `
version: v1
author: test
tools:
  - name: testTool
    description: Test tool
    version: 1.0.0
    functions:
      - name: checkCache
        description: Check cache
        operation: format
        triggers:
          - type: flex_for_user
        input:
          - name: result
            description: Result
            origin: inference
            successCriteria:
              condition: Return 1
            onError:
              strategy: requestN1Support
              message: Failed to get result
        output:
          type: string
          value: $result
      - name: useCache
        description: Use cache
        operation: terminal
        triggers:
          - type: flex_for_user
        needs:
          - name: checkCache
        steps:
          - name: getResult
            action: sh
            with:
              linux: |
                echo "$checkCache"
              windows: |
                echo %checkCache%
        output:
          type: string
          value: result[0]
`,
			wantErr: false, // Terminal operations skip variable validation
		},
		{
			name: "terminal step with explicit input definition should pass",
			yaml: `
version: v1
author: test
tools:
  - name: testTool
    description: Test tool
    version: 1.0.0
    functions:
      - name: checkCache
        description: Check cache
        operation: format
        triggers:
          - type: flex_for_user
        input:
          - name: result
            description: Result
            origin: inference
            successCriteria:
              condition: Return 1
            onError:
              strategy: requestN1Support
              message: Failed to get result
        output:
          type: string
          value: $result
      - name: useCache
        description: Use cache
        operation: terminal
        triggers:
          - type: flex_for_user
        needs:
          - name: checkCache
        input:
          - name: cacheResult
            description: Cache result
            origin: function
            from: checkCache
            onError:
              strategy: requestN1Support
              message: Failed to get cache
        steps:
          - name: getResult
            action: sh
            with:
              linux: |
                echo "$cacheResult"
              windows: |
                echo %cacheResult%
        output:
          type: string
          value: result[0]
`,
			wantErr: false,
		},
		{
			name: "terminal step accessing field from function without input should PASS (validation skipped)",
			yaml: `
version: v1
author: test
tools:
  - name: testTool
    description: Test tool
    version: 1.0.0
    functions:
      - name: getData
        description: Get data
        operation: format
        triggers:
          - type: flex_for_user
        input:
          - name: data
            description: Data
            origin: inference
            successCriteria:
              condition: Return {"field":"value"}
            onError:
              strategy: requestN1Support
              message: Failed to get data
        output:
          type: string
          value: $data
      - name: useData
        description: Use data
        operation: terminal
        triggers:
          - type: flex_for_user
        needs:
          - name: getData
        steps:
          - name: getField
            action: sh
            with:
              linux: |
                echo "$getData.field"
              windows: |
                echo %getData.field%
        output:
          type: string
          value: result[0]
`,
			wantErr: false, // Terminal operations skip variable validation
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := CreateTool(tt.yaml)
			if (err != nil) != tt.wantErr {
				t.Errorf("CreateTool() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil && tt.errMsg != "" {
				if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("CreateTool() error = %v, should contain %v", err.Error(), tt.errMsg)
				}
			}
		})
	}
}

// TestValidateStepVariableReferences_DB tests that DB operations
// require explicit input definitions for function result access
func TestValidateStepVariableReferences_DB(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr bool
		errMsg  string
	}{
		{
			name: "db step referencing function without input definition should fail",
			yaml: `
version: v1
author: test
tools:
  - name: testTool
    description: Test tool
    version: 1.0.0
    functions:
      - name: getUserID
        description: Get user ID
        operation: format
        triggers:
          - type: flex_for_user
        input:
          - name: userId
            description: User ID
            origin: inference
            successCriteria:
              condition: Return 123
            onError:
              strategy: requestN1Support
              message: Failed to get user ID
        output:
          type: string
          value: $userId
      - name: queryUser
        description: Query user
        operation: db
        triggers:
          - type: flex_for_user
        needs:
          - name: getUserID
        with:
          init: |
            CREATE TABLE IF NOT EXISTS users (id INTEGER PRIMARY KEY, name TEXT);
        steps:
          - name: selectUser
            action: select
            with:
              select: SELECT * FROM users WHERE id = $getUserID
        output:
          type: string
          value: result[0]
`,
			wantErr: true,
			errMsg:  "references undefined variable '$getUserID'",
		},
		{
			name: "db step with explicit input definition should pass",
			yaml: `
version: v1
author: test
tools:
  - name: testTool
    description: Test tool
    version: 1.0.0
    functions:
      - name: getUserID
        description: Get user ID
        operation: format
        triggers:
          - type: flex_for_user
        input:
          - name: userId
            description: User ID
            origin: inference
            successCriteria:
              condition: Return 123
            onError:
              strategy: requestN1Support
              message: Failed to get user ID
        output:
          type: string
          value: $userId
      - name: queryUser
        description: Query user
        operation: db
        triggers:
          - type: flex_for_user
        needs:
          - name: getUserID
        input:
          - name: uid
            description: User ID from function
            origin: function
            from: getUserID
            onError:
              strategy: requestN1Support
              message: Failed to get user ID
        with:
          init: |
            CREATE TABLE IF NOT EXISTS users (id INTEGER PRIMARY KEY, name TEXT);
        steps:
          - name: selectUser
            action: select
            with:
              select: SELECT * FROM users WHERE id = $uid
        output:
          type: string
          value: result[0]
`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := CreateTool(tt.yaml)
			if (err != nil) != tt.wantErr {
				t.Errorf("CreateTool() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil && tt.errMsg != "" {
				if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("CreateTool() error = %v, should contain %v", err.Error(), tt.errMsg)
				}
			}
		})
	}
}

// TestValidateStepVariableReferences_API tests that API operations
// require explicit input definitions for function result access
func TestValidateStepVariableReferences_API(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr bool
		errMsg  string
	}{
		{
			name: "api step referencing function in URL without input definition should fail",
			yaml: `
version: v1
author: test
tools:
  - name: testTool
    description: Test tool
    version: 1.0.0
    functions:
      - name: getAPIKey
        description: Get API key
        operation: format
        triggers:
          - type: flex_for_user
        input:
          - name: key
            description: API key
            origin: inference
            successCriteria:
              condition: Return abc123
            onError:
              strategy: requestN1Support
              message: Failed to get API key
        output:
          type: string
          value: $key
      - name: callAPI
        description: Call API
        operation: api_call
        triggers:
          - type: flex_for_user
        needs:
          - name: getAPIKey
        steps:
          - name: makeRequest
            action: GET
            with:
              url: https://api.example.com/data?key=$getAPIKey
`,
			wantErr: true,
			errMsg:  "references undefined variable '$getAPIKey'",
		},
		{
			name: "api step with explicit input definition should pass",
			yaml: `
version: v1
author: test
tools:
  - name: testTool
    description: Test tool
    version: 1.0.0
    functions:
      - name: getAPIKey
        description: Get API key
        operation: format
        triggers:
          - type: flex_for_user
        input:
          - name: key
            description: API key
            origin: inference
            successCriteria:
              condition: Return abc123
            onError:
              strategy: requestN1Support
              message: Failed to get API key
        output:
          type: string
          value: $key
      - name: callAPI
        description: Call API
        operation: api_call
        triggers:
          - type: flex_for_user
        needs:
          - name: getAPIKey
        input:
          - name: apiKey
            description: API key from function
            origin: function
            from: getAPIKey
            onError:
              strategy: requestN1Support
              message: Failed to get API key
        steps:
          - name: makeRequest
            action: GET
            with:
              url: https://api.example.com/data?key=$apiKey
`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := CreateTool(tt.yaml)
			if (err != nil) != tt.wantErr {
				t.Errorf("CreateTool() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil && tt.errMsg != "" {
				if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("CreateTool() error = %v, should contain %v", err.Error(), tt.errMsg)
				}
			}
		})
	}
}

// TestValidateStepVariableReferences_MultipleVariables tests validation with multiple variable references
func TestValidateStepVariableReferences_MultipleVariables(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr bool
		errMsg  string
	}{
		{
			name: "mix of valid inputs and invalid function references should PASS for terminal (validation skipped)",
			yaml: `
version: v1
author: test
tools:
  - name: testTool
    description: Test tool
    version: 1.0.0
    functions:
      - name: getData
        description: Get data
        operation: format
        triggers:
          - type: flex_for_user
        input:
          - name: data
            description: Data
            origin: inference
            successCriteria:
              condition: Return value1
            onError:
              strategy: requestN1Support
              message: Failed to get data
        output:
          type: string
          value: $data
      - name: getOtherData
        description: Get other data
        operation: format
        triggers:
          - type: flex_for_user
        input:
          - name: data
            description: Data
            origin: inference
            successCriteria:
              condition: Return value2
            onError:
              strategy: requestN1Support
              message: Failed to get data
        output:
          type: string
          value: $data
      - name: combineData
        description: Combine data
        operation: terminal
        triggers:
          - type: flex_for_user
        needs:
          - name: getData
          - name: getOtherData
        input:
          - name: data1
            description: First data
            origin: function
            from: getData
            onError:
              strategy: requestN1Support
              message: Failed
        steps:
          - name: combine
            action: sh
            with:
              linux: |
                echo "$data1 and $getOtherData"
              windows: |
                echo %data1% and %getOtherData%
        output:
          type: string
          value: result[0]
`,
			wantErr: false, // Terminal operations skip variable validation
		},
		{
			name: "all variables have input definitions should pass",
			yaml: `
version: v1
author: test
tools:
  - name: testTool
    description: Test tool
    version: 1.0.0
    functions:
      - name: getData
        description: Get data
        operation: format
        triggers:
          - type: flex_for_user
        input:
          - name: data
            description: Data
            origin: inference
            successCriteria:
              condition: Return value1
            onError:
              strategy: requestN1Support
              message: Failed to get data
        output:
          type: string
          value: $data
      - name: getOtherData
        description: Get other data
        operation: format
        triggers:
          - type: flex_for_user
        input:
          - name: data
            description: Data
            origin: inference
            successCriteria:
              condition: Return value2
            onError:
              strategy: requestN1Support
              message: Failed to get data
        output:
          type: string
          value: $data
      - name: combineData
        description: Combine data
        operation: terminal
        triggers:
          - type: flex_for_user
        needs:
          - name: getData
          - name: getOtherData
        input:
          - name: data1
            description: First data
            origin: function
            from: getData
            onError:
              strategy: requestN1Support
              message: Failed
          - name: data2
            description: Second data
            origin: function
            from: getOtherData
            onError:
              strategy: requestN1Support
              message: Failed
        steps:
          - name: combine
            action: sh
            with:
              linux: |
                echo "$data1 and $data2"
              windows: |
                echo %data1% and %data2%
        output:
          type: string
          value: result[0]
`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := CreateTool(tt.yaml)
			if (err != nil) != tt.wantErr {
				t.Errorf("CreateTool() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil && tt.errMsg != "" {
				if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("CreateTool() error = %v, should contain %v", err.Error(), tt.errMsg)
				}
			}
		})
	}
}

// TestEscapedDollarVariableValidation tests that {$$...} escape sequences
// are skipped during variable validation (they produce literal $ in output)
func TestEscapedDollarVariableValidation(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr bool
		errMsg  string
	}{
		{
			name: "GraphQL mutation with {$$file} escape should PASS",
			yaml: `
version: v1
author: test
tools:
  - name: testTool
    description: Test tool
    version: 1.0.0
    functions:
      - name: uploadFile
        description: Upload file to Monday.com
        operation: api_call
        private: true
        input:
          - name: itemId
            description: Item ID
            origin: inference
            successCriteria:
              condition: Return item ID
            onError:
              strategy: requestN1Support
              message: Need item ID
          - name: pdfFile
            description: PDF file
            origin: inference
            successCriteria:
              condition: Return file
            onError:
              strategy: requestN1Support
              message: Need file
        steps:
          - name: upload
            action: POST
            resultIndex: 1
            with:
              url: "https://api.monday.com/v2/file"
              requestBody:
                type: "multipart/form-data"
                with:
                  query: 'mutation ({$$file}: File!) { add_file_to_column(item_id: $itemId, file: {$$file}) { id } }'
                  variables[file]: "$pdfFile"
        output:
          type: string
          value: "done"
      - name: ProcessUpload
        description: Process file upload
        operation: format
        triggers:
          - type: flex_for_user
        input:
          - name: dummy
            description: Dummy input
            origin: inference
            successCriteria:
              condition: Return value
            onError:
              strategy: requestN1Support
              message: Need value
        needs:
          - name: uploadFile
        output:
          type: string
          value: "$dummy"
`,
			wantErr: false,
		},
		{
			name: "Actual undefined variable should FAIL",
			yaml: `
version: v1
author: test
tools:
  - name: testTool
    description: Test tool
    version: 1.0.0
    functions:
      - name: undefinedVar
        description: Has undefined variable
        operation: api_call
        triggers:
          - type: flex_for_user
        input:
          - name: itemId
            description: Item ID
            origin: inference
            successCriteria:
              condition: Return item ID
            onError:
              strategy: requestN1Support
              message: Need item ID
        steps:
          - name: call
            action: POST
            resultIndex: 1
            with:
              url: "https://api.example.com"
              requestBody:
                type: "application/json"
                with:
                  query: 'mutation { op(item_id: $itemId, file: $undefinedFile) }'
        output:
          type: string
          value: "done"
`,
			wantErr: true,
			errMsg:  "references undefined variable '$undefinedFile'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := CreateTool(tt.yaml)
			if (err != nil) != tt.wantErr {
				t.Errorf("CreateTool() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil && tt.errMsg != "" {
				if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("CreateTool() error = %v, should contain %v", err.Error(), tt.errMsg)
				}
			}
		})
	}
}
