package skill

import (
	"testing"
)

func TestParseOnError_WithCall(t *testing.T) {
	tests := []struct {
		name      string
		input     map[interface{}]interface{}
		expected  *OnError
		expectErr bool
	}{
		{
			name: "call only",
			input: map[interface{}]interface{}{
				"call": "getAvailableOptions",
			},
			expected: &OnError{
				Call: &FunctionCall{Name: "getAvailableOptions"},
			},
			expectErr: false,
		},
		{
			name: "strategy only",
			input: map[interface{}]interface{}{
				"strategy": "requestUserInput",
				"message":  "Please provide input",
			},
			expected: &OnError{
				Strategy: "requestUserInput",
				Message:  "Please provide input",
			},
			expectErr: false,
		},
		{
			name: "call and strategy together",
			input: map[interface{}]interface{}{
				"call":     "getAvailableOptions",
				"strategy": "requestUserInput",
				"message":  "Please select an option",
			},
			expected: &OnError{
				Call:     &FunctionCall{Name: "getAvailableOptions"},
				Strategy: "requestUserInput",
				Message:  "Please select an option",
			},
			expectErr: false,
		},
		{
			name: "call with strategy and with options",
			input: map[interface{}]interface{}{
				"call":     "listServices",
				"strategy": "requestUserInput",
				"message":  "Select a service",
				"with": map[interface{}]interface{}{
					"oneOf": "$availableServices",
				},
			},
			expected: &OnError{
				Call:     &FunctionCall{Name: "listServices"},
				Strategy: "requestUserInput",
				Message:  "Select a service",
				With: &InputWithOptions{
					OneOf: "$availableServices",
				},
			},
			expectErr: false,
		},
		{
			name:      "neither call nor strategy",
			input:     map[interface{}]interface{}{},
			expected:  nil,
			expectErr: true,
		},
		{
			name: "invalid call type",
			input: map[interface{}]interface{}{
				"call": 123,
			},
			expected:  nil,
			expectErr: true,
		},
		{
			name: "call with params",
			input: map[interface{}]interface{}{
				"call": map[interface{}]interface{}{
					"name": "getAvailableSlots",
					"params": map[interface{}]interface{}{
						"date": "$appointmentDate",
					},
				},
				"strategy": "requestUserInput",
				"message":  "Please select a time slot",
			},
			expected: &OnError{
				Call: &FunctionCall{
					Name: "getAvailableSlots",
					Params: map[string]interface{}{
						"date": "$appointmentDate",
					},
				},
				Strategy: "requestUserInput",
				Message:  "Please select a time slot",
			},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseOnError(tt.input)

			if tt.expectErr {
				if err == nil {
					t.Errorf("expected error but got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if result == nil {
				t.Errorf("expected result but got nil")
				return
			}

			// Compare Call field
			if tt.expected.Call != nil {
				if result.Call == nil {
					t.Errorf("expected Call but got nil")
					return
				}
				if result.Call.Name != tt.expected.Call.Name {
					t.Errorf("Call.Name: expected %q, got %q", tt.expected.Call.Name, result.Call.Name)
				}
			} else {
				if result.Call != nil {
					t.Errorf("expected nil Call but got %v", result.Call)
				}
			}

			if result.Strategy != tt.expected.Strategy {
				t.Errorf("Strategy: expected %q, got %q", tt.expected.Strategy, result.Strategy)
			}

			if result.Message != tt.expected.Message {
				t.Errorf("Message: expected %q, got %q", tt.expected.Message, result.Message)
			}

			if tt.expected.With != nil {
				if result.With == nil {
					t.Errorf("expected With options but got nil")
					return
				}
				if result.With.OneOf != tt.expected.With.OneOf {
					t.Errorf("With.OneOf: expected %q, got %q", tt.expected.With.OneOf, result.With.OneOf)
				}
			}
		})
	}
}

func TestParseOnErrorFromStringMap_WithCall(t *testing.T) {
	tests := []struct {
		name      string
		input     map[string]interface{}
		expected  *OnError
		expectErr bool
	}{
		{
			name: "call only",
			input: map[string]interface{}{
				"call": "getAvailableOptions",
			},
			expected: &OnError{
				Call: &FunctionCall{Name: "getAvailableOptions"},
			},
			expectErr: false,
		},
		{
			name: "call and strategy together",
			input: map[string]interface{}{
				"call":     "listProducts",
				"strategy": "requestN1Support",
				"message":  "Unable to find product",
			},
			expected: &OnError{
				Call:     &FunctionCall{Name: "listProducts"},
				Strategy: "requestN1Support",
				Message:  "Unable to find product",
			},
			expectErr: false,
		},
		{
			name:      "neither call nor strategy",
			input:     map[string]interface{}{},
			expected:  nil,
			expectErr: true,
		},
		{
			name: "call with params using map",
			input: map[string]interface{}{
				"call": map[string]interface{}{
					"name": "fetchDetails",
					"params": map[string]interface{}{
						"id": "$selectedId",
					},
				},
				"strategy": "requestUserInput",
				"message":  "Please confirm",
			},
			expected: &OnError{
				Call: &FunctionCall{
					Name: "fetchDetails",
					Params: map[string]interface{}{
						"id": "$selectedId",
					},
				},
				Strategy: "requestUserInput",
				Message:  "Please confirm",
			},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseOnErrorFromStringMap(tt.input)

			if tt.expectErr {
				if err == nil {
					t.Errorf("expected error but got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if result == nil {
				t.Errorf("expected result but got nil")
				return
			}

			// Compare Call field
			if tt.expected.Call != nil {
				if result.Call == nil {
					t.Errorf("expected Call but got nil")
					return
				}
				if result.Call.Name != tt.expected.Call.Name {
					t.Errorf("Call.Name: expected %q, got %q", tt.expected.Call.Name, result.Call.Name)
				}
			} else {
				if result.Call != nil {
					t.Errorf("expected nil Call but got %v", result.Call)
				}
			}

			if result.Strategy != tt.expected.Strategy {
				t.Errorf("Strategy: expected %q, got %q", tt.expected.Strategy, result.Strategy)
			}

			if result.Message != tt.expected.Message {
				t.Errorf("Message: expected %q, got %q", tt.expected.Message, result.Message)
			}
		})
	}
}
