package skill

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestValidateNoQuotedSQLVariables_SingleQuotes tests detection of single-quoted variables
func TestValidateNoQuotedSQLVariables_SingleQuotes(t *testing.T) {
	tool := Tool{
		Name: "TestTool",
		Functions: []Function{
			{
				Name:      "testFunc",
				Operation: OperationDB,
				Steps: []Step{
					{
						Name:   "insert_step",
						Action: StepActionWrite,
						With: map[string]interface{}{
							"write": "INSERT INTO users (name, email) VALUES ('$userName', '$userEmail')",
						},
					},
				},
			},
		},
	}

	err := validateNoQuotedSQLVariables(tool)

	assert.Error(t, err, "Should return error for quoted variables")
	assert.True(t, strings.Contains(err.Error(), "$userName"), "Should mention $userName")
	assert.True(t, strings.Contains(err.Error(), "Remove the quotes"), "Should explain the fix")
}

// TestValidateNoQuotedSQLVariables_DoubleQuotes tests detection of double-quoted variables
func TestValidateNoQuotedSQLVariables_DoubleQuotes(t *testing.T) {
	tool := Tool{
		Name: "TestTool",
		Functions: []Function{
			{
				Name:      "testFunc",
				Operation: OperationDB,
				Steps: []Step{
					{
						Name:   "insert_step",
						Action: StepActionWrite,
						With: map[string]interface{}{
							"write": `INSERT INTO users (name) VALUES ("$userName")`,
						},
					},
				},
			},
		},
	}

	err := validateNoQuotedSQLVariables(tool)

	assert.Error(t, err, "Should return error for double-quoted variables")
	assert.True(t, strings.Contains(err.Error(), "$userName"), "Should mention $userName")
}

// TestValidateNoQuotedSQLVariables_NoQuotes tests that unquoted variables don't trigger errors
func TestValidateNoQuotedSQLVariables_NoQuotes(t *testing.T) {
	tool := Tool{
		Name: "TestTool",
		Functions: []Function{
			{
				Name:      "testFunc",
				Operation: OperationDB,
				Steps: []Step{
					{
						Name:   "insert_step",
						Action: StepActionWrite,
						With: map[string]interface{}{
							"write": "INSERT INTO users (name, email) VALUES ($userName, $userEmail)",
						},
					},
				},
			},
		},
	}

	err := validateNoQuotedSQLVariables(tool)

	assert.NoError(t, err, "Should not return error for unquoted variables")
}

// TestValidateNoQuotedSQLVariables_SelectQuery tests detection in SELECT queries
func TestValidateNoQuotedSQLVariables_SelectQuery(t *testing.T) {
	tool := Tool{
		Name: "TestTool",
		Functions: []Function{
			{
				Name:      "testFunc",
				Operation: OperationDB,
				Steps: []Step{
					{
						Name:   "select_step",
						Action: StepActionSelect,
						With: map[string]interface{}{
							"select": "SELECT * FROM users WHERE id = '$userId'",
						},
					},
				},
			},
		},
	}

	err := validateNoQuotedSQLVariables(tool)

	assert.Error(t, err, "Should detect quoted variable in SELECT query")
	assert.True(t, strings.Contains(err.Error(), "$userId"), "Should mention $userId")
}

// TestValidateNoQuotedSQLVariables_MixedQuotedAndUnquoted tests mixed usage
func TestValidateNoQuotedSQLVariables_MixedQuotedAndUnquoted(t *testing.T) {
	tool := Tool{
		Name: "TestTool",
		Functions: []Function{
			{
				Name:      "testFunc",
				Operation: OperationDB,
				Steps: []Step{
					{
						Name:   "insert_step",
						Action: StepActionWrite,
						With: map[string]interface{}{
							// $goodVar is unquoted (correct), $badVar is quoted (incorrect)
							"write": "INSERT INTO users (name, email) VALUES ($goodVar, '$badVar')",
						},
					},
				},
			},
		},
	}

	err := validateNoQuotedSQLVariables(tool)

	assert.Error(t, err, "Should detect the quoted variable")
	assert.True(t, strings.Contains(err.Error(), "$badVar"), "Should mention $badVar")
}

// TestValidateNoQuotedSQLVariables_NonDBOperation tests that non-db operations are ignored
func TestValidateNoQuotedSQLVariables_NonDBOperation(t *testing.T) {
	tool := Tool{
		Name: "TestTool",
		Functions: []Function{
			{
				Name:      "testFunc",
				Operation: OperationAPI, // Not a db operation
				Steps: []Step{
					{
						Name:   "api_step",
						Action: "POST",
						With: map[string]interface{}{
							// This isn't SQL, so it should be ignored even if it has quoted vars
							"body": `{"name": "$userName"}`,
						},
					},
				},
			},
		},
	}

	err := validateNoQuotedSQLVariables(tool)

	assert.NoError(t, err, "Should not check non-db operations")
}

// TestValidateNoQuotedSQLVariables_MultipleSteps tests that error is returned on first issue found
func TestValidateNoQuotedSQLVariables_MultipleSteps(t *testing.T) {
	tool := Tool{
		Name: "TestTool",
		Functions: []Function{
			{
				Name:      "testFunc",
				Operation: OperationDB,
				Steps: []Step{
					{
						Name:   "step1",
						Action: StepActionWrite,
						With: map[string]interface{}{
							"write": "INSERT INTO t1 VALUES ('$var1')",
						},
					},
					{
						Name:   "step2",
						Action: StepActionSelect,
						With: map[string]interface{}{
							"select": "SELECT * FROM t2 WHERE x = '$var2'",
						},
					},
					{
						Name:   "step3",
						Action: StepActionWrite,
						With: map[string]interface{}{
							"write": "UPDATE t3 SET y = $var3", // Correct - no quotes
						},
					},
				},
			},
		},
	}

	err := validateNoQuotedSQLVariables(tool)

	assert.Error(t, err, "Should return error on first quoted variable found")
	assert.True(t, strings.Contains(err.Error(), "step1"), "Should mention first problematic step")
}

// TestValidateNoQuotedSQLVariables_MultipleFunctions tests multiple functions
func TestValidateNoQuotedSQLVariables_MultipleFunctions(t *testing.T) {
	tool := Tool{
		Name: "TestTool",
		Functions: []Function{
			{
				Name:      "func1",
				Operation: OperationDB,
				Steps: []Step{
					{
						Name:   "step1",
						Action: StepActionWrite,
						With: map[string]interface{}{
							"write": "INSERT INTO t1 VALUES ($var1)", // Correct
						},
					},
				},
			},
			{
				Name:      "func2",
				Operation: OperationDB,
				Steps: []Step{
					{
						Name:   "step2",
						Action: StepActionWrite,
						With: map[string]interface{}{
							"write": "INSERT INTO t2 VALUES ('$var2')", // Incorrect
						},
					},
				},
			},
		},
	}

	err := validateNoQuotedSQLVariables(tool)

	assert.Error(t, err, "Should detect error in func2")
	assert.True(t, strings.Contains(err.Error(), "func2"), "Should mention func2")
}

// TestValidateNoQuotedSQLVariables_SpecialVariableNames tests various variable name formats
func TestValidateNoQuotedSQLVariables_SpecialVariableNames(t *testing.T) {
	testCases := []struct {
		name      string
		sql       string
		expectErr bool
		varName   string
	}{
		{
			name:      "underscore in name",
			sql:       "INSERT INTO t VALUES ('$user_name')",
			expectErr: true,
			varName:   "$user_name",
		},
		{
			name:      "numbers in name",
			sql:       "INSERT INTO t VALUES ('$var123')",
			expectErr: true,
			varName:   "$var123",
		},
		{
			name:      "camelCase",
			sql:       "INSERT INTO t VALUES ('$userName')",
			expectErr: true,
			varName:   "$userName",
		},
		{
			name:      "all caps - environment variable (allowed)",
			sql:       "INSERT INTO t VALUES ('$USER_ID')",
			expectErr: false, // Environment variables (uppercase) are replaced with literal values, so quoting is OK
			varName:   "",
		},
		{
			name:      "single char lowercase",
			sql:       "INSERT INTO t VALUES ('$x')",
			expectErr: true,
			varName:   "$x",
		},
		{
			name:      "correct usage - no quotes",
			sql:       "INSERT INTO t VALUES ($userName)",
			expectErr: false,
			varName:   "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tool := Tool{
				Name: "TestTool",
				Functions: []Function{
					{
						Name:      "testFunc",
						Operation: OperationDB,
						Steps: []Step{
							{
								Name:   "step",
								Action: StepActionWrite,
								With: map[string]interface{}{
									"write": tc.sql,
								},
							},
						},
					},
				},
			}

			err := validateNoQuotedSQLVariables(tool)

			if tc.expectErr {
				assert.Error(t, err, "Expected error for %s", tc.name)
				assert.True(t, strings.Contains(err.Error(), tc.varName), "Should mention %s", tc.varName)
			} else {
				assert.NoError(t, err, "Should not error for %s", tc.name)
			}
		})
	}
}

// TestValidateNoQuotedSQLVariables_EnvironmentVariableInCoalesce tests that env vars in COALESCE are allowed
func TestValidateNoQuotedSQLVariables_EnvironmentVariableInCoalesce(t *testing.T) {
	tool := Tool{
		Name: "TestTool",
		Functions: []Function{
			{
				Name:      "getUserPreference",
				Operation: OperationDB,
				Steps: []Step{
					{
						Name:   "get_preferred_dentist",
						Action: StepActionSelect,
						With: map[string]interface{}{
							// This is a valid use case: $MALE_DENTIST_ID is an environment variable
							// that gets replaced with its literal value before parameterization.
							// So '$MALE_DENTIST_ID' becomes '6521' which is correct SQL.
							"select": `SELECT COALESCE(
								(SELECT preferred_dentist_id FROM user_preferences WHERE user_id = $userId),
								'$MALE_DENTIST_ID'
							) as dentist_id`,
						},
					},
				},
			},
		},
	}

	err := validateNoQuotedSQLVariables(tool)

	assert.NoError(t, err, "Environment variables (uppercase) should be allowed in quotes because they're replaced with literal values")
}

// TestValidateNoQuotedSQLVariables_InputVariableInCoalesce tests that input vars in COALESCE are NOT allowed
func TestValidateNoQuotedSQLVariables_InputVariableInCoalesce(t *testing.T) {
	tool := Tool{
		Name: "TestTool",
		Functions: []Function{
			{
				Name:      "testFunc",
				Operation: OperationDB,
				Steps: []Step{
					{
						Name:   "select_step",
						Action: StepActionSelect,
						With: map[string]interface{}{
							// This is WRONG: $defaultValue is an input variable (lowercase)
							// that becomes a parameterized placeholder, so '$defaultValue'
							// becomes '?' which is a literal string, not a placeholder.
							"select": `SELECT COALESCE(name, '$defaultValue') FROM users WHERE id = $userId`,
						},
					},
				},
			},
		},
	}

	err := validateNoQuotedSQLVariables(tool)

	assert.Error(t, err, "Input variables (lowercase) should NOT be allowed in quotes")
	assert.True(t, strings.Contains(err.Error(), "$defaultValue"), "Should mention $defaultValue")
}

// TestValidateNoQuotedSQLVariables_NilWith tests handling of nil With block
func TestValidateNoQuotedSQLVariables_NilWith(t *testing.T) {
	tool := Tool{
		Name: "TestTool",
		Functions: []Function{
			{
				Name:      "testFunc",
				Operation: OperationDB,
				Steps: []Step{
					{
						Name:   "step",
						Action: StepActionWrite,
						With:   nil, // No With block
					},
				},
			},
		},
	}

	err := validateNoQuotedSQLVariables(tool)

	assert.NoError(t, err, "Should handle nil With gracefully")
}

// TestValidateNoQuotedSQLVariables_NonStringSQL tests handling of non-string SQL values
func TestValidateNoQuotedSQLVariables_NonStringSQL(t *testing.T) {
	tool := Tool{
		Name: "TestTool",
		Functions: []Function{
			{
				Name:      "testFunc",
				Operation: OperationDB,
				Steps: []Step{
					{
						Name:   "step",
						Action: StepActionWrite,
						With: map[string]interface{}{
							"write": 123, // Not a string
						},
					},
				},
			},
		},
	}

	err := validateNoQuotedSQLVariables(tool)

	assert.NoError(t, err, "Should handle non-string SQL gracefully")
}

// TestValidateNoQuotedSQLVariables_EmptyTool tests handling of empty tool
func TestValidateNoQuotedSQLVariables_EmptyTool(t *testing.T) {
	tool := Tool{
		Name:      "TestTool",
		Functions: []Function{},
	}

	err := validateNoQuotedSQLVariables(tool)

	assert.NoError(t, err, "Should handle empty functions list")
}

// TestValidateNoQuotedSQLVariables_ErrorMessage tests the error message format
func TestValidateNoQuotedSQLVariables_ErrorMessage(t *testing.T) {
	tool := Tool{
		Name: "TestTool",
		Functions: []Function{
			{
				Name:      "registerKpi",
				Operation: OperationDB,
				Steps: []Step{
					{
						Name:   "insert_kpi",
						Action: StepActionWrite,
						With: map[string]interface{}{
							"write": "INSERT INTO kpi_events VALUES ('$eventType')",
						},
					},
				},
			},
		},
	}

	err := validateNoQuotedSQLVariables(tool)

	assert.Error(t, err)
	errMsg := err.Error()

	// Check all parts of the error message
	assert.True(t, strings.Contains(errMsg, "registerKpi"), "Should mention function name")
	assert.True(t, strings.Contains(errMsg, "insert_kpi"), "Should mention step name")
	assert.True(t, strings.Contains(errMsg, "$eventType"), "Should mention variable name")
	assert.True(t, strings.Contains(errMsg, "literal string '?'"), "Should explain the problem")
	assert.True(t, strings.Contains(errMsg, "Remove the quotes"), "Should suggest the fix")
	assert.True(t, strings.Contains(errMsg, "VALUES($eventType"), "Should show correct example")
	assert.True(t, strings.Contains(errMsg, "VALUES('$eventType'"), "Should show incorrect example")
}

// TestValidateNoQuotedSQLVariables_Integration tests with CreateToolWithWarnings
func TestValidateNoQuotedSQLVariables_Integration(t *testing.T) {
	yamlWithQuotedVars := `
version: "v1"
author: "Test"
tools:
  - name: "TestTool"
    description: "Test tool"
    version: "1.0.0"
    functions:
      - name: "badFunc"
        operation: "db"
        description: "Function with quoted SQL variables"
        triggers:
          - type: "flex_for_user"
        with:
          init: |
            CREATE TABLE IF NOT EXISTS test_table (
              id INTEGER PRIMARY KEY,
              name TEXT
            );
        input:
          - name: "userName"
            description: "The user name"
            value: "test"
        steps:
          - name: "insert"
            action: "write"
            with:
              write: |
                INSERT INTO test_table (name) VALUES ('$userName')
`

	_, err := CreateToolWithWarnings(yamlWithQuotedVars)

	assert.Error(t, err, "Should fail validation with quoted variables")
	assert.True(t, strings.Contains(err.Error(), "$userName"), "Error should mention the variable")
	assert.True(t, strings.Contains(err.Error(), "quotes"), "Error should mention quotes")
}

// TestValidateNoQuotedSQLVariables_NoErrorForCorrectUsage tests correct usage produces no error
func TestValidateNoQuotedSQLVariables_NoErrorForCorrectUsage(t *testing.T) {
	yamlCorrect := `
version: "v1"
author: "Test"
tools:
  - name: "TestTool"
    description: "Test tool"
    version: "1.0.0"
    functions:
      - name: "goodFunc"
        operation: "db"
        description: "Function with correct SQL variables"
        triggers:
          - type: "flex_for_user"
        with:
          init: |
            CREATE TABLE IF NOT EXISTS test_table (
              id INTEGER PRIMARY KEY,
              name TEXT
            );
        input:
          - name: "userName"
            description: "The user name"
            value: "test"
        steps:
          - name: "insert"
            action: "write"
            with:
              write: |
                INSERT INTO test_table (name) VALUES ($userName)
`

	result, err := CreateToolWithWarnings(yamlCorrect)

	assert.NoError(t, err, "Should parse successfully with correct usage")
	assert.NotNil(t, result.Tool, "Should return valid tool")
}

// TestValidateNoQuotedSQLVariables_CreateToolReturnsError tests CreateTool also returns error
func TestValidateNoQuotedSQLVariables_CreateToolReturnsError(t *testing.T) {
	yamlWithQuotedVars := `
version: "v1"
author: "Test"
tools:
  - name: "TestTool"
    description: "Test tool"
    version: "1.0.0"
    functions:
      - name: "badFunc"
        operation: "db"
        description: "Function with quoted SQL variables"
        triggers:
          - type: "flex_for_user"
        with:
          init: |
            CREATE TABLE IF NOT EXISTS test_table (id INTEGER PRIMARY KEY, name TEXT);
        input:
          - name: "userName"
            description: "The user name"
            value: "test"
        steps:
          - name: "insert"
            action: "write"
            with:
              write: "INSERT INTO test_table (name) VALUES ('$userName')"
`

	_, err := CreateTool(yamlWithQuotedVars)

	assert.Error(t, err, "CreateTool should also return error for quoted variables")
	assert.True(t, strings.Contains(err.Error(), "$userName"), "Error should mention the variable")
}
