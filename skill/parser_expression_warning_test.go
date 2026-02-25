package skill

import (
	"strings"
	"testing"
)

func TestCheckVariablesInsideNestedStrings_InputValue(t *testing.T) {
	tests := []struct {
		name          string
		yaml          string
		wantWarning   bool
		warningSubstr string
	}{
		{
			name: "expression in input value should warn",
			yaml: `
version: v1
author: test
tools:
  - name: test-tool
    description: test tool
    version: 1.0.0
    functions:
      - name: testFunc
        operation: format
        description: test function
        triggers:
          - type: flex_for_user
        input:
          - name: result
            description: test input
            origin: inference
            successCriteria:
              condition: valid JSON
            onError:
              strategy: requestUserInput
              message: Error
          - name: computed
            description: computed value
            value: '{"canProceed": $result.busy.length == 0}'
        output:
          type: object
          fields:
            - name: data
              value: $result
`,
			wantWarning:   true,
			warningSubstr: "contains expression syntax",
		},
		{
			name: "simple variable reference should not warn",
			yaml: `
version: v1
author: test
tools:
  - name: test-tool
    description: test tool
    version: 1.0.0
    functions:
      - name: testFunc
        operation: format
        description: test function
        triggers:
          - type: flex_for_user
        input:
          - name: userId
            description: user id
            origin: inference
            successCriteria:
              condition: valid user id
            onError:
              strategy: requestUserInput
              message: Error
          - name: greeting
            description: greeting message
            value: 'Hello $userId'
        output:
          type: object
          fields:
            - name: message
              value: $greeting
`,
			wantWarning: false,
		},
		{
			name: "comparison operators should warn",
			yaml: `
version: v1
author: test
tools:
  - name: test-tool
    description: test tool
    version: 1.0.0
    functions:
      - name: testFunc
        operation: format
        description: test function
        triggers:
          - type: flex_for_user
        input:
          - name: count
            description: count
            origin: inference
            successCriteria:
              condition: valid number
            onError:
              strategy: requestUserInput
              message: Error
          - name: isValid
            description: validity check
            value: '$count >= 10'
        output:
          type: object
          fields:
            - name: result
              value: $isValid
`,
			wantWarning:   true,
			warningSubstr: ">=",
		},
		{
			name: "logical operators should warn",
			yaml: `
version: v1
author: test
tools:
  - name: test-tool
    description: test tool
    version: 1.0.0
    functions:
      - name: testFunc
        operation: format
        description: test function
        triggers:
          - type: flex_for_user
        input:
          - name: a
            description: a
            origin: inference
            successCriteria:
              condition: valid
            onError:
              strategy: requestUserInput
              message: Error
          - name: b
            description: b
            origin: inference
            successCriteria:
              condition: valid
            onError:
              strategy: requestUserInput
              message: Error
          - name: result
            description: result
            value: '$a && $b'
        output:
          type: object
          fields:
            - name: data
              value: $result
`,
			wantWarning:   true,
			warningSubstr: "&&",
		},
		{
			name: "JS method calls should warn",
			yaml: `
version: v1
author: test
tools:
  - name: test-tool
    description: test tool
    version: 1.0.0
    functions:
      - name: testFunc
        operation: format
        description: test function
        triggers:
          - type: flex_for_user
        input:
          - name: items
            description: items
            origin: inference
            successCriteria:
              condition: valid list
            onError:
              strategy: requestUserInput
              message: Error
          - name: count
            description: count
            value: '$items.length'
        output:
          type: object
          fields:
            - name: result
              value: $count
`,
			wantWarning:   true,
			warningSubstr: ".length",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := CreateToolWithWarnings(tt.yaml)
			if err != nil {
				t.Fatalf("CreateToolWithWarnings() error = %v", err)
			}

			hasRelevantWarning := false
			for _, w := range result.Warnings {
				if strings.Contains(w, "expression syntax") {
					hasRelevantWarning = true
					break
				}
			}

			if hasRelevantWarning != tt.wantWarning {
				t.Errorf("wantWarning = %v, got warnings: %v", tt.wantWarning, result.Warnings)
			}

			if tt.wantWarning && tt.warningSubstr != "" {
				found := false
				for _, w := range result.Warnings {
					if strings.Contains(w, tt.warningSubstr) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected warning containing %q, got: %v", tt.warningSubstr, result.Warnings)
				}
			}
		})
	}
}

func TestCheckVariablesInsideNestedStrings_OutputValue(t *testing.T) {
	yaml := `
version: v1
author: test
tools:
  - name: test-tool
    description: test tool
    version: 1.0.0
    functions:
      - name: testFunc
        operation: format
        description: test function
        triggers:
          - type: flex_for_user
        input:
          - name: result
            description: test input
            origin: inference
            successCriteria:
              condition: valid input
            onError:
              strategy: requestUserInput
              message: Error
        output:
          type: string
          value: '{"valid": $result.status == "active"}'
`
	result, err := CreateToolWithWarnings(yaml)
	if err != nil {
		t.Fatalf("CreateToolWithWarnings() error = %v", err)
	}

	found := false
	for _, w := range result.Warnings {
		if strings.Contains(w, "output.value") && strings.Contains(w, "expression syntax") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected warning about output.value expression, got: %v", result.Warnings)
	}
}

func TestCheckVariablesInsideNestedStrings_StepWith(t *testing.T) {
	// Unit test for step.with expression detection
	functions := []Function{
		{
			Name: "testFunc",
			Steps: []Step{
				{
					Name:   "callApi",
					Action: "POST",
					With: map[string]interface{}{
						"url":  "https://api.example.com",
						"body": `{"isValid": $userId != null}`,
					},
				},
			},
		},
	}

	warnings := checkVariablesInsideNestedStrings(functions)

	found := false
	for _, w := range warnings {
		if strings.Contains(w, "step") && strings.Contains(w, "body") && strings.Contains(w, "expression syntax") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected warning about step.with expression, got: %v", warnings)
	}
}

func TestCheckVariablesInsideNestedStrings_DeterministicFieldShouldNotWarn(t *testing.T) {
	// Unit test to verify deterministic field is skipped
	functions := []Function{
		{
			Name: "testFunc",
			Steps: []Step{
				{
					Name:   "someStep",
					Action: "check",
					With: map[string]interface{}{
						"deterministic": `$status == "active"`, // Should NOT warn
						"otherField":    `$status == "active"`, // Should warn
					},
				},
			},
		},
	}

	warnings := checkVariablesInsideNestedStrings(functions)

	// Should have warning for otherField but NOT for deterministic param
	for _, w := range warnings {
		// Check that the warning is not about the 'deterministic' param (as opposed to mentioning deterministic in the suggestion)
		if strings.Contains(w, "param 'deterministic'") {
			t.Errorf("should not warn about deterministic param, but got: %s", w)
		}
	}

	// Should have warning for otherField
	found := false
	for _, w := range warnings {
		if strings.Contains(w, "otherField") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected warning about otherField, got: %v", warnings)
	}
}

func TestCheckVariablesInsideNestedStrings_OnSuccessParams(t *testing.T) {
	yaml := `
version: v1
author: test
tools:
  - name: test-tool
    description: test tool
    version: 1.0.0
    functions:
      - name: firstFunc
        operation: format
        description: first function
        triggers:
          - type: flex_for_user
        input:
          - name: data
            description: data
            origin: inference
            successCriteria:
              condition: valid data
            onError:
              strategy: requestUserInput
              message: Error
        output:
          type: object
          fields:
            - name: result
              value: $data
        onSuccess:
          - name: secondFunc
            params:
              computed: '$data.length > 0'
      - name: secondFunc
        operation: format
        description: second function
        input:
          - name: computed
            description: computed
            origin: inference
            successCriteria:
              condition: valid
            onError:
              strategy: requestUserInput
              message: Error
        output:
          type: object
          fields:
            - name: result
              value: $computed
`
	result, err := CreateToolWithWarnings(yaml)
	if err != nil {
		t.Fatalf("CreateToolWithWarnings() error = %v", err)
	}

	found := false
	for _, w := range result.Warnings {
		if strings.Contains(w, "onSuccess") && strings.Contains(w, "expression syntax") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected warning about onSuccess params expression, got: %v", result.Warnings)
	}
}

func TestCheckVariablesInsideNestedStrings_NeedsParams(t *testing.T) {
	yaml := `
version: v1
author: test
tools:
  - name: test-tool
    description: test tool
    version: 1.0.0
    functions:
      - name: helper
        operation: format
        description: helper function
        triggers:
          - type: flex_for_user
        input:
          - name: filter
            description: filter
            origin: inference
            successCriteria:
              condition: valid filter
            onError:
              strategy: requestUserInput
              message: Error
        output:
          type: object
          fields:
            - name: result
              value: $filter
      - name: mainFunc
        operation: format
        description: main function
        triggers:
          - type: flex_for_user
        needs:
          - name: helper
            params:
              filter: '$items.length >= 5'
        input:
          - name: items
            description: items
            origin: inference
            successCriteria:
              condition: valid list
            onError:
              strategy: requestUserInput
              message: Error
        output:
          type: object
          fields:
            - name: result
              value: $items
`
	result, err := CreateToolWithWarnings(yaml)
	if err != nil {
		t.Fatalf("CreateToolWithWarnings() error = %v", err)
	}

	found := false
	for _, w := range result.Warnings {
		if strings.Contains(w, "needs") && strings.Contains(w, "expression syntax") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected warning about needs params expression, got: %v", result.Warnings)
	}
}

func TestCheckVariablesInsideNestedStrings_ComplexJSONExpression(t *testing.T) {
	// This test covers the exact example from the user
	// Note: We use $calendarId (lowercase) instead of $GOOGLE_CALENDAR_ID to avoid system variable validation
	yaml := `
version: v1
author: test
tools:
  - name: test-tool
    description: test tool
    version: 1.0.0
    functions:
      - name: checkAvailability
        operation: format
        description: check calendar availability
        triggers:
          - type: flex_for_user
        input:
          - name: calendarResult
            description: calendar result
            origin: inference
            successCriteria:
              condition: valid calendar data
            onError:
              strategy: requestUserInput
              message: Error
          - name: appointmentData
            description: appointment data
            origin: inference
            successCriteria:
              condition: valid appointment
            onError:
              strategy: requestUserInput
              message: Error
          - name: availability
            description: computed availability
            value: '{"canProceed": result[1].calendars["$calendarId"].busy.length == 0, "requestedTime": "$appointmentData.startDateTimeRFC3339"}'
        output:
          type: object
          fields:
            - name: result
              value: $availability
`
	result, err := CreateToolWithWarnings(yaml)
	if err != nil {
		t.Fatalf("CreateToolWithWarnings() error = %v", err)
	}

	found := false
	for _, w := range result.Warnings {
		if strings.Contains(w, "availability") && strings.Contains(w, "expression syntax") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected warning about complex expression, got: %v", result.Warnings)
	}
}

func TestTruncateForWarning(t *testing.T) {
	tests := []struct {
		input  string
		maxLen int
		want   string
	}{
		{"short", 10, "short"},
		{"exactly10!", 10, "exactly10!"},
		{"this is a longer string", 10, "this is..."},
		{"abc", 3, "abc"},
		{"abcd", 3, "..."},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := truncateForWarning(tt.input, tt.maxLen)
			if got != tt.want {
				t.Errorf("truncateForWarning(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.want)
			}
		})
	}
}
