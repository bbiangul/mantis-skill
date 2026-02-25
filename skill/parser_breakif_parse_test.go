package skill

import (
	"testing"
)

func TestParseBreakIf(t *testing.T) {
	testCases := []struct {
		name        string
		breakIf     interface{}
		expectStr   string
		expectObj   bool
		expectError bool
	}{
		{
			name:        "String condition",
			breakIf:     "$item.status == 'completed'",
			expectStr:   "$item.status == 'completed'",
			expectObj:   false,
			expectError: false,
		},
		{
			name:        "Nil",
			breakIf:     nil,
			expectStr:   "",
			expectObj:   false,
			expectError: false,
		},
		{
			name: "Object condition",
			breakIf: map[string]interface{}{
				"condition": "Check if match",
				"from":      []string{"func1"},
			},
			expectStr:   "",
			expectObj:   true,
			expectError: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			strCond, objCond, err := ParseBreakIf(tc.breakIf)

			if tc.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %s", err.Error())
				}

				if strCond != tc.expectStr {
					t.Errorf("Expected string '%s', got '%s'", tc.expectStr, strCond)
				}

				if tc.expectObj && objCond == nil {
					t.Errorf("Expected object but got nil")
				} else if !tc.expectObj && objCond != nil {
					t.Errorf("Expected nil object but got %v", objCond)
				}
			}
		})
	}
}
