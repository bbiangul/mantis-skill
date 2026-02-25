package engine

import (
	"regexp"
	"strings"

	"github.com/bbiangul/mantis-skill/skill"
)

// SupportedExpressionFunctions lists all functions that can be evaluated in expressions.
// These are provided by skill and include: len, isEmpty, contains, exists, coalesce
var SupportedExpressionFunctions = []string{"len", "isEmpty", "contains", "exists", "coalesce", "date"}

// ExpressionEvaluator handles function-like expressions in text.
// It uses skill's built-in function evaluation for consistency.
type ExpressionEvaluator struct {
	funcPattern *regexp.Regexp
}

// NewExpressionEvaluator creates a new ExpressionEvaluator
func NewExpressionEvaluator() *ExpressionEvaluator {
	// Build regex pattern for all supported functions
	// Uses word boundary \b to avoid matching partial words like "mycoalesce"
	funcNames := make([]string, 0, len(SupportedExpressionFunctions))
	for _, name := range SupportedExpressionFunctions {
		funcNames = append(funcNames, regexp.QuoteMeta(name))
	}
	pattern := `\b(` + strings.Join(funcNames, "|") + `)\(`

	return &ExpressionEvaluator{
		funcPattern: regexp.MustCompile(pattern),
	}
}

// Evaluate finds and evaluates all function expressions in the text.
// Supports nested function calls by evaluating innermost functions first.
// Uses skill's EvaluateBuiltinFunction for consistent behavior with deterministic expressions.
func (e *ExpressionEvaluator) Evaluate(text string) string {
	if e.funcPattern == nil {
		return text
	}

	// Keep evaluating until no more functions are found (handles nested calls)
	maxIterations := 100 // Prevent infinite loops
	for i := 0; i < maxIterations; i++ {
		// Find all matches and process the LAST one first (innermost)
		// This ensures nested functions are evaluated from inside out
		matches := e.funcPattern.FindAllStringIndex(text, -1)
		if len(matches) == 0 {
			break
		}

		// Process the last match (innermost function in nested calls)
		match := matches[len(matches)-1]

		// Find the matching closing parenthesis
		funcStart := match[0]
		funcNameEnd := match[1] - 1 // Position of '('

		// Find matching closing paren
		argsStart := funcNameEnd + 1
		closeIdx := findMatchingParen(text, argsStart)
		if closeIdx == -1 {
			// Malformed expression, stop processing
			break
		}

		// Extract the full expression (e.g., "coalesce(null, 'value')")
		fullExpr := text[funcStart : closeIdx+1]

		// Evaluate using skill
		result, handled, err := skill.EvaluateBuiltinFunction(fullExpr)
		if err != nil {
			// On error, leave the expression as-is
			break
		}
		if !handled {
			// Not a recognized function, should not happen but be safe
			break
		}

		// Replace the function call with its result
		text = strings.Replace(text, fullExpr, result, 1)
	}

	return text
}

// findMatchingParen finds the index of the closing parenthesis that matches
// the opening one at the given position. Returns -1 if not found.
func findMatchingParen(text string, openParenPos int) int {
	depth := 1
	inQuote := false
	quoteChar := rune(0)

	for i := openParenPos; i < len(text); i++ {
		ch := rune(text[i])

		switch {
		case (ch == '"' || ch == '\'') && (i == 0 || text[i-1] != '\\'):
			if !inQuote {
				inQuote = true
				quoteChar = ch
			} else if ch == quoteChar {
				inQuote = false
			}
		case ch == '(' && !inQuote:
			depth++
		case ch == ')' && !inQuote:
			depth--
			if depth == 0 {
				return i
			}
		}
	}

	return -1
}
