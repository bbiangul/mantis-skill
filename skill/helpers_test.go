package skill

import (
	"testing"

	"gopkg.in/yaml.v2"
)

func TestParseRunOnlyIf(t *testing.T) {
	tests := []struct {
		name      string
		input     interface{}
		expected  *RunOnlyIfObject
		expectErr bool
	}{
		{
			name:     "nil input",
			input:    nil,
			expected: nil,
		},
		{
			name:  "simple string",
			input: "user has admin privileges",
			expected: &RunOnlyIfObject{
				Condition: "user has admin privileges",
			},
		},
		{
			name:     "empty string",
			input:    "",
			expected: nil,
		},
		{
			name: "object with condition only",
			input: map[string]interface{}{
				"condition": "check user permissions",
			},
			expected: &RunOnlyIfObject{
				Condition: "check user permissions",
			},
		},
		{
			name: "object with condition and from",
			input: map[string]interface{}{
				"condition": "check user permissions",
				"from":      []string{"getUserRole", "checkPermissions"},
			},
			expected: &RunOnlyIfObject{
				Condition: "check user permissions",
				From:      []string{"getUserRole", "checkPermissions"},
			},
		},
		{
			name: "object with onError",
			input: map[string]interface{}{
				"condition": "check auth status",
				"onError": map[interface{}]interface{}{
					"strategy": "requestUserInput",
					"message":  "Cannot verify auth",
				},
			},
			expected: &RunOnlyIfObject{
				Condition: "check auth status",
				OnError: &OnError{
					Strategy: "requestUserInput",
					Message:  "Cannot verify auth",
				},
			},
		},
		{
			name: "object without condition",
			input: map[string]interface{}{
				"from": []string{"func1"},
			},
			expectErr: true,
		},
		{
			name: "already parsed object",
			input: &RunOnlyIfObject{
				Condition: "already parsed",
			},
			expected: &RunOnlyIfObject{
				Condition: "already parsed",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseRunOnlyIf(tt.input)

			if tt.expectErr {
				if err == nil {
					t.Errorf("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if tt.expected == nil {
				if result != nil {
					t.Errorf("expected nil but got %v", result)
				}
				return
			}

			if result == nil {
				t.Errorf("expected non-nil but got nil")
				return
			}

			if result.Condition != tt.expected.Condition {
				t.Errorf("condition mismatch: expected %s, got %s", tt.expected.Condition, result.Condition)
			}

			if len(result.From) != len(tt.expected.From) {
				t.Errorf("from length mismatch: expected %d, got %d", len(tt.expected.From), len(result.From))
			} else {
				for i, v := range tt.expected.From {
					if result.From[i] != v {
						t.Errorf("from[%d] mismatch: expected %s, got %s", i, v, result.From[i])
					}
				}
			}

			if tt.expected.OnError != nil {
				if result.OnError == nil {
					t.Errorf("expected OnError but got nil")
				} else {
					if result.OnError.Strategy != tt.expected.OnError.Strategy {
						t.Errorf("OnError.Strategy mismatch: expected %s, got %s",
							tt.expected.OnError.Strategy, result.OnError.Strategy)
					}
					if result.OnError.Message != tt.expected.OnError.Message {
						t.Errorf("OnError.Message mismatch: expected %s, got %s",
							tt.expected.OnError.Message, result.OnError.Message)
					}
				}
			}
		})
	}
}

func TestParseSuccessCriteria(t *testing.T) {
	tests := []struct {
		name      string
		input     interface{}
		expected  *SuccessCriteriaObject
		expectErr bool
	}{
		{
			name:     "nil input",
			input:    nil,
			expected: nil,
		},
		{
			name:  "simple string",
			input: "extract user email",
			expected: &SuccessCriteriaObject{
				Condition: "extract user email",
			},
		},
		{
			name:     "empty string",
			input:    "",
			expected: nil,
		},
		{
			name: "object with condition only",
			input: map[string]interface{}{
				"condition": "extract user information",
			},
			expected: &SuccessCriteriaObject{
				Condition: "extract user information",
			},
		},
		{
			name: "object with condition and from",
			input: map[string]interface{}{
				"condition": "extract user information",
				"from":      []string{"getUserData", "enrichUserInfo"},
			},
			expected: &SuccessCriteriaObject{
				Condition: "extract user information",
				From:      []string{"getUserData", "enrichUserInfo"},
			},
		},
		{
			name: "object without condition",
			input: map[string]interface{}{
				"from": []string{"func1"},
			},
			expectErr: true,
		},
		{
			name: "already parsed object",
			input: &SuccessCriteriaObject{
				Condition: "already parsed",
			},
			expected: &SuccessCriteriaObject{
				Condition: "already parsed",
			},
		},
		{
			name: "object with allowedSystemFunctions",
			input: map[string]interface{}{
				"condition":              "extract user information",
				"allowedSystemFunctions": []string{"askToTheConversationHistoryWithCustomer"},
			},
			expected: &SuccessCriteriaObject{
				Condition:              "extract user information",
				AllowedSystemFunctions: []string{"askToTheConversationHistoryWithCustomer"},
			},
		},
		{
			name: "object with multiple allowedSystemFunctions",
			input: map[string]interface{}{
				"condition": "check context",
				"allowedSystemFunctions": []string{
					"askToTheConversationHistoryWithCustomer",
					"askToContext",
					"queryMemories",
				},
			},
			expected: &SuccessCriteriaObject{
				Condition: "check context",
				AllowedSystemFunctions: []string{
					"askToTheConversationHistoryWithCustomer",
					"askToContext",
					"queryMemories",
				},
			},
		},
		{
			name: "object with from and allowedSystemFunctions",
			input: map[string]interface{}{
				"condition":              "check parent results",
				"from":                   []string{"parent"},
				"allowedSystemFunctions": []string{"askToContext"},
			},
			expected: &SuccessCriteriaObject{
				Condition:              "check parent results",
				From:                   []string{"parent"},
				AllowedSystemFunctions: []string{"askToContext"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseSuccessCriteria(tt.input)

			if tt.expectErr {
				if err == nil {
					t.Errorf("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if tt.expected == nil {
				if result != nil {
					t.Errorf("expected nil but got %v", result)
				}
				return
			}

			if result == nil {
				t.Errorf("expected non-nil but got nil")
				return
			}

			if result.Condition != tt.expected.Condition {
				t.Errorf("condition mismatch: expected %s, got %s", tt.expected.Condition, result.Condition)
			}

			if len(result.From) != len(tt.expected.From) {
				t.Errorf("from length mismatch: expected %d, got %d", len(tt.expected.From), len(result.From))
			} else {
				for i, v := range tt.expected.From {
					if result.From[i] != v {
						t.Errorf("from[%d] mismatch: expected %s, got %s", i, v, result.From[i])
					}
				}
			}

			if len(result.AllowedSystemFunctions) != len(tt.expected.AllowedSystemFunctions) {
				t.Errorf("allowedSystemFunctions length mismatch: expected %d, got %d", len(tt.expected.AllowedSystemFunctions), len(result.AllowedSystemFunctions))
			} else {
				for i, v := range tt.expected.AllowedSystemFunctions {
					if result.AllowedSystemFunctions[i] != v {
						t.Errorf("allowedSystemFunctions[%d] mismatch: expected %s, got %s", i, v, result.AllowedSystemFunctions[i])
					}
				}
			}
		})
	}
}

func TestGetRunOnlyIfCondition(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected string
	}{
		{
			name:     "nil input",
			input:    nil,
			expected: "",
		},
		{
			name:     "simple string",
			input:    "test condition",
			expected: "test condition",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name: "RunOnlyIfObject",
			input: &RunOnlyIfObject{
				Condition: "object condition",
			},
			expected: "object condition",
		},
		{
			name: "map with condition",
			input: map[string]interface{}{
				"condition": "map condition",
			},
			expected: "map condition",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetRunOnlyIfCondition(tt.input)
			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestGetSuccessCriteriaCondition(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected string
	}{
		{
			name:     "nil input",
			input:    nil,
			expected: "",
		},
		{
			name:     "simple string",
			input:    "test criteria",
			expected: "test criteria",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name: "SuccessCriteriaObject",
			input: &SuccessCriteriaObject{
				Condition: "object criteria",
			},
			expected: "object criteria",
		},
		{
			name: "map with condition",
			input: map[string]interface{}{
				"condition": "map criteria",
			},
			expected: "map criteria",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetSuccessCriteriaCondition(tt.input)
			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestParseSuccessCriteria_FromYAML(t *testing.T) {
	tests := []struct {
		name                           string
		yamlInput                      string
		expectedConditionPrefix        string
		expectedAllowedSystemFunctions []string
	}{
		{
			name: "YAML with allowedSystemFunctions array notation",
			yamlInput: `
successCriteria:
  allowedSystemFunctions: [ "askToTheConversationHistoryWithCustomer" ]
  condition: |
    Generate a greeting message
`,
			expectedConditionPrefix:        "Generate a greeting message",
			expectedAllowedSystemFunctions: []string{"askToTheConversationHistoryWithCustomer"},
		},
		{
			name: "YAML with allowedSystemFunctions list notation",
			yamlInput: `
successCriteria:
  allowedSystemFunctions:
    - "askToTheConversationHistoryWithCustomer"
    - "askToContext"
  condition: "Check context"
`,
			expectedConditionPrefix:        "Check context",
			expectedAllowedSystemFunctions: []string{"askToTheConversationHistoryWithCustomer", "askToContext"},
		},
		{
			name: "Full input object from YAML",
			yamlInput: `
name: "greetingMessage"
description: "The greeting message to send"
origin: "inference"
successCriteria:
  allowedSystemFunctions: [ "askToTheConversationHistoryWithCustomer" ]
  condition: |
    Generate a warm, friendly, contextual greeting message
`,
			expectedConditionPrefix:        "Generate a warm, friendly, contextual greeting message",
			expectedAllowedSystemFunctions: []string{"askToTheConversationHistoryWithCustomer"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var data struct {
				SuccessCriteria interface{} `yaml:"successCriteria"`
			}

			// If testing full input object
			if len(tt.yamlInput) > 0 && tt.yamlInput[0:5] != "succe" {
				var input Input
				err := yaml.Unmarshal([]byte(tt.yamlInput), &input)
				if err != nil {
					t.Fatalf("Failed to unmarshal YAML: %v", err)
				}
				data.SuccessCriteria = input.SuccessCriteria
				t.Logf("Unmarshaled full Input, SuccessCriteria type: %T", input.SuccessCriteria)
			} else {
				err := yaml.Unmarshal([]byte(tt.yamlInput), &data)
				if err != nil {
					t.Fatalf("Failed to unmarshal YAML: %v", err)
				}
				t.Logf("Unmarshaled successCriteria type: %T", data.SuccessCriteria)
			}

			result, err := ParseSuccessCriteria(data.SuccessCriteria)
			if err != nil {
				t.Fatalf("Failed to parse successCriteria: %v", err)
			}

			if result == nil {
				t.Fatal("Expected non-nil result")
			}

			// Check condition prefix
			if len(result.Condition) < len(tt.expectedConditionPrefix) ||
				result.Condition[:len(tt.expectedConditionPrefix)] != tt.expectedConditionPrefix {
				t.Errorf("Expected condition to start with %q, got %q", tt.expectedConditionPrefix, result.Condition)
			}

			// Check allowedSystemFunctions
			if len(result.AllowedSystemFunctions) != len(tt.expectedAllowedSystemFunctions) {
				t.Errorf("Expected %d allowedSystemFunctions, got %d: %v",
					len(tt.expectedAllowedSystemFunctions),
					len(result.AllowedSystemFunctions),
					result.AllowedSystemFunctions)
			}

			for i, expected := range tt.expectedAllowedSystemFunctions {
				if i >= len(result.AllowedSystemFunctions) {
					t.Errorf("Missing allowedSystemFunction at index %d: %s", i, expected)
				} else if result.AllowedSystemFunctions[i] != expected {
					t.Errorf("Expected allowedSystemFunction[%d] = %q, got %q", i, expected, result.AllowedSystemFunctions[i])
				}
			}

			t.Logf("Successfully parsed: allowedSystemFunctions=%v", result.AllowedSystemFunctions)
		})
	}
}

func TestParseSuccessCriteria_ExactGreetingMessageYAML(t *testing.T) {
	// This is the EXACT YAML from tools/sdr.yaml greetingMessage input
	yamlInput := `name: "greetingMessage"
description: "The greeting message to send"
origin: "inference"
shouldBeHandledAsMessageToUser: true
successCriteria:
  allowedSystemFunctions: [ "askToTheConversationHistoryWithCustomer" ]
  condition: |
    Generate a warm, friendly, contextual greeting message for $userName.

    TIME-BASED GREETING:
    Based on $timeOfDay (hour in 24h format): if 0-11 say 'Good morning', if 12-17 say 'Good afternoon', if 18-23 say 'Good evening'.

    CONTEXT-AWARE WELCOME:
    - Check if there's any conversation history or previous interactions in the context
    - If this is a RETURNING user (conversation history exists):
      * Say "Welcome back!" or "Great to see you again!"
      * Reference what you talked about before (e.g., "I remember we discussed...", "Last time we talked about...")
      * If they joined waitlist: acknowledge it ("Thanks for joining our waitlist!")
      * If they scheduled appointment: mention it ("Looking forward to our call on...")
      * If you explained Jesss before: ask if they have more questions or want to dive deeper into specific topics
      * Keep it natural and conversational, not robotic

    - If this is a FIRST-TIME user (no conversation history):
      * Say "I'm Jesss!" and briefly introduce yourself as a digital employee
      * Explain you can help them understand how you work and if you're a good fit for their team
      * Keep it welcoming and not overwhelming

    TONE: Warm, personal, and authentic. Make returning users feel remembered and valued. Make new users feel welcomed.
onError:
  strategy: "inference"
  message: "Failed to generate greeting"`

	var input Input
	err := yaml.Unmarshal([]byte(yamlInput), &input)
	if err != nil {
		t.Fatalf("Failed to unmarshal YAML: %v", err)
	}

	t.Logf("Unmarshaled Input.SuccessCriteria type: %T", input.SuccessCriteria)
	t.Logf("Unmarshaled Input.SuccessCriteria value: %+v", input.SuccessCriteria)

	result, err := ParseSuccessCriteria(input.SuccessCriteria)
	if err != nil {
		t.Fatalf("Failed to parse successCriteria: %v", err)
	}

	if result == nil {
		t.Fatal("Expected non-nil result")
	}

	t.Logf("Parsed result: Condition length=%d, AllowedSystemFunctions=%v",
		len(result.Condition), result.AllowedSystemFunctions)

	// Check allowedSystemFunctions
	expectedFunctions := []string{"askToTheConversationHistoryWithCustomer"}
	if len(result.AllowedSystemFunctions) != len(expectedFunctions) {
		t.Errorf("Expected %d allowedSystemFunctions, got %d: %v",
			len(expectedFunctions),
			len(result.AllowedSystemFunctions),
			result.AllowedSystemFunctions)
	} else {
		for i, expected := range expectedFunctions {
			if result.AllowedSystemFunctions[i] != expected {
				t.Errorf("Expected allowedSystemFunction[%d] = %q, got %q",
					i, expected, result.AllowedSystemFunctions[i])
			}
		}
	}

	// Check condition exists
	if len(result.Condition) == 0 {
		t.Error("Expected non-empty condition")
	}

	if len(result.AllowedSystemFunctions) > 0 {
		t.Logf("✓ SUCCESS: Parsed allowedSystemFunctions correctly: %v", result.AllowedSystemFunctions)
	} else {
		t.Error("✗ FAIL: allowedSystemFunctions is empty!")
	}
}
