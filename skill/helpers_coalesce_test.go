package skill

import (
	"strings"
	"testing"
)

// TestCoalesceFunctionInDeterministicExpression tests the coalesce() function in deterministic expressions
func TestCoalesceFunctionInDeterministicExpression(t *testing.T) {
	tests := []struct {
		name       string
		expression string
		want       bool
		wantErr    bool
	}{
		// Basic coalesce usage
		{
			name:       "coalesce returns first non-null - equality check",
			expression: "coalesce(null, 'value') == 'value'",
			want:       true,
		},
		{
			name:       "coalesce with first value non-null",
			expression: "coalesce('first', 'second') == 'first'",
			want:       true,
		},
		{
			name:       "coalesce with multiple nulls",
			expression: "coalesce(null, null, 'third') == 'third'",
			want:       true,
		},
		{
			name:       "coalesce all nulls returns null",
			expression: "coalesce(null, null) == null",
			want:       true,
		},
		{
			name:       "coalesce with numbers",
			expression: "coalesce(null, 42) == 42",
			want:       true,
		},
		{
			name:       "coalesce with zero (zero is valid)",
			expression: "coalesce(null, 0) == 0",
			want:       true,
		},

		// Coalesce in comparison expressions
		{
			name:       "coalesce result greater than comparison",
			expression: "coalesce(null, 100) > 50",
			want:       true,
		},
		{
			name:       "coalesce result less than comparison",
			expression: "coalesce(null, 10) < 50",
			want:       true,
		},

		// Coalesce with logical operators
		{
			name:       "coalesce in AND expression",
			expression: "coalesce(null, 'yes') == 'yes' && true == true",
			want:       true,
		},
		{
			name:       "coalesce in OR expression - first true",
			expression: "coalesce('value', null) == 'value' || false == true",
			want:       true,
		},

		// Empty string treated as null
		{
			name:       "coalesce empty string treated as null",
			expression: "coalesce('', 'fallback') == 'fallback'",
			want:       true,
		},
		{
			name:       "coalesce quoted empty string treated as null",
			expression: `coalesce("", 'fallback') == 'fallback'`,
			want:       true,
		},

		// Nested coalesce
		{
			name:       "nested coalesce - inner null",
			expression: "coalesce(coalesce(null, null), 'outer') == 'outer'",
			want:       true,
		},
		{
			name:       "nested coalesce - inner has value",
			expression: "coalesce(coalesce(null, 'inner'), 'outer') == 'inner'",
			want:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := EvaluateDeterministicExpression(tt.expression)
			if (err != nil) != tt.wantErr {
				t.Errorf("EvaluateDeterministicExpression() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("EvaluateDeterministicExpression() = %v, want %v for expression: %s", got, tt.want, tt.expression)
			}
		})
	}
}

// TestValidateExpressionFunctions tests the ValidateExpressionFunctions helper
func TestValidateExpressionFunctions(t *testing.T) {
	tests := []struct {
		name        string
		value       string
		wantErr     bool
		errContains string
	}{
		// Valid cases
		{
			name:    "valid - coalesce function",
			value:   "coalesce($a, $b)",
			wantErr: false,
		},
		{
			name:    "valid - no function calls",
			value:   "just a plain string",
			wantErr: false,
		},
		{
			name:    "valid - variable reference",
			value:   "$user.name",
			wantErr: false,
		},
		{
			name:    "valid - nested coalesce",
			value:   "coalesce(coalesce($a, $b), $c)",
			wantErr: false,
		},
		{
			name:    "valid - coalesce in JSON",
			value:   `{"id": coalesce($primaryId, $fallbackId)}`,
			wantErr: false,
		},
		{
			name:    "valid - len function",
			value:   "len($items)",
			wantErr: false,
		},
		{
			name:    "valid - isEmpty function",
			value:   "isEmpty($data)",
			wantErr: false,
		},
		{
			name:    "valid - contains function",
			value:   "contains($list, 'value')",
			wantErr: false,
		},
		{
			name:    "valid - exists function",
			value:   "exists($field)",
			wantErr: false,
		},
		{
			name:    "valid - combined functions",
			value:   "len(coalesce($items, [])) and isEmpty($data)",
			wantErr: false,
		},

		// Valid - Portuguese/Spanish gendered word patterns (not function calls)
		{
			name:    "valid - Portuguese gendered word cuidado(a)",
			value:   "Queremos que você se sinta bem cuidado(a) aqui.",
			wantErr: false,
		},
		{
			name:    "valid - Portuguese gendered word querido(a)",
			value:   "Olá, querido(a) paciente!",
			wantErr: false,
		},
		{
			name:    "valid - Portuguese gendered word filho(a)",
			value:   "Traga seu filho(a) para avaliação.",
			wantErr: false,
		},
		{
			name:    "valid - Spanish gendered word amigo(o)",
			value:   "Hola, amigo(o)!",
			wantErr: false,
		},
		{
			name:    "valid - Portuguese plural marker amigos(as)",
			value:   "Traga seus amigos(as).",
			wantErr: false,
		},
		{
			name:    "valid - multiple gendered words in text",
			value:   "Olá! Você está cuidado(a) e querido(a) aqui conosco.",
			wantErr: false,
		},

		// Invalid cases
		{
			name:        "invalid - upper function not supported",
			value:       "upper($name)",
			wantErr:     true,
			errContains: "unsupported function",
		},
		{
			name:        "invalid - lower function not supported",
			value:       "lower($name)",
			wantErr:     true,
			errContains: "unsupported function",
		},
		{
			name:        "invalid - trim function not supported",
			value:       "trim($value)",
			wantErr:     true,
			errContains: "unsupported function",
		},
		{
			name:        "invalid - custom function not supported",
			value:       "myCustomFunc($arg)",
			wantErr:     true,
			errContains: "unsupported function",
		},
		{
			name:        "invalid - multiple unsupported functions",
			value:       "upper(lower($name))",
			wantErr:     true,
			errContains: "unsupported function",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateExpressionFunctions(tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateExpressionFunctions() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errContains != "" {
				if !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("ValidateExpressionFunctions() error = %v, want error containing %q", err, tt.errContains)
				}
			}
		})
	}
}

// TestCoalesceInValidateDeterministicExpression verifies coalesce is now a supported function
func TestCoalesceInValidateDeterministicExpression(t *testing.T) {
	// coalesce should be valid in deterministic expressions
	err := ValidateDeterministicExpression("coalesce($a, $b) == 'value'")
	if err != nil {
		t.Errorf("ValidateDeterministicExpression() should allow coalesce, got error: %v", err)
	}

	// coalesce with len should be valid
	err = ValidateDeterministicExpression("coalesce($a, $b) == 'value' && len($arr) > 0")
	if err != nil {
		t.Errorf("ValidateDeterministicExpression() should allow coalesce with other functions, got error: %v", err)
	}
}
