package skill

import (
	"strings"
	"testing"
)

func TestValidateTerminalOperation(t *testing.T) {
	tests := []struct {
		name     string
		function Function
		toolName string
		wantErr  bool
	}{
		{
			name: "valid terminal operation",
			function: Function{
				Name: "testFunction",
				Steps: []Step{
					{
						Name:   "test step",
						Action: StepActionSh,
						With: map[string]interface{}{
							StepWithLinux:   "echo 'hello'",
							StepWithWindows: "echo hello",
						},
					},
				},
			},
			toolName: "testTool",
			wantErr:  false,
		},
		{
			name: "valid terminal operation with bash",
			function: Function{
				Name: "testFunction",
				Steps: []Step{
					{
						Name:   "test step",
						Action: StepActionBash,
						With: map[string]interface{}{
							StepWithLinux:   "echo 'hello'",
							StepWithWindows: "echo hello",
							StepWithMacOS:   "echo 'hello'",
						},
					},
				},
			},
			toolName: "testTool",
			wantErr:  false,
		},
		{
			name: "missing steps",
			function: Function{
				Name:  "testFunction",
				Steps: []Step{},
			},
			toolName: "testTool",
			wantErr:  true,
		},
		{
			name: "invalid step action",
			function: Function{
				Name: "testFunction",
				Steps: []Step{
					{
						Name:   "test step",
						Action: "invalid",
						With: map[string]interface{}{
							StepWithLinux:   "echo 'hello'",
							StepWithWindows: "echo hello",
						},
					},
				},
			},
			toolName: "testTool",
			wantErr:  true,
		},
		{
			name: "missing with block",
			function: Function{
				Name: "testFunction",
				Steps: []Step{
					{
						Name:   "test step",
						Action: StepActionSh,
					},
				},
			},
			toolName: "testTool",
			wantErr:  true,
		},
		{
			name: "missing linux script",
			function: Function{
				Name: "testFunction",
				Steps: []Step{
					{
						Name:   "test step",
						Action: StepActionSh,
						With: map[string]interface{}{
							StepWithWindows: "echo hello",
						},
					},
				},
			},
			toolName: "testTool",
			wantErr:  true,
		},
		{
			name: "missing windows script",
			function: Function{
				Name: "testFunction",
				Steps: []Step{
					{
						Name:   "test step",
						Action: StepActionSh,
						With: map[string]interface{}{
							StepWithLinux: "echo 'hello'",
						},
					},
				},
			},
			toolName: "testTool",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateTerminalOperation(tt.function, tt.toolName)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateTerminalOperation() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateTerminalStep(t *testing.T) {
	tests := []struct {
		name         string
		step         Step
		functionName string
		toolName     string
		wantErr      bool
	}{
		{
			name: "valid sh step",
			step: Step{
				Name:   "test step",
				Action: StepActionSh,
				With: map[string]interface{}{
					StepWithLinux:   "echo 'hello'",
					StepWithWindows: "echo hello",
				},
			},
			functionName: "testFunction",
			toolName:     "testTool",
			wantErr:      false,
		},
		{
			name: "valid bash step",
			step: Step{
				Name:   "test step",
				Action: StepActionBash,
				With: map[string]interface{}{
					StepWithLinux:   "echo 'hello'",
					StepWithWindows: "echo hello",
					StepWithMacOS:   "echo 'hello'",
				},
			},
			functionName: "testFunction",
			toolName:     "testTool",
			wantErr:      false,
		},
		{
			name: "valid step with timeout",
			step: Step{
				Name:   "test step",
				Action: StepActionSh,
				With: map[string]interface{}{
					StepWithLinux:   "echo 'hello'",
					StepWithWindows: "echo hello",
					StepWithTimeout: 45,
				},
			},
			functionName: "testFunction",
			toolName:     "testTool",
			wantErr:      false,
		},
		{
			name: "invalid action",
			step: Step{
				Name:   "test step",
				Action: "invalid",
				With: map[string]interface{}{
					StepWithLinux:   "echo 'hello'",
					StepWithWindows: "echo hello",
				},
			},
			functionName: "testFunction",
			toolName:     "testTool",
			wantErr:      true,
		},
		{
			name: "missing with block",
			step: Step{
				Name:   "test step",
				Action: StepActionSh,
			},
			functionName: "testFunction",
			toolName:     "testTool",
			wantErr:      true,
		},
		{
			name: "invalid field in with block",
			step: Step{
				Name:   "test step",
				Action: StepActionSh,
				With: map[string]interface{}{
					StepWithLinux:   "echo 'hello'",
					StepWithWindows: "echo hello",
					"invalid":       "should not be here",
				},
			},
			functionName: "testFunction",
			toolName:     "testTool",
			wantErr:      true,
		},
		{
			name: "missing required scripts",
			step: Step{
				Name:   "test step",
				Action: StepActionSh,
				With: map[string]interface{}{
					StepWithLinux: "echo 'hello'",
				},
			},
			functionName: "testFunction",
			toolName:     "testTool",
			wantErr:      true,
		},
		{
			name: "invalid timeout value (zero)",
			step: Step{
				Name:   "test step",
				Action: StepActionSh,
				With: map[string]interface{}{
					StepWithLinux:   "echo 'hello'",
					StepWithWindows: "echo hello",
					StepWithTimeout: 0,
				},
			},
			functionName: "testFunction",
			toolName:     "testTool",
			wantErr:      true,
		},
		{
			name: "invalid timeout value (negative)",
			step: Step{
				Name:   "test step",
				Action: StepActionSh,
				With: map[string]interface{}{
					StepWithLinux:   "echo 'hello'",
					StepWithWindows: "echo hello",
					StepWithTimeout: -5,
				},
			},
			functionName: "testFunction",
			toolName:     "testTool",
			wantErr:      true,
		},
		{
			name: "invalid timeout type (string)",
			step: Step{
				Name:   "test step",
				Action: StepActionSh,
				With: map[string]interface{}{
					StepWithLinux:   "echo 'hello'",
					StepWithWindows: "echo hello",
					StepWithTimeout: "invalid",
				},
			},
			functionName: "testFunction",
			toolName:     "testTool",
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateTerminalStep(tt.step, tt.functionName, tt.toolName)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateTerminalStep() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestStripQuotedStrings(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "no quotes",
			input:    "echo hello world",
			expected: "echo hello world",
		},
		{
			name:     "double quotes",
			input:    `echo "hello world"`,
			expected: `echo ""`,
		},
		{
			name:     "single quotes",
			input:    `echo 'hello world'`,
			expected: `echo ''`,
		},
		{
			name:     "mixed quotes",
			input:    `echo "hello" and 'world'`,
			expected: `echo "" and ''`,
		},
		{
			name:     "escaped quotes in double quotes",
			input:    `echo "He said \"hello\""`,
			expected: `echo ""`,
		},
		{
			name:     "At Risk in JSON",
			input:    `printf '{"status":"At Risk"}'`,
			expected: `printf ''`,
		},
		{
			name:     "multiple quoted strings",
			input:    `echo "first" then "second" and 'third'`,
			expected: `echo "" then "" and ''`,
		},
		{
			name:     "command with quoted unsafe word",
			input:    `echo "do not use curl here"`,
			expected: `echo ""`,
		},
		{
			name:     "preserves unquoted content",
			input:    `actual_command "with quoted arg"`,
			expected: `actual_command ""`,
		},
		{
			name: "multiline script",
			input: `#!/bin/bash
status="At Risk"
echo "$status"`,
			expected: `
status=""
echo ""`,
		},
		// ===== COMMENT STRIPPING TESTS =====
		{
			name: "comment line should be stripped",
			input: `# This is a comment
echo "hello"`,
			expected: `
echo ""`,
		},
		{
			name:     "inline comment should be stripped",
			input:    `echo "hello" # this is an inline comment`,
			expected: `echo "" `,
		},
		{
			name: "comment with unsafe word 'at' should be stripped",
			input: `# Get flight at rank (0-indexed)
INDEX=$((RANK - 1))`,
			expected: `
INDEX=$((RANK - 1))`,
		},
		{
			name: "multiple comments should all be stripped",
			input: `# First comment
echo "test"
# Second comment with curl mentioned
ls -la`,
			expected: `
echo ""

ls -la`,
		},
		{
			name: "shebang is a comment and should be stripped",
			input: `#!/bin/bash
# at the start of this script
echo "done"`,
			expected: `

echo ""`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := stripQuotedStrings(tt.input)
			if result != tt.expected {
				t.Errorf("stripQuotedStrings() = %q, expected %q", result, tt.expected)
			}
		})
	}
}

func TestValidateScriptSafety(t *testing.T) {
	tests := []struct {
		name         string
		script       string
		osName       string
		functionName string
		toolName     string
		stepName     string
		wantErr      bool
	}{
		{
			name:         "safe script",
			script:       "echo 'hello world'",
			osName:       "linux",
			functionName: "testFunction",
			toolName:     "testTool",
			stepName:     "testStep",
			wantErr:      false,
		},
		{
			name:         "safe multi-line script",
			script:       "echo 'hello'\nls -la\nps aux",
			osName:       "linux",
			functionName: "testFunction",
			toolName:     "testTool",
			stepName:     "testStep",
			wantErr:      false,
		},
		{
			name:         "unsafe rm -rf command",
			script:       "rm -rf /tmp/test",
			osName:       "linux",
			functionName: "testFunction",
			toolName:     "testTool",
			stepName:     "testStep",
			wantErr:      true,
		},
		{
			name:         "unsafe sudo command",
			script:       "sudo apt install package",
			osName:       "linux",
			functionName: "testFunction",
			toolName:     "testTool",
			stepName:     "testStep",
			wantErr:      true,
		},
		{
			name:         "unsafe wget command",
			script:       "wget http://example.com/file.sh",
			osName:       "linux",
			functionName: "testFunction",
			toolName:     "testTool",
			stepName:     "testStep",
			wantErr:      true,
		},
		{
			name:         "unsafe curl command",
			script:       "curl -O http://example.com/file.sh",
			osName:       "linux",
			functionName: "testFunction",
			toolName:     "testTool",
			stepName:     "testStep",
			wantErr:      true,
		},
		{
			name:         "unsafe shutdown command",
			script:       "shutdown -h now",
			osName:       "linux",
			functionName: "testFunction",
			toolName:     "testTool",
			stepName:     "testStep",
			wantErr:      true,
		},
		{
			name:         "unsafe windows del command",
			script:       "del /s C:\\Windows",
			osName:       "windows",
			functionName: "testFunction",
			toolName:     "testTool",
			stepName:     "testStep",
			wantErr:      true,
		},
		{
			name:         "format command is currently allowed (pattern commented out)",
			script:       "format C:",
			osName:       "windows",
			functionName: "testFunction",
			toolName:     "testTool",
			stepName:     "testStep",
			wantErr:      false, // NOTE: format pattern is intentionally commented out in parser.go
		},
		{
			name:         "unsafe taskkill command",
			script:       "taskkill /f /im explorer.exe",
			osName:       "windows",
			functionName: "testFunction",
			toolName:     "testTool",
			stepName:     "testStep",
			wantErr:      true,
		},
		{
			name:         "unsafe path manipulation",
			script:       "export PATH=/malicious/path:$PATH",
			osName:       "linux",
			functionName: "testFunction",
			toolName:     "testTool",
			stepName:     "testStep",
			wantErr:      true,
		},
		{
			name:         "unsafe file redirection",
			script:       "echo malicious > /etc/passwd",
			osName:       "linux",
			functionName: "testFunction",
			toolName:     "testTool",
			stepName:     "testStep",
			wantErr:      true,
		},
		{
			name:         "case insensitive detection",
			script:       "SUDO apt install package",
			osName:       "linux",
			functionName: "testFunction",
			toolName:     "testTool",
			stepName:     "testStep",
			wantErr:      true,
		},
		{
			name:         "mixed case detection",
			script:       "ShUtDoWn -h now",
			osName:       "linux",
			functionName: "testFunction",
			toolName:     "testTool",
			stepName:     "testStep",
			wantErr:      true,
		},
		// ===== FALSE POSITIVE TESTS - Quoted strings should NOT trigger =====
		{
			name:         "safe: 'At Risk' in double quotes should NOT trigger at detection",
			script:       `printf '{"status":"At Risk"}' "$var"`,
			osName:       "linux",
			functionName: "testFunction",
			toolName:     "testTool",
			stepName:     "testStep",
			wantErr:      false,
		},
		{
			name:         "safe: 'at risk' lowercase in double quotes",
			script:       `echo "Status: at risk"`,
			osName:       "linux",
			functionName: "testFunction",
			toolName:     "testTool",
			stepName:     "testStep",
			wantErr:      false,
		},
		{
			name:         "safe: single quoted string with 'at ' inside",
			script:       `echo 'meeting at noon'`,
			osName:       "linux",
			functionName: "testFunction",
			toolName:     "testTool",
			stepName:     "testStep",
			wantErr:      false,
		},
		{
			name:         "safe: 'sudo' mentioned in double quoted string",
			script:       `echo "Do not use sudo here"`,
			osName:       "linux",
			functionName: "testFunction",
			toolName:     "testTool",
			stepName:     "testStep",
			wantErr:      false,
		},
		{
			name:         "safe: 'curl' mentioned in single quoted JSON",
			script:       `printf '{"error":"curl command not allowed"}'`,
			osName:       "linux",
			functionName: "testFunction",
			toolName:     "testTool",
			stepName:     "testStep",
			wantErr:      false,
		},
		{
			name:         "safe: 'wget' in error message",
			script:       `echo "wget is disabled in this environment"`,
			osName:       "linux",
			functionName: "testFunction",
			toolName:     "testTool",
			stepName:     "testStep",
			wantErr:      false,
		},
		{
			name:         "safe: 'rm -rf' in warning message",
			script:       `echo "Warning: rm -rf is dangerous"`,
			osName:       "linux",
			functionName: "testFunction",
			toolName:     "testTool",
			stepName:     "testStep",
			wantErr:      false,
		},
		{
			name:         "safe: 'shutdown' in JSON output",
			script:       `printf '{"action":"shutdown","status":"prevented"}'`,
			osName:       "linux",
			functionName: "testFunction",
			toolName:     "testTool",
			stepName:     "testStep",
			wantErr:      false,
		},
		{
			name: "safe: complex script with multiple quoted unsafe words",
			script: `#!/bin/bash
export LC_ALL=C.UTF-8
status="At Risk"
message="Warning: do not use sudo or curl"
printf '{"status":"%s","message":"%s"}' "$status" "$message"`,
			osName:       "linux",
			functionName: "testFunction",
			toolName:     "testTool",
			stepName:     "testStep",
			wantErr:      false,
		},
		{
			name: "safe: real altabev script pattern with At Risk",
			script: `#!/bin/bash
export LC_ALL=C.UTF-8
export LANG=C.UTF-8

printf '{"refFileId":"%s","cancellationType":"%s","status":"handled","tsStatus":"At Risk"}' "$refFileId" "$cancellationType"`,
			osName:       "linux",
			functionName: "testFunction",
			toolName:     "testTool",
			stepName:     "testStep",
			wantErr:      false,
		},
		{
			name:         "safe: escaped quotes inside double quotes",
			script:       `echo "He said \"at that moment\" something"`,
			osName:       "linux",
			functionName: "testFunction",
			toolName:     "testTool",
			stepName:     "testStep",
			wantErr:      false,
		},
		// ===== COMMENT FALSE POSITIVES - Should NOT trigger =====
		{
			name: "safe: 'at ' in bash comment should NOT trigger",
			script: `# Get flight at rank (0-indexed)
INDEX=$((RANK - 1))`,
			osName:       "linux",
			functionName: "testFunction",
			toolName:     "testTool",
			stepName:     "testStep",
			wantErr:      false,
		},
		{
			name: "safe: 'curl' mentioned in comment should NOT trigger",
			script: `# Do not use curl directly, use api_call instead
echo "fetching data"`,
			osName:       "linux",
			functionName: "testFunction",
			toolName:     "testTool",
			stepName:     "testStep",
			wantErr:      false,
		},
		{
			name: "safe: 'sudo' mentioned in comment should NOT trigger",
			script: `# Warning: never use sudo in production
echo "running as user"`,
			osName:       "linux",
			functionName: "testFunction",
			toolName:     "testTool",
			stepName:     "testStep",
			wantErr:      false,
		},
		{
			name: "safe: multiple comments with unsafe words should NOT trigger",
			script: `#!/bin/bash
# This script should not use curl or wget
# Run at your own risk
# Do not sudo anything
echo "safe script"`,
			osName:       "linux",
			functionName: "testFunction",
			toolName:     "testTool",
			stepName:     "testStep",
			wantErr:      false,
		},
		{
			name:         "safe: inline comment with unsafe word should NOT trigger",
			script:       `echo "hello" # at this point we're done`,
			osName:       "linux",
			functionName: "testFunction",
			toolName:     "testTool",
			stepName:     "testStep",
			wantErr:      false,
		},
		{
			name:         "safe: flight_data variable should NOT trigger (no word boundary for 'at ')",
			script:       `FLIGHT_DATA=$(echo "$FLIGHT_RESULTS" | jq -r '.flight_data // empty')`,
			osName:       "linux",
			functionName: "testFunction",
			toolName:     "testTool",
			stepName:     "testStep",
			wantErr:      false,
		},
		// ===== ACTUAL UNSAFE COMMANDS - Should still trigger =====
		{
			name:         "unsafe: actual at command usage",
			script:       `at now + 1 hour < script.sh`,
			osName:       "linux",
			functionName: "testFunction",
			toolName:     "testTool",
			stepName:     "testStep",
			wantErr:      true,
		},
		{
			name:         "unsafe: at command with time",
			script:       `at 10:00 tomorrow < backup.sh`,
			osName:       "linux",
			functionName: "testFunction",
			toolName:     "testTool",
			stepName:     "testStep",
			wantErr:      true,
		},
		{
			name:         "unsafe: at command after semicolon",
			script:       `echo "done"; at noon < task.sh`,
			osName:       "linux",
			functionName: "testFunction",
			toolName:     "testTool",
			stepName:     "testStep",
			wantErr:      true,
		},
		{
			name:         "unsafe: curl outside quotes with quoted arg",
			script:       `curl "http://example.com"`,
			osName:       "linux",
			functionName: "testFunction",
			toolName:     "testTool",
			stepName:     "testStep",
			wantErr:      true,
		},
		{
			name:         "unsafe: sudo with quoted argument",
			script:       `sudo apt install "package-name"`,
			osName:       "linux",
			functionName: "testFunction",
			toolName:     "testTool",
			stepName:     "testStep",
			wantErr:      true,
		},
		{
			name: "unsafe: mixed safe quoted and unsafe unquoted",
			script: `echo "I will not use curl"
curl http://example.com`,
			osName:       "linux",
			functionName: "testFunction",
			toolName:     "testTool",
			stepName:     "testStep",
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateScriptSafety(tt.script, tt.osName, tt.functionName, tt.toolName, tt.stepName)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateScriptSafety() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateTerminalScripts(t *testing.T) {
	tests := []struct {
		name         string
		stepWith     map[string]interface{}
		functionName string
		toolName     string
		stepName     string
		wantErr      bool
	}{
		{
			name: "valid scripts",
			stepWith: map[string]interface{}{
				StepWithLinux:   "echo 'hello'",
				StepWithWindows: "echo hello",
				StepWithMacOS:   "echo 'hello'",
			},
			functionName: "testFunction",
			toolName:     "testTool",
			stepName:     "testStep",
			wantErr:      false,
		},
		{
			name: "invalid script type",
			stepWith: map[string]interface{}{
				StepWithLinux:   123,
				StepWithWindows: "echo hello",
			},
			functionName: "testFunction",
			toolName:     "testTool",
			stepName:     "testStep",
			wantErr:      true,
		},
		{
			name: "unsafe script content",
			stepWith: map[string]interface{}{
				StepWithLinux:   "rm -rf /",
				StepWithWindows: "echo hello",
			},
			functionName: "testFunction",
			toolName:     "testTool",
			stepName:     "testStep",
			wantErr:      true,
		},
		{
			name: "mixed valid and invalid scripts",
			stepWith: map[string]interface{}{
				StepWithLinux:   "echo 'hello'",
				StepWithWindows: "del /s C:\\Windows",
			},
			functionName: "testFunction",
			toolName:     "testTool",
			stepName:     "testStep",
			wantErr:      true,
		},
		{
			name:         "empty stepWith",
			stepWith:     map[string]interface{}{},
			functionName: "testFunction",
			toolName:     "testTool",
			stepName:     "testStep",
			wantErr:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateTerminalScripts(tt.stepWith, tt.functionName, tt.toolName, tt.stepName)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateTerminalScripts() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateStep_Terminal(t *testing.T) {
	tests := []struct {
		name         string
		step         Step
		functionName string
		toolName     string
		operation    string
		wantErr      bool
	}{
		{
			name: "valid terminal step",
			step: Step{
				Name:   "test step",
				Action: StepActionSh,
				With: map[string]interface{}{
					StepWithLinux:   "echo 'hello'",
					StepWithWindows: "echo hello",
				},
			},
			functionName: "testFunction",
			toolName:     "testTool",
			operation:    OperationTerminal,
			wantErr:      false,
		},
		{
			name: "invalid terminal step action",
			step: Step{
				Name:   "test step",
				Action: "invalid",
				With: map[string]interface{}{
					StepWithLinux:   "echo 'hello'",
					StepWithWindows: "echo hello",
				},
			},
			functionName: "testFunction",
			toolName:     "testTool",
			operation:    OperationTerminal,
			wantErr:      true,
		},
		{
			name: "terminal step with unsafe script",
			step: Step{
				Name:   "test step",
				Action: StepActionSh,
				With: map[string]interface{}{
					StepWithLinux:   "sudo rm -rf /",
					StepWithWindows: "echo hello",
				},
			},
			functionName: "testFunction",
			toolName:     "testTool",
			operation:    OperationTerminal,
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateStep(tt.step, tt.functionName, tt.toolName, tt.operation)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateStep() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateFunction_Terminal(t *testing.T) {
	tests := []struct {
		name          string
		function      Function
		tool          Tool
		configEnvVars []EnvVar
		wantErr       bool
	}{
		{
			name: "valid terminal function",
			function: Function{
				Name:        "testFunction",
				Operation:   OperationTerminal,
				Description: "Test terminal function",
				Triggers: []Trigger{
					{Type: TriggerOnUserMessage},
				},
				Steps: []Step{
					{
						Name:   "test step",
						Action: StepActionSh,
						With: map[string]interface{}{
							StepWithLinux:   "echo 'hello'",
							StepWithWindows: "echo hello",
						},
					},
				},
			},
			tool: Tool{
				Name: "testTool",
			},
			configEnvVars: []EnvVar{},
			wantErr:       false,
		},
		{
			name: "terminal function with unsafe script",
			function: Function{
				Name:        "testFunction",
				Operation:   OperationTerminal,
				Description: "Test terminal function",
				Triggers: []Trigger{
					{Type: TriggerOnUserMessage},
				},
				Steps: []Step{
					{
						Name:   "test step",
						Action: StepActionSh,
						With: map[string]interface{}{
							StepWithLinux:   "rm -rf /tmp",
							StepWithWindows: "echo hello",
						},
					},
				},
			},
			tool: Tool{
				Name: "testTool",
			},
			configEnvVars: []EnvVar{},
			wantErr:       true,
		},
		{
			name: "terminal function missing required scripts",
			function: Function{
				Name:        "testFunction",
				Operation:   OperationTerminal,
				Description: "Test terminal function",
				Triggers: []Trigger{
					{Type: TriggerOnUserMessage},
				},
				Steps: []Step{
					{
						Name:   "test step",
						Action: StepActionSh,
						With: map[string]interface{}{
							StepWithLinux: "echo 'hello'",
						},
					},
				},
			},
			tool: Tool{
				Name: "testTool",
			},
			configEnvVars: []EnvVar{},
			wantErr:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateFunction(tt.function, tt.tool, tt.configEnvVars, false, nil)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateFunction() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateNoBashisms(t *testing.T) {
	tests := []struct {
		name         string
		stepWith     map[string]interface{}
		functionName string
		toolName     string
		stepName     string
		wantErr      bool
		errContains  string
	}{
		{
			name: "POSIX-compatible script should pass",
			stepWith: map[string]interface{}{
				StepWithLinux: `
					echo "hello"
					if [ -f /tmp/test ]; then
						cat /tmp/test
					fi
				`,
			},
			functionName: "testFunc",
			toolName:     "testTool",
			stepName:     "testStep",
			wantErr:      false,
		},
		{
			name: "here-string (<<<) should fail",
			stepWith: map[string]interface{}{
				StepWithLinux: `
					while read -r line; do
						echo "$line"
					done <<< "$INPUT"
				`,
			},
			functionName: "testFunc",
			toolName:     "testTool",
			stepName:     "testStep",
			wantErr:      true,
			errContains:  "here-strings",
		},
		{
			name: "extended test [[ ]] should fail",
			stepWith: map[string]interface{}{
				StepWithLinux: `
					if [[ "$VAR" == "test" ]]; then
						echo "match"
					fi
				`,
			},
			functionName: "testFunc",
			toolName:     "testTool",
			stepName:     "testStep",
			wantErr:      true,
			errContains:  "extended test",
		},
		{
			name: "pattern substitution ${var//} should fail",
			stepWith: map[string]interface{}{
				StepWithLinux: `
					VAR="hello world"
					echo "${VAR//world/universe}"
				`,
			},
			functionName: "testFunc",
			toolName:     "testTool",
			stepName:     "testStep",
			wantErr:      true,
			errContains:  "pattern substitution",
		},
		{
			name: "process substitution <() should fail",
			stepWith: map[string]interface{}{
				StepWithLinux: `
					diff <(cat file1) <(cat file2)
				`,
			},
			functionName: "testFunc",
			toolName:     "testTool",
			stepName:     "testStep",
			wantErr:      true,
			errContains:  "process substitution",
		},
		{
			name: "combined redirect &> should fail",
			stepWith: map[string]interface{}{
				StepWithLinux: `
					command &> /dev/null
				`,
			},
			functionName: "testFunc",
			toolName:     "testTool",
			stepName:     "testStep",
			wantErr:      true,
			errContains:  "combined redirect",
		},
		{
			name: "declare keyword should fail",
			stepWith: map[string]interface{}{
				StepWithLinux: `
					declare -A myarray
					myarray[key]="value"
				`,
			},
			functionName: "testFunc",
			toolName:     "testTool",
			stepName:     "testStep",
			wantErr:      true,
			errContains:  "declare keyword",
		},
		{
			name: "read -a (array) should fail",
			stepWith: map[string]interface{}{
				StepWithLinux: `
					read -a arr <<< "1 2 3"
				`,
			},
			functionName: "testFunc",
			toolName:     "testTool",
			stepName:     "testStep",
			wantErr:      true,
			errContains:  "here-strings", // This also has <<<, which will be caught first
		},
		{
			name: "ANSI-C quoting $'...' should fail",
			stepWith: map[string]interface{}{
				StepWithLinux: `
					echo $'hello\nworld'
				`,
			},
			functionName: "testFunc",
			toolName:     "testTool",
			stepName:     "testStep",
			wantErr:      true,
			errContains:  "ANSI-C quoting",
		},
		{
			name: "macos script with bashism should also fail",
			stepWith: map[string]interface{}{
				StepWithLinux: `echo "hello"`,
				StepWithMacOS: `
					while read line; do
						echo "$line"
					done <<< "$INPUT"
				`,
			},
			functionName: "testFunc",
			toolName:     "testTool",
			stepName:     "testStep",
			wantErr:      true,
			errContains:  "here-strings",
		},
		{
			name: "windows script is not checked for bashisms",
			stepWith: map[string]interface{}{
				StepWithLinux:   `echo "hello"`,
				StepWithWindows: `echo <<< "this would fail in sh but windows is not checked"`,
			},
			functionName: "testFunc",
			toolName:     "testTool",
			stepName:     "testStep",
			wantErr:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateNoBashisms(tt.stepWith, tt.functionName, tt.toolName, tt.stepName)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateNoBashisms() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && tt.errContains != "" && err != nil {
				if !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("validateNoBashisms() error = %v, should contain %q", err, tt.errContains)
				}
			}
		})
	}
}
