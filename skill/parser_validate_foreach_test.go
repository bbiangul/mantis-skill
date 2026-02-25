package skill

import (
	"testing"
)

func TestValidateForEachDirect(t *testing.T) {
	testCases := []struct {
		name        string
		forEach     *ForEach
		expectError bool
		errorText   string
	}{
		{
			name: "Valid: with breakIf",
			forEach: &ForEach{
				Items:    "$items",
				ItemVar:  "item",
				IndexVar: "index",
				BreakIf:  "$item.status == 'completed'",
			},
			expectError: false,
		},
		{
			name: "Invalid: breakIf without operator",
			forEach: &ForEach{
				Items:    "$items",
				ItemVar:  "item",
				IndexVar: "index",
				BreakIf:  "$item.status",
			},
			expectError: true,
			errorText:   "invalid syntax",
		},
		{
			name: "Invalid: breakIf with undefined variable",
			forEach: &ForEach{
				Items:    "$items",
				ItemVar:  "item",
				IndexVar: "idx",
				BreakIf:  "$someVar == 'done'",
			},
			expectError: true,
			errorText:   "invalid variable",
		},
		{
			name: "Invalid: inference breakIf with empty condition",
			forEach: &ForEach{
				Items:    "$items",
				ItemVar:  "item",
				IndexVar: "index",
				BreakIf: &SuccessCriteriaObject{
					Condition: "",
					From:      []string{},
				},
			},
			expectError: true,
			errorText:   "empty condition",
		},
		// WaitFor test cases
		{
			name: "Valid: waitFor with all fields",
			forEach: &ForEach{
				Items:    "$items",
				ItemVar:  "item",
				IndexVar: "index",
				WaitFor: &WaitFor{
					Name:                "checkReady",
					PollIntervalSeconds: 5,
					MaxWaitingSeconds:   60,
					Params: map[string]interface{}{
						"customParam": "$item.id",
					},
				},
			},
			expectError: false,
		},
		{
			name: "Valid: waitFor with only required fields (defaults applied)",
			forEach: &ForEach{
				Items:    "$items",
				ItemVar:  "item",
				IndexVar: "index",
				WaitFor: &WaitFor{
					Name: "checkReady",
				},
			},
			expectError: false,
		},
		{
			name: "Valid: waitFor combined with breakIf",
			forEach: &ForEach{
				Items:    "$items",
				ItemVar:  "item",
				IndexVar: "index",
				WaitFor: &WaitFor{
					Name:                "checkReady",
					PollIntervalSeconds: 5,
					MaxWaitingSeconds:   60,
				},
				BreakIf: "$item.status == 'completed'",
			},
			expectError: false,
		},
		{
			name: "Invalid: waitFor with empty name",
			forEach: &ForEach{
				Items:    "$items",
				ItemVar:  "item",
				IndexVar: "index",
				WaitFor: &WaitFor{
					Name:                "",
					PollIntervalSeconds: 5,
					MaxWaitingSeconds:   60,
				},
			},
			expectError: true,
			errorText:   "empty name",
		},
		{
			name: "Invalid: waitFor with maxWaitingSeconds <= pollIntervalSeconds",
			forEach: &ForEach{
				Items:    "$items",
				ItemVar:  "item",
				IndexVar: "index",
				WaitFor: &WaitFor{
					Name:                "checkReady",
					PollIntervalSeconds: 60,
					MaxWaitingSeconds:   60,
				},
			},
			expectError: true,
			errorText:   "maxWaitingSeconds",
		},
		{
			name: "Invalid: waitFor with maxWaitingSeconds < pollIntervalSeconds",
			forEach: &ForEach{
				Items:    "$items",
				ItemVar:  "item",
				IndexVar: "index",
				WaitFor: &WaitFor{
					Name:                "checkReady",
					PollIntervalSeconds: 30,
					MaxWaitingSeconds:   10,
				},
			},
			expectError: true,
			errorText:   "maxWaitingSeconds",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateForEach(tc.forEach, "testStep", "testFunc", "testTool")

			if tc.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				} else if tc.errorText != "" && !contains(err.Error(), tc.errorText) {
					t.Errorf("Expected error containing '%s', got: %s", tc.errorText, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %s", err.Error())
				}
			}
		})
	}
}
