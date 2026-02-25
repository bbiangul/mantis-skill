package skill

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseMigrations(t *testing.T) {
	tests := []struct {
		name          string
		withBlock     map[string]interface{}
		expectError   bool
		errorContains string
		expectedCount int
		expectedFirst *Migration
	}{
		{
			name:          "Nil with block - no migrations",
			withBlock:     nil,
			expectError:   false,
			expectedCount: 0,
		},
		{
			name:          "With block without migrations - no migrations",
			withBlock:     map[string]interface{}{"init": "CREATE TABLE test (id INT)"},
			expectError:   false,
			expectedCount: 0,
		},
		{
			name: "Single migration",
			withBlock: map[string]interface{}{
				"migrations": []interface{}{
					map[string]interface{}{"version": 1, "sql": "ALTER TABLE test ADD COLUMN name TEXT"},
				},
			},
			expectError:   false,
			expectedCount: 1,
			expectedFirst: &Migration{Version: 1, SQL: "ALTER TABLE test ADD COLUMN name TEXT"},
		},
		{
			name: "Multiple migrations in order",
			withBlock: map[string]interface{}{
				"migrations": []interface{}{
					map[string]interface{}{"version": 1, "sql": "ALTER TABLE test ADD COLUMN a TEXT"},
					map[string]interface{}{"version": 2, "sql": "ALTER TABLE test ADD COLUMN b TEXT"},
					map[string]interface{}{"version": 3, "sql": "ALTER TABLE test ADD COLUMN c TEXT"},
				},
			},
			expectError:   false,
			expectedCount: 3,
			expectedFirst: &Migration{Version: 1, SQL: "ALTER TABLE test ADD COLUMN a TEXT"},
		},
		{
			name: "Version as float64 (YAML parsing)",
			withBlock: map[string]interface{}{
				"migrations": []interface{}{
					map[string]interface{}{"version": float64(1), "sql": "ALTER TABLE test ADD COLUMN name TEXT"},
				},
			},
			expectError:   false,
			expectedCount: 1,
			expectedFirst: &Migration{Version: 1, SQL: "ALTER TABLE test ADD COLUMN name TEXT"},
		},
		{
			name: "map[interface{}]interface{} type (YAML parsing)",
			withBlock: map[string]interface{}{
				"migrations": []interface{}{
					map[interface{}]interface{}{"version": 1, "sql": "ALTER TABLE test ADD COLUMN name TEXT"},
				},
			},
			expectError:   false,
			expectedCount: 1,
			expectedFirst: &Migration{Version: 1, SQL: "ALTER TABLE test ADD COLUMN name TEXT"},
		},
		{
			name: "Missing version field",
			withBlock: map[string]interface{}{
				"migrations": []interface{}{
					map[string]interface{}{"sql": "ALTER TABLE test ADD COLUMN name TEXT"},
				},
			},
			expectError:   true,
			errorContains: "must have 'version' and 'sql' fields",
		},
		{
			name: "Missing sql field",
			withBlock: map[string]interface{}{
				"migrations": []interface{}{
					map[string]interface{}{"version": 1},
				},
			},
			expectError:   true,
			errorContains: "must have 'version' and 'sql' fields",
		},
		{
			name: "Version zero is invalid",
			withBlock: map[string]interface{}{
				"migrations": []interface{}{
					map[string]interface{}{"version": 0, "sql": "ALTER TABLE test ADD COLUMN name TEXT"},
				},
			},
			expectError:   true,
			errorContains: "must be a positive integer",
		},
		{
			name: "Negative version is invalid",
			withBlock: map[string]interface{}{
				"migrations": []interface{}{
					map[string]interface{}{"version": -1, "sql": "ALTER TABLE test ADD COLUMN name TEXT"},
				},
			},
			expectError:   true,
			errorContains: "must be a positive integer",
		},
		{
			name: "Empty SQL is invalid",
			withBlock: map[string]interface{}{
				"migrations": []interface{}{
					map[string]interface{}{"version": 1, "sql": ""},
				},
			},
			expectError:   true,
			errorContains: "must be a non-empty string",
		},
		{
			name: "Duplicate version in same function",
			withBlock: map[string]interface{}{
				"migrations": []interface{}{
					map[string]interface{}{"version": 1, "sql": "ALTER TABLE test ADD COLUMN a TEXT"},
					map[string]interface{}{"version": 1, "sql": "ALTER TABLE test ADD COLUMN b TEXT"},
				},
			},
			expectError:   true,
			errorContains: "duplicate migration version 1",
		},
		{
			name: "migrations is not a list",
			withBlock: map[string]interface{}{
				"migrations": "not a list",
			},
			expectError:   true,
			errorContains: "must be a list",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			migrations, err := parseMigrations(tt.withBlock, "testFunc", "testTool")

			if tt.expectError {
				require.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				require.NoError(t, err)
				assert.Len(t, migrations, tt.expectedCount)
				if tt.expectedFirst != nil && len(migrations) > 0 {
					assert.Equal(t, tt.expectedFirst.Version, migrations[0].Version)
					assert.Equal(t, tt.expectedFirst.SQL, migrations[0].SQL)
				}
			}
		})
	}
}

func TestValidateToolMigrations(t *testing.T) {
	tests := []struct {
		name          string
		tools         []Tool
		isSystemTool  bool
		expectError   bool
		errorContains string
	}{
		{
			name:         "No tools - valid",
			tools:        []Tool{},
			isSystemTool: false,
			expectError:  false,
		},
		{
			name: "No migrations - valid",
			tools: []Tool{
				{
					Name: "TestTool",
					Functions: []Function{
						{Name: "func1", With: map[string]interface{}{"init": "CREATE TABLE test (id INT)"}},
					},
				},
			},
			isSystemTool: false,
			expectError:  false,
		},
		{
			name: "Single function with migrations - valid",
			tools: []Tool{
				{
					Name: "TestTool",
					Functions: []Function{
						{
							Name: "func1",
							With: map[string]interface{}{
								"migrations": []interface{}{
									map[string]interface{}{"version": 1, "sql": "ALTER TABLE test ADD COLUMN a TEXT"},
								},
							},
						},
					},
				},
			},
			isSystemTool: false,
			expectError:  false,
		},
		{
			name: "Same version same SQL across functions - valid",
			tools: []Tool{
				{
					Name: "TestTool",
					Functions: []Function{
						{
							Name: "func1",
							With: map[string]interface{}{
								"migrations": []interface{}{
									map[string]interface{}{"version": 1, "sql": "ALTER TABLE test ADD COLUMN a TEXT"},
								},
							},
						},
						{
							Name: "func2",
							With: map[string]interface{}{
								"migrations": []interface{}{
									map[string]interface{}{"version": 1, "sql": "ALTER TABLE test ADD COLUMN a TEXT"},
								},
							},
						},
					},
				},
			},
			isSystemTool: false,
			expectError:  false,
		},
		{
			name: "Same version different SQL across functions - invalid",
			tools: []Tool{
				{
					Name: "TestTool",
					Functions: []Function{
						{
							Name: "func1",
							With: map[string]interface{}{
								"migrations": []interface{}{
									map[string]interface{}{"version": 1, "sql": "ALTER TABLE test ADD COLUMN a TEXT"},
								},
							},
						},
						{
							Name: "func2",
							With: map[string]interface{}{
								"migrations": []interface{}{
									map[string]interface{}{"version": 1, "sql": "ALTER TABLE test ADD COLUMN b TEXT"},
								},
							},
						},
					},
				},
			},
			isSystemTool:  false,
			expectError:   true,
			errorContains: "conflicting migration v1",
		},
		{
			name: "System tool skips validation - valid even with conflicts",
			tools: []Tool{
				{
					Name: "TestTool",
					Functions: []Function{
						{
							Name: "func1",
							With: map[string]interface{}{
								"migrations": []interface{}{
									map[string]interface{}{"version": 1, "sql": "ALTER TABLE test ADD COLUMN a TEXT"},
								},
							},
						},
						{
							Name: "func2",
							With: map[string]interface{}{
								"migrations": []interface{}{
									map[string]interface{}{"version": 1, "sql": "ALTER TABLE test ADD COLUMN b TEXT"},
								},
							},
						},
					},
				},
			},
			isSystemTool: true,
			expectError:  false,
		},
		{
			name: "Different versions across functions - valid",
			tools: []Tool{
				{
					Name: "TestTool",
					Functions: []Function{
						{
							Name: "func1",
							With: map[string]interface{}{
								"migrations": []interface{}{
									map[string]interface{}{"version": 1, "sql": "ALTER TABLE test ADD COLUMN a TEXT"},
								},
							},
						},
						{
							Name: "func2",
							With: map[string]interface{}{
								"migrations": []interface{}{
									map[string]interface{}{"version": 2, "sql": "ALTER TABLE test ADD COLUMN b TEXT"},
								},
							},
						},
					},
				},
			},
			isSystemTool: false,
			expectError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateToolMigrations(tt.tools, tt.isSystemTool)

			if tt.expectError {
				require.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestCreateToolWithMigrations(t *testing.T) {
	yaml := `
version: v1
author: TestAuthor
tools:
  - name: "TestTool"
    version: "1.0.0"
    description: "A test tool with migrations"
    functions:
      - name: "TestFunc"
        operation: "db"
        description: "Test function with migrations"
        triggers:
          - type: "flex_for_user"
        with:
          init: |
            CREATE TABLE IF NOT EXISTS users (
              id INTEGER PRIMARY KEY,
              name TEXT NOT NULL
            );
          migrations:
            - version: 1
              sql: "ALTER TABLE users ADD COLUMN email TEXT"
            - version: 2
              sql: "CREATE INDEX IF NOT EXISTS idx_users_email ON users(email)"
        steps:
          - name: "getUsers"
            action: "select"
            with:
              select: "SELECT * FROM users LIMIT 10"
        output:
          type: "string"
          value: "done"
`

	tool, err := CreateTool(yaml)
	require.NoError(t, err)
	assert.Equal(t, "TestTool", tool.Tools[0].Name)
	assert.Len(t, tool.Tools[0].Functions, 1)

	// Verify migrations are in the With block
	fn := tool.Tools[0].Functions[0]
	require.NotNil(t, fn.With)
	require.NotNil(t, fn.With[WithMigrations])

	migrations, err := parseMigrations(fn.With, fn.Name, tool.Tools[0].Name)
	require.NoError(t, err)
	assert.Len(t, migrations, 2)
	assert.Equal(t, 1, migrations[0].Version)
	assert.Equal(t, "ALTER TABLE users ADD COLUMN email TEXT", migrations[0].SQL)
	assert.Equal(t, 2, migrations[1].Version)
	assert.Contains(t, migrations[1].SQL, "CREATE INDEX")
}

func TestCreateToolWithConflictingMigrations(t *testing.T) {
	yaml := `
version: v1
author: TestAuthor
tools:
  - name: "TestTool"
    version: "1.0.0"
    description: "A test tool with conflicting migrations"
    functions:
      - name: "Func1"
        operation: "db"
        description: "First function"
        triggers:
          - type: "flex_for_user"
        with:
          init: |
            CREATE TABLE IF NOT EXISTS users (id INTEGER PRIMARY KEY);
          migrations:
            - version: 1
              sql: "ALTER TABLE users ADD COLUMN a TEXT"
        steps:
          - name: "select"
            action: "select"
            with:
              select: "SELECT * FROM users"
        output:
          type: "string"
          value: "done"
      - name: "Func2"
        operation: "db"
        description: "Second function with conflicting migration"
        triggers:
          - type: "flex_for_user"
        with:
          init: |
            CREATE TABLE IF NOT EXISTS users (id INTEGER PRIMARY KEY);
          migrations:
            - version: 1
              sql: "ALTER TABLE users ADD COLUMN b TEXT"
        steps:
          - name: "select"
            action: "select"
            with:
              select: "SELECT * FROM users"
        output:
          type: "string"
          value: "done"
`

	_, err := CreateTool(yaml)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "conflicting migration v1")
}

// Tests for validateInitAndMigrationsTogether - ephemeral DB validation
func TestValidateInitAndMigrationsTogether(t *testing.T) {
	tests := []struct {
		name          string
		tools         []Tool
		isSystemTool  bool
		expectError   bool
		errorContains string
	}{
		{
			name:         "No tools - valid",
			tools:        []Tool{},
			isSystemTool: false,
			expectError:  false,
		},
		{
			name: "No migrations - skips validation",
			tools: []Tool{
				{
					Name: "TestTool",
					Functions: []Function{
						{
							Name: "func1",
							With: map[string]interface{}{
								"init": "CREATE TABLE IF NOT EXISTS users (id INTEGER PRIMARY KEY, name TEXT);",
							},
						},
					},
				},
			},
			isSystemTool: false,
			expectError:  false,
		},
		{
			name: "Init + valid migration - column not in init",
			tools: []Tool{
				{
					Name: "TestTool",
					Functions: []Function{
						{
							Name: "func1",
							With: map[string]interface{}{
								"init": "CREATE TABLE IF NOT EXISTS users (id INTEGER PRIMARY KEY, name TEXT);",
								"migrations": []interface{}{
									map[string]interface{}{"version": 1, "sql": "ALTER TABLE users ADD COLUMN email TEXT"},
								},
							},
						},
					},
				},
			},
			isSystemTool: false,
			expectError:  false,
		},
		{
			name: "Init + multiple valid migrations in order",
			tools: []Tool{
				{
					Name: "TestTool",
					Functions: []Function{
						{
							Name: "func1",
							With: map[string]interface{}{
								"init": "CREATE TABLE IF NOT EXISTS users (id INTEGER PRIMARY KEY, name TEXT);",
								"migrations": []interface{}{
									map[string]interface{}{"version": 1, "sql": "ALTER TABLE users ADD COLUMN email TEXT"},
									map[string]interface{}{"version": 2, "sql": "ALTER TABLE users ADD COLUMN phone TEXT"},
									map[string]interface{}{"version": 3, "sql": "CREATE INDEX IF NOT EXISTS idx_email ON users(email)"},
								},
							},
						},
					},
				},
			},
			isSystemTool: false,
			expectError:  false,
		},
		{
			name: "Migration adds column that already exists in init - duplicate column error",
			tools: []Tool{
				{
					Name: "TestTool",
					Functions: []Function{
						{
							Name: "func1",
							With: map[string]interface{}{
								"init": "CREATE TABLE IF NOT EXISTS users (id INTEGER PRIMARY KEY, name TEXT, email TEXT);",
								"migrations": []interface{}{
									map[string]interface{}{"version": 1, "sql": "ALTER TABLE users ADD COLUMN email TEXT"},
								},
							},
						},
					},
				},
			},
			isSystemTool:  false,
			expectError:   true,
			errorContains: "duplicate column name",
		},
		{
			name: "Migration references non-existent table - no such table error",
			tools: []Tool{
				{
					Name: "TestTool",
					Functions: []Function{
						{
							Name: "func1",
							With: map[string]interface{}{
								"init": "CREATE TABLE IF NOT EXISTS users (id INTEGER PRIMARY KEY);",
								"migrations": []interface{}{
									map[string]interface{}{"version": 1, "sql": "ALTER TABLE orders ADD COLUMN status TEXT"},
								},
							},
						},
					},
				},
			},
			isSystemTool:  false,
			expectError:   true,
			errorContains: "no such table",
		},
		{
			name: "Migration without any init - table doesn't exist",
			tools: []Tool{
				{
					Name: "TestTool",
					Functions: []Function{
						{
							Name: "func1",
							With: map[string]interface{}{
								"migrations": []interface{}{
									map[string]interface{}{"version": 1, "sql": "ALTER TABLE users ADD COLUMN email TEXT"},
								},
							},
						},
					},
				},
			},
			isSystemTool:  false,
			expectError:   true,
			errorContains: "no such table",
		},
		{
			name: "Migration with SQL syntax error",
			tools: []Tool{
				{
					Name: "TestTool",
					Functions: []Function{
						{
							Name: "func1",
							With: map[string]interface{}{
								"init": "CREATE TABLE IF NOT EXISTS users (id INTEGER PRIMARY KEY);",
								"migrations": []interface{}{
									map[string]interface{}{"version": 1, "sql": "ALTER TABEL users ADD COLUMN email TEXT"}, // Typo: TABEL
								},
							},
						},
					},
				},
			},
			isSystemTool:  false,
			expectError:   true,
			errorContains: "migration v1 failed",
		},
		{
			name: "System tool skips validation - even with conflicts",
			tools: []Tool{
				{
					Name: "TestTool",
					Functions: []Function{
						{
							Name: "func1",
							With: map[string]interface{}{
								"init": "CREATE TABLE IF NOT EXISTS users (id INTEGER PRIMARY KEY, email TEXT);",
								"migrations": []interface{}{
									map[string]interface{}{"version": 1, "sql": "ALTER TABLE users ADD COLUMN email TEXT"},
								},
							},
						},
					},
				},
			},
			isSystemTool: true,
			expectError:  false,
		},
		{
			name: "Migrations from multiple functions collected together - valid",
			tools: []Tool{
				{
					Name: "TestTool",
					Functions: []Function{
						{
							Name: "func1",
							With: map[string]interface{}{
								"init": "CREATE TABLE IF NOT EXISTS users (id INTEGER PRIMARY KEY, name TEXT);",
								"migrations": []interface{}{
									map[string]interface{}{"version": 1, "sql": "ALTER TABLE users ADD COLUMN email TEXT"},
								},
							},
						},
						{
							Name: "func2",
							With: map[string]interface{}{
								"init": "CREATE TABLE IF NOT EXISTS users (id INTEGER PRIMARY KEY, name TEXT);",
								"migrations": []interface{}{
									map[string]interface{}{"version": 2, "sql": "ALTER TABLE users ADD COLUMN phone TEXT"},
								},
							},
						},
					},
				},
			},
			isSystemTool: false,
			expectError:  false,
		},
		{
			name: "Migration v2 depends on v1 - applied in order successfully",
			tools: []Tool{
				{
					Name: "TestTool",
					Functions: []Function{
						{
							Name: "func1",
							With: map[string]interface{}{
								"init": "CREATE TABLE IF NOT EXISTS users (id INTEGER PRIMARY KEY);",
								"migrations": []interface{}{
									map[string]interface{}{"version": 2, "sql": "CREATE INDEX IF NOT EXISTS idx_email ON users(email)"},
									map[string]interface{}{"version": 1, "sql": "ALTER TABLE users ADD COLUMN email TEXT"},
								},
							},
						},
					},
				},
			},
			isSystemTool: false,
			expectError:  false, // Should work because migrations are sorted by version
		},
		{
			name: "Migration conflicts between v1 and v2 - v2 adds same column as v1",
			tools: []Tool{
				{
					Name: "TestTool",
					Functions: []Function{
						{
							Name: "func1",
							With: map[string]interface{}{
								"init": "CREATE TABLE IF NOT EXISTS users (id INTEGER PRIMARY KEY);",
								"migrations": []interface{}{
									map[string]interface{}{"version": 1, "sql": "ALTER TABLE users ADD COLUMN email TEXT"},
									map[string]interface{}{"version": 2, "sql": "ALTER TABLE users ADD COLUMN email TEXT"},
								},
							},
						},
					},
				},
			},
			isSystemTool:  false,
			expectError:   true,
			errorContains: "duplicate column name",
		},
		{
			name: "Multiple tools - each validated independently",
			tools: []Tool{
				{
					Name: "Tool1",
					Functions: []Function{
						{
							Name: "func1",
							With: map[string]interface{}{
								"init": "CREATE TABLE IF NOT EXISTS tool1_users (id INTEGER PRIMARY KEY);",
								"migrations": []interface{}{
									map[string]interface{}{"version": 1, "sql": "ALTER TABLE tool1_users ADD COLUMN email TEXT"},
								},
							},
						},
					},
				},
				{
					Name: "Tool2",
					Functions: []Function{
						{
							Name: "func1",
							With: map[string]interface{}{
								"init": "CREATE TABLE IF NOT EXISTS tool2_users (id INTEGER PRIMARY KEY);",
								"migrations": []interface{}{
									map[string]interface{}{"version": 1, "sql": "ALTER TABLE tool2_users ADD COLUMN phone TEXT"},
								},
							},
						},
					},
				},
			},
			isSystemTool: false,
			expectError:  false,
		},
		{
			name: "Init fails - propagates error",
			tools: []Tool{
				{
					Name: "TestTool",
					Functions: []Function{
						{
							Name: "func1",
							With: map[string]interface{}{
								"init": "CREATE TABEL users (id INTEGER PRIMARY KEY);", // Typo: TABEL
								"migrations": []interface{}{
									map[string]interface{}{"version": 1, "sql": "ALTER TABLE users ADD COLUMN email TEXT"},
								},
							},
						},
					},
				},
			},
			isSystemTool:  false,
			expectError:   true,
			errorContains: "init SQL failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateInitAndMigrationsTogether(tt.tools, tt.isSystemTool)

			if tt.expectError {
				require.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// Integration tests using CreateTool with full YAML
func TestCreateToolWithInitAndMigrationConflict(t *testing.T) {
	yaml := `
version: v1
author: TestAuthor
tools:
  - name: "TestTool"
    version: "1.0.0"
    description: "Tool with init/migration column conflict"
    functions:
      - name: "TestFunc"
        operation: "db"
        description: "Function where init already has the column migration tries to add"
        triggers:
          - type: "flex_for_user"
        with:
          init: |
            CREATE TABLE IF NOT EXISTS important_tracker (
              id INTEGER PRIMARY KEY,
              conversation_id TEXT,
              follow_up_stages TEXT
            );
          migrations:
            - version: 1
              sql: "ALTER TABLE important_tracker ADD COLUMN follow_up_stages TEXT"
        steps:
          - name: "select"
            action: "select"
            with:
              select: "SELECT * FROM important_tracker LIMIT 1"
        output:
          type: "string"
          value: "done"
`

	_, err := CreateTool(yaml)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate column name")
	assert.Contains(t, err.Error(), "already exists in the init script")
}

func TestCreateToolWithMigrationReferencingNonExistentTable(t *testing.T) {
	yaml := `
version: v1
author: TestAuthor
tools:
  - name: "TestTool"
    version: "1.0.0"
    description: "Tool with migration referencing wrong table"
    functions:
      - name: "TestFunc"
        operation: "db"
        description: "Migration references table not in init"
        triggers:
          - type: "flex_for_user"
        with:
          init: |
            CREATE TABLE IF NOT EXISTS users (id INTEGER PRIMARY KEY);
          migrations:
            - version: 1
              sql: "ALTER TABLE orders ADD COLUMN status TEXT"
        steps:
          - name: "select"
            action: "select"
            with:
              select: "SELECT * FROM users LIMIT 1"
        output:
          type: "string"
          value: "done"
`

	_, err := CreateTool(yaml)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no such table")
}

func TestCreateToolWithValidInitAndMigrations(t *testing.T) {
	yaml := `
version: v1
author: TestAuthor
tools:
  - name: "TestTool"
    version: "1.0.0"
    description: "Tool with valid init and migrations"
    functions:
      - name: "TestFunc"
        operation: "db"
        description: "Valid setup"
        triggers:
          - type: "flex_for_user"
        with:
          init: |
            CREATE TABLE IF NOT EXISTS users (
              id INTEGER PRIMARY KEY,
              name TEXT NOT NULL
            );
          migrations:
            - version: 1
              sql: "ALTER TABLE users ADD COLUMN email TEXT"
            - version: 2
              sql: "ALTER TABLE users ADD COLUMN phone TEXT"
            - version: 3
              sql: "CREATE INDEX IF NOT EXISTS idx_users_email ON users(email)"
        steps:
          - name: "select"
            action: "select"
            with:
              select: "SELECT * FROM users LIMIT 1"
        output:
          type: "string"
          value: "done"
`

	tool, err := CreateTool(yaml)
	require.NoError(t, err)
	assert.Equal(t, "TestTool", tool.Tools[0].Name)
}

func TestCreateToolWithMultipleFunctionsMigrationsValidated(t *testing.T) {
	yaml := `
version: v1
author: TestAuthor
tools:
  - name: "TestTool"
    version: "1.0.0"
    description: "Tool with multiple functions and migrations"
    functions:
      - name: "GetUsers"
        operation: "db"
        description: "Get users"
        triggers:
          - type: "flex_for_user"
        with:
          init: |
            CREATE TABLE IF NOT EXISTS users (id INTEGER PRIMARY KEY, name TEXT);
          migrations:
            - version: 1
              sql: "ALTER TABLE users ADD COLUMN email TEXT"
        steps:
          - name: "select"
            action: "select"
            with:
              select: "SELECT * FROM users"
        output:
          type: "string"
          value: "done"
      - name: "UpdateUser"
        operation: "db"
        description: "Update user"
        triggers:
          - type: "flex_for_user"
        with:
          init: |
            CREATE TABLE IF NOT EXISTS users (id INTEGER PRIMARY KEY, name TEXT);
          migrations:
            - version: 1
              sql: "ALTER TABLE users ADD COLUMN email TEXT"
            - version: 2
              sql: "ALTER TABLE users ADD COLUMN phone TEXT"
        steps:
          - name: "write"
            action: "write"
            with:
              write: "UPDATE users SET name = 'test' WHERE id = 1"
        output:
          type: "string"
          value: "done"
`

	tool, err := CreateTool(yaml)
	require.NoError(t, err)
	assert.Len(t, tool.Tools[0].Functions, 2)
}

func TestCreateToolMigrationWithoutInit(t *testing.T) {
	yaml := `
version: v1
author: TestAuthor
tools:
  - name: "TestTool"
    version: "1.0.0"
    description: "Tool with migration but no init"
    functions:
      - name: "TestFunc"
        operation: "db"
        description: "Has migration but no init to create table"
        triggers:
          - type: "flex_for_user"
        with:
          migrations:
            - version: 1
              sql: "ALTER TABLE users ADD COLUMN email TEXT"
        steps:
          - name: "select"
            action: "select"
            with:
              select: "SELECT * FROM users"
        output:
          type: "string"
          value: "done"
`

	_, err := CreateTool(yaml)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no such table")
}

func TestCreateToolMigrationOutOfOrder(t *testing.T) {
	// Test that migrations are sorted by version before execution
	yaml := `
version: v1
author: TestAuthor
tools:
  - name: "TestTool"
    version: "1.0.0"
    description: "Tool with out-of-order migrations"
    functions:
      - name: "TestFunc"
        operation: "db"
        description: "Migrations defined out of order but should be sorted"
        triggers:
          - type: "flex_for_user"
        with:
          init: |
            CREATE TABLE IF NOT EXISTS users (id INTEGER PRIMARY KEY);
          migrations:
            - version: 3
              sql: "CREATE INDEX IF NOT EXISTS idx_users_phone ON users(phone)"
            - version: 1
              sql: "ALTER TABLE users ADD COLUMN email TEXT"
            - version: 2
              sql: "ALTER TABLE users ADD COLUMN phone TEXT"
        steps:
          - name: "select"
            action: "select"
            with:
              select: "SELECT * FROM users"
        output:
          type: "string"
          value: "done"
`

	// This should succeed because migrations are sorted by version
	tool, err := CreateTool(yaml)
	require.NoError(t, err)
	assert.Equal(t, "TestTool", tool.Tools[0].Name)
}

func TestCreateToolMigrationSQLSyntaxError(t *testing.T) {
	yaml := `
version: v1
author: TestAuthor
tools:
  - name: "TestTool"
    version: "1.0.0"
    description: "Tool with SQL syntax error in migration"
    functions:
      - name: "TestFunc"
        operation: "db"
        description: "Migration has SQL typo"
        triggers:
          - type: "flex_for_user"
        with:
          init: |
            CREATE TABLE IF NOT EXISTS users (id INTEGER PRIMARY KEY);
          migrations:
            - version: 1
              sql: "ALTR TABLE users ADD COLUMN email TEXT"
        steps:
          - name: "select"
            action: "select"
            with:
              select: "SELECT * FROM users"
        output:
          type: "string"
          value: "done"
`

	_, err := CreateTool(yaml)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "migration v1 failed")
}

// Tests for per-function validation (simulates runtime behavior)
func TestValidatePerFunctionInitWithAllMigrations(t *testing.T) {
	tests := []struct {
		name          string
		tools         []Tool
		expectError   bool
		errorContains string
	}{
		{
			name: "All functions have same init - valid",
			tools: []Tool{
				{
					Name: "TestTool",
					Functions: []Function{
						{
							Name:      "GetUsers",
							Operation: OperationDB,
							With: map[string]interface{}{
								"init": "CREATE TABLE IF NOT EXISTS users (id INTEGER PRIMARY KEY);",
								"migrations": []interface{}{
									map[string]interface{}{"version": 1, "sql": "ALTER TABLE users ADD COLUMN email TEXT"},
								},
							},
						},
						{
							Name:      "UpdateUser",
							Operation: OperationDB,
							With: map[string]interface{}{
								"init": "CREATE TABLE IF NOT EXISTS users (id INTEGER PRIMARY KEY);",
								"migrations": []interface{}{
									map[string]interface{}{"version": 1, "sql": "ALTER TABLE users ADD COLUMN email TEXT"},
								},
							},
						},
					},
				},
			},
			expectError: false,
		},
		{
			name: "Migration references table from another function's init - FAILS",
			tools: []Tool{
				{
					Name: "TestTool",
					Functions: []Function{
						{
							Name:      "GetUsers",
							Operation: OperationDB,
							With: map[string]interface{}{
								"init": "CREATE TABLE IF NOT EXISTS users (id INTEGER PRIMARY KEY);",
								"migrations": []interface{}{
									map[string]interface{}{"version": 1, "sql": "ALTER TABLE users ADD COLUMN email TEXT"},
								},
							},
						},
						{
							Name:      "GetOrders",
							Operation: OperationDB,
							With: map[string]interface{}{
								"init": "CREATE TABLE IF NOT EXISTS orders (id INTEGER PRIMARY KEY);",
								"migrations": []interface{}{
									map[string]interface{}{"version": 2, "sql": "ALTER TABLE orders ADD COLUMN status TEXT"},
								},
							},
						},
					},
				},
			},
			expectError:   true,
			errorContains: "no such table",
		},
		{
			name: "Function without init but tool has migrations - FAILS",
			tools: []Tool{
				{
					Name: "TestTool",
					Functions: []Function{
						{
							Name:      "GetUsers",
							Operation: OperationDB,
							With: map[string]interface{}{
								"init": "CREATE TABLE IF NOT EXISTS users (id INTEGER PRIMARY KEY);",
								"migrations": []interface{}{
									map[string]interface{}{"version": 1, "sql": "ALTER TABLE users ADD COLUMN email TEXT"},
								},
							},
						},
						{
							Name:      "GetStats",
							Operation: OperationDB,
							With:      map[string]interface{}{}, // No init!
						},
					},
				},
			},
			expectError:   true,
			errorContains: "no such table",
		},
		{
			name: "Non-db operation functions are skipped",
			tools: []Tool{
				{
					Name: "TestTool",
					Functions: []Function{
						{
							Name:      "GetUsers",
							Operation: OperationDB,
							With: map[string]interface{}{
								"init": "CREATE TABLE IF NOT EXISTS users (id INTEGER PRIMARY KEY);",
								"migrations": []interface{}{
									map[string]interface{}{"version": 1, "sql": "ALTER TABLE users ADD COLUMN email TEXT"},
								},
							},
						},
						{
							Name:      "FormatData",
							Operation: OperationFormat, // Not a db operation
							With:      map[string]interface{}{},
						},
					},
				},
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateInitAndMigrationsTogether(tt.tools, false)

			if tt.expectError {
				require.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestCreateToolWithCrossFunctionMigrationReference(t *testing.T) {
	// This test simulates the runtime issue:
	// GetOrders has migration that references 'users' table, but only GetUsers' init creates it
	yaml := `
version: v1
author: TestAuthor
tools:
  - name: "TestTool"
    version: "1.0.0"
    description: "Tool with cross-function migration reference"
    functions:
      - name: "GetUsers"
        operation: "db"
        description: "Works with users table"
        triggers:
          - type: "flex_for_user"
        with:
          init: |
            CREATE TABLE IF NOT EXISTS users (id INTEGER PRIMARY KEY, name TEXT);
          migrations:
            - version: 1
              sql: "ALTER TABLE users ADD COLUMN email TEXT"
        steps:
          - name: "select"
            action: "select"
            with:
              select: "SELECT * FROM users"
        output:
          type: "string"
          value: "done"
      - name: "GetOrders"
        operation: "db"
        description: "Works with orders table but migration references users"
        triggers:
          - type: "flex_for_user"
        with:
          init: |
            CREATE TABLE IF NOT EXISTS orders (id INTEGER PRIMARY KEY);
          migrations:
            - version: 2
              sql: "ALTER TABLE orders ADD COLUMN user_id INTEGER REFERENCES users(id)"
        steps:
          - name: "select"
            action: "select"
            with:
              select: "SELECT * FROM orders"
        output:
          type: "string"
          value: "done"
`

	_, err := CreateTool(yaml)
	require.Error(t, err)
	// GetUsers init doesn't create 'orders' table, so migration v2 (ALTER orders) will fail
	// when testing GetUsers' init + all migrations
	assert.Contains(t, err.Error(), "no such table")
	assert.Contains(t, err.Error(), "migration v2 failed")
}

func TestCreateToolWithConsistentInitsAcrossFunctions(t *testing.T) {
	// This is the CORRECT pattern: all functions have the same init
	yaml := `
version: v1
author: TestAuthor
tools:
  - name: "TestTool"
    version: "1.0.0"
    description: "Tool with consistent inits"
    functions:
      - name: "GetUsers"
        operation: "db"
        description: "Works with users table"
        triggers:
          - type: "flex_for_user"
        with:
          init: |
            CREATE TABLE IF NOT EXISTS users (id INTEGER PRIMARY KEY);
            CREATE TABLE IF NOT EXISTS orders (id INTEGER PRIMARY KEY);
          migrations:
            - version: 1
              sql: "ALTER TABLE users ADD COLUMN email TEXT"
        steps:
          - name: "select"
            action: "select"
            with:
              select: "SELECT * FROM users"
        output:
          type: "string"
          value: "done"
      - name: "GetOrders"
        operation: "db"
        description: "Works with orders table"
        triggers:
          - type: "flex_for_user"
        with:
          init: |
            CREATE TABLE IF NOT EXISTS users (id INTEGER PRIMARY KEY);
            CREATE TABLE IF NOT EXISTS orders (id INTEGER PRIMARY KEY);
          migrations:
            - version: 1
              sql: "ALTER TABLE users ADD COLUMN email TEXT"
            - version: 2
              sql: "ALTER TABLE orders ADD COLUMN status TEXT"
        steps:
          - name: "select"
            action: "select"
            with:
              select: "SELECT * FROM orders"
        output:
          type: "string"
          value: "done"
`

	tool, err := CreateTool(yaml)
	require.NoError(t, err)
	assert.Equal(t, "TestTool", tool.Tools[0].Name)
}
