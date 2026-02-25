// Package sanitizer provides prompt injection protection for tool protocol outputs.
//
// It sanitizes content from external sources (emails, web pages, user-submitted text)
// before it enters the LLM agent's conversation thread, preventing malicious content
// from hijacking the agent's behavior.
//
// Three strategies are supported:
//   - "fence": Wraps content with nonce-based delimiters (lightest, no content modification)
//   - "strict": Fence + regex pattern stripping of known injection patterns
//   - "llm_extract": Sends content to a separate LLM for structured field extraction (most robust)
package sanitizer

import (
	"context"
	"fmt"

	"github.com/bbiangul/mantis-skill/skill"
)

// Sanitize applies the configured sanitization strategy to content.
// It dispatches to the appropriate strategy based on config.Strategy:
//   - "fence"       → Fence(content, nonce)
//   - "strict"      → StripPatterns(content) then Fence(result, nonce)
//   - "llm_extract" → LLMExtract(content, fields) then Fence(result, nonce)
//
// If config.MaxLength > 0, content is truncated before processing.
// The nonce should be generated once per workflow session via GenerateNonce().
func Sanitize(ctx context.Context, content string, config *skill.SanitizeConfig, nonce string, extractFn LLMExtractFunc) (string, error) {
	if config == nil {
		return content, nil
	}

	// Apply max length truncation if configured
	if config.MaxLength > 0 && len(content) > config.MaxLength {
		content = content[:config.MaxLength] + "\n[TRUNCATED - content exceeded maximum length]"
	}

	switch config.Strategy {
	case skill.SanitizeStrategyFence:
		return Fence(content, nonce), nil

	case skill.SanitizeStrategyStrict:
		stripped := StripPatterns(content, config.CustomPatterns)
		return Fence(stripped, nonce), nil

	case skill.SanitizeStrategyLLMExtract:
		extracted, err := LLMExtract(ctx, content, config.Extract, extractFn)
		if err != nil {
			// Fall back to strict mode if extraction fails
			stripped := StripPatterns(content, nil)
			return Fence(stripped, nonce), fmt.Errorf("llm_extract failed, falling back to strict: %w", err)
		}
		return Fence(extracted, nonce), nil

	default:
		return content, fmt.Errorf("unknown sanitize strategy: %s", config.Strategy)
	}
}

// SanitizeSimple applies sanitization without LLM extraction support.
// Use this for fence and strict strategies where no LLM call is needed.
func SanitizeSimple(content string, config *skill.SanitizeConfig, nonce string) string {
	if config == nil {
		return content
	}

	// Apply max length truncation if configured
	if config.MaxLength > 0 && len(content) > config.MaxLength {
		content = content[:config.MaxLength] + "\n[TRUNCATED - content exceeded maximum length]"
	}

	switch config.Strategy {
	case skill.SanitizeStrategyFence:
		return Fence(content, nonce)

	case skill.SanitizeStrategyStrict:
		stripped := StripPatterns(content, config.CustomPatterns)
		return Fence(stripped, nonce)

	default:
		// For llm_extract or unknown, fall back to fence
		return Fence(content, nonce)
	}
}
