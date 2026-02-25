package skill

import (
	"testing"
)

func TestValidateDeterministicBreakCondition(t *testing.T) {
	testCases := []struct {
		name        string
		condition   string
		itemVar     string
		indexVar    string
		expectError bool
		errorText   string
	}{
		{
			name:        "Valid: simple equality",
			condition:   "$item.status == 'completed'",
			itemVar:     "item",
			indexVar:    "index",
			expectError: false,
		},
		{
			name:        "Invalid: no operator",
			condition:   "$item.status",
			itemVar:     "item",
			indexVar:    "index",
			expectError: true,
			errorText:   "invalid syntax",
		},
		{
			name:        "Invalid: undefined variable",
			condition:   "$someVar == 'done'",
			itemVar:     "item",
			indexVar:    "index",
			expectError: true,
			errorText:   "invalid variable",
		},
		{
			name:        "Valid: index comparison",
			condition:   "$index >= 10",
			itemVar:     "item",
			indexVar:    "index",
			expectError: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateDeterministicBreakCondition(tc.condition, tc.itemVar, tc.indexVar, "testStep", "testFunc", "testTool")

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

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
