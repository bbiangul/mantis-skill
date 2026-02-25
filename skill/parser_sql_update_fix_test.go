package skill

import (
	"sort"
	"strings"
	"testing"
)

// TestExtractTableNamesFromSQL_UpdateStatements tests UPDATE statement parsing
// to ensure backward compatibility and correct handling of ON CONFLICT DO UPDATE SET
func TestExtractTableNamesFromSQL_UpdateStatements(t *testing.T) {
	tests := []struct {
		name     string
		sql      string
		expected []string
	}{
		// Backward compatibility tests - normal UPDATE statements
		{
			name:     "simple UPDATE statement",
			sql:      "UPDATE users SET name = 'John'",
			expected: []string{"USERS"},
		},
		{
			name:     "UPDATE with WHERE clause",
			sql:      "UPDATE users SET name = 'John' WHERE id = 1",
			expected: []string{"USERS"},
		},
		{
			name:     "UPDATE with multiple SET clauses",
			sql:      "UPDATE users SET name = 'John', age = 30 WHERE id = 1",
			expected: []string{"USERS"},
		},
		{
			name:     "UPDATE with lowercase",
			sql:      "update customers set status = 'active'",
			expected: []string{"CUSTOMERS"},
		},

		// The fix - ON CONFLICT DO UPDATE SET should NOT capture "SET" as table
		{
			name:     "INSERT with ON CONFLICT DO UPDATE SET - single line",
			sql:      "INSERT INTO user_preferences (user_id, preferred_dentist_id) VALUES ($userId, $dentistId) ON CONFLICT(user_id) DO UPDATE SET preferred_dentist_id = excluded.preferred_dentist_id",
			expected: []string{"USER_PREFERENCES"},
		},
		{
			name: "INSERT with ON CONFLICT DO UPDATE SET - multiline",
			sql: `INSERT INTO user_preferences (user_id, preferred_dentist_id)
VALUES ($userId, $dentistId)
ON CONFLICT(user_id) DO UPDATE SET
  preferred_dentist_id = excluded.preferred_dentist_id`,
			expected: []string{"USER_PREFERENCES"},
		},
		{
			name: "INSERT with ON CONFLICT DO UPDATE SET - multiple columns",
			sql: `INSERT INTO users (id, name, email)
VALUES (1, 'John', 'john@example.com')
ON CONFLICT(id) DO UPDATE SET
  name = excluded.name,
  email = excluded.email`,
			expected: []string{"USERS"},
		},
		{
			name: "UPSERT with ON CONFLICT DO UPDATE SET - complex",
			sql: `INSERT INTO patient_id_cache (user_id, patient_id)
VALUES ($userId, $patientIDExtracted)
ON CONFLICT(user_id) DO UPDATE SET
  patient_id = excluded.patient_id,
  updated_at = CURRENT_TIMESTAMP`,
			expected: []string{"PATIENT_ID_CACHE"},
		},

		// Edge cases
		{
			name:     "UPDATE with table alias",
			sql:      "UPDATE users u SET u.name = 'John'",
			expected: []string{"USERS"},
		},
		{
			name: "Multiple statements with UPDATE",
			sql: `CREATE TABLE users (id INT);
UPDATE users SET name = 'John';
INSERT INTO users VALUES (1)`,
			expected: []string{"USERS"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractTableNamesFromSQL(tt.sql)

			// Sort both slices for comparison
			sort.Strings(result)
			sort.Strings(tt.expected)

			if len(result) != len(tt.expected) {
				t.Errorf("extractTableNamesFromSQL() = %v, want %v", result, tt.expected)
				return
			}

			for i := range result {
				if result[i] != tt.expected[i] {
					t.Errorf("extractTableNamesFromSQL() = %v, want %v", result, tt.expected)
					return
				}
			}
		})
	}
}

// TestExtractTableNamesFromSQL_AllSQLPatterns tests all SQL patterns for backward compatibility
func TestExtractTableNamesFromSQL_AllSQLPatterns(t *testing.T) {
	tests := []struct {
		name     string
		sql      string
		expected []string
	}{
		// CREATE TABLE
		{
			name:     "CREATE TABLE",
			sql:      "CREATE TABLE users (id INT)",
			expected: []string{"USERS"},
		},
		{
			name:     "CREATE TABLE IF NOT EXISTS",
			sql:      "CREATE TABLE IF NOT EXISTS users (id INT)",
			expected: []string{"USERS"},
		},

		// INSERT
		{
			name:     "INSERT INTO",
			sql:      "INSERT INTO users (name) VALUES ('John')",
			expected: []string{"USERS"},
		},
		{
			name:     "INSERT OR REPLACE INTO",
			sql:      "INSERT OR REPLACE INTO users (name) VALUES ('John')",
			expected: []string{"USERS"},
		},
		{
			name:     "INSERT OR IGNORE INTO",
			sql:      "INSERT OR IGNORE INTO users (name) VALUES ('John')",
			expected: []string{"USERS"},
		},

		// SELECT / FROM
		{
			name:     "SELECT FROM",
			sql:      "SELECT * FROM users",
			expected: []string{"USERS"},
		},
		{
			name:     "SELECT FROM with WHERE",
			sql:      "SELECT * FROM users WHERE id = 1",
			expected: []string{"USERS"},
		},

		// JOIN
		{
			name:     "INNER JOIN",
			sql:      "SELECT * FROM users INNER JOIN orders ON users.id = orders.user_id",
			expected: []string{"USERS", "ORDERS"},
		},
		{
			name:     "LEFT JOIN",
			sql:      "SELECT * FROM users LEFT JOIN orders ON users.id = orders.user_id",
			expected: []string{"USERS", "ORDERS"},
		},

		// DELETE
		{
			name:     "DELETE FROM",
			sql:      "DELETE FROM users WHERE id = 1",
			expected: []string{"USERS"},
		},

		// Complex multi-table operations
		{
			name: "Multiple operations",
			sql: `CREATE TABLE users (id INT);
INSERT INTO users VALUES (1);
UPDATE users SET name = 'John';
SELECT * FROM users;
DELETE FROM users WHERE id = 1`,
			expected: []string{"USERS"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractTableNamesFromSQL(tt.sql)

			// Sort both slices for comparison
			sort.Strings(result)
			sort.Strings(tt.expected)

			if len(result) != len(tt.expected) {
				t.Errorf("extractTableNamesFromSQL() = %v, want %v", result, tt.expected)
				return
			}

			for i := range result {
				if result[i] != tt.expected[i] {
					t.Errorf("extractTableNamesFromSQL() = %v, want %v", result, tt.expected)
					return
				}
			}
		})
	}
}

// TestExtractTableNamesFromSQL_ShouldNotCaptureReservedKeywords ensures reserved keywords are filtered
func TestExtractTableNamesFromSQL_ShouldNotCaptureReservedKeywords(t *testing.T) {
	tests := []struct {
		name     string
		sql      string
		expected []string
	}{
		{
			name:     "UPDATE SET should not capture SET as table",
			sql:      "UPDATE users SET name = 'John'",
			expected: []string{"USERS"},
		},
		{
			name:     "ON CONFLICT DO UPDATE SET should not capture SET as table",
			sql:      "INSERT INTO users (id) VALUES (1) ON CONFLICT(id) DO UPDATE SET name = 'John'",
			expected: []string{"USERS"},
		},
		{
			name:     "SELECT with WHERE should not capture WHERE as table",
			sql:      "SELECT * FROM users WHERE id = 1",
			expected: []string{"USERS"},
		},
		{
			name:     "INSERT with VALUES should not capture VALUES as table",
			sql:      "INSERT INTO users VALUES (1, 'John')",
			expected: []string{"USERS"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractTableNamesFromSQL(tt.sql)

			// Sort both slices for comparison
			sort.Strings(result)
			sort.Strings(tt.expected)

			// Check that result doesn't contain any reserved keywords
			reservedKeywords := []string{"SET", "WHERE", "VALUES", "SELECT", "ORDER", "GROUP", "HAVING"}
			for _, keyword := range reservedKeywords {
				for _, table := range result {
					if table == keyword {
						t.Errorf("extractTableNamesFromSQL() captured reserved keyword %s as table name", keyword)
					}
				}
			}

			if len(result) != len(tt.expected) {
				t.Errorf("extractTableNamesFromSQL() = %v, want %v", result, tt.expected)
				return
			}

			for i := range result {
				if result[i] != tt.expected[i] {
					t.Errorf("extractTableNamesFromSQL() = %v, want %v", result, tt.expected)
					return
				}
			}
		})
	}
}

// TestExtractTableNamesFromSQL_RealWorldOralUnicExample tests the exact SQL from oralunic.yaml
func TestExtractTableNamesFromSQL_RealWorldOralUnicExample(t *testing.T) {
	// This is the exact SQL that was causing the issue in production
	sql := `INSERT INTO user_preferences (user_id, preferred_dentist_id)
VALUES ($userId, $dentistId)
ON CONFLICT(user_id) DO UPDATE SET
  preferred_dentist_id = excluded.preferred_dentist_id`

	result := extractTableNamesFromSQL(sql)
	expected := []string{"USER_PREFERENCES"}

	sort.Strings(result)
	sort.Strings(expected)

	if len(result) != len(expected) {
		t.Errorf("Real-world OralUnic example failed: got %v tables, want %v tables", len(result), len(expected))
		t.Errorf("  Got: %v", result)
		t.Errorf("  Want: %v", expected)
		return
	}

	for i := range result {
		if result[i] != expected[i] {
			t.Errorf("Real-world OralUnic example failed: got %v, want %v", result, expected)
			return
		}
	}

	// Explicitly verify SET is NOT in the result
	for _, table := range result {
		if table == "SET" {
			t.Errorf("CRITICAL BUG: 'SET' was captured as a table name! Result: %v", result)
		}
	}
}

// TestExtractTableNamesFromSQL_TableValuedFunctions tests that SQLite table-valued functions
// (like json_each, json_tree) are NOT captured as table names
func TestExtractTableNamesFromSQL_TableValuedFunctions(t *testing.T) {
	tests := []struct {
		name     string
		sql      string
		expected []string
	}{
		{
			name: "json_each in subquery",
			sql: `SELECT
              user_id, follow_up_count,
              COALESCE(
                (SELECT json_extract(je.value, '$.prompt')
                 FROM json_each(follow_up_stages) je
                 WHERE json_extract(je.value, '$.iteration') = follow_up_count
                 LIMIT 1),
                follow_up_prompt
              ) as follow_up_prompt
            FROM important_conversation_tracker`,
			expected: []string{"IMPORTANT_CONVERSATION_TRACKER"},
		},
		{
			name:     "json_each standalone",
			sql:      "SELECT * FROM json_each('[1,2,3]')",
			expected: []string{},
		},
		{
			name:     "json_tree function",
			sql:      "SELECT * FROM json_tree('{\"a\":1}')",
			expected: []string{},
		},
		{
			name: "json_each with real table",
			sql: `SELECT t.id, je.value
                  FROM my_table t, json_each(t.json_column) je
                  WHERE t.active = 1`,
			expected: []string{"MY_TABLE"},
		},
		{
			name: "multiple json_each in same query",
			sql: `SELECT
                    COALESCE(
                      (SELECT json_extract(je.value, '$.prompt')
                       FROM json_each(follow_up_stages) je
                       WHERE json_extract(je.value, '$.iteration') = follow_up_count),
                      follow_up_prompt
                    ) as prompt,
                    COALESCE(
                      (SELECT json_extract(jc.value, '$.context')
                       FROM json_each(follow_up_stages) jc
                       WHERE json_extract(jc.value, '$.iteration') = follow_up_count),
                      context_template
                    ) as context
                  FROM users`,
			expected: []string{"USERS"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractTableNamesFromSQL(tt.sql)

			// Sort both slices for comparison
			sort.Strings(result)
			sort.Strings(tt.expected)

			if len(result) != len(tt.expected) {
				t.Errorf("extractTableNamesFromSQL() = %v, want %v", result, tt.expected)
				return
			}

			for i := range result {
				if result[i] != tt.expected[i] {
					t.Errorf("extractTableNamesFromSQL() = %v, want %v", result, tt.expected)
					return
				}
			}

			// Verify no function calls are captured as table names
			for _, table := range result {
				if strings.Contains(table, "(") {
					t.Errorf("CRITICAL BUG: table-valued function '%s' was captured as a table name! Result: %v", table, result)
				}
			}
		})
	}
}
