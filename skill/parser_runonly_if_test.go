package skill

import (
	"testing"
)

// TestParseRunOnlyIf_BackwardCompatibility tests backward compatible string format
func TestParseRunOnlyIf_BackwardCompatibility(t *testing.T) {
	tests := []struct {
		name    string
		input   interface{}
		want    *RunOnlyIfObject
		wantErr bool
	}{
		{
			name:  "simple string condition",
			input: "user has mentioned they want detailed analysis",
			want: &RunOnlyIfObject{
				Condition: "user has mentioned they want detailed analysis",
			},
			wantErr: false,
		},
		{
			name:    "empty string",
			input:   "",
			want:    nil,
			wantErr: false,
		},
		{
			name:    "nil input",
			input:   nil,
			want:    nil,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseRunOnlyIf(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseRunOnlyIf() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !compareRunOnlyIfObjects(got, tt.want) {
				t.Errorf("ParseRunOnlyIf() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// TestParseRunOnlyIf_DeterministicMode tests deterministic condition parsing
func TestParseRunOnlyIf_DeterministicMode(t *testing.T) {
	tests := []struct {
		name    string
		input   map[string]interface{}
		want    *RunOnlyIfObject
		wantErr bool
	}{
		{
			name: "simple deterministic condition",
			input: map[string]interface{}{
				"deterministic": "$getUserStatus.result.isActive == true",
			},
			want: &RunOnlyIfObject{
				Deterministic: "$getUserStatus.result.isActive == true",
			},
			wantErr: false,
		},
		{
			name: "deterministic with && operator",
			input: map[string]interface{}{
				"deterministic": "$getUserStatus.result.isActive == true && $getBalance.result.amount > 100",
			},
			want: &RunOnlyIfObject{
				Deterministic: "$getUserStatus.result.isActive == true && $getBalance.result.amount > 100",
			},
			wantErr: false,
		},
		{
			name: "deterministic with || operator",
			input: map[string]interface{}{
				"deterministic": "$tier.result == 'premium' || $isAdmin.result == true",
			},
			want: &RunOnlyIfObject{
				Deterministic: "$tier.result == 'premium' || $isAdmin.result == true",
			},
			wantErr: false,
		},
		{
			name: "deterministic must be string",
			input: map[string]interface{}{
				"deterministic": 123,
			},
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseRunOnlyIf(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseRunOnlyIf() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && !compareRunOnlyIfObjects(got, tt.want) {
				t.Errorf("ParseRunOnlyIf() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// TestParseRunOnlyIf_InferenceObjectMode tests advanced inference object mode
func TestParseRunOnlyIf_InferenceObjectMode(t *testing.T) {
	tests := []struct {
		name    string
		input   map[string]interface{}
		want    *RunOnlyIfObject
		wantErr bool
	}{
		{
			name: "inference object with condition",
			input: map[string]interface{}{
				"inference": map[string]interface{}{
					"condition": "user explicitly requested premium features",
				},
			},
			want: &RunOnlyIfObject{
				Inference: &InferenceCondition{
					Condition: "user explicitly requested premium features",
				},
			},
			wantErr: false,
		},
		{
			name: "inference object with from",
			input: map[string]interface{}{
				"inference": map[string]interface{}{
					"condition": "user has permissions",
					"from":      []interface{}{"getUserRole", "checkPermissions"},
				},
			},
			want: &RunOnlyIfObject{
				Inference: &InferenceCondition{
					Condition: "user has permissions",
					From:      []string{"getUserRole", "checkPermissions"},
				},
			},
			wantErr: false,
		},
		{
			name: "inference object with allowedSystemFunctions",
			input: map[string]interface{}{
				"inference": map[string]interface{}{
					"condition":              "check conversation history",
					"allowedSystemFunctions": []interface{}{"askToTheConversationHistoryWithCustomer"},
				},
			},
			want: &RunOnlyIfObject{
				Inference: &InferenceCondition{
					Condition:              "check conversation history",
					AllowedSystemFunctions: []string{"askToTheConversationHistoryWithCustomer"},
				},
			},
			wantErr: false,
		},
		{
			name: "inference object without condition",
			input: map[string]interface{}{
				"inference": map[string]interface{}{
					"from": []interface{}{"getUserRole"},
				},
			},
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseRunOnlyIf(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseRunOnlyIf() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && !compareRunOnlyIfObjects(got, tt.want) {
				t.Errorf("ParseRunOnlyIf() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// TestParseRunOnlyIf_HybridMode tests hybrid mode (deterministic + inference)
func TestParseRunOnlyIf_HybridMode(t *testing.T) {
	tests := []struct {
		name    string
		input   map[string]interface{}
		want    *RunOnlyIfObject
		wantErr bool
	}{
		{
			name: "hybrid with AND combine",
			input: map[string]interface{}{
				"deterministic": "$getUserStatus.result.isActive == true",
				"inference": map[string]interface{}{
					"condition": "user explicitly requested this action",
				},
				"combineWith": "AND",
			},
			want: &RunOnlyIfObject{
				Deterministic: "$getUserStatus.result.isActive == true",
				Inference: &InferenceCondition{
					Condition: "user explicitly requested this action",
				},
				CombineMode: "AND",
			},
			wantErr: false,
		},
		{
			name: "hybrid with OR combine",
			input: map[string]interface{}{
				"deterministic": "$isAdmin.result == true",
				"condition":     "user has special override permission",
				"combineWith":   "OR",
			},
			want: &RunOnlyIfObject{
				Deterministic: "$isAdmin.result == true",
				Condition:     "user has special override permission",
				CombineMode:   "OR",
			},
			wantErr: false,
		},
		{
			name: "hybrid with default AND",
			input: map[string]interface{}{
				"deterministic": "$balance.result > 100",
				"condition":     "user confirmed",
			},
			want: &RunOnlyIfObject{
				Deterministic: "$balance.result > 100",
				Condition:     "user confirmed",
				CombineMode:   "AND", // Default
			},
			wantErr: false,
		},
		{
			name: "invalid combineWith value",
			input: map[string]interface{}{
				"deterministic": "$status == true",
				"condition":     "user confirmed",
				"combineWith":   "INVALID",
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "combineWith without both conditions",
			input: map[string]interface{}{
				"deterministic": "$status == true",
				"combineWith":   "AND",
			},
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseRunOnlyIf(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseRunOnlyIf() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && !compareRunOnlyIfObjects(got, tt.want) {
				t.Errorf("ParseRunOnlyIf() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// TestParseRunOnlyIf_ValidationErrors tests validation errors
func TestParseRunOnlyIf_ValidationErrors(t *testing.T) {
	tests := []struct {
		name    string
		input   map[string]interface{}
		wantErr bool
		errMsg  string
	}{
		{
			name: "both condition and inference specified",
			input: map[string]interface{}{
				"condition": "check something",
				"inference": map[string]interface{}{
					"condition": "check something else",
				},
			},
			wantErr: true,
			errMsg:  "cannot have both 'condition' and 'inference' fields",
		},
		{
			name: "disableAllSystemFunctions with allowedSystemFunctions",
			input: map[string]interface{}{
				"condition":                 "check something",
				"disableAllSystemFunctions": true,
				"allowedSystemFunctions":    []interface{}{"askToContext"},
			},
			wantErr: true,
			errMsg:  "cannot have both disableAllSystemFunctions=true and allowedSystemFunctions",
		},
		{
			name:    "no condition specified",
			input:   map[string]interface{}{},
			wantErr: true,
			errMsg:  "must specify at least one of: condition, deterministic, or inference",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseRunOnlyIf(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseRunOnlyIf() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && err != nil && tt.errMsg != "" {
				if !contains(err.Error(), tt.errMsg) {
					t.Errorf("ParseRunOnlyIf() error = %v, expected to contain %v", err, tt.errMsg)
				}
			}
		})
	}
}

// Helper function to compare RunOnlyIfObjects
func compareRunOnlyIfObjects(got, want *RunOnlyIfObject) bool {
	if got == nil && want == nil {
		return true
	}
	if got == nil || want == nil {
		return false
	}

	if got.Condition != want.Condition {
		return false
	}
	if got.Deterministic != want.Deterministic {
		return false
	}
	if got.CombineMode != want.CombineMode {
		return false
	}

	// Compare Inference objects
	if (got.Inference == nil) != (want.Inference == nil) {
		return false
	}
	if got.Inference != nil {
		if got.Inference.Condition != want.Inference.Condition {
			return false
		}
		if !stringSliceEqual(got.Inference.From, want.Inference.From) {
			return false
		}
		if !stringSliceEqual(got.Inference.AllowedSystemFunctions, want.Inference.AllowedSystemFunctions) {
			return false
		}
		if got.Inference.DisableAllSystemFunctions != want.Inference.DisableAllSystemFunctions {
			return false
		}
	}

	return true
}

func stringSliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

