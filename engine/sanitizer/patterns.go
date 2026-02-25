package sanitizer

import (
	"regexp"
)

// injectionPatterns contains pre-compiled regex patterns for known prompt injection techniques.
// These are compiled once at package init for performance.
var injectionPatterns []*regexp.Regexp

func init() {
	patterns := []string{
		// Role hijacking patterns
		`(?i)ignore\s+(all\s+)?(previous|above|prior)\s+(instructions|context|rules)`,
		`(?i)you\s+are\s+now\s`,
		`(?i)your\s+(new|real|actual)\s+(role|purpose|instructions)`,
		`(?i)forget\s+(everything|all|your)`,
		`(?i)override\s+(your|all|previous|the)\s+(instructions|rules)`,

		// Action directive patterns
		`(?i)(execute|run|call|invoke|trigger)\s+(the\s+)?(tool|function|command)`,
		`(?i)respond\s+(only\s+)?with[:\s]`,
		`(?i)do\s+not\s+(mention|tell|report|say|include)`,

		// Format exploit patterns (Llama, ChatML, etc.)
		`(?s)\[INST\].*?\[/INST\]`,
		`(?s)<<SYS>>.*?<</SYS>>`,
		`(?s)<\|im_start\|>.*?<\|im_end\|>`,

		// Delimiter escape attempts
		`<<<.*?>>>`,
	}

	injectionPatterns = make([]*regexp.Regexp, 0, len(patterns))
	for _, p := range patterns {
		injectionPatterns = append(injectionPatterns, regexp.MustCompile(p))
	}
}

// StripPatterns scans content with pre-compiled injection patterns and replaces matches with [FILTERED].
// customPatterns are additional regex patterns provided via the sanitize config.
// The replacement preserves surrounding context — only the matched injection text is replaced.
func StripPatterns(content string, customPatterns []string) string {
	result := content

	// Apply built-in patterns
	for _, pattern := range injectionPatterns {
		result = pattern.ReplaceAllString(result, "[FILTERED]")
	}

	// Apply custom patterns
	for _, p := range customPatterns {
		re, err := regexp.Compile(p)
		if err != nil {
			// Skip invalid patterns (should have been caught at parse time)
			continue
		}
		result = re.ReplaceAllString(result, "[FILTERED]")
	}

	return result
}
