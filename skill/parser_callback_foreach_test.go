package skill

import (
	"strings"
	"testing"
)

// TestValidateCallbackForEach tests the validateCallbackForEach function
func TestValidateCallbackForEach(t *testing.T) {
	tests := []struct {
		name        string
		forEach     *CallbackForEach
		wantErr     bool
		expectedErr string
	}{
		{
			name: "valid forEach with all fields",
			forEach: &CallbackForEach{
				Items:    "$result.items",
				ItemVar:  "item",
				IndexVar: "idx",
			},
			wantErr: false,
		},
		{
			name: "valid forEach with defaults",
			forEach: &CallbackForEach{
				Items: "$result.appointments",
			},
			wantErr: false,
		},
		{
			name: "empty items field",
			forEach: &CallbackForEach{
				Items: "",
			},
			wantErr:     true,
			expectedErr: "has empty items field",
		},
		{
			name: "same indexVar and itemVar",
			forEach: &CallbackForEach{
				Items:    "$result.items",
				ItemVar:  "item",
				IndexVar: "item",
			},
			wantErr:     true,
			expectedErr: "has same indexVar and itemVar",
		},
		{
			name: "invalid indexVar (starts with number)",
			forEach: &CallbackForEach{
				Items:    "$result.items",
				IndexVar: "1index",
			},
			wantErr:     true,
			expectedErr: "has invalid indexVar",
		},
		{
			name: "invalid itemVar (contains special char)",
			forEach: &CallbackForEach{
				Items:   "$result.items",
				ItemVar: "item-name",
			},
			wantErr:     true,
			expectedErr: "has invalid itemVar",
		},
		{
			name: "valid forEach with underscore in var names",
			forEach: &CallbackForEach{
				Items:    "$result.data",
				ItemVar:  "my_item",
				IndexVar: "loop_index",
			},
			wantErr: false,
		},
		{
			name: "valid forEach with input reference",
			forEach: &CallbackForEach{
				Items: "$appointments",
			},
			wantErr: false,
		},
		{
			name: "valid forEach with custom separator",
			forEach: &CallbackForEach{
				Items:     "$csvData",
				Separator: ";",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateCallbackForEach(tt.forEach, "testCallback", "testFunc", "testTool", "onSuccess")

			if tt.wantErr {
				if err == nil {
					t.Errorf("validateCallbackForEach() expected error containing %q, got nil", tt.expectedErr)
				} else if tt.expectedErr != "" && !strings.Contains(err.Error(), tt.expectedErr) {
					t.Errorf("validateCallbackForEach() error = %v, expected error containing %q", err, tt.expectedErr)
				}
			} else {
				if err != nil {
					t.Errorf("validateCallbackForEach() unexpected error: %v", err)
				}

				// Check that defaults were set
				if tt.forEach.Separator == "" {
					t.Errorf("validateCallbackForEach() did not set default separator")
				}
				if tt.forEach.IndexVar == "" {
					t.Errorf("validateCallbackForEach() did not set default indexVar")
				}
				if tt.forEach.ItemVar == "" {
					t.Errorf("validateCallbackForEach() did not set default itemVar")
				}
			}
		})
	}
}

// TestCallbackForEachModelParsing tests that ForEach fields are correctly parsed
func TestCallbackForEachModelParsing(t *testing.T) {
	// Test that CallbackForEach struct can be created and fields are accessible
	forEach := &CallbackForEach{
		Items:     "$result.items",
		ItemVar:   "currentItem",
		IndexVar:  "currentIndex",
		Separator: ";",
	}

	if forEach.Items != "$result.items" {
		t.Errorf("CallbackForEach.Items = %q, want %q", forEach.Items, "$result.items")
	}
	if forEach.ItemVar != "currentItem" {
		t.Errorf("CallbackForEach.ItemVar = %q, want %q", forEach.ItemVar, "currentItem")
	}
	if forEach.IndexVar != "currentIndex" {
		t.Errorf("CallbackForEach.IndexVar = %q, want %q", forEach.IndexVar, "currentIndex")
	}
	if forEach.Separator != ";" {
		t.Errorf("CallbackForEach.Separator = %q, want %q", forEach.Separator, ";")
	}
}

// TestFunctionCallWithForEach tests that FunctionCall correctly includes ForEach
func TestFunctionCallWithForEach(t *testing.T) {
	forEach := &CallbackForEach{
		Items:    "$result.data",
		ItemVar:  "item",
		IndexVar: "i",
	}

	funcCall := FunctionCall{
		Name: "processItem",
		Params: map[string]interface{}{
			"id":  "$item.id",
			"idx": "$i",
		},
		ForEach: forEach,
	}

	if funcCall.ForEach == nil {
		t.Fatal("FunctionCall.ForEach is nil")
	}

	if funcCall.ForEach.Items != "$result.data" {
		t.Errorf("FunctionCall.ForEach.Items = %q, want %q", funcCall.ForEach.Items, "$result.data")
	}
}
