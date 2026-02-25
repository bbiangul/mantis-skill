package skill

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseReRunIf(t *testing.T) {
	tests := []struct {
		name                  string
		reRunIf               interface{}
		expectError           bool
		expectedDeterministic string
		expectedCondition     string
		expectedMaxRetries    int
		expectedDelayMs       int
		expectedScope         string
		expectedCombineMode   string
		expectedCallName      string
	}{
		{
			name:        "Nil reRunIf - no config",
			reRunIf:     nil,
			expectError: false,
		},
		{
			name:                  "Simple string - deterministic shorthand",
			reRunIf:               "$result.status != 'complete'",
			expectError:           false,
			expectedDeterministic: "$result.status != 'complete'",
			expectedMaxRetries:    ReRunIfDefaultMaxRetries,
		},
		{
			name:    "Empty string - no config",
			reRunIf: "",
		},
		{
			name: "Object with deterministic only",
			reRunIf: map[string]interface{}{
				"deterministic": "$result.count == 0",
			},
			expectError:           false,
			expectedDeterministic: "$result.count == 0",
		},
		{
			name: "Object with all fields",
			reRunIf: map[string]interface{}{
				"deterministic": "$result.status == 'pending'",
				"maxRetries":    10,
				"delayMs":       1000,
				"scope":         "steps",
				"combineWith":   "and",
			},
			expectError:           false,
			expectedDeterministic: "$result.status == 'pending'",
			expectedMaxRetries:    10,
			expectedDelayMs:       1000,
			expectedScope:         "steps",
			expectedCombineMode:   "and",
		},
		{
			name: "Object with scope=full",
			reRunIf: map[string]interface{}{
				"deterministic": "$result.needsMoreData == true",
				"scope":         "full",
				"maxRetries":    5,
			},
			expectError:           false,
			expectedDeterministic: "$result.needsMoreData == true",
			expectedScope:         "full",
			expectedMaxRetries:    5,
		},
		{
			name: "Object with inference condition",
			reRunIf: map[string]interface{}{
				"condition":  "the result indicates more information is needed",
				"maxRetries": 5,
			},
			expectError:        false,
			expectedCondition:  "the result indicates more information is needed",
			expectedMaxRetries: 5,
		},
		{
			name: "Object with function call",
			reRunIf: map[string]interface{}{
				"call": map[string]interface{}{
					"name": "validateResult",
					"params": map[string]interface{}{
						"output": "$result",
					},
				},
				"maxRetries": 100,
				"delayMs":    5000,
			},
			expectError:        false,
			expectedCallName:   "validateResult",
			expectedMaxRetries: 100,
			expectedDelayMs:    5000,
		},
		{
			name: "Object with advanced inference",
			reRunIf: map[string]interface{}{
				"inference": map[string]interface{}{
					"condition": "the job is still processing",
					"from":      []interface{}{"checkStatus"},
				},
				"maxRetries": 50,
			},
			expectError:        false,
			expectedMaxRetries: 50,
		},
		{
			name: "Hybrid mode with deterministic and inference",
			reRunIf: map[string]interface{}{
				"deterministic": "$result.status == 'pending'",
				"condition":     "the response indicates processing",
				"combineWith":   "or",
				"maxRetries":    30,
				"delayMs":       2000,
			},
			expectError:           false,
			expectedDeterministic: "$result.status == 'pending'",
			expectedCondition:     "the response indicates processing",
			expectedCombineMode:   "or",
			expectedMaxRetries:    30,
			expectedDelayMs:       2000,
		},
		{
			name: "Float64 values from YAML parsing",
			reRunIf: map[string]interface{}{
				"deterministic": "$result.done == false",
				"maxRetries":    float64(20),
				"delayMs":       float64(500),
			},
			expectError:           false,
			expectedDeterministic: "$result.done == false",
			expectedMaxRetries:    20,
			expectedDelayMs:       500,
		},
		{
			name:        "Invalid type",
			reRunIf:     123,
			expectError: true,
		},
		{
			name: "Object with params - simple",
			reRunIf: map[string]interface{}{
				"deterministic": "$result.hasMore == true",
				"params": map[string]interface{}{
					"pageToken": "$result.nextPageToken",
				},
				"maxRetries": 100,
			},
			expectError:           false,
			expectedDeterministic: "$result.hasMore == true",
			expectedMaxRetries:    100,
		},
		{
			name: "Object with params - nested",
			reRunIf: map[string]interface{}{
				"deterministic": "$result.hasMore == true",
				"params": map[string]interface{}{
					"offset": "$offset + 100",
					"nested": map[string]interface{}{
						"token": "$result.auth.token",
					},
				},
				"maxRetries": 50,
			},
			expectError:           false,
			expectedDeterministic: "$result.hasMore == true",
			expectedMaxRetries:    50,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config, err := ParseReRunIf(tt.reRunIf)

			if tt.expectError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)

			// If nil input, expect nil config
			if tt.reRunIf == nil || tt.reRunIf == "" {
				assert.Nil(t, config)
				return
			}

			require.NotNil(t, config)

			if tt.expectedDeterministic != "" {
				assert.Equal(t, tt.expectedDeterministic, config.Deterministic)
			}

			if tt.expectedCondition != "" {
				assert.Equal(t, tt.expectedCondition, config.Condition)
			}

			if tt.expectedMaxRetries > 0 {
				assert.Equal(t, tt.expectedMaxRetries, config.MaxRetries)
			}

			if tt.expectedDelayMs > 0 {
				assert.Equal(t, tt.expectedDelayMs, config.DelayMs)
			}

			if tt.expectedScope != "" {
				assert.Equal(t, tt.expectedScope, config.Scope)
			}

			if tt.expectedCombineMode != "" {
				assert.Equal(t, tt.expectedCombineMode, config.CombineMode)
			}

			if tt.expectedCallName != "" {
				require.NotNil(t, config.Call)
				assert.Equal(t, tt.expectedCallName, config.Call.Name)
			}
		})
	}
}

func TestReRunIfConfig_GetMaxRetries(t *testing.T) {
	tests := []struct {
		name     string
		config   ReRunIfConfig
		expected int
	}{
		{
			name:     "Zero returns default",
			config:   ReRunIfConfig{MaxRetries: 0},
			expected: ReRunIfDefaultMaxRetries,
		},
		{
			name:     "Negative returns default",
			config:   ReRunIfConfig{MaxRetries: -5},
			expected: ReRunIfDefaultMaxRetries,
		},
		{
			name:     "Positive returns value",
			config:   ReRunIfConfig{MaxRetries: 10},
			expected: 10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.config.GetMaxRetries())
		})
	}
}

func TestReRunIfConfig_GetScope(t *testing.T) {
	tests := []struct {
		name     string
		config   ReRunIfConfig
		expected string
	}{
		{
			name:     "Empty returns default (steps)",
			config:   ReRunIfConfig{Scope: ""},
			expected: ReRunScopeSteps,
		},
		{
			name:     "steps returns steps",
			config:   ReRunIfConfig{Scope: "steps"},
			expected: "steps",
		},
		{
			name:     "full returns full",
			config:   ReRunIfConfig{Scope: "full"},
			expected: "full",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.config.GetScope())
		})
	}
}

func TestReRunIfConfig_GetCombineMode(t *testing.T) {
	tests := []struct {
		name     string
		config   ReRunIfConfig
		expected string
	}{
		{
			name:     "Empty returns default (OR)",
			config:   ReRunIfConfig{CombineMode: ""},
			expected: CombineModeOR,
		},
		{
			name:     "and returns AND",
			config:   ReRunIfConfig{CombineMode: "and"},
			expected: CombineModeAND,
		},
		{
			name:     "or returns OR",
			config:   ReRunIfConfig{CombineMode: "or"},
			expected: CombineModeOR,
		},
		{
			name:     "AND returns AND",
			config:   ReRunIfConfig{CombineMode: "AND"},
			expected: CombineModeAND,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.config.GetCombineMode())
		})
	}
}

func TestReRunIfConfig_HasConditions(t *testing.T) {
	tests := []struct {
		name     string
		config   ReRunIfConfig
		expected bool
	}{
		{
			name:     "Empty config has no conditions",
			config:   ReRunIfConfig{},
			expected: false,
		},
		{
			name:     "Deterministic only",
			config:   ReRunIfConfig{Deterministic: "$result.status == 'pending'"},
			expected: true,
		},
		{
			name:     "Condition only",
			config:   ReRunIfConfig{Condition: "the result is incomplete"},
			expected: true,
		},
		{
			name:     "Inference only",
			config:   ReRunIfConfig{Inference: &InferenceCondition{Condition: "test"}},
			expected: true,
		},
		{
			name:     "Call only",
			config:   ReRunIfConfig{Call: &FunctionCall{Name: "validateResult"}},
			expected: true,
		},
		{
			name: "Multiple conditions",
			config: ReRunIfConfig{
				Deterministic: "$result.count == 0",
				Condition:     "needs more data",
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.config.HasConditions())
		})
	}
}

func TestValidateReRunIf(t *testing.T) {
	baseFunction := Function{
		Name:        "testFunction",
		Operation:   "api_call",
		Description: "Test function",
		Input: []Input{
			{Name: "pageToken", Description: "Token for pagination"},
			{Name: "offset", Description: "Offset for pagination"},
		},
		Steps: []Step{
			{Name: "step1", Action: "GET"},
		},
	}

	baseTool := Tool{
		Name: "TestTool",
		Functions: []Function{
			baseFunction,
			{
				Name:        "validateResult",
				Operation:   "policy",
				Description: "Validate result",
			},
		},
	}

	tests := []struct {
		name        string
		reRunIf     interface{}
		expectError bool
		errorMsg    string
	}{
		{
			name:        "Valid deterministic",
			reRunIf:     "$result.status != 'complete'",
			expectError: false,
		},
		{
			name: "Valid object format",
			reRunIf: map[string]interface{}{
				"deterministic": "$result.count == 0",
				"maxRetries":    10,
			},
			expectError: false,
		},
		{
			name: "Invalid scope",
			reRunIf: map[string]interface{}{
				"deterministic": "$result.done == false",
				"scope":         "invalid",
			},
			expectError: true,
			errorMsg:    "scope",
		},
		{
			name: "Invalid combineWith",
			reRunIf: map[string]interface{}{
				"deterministic": "$result.done == false",
				"combineWith":   "xor",
			},
			expectError: true,
			errorMsg:    "combineWith",
		},
		{
			name: "Negative maxRetries",
			reRunIf: map[string]interface{}{
				"deterministic": "$result.done == false",
				"maxRetries":    -5,
			},
			expectError: true,
			errorMsg:    "maxRetries",
		},
		{
			name: "Negative delayMs",
			reRunIf: map[string]interface{}{
				"deterministic": "$result.done == false",
				"delayMs":       -100,
			},
			expectError: true,
			errorMsg:    "delayMs",
		},
		{
			name: "Empty reRunIf object",
			reRunIf: map[string]interface{}{
				"maxRetries": 10,
			},
			expectError: true,
			errorMsg:    "at least one of",
		},
		{
			name: "Valid function call",
			reRunIf: map[string]interface{}{
				"call": map[string]interface{}{
					"name": "validateResult",
				},
			},
			expectError: false,
		},
		{
			name: "Function call - function not found",
			reRunIf: map[string]interface{}{
				"call": map[string]interface{}{
					"name": "nonExistentFunction",
				},
			},
			expectError: true,
			errorMsg:    "not available",
		},
		{
			name: "Function call - self reference",
			reRunIf: map[string]interface{}{
				"call": map[string]interface{}{
					"name": "testFunction",
				},
			},
			expectError: true,
			errorMsg:    "call itself",
		},
		{
			name: "Valid params with known input",
			reRunIf: map[string]interface{}{
				"deterministic": "$result.hasMore == true",
				"params": map[string]interface{}{
					"pageToken": "$result.nextPageToken",
				},
			},
			expectError: false,
		},
		{
			name: "Invalid params with unknown input name",
			reRunIf: map[string]interface{}{
				"deterministic": "$result.hasMore == true",
				"params": map[string]interface{}{
					"unknownParam": "$result.nextPageToken",
				},
			},
			expectError: true,
			errorMsg:    "unknown input names",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			function := baseFunction
			function.ReRunIf = tt.reRunIf

			err := validateReRunIf(tt.reRunIf, function, baseTool)

			if tt.expectError {
				require.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestParseReRunIf_Params(t *testing.T) {
	tests := []struct {
		name           string
		reRunIf        interface{}
		expectError    bool
		expectedParams map[string]interface{}
	}{
		{
			name: "Simple params",
			reRunIf: map[string]interface{}{
				"deterministic": "$result.hasMore == true",
				"params": map[string]interface{}{
					"pageToken": "$result.nextPageToken",
				},
			},
			expectError: false,
			expectedParams: map[string]interface{}{
				"pageToken": "$result.nextPageToken",
			},
		},
		{
			name: "Multiple params",
			reRunIf: map[string]interface{}{
				"deterministic": "$result.hasMore == true",
				"params": map[string]interface{}{
					"pageToken": "$result.nextPageToken",
					"offset":    "$offset + 100",
					"cursor":    "result[0].cursor",
				},
			},
			expectError: false,
			expectedParams: map[string]interface{}{
				"pageToken": "$result.nextPageToken",
				"offset":    "$offset + 100",
				"cursor":    "result[0].cursor",
			},
		},
		{
			name: "Nested params",
			reRunIf: map[string]interface{}{
				"deterministic": "$result.hasMore == true",
				"params": map[string]interface{}{
					"nested": map[string]interface{}{
						"token": "$result.auth.token",
						"page":  "$result.nextPage",
					},
				},
			},
			expectError: false,
			expectedParams: map[string]interface{}{
				"nested": map[string]interface{}{
					"token": "$result.auth.token",
					"page":  "$result.nextPage",
				},
			},
		},
		{
			name: "No params",
			reRunIf: map[string]interface{}{
				"deterministic": "$result.done == false",
			},
			expectError:    false,
			expectedParams: nil,
		},
		{
			name: "Empty params",
			reRunIf: map[string]interface{}{
				"deterministic": "$result.done == false",
				"params":        map[string]interface{}{},
			},
			expectError:    false,
			expectedParams: map[string]interface{}{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config, err := ParseReRunIf(tt.reRunIf)

			if tt.expectError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, config)

			if tt.expectedParams == nil {
				assert.Nil(t, config.Params)
			} else {
				assert.Equal(t, tt.expectedParams, config.Params)
			}
		})
	}
}
