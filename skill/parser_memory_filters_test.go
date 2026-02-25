package skill

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseMemoryFilters_InSuccessCriteria(t *testing.T) {
	tests := []struct {
		name            string
		successCriteria interface{}
		wantTopics      []string
		wantMetadata    map[string]interface{}
		wantErr         bool
		errContains     string
	}{
		{
			name: "memoryFilters with topic only",
			successCriteria: map[string]interface{}{
				"condition":              "find meeting notes",
				"allowedSystemFunctions": []string{"queryMemories"},
				"memoryFilters": map[string]interface{}{
					"topic": []interface{}{"meeting_transcript"},
				},
			},
			wantTopics:   []string{"meeting_transcript"},
			wantMetadata: nil,
		},
		{
			name: "memoryFilters with multiple topics",
			successCriteria: map[string]interface{}{
				"condition":              "find meeting notes and chat",
				"allowedSystemFunctions": []string{"queryMemories"},
				"memoryFilters": map[string]interface{}{
					"topic": []interface{}{"meeting_transcript", "meeting_chat"},
				},
			},
			wantTopics:   []string{"meeting_transcript", "meeting_chat"},
			wantMetadata: nil,
		},
		{
			name: "memoryFilters with metadata only",
			successCriteria: map[string]interface{}{
				"condition":              "find company meetings",
				"allowedSystemFunctions": []string{"queryMemories"},
				"memoryFilters": map[string]interface{}{
					"metadata": map[string]interface{}{
						"company_id":          "$companyId",
						"meeting_with_person": "John Doe",
					},
				},
			},
			wantTopics: nil,
			wantMetadata: map[string]interface{}{
				"company_id":          "$companyId",
				"meeting_with_person": "John Doe",
			},
		},
		{
			name: "memoryFilters with both topic and metadata",
			successCriteria: map[string]interface{}{
				"condition":              "find specific meeting",
				"allowedSystemFunctions": []string{"queryMemories"},
				"memoryFilters": map[string]interface{}{
					"topic": []interface{}{"meeting_transcript"},
					"metadata": map[string]interface{}{
						"company_id": "$companyId",
					},
				},
			},
			wantTopics: []string{"meeting_transcript"},
			wantMetadata: map[string]interface{}{
				"company_id": "$companyId",
			},
		},
		{
			name: "no memoryFilters (backward compatible)",
			successCriteria: map[string]interface{}{
				"condition":              "find memories",
				"allowedSystemFunctions": []string{"queryMemories"},
			},
			wantTopics:   nil,
			wantMetadata: nil,
		},
		{
			name:            "string successCriteria (backward compatible)",
			successCriteria: "find all memories",
			wantTopics:      nil,
			wantMetadata:    nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scObj, err := ParseSuccessCriteria(tt.successCriteria)
			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				return
			}
			require.NoError(t, err)
			require.NotNil(t, scObj)

			if tt.wantTopics == nil && tt.wantMetadata == nil {
				assert.Nil(t, scObj.MemoryFilters)
			} else {
				require.NotNil(t, scObj.MemoryFilters)
				assert.Equal(t, tt.wantTopics, scObj.MemoryFilters.Topic)

				if tt.wantMetadata != nil {
					require.NotNil(t, scObj.MemoryFilters.Metadata)
					for k, v := range tt.wantMetadata {
						assert.Equal(t, v, scObj.MemoryFilters.Metadata[k])
					}
				} else {
					assert.Nil(t, scObj.MemoryFilters.Metadata)
				}
			}
		})
	}
}

func TestParseMemoryFilters_InRunOnlyIf(t *testing.T) {
	tests := []struct {
		name         string
		runOnlyIf    interface{}
		wantTopics   []string
		wantMetadata map[string]interface{}
		wantErr      bool
		errContains  string
	}{
		{
			name: "memoryFilters in runOnlyIf with inference",
			runOnlyIf: map[string]interface{}{
				"condition":              "check if there are relevant meetings",
				"allowedSystemFunctions": []string{"queryMemories"},
				"memoryFilters": map[string]interface{}{
					"topic": []interface{}{"meeting_transcript"},
					"metadata": map[string]interface{}{
						"client_id": "$clientId",
					},
				},
			},
			wantTopics: []string{"meeting_transcript"},
			wantMetadata: map[string]interface{}{
				"client_id": "$clientId",
			},
		},
		{
			name: "memoryFilters in runOnlyIf with topic only",
			runOnlyIf: map[string]interface{}{
				"condition": "check function executions",
				"memoryFilters": map[string]interface{}{
					"topic": []interface{}{"function_executed"},
				},
			},
			wantTopics:   []string{"function_executed"},
			wantMetadata: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runOnlyIfObj, err := ParseRunOnlyIf(tt.runOnlyIf)
			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				return
			}
			require.NoError(t, err)
			require.NotNil(t, runOnlyIfObj)

			if tt.wantTopics == nil && tt.wantMetadata == nil {
				assert.Nil(t, runOnlyIfObj.MemoryFilters)
			} else {
				require.NotNil(t, runOnlyIfObj.MemoryFilters)
				assert.Equal(t, tt.wantTopics, runOnlyIfObj.MemoryFilters.Topic)

				if tt.wantMetadata != nil {
					require.NotNil(t, runOnlyIfObj.MemoryFilters.Metadata)
					for k, v := range tt.wantMetadata {
						assert.Equal(t, v, runOnlyIfObj.MemoryFilters.Metadata[k])
					}
				}
			}
		})
	}
}

func TestParseMemoryFiltersFromInterface(t *testing.T) {
	tests := []struct {
		name        string
		input       interface{}
		wantTopics  []string
		wantMeta    map[string]interface{}
		wantErr     bool
		errContains string
	}{
		{
			name: "valid map[string]interface{}",
			input: map[string]interface{}{
				"topic": []interface{}{"meeting_transcript"},
				"metadata": map[string]interface{}{
					"company_id": "123",
				},
			},
			wantTopics: []string{"meeting_transcript"},
			wantMeta: map[string]interface{}{
				"company_id": "123",
			},
		},
		{
			name: "valid map[interface{}]interface{}",
			input: map[interface{}]interface{}{
				"topic": []interface{}{"function_executed"},
				"metadata": map[interface{}]interface{}{
					"function_name": "createDeal",
				},
			},
			wantTopics: []string{"function_executed"},
			wantMeta: map[string]interface{}{
				"function_name": "createDeal",
			},
		},
		{
			name:  "nil input",
			input: nil,
		},
		{
			name:        "empty memoryFilters",
			input:       map[string]interface{}{},
			wantErr:     true,
			errContains: "must specify at least 'topic', 'metadata', or 'timeRange'",
		},
		{
			name:        "invalid type",
			input:       "string value",
			wantErr:     true,
			errContains: "must be an object",
		},
		{
			name: "invalid topic type",
			input: map[string]interface{}{
				"topic": 123,
			},
			wantErr:     true,
			errContains: "topic must be an array",
		},
		{
			name: "invalid metadata type",
			input: map[string]interface{}{
				"metadata": "string",
			},
			wantErr:     true,
			errContains: "metadata must be an object",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseMemoryFiltersFromInterface(tt.input)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				return
			}
			require.NoError(t, err)

			if tt.input == nil {
				assert.Nil(t, result)
				return
			}

			require.NotNil(t, result)
			assert.Equal(t, tt.wantTopics, result.Topic)

			if tt.wantMeta != nil {
				require.NotNil(t, result.Metadata)
				for k, v := range tt.wantMeta {
					assert.Equal(t, v, result.Metadata[k])
				}
			}
		})
	}
}

func TestMemoryFiltersValidation(t *testing.T) {
	t.Run("invalid topic value", func(t *testing.T) {
		input := map[string]interface{}{
			"topic": []interface{}{"invalid_topic"},
		}
		_, err := parseMemoryFiltersFromInterface(input)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "is not valid")
		assert.Contains(t, err.Error(), "invalid_topic")
	})

	t.Run("topic as variable reference is allowed", func(t *testing.T) {
		input := map[string]interface{}{
			"topic": []interface{}{"$topicVar"},
		}
		result, err := parseMemoryFiltersFromInterface(input)
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, []string{"$topicVar"}, result.Topic)
	})

	t.Run("invalid metadata key", func(t *testing.T) {
		input := map[string]interface{}{
			"topic": []interface{}{"meeting_transcript"},
			"metadata": map[string]interface{}{
				"invalid_key": "value",
			},
		}
		_, err := parseMemoryFiltersFromInterface(input)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid key")
		assert.Contains(t, err.Error(), "invalid_key")
	})

	t.Run("multiple invalid metadata keys", func(t *testing.T) {
		input := map[string]interface{}{
			"metadata": map[string]interface{}{
				"bad_key1": "value1",
				"bad_key2": "value2",
			},
		}
		_, err := parseMemoryFiltersFromInterface(input)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid key")
	})

	t.Run("all valid meeting_transcript metadata keys", func(t *testing.T) {
		input := map[string]interface{}{
			"topic": []interface{}{"meeting_transcript"},
			"metadata": map[string]interface{}{
				"company_id":          "123",
				"meeting_url":         "https://example.com",
				"bot_name":            "TestBot",
				"created_by":          "user@example.com",
				"meeting_topic":       "Project Review",
				"meeting_with_person": "John Doe",
				"bot_id":              "bot-123",
			},
		}
		result, err := parseMemoryFiltersFromInterface(input)
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, []string{"meeting_transcript"}, result.Topic)
		assert.Len(t, result.Metadata, 7)
	})

	t.Run("all valid function_executed metadata keys", func(t *testing.T) {
		input := map[string]interface{}{
			"topic": []interface{}{"function_executed"},
			"metadata": map[string]interface{}{
				"client_id":     "client-123",
				"message_id":    "msg-456",
				"function_name": "createDeal",
				"tool_name":     "CRM",
				"event_key":     "event-789",
				"user_message":  "Create a deal for this client",
				"has_error":     false,
			},
		}
		result, err := parseMemoryFiltersFromInterface(input)
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, []string{"function_executed"}, result.Topic)
		assert.Len(t, result.Metadata, 7)
	})

	t.Run("common metadata keys are valid", func(t *testing.T) {
		input := map[string]interface{}{
			"topic": []interface{}{"meeting_transcript"},
			"metadata": map[string]interface{}{
				"timestamp": "2024-01-01T00:00:00Z",
				"type":      "agentic_memory",
				"datetime":  "2024-01-01",
			},
		}
		result, err := parseMemoryFiltersFromInterface(input)
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Len(t, result.Metadata, 3)
	})
}

func TestIsValidMemoryTopic(t *testing.T) {
	tests := []struct {
		topic string
		want  bool
	}{
		{"meeting_transcript", true},
		{"function_executed", true},
		{"meeting_chat", true},
		{"invalid_topic", false},
		{"", false},
		{"MEETING_TRANSCRIPT", false}, // case sensitive
	}

	for _, tt := range tests {
		t.Run(tt.topic, func(t *testing.T) {
			got := IsValidMemoryTopic(tt.topic)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestMetadataContainsOperation(t *testing.T) {
	t.Run("metadata with contains operation for meeting_with_person", func(t *testing.T) {
		input := map[string]interface{}{
			"topic": []interface{}{"meeting_transcript"},
			"metadata": map[string]interface{}{
				"meeting_with_person": map[string]interface{}{
					"value":     "John",
					"operation": "contains",
				},
			},
		}
		result, err := parseMemoryFiltersFromInterface(input)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.NotNil(t, result.Metadata["meeting_with_person"])

		// Check it's a MetadataFilterValue
		filterVal, ok := result.Metadata["meeting_with_person"].(*MetadataFilterValue)
		require.True(t, ok)
		assert.Equal(t, "John", filterVal.Value)
		assert.Equal(t, MetadataFilterOperationContains, filterVal.Operation)
	})

	t.Run("metadata with contains operation for meeting_topic", func(t *testing.T) {
		input := map[string]interface{}{
			"topic": []interface{}{"meeting_transcript"},
			"metadata": map[string]interface{}{
				"meeting_topic": map[string]interface{}{
					"value":     "sales",
					"operation": "contains",
				},
			},
		}
		result, err := parseMemoryFiltersFromInterface(input)
		require.NoError(t, err)
		require.NotNil(t, result)

		filterVal, ok := result.Metadata["meeting_topic"].(*MetadataFilterValue)
		require.True(t, ok)
		assert.Equal(t, "sales", filterVal.Value)
		assert.Equal(t, MetadataFilterOperationContains, filterVal.Operation)
	})

	t.Run("metadata with exact operation (explicit)", func(t *testing.T) {
		input := map[string]interface{}{
			"topic": []interface{}{"meeting_transcript"},
			"metadata": map[string]interface{}{
				"meeting_with_person": map[string]interface{}{
					"value":     "John Doe",
					"operation": "exact",
				},
			},
		}
		result, err := parseMemoryFiltersFromInterface(input)
		require.NoError(t, err)

		filterVal, ok := result.Metadata["meeting_with_person"].(*MetadataFilterValue)
		require.True(t, ok)
		assert.Equal(t, "John Doe", filterVal.Value)
		assert.Equal(t, MetadataFilterOperationExact, filterVal.Operation)
	})

	t.Run("metadata with object form defaults to exact", func(t *testing.T) {
		input := map[string]interface{}{
			"topic": []interface{}{"meeting_transcript"},
			"metadata": map[string]interface{}{
				"meeting_with_person": map[string]interface{}{
					"value": "John Doe",
				},
			},
		}
		result, err := parseMemoryFiltersFromInterface(input)
		require.NoError(t, err)

		filterVal, ok := result.Metadata["meeting_with_person"].(*MetadataFilterValue)
		require.True(t, ok)
		assert.Equal(t, MetadataFilterOperationExact, filterVal.Operation)
	})

	t.Run("contains operation not allowed for company_id", func(t *testing.T) {
		input := map[string]interface{}{
			"topic": []interface{}{"meeting_transcript"},
			"metadata": map[string]interface{}{
				"company_id": map[string]interface{}{
					"value":     "ABC",
					"operation": "contains",
				},
			},
		}
		_, err := parseMemoryFiltersFromInterface(input)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "does not support 'contains' operation")
	})

	t.Run("invalid operation value", func(t *testing.T) {
		input := map[string]interface{}{
			"topic": []interface{}{"meeting_transcript"},
			"metadata": map[string]interface{}{
				"meeting_with_person": map[string]interface{}{
					"value":     "John",
					"operation": "invalid",
				},
			},
		}
		_, err := parseMemoryFiltersFromInterface(input)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be 'exact' or 'contains'")
	})

	t.Run("object form requires value field", func(t *testing.T) {
		input := map[string]interface{}{
			"topic": []interface{}{"meeting_transcript"},
			"metadata": map[string]interface{}{
				"meeting_with_person": map[string]interface{}{
					"operation": "contains",
				},
			},
		}
		_, err := parseMemoryFiltersFromInterface(input)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "requires 'value' field")
	})

	t.Run("mixed simple and object form metadata", func(t *testing.T) {
		input := map[string]interface{}{
			"topic": []interface{}{"meeting_transcript"},
			"metadata": map[string]interface{}{
				"company_id": "ABC123",
				"meeting_with_person": map[string]interface{}{
					"value":     "John",
					"operation": "contains",
				},
			},
		}
		result, err := parseMemoryFiltersFromInterface(input)
		require.NoError(t, err)
		require.NotNil(t, result)

		// Simple form stays as string
		assert.Equal(t, "ABC123", result.Metadata["company_id"])

		// Object form becomes MetadataFilterValue
		filterVal, ok := result.Metadata["meeting_with_person"].(*MetadataFilterValue)
		require.True(t, ok)
		assert.Equal(t, "John", filterVal.Value)
		assert.Equal(t, MetadataFilterOperationContains, filterVal.Operation)
	})
}

func TestSupportsContainsOperation(t *testing.T) {
	assert.True(t, SupportsContainsOperation("meeting_with_person"))
	assert.True(t, SupportsContainsOperation("meeting_topic"))
	assert.False(t, SupportsContainsOperation("company_id"))
	assert.False(t, SupportsContainsOperation("function_name"))
	assert.False(t, SupportsContainsOperation(""))
}

func TestIsValidMemoryMetadataKey(t *testing.T) {
	validKeys := []string{
		// Meeting transcript keys
		"company_id", "meeting_url", "bot_name", "created_by",
		"meeting_topic", "meeting_with_person", "bot_id",
		// Function execution keys
		"client_id", "message_id", "function_name", "tool_name",
		"event_key", "user_message", "has_error",
		// Common keys
		"timestamp", "type", "datetime",
	}

	for _, key := range validKeys {
		t.Run("valid_"+key, func(t *testing.T) {
			assert.True(t, IsValidMemoryMetadataKey(key))
		})
	}

	invalidKeys := []string{
		"invalid_key", "random", "sql_injection", "companyId", // camelCase not allowed
	}

	for _, key := range invalidKeys {
		t.Run("invalid_"+key, func(t *testing.T) {
			assert.False(t, IsValidMemoryMetadataKey(key))
		})
	}
}

func TestTimeRangeParsing(t *testing.T) {
	t.Run("valid timeRange with after only", func(t *testing.T) {
		input := map[string]interface{}{
			"topic": []interface{}{"meeting_transcript"},
			"timeRange": map[string]interface{}{
				"after": "2024-01-01",
			},
		}
		result, err := parseMemoryFiltersFromInterface(input)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.NotNil(t, result.TimeRange)
		assert.Equal(t, "2024-01-01", result.TimeRange.After)
		assert.Equal(t, "", result.TimeRange.Before)
	})

	t.Run("valid timeRange with before only", func(t *testing.T) {
		input := map[string]interface{}{
			"topic": []interface{}{"meeting_transcript"},
			"timeRange": map[string]interface{}{
				"before": "2024-12-31",
			},
		}
		result, err := parseMemoryFiltersFromInterface(input)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.NotNil(t, result.TimeRange)
		assert.Equal(t, "", result.TimeRange.After)
		assert.Equal(t, "2024-12-31", result.TimeRange.Before)
	})

	t.Run("valid timeRange with both after and before", func(t *testing.T) {
		input := map[string]interface{}{
			"topic": []interface{}{"meeting_transcript"},
			"timeRange": map[string]interface{}{
				"after":  "2024-01-01",
				"before": "2024-12-31",
			},
		}
		result, err := parseMemoryFiltersFromInterface(input)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.NotNil(t, result.TimeRange)
		assert.Equal(t, "2024-01-01", result.TimeRange.After)
		assert.Equal(t, "2024-12-31", result.TimeRange.Before)
	})

	t.Run("timeRange with variable references allowed", func(t *testing.T) {
		input := map[string]interface{}{
			"topic": []interface{}{"meeting_transcript"},
			"timeRange": map[string]interface{}{
				"after":  "$startDate",
				"before": "$endDate",
			},
		}
		result, err := parseMemoryFiltersFromInterface(input)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.NotNil(t, result.TimeRange)
		assert.Equal(t, "$startDate", result.TimeRange.After)
		assert.Equal(t, "$endDate", result.TimeRange.Before)
	})

	t.Run("timeRange only (without topic or metadata)", func(t *testing.T) {
		input := map[string]interface{}{
			"timeRange": map[string]interface{}{
				"after": "2024-01-01",
			},
		}
		result, err := parseMemoryFiltersFromInterface(input)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.NotNil(t, result.TimeRange)
		assert.Equal(t, "2024-01-01", result.TimeRange.After)
	})

	t.Run("invalid timeRange - empty", func(t *testing.T) {
		input := map[string]interface{}{
			"topic":     []interface{}{"meeting_transcript"},
			"timeRange": map[string]interface{}{},
		}
		_, err := parseMemoryFiltersFromInterface(input)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must specify at least 'after' or 'before'")
	})

	t.Run("invalid timeRange - wrong type", func(t *testing.T) {
		input := map[string]interface{}{
			"topic":     []interface{}{"meeting_transcript"},
			"timeRange": "invalid",
		}
		_, err := parseMemoryFiltersFromInterface(input)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be an object")
	})

	t.Run("invalid date format in after", func(t *testing.T) {
		input := map[string]interface{}{
			"topic": []interface{}{"meeting_transcript"},
			"timeRange": map[string]interface{}{
				"after": "01-01-2024", // Wrong format
			},
		}
		_, err := parseMemoryFiltersFromInterface(input)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not in valid format")
	})

	t.Run("invalid date format in before", func(t *testing.T) {
		input := map[string]interface{}{
			"topic": []interface{}{"meeting_transcript"},
			"timeRange": map[string]interface{}{
				"before": "December 31, 2024", // Wrong format
			},
		}
		_, err := parseMemoryFiltersFromInterface(input)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not in valid format")
	})

	t.Run("after must be a string", func(t *testing.T) {
		input := map[string]interface{}{
			"topic": []interface{}{"meeting_transcript"},
			"timeRange": map[string]interface{}{
				"after": 20240101,
			},
		}
		_, err := parseMemoryFiltersFromInterface(input)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be a string")
	})
}

func TestIsValidMemoryDateFormat(t *testing.T) {
	tests := []struct {
		date string
		want bool
	}{
		{"2024-01-15", true},
		{"2024-12-31", true},
		{"2023-06-01", true},
		{"$startDate", true},  // Variable reference
		{"$lastWeek", true},   // Variable reference
		{"", true},            // Empty is valid
		{"01-15-2024", false}, // MM-DD-YYYY
		{"15-01-2024", false}, // DD-MM-YYYY
		{"2024/01/15", false}, // Wrong separator
		{"2024-1-15", false},  // Single digit month
		{"2024-01-5", false},  // Single digit day
		{"invalid", false},
		{"January 15, 2024", false},
	}

	for _, tt := range tests {
		t.Run(tt.date, func(t *testing.T) {
			got := IsValidMemoryDateFormat(tt.date)
			assert.Equal(t, tt.want, got, "IsValidMemoryDateFormat(%q) = %v, want %v", tt.date, got, tt.want)
		})
	}
}
