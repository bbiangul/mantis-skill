package skill

import (
	"sort"
	"strings"
	"testing"
)

// TestExtractFunctionReferences tests that function references are extracted correctly
// and system variables like $NOW, $USER, etc. are excluded
func TestExtractFunctionReferences(t *testing.T) {
	tests := []struct {
		name       string
		expression string
		want       []string
	}{
		{
			name:       "simple function reference",
			expression: "$getUserStatus.result.isActive == true",
			want:       []string{"getUserStatus"},
		},
		{
			name:       "multiple function references",
			expression: "$getUserStatus.result.isActive == true && $getBalance.result.amount > 100",
			want:       []string{"getBalance", "getUserStatus"},
		},
		{
			name:       "system variable $NOW should be excluded",
			expression: "$NOW.hour >= 8 && $NOW.hour < 18",
			want:       []string{},
		},
		{
			name:       "mixed function and $NOW system variable",
			expression: "$getUserStatus.isActive == true && $NOW.hour >= 8",
			want:       []string{"getUserStatus"},
		},
		{
			name:       "all system variables should be excluded",
			expression: "$ME.name == 'test' && $NOW.hour >= 8 && $USER.id != '' && $COMPANY.name == 'test'",
			want:       []string{},
		},
		{
			name:       "function with system variable in complex expression",
			expression: "$checkUserEligibility.result == true && $NOW.hour >= 8 && $NOW.hour < 18 && $getBalance.amount > 100",
			want:       []string{"checkUserEligibility", "getBalance"},
		},
		{
			name:       "business hours pattern with $NOW",
			expression: "$NOW.hour >= 8 && $NOW.hour < 18",
			want:       []string{},
		},
		{
			name:       "function reference with array access",
			expression: "$getUsers[0].name == 'admin'",
			want:       []string{"getUsers"},
		},
		{
			name:       "empty expression",
			expression: "",
			want:       []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractFunctionReferences(tt.expression)

			// Sort both slices for consistent comparison
			sort.Strings(got)
			sort.Strings(tt.want)

			if len(got) != len(tt.want) {
				t.Errorf("ExtractFunctionReferences() returned %d items, want %d items. Got: %v, Want: %v",
					len(got), len(tt.want), got, tt.want)
				return
			}

			for i, v := range got {
				if v != tt.want[i] {
					t.Errorf("ExtractFunctionReferences()[%d] = %v, want %v", i, v, tt.want[i])
				}
			}
		})
	}
}

// TestIsSystemVariable tests the IsSystemVariable helper function
func TestIsSystemVariable(t *testing.T) {
	tests := []struct {
		name     string
		varName  string
		expected bool
	}{
		{"ME is system variable", "ME", true},
		{"MESSAGE is system variable", "MESSAGE", true},
		{"NOW is system variable", "NOW", true},
		{"USER is system variable", "USER", true},
		{"ADMIN is system variable", "ADMIN", true},
		{"COMPANY is system variable", "COMPANY", true},
		{"UUID is system variable", "UUID", true},
		{"FILE is system variable", "FILE", true},
		{"getUserStatus is NOT system variable", "getUserStatus", false},
		{"getBalance is NOT system variable", "getBalance", false},
		{"lowercase now is NOT system variable", "now", false},
		{"empty string is NOT system variable", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsSystemVariable(tt.varName)
			if got != tt.expected {
				t.Errorf("IsSystemVariable(%q) = %v, want %v", tt.varName, got, tt.expected)
			}
		})
	}
}

// TestEvaluateDeterministicExpression tests the deterministic expression evaluator
// All tests now use literal values since VariableReplacer handles variable replacement
func TestEvaluateDeterministicExpression(t *testing.T) {
	tests := []struct {
		name       string
		expression string
		want       bool
		wantErr    bool
	}{
		// Simple equality tests
		{
			name:       "simple boolean equality - true",
			expression: "true == true",
			want:       true,
			wantErr:    false,
		},
		{
			name:       "simple boolean equality - false",
			expression: "false == true",
			want:       false,
			wantErr:    false,
		},
		{
			name:       "string equality",
			expression: "'premium' == 'premium'",
			want:       true,
			wantErr:    false,
		},
		{
			name:       "string inequality",
			expression: "'basic' == 'premium'",
			want:       false,
			wantErr:    false,
		},
		{
			name:       "numeric greater than",
			expression: "150 > 100",
			want:       true,
			wantErr:    false,
		},
		{
			name:       "numeric less than",
			expression: "50 < 100",
			want:       true,
			wantErr:    false,
		},
		{
			name:       "numeric greater than or equal",
			expression: "100 >= 100",
			want:       true,
			wantErr:    false,
		},
		{
			name:       "numeric less than or equal",
			expression: "100 <= 100",
			want:       true,
			wantErr:    false,
		},
		{
			name:       "not equal operator",
			expression: "'active' != 'inactive'",
			want:       true,
			wantErr:    false,
		},

		// Logical AND tests
		{
			name:       "AND operator - both true",
			expression: "true == true && 150 > 100",
			want:       true,
			wantErr:    false,
		},
		{
			name:       "AND operator - first false",
			expression: "false == true && 150 > 100",
			want:       false,
			wantErr:    false,
		},
		{
			name:       "AND operator - second false",
			expression: "true == true && 50 > 100",
			want:       false,
			wantErr:    false,
		},

		// Logical OR tests
		{
			name:       "OR operator - both true",
			expression: "'premium' == 'premium' || true == true",
			want:       true,
			wantErr:    false,
		},
		{
			name:       "OR operator - first true, second false",
			expression: "'premium' == 'premium' || false == true",
			want:       true,
			wantErr:    false,
		},
		{
			name:       "OR operator - first false, second true",
			expression: "'basic' == 'premium' || true == true",
			want:       true,
			wantErr:    false,
		},
		{
			name:       "OR operator - both false",
			expression: "'basic' == 'premium' || false == true",
			want:       false,
			wantErr:    false,
		},

		// Complex expressions
		{
			name:       "complex AND and OR",
			expression: "true == true && 150 > 100 || false == true",
			want:       true,
			wantErr:    false,
		},

		// Error cases
		{
			name:       "empty expression",
			expression: "",
			want:       false,
			wantErr:    true,
		},
		{
			name:       "invalid expression format - no operator",
			expression: "true",
			want:       false,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := EvaluateDeterministicExpression(tt.expression)
			if (err != nil) != tt.wantErr {
				t.Errorf("EvaluateDeterministicExpression() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("EvaluateDeterministicExpression() = %v, want %v", got, tt.want)
			}
		})
	}
}

// ================================================================================
// Tests for Built-in Functions: len(), isEmpty(), contains(), exists()
// ================================================================================

// TestLenFunction tests the len() built-in function in deterministic expressions
func TestLenFunction(t *testing.T) {
	tests := []struct {
		name       string
		expression string
		want       bool
		wantErr    bool
	}{
		// ============================================
		// len() with arrays - basic cases
		// ============================================
		{
			name:       "len - empty array equals 0",
			expression: "len([]) == 0",
			want:       true,
			wantErr:    false,
		},
		{
			name:       "len - empty array greater than 0 is false",
			expression: "len([]) > 0",
			want:       false,
			wantErr:    false,
		},
		{
			name:       "len - single element array",
			expression: "len([1]) == 1",
			want:       true,
			wantErr:    false,
		},
		{
			name:       "len - three element array",
			expression: "len([1, 2, 3]) == 3",
			want:       true,
			wantErr:    false,
		},
		{
			name:       "len - array with strings",
			expression: `len(["a", "b", "c"]) == 3`,
			want:       true,
			wantErr:    false,
		},
		{
			name:       "len - array greater than comparison",
			expression: "len([1, 2, 3, 4, 5]) > 3",
			want:       true,
			wantErr:    false,
		},
		{
			name:       "len - array greater than or equal",
			expression: "len([1, 2, 3]) >= 3",
			want:       true,
			wantErr:    false,
		},
		{
			name:       "len - array less than",
			expression: "len([1, 2]) < 5",
			want:       true,
			wantErr:    false,
		},
		{
			name:       "len - array not equal",
			expression: "len([1, 2, 3]) != 0",
			want:       true,
			wantErr:    false,
		},

		// ============================================
		// len() with arrays - complex nested arrays
		// ============================================
		{
			name:       "len - nested arrays count outer elements",
			expression: "len([[1, 2], [3, 4]]) == 2",
			want:       true,
			wantErr:    false,
		},
		{
			name:       "len - array with objects",
			expression: `len([{"id": 1}, {"id": 2}]) == 2`,
			want:       true,
			wantErr:    false,
		},
		{
			name:       "len - array with mixed types",
			expression: `len([1, "two", true, null]) == 4`,
			want:       true,
			wantErr:    false,
		},

		// ============================================
		// len() with strings
		// ============================================
		{
			name:       "len - empty string single quotes",
			expression: "len('') == 0",
			want:       true,
			wantErr:    false,
		},
		{
			name:       "len - empty string double quotes",
			expression: `len("") == 0`,
			want:       true,
			wantErr:    false,
		},
		{
			name:       "len - simple string",
			expression: "len('hello') == 5",
			want:       true,
			wantErr:    false,
		},
		{
			name:       "len - string with spaces",
			expression: "len('hello world') == 11",
			want:       true,
			wantErr:    false,
		},
		{
			name:       "len - string greater than",
			expression: "len('test') > 2",
			want:       true,
			wantErr:    false,
		},

		// ============================================
		// len() with objects
		// ============================================
		{
			name:       "len - empty object",
			expression: "len({}) == 0",
			want:       true,
			wantErr:    false,
		},
		{
			name:       "len - object with one key",
			expression: `len({"key": "value"}) == 1`,
			want:       true,
			wantErr:    false,
		},
		{
			name:       "len - object with multiple keys",
			expression: `len({"a": 1, "b": 2, "c": 3}) == 3`,
			want:       true,
			wantErr:    false,
		},

		// ============================================
		// len() with null
		// ============================================
		{
			name:       "len - null returns 0",
			expression: "len(null) == 0",
			want:       true,
			wantErr:    false,
		},
		{
			name:       "len - null greater than 0 is false",
			expression: "len(null) > 0",
			want:       false,
			wantErr:    false,
		},

		// ============================================
		// len() in complex expressions
		// ============================================
		{
			name:       "len - with AND operator",
			expression: "len([1, 2, 3]) > 0 && len([4, 5]) > 0",
			want:       true,
			wantErr:    false,
		},
		{
			name:       "len - with OR operator",
			expression: "len([]) > 0 || len([1]) > 0",
			want:       true,
			wantErr:    false,
		},
		{
			name:       "len - comparing two lens",
			expression: "len([1, 2, 3]) > len([1])",
			want:       true,
			wantErr:    false,
		},
		{
			name:       "len - in parentheses",
			expression: "(len([1, 2, 3]) == 3)",
			want:       true,
			wantErr:    false,
		},

		// ============================================
		// Real-world use case: checking pending workflows
		// ============================================
		{
			name:       "len - realistic workflow check non-empty",
			expression: `len([{"id": 1, "user_id": "abc"}, {"id": 2, "user_id": "def"}]) > 0`,
			want:       true,
			wantErr:    false,
		},
		{
			name:       "len - realistic workflow check empty",
			expression: `len([]) > 0`,
			want:       false,
			wantErr:    false,
		},

		// ============================================
		// Go's fmt.Sprintf("%v", value) representations
		// These come from VariableReplacer when replacing $functionName with actual values
		// ============================================
		{
			name:       "len - Go empty map representation (map[])",
			expression: "len(map[]) > 0",
			want:       false,
			wantErr:    false,
		},
		{
			name:       "len - Go empty map equals 0",
			expression: "len(map[]) == 0",
			want:       true,
			wantErr:    false,
		},
		{
			name:       "len - Go non-empty map representation",
			expression: "len(map[key1:value1 key2:value2]) == 2",
			want:       true,
			wantErr:    false,
		},
		{
			name:       "len - Go map with nested structure",
			expression: "len(map[id:123 data:map[name:test]]) == 2",
			want:       true,
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := EvaluateDeterministicExpression(tt.expression)
			if (err != nil) != tt.wantErr {
				t.Errorf("EvaluateDeterministicExpression() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("EvaluateDeterministicExpression() = %v, want %v for expression: %s", got, tt.want, tt.expression)
			}
		})
	}
}

// TestIsEmptyFunction tests the isEmpty() built-in function in deterministic expressions
func TestIsEmptyFunction(t *testing.T) {
	tests := []struct {
		name       string
		expression string
		want       bool
		wantErr    bool
	}{
		// ============================================
		// isEmpty() with arrays
		// ============================================
		{
			name:       "isEmpty - empty array is true",
			expression: "isEmpty([]) == true",
			want:       true,
			wantErr:    false,
		},
		{
			name:       "isEmpty - non-empty array is false",
			expression: "isEmpty([1, 2, 3]) == false",
			want:       true,
			wantErr:    false,
		},
		{
			name:       "isEmpty - single element array is not empty",
			expression: "isEmpty([1]) == false",
			want:       true,
			wantErr:    false,
		},
		{
			name:       "isEmpty - array with objects is not empty",
			expression: `isEmpty([{"id": 1}]) == false`,
			want:       true,
			wantErr:    false,
		},

		// ============================================
		// isEmpty() with strings
		// ============================================
		{
			name:       "isEmpty - empty string single quotes is true",
			expression: "isEmpty('') == true",
			want:       true,
			wantErr:    false,
		},
		{
			name:       "isEmpty - empty string double quotes is true",
			expression: `isEmpty("") == true`,
			want:       true,
			wantErr:    false,
		},
		{
			name:       "isEmpty - non-empty string is false",
			expression: "isEmpty('hello') == false",
			want:       true,
			wantErr:    false,
		},
		{
			name:       "isEmpty - string with space is not empty",
			expression: "isEmpty(' ') == false",
			want:       true,
			wantErr:    false,
		},

		// ============================================
		// isEmpty() with objects
		// ============================================
		{
			name:       "isEmpty - empty object is true",
			expression: "isEmpty({}) == true",
			want:       true,
			wantErr:    false,
		},
		{
			name:       "isEmpty - non-empty object is false",
			expression: `isEmpty({"key": "value"}) == false`,
			want:       true,
			wantErr:    false,
		},

		// ============================================
		// isEmpty() with null
		// ============================================
		{
			name:       "isEmpty - null is true",
			expression: "isEmpty(null) == true",
			want:       true,
			wantErr:    false,
		},

		// ============================================
		// isEmpty() in conditional expressions
		// ============================================
		{
			name:       "isEmpty - negation check",
			expression: "isEmpty([1, 2]) == false",
			want:       true,
			wantErr:    false,
		},
		{
			name:       "isEmpty - with AND",
			expression: "isEmpty([]) == true && isEmpty('') == true",
			want:       true,
			wantErr:    false,
		},
		{
			name:       "isEmpty - with OR",
			expression: "isEmpty([1]) == true || isEmpty([]) == true",
			want:       true,
			wantErr:    false,
		},

		// ============================================
		// isEmpty() real-world use cases
		// ============================================
		{
			name:       "isEmpty - check if results exist before processing",
			expression: "isEmpty([]) == false",
			want:       false,
			wantErr:    false,
		},
		{
			name:       "isEmpty - skip processing if empty",
			expression: "isEmpty([]) == true",
			want:       true,
			wantErr:    false,
		},

		// ============================================
		// Go's fmt.Sprintf("%v", value) representations
		// ============================================
		{
			name:       "isEmpty - Go empty map representation (map[])",
			expression: "isEmpty(map[]) == true",
			want:       true,
			wantErr:    false,
		},
		{
			name:       "isEmpty - Go non-empty map representation",
			expression: "isEmpty(map[key:value]) == false",
			want:       true,
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := EvaluateDeterministicExpression(tt.expression)
			if (err != nil) != tt.wantErr {
				t.Errorf("EvaluateDeterministicExpression() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("EvaluateDeterministicExpression() = %v, want %v for expression: %s", got, tt.want, tt.expression)
			}
		})
	}
}

// TestContainsFunction tests the contains() built-in function in deterministic expressions
func TestContainsFunction(t *testing.T) {
	tests := []struct {
		name       string
		expression string
		want       bool
		wantErr    bool
	}{
		// ============================================
		// contains() with strings - substring search
		// ============================================
		{
			name:       "contains - string contains substring",
			expression: "contains('hello world', 'world') == true",
			want:       true,
			wantErr:    false,
		},
		{
			name:       "contains - string does not contain substring",
			expression: "contains('hello world', 'xyz') == false",
			want:       true,
			wantErr:    false,
		},
		{
			name:       "contains - string contains at beginning",
			expression: "contains('hello world', 'hello') == true",
			want:       true,
			wantErr:    false,
		},
		{
			name:       "contains - string contains at end",
			expression: "contains('hello world', 'world') == true",
			want:       true,
			wantErr:    false,
		},
		{
			name:       "contains - string contains single char",
			expression: "contains('hello', 'e') == true",
			want:       true,
			wantErr:    false,
		},
		{
			name:       "contains - empty needle always matches",
			expression: "contains('hello', '') == true",
			want:       true,
			wantErr:    false,
		},
		{
			name:       "contains - case sensitive",
			expression: "contains('Hello', 'hello') == false",
			want:       true,
			wantErr:    false,
		},

		// ============================================
		// contains() with arrays - element search
		// ============================================
		{
			name:       "contains - array contains number",
			expression: "contains([1, 2, 3], 2) == true",
			want:       true,
			wantErr:    false,
		},
		{
			name:       "contains - array does not contain number",
			expression: "contains([1, 2, 3], 5) == false",
			want:       true,
			wantErr:    false,
		},
		{
			name:       "contains - array contains string",
			expression: `contains(["a", "b", "c"], "b") == true`,
			want:       true,
			wantErr:    false,
		},
		{
			name:       "contains - array does not contain string",
			expression: `contains(["a", "b", "c"], "x") == false`,
			want:       true,
			wantErr:    false,
		},
		{
			name:       "contains - array contains first element",
			expression: "contains([1, 2, 3], 1) == true",
			want:       true,
			wantErr:    false,
		},
		{
			name:       "contains - array contains last element",
			expression: "contains([1, 2, 3], 3) == true",
			want:       true,
			wantErr:    false,
		},
		{
			name:       "contains - empty array contains nothing",
			expression: "contains([], 1) == false",
			want:       true,
			wantErr:    false,
		},

		// ============================================
		// contains() with null
		// ============================================
		{
			name:       "contains - null haystack returns false",
			expression: "contains(null, 'test') == false",
			want:       true,
			wantErr:    false,
		},

		// ============================================
		// contains() in complex expressions
		// ============================================
		{
			name:       "contains - with AND operator",
			expression: "contains('hello', 'ell') == true && contains('world', 'orl') == true",
			want:       true,
			wantErr:    false,
		},
		{
			name:       "contains - with OR operator",
			expression: "contains('hello', 'xyz') == true || contains('hello', 'ell') == true",
			want:       true,
			wantErr:    false,
		},

		// ============================================
		// contains() real-world use cases
		// ============================================
		{
			name:       "contains - check status in allowed list",
			expression: `contains(["pending", "processing", "completed"], "pending") == true`,
			want:       true,
			wantErr:    false,
		},
		{
			name:       "contains - check error message contains keyword",
			expression: "contains('Connection timeout error', 'timeout') == true",
			want:       true,
			wantErr:    false,
		},
		{
			name:       "contains - validate phone number prefix",
			expression: "contains('+5511999999999', '+55') == true",
			want:       true,
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := EvaluateDeterministicExpression(tt.expression)
			if (err != nil) != tt.wantErr {
				t.Errorf("EvaluateDeterministicExpression() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("EvaluateDeterministicExpression() = %v, want %v for expression: %s", got, tt.want, tt.expression)
			}
		})
	}
}

// TestExistsFunction tests the exists() built-in function in deterministic expressions
func TestExistsFunction(t *testing.T) {
	tests := []struct {
		name       string
		expression string
		want       bool
		wantErr    bool
	}{
		// ============================================
		// exists() with null
		// ============================================
		{
			name:       "exists - null does not exist",
			expression: "exists(null) == false",
			want:       true,
			wantErr:    false,
		},
		{
			name:       "exists - null exists check negation",
			expression: "exists(null) == true",
			want:       false,
			wantErr:    false,
		},

		// ============================================
		// exists() with empty strings
		// ============================================
		{
			name:       "exists - empty string single quotes does not exist",
			expression: "exists('') == false",
			want:       true,
			wantErr:    false,
		},
		{
			name:       "exists - empty string double quotes does not exist",
			expression: `exists("") == false`,
			want:       true,
			wantErr:    false,
		},

		// ============================================
		// exists() with non-empty values - all should exist
		// ============================================
		{
			name:       "exists - non-empty string exists",
			expression: "exists('hello') == true",
			want:       true,
			wantErr:    false,
		},
		{
			name:       "exists - string with space exists",
			expression: "exists(' ') == true",
			want:       true,
			wantErr:    false,
		},
		{
			name:       "exists - number exists",
			expression: "exists(123) == true",
			want:       true,
			wantErr:    false,
		},
		{
			name:       "exists - zero exists",
			expression: "exists(0) == true",
			want:       true,
			wantErr:    false,
		},
		{
			name:       "exists - false boolean exists",
			expression: "exists(false) == true",
			want:       true,
			wantErr:    false,
		},
		{
			name:       "exists - true boolean exists",
			expression: "exists(true) == true",
			want:       true,
			wantErr:    false,
		},

		// ============================================
		// exists() with arrays and objects - all exist even if empty
		// ============================================
		{
			name:       "exists - empty array exists",
			expression: "exists([]) == true",
			want:       true,
			wantErr:    false,
		},
		{
			name:       "exists - non-empty array exists",
			expression: "exists([1, 2, 3]) == true",
			want:       true,
			wantErr:    false,
		},
		{
			name:       "exists - empty object exists",
			expression: "exists({}) == true",
			want:       true,
			wantErr:    false,
		},
		{
			name:       "exists - non-empty object exists",
			expression: `exists({"key": "value"}) == true`,
			want:       true,
			wantErr:    false,
		},

		// ============================================
		// exists() in complex expressions
		// ============================================
		{
			name:       "exists - with AND operator",
			expression: "exists('hello') == true && exists(null) == false",
			want:       true,
			wantErr:    false,
		},
		{
			name:       "exists - with OR operator",
			expression: "exists(null) == true || exists('value') == true",
			want:       true,
			wantErr:    false,
		},

		// ============================================
		// exists() real-world use cases
		// ============================================
		{
			name:       "exists - check optional field exists",
			expression: "exists('user@email.com') == true",
			want:       true,
			wantErr:    false,
		},
		{
			name:       "exists - check optional field missing",
			expression: "exists(null) == false",
			want:       true,
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := EvaluateDeterministicExpression(tt.expression)
			if (err != nil) != tt.wantErr {
				t.Errorf("EvaluateDeterministicExpression() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("EvaluateDeterministicExpression() = %v, want %v for expression: %s", got, tt.want, tt.expression)
			}
		})
	}
}

// TestBuiltinFunctionsCombined tests multiple built-in functions used together
func TestBuiltinFunctionsCombined(t *testing.T) {
	tests := []struct {
		name       string
		expression string
		want       bool
		wantErr    bool
	}{
		// ============================================
		// Combining multiple functions
		// ============================================
		{
			name:       "len and isEmpty - consistent for empty array",
			expression: "len([]) == 0 && isEmpty([]) == true",
			want:       true,
			wantErr:    false,
		},
		{
			name:       "len and isEmpty - consistent for non-empty array",
			expression: "len([1, 2]) > 0 && isEmpty([1, 2]) == false",
			want:       true,
			wantErr:    false,
		},
		{
			name:       "exists and isEmpty - null case",
			expression: "exists(null) == false && isEmpty(null) == true",
			want:       true,
			wantErr:    false,
		},
		{
			name:       "len and contains - array operations",
			expression: "len([1, 2, 3]) == 3 && contains([1, 2, 3], 2) == true",
			want:       true,
			wantErr:    false,
		},

		// ============================================
		// Real-world combined use cases
		// ============================================
		{
			name:       "workflow guard - has items and contains status",
			expression: `len([{"status": "pending"}]) > 0 && contains(["pending", "active"], "pending") == true`,
			want:       true,
			wantErr:    false,
		},
		{
			name:       "validation - exists and not empty",
			expression: "exists('value') == true && isEmpty('value') == false",
			want:       true,
			wantErr:    false,
		},

		// ============================================
		// Complex nested conditions
		// ============================================
		{
			name:       "nested OR and AND with functions",
			expression: "(len([]) == 0 || len([1]) == 1) && exists('test') == true",
			want:       true,
			wantErr:    false,
		},
		{
			name:       "multiple function calls in OR",
			expression: "isEmpty([]) == true || isEmpty([1]) == true || isEmpty([2]) == true",
			want:       true,
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := EvaluateDeterministicExpression(tt.expression)
			if (err != nil) != tt.wantErr {
				t.Errorf("EvaluateDeterministicExpression() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("EvaluateDeterministicExpression() = %v, want %v for expression: %s", got, tt.want, tt.expression)
			}
		})
	}
}

// TestBackwardsCompatibility ensures all original functionality still works
func TestDeterministicBackwardsCompatibility(t *testing.T) {
	tests := []struct {
		name       string
		expression string
		want       bool
		wantErr    bool
	}{
		// All original tests should still pass
		{
			name:       "original - simple equality",
			expression: "'D' == 'D'",
			want:       true,
			wantErr:    false,
		},
		{
			name:       "original - number comparison",
			expression: "157 > 100",
			want:       true,
			wantErr:    false,
		},
		{
			name:       "original - OR expression",
			expression: "'D' == 'S' || 'D' == 'D'",
			want:       true,
			wantErr:    false,
		},
		{
			name:       "original - AND expression",
			expression: "'D' == 'D' && 157 > 100",
			want:       true,
			wantErr:    false,
		},
		{
			name:       "original - null comparison",
			expression: "null == null",
			want:       true,
			wantErr:    false,
		},
		{
			name:       "original - null vs value",
			expression: "null == 'test'",
			want:       false,
			wantErr:    false,
		},
		{
			name:       "null equals empty string",
			expression: "null == ''",
			want:       true,
			wantErr:    false,
		},
		{
			name:       "empty string equals null",
			expression: "'' == null",
			want:       true,
			wantErr:    false,
		},
		{
			name:       "null not equal empty string",
			expression: "null != ''",
			want:       false,
			wantErr:    false,
		},
		{
			name:       "empty string not equal null",
			expression: "'' != null",
			want:       false,
			wantErr:    false,
		},
		{
			name:       "original - boolean true",
			expression: "true == true",
			want:       true,
			wantErr:    false,
		},
		{
			name:       "original - boolean false",
			expression: "false == true",
			want:       false,
			wantErr:    false,
		},
		{
			name:       "original - not equal",
			expression: "'active' != 'inactive'",
			want:       true,
			wantErr:    false,
		},
		{
			name:       "original - less than or equal",
			expression: "100 <= 150",
			want:       true,
			wantErr:    false,
		},
		{
			name:       "original - greater than or equal",
			expression: "150 >= 100",
			want:       true,
			wantErr:    false,
		},
		{
			name:       "original - parentheses",
			expression: "(true == true)",
			want:       true,
			wantErr:    false,
		},
		{
			name:       "original - complex with parentheses",
			expression: "('D' == 'D' || 'S' == 'S') && 100 > 50",
			want:       true,
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := EvaluateDeterministicExpression(tt.expression)
			if (err != nil) != tt.wantErr {
				t.Errorf("EvaluateDeterministicExpression() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("EvaluateDeterministicExpression() = %v, want %v for expression: %s", got, tt.want, tt.expression)
			}
		})
	}
}

// TestEdgeCasesAndErrorHandling tests edge cases and error conditions
func TestEdgeCasesAndErrorHandling(t *testing.T) {
	tests := []struct {
		name       string
		expression string
		want       bool
		wantErr    bool
	}{
		// ============================================
		// Edge cases that should work
		// ============================================
		{
			name:       "edge - whitespace around function",
			expression: " len([1, 2, 3]) == 3 ",
			want:       true,
			wantErr:    false,
		},
		{
			name:       "edge - whitespace in array",
			expression: "len([ 1 , 2 , 3 ]) == 3",
			want:       true,
			wantErr:    false,
		},
		{
			name:       "edge - nested brackets in string",
			expression: "contains('array[0]', '[') == true",
			want:       true,
			wantErr:    false,
		},
		{
			name:       "edge - special chars in string",
			expression: "contains('hello@world.com', '@') == true",
			want:       true,
			wantErr:    false,
		},

		// ============================================
		// Function inside function should NOT work (no nesting)
		// ============================================
		// Note: We explicitly do NOT support nested function calls
		// These should evaluate but may give unexpected results

		// ============================================
		// Error cases
		// ============================================
		{
			name:       "error - contains with one argument",
			expression: "contains('hello') == true",
			want:       false,
			wantErr:    true,
		},
		{
			name:       "error - empty expression",
			expression: "",
			want:       false,
			wantErr:    true,
		},
		{
			name:       "error - no operator",
			expression: "len([1, 2, 3])",
			want:       false,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := EvaluateDeterministicExpression(tt.expression)
			if (err != nil) != tt.wantErr {
				t.Errorf("EvaluateDeterministicExpression() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("EvaluateDeterministicExpression() = %v, want %v for expression: %s", got, tt.want, tt.expression)
			}
		})
	}
}

// TestValidateDeterministicExpression tests the parse-time validation of deterministic expressions
// This validates that unsupported functions are rejected at parse time
func TestValidateDeterministicExpression(t *testing.T) {
	tests := []struct {
		name        string
		expression  string
		wantErr     bool
		errContains string
	}{
		// ============================================
		// Valid expressions - supported functions
		// ============================================
		{
			name:       "valid - len function",
			expression: "len([1, 2, 3]) > 0",
			wantErr:    false,
		},
		{
			name:       "valid - isEmpty function",
			expression: "isEmpty([]) == true",
			wantErr:    false,
		},
		{
			name:       "valid - contains function",
			expression: "contains('hello', 'ell') == true",
			wantErr:    false,
		},
		{
			name:       "valid - exists function",
			expression: "exists($value) == true",
			wantErr:    false,
		},
		{
			name:       "valid - multiple supported functions",
			expression: "len([1, 2]) > 0 && isEmpty([]) == true",
			wantErr:    false,
		},
		{
			name:       "valid - no functions, just comparisons",
			expression: "'D' == 'D' && 100 > 50",
			wantErr:    false,
		},
		{
			name:       "valid - variable references (not functions)",
			expression: "$result.status == 'active'",
			wantErr:    false,
		},
		{
			name:       "valid - complex valid expression",
			expression: "len($getPendingWorkflows) > 0 && contains(['pending', 'active'], $status) == true",
			wantErr:    false,
		},

		// ============================================
		// Invalid expressions - unsupported functions
		// ============================================
		{
			name:        "invalid - uppercase function",
			expression:  "upper('hello') == 'HELLO'",
			wantErr:     true,
			errContains: "unsupported function",
		},
		{
			name:        "invalid - lowercase function",
			expression:  "lower('HELLO') == 'hello'",
			wantErr:     true,
			errContains: "unsupported function",
		},
		{
			name:        "invalid - startsWith function",
			expression:  "startsWith('hello', 'he') == true",
			wantErr:     true,
			errContains: "unsupported function",
		},
		{
			name:        "invalid - endsWith function",
			expression:  "endsWith('hello', 'lo') == true",
			wantErr:     true,
			errContains: "unsupported function",
		},
		{
			name:        "invalid - trim function",
			expression:  "trim('  hello  ') == 'hello'",
			wantErr:     true,
			errContains: "unsupported function",
		},
		{
			name:        "invalid - int function",
			expression:  "int('123') == 123",
			wantErr:     true,
			errContains: "unsupported function",
		},
		{
			name:        "invalid - float function",
			expression:  "float('12.5') > 10",
			wantErr:     true,
			errContains: "unsupported function",
		},
		{
			name:        "invalid - matches (regex) function",
			expression:  "matches($email, '^[a-z]+@[a-z]+\\.com$') == true",
			wantErr:     true,
			errContains: "unsupported function",
		},
		{
			name:        "invalid - substr function",
			expression:  "substr('hello', 0, 2) == 'he'",
			wantErr:     true,
			errContains: "unsupported function",
		},
		{
			name:        "invalid - indexOf function",
			expression:  "indexOf('hello', 'l') == 2",
			wantErr:     true,
			errContains: "unsupported function",
		},
		{
			name:        "invalid - split function",
			expression:  "len(split('a,b,c', ',')) == 3",
			wantErr:     true,
			errContains: "unsupported function",
		},
		{
			name:        "invalid - join function",
			expression:  "join(['a', 'b', 'c'], ',') == 'a,b,c'",
			wantErr:     true,
			errContains: "unsupported function",
		},
		{
			name:        "invalid - replace function",
			expression:  "replace('hello', 'l', 'x') == 'hexxo'",
			wantErr:     true,
			errContains: "unsupported function",
		},
		{
			name:        "invalid - abs function",
			expression:  "abs(-5) == 5",
			wantErr:     true,
			errContains: "unsupported function",
		},
		{
			name:        "invalid - max function",
			expression:  "max(1, 2, 3) == 3",
			wantErr:     true,
			errContains: "unsupported function",
		},
		{
			name:        "invalid - min function",
			expression:  "min(1, 2, 3) == 1",
			wantErr:     true,
			errContains: "unsupported function",
		},
		{
			name:        "invalid - custom/unknown function",
			expression:  "customFunc($value) == true",
			wantErr:     true,
			errContains: "unsupported function",
		},
		{
			name:        "invalid - mixed valid and invalid functions",
			expression:  "len([1, 2]) > 0 && upper('test') == 'TEST'",
			wantErr:     true,
			errContains: "unsupported function",
		},

		// ============================================
		// Edge cases
		// ============================================
		{
			name:       "valid - function name as string value (not a call)",
			expression: "'len' == 'len'",
			wantErr:    false,
		},
		{
			name:       "valid - function-like substring in variable",
			expression: "$strlen == 5",
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateDeterministicExpression(tt.expression)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateDeterministicExpression() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errContains != "" && err != nil {
				if !containsSubstring(err.Error(), tt.errContains) {
					t.Errorf("ValidateDeterministicExpression() error = %v, want error containing %q", err, tt.errContains)
				}
			}
		})
	}
}

// Helper function
func containsSubstring(s, substr string) bool {
	return strings.Contains(s, substr)
}

// TestValidateNoKeywordOperators tests detection of 'and'/'or' keywords that should be '&&'/'||'
// This is critical for catching common mistakes while avoiding false positives
func TestValidateNoKeywordOperators(t *testing.T) {
	tests := []struct {
		name        string
		expression  string
		wantErr     bool
		errContains string
	}{
		// ============================================
		// SHOULD ERROR - Unsupported 'and' keyword
		// ============================================
		{
			name:        "and keyword - simple case",
			expression:  "x == true and y == false",
			wantErr:     true,
			errContains: "and",
		},
		{
			name:        "and keyword - at start",
			expression:  "and x == true",
			wantErr:     true,
			errContains: "and",
		},
		{
			name:        "and keyword - at end",
			expression:  "x == true and",
			wantErr:     true,
			errContains: "and",
		},
		{
			name:        "and keyword - with parentheses",
			expression:  "(x == true) and (y == false)",
			wantErr:     true,
			errContains: "and",
		},
		{
			name:        "and keyword - inside parentheses",
			expression:  "(x == true and y == false)",
			wantErr:     true,
			errContains: "and",
		},
		{
			name:        "and keyword - mixed with &&",
			expression:  "a == 1 && b == 2 and c == 3",
			wantErr:     true,
			errContains: "and",
		},
		{
			name:        "and keyword - uppercase AND",
			expression:  "x == true AND y == false",
			wantErr:     true,
			errContains: "and",
		},
		{
			name:        "and keyword - mixed case And",
			expression:  "x == true And y == false",
			wantErr:     true,
			errContains: "and",
		},
		{
			name:        "and keyword - result reference",
			expression:  "result[1].isConfirmed == false and result[1].userId != ''",
			wantErr:     true,
			errContains: "and",
		},

		// ============================================
		// SHOULD ERROR - Unsupported 'or' keyword
		// ============================================
		{
			name:        "or keyword - simple case",
			expression:  "x == true or y == false",
			wantErr:     true,
			errContains: "or",
		},
		{
			name:        "or keyword - at start",
			expression:  "or x == true",
			wantErr:     true,
			errContains: "or",
		},
		{
			name:        "or keyword - at end",
			expression:  "x == true or",
			wantErr:     true,
			errContains: "or",
		},
		{
			name:        "or keyword - with parentheses",
			expression:  "(x == true) or (y == false)",
			wantErr:     true,
			errContains: "or",
		},
		{
			name:        "or keyword - uppercase OR",
			expression:  "x == true OR y == false",
			wantErr:     true,
			errContains: "or",
		},
		{
			name:        "or keyword - mixed case Or",
			expression:  "x == true Or y == false",
			wantErr:     true,
			errContains: "or",
		},

		// ============================================
		// SHOULD PASS - False positive prevention: 'and'/'or' inside quoted strings
		// ============================================
		{
			name:       "no error - 'and' in single-quoted string",
			expression: "contains('bread and butter', 'and') == true",
			wantErr:    false,
		},
		{
			name:       "no error - 'and' in double-quoted string",
			expression: `contains("bread and butter", "and") == true`,
			wantErr:    false,
		},
		{
			name:       "no error - 'or' in single-quoted string",
			expression: "contains('yes or no', 'or') == true",
			wantErr:    false,
		},
		{
			name:       "no error - 'or' in double-quoted string",
			expression: `contains("yes or no", "or") == true`,
			wantErr:    false,
		},
		{
			name:       "no error - multiple 'and' in quoted strings",
			expression: "'rock and roll' == 'rock and roll'",
			wantErr:    false,
		},
		{
			name:       "no error - 'and' value comparison",
			expression: "$operator == 'and'",
			wantErr:    false,
		},
		{
			name:       "no error - 'or' value comparison",
			expression: "$operator == 'or'",
			wantErr:    false,
		},

		// ============================================
		// SHOULD PASS - False positive prevention: 'and'/'or' as part of words
		// ============================================
		{
			name:       "no error - 'android' contains 'and'",
			expression: "$platform == 'android'",
			wantErr:    false,
		},
		{
			name:       "no error - 'sandbox' contains 'and'",
			expression: "$environment == 'sandbox'",
			wantErr:    false,
		},
		{
			name:       "no error - 'command' contains 'and'",
			expression: "$command == 'start'",
			wantErr:    false,
		},
		{
			name:       "no error - 'operand' contains 'and'",
			expression: "$operand == 5",
			wantErr:    false,
		},
		{
			name:       "no error - 'random' contains 'and'",
			expression: "$random > 0",
			wantErr:    false,
		},
		{
			name:       "no error - 'mandatory' contains 'and'",
			expression: "$mandatory == true",
			wantErr:    false,
		},
		{
			name:       "no error - 'standard' contains 'and'",
			expression: "$standard == 'ISO'",
			wantErr:    false,
		},
		{
			name:       "no error - 'brand' contains 'and'",
			expression: "$brand == 'Nike'",
			wantErr:    false,
		},
		{
			name:       "no error - 'candid' contains 'and'",
			expression: "$candid == true",
			wantErr:    false,
		},
		{
			name:       "no error - 'operator' contains 'or'",
			expression: "$operator == '+'",
			wantErr:    false,
		},
		{
			name:       "no error - 'order' contains 'or'",
			expression: "$order == 'asc'",
			wantErr:    false,
		},
		{
			name:       "no error - 'factor' contains 'or'",
			expression: "$factor > 1",
			wantErr:    false,
		},
		{
			name:       "no error - 'memory' contains 'or'",
			expression: "$memory == 'high'",
			wantErr:    false,
		},
		{
			name:       "no error - 'category' contains 'or'",
			expression: "$category == 'sports'",
			wantErr:    false,
		},
		{
			name:       "no error - 'priority' contains 'or'",
			expression: "$priority == 'high'",
			wantErr:    false,
		},
		{
			name:       "no error - 'error' contains 'or'",
			expression: "$error == false",
			wantErr:    false,
		},
		{
			name:       "no error - 'vendor' contains 'or'",
			expression: "$vendor == 'Apple'",
			wantErr:    false,
		},
		{
			name:       "no error - 'for' contains 'or' at end",
			expression: "$for == 'testing'",
			wantErr:    false,
		},
		{
			name:       "no error - 'coordinator' contains 'or'",
			expression: "$coordinator == 'John'",
			wantErr:    false,
		},

		// ============================================
		// SHOULD PASS - False positive prevention: field names
		// ============================================
		{
			name:       "no error - field name 'operand'",
			expression: "result[1].operand == 5",
			wantErr:    false,
		},
		{
			name:       "no error - field name 'commander'",
			expression: "$result.commander == 'active'",
			wantErr:    false,
		},
		{
			name:       "no error - field name 'orderStatus'",
			expression: "$result.orderStatus == 'pending'",
			wantErr:    false,
		},
		{
			name:       "no error - field name 'android'",
			expression: "$config.android == true",
			wantErr:    false,
		},

		// ============================================
		// SHOULD PASS - Valid expressions with && and ||
		// ============================================
		{
			name:       "no error - && operator",
			expression: "x == true && y == false",
			wantErr:    false,
		},
		{
			name:       "no error - || operator",
			expression: "x == true || y == false",
			wantErr:    false,
		},
		{
			name:       "no error - complex && and ||",
			expression: "(a == 1 && b == 2) || (c == 3 && d == 4)",
			wantErr:    false,
		},
		{
			name:       "no error - result reference with &&",
			expression: "result[1].isConfirmed == false && result[1].userId != ''",
			wantErr:    false,
		},

		// ============================================
		// Edge cases
		// ============================================
		{
			name:       "no error - empty expression",
			expression: "",
			wantErr:    false,
		},
		{
			name:       "no error - single comparison",
			expression: "x == 1",
			wantErr:    false,
		},
		{
			name:        "and keyword - no spaces but with parens",
			expression:  "(x)and(y)",
			wantErr:     true,
			errContains: "and",
		},
		{
			name:        "or keyword - no spaces but with parens",
			expression:  "(x)or(y)",
			wantErr:     true,
			errContains: "or",
		},
		{
			name:       "no error - 'andromeda' starts with 'and'",
			expression: "$galaxy == 'andromeda'",
			wantErr:    false,
		},
		{
			name:       "no error - 'orlando' contains 'or'",
			expression: "$city == 'orlando'",
			wantErr:    false,
		},
		{
			name:       "no error - nested quotes with and",
			expression: `contains("it's bread and butter", 'and')`,
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateNoKeywordOperators(tt.expression)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateNoKeywordOperators(%q) error = %v, wantErr %v", tt.expression, err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errContains != "" && err != nil {
				if !containsSubstring(strings.ToLower(err.Error()), tt.errContains) {
					t.Errorf("ValidateNoKeywordOperators() error = %v, want error containing %q", err, tt.errContains)
				}
			}
		})
	}
}
