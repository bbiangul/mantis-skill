package skill

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"
)

// TestFunctionCallUnmarshalYAML tests that FunctionCall can unmarshal from both
// simple string format and complex object format
func TestFunctionCallUnmarshalYAML(t *testing.T) {
	testCases := []struct {
		name     string
		yamlData string
		expected FunctionCall
	}{
		{
			name:     "Simple string format",
			yamlData: `"sendEmail"`,
			expected: FunctionCall{Name: "sendEmail"},
		},
		{
			name: "Complex object format with params",
			yamlData: `
name: "sendEmail"
params:
  recipient: "$customerEmail"
  subject: "Confirmation"
`,
			expected: FunctionCall{
				Name: "sendEmail",
				Params: map[string]interface{}{
					"recipient": "$customerEmail",
					"subject":   "Confirmation",
				},
			},
		},
		{
			name: "Complex format without params",
			yamlData: `
name: "logToDatabase"
`,
			expected: FunctionCall{Name: "logToDatabase"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var result FunctionCall
			err := yaml.Unmarshal([]byte(tc.yamlData), &result)
			assert.NoError(t, err)
			assert.Equal(t, tc.expected.Name, result.Name)
			if tc.expected.Params != nil {
				assert.Equal(t, len(tc.expected.Params), len(result.Params))
				for k, v := range tc.expected.Params {
					assert.Equal(t, v, result.Params[k])
				}
			}
		})
	}
}

// TestNeedItemWithParams tests that NeedItem can unmarshal with params
func TestNeedItemWithParams(t *testing.T) {
	testCases := []struct {
		name     string
		yamlData string
		expected NeedItem
	}{
		{
			name:     "Simple string format",
			yamlData: `"validateUser"`,
			expected: NeedItem{Name: "validateUser"},
		},
		{
			name: "Complex format with params",
			yamlData: `
name: "fetchPricing"
params:
  serviceType: "$selectedService"
  userId: "$USER.id"
`,
			expected: NeedItem{
				Name: "fetchPricing",
				Params: map[string]interface{}{
					"serviceType": "$selectedService",
					"userId":      "$USER.id",
				},
			},
		},
		{
			name: "askToKnowledgeBase with query",
			yamlData: `
name: "askToKnowledgeBase"
query: "What are the business hours?"
`,
			expected: NeedItem{
				Name:  "askToKnowledgeBase",
				Query: "What are the business hours?",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var result NeedItem
			err := yaml.Unmarshal([]byte(tc.yamlData), &result)
			assert.NoError(t, err)
			assert.Equal(t, tc.expected.Name, result.Name)
			assert.Equal(t, tc.expected.Query, result.Query)
			if tc.expected.Params != nil {
				assert.Equal(t, len(tc.expected.Params), len(result.Params))
			}
		})
	}
}

// TestOnErrorUnmarshalYAML tests OnError unmarshaling with call field
func TestOnErrorUnmarshalYAML(t *testing.T) {
	testCases := []struct {
		name         string
		yamlData     string
		expectedCall *FunctionCall
	}{
		{
			name: "call as string",
			yamlData: `
strategy: "requestUserInput"
message: "Please select"
call: "getOptions"
`,
			expectedCall: &FunctionCall{Name: "getOptions"},
		},
		{
			name: "call as object with params",
			yamlData: `
strategy: "requestUserInput"
message: "Please select a time slot"
call:
  name: "getAvailableSlots"
  params:
    date: "$appointmentDate"
`,
			expectedCall: &FunctionCall{
				Name: "getAvailableSlots",
				Params: map[string]interface{}{
					"date": "$appointmentDate",
				},
			},
		},
		{
			name: "no call field",
			yamlData: `
strategy: "requestUserInput"
message: "Please provide input"
`,
			expectedCall: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var result OnError
			err := yaml.Unmarshal([]byte(tc.yamlData), &result)
			assert.NoError(t, err)
			if tc.expectedCall != nil {
				assert.NotNil(t, result.Call)
				assert.Equal(t, tc.expectedCall.Name, result.Call.Name)
				if tc.expectedCall.Params != nil {
					assert.Equal(t, len(tc.expectedCall.Params), len(result.Call.Params))
				}
			} else {
				assert.Nil(t, result.Call)
			}
		})
	}
}

// TestCheckNeedsParamsFromCallers tests that warnings are generated when callers don't provide required params
func TestCheckNeedsParamsFromCallers(t *testing.T) {
	// Test the checkNeedsParamsFromCallers function directly with Function slices
	// This avoids the full YAML validation complexity

	testCases := []struct {
		name            string
		functions       []Function
		expectWarnings  bool
		warningContains string
	}{
		{
			name: "onSuccess caller provides required param - no warning",
			functions: []Function{
				{
					Name: "parentFunction",
					Input: []Input{
						{Name: "orgId"},
					},
					OnSuccess: []FunctionCall{
						{Name: "childFunction", Params: map[string]interface{}{"orgId": "$orgId"}},
					},
				},
				{
					Name: "childFunction",
					Input: []Input{
						{Name: "orgId"},
					},
					Needs: []NeedItem{
						{Name: "fetchOrg", Params: map[string]interface{}{"orgId": "$orgId"}},
					},
				},
				{
					Name: "fetchOrg",
					Input: []Input{
						{Name: "orgId"},
					},
				},
			},
			expectWarnings:  false,
			warningContains: "",
		},
		{
			name: "onSuccess caller missing required param - warning",
			functions: []Function{
				{
					Name: "parentFunction",
					Input: []Input{
						{Name: "orgId"},
					},
					OnSuccess: []FunctionCall{
						{Name: "childFunction"}, // Missing params
					},
				},
				{
					Name: "childFunction",
					Input: []Input{
						{Name: "orgId"},
					},
					Needs: []NeedItem{
						{Name: "fetchOrg", Params: map[string]interface{}{"orgId": "$orgId"}},
					},
				},
				{
					Name: "fetchOrg",
					Input: []Input{
						{Name: "orgId"},
					},
				},
			},
			expectWarnings:  true,
			warningContains: "doesn't provide param 'orgId'",
		},
		{
			name: "needs caller missing required param - warning",
			functions: []Function{
				{
					Name: "parentFunction",
					Input: []Input{
						{Name: "orgId"},
					},
					Needs: []NeedItem{
						{Name: "childFunction"}, // Missing params
					},
				},
				{
					Name: "childFunction",
					Input: []Input{
						{Name: "orgId"},
					},
					Needs: []NeedItem{
						{Name: "fetchOrg", Params: map[string]interface{}{"orgId": "$orgId"}},
					},
				},
				{
					Name: "fetchOrg",
					Input: []Input{
						{Name: "orgId"},
					},
				},
			},
			expectWarnings:  true,
			warningContains: "doesn't provide param 'orgId'",
		},
		{
			name: "no needs params using own inputs - no warning",
			functions: []Function{
				{
					Name: "parentFunction",
					Input: []Input{
						{Name: "orgId"},
					},
					OnSuccess: []FunctionCall{
						{Name: "childFunction"},
					},
				},
				{
					Name:  "childFunction",
					Input: []Input{},
					Needs: []NeedItem{
						{Name: "fetchOrg"}, // No params, so no requirement
					},
				},
				{
					Name:  "fetchOrg",
					Input: []Input{},
				},
			},
			expectWarnings:  false,
			warningContains: "",
		},
		{
			name: "onFailure caller missing required param - warning",
			functions: []Function{
				{
					Name: "parentFunction",
					Input: []Input{
						{Name: "orgId"},
					},
					OnFailure: []FunctionCall{
						{Name: "errorHandler"}, // Missing params
					},
				},
				{
					Name: "errorHandler",
					Input: []Input{
						{Name: "orgId"},
					},
					Needs: []NeedItem{
						{Name: "logError", Params: map[string]interface{}{"orgId": "$orgId"}},
					},
				},
				{
					Name: "logError",
					Input: []Input{
						{Name: "orgId"},
					},
				},
			},
			expectWarnings:  true,
			warningContains: "doesn't provide param 'orgId'",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			warnings := checkNeedsParamsFromCallers(tc.functions)

			if tc.expectWarnings {
				assert.NotEmpty(t, warnings, "Expected warnings but got none")
				found := false
				for _, w := range warnings {
					if strings.Contains(w, tc.warningContains) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected warning containing '%s', got: %v", tc.warningContains, warnings)
				}
			} else {
				assert.Empty(t, warnings, "Expected no warnings but got: %v", warnings)
			}
		})
	}
}

// TestCheckInputFromWithoutNeeds tests that warnings are generated when input.from references
// a function not in needs
func TestCheckInputFromWithoutNeeds(t *testing.T) {
	testCases := []struct {
		name            string
		functions       []Function
		expectWarnings  bool
		warningContains string
	}{
		{
			name: "input.from with function in needs - no warning",
			functions: []Function{
				{
					Name: "mainFunc",
					Input: []Input{
						{Name: "data", Origin: DataOriginFunction, From: "fetchData"},
					},
					Needs: []NeedItem{
						{Name: "fetchData"},
					},
				},
				{
					Name:  "fetchData",
					Input: []Input{},
				},
			},
			expectWarnings:  false,
			warningContains: "",
		},
		{
			name: "input.from without function in needs - warning",
			functions: []Function{
				{
					Name: "mainFunc",
					Input: []Input{
						{Name: "data", Origin: DataOriginFunction, From: "fetchData"},
					},
					Needs: []NeedItem{}, // fetchData not in needs!
				},
				{
					Name:  "fetchData",
					Input: []Input{},
				},
			},
			expectWarnings:  true,
			warningContains: "not in its needs block",
		},
		{
			name: "input.from with dot notation, function in needs - no warning",
			functions: []Function{
				{
					Name: "mainFunc",
					Input: []Input{
						{Name: "userId", Origin: DataOriginFunction, From: "fetchUser.id"},
					},
					Needs: []NeedItem{
						{Name: "fetchUser"},
					},
				},
				{
					Name:  "fetchUser",
					Input: []Input{},
				},
			},
			expectWarnings:  false,
			warningContains: "",
		},
		{
			name: "input.from with dot notation, function not in needs - warning",
			functions: []Function{
				{
					Name: "mainFunc",
					Input: []Input{
						{Name: "userId", Origin: DataOriginFunction, From: "fetchUser.id"},
					},
					Needs: []NeedItem{}, // fetchUser not in needs!
				},
				{
					Name:  "fetchUser",
					Input: []Input{},
				},
			},
			expectWarnings:  true,
			warningContains: "fetchUser",
		},
		{
			name: "input with chat origin - no warning",
			functions: []Function{
				{
					Name: "mainFunc",
					Input: []Input{
						{Name: "query", Origin: DataOriginChat},
					},
					Needs: []NeedItem{},
				},
			},
			expectWarnings:  false,
			warningContains: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			warnings := checkInputFromWithoutNeeds(tc.functions)

			if tc.expectWarnings {
				assert.NotEmpty(t, warnings, "Expected warnings but got none")
				found := false
				for _, w := range warnings {
					if strings.Contains(w, tc.warningContains) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected warning containing '%s', got: %v", tc.warningContains, warnings)
				}
			} else {
				assert.Empty(t, warnings, "Expected no warnings but got: %v", warnings)
			}
		})
	}
}

// TestBackwardCompatibility tests that existing YAML formats still work
func TestBackwardCompatibility(t *testing.T) {
	testCases := []struct {
		name    string
		yaml    string
		wantErr bool
	}{
		{
			name: "Existing onSuccess format - array of strings",
			yaml: `
version: "v1"
author: "Test"
tools:
  - name: "TestTool"
    description: "Tool for testing"
    version: "1.0.0"
    functions:
      - name: "createBooking"
        operation: "api_call"
        description: "Create a booking"
        input: []
        onSuccess:
          - "sendEmail"
          - "logToDatabase"
        triggers:
          - type: "flex_for_user"
        steps:
          - name: "book"
            action: "POST"
            with:
              url: "https://api.example.com/book"

      - name: "sendEmail"
        operation: "api_call"
        description: "Send email"
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "send"
            action: "POST"
            with:
              url: "https://api.example.com/email"

      - name: "logToDatabase"
        operation: "db"
        description: "Log to database"
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "log"
            action: "write"
            with:
              write: "INSERT INTO logs VALUES ('booked')"
`,
			wantErr: false,
		},
		{
			name: "Existing needs format - array of strings",
			yaml: `
version: "v1"
author: "Test"
tools:
  - name: "TestTool"
    description: "Tool for testing"
    version: "1.0.0"
    functions:
      - name: "processOrder"
        operation: "api_call"
        description: "Process an order"
        input: []
        needs:
          - "validateUser"
          - "checkInventory"
        triggers:
          - type: "flex_for_user"
        steps:
          - name: "process"
            action: "POST"
            with:
              url: "https://api.example.com/process"

      - name: "validateUser"
        operation: "api_call"
        description: "Validate user"
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "validate"
            action: "GET"
            with:
              url: "https://api.example.com/validate"

      - name: "checkInventory"
        operation: "api_call"
        description: "Check inventory"
        triggers:
          - type: "flex_for_user"
        input: []
        steps:
          - name: "check"
            action: "GET"
            with:
              url: "https://api.example.com/inventory"
`,
			wantErr: false,
		},
		{
			name: "Existing askToKnowledgeBase with query in needs",
			yaml: `
version: "v1"
author: "Test"
tools:
  - name: "TestTool"
    description: "Tool for testing"
    version: "1.0.0"
    functions:
      - name: "answerQuestion"
        operation: "api_call"
        description: "Answer a question"
        input: []
        needs:
          - name: "askToKnowledgeBase"
            query: "What are the office hours?"
        triggers:
          - type: "flex_for_user"
        steps:
          - name: "respond"
            action: "GET"
            with:
              url: "https://api.example.com/respond"
`,
			wantErr: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := CreateTool(tc.yaml)
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
