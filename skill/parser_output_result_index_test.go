package skill

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Tests for output result[X] reference validation
// These check that output.value references to result[X] have corresponding resultIndex in steps

func TestOutputResultIndexValidation(t *testing.T) {
	t.Run("output references result[1] without resultIndex should fail", func(t *testing.T) {
		yaml := `
version: "v1"
author: "test"
tools:
  - name: "TestTool"
    version: "1.0.0"
    description: "A test tool"
    functions:
      - name: "TestFunc"
        operation: "api_call"
        description: "A test function"
        triggers:
          - type: "flex_for_user"
        steps:
          - name: "apiCall"
            action: "POST"
            with:
              url: "https://example.com/api"
        output:
          type: "string"
          value: '{"success":"result[1].Ok","message":"result[1].Mensagem"}'
`
		_, err := CreateTool(yaml)
		require.Error(t, err)
		assert.True(t, strings.Contains(err.Error(), "result[1]"), "Error should mention result[1]")
		assert.True(t, strings.Contains(err.Error(), "resultIndex: 1"), "Error should suggest adding resultIndex")
	})

	t.Run("output references result[1] with resultIndex should pass", func(t *testing.T) {
		yaml := `
version: "v1"
author: "test"
tools:
  - name: "TestTool"
    version: "1.0.0"
    description: "A test tool"
    functions:
      - name: "TestFunc"
        operation: "api_call"
        description: "A test function"
        triggers:
          - type: "flex_for_user"
        steps:
          - name: "apiCall"
            action: "POST"
            resultIndex: 1
            with:
              url: "https://example.com/api"
        output:
          type: "string"
          value: '{"success":"result[1].Ok","message":"result[1].Mensagem"}'
`
		tool, err := CreateTool(yaml)
		require.NoError(t, err)
		require.NotNil(t, tool)
	})

	t.Run("output references result[2] but only result[1] exists should fail", func(t *testing.T) {
		yaml := `
version: "v1"
author: "test"
tools:
  - name: "TestTool"
    version: "1.0.0"
    description: "A test tool"
    functions:
      - name: "TestFunc"
        operation: "api_call"
        description: "A test function"
        triggers:
          - type: "flex_for_user"
        steps:
          - name: "step1"
            action: "GET"
            resultIndex: 1
            with:
              url: "https://example.com/api"
        output:
          type: "string"
          value: '{"data":"result[2].value"}'
`
		_, err := CreateTool(yaml)
		require.Error(t, err)
		assert.True(t, strings.Contains(err.Error(), "result[2]"), "Error should mention result[2]")
	})

	t.Run("output with multiple valid result references should pass", func(t *testing.T) {
		yaml := `
version: "v1"
author: "test"
tools:
  - name: "TestTool"
    version: "1.0.0"
    description: "A test tool"
    functions:
      - name: "TestFunc"
        operation: "api_call"
        description: "A test function"
        triggers:
          - type: "flex_for_user"
        steps:
          - name: "step1"
            action: "GET"
            resultIndex: 1
            with:
              url: "https://example.com/api1"
          - name: "step2"
            action: "POST"
            resultIndex: 2
            with:
              url: "https://example.com/api2"
        output:
          type: "string"
          value: '{"first":"result[1].data","second":"result[2].data"}'
`
		tool, err := CreateTool(yaml)
		require.NoError(t, err)
		require.NotNil(t, tool)
	})

	t.Run("output without result references should pass", func(t *testing.T) {
		yaml := `
version: "v1"
author: "test"
tools:
  - name: "TestTool"
    version: "1.0.0"
    description: "A test tool"
    functions:
      - name: "TestFunc"
        operation: "api_call"
        description: "A test function"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "userName"
            description: "User name"
            origin: "chat"
            isOptional: true
        steps:
          - name: "apiCall"
            action: "POST"
            with:
              url: "https://example.com/api"
        output:
          type: "string"
          value: 'Hello $userName, your request was processed!'
`
		tool, err := CreateTool(yaml)
		require.NoError(t, err)
		require.NotNil(t, tool)
	})
}
