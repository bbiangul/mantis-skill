package skill

import (
	"testing"
)

func TestExtractTableNames_UPSERT(t *testing.T) {
	tests := []struct {
		name           string
		sql            string
		expectedTables []string
	}{
		{
			name: "UPSERT with ON CONFLICT DO UPDATE SET",
			sql: `INSERT INTO user_preferences (user_id, preferred_dentist_id)
VALUES ($userId, $dentistId)
ON CONFLICT(user_id) DO UPDATE SET
  preferred_dentist_id = excluded.preferred_dentist_id`,
			expectedTables: []string{"USER_PREFERENCES"},
		},
		{
			name:           "Regular UPDATE statement",
			sql:            `UPDATE users SET name = 'John' WHERE id = 1`,
			expectedTables: []string{"USERS"},
		},
		{
			name:           "Regular INSERT statement",
			sql:            `INSERT INTO customers (name, email) VALUES ('Jane', 'jane@example.com')`,
			expectedTables: []string{"CUSTOMERS"},
		},
		{
			name: "UPSERT with multiple columns in SET",
			sql: `INSERT INTO config (key, value, updated_at)
VALUES ($key, $value, CURRENT_TIMESTAMP)
ON CONFLICT(key) DO UPDATE SET
  value = excluded.value,
  updated_at = excluded.updated_at`,
			expectedTables: []string{"CONFIG"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tables := extractTableNamesFromSQL(tt.sql)

			// Check if we got the expected number of tables
			if len(tables) != len(tt.expectedTables) {
				t.Errorf("Expected %d tables, got %d: %v", len(tt.expectedTables), len(tables), tables)
				return
			}

			// Check if all expected tables are present
			tableMap := make(map[string]bool)
			for _, table := range tables {
				tableMap[table] = true
			}

			for _, expected := range tt.expectedTables {
				if !tableMap[expected] {
					t.Errorf("Expected table '%s' not found in extracted tables: %v", expected, tables)
				}
			}

			// Make sure "SET" is not in the results
			for _, table := range tables {
				if table == "SET" {
					t.Errorf("Reserved keyword 'SET' should not be extracted as a table name")
				}
			}
		})
	}
}
