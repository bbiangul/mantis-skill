package skill

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseFunctionCache(t *testing.T) {
	tests := []struct {
		name           string
		cache          interface{}
		expectError    bool
		expectedScope  CacheScope
		expectedTTL    int
		expectedInputs bool
	}{
		{
			name:           "Nil cache - no caching",
			cache:          nil,
			expectError:    false,
			expectedScope:  "",
			expectedTTL:    0,
			expectedInputs: false,
		},
		{
			name:           "Int cache - backwards compatible (int)",
			cache:          300,
			expectError:    false,
			expectedScope:  CacheScopeGlobal,
			expectedTTL:    300,
			expectedInputs: true,
		},
		{
			name:           "Int cache - backwards compatible (float64 from YAML)",
			cache:          float64(300),
			expectError:    false,
			expectedScope:  CacheScopeGlobal,
			expectedTTL:    300,
			expectedInputs: true,
		},
		{
			name:        "Int cache - zero means no caching",
			cache:       0,
			expectError: false,
			expectedTTL: 0,
		},
		{
			name:        "Int cache - negative is invalid",
			cache:       -1,
			expectError: true,
		},
		{
			name: "Object cache - global scope with defaults",
			cache: map[string]interface{}{
				"scope": "global",
				"ttl":   600,
			},
			expectError:    false,
			expectedScope:  CacheScopeGlobal,
			expectedTTL:    600,
			expectedInputs: true,
		},
		{
			name: "Object cache - client scope",
			cache: map[string]interface{}{
				"scope":         "client",
				"ttl":           300,
				"includeInputs": false,
			},
			expectError:    false,
			expectedScope:  CacheScopeClient,
			expectedTTL:    300,
			expectedInputs: false,
		},
		{
			name: "Object cache - message scope",
			cache: map[string]interface{}{
				"scope":         "message",
				"ttl":           60,
				"includeInputs": true,
			},
			expectError:    false,
			expectedScope:  CacheScopeMessage,
			expectedTTL:    60,
			expectedInputs: true,
		},
		{
			name: "Object cache - default scope when omitted",
			cache: map[string]interface{}{
				"ttl": 300,
			},
			expectError:    false,
			expectedScope:  CacheScopeGlobal,
			expectedTTL:    300,
			expectedInputs: true,
		},
		{
			name: "Object cache - invalid scope",
			cache: map[string]interface{}{
				"scope": "invalid",
				"ttl":   300,
			},
			expectError: true,
		},
		{
			name: "Object cache - missing ttl",
			cache: map[string]interface{}{
				"scope": "client",
			},
			expectError: true,
		},
		{
			name: "Object cache - zero ttl",
			cache: map[string]interface{}{
				"scope": "client",
				"ttl":   0,
			},
			expectError: true,
		},
		{
			name: "Object cache - float64 ttl from YAML",
			cache: map[string]interface{}{
				"scope": "client",
				"ttl":   float64(300),
			},
			expectError:    false,
			expectedScope:  CacheScopeClient,
			expectedTTL:    300,
			expectedInputs: true,
		},
		{
			name:        "Invalid type - string",
			cache:       "invalid",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			function := Function{
				Name:  "TestFunction",
				Cache: tt.cache,
			}

			err := parseFunctionCache(&function, "TestTool")

			if tt.expectError {
				assert.Error(t, err, "Expected error for test case: %s", tt.name)
				return
			}

			require.NoError(t, err, "Unexpected error for test case: %s", tt.name)

			if tt.expectedTTL == 0 {
				assert.Nil(t, function.ParsedCache, "Expected nil ParsedCache")
				return
			}

			require.NotNil(t, function.ParsedCache, "Expected non-nil ParsedCache")
			assert.Equal(t, tt.expectedScope, function.ParsedCache.GetScope())
			assert.Equal(t, tt.expectedTTL, function.ParsedCache.TTL)
			assert.Equal(t, tt.expectedInputs, function.ParsedCache.GetIncludeInputs())
		})
	}
}

func TestCacheConfigHelpers(t *testing.T) {
	t.Run("GetIncludeInputs defaults to true", func(t *testing.T) {
		config := &CacheConfig{
			Scope:         CacheScopeClient,
			TTL:           300,
			IncludeInputs: nil, // Not set
		}
		assert.True(t, config.GetIncludeInputs())
	})

	t.Run("GetIncludeInputs returns set value", func(t *testing.T) {
		falseVal := false
		config := &CacheConfig{
			Scope:         CacheScopeClient,
			TTL:           300,
			IncludeInputs: &falseVal,
		}
		assert.False(t, config.GetIncludeInputs())
	})

	t.Run("GetScope defaults to global", func(t *testing.T) {
		config := &CacheConfig{
			Scope: "", // Empty
			TTL:   300,
		}
		assert.Equal(t, CacheScopeGlobal, config.GetScope())
	})

	t.Run("GetScope returns set value", func(t *testing.T) {
		config := &CacheConfig{
			Scope: CacheScopeClient,
			TTL:   300,
		}
		assert.Equal(t, CacheScopeClient, config.GetScope())
	})
}

func TestCheckCacheConfigWarnings(t *testing.T) {
	t.Run("No warnings for valid config", func(t *testing.T) {
		includeInputs := true
		functions := []Function{
			{
				Name: "func1",
				ParsedCache: &CacheConfig{
					Scope:         CacheScopeGlobal,
					TTL:           300,
					IncludeInputs: &includeInputs,
				},
			},
		}
		warnings := checkCacheConfigWarnings(functions)
		assert.Empty(t, warnings)
	})

	t.Run("Warning for global scope with includeInputs false", func(t *testing.T) {
		includeInputs := false
		functions := []Function{
			{
				Name: "dangerousFunc",
				ParsedCache: &CacheConfig{
					Scope:         CacheScopeGlobal,
					TTL:           300,
					IncludeInputs: &includeInputs,
				},
			},
		}
		warnings := checkCacheConfigWarnings(functions)
		require.Len(t, warnings, 1)
		assert.Contains(t, warnings[0], "dangerousFunc")
		assert.Contains(t, warnings[0], "global")
		assert.Contains(t, warnings[0], "includeInputs: false")
	})

	t.Run("Warning for message scope with long TTL", func(t *testing.T) {
		includeInputs := true
		functions := []Function{
			{
				Name: "longTTLFunc",
				ParsedCache: &CacheConfig{
					Scope:         CacheScopeMessage,
					TTL:           7200, // 2 hours
					IncludeInputs: &includeInputs,
				},
			},
		}
		warnings := checkCacheConfigWarnings(functions)
		require.Len(t, warnings, 1)
		assert.Contains(t, warnings[0], "longTTLFunc")
		assert.Contains(t, warnings[0], "message")
		assert.Contains(t, warnings[0], "7200")
	})

	t.Run("No warning for client scope with includeInputs false", func(t *testing.T) {
		includeInputs := false
		functions := []Function{
			{
				Name: "clientFunc",
				ParsedCache: &CacheConfig{
					Scope:         CacheScopeClient,
					TTL:           300,
					IncludeInputs: &includeInputs,
				},
			},
		}
		warnings := checkCacheConfigWarnings(functions)
		assert.Empty(t, warnings)
	})

	t.Run("No warning for message scope with reasonable TTL", func(t *testing.T) {
		includeInputs := true
		functions := []Function{
			{
				Name: "messageFunc",
				ParsedCache: &CacheConfig{
					Scope:         CacheScopeMessage,
					TTL:           300, // 5 minutes - reasonable
					IncludeInputs: &includeInputs,
				},
			},
		}
		warnings := checkCacheConfigWarnings(functions)
		assert.Empty(t, warnings)
	})

	t.Run("Multiple warnings for multiple problematic functions", func(t *testing.T) {
		includeInputsFalse := false
		includeInputsTrue := true
		functions := []Function{
			{
				Name: "globalNoInputs",
				ParsedCache: &CacheConfig{
					Scope:         CacheScopeGlobal,
					TTL:           300,
					IncludeInputs: &includeInputsFalse,
				},
			},
			{
				Name: "messageLongTTL",
				ParsedCache: &CacheConfig{
					Scope:         CacheScopeMessage,
					TTL:           7200,
					IncludeInputs: &includeInputsTrue,
				},
			},
		}
		warnings := checkCacheConfigWarnings(functions)
		require.Len(t, warnings, 2)
	})
}

func TestPreValidateInputCacheFormat(t *testing.T) {
	t.Run("Valid input with int cache", func(t *testing.T) {
		yaml := `
tools:
  - name: "TestTool"
    functions:
      - name: "TestFunc"
        input:
          - name: "testInput"
            cache: 300
`
		err := preValidateInputCacheFormat(yaml)
		assert.NoError(t, err)
	})

	t.Run("Valid input without cache", func(t *testing.T) {
		yaml := `
tools:
  - name: "TestTool"
    functions:
      - name: "TestFunc"
        input:
          - name: "testInput"
            description: "A test input"
`
		err := preValidateInputCacheFormat(yaml)
		assert.NoError(t, err)
	})

	t.Run("Invalid input with object cache format", func(t *testing.T) {
		yaml := `
tools:
  - name: "TestTool"
    functions:
      - name: "TestFunc"
        input:
          - name: "testInput"
            cache:
              scope: message
              ttl: 300
              includeInputs: false
`
		err := preValidateInputCacheFormat(yaml)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "testInput")
		assert.Contains(t, err.Error(), "TestFunc")
		assert.Contains(t, err.Error(), "TestTool")
		assert.Contains(t, err.Error(), "input-level cache only supports integer values")
		assert.Contains(t, err.Error(), "function level")
	})

	t.Run("Invalid input with partial object cache format", func(t *testing.T) {
		yaml := `
tools:
  - name: "MyTool"
    functions:
      - name: "MyFunc"
        input:
          - name: "myInput"
            cache:
              ttl: 300
`
		err := preValidateInputCacheFormat(yaml)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "myInput")
		assert.Contains(t, err.Error(), "MyFunc")
		assert.Contains(t, err.Error(), "MyTool")
	})

	t.Run("Valid function-level object cache is ignored", func(t *testing.T) {
		yaml := `
tools:
  - name: "TestTool"
    functions:
      - name: "TestFunc"
        cache:
          scope: message
          ttl: 300
          includeInputs: false
        input:
          - name: "testInput"
            description: "A test input"
`
		err := preValidateInputCacheFormat(yaml)
		assert.NoError(t, err)
	})

	t.Run("Multiple inputs - one invalid", func(t *testing.T) {
		yaml := `
tools:
  - name: "TestTool"
    functions:
      - name: "TestFunc"
        input:
          - name: "validInput"
            cache: 300
          - name: "invalidInput"
            cache:
              scope: client
              ttl: 600
`
		err := preValidateInputCacheFormat(yaml)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalidInput")
	})

	t.Run("Multiple functions - error in second function", func(t *testing.T) {
		yaml := `
tools:
  - name: "TestTool"
    functions:
      - name: "ValidFunc"
        input:
          - name: "validInput"
            cache: 300
      - name: "InvalidFunc"
        input:
          - name: "badInput"
            cache:
              scope: global
              ttl: 100
`
		err := preValidateInputCacheFormat(yaml)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "badInput")
		assert.Contains(t, err.Error(), "InvalidFunc")
	})

	t.Run("Invalid YAML is passed through", func(t *testing.T) {
		yaml := `this is not valid yaml: [[[`
		err := preValidateInputCacheFormat(yaml)
		// Should return nil to let the main parser handle the error
		assert.NoError(t, err)
	})

	t.Run("Empty YAML", func(t *testing.T) {
		yaml := ``
		err := preValidateInputCacheFormat(yaml)
		assert.NoError(t, err)
	})

	t.Run("YAML without tools", func(t *testing.T) {
		yaml := `
version: "1.0"
env:
  - name: "API_KEY"
`
		err := preValidateInputCacheFormat(yaml)
		assert.NoError(t, err)
	})

	t.Run("Function without inputs", func(t *testing.T) {
		yaml := `
tools:
  - name: "TestTool"
    functions:
      - name: "NoInputFunc"
        description: "A function without inputs"
`
		err := preValidateInputCacheFormat(yaml)
		assert.NoError(t, err)
	})
}

func TestCreateToolWithInputCacheValidation(t *testing.T) {
	t.Run("CreateTool rejects input with object cache format", func(t *testing.T) {
		yaml := `
version: "v1"
tools:
  - name: "TestTool"
    description: "A test tool"
    functions:
      - name: "TestFunc"
        operation: "api_call"
        description: "A test function"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "testInput"
            description: "A test input"
            origin: "chat"
            cache:
              scope: message
              ttl: 300
        steps:
          - name: "step1"
            action: "fetch"
            with:
              url: "https://example.com"
`
		_, err := CreateTool(yaml)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "testInput")
		assert.Contains(t, err.Error(), "input-level cache only supports integer values")
	})

	t.Run("CreateTool accepts input with int cache", func(t *testing.T) {
		yaml := `
version: "v1"
author: "test"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "TestFunc"
        operation: "format"
        description: "A test function"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "testInput"
            description: "A test input"
            origin: "inference"
            cache: 300
            successCriteria:
              condition: "test"
            onError:
              strategy: "requestUserInput"
              message: "Please provide input"
`
		tool, err := CreateTool(yaml)
		require.NoError(t, err)
		assert.Equal(t, 300, tool.Tools[0].Functions[0].Input[0].Cache)
	})

	t.Run("CreateTool accepts function-level object cache", func(t *testing.T) {
		yaml := `
version: "v1"
author: "test"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
    functions:
      - name: "TestFunc"
        operation: "format"
        description: "A test function"
        cache:
          scope: client
          ttl: 300
          includeInputs: false
        triggers:
          - type: "flex_for_user"
        input:
          - name: "testInput"
            description: "A test input"
            origin: "inference"
            successCriteria:
              condition: "test"
            onError:
              strategy: "requestUserInput"
              message: "Please provide input"
`
		tool, err := CreateTool(yaml)
		require.NoError(t, err)
		require.NotNil(t, tool.Tools[0].Functions[0].ParsedCache)
		assert.Equal(t, CacheScopeClient, tool.Tools[0].Functions[0].ParsedCache.GetScope())
		assert.Equal(t, 300, tool.Tools[0].Functions[0].ParsedCache.TTL)
		assert.False(t, tool.Tools[0].Functions[0].ParsedCache.GetIncludeInputs())
	})
}
