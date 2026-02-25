package skill

import (
	"strings"
	"testing"
)

// TestOptionalInputsWithoutOriginOrValue tests the caller injection pattern:
// Optional inputs can have no origin and no value, expecting values to be injected
// via params from onSuccess/onFailure/needs callers. If not injected, the input
// is treated as NULL (DB ops) or empty string (terminal ops).
func TestOptionalInputsWithoutOriginOrValue(t *testing.T) {
	testCases := []struct {
		name            string
		yamlInput       string
		expectError     bool
		errorContains   string
		expectWarning   bool
		warningContains string
	}{
		// Case 1: Pass - Optional input without origin/value (caller injection pattern)
		{
			name: "Pass: Optional input without origin/value generates warning",
			yamlInput: `
version: "v1"
author: "Test"
tools:
  - name: "TestTool"
    description: "Test tool"
    version: "1.0.0"
    functions:
      - name: "markConversationAsImportant"
        operation: "db"
        description: "Mark conversation as important for follow-up tracking"
        triggers:
          - type: "flex_for_user"
        with:
          init: |
            CREATE TABLE IF NOT EXISTS important_tracker (
              user_id TEXT PRIMARY KEY,
              code TEXT,
              reason TEXT
            );
        input:
          - name: "userId"
            description: "User's WhatsApp ID"
            value: "$USER.id"
          - name: "code"
            description: "Reference code (e.g., deal ID)"
            isOptional: true
          - name: "reason"
            description: "Reason for importance"
            isOptional: true
        steps:
          - name: "insert"
            action: "write"
            with:
              write: |
                INSERT INTO important_tracker (user_id, code, reason)
                VALUES ($userId, $code, $reason)
`,
			expectError:     false,
			expectWarning:   true,
			warningContains: "caller injection pattern",
		},

		// Case 2: Error - Required input without origin/value should fail
		{
			name: "Error: Required input without origin/value fails validation",
			yamlInput: `
version: "v1"
author: "Test"
tools:
  - name: "TestTool"
    description: "Test tool"
    version: "1.0.0"
    functions:
      - name: "testFunction"
        operation: "db"
        description: "Test function"
        triggers:
          - type: "flex_for_user"
        with:
          init: "CREATE TABLE IF NOT EXISTS test (id TEXT PRIMARY KEY)"
        input:
          - name: "requiredInput"
            description: "This is required"
        steps:
          - name: "insert"
            action: "write"
            with:
              write: "INSERT INTO test VALUES ($requiredInput)"
`,
			expectError:   true,
			errorContains: "must have an origin or a static value",
		},

		// Case 3: Pass - Optional input with value (no warning)
		{
			name: "Pass: Optional input with value generates no warning",
			yamlInput: `
version: "v1"
author: "Test"
tools:
  - name: "TestTool"
    description: "Test tool"
    version: "1.0.0"
    functions:
      - name: "testFunction"
        operation: "db"
        description: "Test function"
        triggers:
          - type: "flex_for_user"
        with:
          init: "CREATE TABLE IF NOT EXISTS test (id TEXT PRIMARY KEY)"
        input:
          - name: "optionalWithValue"
            description: "Optional with default value"
            value: "default_value"
            isOptional: true
        steps:
          - name: "insert"
            action: "write"
            with:
              write: "INSERT INTO test VALUES ($optionalWithValue)"
`,
			expectError:   false,
			expectWarning: false,
		},

		// Case 4: Pass - Optional input with value defined (no caller injection warning)
		{
			name: "Pass: Optional input with value generates no caller injection warning",
			yamlInput: `
version: "v1"
author: "Test"
tools:
  - name: "TestTool"
    description: "Test tool"
    version: "1.0.0"
    functions:
      - name: "testFunction"
        operation: "db"
        description: "Test function"
        triggers:
          - type: "flex_for_user"
        with:
          init: "CREATE TABLE IF NOT EXISTS test (id TEXT PRIMARY KEY)"
        input:
          - name: "optionalWithDefault"
            description: "Optional with default"
            value: "fallback_value"
            isOptional: true
        steps:
          - name: "insert"
            action: "write"
            with:
              write: "INSERT INTO test VALUES ($optionalWithDefault)"
`,
			expectError:   false,
			expectWarning: false,
		},

		// Case 5: Pass - Multiple optional inputs without origin/value
		{
			name: "Pass: Multiple optional inputs without origin/value",
			yamlInput: `
version: "v1"
author: "Test"
tools:
  - name: "TestTool"
    description: "Test tool"
    version: "1.0.0"
    functions:
      - name: "testFunction"
        operation: "db"
        description: "Test function"
        triggers:
          - type: "flex_for_user"
        with:
          init: "CREATE TABLE IF NOT EXISTS test (id TEXT, a TEXT, b TEXT, c TEXT)"
        input:
          - name: "id"
            description: "User ID"
            value: "$USER.id"
          - name: "optA"
            description: "Optional A"
            isOptional: true
          - name: "optB"
            description: "Optional B"
            isOptional: true
          - name: "optC"
            description: "Optional C"
            isOptional: true
        steps:
          - name: "insert"
            action: "write"
            with:
              write: "INSERT INTO test VALUES ($id, $optA, $optB, $optC)"
`,
			expectError:     false,
			expectWarning:   true,
			warningContains: "optA",
		},

		// Case 6: Pass - Terminal operation with optional inputs
		{
			name: "Pass: Terminal operation with optional inputs",
			yamlInput: `
version: "v1"
author: "Test"
tools:
  - name: "TestTool"
    description: "Test tool"
    version: "1.0.0"
    functions:
      - name: "runCommand"
        operation: "terminal"
        description: "Run a command with optional flags"
        triggers:
          - type: "flex_for_team"
        input:
          - name: "command"
            description: "Base command"
            value: "echo"
          - name: "flags"
            description: "Optional flags"
            isOptional: true
        steps:
          - name: "execute"
            action: "bash"
            resultIndex: 1
            with:
              linux: "$command $flags"
              windows: "$command $flags"
        output:
          type: "string"
          value: "result[1]"
`,
			expectError:     false,
			expectWarning:   true,
			warningContains: "flags",
		},

		// Case 7: DB operation with optional inputs called via onSuccess
		{
			name: "Pass: DB operation with optional inputs used via onSuccess",
			yamlInput: `
version: "v1"
author: "Test"
tools:
  - name: "TestTool"
    description: "Test tool"
    version: "1.0.0"
    functions:
      - name: "entry"
        operation: "terminal"
        description: "Entry point"
        triggers:
          - type: "flex_for_team"
        onSuccess:
          - name: "saveData"
            params:
              optionalField: "injected_value"
        steps:
          - name: "run"
            action: "bash"
            resultIndex: 1
            with:
              linux: "echo ok"
              windows: "echo ok"
        output:
          type: "string"
          value: "result[1]"
      - name: "saveData"
        operation: "db"
        description: "Save data with optional field"
        with:
          init: "CREATE TABLE IF NOT EXISTS data (id TEXT, optional_field TEXT)"
        input:
          - name: "id"
            description: "ID"
            value: "$USER.id"
          - name: "optionalField"
            description: "Optional field to inject"
            isOptional: true
        steps:
          - name: "insert"
            action: "write"
            with:
              write: "INSERT INTO data VALUES ($id, $optionalField)"
`,
			expectError:     false,
			expectWarning:   true,
			warningContains: "optionalField",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := CreateToolWithWarnings(tc.yamlInput)
			hasError := err != nil

			// Check error expectations
			if tc.expectError && !hasError {
				t.Errorf("expected error but got none. Warnings: %v", result.Warnings)
			}
			if !tc.expectError && hasError {
				t.Errorf("unexpected error: %v", err)
			}
			if tc.expectError && tc.errorContains != "" && err != nil {
				if !strings.Contains(err.Error(), tc.errorContains) {
					t.Errorf("expected error containing '%s', got: %v", tc.errorContains, err)
				}
			}

			// Check warning expectations
			if tc.expectWarning {
				hasWarning := false
				for _, w := range result.Warnings {
					if strings.Contains(w, tc.warningContains) {
						hasWarning = true
						break
					}
				}
				if !hasWarning {
					t.Errorf("expected warning containing '%s', got warnings: %v", tc.warningContains, result.Warnings)
				}
			}

			// Check no warning when not expected
			if !tc.expectWarning && !tc.expectError {
				hasCallerInjectionWarning := false
				for _, w := range result.Warnings {
					if strings.Contains(w, "caller injection pattern") {
						hasCallerInjectionWarning = true
						break
					}
				}
				if hasCallerInjectionWarning {
					t.Errorf("unexpected caller injection warning: %v", result.Warnings)
				}
			}
		})
	}
}

// TestCallerInjectionPatternIntegration tests the integration of caller injection:
// When a function with optional inputs is called via onSuccess with params,
// the values should be injectable.
func TestCallerInjectionPatternIntegration(t *testing.T) {
	testCases := []struct {
		name          string
		yamlInput     string
		expectError   bool
		errorContains string
	}{
		// Case 1: Valid - onSuccess provides params to function with optional inputs
		{
			name: "Pass: onSuccess provides params to function with optional inputs",
			yamlInput: `
version: "v1"
author: "Test"
tools:
  - name: "TestTool"
    description: "Test tool"
    version: "1.0.0"
    functions:
      - name: "getDealInfo"
        operation: "terminal"
        description: "Get deal information"
        triggers:
          - type: "flex_for_user"
        onSuccess:
          - name: "markConversation"
            params:
              code: "$dealId"
              reason: "deal_inquiry"
        input:
          - name: "dealId"
            description: "Deal ID"
            value: "test-deal-123"
        steps:
          - name: "fetch"
            action: "bash"
            resultIndex: 1
            with:
              linux: "echo $dealId"
              windows: "echo %dealId%"
        output:
          type: "string"
          value: "result[1]"
      - name: "markConversation"
        operation: "db"
        description: "Mark conversation"
        with:
          init: "CREATE TABLE IF NOT EXISTS tracker (user_id TEXT, code TEXT, reason TEXT)"
        input:
          - name: "userId"
            description: "User ID"
            value: "$USER.id"
          - name: "code"
            description: "Reference code"
            isOptional: true
          - name: "reason"
            description: "Reason"
            isOptional: true
        steps:
          - name: "insert"
            action: "write"
            with:
              write: "INSERT INTO tracker VALUES ($userId, $code, $reason)"
`,
			expectError: false,
		},

		// Case 2: Valid - needs provides params to function with optional inputs
		{
			name: "Pass: needs provides params to function with optional inputs",
			yamlInput: `
version: "v1"
author: "Test"
tools:
  - name: "TestTool"
    description: "Test tool"
    version: "1.0.0"
    functions:
      - name: "processOrder"
        operation: "terminal"
        description: "Process an order"
        triggers:
          - type: "flex_for_user"
        needs:
          - name: "trackOrder"
            params:
              orderId: "$orderId"
        input:
          - name: "orderId"
            description: "Order ID"
            value: "order-123"
        steps:
          - name: "process"
            action: "bash"
            resultIndex: 1
            with:
              linux: "echo processing"
              windows: "echo processing"
        output:
          type: "string"
          value: "result[1]"
      - name: "trackOrder"
        operation: "db"
        description: "Track order"
        with:
          init: "CREATE TABLE IF NOT EXISTS orders (id TEXT, extra TEXT)"
        input:
          - name: "orderId"
            description: "Order ID"
            isOptional: true
          - name: "extra"
            description: "Extra info"
            isOptional: true
        steps:
          - name: "insert"
            action: "write"
            with:
              write: "INSERT INTO orders VALUES ($orderId, $extra)"
`,
			expectError: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := CreateToolWithWarnings(tc.yamlInput)
			hasError := err != nil

			if tc.expectError && !hasError {
				t.Errorf("expected error but got none. Warnings: %v", result.Warnings)
			}
			if !tc.expectError && hasError {
				t.Errorf("unexpected error: %v", err)
			}
			if tc.expectError && tc.errorContains != "" && err != nil {
				if !strings.Contains(err.Error(), tc.errorContains) {
					t.Errorf("expected error containing '%s', got: %v", tc.errorContains, err)
				}
			}
		})
	}
}
