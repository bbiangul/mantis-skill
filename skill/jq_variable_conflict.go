package skill

import (
	"regexp"
	"strings"
)

// JqVariableConflict represents a detected conflict between a jq variable name
// and a YAML input parameter name. This is a critical issue because the framework
// replaces ALL $variableName patterns in bash scripts BEFORE execution.
type JqVariableConflict struct {
	VarName   string // The conflicting variable name
	Location  string // Where it was found (e.g., "--argjson", "--arg", "as $")
	Line      int    // Line number in the script (0 if not determinable)
	InputName string // The input parameter it conflicts with
}

// jqBuiltinVars contains jq built-in special variables that should not trigger conflicts
var jqBuiltinVars = map[string]bool{
	"__loc__":     true, // Current source location
	"__prog__":    true, // Program being executed (jq 1.7+)
	"ENV":         true, // Environment variables object
	"__version__": true, // jq version (hypothetical, for future-proofing)
}

// DetectJqVariableConflicts scans a bash script for jq variable declarations
// that match input parameter names. Such conflicts cause bugs because the
// framework's variable replacement happens BEFORE bash execution, breaking jq syntax.
//
// Example of the bug this prevents:
//
//	Input parameter: dealbreakerAnalysis
//	jq expression: ... ) as $dealbreakerAnalysis | ...
//	After framework replacement: ... ) as [{"found":true,...}] | ...
//	Result: jq syntax error
//
// Detection covers:
//   - jq --argjson varName
//   - jq --arg varName
//   - jq --slurpfile varName
//   - jq --rawfile varName
//   - jq '... as $varName | ...'
//   - jq 'reduce ... as $varName ...'
//
// Does NOT flag:
//   - jq built-in variables ($__loc__, $ENV, etc.)
//   - jq function definition parameters (def f(x): ...)
//   - Bash variables, awk variables, or other non-jq patterns
func DetectJqVariableConflicts(inputParams []string, bashScript string) []JqVariableConflict {
	if len(inputParams) == 0 || bashScript == "" {
		return nil
	}

	// Build a set of input parameter names for O(1) lookup
	inputSet := make(map[string]bool, len(inputParams))
	for _, param := range inputParams {
		// Skip jq built-in variable names - they shouldn't be input params anyway,
		// but if they are, don't flag them
		if jqBuiltinVars[param] {
			continue
		}
		inputSet[param] = true
	}

	if len(inputSet) == 0 {
		return nil
	}

	var conflicts []JqVariableConflict

	// Remove comments to avoid false positives
	scriptWithoutComments := removeShellComments(bashScript)

	// Pattern 1: jq --argjson/--arg/--slurpfile/--rawfile varName
	// Matches: --argjson varName, --arg varName, etc.
	// Allows whitespace including tabs between flag and variable name
	jqArgPattern := regexp.MustCompile(`--(argjson|arg|slurpfile|rawfile)[\s\t]+([a-zA-Z_][a-zA-Z0-9_]*)`)
	matches := jqArgPattern.FindAllStringSubmatch(scriptWithoutComments, -1)
	for _, match := range matches {
		if len(match) >= 3 {
			argType := match[1]
			varName := match[2]
			if inputSet[varName] && !jqBuiltinVars[varName] {
				conflicts = append(conflicts, JqVariableConflict{
					VarName:   varName,
					Location:  "--" + argType,
					InputName: varName,
				})
			}
		}
	}

	// Pattern 2: as $varName (jq variable binding)
	// This is trickier - we need to find jq expressions and look for 'as $varName'
	// Matches: ) as $varName, . as $varName, etc.
	// The \b ensures we match 'as' as a complete word (not 'has' or 'alias')
	asVarPattern := regexp.MustCompile(`\bas[\s\t]+\$([a-zA-Z_][a-zA-Z0-9_]*)`)
	asMatches := asVarPattern.FindAllStringSubmatch(scriptWithoutComments, -1)
	for _, match := range asMatches {
		if len(match) >= 2 {
			varName := match[1]
			if inputSet[varName] && !jqBuiltinVars[varName] {
				conflicts = append(conflicts, JqVariableConflict{
					VarName:   varName,
					Location:  "as $",
					InputName: varName,
				})
			}
		}
	}

	// Deduplicate conflicts (same var might be used multiple times)
	return deduplicateConflicts(conflicts)
}

// removeShellComments removes shell-style comments from the script
// to avoid false positives from commented-out code.
func removeShellComments(script string) string {
	lines := strings.Split(script, "\n")
	var result []string

	for _, line := range lines {
		// Find # that's not inside quotes
		cleanLine := removeCommentFromLine(line)
		result = append(result, cleanLine)
	}

	return strings.Join(result, "\n")
}

// removeCommentFromLine removes the comment portion of a line,
// being careful not to remove # inside strings.
func removeCommentFromLine(line string) string {
	// Simple approach: if line starts with # (possibly with whitespace), it's a comment
	trimmed := strings.TrimSpace(line)
	if strings.HasPrefix(trimmed, "#") {
		return ""
	}

	// For inline comments, we need to be more careful
	// This is a simplified version - it doesn't handle all edge cases
	// but is good enough for our purposes
	inSingleQuote := false
	inDoubleQuote := false

	for i, ch := range line {
		switch ch {
		case '\'':
			if !inDoubleQuote {
				inSingleQuote = !inSingleQuote
			}
		case '"':
			if !inSingleQuote {
				inDoubleQuote = !inDoubleQuote
			}
		case '#':
			if !inSingleQuote && !inDoubleQuote {
				return line[:i]
			}
		}
	}

	return line
}

// deduplicateConflicts removes duplicate conflicts (same varName and location)
func deduplicateConflicts(conflicts []JqVariableConflict) []JqVariableConflict {
	seen := make(map[string]bool)
	var result []JqVariableConflict

	for _, c := range conflicts {
		key := c.VarName + "|" + c.Location
		if !seen[key] {
			seen[key] = true
			result = append(result, c)
		}
	}

	return result
}

// ValidateFunctionJqVariableConflicts checks a function for jq variable conflicts
// and returns warnings for any found. This should be called during parsing validation.
func ValidateFunctionJqVariableConflicts(function Function, toolName string) []string {
	var warnings []string

	// Collect input parameter names
	var inputParams []string
	for _, input := range function.Input {
		inputParams = append(inputParams, input.Name)
	}

	if len(inputParams) == 0 {
		return nil
	}

	// Check each step for bash scripts
	for _, step := range function.Steps {
		if step.Action != "bash" && step.Action != "terminal" {
			continue
		}

		// Get the linux script
		var bashScript string
		if step.With != nil {
			if linux, ok := step.With["linux"].(string); ok {
				bashScript = linux
			}
		}

		if bashScript == "" {
			continue
		}

		conflicts := DetectJqVariableConflicts(inputParams, bashScript)
		for _, conflict := range conflicts {
			warning := formatConflictWarning(function.Name, toolName, step.Name, conflict)
			warnings = append(warnings, warning)
		}
	}

	return warnings
}

// formatConflictWarning creates a human-readable warning message
func formatConflictWarning(funcName, toolName, stepName string, conflict JqVariableConflict) string {
	return "jq variable conflict in function '" + funcName + "' (tool '" + toolName + "'), step '" + stepName + "': " +
		"jq variable '" + conflict.VarName + "' (declared via " + conflict.Location + ") matches input parameter name. " +
		"The framework will replace $" + conflict.VarName + " with actual data BEFORE bash execution, breaking jq syntax. " +
		"Rename the jq variable to avoid conflict (e.g., use abbreviated name like '$" + suggestAlternativeName(conflict.VarName) + "')."
}

// suggestAlternativeName suggests an abbreviated alternative name
func suggestAlternativeName(varName string) string {
	// Common patterns and their abbreviations
	replacements := map[string]string{
		"Analysis":     "Result",
		"analysis":     "Res",
		"Reviews":      "Revs",
		"reviews":      "revs",
		"Data":         "D",
		"data":         "d",
		"Availability": "Avail",
		"availability": "avail",
	}

	result := varName
	for old, new := range replacements {
		if strings.Contains(result, old) {
			result = strings.Replace(result, old, new, 1)
			break
		}
	}

	// If no replacement found, just add a suffix
	if result == varName {
		// Try to abbreviate by taking first few chars
		if len(varName) > 6 {
			// CamelCase abbreviation: take first letter of each word
			var abbrev strings.Builder
			abbrev.WriteByte(varName[0])
			for i := 1; i < len(varName); i++ {
				if varName[i] >= 'A' && varName[i] <= 'Z' {
					abbrev.WriteByte(byte(varName[i] + 32)) // lowercase
				}
			}
			if abbrev.Len() > 1 {
				result = abbrev.String()
			} else {
				result = varName[:3] + "Var"
			}
		}
	}

	return result
}
