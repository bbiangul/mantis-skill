package skill

import (
	"testing"
)

// Note: TestExtractFunctionReferences is in helpers_test.go since that function is in helpers.go

func TestExtractInputReferences(t *testing.T) {
	inputNames := []string{"userId", "email", "name"}

	testCases := []struct {
		name     string
		value    string
		expected []string
	}{
		{
			name:     "Single input reference",
			value:    "$userId",
			expected: []string{"userId"},
		},
		{
			name:     "Multiple input references",
			value:    "$userId and $email",
			expected: []string{"userId", "email"},
		},
		{
			name:     "Non-input variable",
			value:    "$unknownVar",
			expected: nil,
		},
		{
			name:     "Mixed input and other references",
			value:    "$userId && $unknownVar",
			expected: []string{"userId"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := ExtractInputReferences(tc.value, inputNames)
			if len(result) != len(tc.expected) {
				t.Errorf("Expected %v, got %v", tc.expected, result)
				return
			}
			for i, ref := range result {
				if ref != tc.expected[i] {
					t.Errorf("Expected %v, got %v", tc.expected, result)
					return
				}
			}
		})
	}
}

func TestExtractResultReferences(t *testing.T) {
	testCases := []struct {
		name     string
		value    string
		expected []int
	}{
		{
			name:     "Single result reference",
			value:    "result[1].data",
			expected: []int{1},
		},
		{
			name:     "Multiple result references",
			value:    "result[1].id && result[2].name",
			expected: []int{1, 2},
		},
		{
			name:     "No result references",
			value:    "$funcName.result",
			expected: nil,
		},
		{
			name:     "Result with higher index",
			value:    "result[10]",
			expected: []int{10},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := ExtractResultReferences(tc.value)
			if len(result) != len(tc.expected) {
				t.Errorf("Expected %v, got %v", tc.expected, result)
				return
			}
			for i, ref := range result {
				if ref != tc.expected[i] {
					t.Errorf("Expected %v, got %v", tc.expected, result)
					return
				}
			}
		})
	}
}

func TestGetAvailableSystemVars(t *testing.T) {
	// Test time_based trigger (no $MESSAGE or $USER)
	vars := GetAvailableSystemVars(TriggerTime)
	hasMessage := false
	hasUser := false
	for _, v := range vars {
		if v == SysVarMessage {
			hasMessage = true
		}
		if v == SysVarUser {
			hasUser = true
		}
	}
	if hasMessage {
		t.Error("Expected $MESSAGE to NOT be available for time_based trigger")
	}
	if hasUser {
		t.Error("Expected $USER to NOT be available for time_based trigger")
	}

	// Test always_on_user_message trigger (has $MESSAGE and $USER)
	vars = GetAvailableSystemVars(TriggerOnUserMessage)
	hasMessage = false
	hasUser = false
	for _, v := range vars {
		if v == SysVarMessage {
			hasMessage = true
		}
		if v == SysVarUser {
			hasUser = true
		}
	}
	if !hasMessage {
		t.Error("Expected $MESSAGE to be available for always_on_user_message trigger")
	}
	if !hasUser {
		t.Error("Expected $USER to be available for always_on_user_message trigger")
	}
}

// Note: TestIsSystemVariable is in helpers_test.go since that function is in helpers.go

func TestBuildFunctionDependencyMap(t *testing.T) {
	functions := []Function{
		{
			Name: "FuncA",
			Needs: []NeedItem{
				{Name: "FuncB"},
				{Name: "FuncC"},
			},
		},
		{
			Name: "FuncB",
			Needs: []NeedItem{
				{Name: "FuncC"},
			},
		},
		{
			Name: "FuncC",
		},
	}

	depMap := BuildFunctionDependencyMap(functions)

	// Check FuncA dependencies
	if len(depMap["FuncA"]) != 2 {
		t.Errorf("Expected FuncA to have 2 dependencies, got %d", len(depMap["FuncA"]))
	}

	// Check FuncB dependencies
	if len(depMap["FuncB"]) != 1 {
		t.Errorf("Expected FuncB to have 1 dependency, got %d", len(depMap["FuncB"]))
	}

	// Check FuncC dependencies (should have none)
	if len(depMap["FuncC"]) != 0 {
		t.Errorf("Expected FuncC to have 0 dependencies, got %d", len(depMap["FuncC"]))
	}
}

func TestGetReachableFunctions(t *testing.T) {
	depMap := map[string][]string{
		"FuncA": {"FuncB", "FuncC"},
		"FuncB": {"FuncC"},
		"FuncC": {},
	}

	reachable := GetReachableFunctions("FuncA", depMap)

	// Should include FuncB and FuncC
	if len(reachable) != 2 {
		t.Errorf("Expected 2 reachable functions, got %d", len(reachable))
	}

	// Check specific functions are included
	hasFuncB := false
	hasFuncC := false
	for _, f := range reachable {
		if f == "FuncB" {
			hasFuncB = true
		}
		if f == "FuncC" {
			hasFuncC = true
		}
	}

	if !hasFuncB {
		t.Error("Expected FuncB to be reachable")
	}
	if !hasFuncC {
		t.Error("Expected FuncC to be reachable")
	}
}
