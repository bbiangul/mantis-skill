package skill

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOutputAllowInferenceParsing(t *testing.T) {
	t.Run("Default allowInference is false", func(t *testing.T) {
		yaml := `
version: "v1"
author: "test"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
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
            onError:
              strategy: "requestUserInput"
              message: "Please provide input"
        steps:
          - name: "step1"
            action: "GET"
            with:
              url: "https://example.com"
        output:
          type: "object"
          fields:
            - "id"
            - "name"
`
		tool, err := CreateTool(yaml)
		require.NoError(t, err)
		require.NotNil(t, tool.Tools[0].Functions[0].Output)
		assert.False(t, tool.Tools[0].Functions[0].Output.AllowInference, "Expected allowInference to be false by default")
	})

	t.Run("allowInference can be set to true", func(t *testing.T) {
		yaml := `
version: "v1"
author: "test"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
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
            onError:
              strategy: "requestUserInput"
              message: "Please provide input"
        steps:
          - name: "step1"
            action: "GET"
            with:
              url: "https://example.com"
        output:
          type: "object"
          allowInference: true
          fields:
            - "id"
            - "name"
`
		tool, err := CreateTool(yaml)
		require.NoError(t, err)
		require.NotNil(t, tool.Tools[0].Functions[0].Output)
		assert.True(t, tool.Tools[0].Functions[0].Output.AllowInference, "Expected allowInference to be true")
	})

	t.Run("allowInference can be explicitly set to false", func(t *testing.T) {
		yaml := `
version: "v1"
author: "test"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
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
            onError:
              strategy: "requestUserInput"
              message: "Please provide input"
        steps:
          - name: "step1"
            action: "GET"
            with:
              url: "https://example.com"
        output:
          type: "object"
          allowInference: false
          fields:
            - "id"
            - "name"
`
		tool, err := CreateTool(yaml)
		require.NoError(t, err)
		require.NotNil(t, tool.Tools[0].Functions[0].Output)
		assert.False(t, tool.Tools[0].Functions[0].Output.AllowInference, "Expected allowInference to be false")
	})

	t.Run("allowInference with string output type", func(t *testing.T) {
		yaml := `
version: "v1"
author: "test"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
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
            onError:
              strategy: "requestUserInput"
              message: "Please provide input"
        steps:
          - name: "step1"
            action: "GET"
            resultIndex: 1
            with:
              url: "https://example.com"
        output:
          type: "string"
          value: "result[1].id"
          allowInference: true
`
		tool, err := CreateTool(yaml)
		require.NoError(t, err)
		require.NotNil(t, tool.Tools[0].Functions[0].Output)
		assert.True(t, tool.Tools[0].Functions[0].Output.AllowInference, "Expected allowInference to be true for string output")
	})

	t.Run("allowInference with list[object] output type", func(t *testing.T) {
		yaml := `
version: "v1"
author: "test"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
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
            onError:
              strategy: "requestUserInput"
              message: "Please provide input"
        steps:
          - name: "step1"
            action: "GET"
            with:
              url: "https://example.com"
        output:
          type: "list[object]"
          allowInference: true
          fields:
            - "id"
            - "name"
`
		tool, err := CreateTool(yaml)
		require.NoError(t, err)
		require.NotNil(t, tool.Tools[0].Functions[0].Output)
		assert.True(t, tool.Tools[0].Functions[0].Output.AllowInference, "Expected allowInference to be true for list[object] output")
	})

	t.Run("allowInference combined with flatten", func(t *testing.T) {
		yaml := `
version: "v1"
author: "test"
tools:
  - name: "TestTool"
    description: "A test tool"
    version: "1.0.0"
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
            onError:
              strategy: "requestUserInput"
              message: "Please provide input"
        steps:
          - name: "step1"
            action: "GET"
            with:
              url: "https://example.com"
        output:
          type: "list[object]"
          allowInference: true
          flatten: true
          fields:
            - "id"
            - "name"
`
		tool, err := CreateTool(yaml)
		require.NoError(t, err)
		require.NotNil(t, tool.Tools[0].Functions[0].Output)
		assert.True(t, tool.Tools[0].Functions[0].Output.AllowInference, "Expected allowInference to be true")
		assert.True(t, tool.Tools[0].Functions[0].Output.Flatten, "Expected flatten to be true")
	})
}
