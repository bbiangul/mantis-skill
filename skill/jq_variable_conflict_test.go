package skill

import (
	"fmt"
	"strings"
	"testing"
)

// TestDetectJqVariableConflicts tests the detection of jq variable name conflicts
// with YAML input parameters. This is critical because the framework replaces ALL
// $variableName patterns in bash scripts BEFORE execution, breaking jq expressions
// that use the same variable names as input parameters.
//
// Example of the bug this prevents:
//   Input parameter: dealbreakerAnalysis
//   jq expression: ... ) as $dealbreakerAnalysis | ...
//   Framework replaces $dealbreakerAnalysis with actual JSON data, breaking jq syntax

func TestDetectJqVariableConflicts_NoConflict(t *testing.T) {
	tests := []struct {
		name         string
		inputParams  []string
		bashScript   string
		wantConflict bool
	}{
		{
			name:        "no jq, no conflict",
			inputParams: []string{"userData", "config"},
			bashScript: `
				echo "Hello World"
				VAR="test"
			`,
			wantConflict: false,
		},
		{
			name:        "jq with different variable names",
			inputParams: []string{"userData", "config"},
			bashScript: `
				echo "$userData" | jq --argjson data "$DATA" '.items[] | select(.id == $data.id)'
			`,
			wantConflict: false,
		},
		{
			name:        "jq as $varName with different names",
			inputParams: []string{"hotels", "reviews"},
			bashScript: `
				jq '(.count) as $cnt | ($cnt * 2) as $doubled | { total: $doubled }'
			`,
			wantConflict: false,
		},
		{
			name:        "jq --argjson with abbreviated names",
			inputParams: []string{"dealbreakerAnalysis", "recentReviews"},
			bashScript: `
				jq --argjson dbResult "$DEALBREAKER" \
				   --argjson recentRevs "$REVIEWS" \
				   '.items | map(select($dbResult.hasDealbreaker))'
			`,
			wantConflict: false,
		},
		{
			name:        "complex jq with many variables, no conflict",
			inputParams: []string{"scoredFlights", "userPrefs", "connectionRisks"},
			bashScript: `
				echo "$scoredFlights" | jq --argjson prefs "$USER_PREFS" \
					--argjson risks "$CONN_RISKS" \
					'
					. as $flights |
					($prefs.loyalty // "") as $loyalty |
					($risks // []) as $riskList |
					$flights.topFlights | map(
						. as $f |
						{
							price: $f.price,
							hasRisk: ($riskList | any(.code == $f.connectionAirport))
						}
					)
					'
			`,
			wantConflict: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conflicts := DetectJqVariableConflicts(tt.inputParams, tt.bashScript)
			if len(conflicts) > 0 && !tt.wantConflict {
				t.Errorf("Expected no conflicts, but got: %v", conflicts)
			}
			if len(conflicts) == 0 && tt.wantConflict {
				t.Errorf("Expected conflicts, but got none")
			}
		})
	}
}

func TestDetectJqVariableConflicts_WithConflict(t *testing.T) {
	tests := []struct {
		name            string
		inputParams     []string
		bashScript      string
		wantConflicts   []string // Expected conflicting variable names
		wantConflictLoc []string // Expected location descriptions
	}{
		{
			name:        "jq --argjson matches input param",
			inputParams: []string{"userData", "config"},
			bashScript: `
				echo "$data" | jq --argjson userData "$USER_DATA" '.items'
			`,
			wantConflicts:   []string{"userData"},
			wantConflictLoc: []string{"--argjson"},
		},
		{
			name:        "jq --arg matches input param",
			inputParams: []string{"searchQuery", "filters"},
			bashScript: `
				jq --arg searchQuery "$QUERY" '.items | map(select(.name | contains($searchQuery)))'
			`,
			wantConflicts:   []string{"searchQuery"},
			wantConflictLoc: []string{"--arg"},
		},
		{
			name:        "jq as $varName matches input param",
			inputParams: []string{"dealbreakerAnalysis", "reviews"},
			bashScript: `
				jq '
					(
						if .hasDealbreaker then
							{ found: true, count: .mentions }
						else
							{ found: false, count: 0 }
						end
					) as $dealbreakerAnalysis |
					if $dealbreakerAnalysis.found then -50 else 0 end
				'
			`,
			wantConflicts:   []string{"dealbreakerAnalysis"},
			wantConflictLoc: []string{"as $"},
		},
		{
			name:        "multiple conflicts in same script",
			inputParams: []string{"bestReviews", "recentReviews", "hotelData"},
			bashScript: `
				jq --argjson bestReviews "$BEST" \
				   --argjson recentReviews "$RECENT" \
				   '
				   ($bestReviews[0]) as $first |
				   ($recentReviews | length) as $count |
				   { first: $first, count: $count }
				   '
			`,
			wantConflicts:   []string{"bestReviews", "recentReviews"},
			wantConflictLoc: []string{"--argjson", "--argjson"},
		},
		{
			name:        "conflict in complex heredoc",
			inputParams: []string{"flightData", "seatAvailability"},
			bashScript: `
				SCORED=$(cat << 'JSONINPUT'
				$flightData
				JSONINPUT
				)

				echo "$SCORED" | jq --argjson seatAvailability "$SEAT_DATA" '
					.flights | map(
						. + { hasSeat: ($seatAvailability | any(.flightId == .id)) }
					)
				'
			`,
			wantConflicts:   []string{"seatAvailability"},
			wantConflictLoc: []string{"--argjson"},
		},
		{
			name:        "conflict with --slurpfile",
			inputParams: []string{"configData"},
			bashScript: `
				jq --slurpfile configData /tmp/config.json '.settings + $configData[0]'
			`,
			wantConflicts:   []string{"configData"},
			wantConflictLoc: []string{"--slurpfile"},
		},
		{
			name:        "conflict with --rawfile",
			inputParams: []string{"templateContent"},
			bashScript: `
				jq --rawfile templateContent /tmp/template.txt '.body = $templateContent'
			`,
			wantConflicts:   []string{"templateContent"},
			wantConflictLoc: []string{"--rawfile"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conflicts := DetectJqVariableConflicts(tt.inputParams, tt.bashScript)

			if len(conflicts) != len(tt.wantConflicts) {
				t.Errorf("Expected %d conflicts, got %d: %v", len(tt.wantConflicts), len(conflicts), conflicts)
				return
			}

			for i, want := range tt.wantConflicts {
				found := false
				for _, conflict := range conflicts {
					if conflict.VarName == want {
						found = true
						// Also check location type if specified
						if i < len(tt.wantConflictLoc) && conflict.Location != tt.wantConflictLoc[i] {
							t.Errorf("Conflict '%s' expected location '%s', got '%s'", want, tt.wantConflictLoc[i], conflict.Location)
						}
						break
					}
				}
				if !found {
					t.Errorf("Expected conflict for '%s', but not found in %v", want, conflicts)
				}
			}
		})
	}
}

func TestDetectJqVariableConflicts_EdgeCases(t *testing.T) {
	tests := []struct {
		name         string
		inputParams  []string
		bashScript   string
		wantConflict bool
		description  string
	}{
		{
			name:         "empty script",
			inputParams:  []string{"data"},
			bashScript:   "",
			wantConflict: false,
			description:  "Empty scripts should not cause errors",
		},
		{
			name:         "empty input params",
			inputParams:  []string{},
			bashScript:   `jq --argjson data "$DATA" '.items'`,
			wantConflict: false,
			description:  "No input params means no possible conflicts",
		},
		{
			name:         "nil input params",
			inputParams:  nil,
			bashScript:   `jq --argjson data "$DATA" '.items'`,
			wantConflict: false,
			description:  "Nil input params should be handled safely",
		},
		{
			name:         "jq in comment should not trigger",
			inputParams:  []string{"userData"},
			bashScript:   `# jq --argjson userData "$DATA" '.items'`,
			wantConflict: false,
			description:  "Comments should be ignored",
		},
		{
			name:         "partial match should not trigger",
			inputParams:  []string{"data"},
			bashScript:   `jq --argjson dataExtended "$DATA" '.items'`,
			wantConflict: false,
			description:  "Only exact matches should trigger, not partial",
		},
		{
			name:         "case sensitive - no match for different case",
			inputParams:  []string{"userData"},
			bashScript:   `jq --argjson userdata "$DATA" '.items'`,
			wantConflict: false,
			description:  "Matching should be case-sensitive",
		},
		{
			name:         "variable in string literal should not trigger",
			inputParams:  []string{"name"},
			bashScript:   `jq '.greeting = "Hello $name"'`,
			wantConflict: false,
			description:  "Variables in jq string literals are not jq variables",
		},
		{
			name:        "bash variable same as input but not jq variable",
			inputParams: []string{"data"},
			bashScript: `
				data="test"
				echo "$data" | jq '.items'
			`,
			wantConflict: false,
			description:  "Bash variables are fine, only jq variables matter",
		},
		{
			name:        "multiple jq commands, conflict in second",
			inputParams: []string{"result"},
			bashScript: `
				echo "$input" | jq '.first'
				echo "$other" | jq --argjson result "$RES" '.second + $result'
			`,
			wantConflict: true,
			description:  "Should detect conflicts in any jq command",
		},
		{
			name:        "jq inside function definition",
			inputParams: []string{"config"},
			bashScript: `
				process_data() {
					jq --argjson config "$1" '.settings + $config'
				}
				process_data "$CONFIG"
			`,
			wantConflict: true,
			description:  "Should detect conflicts inside bash functions",
		},
		{
			name:         "jq in subshell",
			inputParams:  []string{"items"},
			bashScript:   `RESULT=$(echo "$data" | jq --argjson items "$ITEMS" '$items | length')`,
			wantConflict: true,
			description:  "Should detect conflicts in subshells",
		},
		{
			name:         "reduce with as clause conflict",
			inputParams:  []string{"acc"},
			bashScript:   `jq 'reduce .[] as $acc (0; . + $acc)'`,
			wantConflict: true,
			description:  "reduce with 'as $var' pattern should be detected",
		},
		{
			name:         "def with args that match input - should NOT conflict",
			inputParams:  []string{"x"},
			bashScript:   `jq 'def double(x): x * 2; .value | double'`,
			wantConflict: false,
			description:  "jq function definitions use different syntax, no $ prefix",
		},
		{
			name:         "try-catch with as clause",
			inputParams:  []string{"err"},
			bashScript:   `jq 'try .value catch . as $err | "Error: \($err)"'`,
			wantConflict: true,
			description:  "try-catch with 'as $var' should be detected",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conflicts := DetectJqVariableConflicts(tt.inputParams, tt.bashScript)
			hasConflict := len(conflicts) > 0

			if hasConflict != tt.wantConflict {
				if tt.wantConflict {
					t.Errorf("%s: Expected conflict but got none", tt.description)
				} else {
					t.Errorf("%s: Expected no conflict but got: %v", tt.description, conflicts)
				}
			}
		})
	}
}

func TestDetectJqVariableConflicts_RealWorldExamples(t *testing.T) {
	// These are based on actual bugs found in the traveler tool
	tests := []struct {
		name         string
		inputParams  []string
		bashScript   string
		wantConflict bool
		bugID        string
	}{
		{
			name:        "BUG-012: analyzeHotelOptions dealbreakerAnalysis conflict",
			inputParams: []string{"scoredHotels", "dealbreakerAnalysis", "hotelReviews"},
			bashScript: `
				# Parse dealbreaker analysis
				DEALBREAKER=$(cat << 'JSONINPUT'
				$dealbreakerAnalysis
				JSONINPUT
				)

				echo "$scoredHotels" | jq --argjson reviews "$REVIEWS" '
					(
						if $DEALBREAKER.hasDealbreaker then
							{ found: true, warning: $DEALBREAKER.warning }
						else
							{ found: false, warning: null }
						end
					) as $dealbreakerAnalysis |
					.topHotels | map(
						. + {
							hasDealbreaker: $dealbreakerAnalysis.found,
							dealbreakerWarning: $dealbreakerAnalysis.warning
						}
					)
				'
			`,
			wantConflict: true,
			bugID:        "BUG-012",
		},
		{
			name:        "BUG-012 FIXED: using $dbResult instead",
			inputParams: []string{"scoredHotels", "dealbreakerAnalysis", "hotelReviews"},
			bashScript: `
				# Parse dealbreaker analysis
				DEALBREAKER=$(cat << 'JSONINPUT'
				$dealbreakerAnalysis
				JSONINPUT
				)

				echo "$scoredHotels" | jq --argjson reviews "$REVIEWS" '
					(
						if $DEALBREAKER.hasDealbreaker then
							{ found: true, warning: $DEALBREAKER.warning }
						else
							{ found: false, warning: null }
						end
					) as $dbResult |
					.topHotels | map(
						. + {
							hasDealbreaker: $dbResult.found,
							dealbreakerWarning: $dbResult.warning
						}
					)
				'
			`,
			wantConflict: false,
			bugID:        "BUG-012 FIXED",
		},
		{
			name:        "BUG-004: recentReviews conflict in analyzeHotelOptions",
			inputParams: []string{"bestReviews", "recentReviews"},
			bashScript: `
				jq --argjson bestReviews "$VALID_BEST" \
				   --argjson recentReviews "$VALID_RECENT" \
				   '
				   {
				     bestCount: ($bestReviews | length),
				     recentCount: ($recentReviews | length)
				   }
				   '
			`,
			wantConflict: true,
			bugID:        "BUG-004",
		},
		{
			name:        "BUG-004 FIXED: using abbreviated names",
			inputParams: []string{"bestReviews", "recentReviews"},
			bashScript: `
				jq --argjson bestRevs "$VALID_BEST" \
				   --argjson recentRevs "$VALID_RECENT" \
				   '
				   {
				     bestCount: ($bestRevs | length),
				     recentCount: ($recentRevs | length)
				   }
				   '
			`,
			wantConflict: false,
			bugID:        "BUG-004 FIXED",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conflicts := DetectJqVariableConflicts(tt.inputParams, tt.bashScript)
			hasConflict := len(conflicts) > 0

			if hasConflict != tt.wantConflict {
				if tt.wantConflict {
					t.Errorf("%s: Expected conflict for this known bug pattern, but got none", tt.bugID)
				} else {
					t.Errorf("%s: This pattern should be safe (fixed version), but got conflicts: %v", tt.bugID, conflicts)
				}
			}
		})
	}
}

func TestDetectJqVariableConflicts_WarningMessage(t *testing.T) {
	// Test that the warning messages are informative
	conflicts := DetectJqVariableConflicts(
		[]string{"userData", "config"},
		`jq --argjson userData "$DATA" '.items'`,
	)

	if len(conflicts) != 1 {
		t.Fatalf("Expected 1 conflict, got %d", len(conflicts))
	}

	conflict := conflicts[0]

	// Check all required fields are populated
	if conflict.VarName != "userData" {
		t.Errorf("VarName should be 'userData', got '%s'", conflict.VarName)
	}
	if conflict.Location != "--argjson" {
		t.Errorf("Location should be '--argjson', got '%s'", conflict.Location)
	}
	if conflict.Line == 0 {
		// Line number should be set if possible
		t.Log("Line number not set (acceptable for now)")
	}
}

func TestDetectJqVariableConflicts_JqPatternVariations(t *testing.T) {
	// Test various jq invocation patterns and syntax variations
	tests := []struct {
		name         string
		inputParams  []string
		bashScript   string
		wantConflict bool
		description  string
	}{
		{
			name:         "jq with single quotes",
			inputParams:  []string{"data"},
			bashScript:   `echo "$input" | jq --argjson data "$DATA" '.items'`,
			wantConflict: true,
			description:  "Standard jq invocation with single quotes",
		},
		{
			name:         "jq with double quotes",
			inputParams:  []string{"data"},
			bashScript:   `echo "$input" | jq --argjson data "$DATA" ".items"`,
			wantConflict: true,
			description:  "jq with double-quoted filter",
		},
		{
			name:        "jq with heredoc filter",
			inputParams: []string{"config"},
			bashScript: `
				echo "$input" | jq --argjson config "$CFG" '
					.settings | map(
						. + $config
					)
				'
			`,
			wantConflict: true,
			description:  "jq with multi-line heredoc-style filter",
		},
		{
			name:         "jq -r flag before args",
			inputParams:  []string{"filter"},
			bashScript:   `jq -r --arg filter "$FILTER" '.[] | select(. | contains($filter))'`,
			wantConflict: true,
			description:  "jq with -r flag before --arg",
		},
		{
			name:         "jq -c -r flags combined",
			inputParams:  []string{"value"},
			bashScript:   `jq -cr --argjson value "$VAL" '.data + $value'`,
			wantConflict: true,
			description:  "jq with combined flags",
		},
		{
			name:         "jq with -n null input",
			inputParams:  []string{"items"},
			bashScript:   `jq -n --argjson items "$ITEMS" '$items | length'`,
			wantConflict: true,
			description:  "jq with null input flag",
		},
		{
			name:         "jq with --tab flag",
			inputParams:  []string{"result"},
			bashScript:   `jq --tab --argjson result "$RES" '.data = $result'`,
			wantConflict: true,
			description:  "jq with formatting flag before args",
		},
		{
			name:         "multiple --argjson in sequence",
			inputParams:  []string{"first", "second", "third"},
			bashScript:   `jq --argjson first "$A" --argjson second "$B" --argjson third "$C" '.x'`,
			wantConflict: true,
			description:  "All three args conflict",
		},
		{
			name:         "mixed --arg and --argjson",
			inputParams:  []string{"name", "data"},
			bashScript:   `jq --arg name "$NAME" --argjson data "$DATA" '.user = $name | .info = $data'`,
			wantConflict: true,
			description:  "Both string and json args conflict",
		},
		{
			name:         "jq in pipeline chain",
			inputParams:  []string{"filter"},
			bashScript:   `cat file.json | jq --arg filter "$F" 'select(.)' | jq '.items'`,
			wantConflict: true,
			description:  "jq in middle of pipeline",
		},
		{
			name:         "jq called via xargs",
			inputParams:  []string{"template"},
			bashScript:   `echo "$files" | xargs -I{} jq --argjson template "$TPL" '. + $template' {}`,
			wantConflict: true,
			description:  "jq invoked through xargs",
		},
		{
			name:        "jq in while loop",
			inputParams: []string{"config"},
			bashScript: `
				while read line; do
					echo "$line" | jq --argjson config "$CFG" '. + $config'
				done < input.txt
			`,
			wantConflict: true,
			description:  "jq inside while loop",
		},
		{
			name:        "jq in if statement",
			inputParams: []string{"threshold"},
			bashScript: `
				if [ "$condition" = "true" ]; then
					jq --argjson threshold "$THR" 'select(.value > $threshold)'
				fi
			`,
			wantConflict: true,
			description:  "jq inside conditional",
		},
		{
			name:         "jq with file input",
			inputParams:  []string{"settings"},
			bashScript:   `jq --argjson settings "$SETTINGS" '. + $settings' input.json > output.json`,
			wantConflict: true,
			description:  "jq with file I/O redirection",
		},
		{
			name:         "jq with slurp mode",
			inputParams:  []string{"base"},
			bashScript:   `jq -s --argjson base "$BASE" '.[0] + $base' file1.json file2.json`,
			wantConflict: true,
			description:  "jq in slurp mode with multiple files",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conflicts := DetectJqVariableConflicts(tt.inputParams, tt.bashScript)
			hasConflict := len(conflicts) > 0

			if hasConflict != tt.wantConflict {
				if tt.wantConflict {
					t.Errorf("%s: Expected conflict but got none", tt.description)
				} else {
					t.Errorf("%s: Expected no conflict but got: %v", tt.description, conflicts)
				}
			}
		})
	}
}

func TestDetectJqVariableConflicts_AsPatternVariations(t *testing.T) {
	// Test various 'as $var' pattern variations in jq
	tests := []struct {
		name         string
		inputParams  []string
		bashScript   string
		wantConflict bool
		description  string
	}{
		{
			name:         "simple as binding",
			inputParams:  []string{"item"},
			bashScript:   `jq '.data as $item | $item.name'`,
			wantConflict: true,
			description:  "Basic as binding",
		},
		{
			name:         "as binding after pipe",
			inputParams:  []string{"value"},
			bashScript:   `jq '.items[] | . as $value | {v: $value}'`,
			wantConflict: true,
			description:  "as binding in pipeline",
		},
		{
			name:         "as binding with parentheses",
			inputParams:  []string{"result"},
			bashScript:   `jq '(.x + .y) as $result | $result * 2'`,
			wantConflict: true,
			description:  "as binding with expression in parens",
		},
		{
			name:         "nested as bindings",
			inputParams:  []string{"outer"},
			bashScript:   `jq '.a as $outer | .b as $inner | [$outer, $inner]'`,
			wantConflict: true,
			description:  "Multiple as bindings, first conflicts",
		},
		{
			name:         "nested as bindings - second conflicts",
			inputParams:  []string{"inner"},
			bashScript:   `jq '.a as $outer | .b as $inner | [$outer, $inner]'`,
			wantConflict: true,
			description:  "Multiple as bindings, second conflicts",
		},
		{
			name:         "reduce pattern",
			inputParams:  []string{"acc"},
			bashScript:   `jq 'reduce .[] as $x (0; . + $x) as $acc | $acc'`,
			wantConflict: true,
			description:  "reduce with as binding for accumulator",
		},
		{
			name:         "reduce with item variable conflict",
			inputParams:  []string{"item"},
			bashScript:   `jq 'reduce .[] as $item (0; . + $item)'`,
			wantConflict: true,
			description:  "reduce iteration variable conflicts",
		},
		{
			name:         "foreach pattern",
			inputParams:  []string{"x"},
			bashScript:   `jq '[foreach .[] as $x (0; . + 1)]'`,
			wantConflict: true,
			description:  "foreach with conflicting variable",
		},
		{
			name:         "try-catch with as",
			inputParams:  []string{"e"},
			bashScript:   `jq 'try .value catch . as $e | "Error: \($e)"'`,
			wantConflict: true,
			description:  "try-catch error binding",
		},
		{
			name:         "optional object with as",
			inputParams:  []string{"obj"},
			bashScript:   `jq '(.config // {}) as $obj | $obj.setting'`,
			wantConflict: true,
			description:  "Optional/alternative with as binding",
		},
		{
			name:         "if-then-else with as",
			inputParams:  []string{"val"},
			bashScript:   `jq 'if .x > 0 then .x else 0 end as $val | $val * 2'`,
			wantConflict: true,
			description:  "Conditional result bound with as",
		},
		{
			name:         "select with as",
			inputParams:  []string{"selected"},
			bashScript:   `jq '[.[] | select(.active)] as $selected | $selected | length'`,
			wantConflict: true,
			description:  "select result bound with as",
		},
		{
			name:         "map with as",
			inputParams:  []string{"mapped"},
			bashScript:   `jq '(.items | map(.name)) as $mapped | {names: $mapped}'`,
			wantConflict: true,
			description:  "map result bound with as",
		},
		{
			name:         "group_by with as",
			inputParams:  []string{"groups"},
			bashScript:   `jq 'group_by(.type) as $groups | $groups | length'`,
			wantConflict: true,
			description:  "group_by result bound with as",
		},
		{
			name:         "input/inputs with as",
			inputParams:  []string{"all"},
			bashScript:   `jq -n '[inputs] as $all | $all | add'`,
			wantConflict: true,
			description:  "inputs collection bound with as",
		},
		{
			name:         "def parameter should not conflict",
			inputParams:  []string{"x", "y"},
			bashScript:   `jq 'def add(x; y): x + y; add(.a; .b)'`,
			wantConflict: false,
			description:  "def parameters use different syntax",
		},
		{
			name:         "def with body variable should not conflict",
			inputParams:  []string{"result"},
			bashScript:   `jq 'def double: . * 2; .value | double'`,
			wantConflict: false,
			description:  "def body doesn't use $var syntax",
		},
		{
			name:         "$ENV access should not conflict",
			inputParams:  []string{"HOME", "PATH"},
			bashScript:   `jq -n 'env.HOME'`,
			wantConflict: false,
			description:  "env.VAR syntax is different from $VAR",
		},
		{
			name:         "$__loc__ should not conflict",
			inputParams:  []string{"__loc__"},
			bashScript:   `jq '$__loc__'`,
			wantConflict: false,
			description:  "Built-in special variables shouldn't be flagged",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conflicts := DetectJqVariableConflicts(tt.inputParams, tt.bashScript)
			hasConflict := len(conflicts) > 0

			if hasConflict != tt.wantConflict {
				if tt.wantConflict {
					t.Errorf("%s: Expected conflict but got none", tt.description)
				} else {
					t.Errorf("%s: Expected no conflict but got: %v", tt.description, conflicts)
				}
			}
		})
	}
}

func TestDetectJqVariableConflicts_FalsePositivePrevention(t *testing.T) {
	// Tests specifically designed to prevent false positives
	tests := []struct {
		name         string
		inputParams  []string
		bashScript   string
		wantConflict bool
		description  string
	}{
		{
			name:         "bash variable assignment",
			inputParams:  []string{"data", "result"},
			bashScript:   `data="test"; result=$(echo "$data")`,
			wantConflict: false,
			description:  "Bash variable assignments are not jq variables",
		},
		{
			name:         "bash for loop variable",
			inputParams:  []string{"item"},
			bashScript:   `for item in a b c; do echo "$item"; done`,
			wantConflict: false,
			description:  "Bash for loop variables are not jq",
		},
		{
			name:         "bash read variable",
			inputParams:  []string{"line"},
			bashScript:   `while read line; do echo "$line"; done`,
			wantConflict: false,
			description:  "Bash read variables are not jq",
		},
		{
			name:         "bash array variable",
			inputParams:  []string{"arr"},
			bashScript:   `arr=(1 2 3); echo "${arr[@]}"`,
			wantConflict: false,
			description:  "Bash arrays are not jq variables",
		},
		{
			name:         "bash function parameter",
			inputParams:  []string{"param"},
			bashScript:   `myfunc() { local param="$1"; echo "$param"; }`,
			wantConflict: false,
			description:  "Bash function params are not jq",
		},
		{
			name:         "jq string interpolation",
			inputParams:  []string{"name"},
			bashScript:   `jq '.greeting = "Hello \(.name)"'`,
			wantConflict: false,
			description:  "String interpolation uses \\(.x) not $x",
		},
		{
			name:         "jq object key",
			inputParams:  []string{"key"},
			bashScript:   `jq '{key: .value}'`,
			wantConflict: false,
			description:  "Object keys are not variables",
		},
		{
			name:         "jq field access",
			inputParams:  []string{"data"},
			bashScript:   `jq '.data.items'`,
			wantConflict: false,
			description:  "Field access .data is not $data",
		},
		{
			name:         "jq recursive descent",
			inputParams:  []string{"name"},
			bashScript:   `jq '.. | .name? // empty'`,
			wantConflict: false,
			description:  "Recursive field access is not variable",
		},
		{
			name:         "jq builtin function name",
			inputParams:  []string{"length", "keys", "values"},
			bashScript:   `jq '.items | length'`,
			wantConflict: false,
			description:  "Builtin functions are not $variables",
		},
		{
			name:         "jq type check",
			inputParams:  []string{"type", "string", "number"},
			bashScript:   `jq 'type == "string"'`,
			wantConflict: false,
			description:  "Type names are not variables",
		},
		{
			name:         "jq format string",
			inputParams:  []string{"html", "json", "text"},
			bashScript:   `jq '@html'`,
			wantConflict: false,
			description:  "Format strings @x are not variables",
		},
		{
			name:         "jq path expression",
			inputParams:  []string{"path"},
			bashScript:   `jq 'path(.items[0])'`,
			wantConflict: false,
			description:  "path() function is not $path variable",
		},
		{
			name:         "SQL-like variable in bash",
			inputParams:  []string{"id"},
			bashScript:   `sqlite3 db.sqlite "SELECT * FROM users WHERE id = $id"`,
			wantConflict: false,
			description:  "SQL variables are not jq",
		},
		{
			name:         "environment variable in bash",
			inputParams:  []string{"HOME", "PATH", "USER"},
			bashScript:   `echo "$HOME" && ls "$PATH"`,
			wantConflict: false,
			description:  "Env vars are not jq variables",
		},
		{
			name:         "awk variable",
			inputParams:  []string{"NR", "NF", "FS"},
			bashScript:   `awk '{print NR, $1}' file.txt`,
			wantConflict: false,
			description:  "Awk variables are not jq",
		},
		{
			name:         "sed variable",
			inputParams:  []string{"pattern"},
			bashScript:   `sed 's/old/new/g' file.txt`,
			wantConflict: false,
			description:  "Sed doesn't use jq-style vars",
		},
		{
			name:         "grep with pattern",
			inputParams:  []string{"match"},
			bashScript:   `grep -E "pattern" file.txt`,
			wantConflict: false,
			description:  "Grep patterns are not jq vars",
		},
		{
			name:         "curl with data",
			inputParams:  []string{"data"},
			bashScript:   `curl -d '{"key":"value"}' http://api.example.com`,
			wantConflict: false,
			description:  "Curl data is not jq",
		},
		{
			name:         "python inline",
			inputParams:  []string{"x", "y"},
			bashScript:   `python3 -c "x = 1; y = 2; print(x + y)"`,
			wantConflict: false,
			description:  "Python variables are not jq",
		},
		{
			name:         "node inline",
			inputParams:  []string{"result"},
			bashScript:   `node -e "const result = 42; console.log(result)"`,
			wantConflict: false,
			description:  "Node variables are not jq",
		},
		{
			name:         "similar but not matching name",
			inputParams:  []string{"data"},
			bashScript:   `jq --argjson dataExtended "$D" '$dataExtended'`,
			wantConflict: false,
			description:  "dataExtended != data (partial match)",
		},
		{
			name:         "prefix match should not trigger",
			inputParams:  []string{"user"},
			bashScript:   `jq --argjson userData "$U" '$userData'`,
			wantConflict: false,
			description:  "userData != user (prefix)",
		},
		{
			name:         "suffix match should not trigger",
			inputParams:  []string{"Data"},
			bashScript:   `jq --argjson userData "$U" '$userData'`,
			wantConflict: false,
			description:  "userData != Data (suffix)",
		},
		{
			name:         "underscore vs camelCase",
			inputParams:  []string{"user_data"},
			bashScript:   `jq --argjson userData "$U" '$userData'`,
			wantConflict: false,
			description:  "user_data != userData (different naming)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conflicts := DetectJqVariableConflicts(tt.inputParams, tt.bashScript)
			hasConflict := len(conflicts) > 0

			if hasConflict != tt.wantConflict {
				if tt.wantConflict {
					t.Errorf("%s: Expected conflict but got none", tt.description)
				} else {
					t.Errorf("%s: Expected no conflict but got: %v", tt.description, conflicts)
				}
			}
		})
	}
}

func TestDetectJqVariableConflicts_ComplexScripts(t *testing.T) {
	// Test with complex, realistic scripts
	tests := []struct {
		name          string
		inputParams   []string
		bashScript    string
		wantConflicts int
		conflictVars  []string
		description   string
	}{
		{
			name:        "hotel analysis script with multiple jq calls",
			inputParams: []string{"scoredHotels", "dealbreakerAnalysis", "hotelReviews", "userPrefs"},
			bashScript: `
				#!/bin/bash
				set -e

				# Parse inputs
				SCORED=$(cat << 'JSONINPUT'
				$scoredHotels
				JSONINPUT
				)

				DEALBREAKER=$(cat << 'JSONINPUT'
				$dealbreakerAnalysis
				JSONINPUT
				)

				# First jq: Extract top hotels
				TOP_HOTELS=$(echo "$SCORED" | jq '.topHotels[:5]')

				# Second jq: Analyze with dealbreaker - THIS HAS THE BUG
				ANALYZED=$(echo "$TOP_HOTELS" | jq --argjson reviews "$REVIEWS" '
					(
						if $DEALBREAKER.hasDealbreaker then
							{ found: true, warning: $DEALBREAKER.warning }
						else
							{ found: false, warning: null }
						end
					) as $dealbreakerAnalysis |
					map(
						. + {
							hasDealbreaker: $dealbreakerAnalysis.found,
							warning: $dealbreakerAnalysis.warning
						}
					)
				')

				echo "$ANALYZED"
			`,
			wantConflicts: 1,
			conflictVars:  []string{"dealbreakerAnalysis"},
			description:   "Real hotel analysis script with known bug",
		},
		{
			name:        "flight scoring script - fixed version",
			inputParams: []string{"rawFlights", "userPrefs", "connectionRisks"},
			bashScript: `
				#!/bin/bash

				FLIGHTS=$(cat << 'JSONINPUT'
				$rawFlights
				JSONINPUT
				)

				PREFS=$(cat << 'JSONINPUT'
				$userPrefs
				JSONINPUT
				)

				RISKS=$(cat << 'JSONINPUT'
				$connectionRisks
				JSONINPUT
				)

				# Using abbreviated names to avoid conflicts
				echo "$FLIGHTS" | jq --argjson prefs "$PREFS" \
					--argjson risks "$RISKS" \
					'
					($prefs.loyalty // "") as $loyalty |
					($risks // []) as $riskList |
					.flights | map(
						. as $f |
						($riskList | map(select(.code == $f.connectionAirport)) | .[0]) as $risk |
						{
							id: $f.id,
							price: $f.price,
							hasRisk: ($risk != null),
							riskWarning: ($risk.warning // null)
						}
					)
					'
			`,
			wantConflicts: 0,
			conflictVars:  nil,
			description:   "Properly written script with no conflicts",
		},
		{
			name:        "multi-step processing with conflicts",
			inputParams: []string{"inputData", "config", "template", "output"},
			bashScript: `
				# Step 1: Validate
				VALID=$(echo "$inputData" | jq --argjson config "$CONFIG" '
					. as $inputData |
					if $config.validate then
						select(.valid)
					else
						.
					end
				')

				# Step 2: Transform
				TRANSFORMED=$(echo "$VALID" | jq --argjson template "$TEMPLATE" '
					. as $data |
					$template | . + {data: $data}
				')

				# Step 3: Output
				echo "$TRANSFORMED" | jq --argjson output "$OUT" '
					. + {output: $output}
				'
			`,
			wantConflicts: 4, // config (--argjson), template (--argjson), output (--argjson), inputData (as $)
			conflictVars:  []string{"inputData", "config", "template", "output"},
			description:   "Multiple conflicts across multiple jq calls",
		},
		{
			name:        "nested function calls in bash",
			inputParams: []string{"items", "filter"},
			bashScript: `
				process_item() {
					local item="$1"
					echo "$item" | jq --argjson filter "$FILTER" '
						. as $items |
						select($filter.active) |
						{ processed: true, data: $items }
					'
				}

				for item in $(echo "$items" | jq -r '.[]'); do
					process_item "$item"
				done
			`,
			wantConflicts: 2,
			conflictVars:  []string{"items", "filter"},
			description:   "Conflicts inside bash function definitions",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conflicts := DetectJqVariableConflicts(tt.inputParams, tt.bashScript)

			if len(conflicts) != tt.wantConflicts {
				t.Errorf("%s: Expected %d conflicts, got %d: %v",
					tt.description, tt.wantConflicts, len(conflicts), conflicts)
				return
			}

			// Verify specific conflict variables if specified
			if tt.conflictVars != nil {
				for _, want := range tt.conflictVars {
					found := false
					for _, c := range conflicts {
						if c.VarName == want {
							found = true
							break
						}
					}
					if !found {
						t.Errorf("%s: Expected conflict for '%s', not found in %v",
							tt.description, want, conflicts)
					}
				}
			}
		})
	}
}

func TestDetectJqVariableConflicts_CommentHandling(t *testing.T) {
	// Test comment handling edge cases
	tests := []struct {
		name         string
		inputParams  []string
		bashScript   string
		wantConflict bool
		description  string
	}{
		{
			name:        "full line comment",
			inputParams: []string{"data"},
			bashScript: `
				# jq --argjson data "$DATA" '.items'
				echo "no jq here"
			`,
			wantConflict: false,
			description:  "Full line comment should be ignored",
		},
		{
			name:         "inline comment after code",
			inputParams:  []string{"value"},
			bashScript:   `echo "test" # jq --argjson value "$VAL" '.x'`,
			wantConflict: false,
			description:  "Inline comment should be ignored",
		},
		{
			name:         "hash in string should not be treated as comment",
			inputParams:  []string{"color"},
			bashScript:   `jq --arg color "#FF0000" '.style.color = $color'`,
			wantConflict: true,
			description:  "Hash in string is not a comment - conflict exists",
		},
		{
			name:        "commented and uncommented",
			inputParams: []string{"data"},
			bashScript: `
				# Old code: jq --argjson data "$D" '.x'
				jq --argjson data "$DATA" '.items'
			`,
			wantConflict: true,
			description:  "Should detect conflict in uncommented line",
		},
		{
			name:        "multi-line with mixed comments",
			inputParams: []string{"first", "second"},
			bashScript: `
				# This is commented:
				# jq --argjson first "$F" '.x'

				# But this is not:
				jq --argjson second "$S" '.y'
			`,
			wantConflict: true,
			description:  "Only 'second' should be detected as conflict",
		},
		{
			name:        "heredoc with hash",
			inputParams: []string{"data"},
			bashScript: `
				cat << 'EOF'
				# This is inside heredoc, not a bash comment
				jq --argjson data "$D" '.x'
				EOF
			`,
			wantConflict: true,
			description:  "Content inside heredoc delimiter is still scanned",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conflicts := DetectJqVariableConflicts(tt.inputParams, tt.bashScript)
			hasConflict := len(conflicts) > 0

			if hasConflict != tt.wantConflict {
				if tt.wantConflict {
					t.Errorf("%s: Expected conflict but got none", tt.description)
				} else {
					t.Errorf("%s: Expected no conflict but got: %v", tt.description, conflicts)
				}
			}
		})
	}
}

func TestDetectJqVariableConflicts_SpecialCharacters(t *testing.T) {
	// Test handling of special characters in variable names and scripts
	tests := []struct {
		name         string
		inputParams  []string
		bashScript   string
		wantConflict bool
		description  string
	}{
		{
			name:         "underscore in variable name",
			inputParams:  []string{"user_data"},
			bashScript:   `jq --argjson user_data "$UD" '.x'`,
			wantConflict: true,
			description:  "Underscores in variable names",
		},
		{
			name:         "numbers in variable name",
			inputParams:  []string{"data2"},
			bashScript:   `jq --argjson data2 "$D2" '.x'`,
			wantConflict: true,
			description:  "Numbers in variable names",
		},
		{
			name:         "mixed underscore and numbers",
			inputParams:  []string{"user_data_v2"},
			bashScript:   `jq --argjson user_data_v2 "$UDV2" '.x'`,
			wantConflict: true,
			description:  "Complex variable name with underscore and numbers",
		},
		{
			name:         "single character variable",
			inputParams:  []string{"x"},
			bashScript:   `jq --argjson x "$X" '.x'`,
			wantConflict: true,
			description:  "Single character variable name",
		},
		{
			name:         "very long variable name",
			inputParams:  []string{"thisIsAVeryLongVariableNameThatShouldStillWork"},
			bashScript:   `jq --argjson thisIsAVeryLongVariableNameThatShouldStillWork "$V" '.x'`,
			wantConflict: true,
			description:  "Long variable name",
		},
		{
			name:        "newlines in jq expression",
			inputParams: []string{"data"},
			bashScript: `jq '
				.items
				| map(.)
			' --argjson data "$D" <<< "$input"`,
			wantConflict: true,
			description:  "Newlines in jq expression shouldn't break detection",
		},
		{
			name:         "tabs in script",
			inputParams:  []string{"config"},
			bashScript:   "jq\t--argjson\tconfig\t\"$CFG\"\t'.x'",
			wantConflict: true,
			description:  "Tabs as separators",
		},
		{
			name:         "multiple spaces",
			inputParams:  []string{"value"},
			bashScript:   `jq   --argjson   value   "$V"   '.x'`,
			wantConflict: true,
			description:  "Multiple spaces as separators",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conflicts := DetectJqVariableConflicts(tt.inputParams, tt.bashScript)
			hasConflict := len(conflicts) > 0

			if hasConflict != tt.wantConflict {
				if tt.wantConflict {
					t.Errorf("%s: Expected conflict but got none", tt.description)
				} else {
					t.Errorf("%s: Expected no conflict but got: %v", tt.description, conflicts)
				}
			}
		})
	}
}

func TestDetectJqVariableConflicts_RegexEdgeCases(t *testing.T) {
	// Test edge cases that could cause regex false positives
	tests := []struct {
		name         string
		inputParams  []string
		bashScript   string
		wantConflict bool
		description  string
	}{
		{
			name:         "has() function should not trigger as pattern",
			inputParams:  []string{"key"},
			bashScript:   `jq 'has("key")'`,
			wantConflict: false,
			description:  "jq has() is different from 'as'",
		},
		{
			name:         "alias should not trigger as pattern",
			inputParams:  []string{"x"},
			bashScript:   `alias x='echo test'`,
			wantConflict: false,
			description:  "bash alias is not jq as",
		},
		{
			name:         "class keyword should not trigger as pattern",
			inputParams:  []string{"x"},
			bashScript:   `node -e "class Foo {}"`,
			wantConflict: false,
			description:  "JavaScript class is not jq as",
		},
		{
			name:         "--arg followed by equals (invalid jq syntax)",
			inputParams:  []string{"key"},
			bashScript:   `jq --arg key="$VALUE" '.x'`,
			wantConflict: true, // Conservative: detects pattern even in invalid syntax
			description:  "Malformed --arg still triggers (safe false positive)",
		},
		{
			name:         "argjson without double dash",
			inputParams:  []string{"data"},
			bashScript:   `echo "argjson data something"`,
			wantConflict: false,
			description:  "Plain text 'argjson' is not jq flag",
		},
		{
			name:         "--argjson in echo string (edge case)",
			inputParams:  []string{"value"},
			bashScript:   `echo "--argjson value test"`,
			wantConflict: true, // Conservative: pattern in string still triggers
			description:  "Pattern in echo still triggers (safe false positive)",
		},
		{
			name:         "jq with escaped quotes",
			inputParams:  []string{"data"},
			bashScript:   `jq --argjson data "$D" '.x = "test\"value"'`,
			wantConflict: true,
			description:  "Escaped quotes shouldn't break detection",
		},
		{
			name:         "as word in jq string",
			inputParams:  []string{"alias"},
			bashScript:   `jq '.message = "save as draft"'`,
			wantConflict: false,
			description:  "'as' in string literal is not binding",
		},
		{
			name:         "as in variable name but not binding",
			inputParams:  []string{"hasData"},
			bashScript:   `jq '.hasData // false'`,
			wantConflict: false,
			description:  "'.hasData' field access is not '$hasData' binding",
		},
		{
			name:         "gsub with as-like pattern",
			inputParams:  []string{"old"},
			bashScript:   `jq 'gsub("as old"; "as new")'`,
			wantConflict: false,
			description:  "'as' in gsub pattern is string, not binding",
		},
		{
			name:         "test/match with as",
			inputParams:  []string{"pattern"},
			bashScript:   `jq 'test("as pattern")'`,
			wantConflict: false,
			description:  "test() regex contains 'as' but it's a string",
		},
		{
			name:         "split with as",
			inputParams:  []string{"delimiter"},
			bashScript:   `jq 'split("as delimiter")'`,
			wantConflict: false,
			description:  "split() argument is string",
		},
		{
			name:         "valid as with $",
			inputParams:  []string{"result"},
			bashScript:   `jq '. as $result | $result + 1'`,
			wantConflict: true,
			description:  "Valid 'as $result' should be detected",
		},
		{
			name:         "as $var with no space after as",
			inputParams:  []string{"x"},
			bashScript:   `jq '.as$x'`, // This is actually field access .as then $x variable
			wantConflict: false,
			description:  "Field access .as followed by $x is not 'as $x' binding",
		},
		{
			name:         "assignment in bash vs jq as",
			inputParams:  []string{"value"},
			bashScript:   `value=test; echo "$value"`,
			wantConflict: false,
			description:  "Bash assignment is not jq as",
		},
		{
			name:         "jq alternative operator",
			inputParams:  []string{"default"},
			bashScript:   `jq '.value // "default"'`,
			wantConflict: false,
			description:  "Alternative operator // is not as binding",
		},
		{
			name:         "jq try-catch without as",
			inputParams:  []string{"error"},
			bashScript:   `jq 'try .value catch "error"'`,
			wantConflict: false,
			description:  "try-catch without 'as' doesn't bind",
		},
		{
			name:         "destructuring assignment in other languages",
			inputParams:  []string{"x", "y"},
			bashScript:   `python3 -c "x, y = 1, 2"`,
			wantConflict: false,
			description:  "Python unpacking is not jq",
		},
		{
			name:         "TypeScript as cast",
			inputParams:  []string{"Type"},
			bashScript:   `node -e "(value as Type)"`,
			wantConflict: false,
			description:  "TypeScript 'as' cast is not jq binding",
		},
		{
			name:         "SQL AS alias",
			inputParams:  []string{"alias"},
			bashScript:   `sqlite3 db "SELECT name AS alias FROM users"`,
			wantConflict: false,
			description:  "SQL AS is column alias, not jq",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conflicts := DetectJqVariableConflicts(tt.inputParams, tt.bashScript)
			hasConflict := len(conflicts) > 0

			if hasConflict != tt.wantConflict {
				if tt.wantConflict {
					t.Errorf("%s: Expected conflict but got none", tt.description)
				} else {
					t.Errorf("%s: Expected no conflict but got: %v", tt.description, conflicts)
				}
			}
		})
	}
}

func TestDetectJqVariableConflicts_StressTest(t *testing.T) {
	// Test with very large scripts and many input params
	t.Run("large number of input params", func(t *testing.T) {
		inputParams := make([]string, 100)
		for i := 0; i < 100; i++ {
			inputParams[i] = fmt.Sprintf("param%d", i)
		}

		// Only param50 should conflict
		bashScript := `jq --argjson param50 "$DATA" '.x'`
		conflicts := DetectJqVariableConflicts(inputParams, bashScript)

		if len(conflicts) != 1 {
			t.Errorf("Expected 1 conflict, got %d", len(conflicts))
		}
		if len(conflicts) > 0 && conflicts[0].VarName != "param50" {
			t.Errorf("Expected param50 conflict, got %s", conflicts[0].VarName)
		}
	})

	t.Run("very large script", func(t *testing.T) {
		inputParams := []string{"targetVar"}

		// Build a large script with the conflict buried in the middle
		var sb strings.Builder
		for i := 0; i < 100; i++ {
			sb.WriteString(fmt.Sprintf("echo 'line %d'\n", i))
		}
		sb.WriteString(`jq --argjson targetVar "$DATA" '.x'`)
		sb.WriteString("\n")
		for i := 0; i < 100; i++ {
			sb.WriteString(fmt.Sprintf("echo 'after %d'\n", i))
		}

		conflicts := DetectJqVariableConflicts(inputParams, sb.String())
		if len(conflicts) != 1 {
			t.Errorf("Expected 1 conflict in large script, got %d", len(conflicts))
		}
	})

	t.Run("many jq commands", func(t *testing.T) {
		inputParams := []string{"data"}

		var sb strings.Builder
		for i := 0; i < 50; i++ {
			// Every 10th jq command has the conflict
			if i%10 == 0 {
				sb.WriteString(`jq --argjson data "$D" '.x'`)
			} else {
				sb.WriteString(`jq --argjson other "$O" '.y'`)
			}
			sb.WriteString("\n")
		}

		conflicts := DetectJqVariableConflicts(inputParams, sb.String())
		// Should find conflict but deduplicated
		if len(conflicts) != 1 {
			t.Errorf("Expected 1 deduplicated conflict, got %d", len(conflicts))
		}
	})
}

func TestValidateFunctionJqVariableConflicts(t *testing.T) {
	// Test the function-level validation helper
	tests := []struct {
		name         string
		function     Function
		toolName     string
		wantWarnings int
		description  string
	}{
		{
			name: "function with no bash steps",
			function: Function{
				Name: "apiCall",
				Input: []Input{
					{Name: "endpoint"},
					{Name: "data"},
				},
				Steps: []Step{
					{
						Name:   "call",
						Action: "api_call",
						With: map[string]interface{}{
							"url": "https://api.example.com",
						},
					},
				},
			},
			toolName:     "TestTool",
			wantWarnings: 0,
			description:  "No bash steps means no jq conflicts",
		},
		{
			name: "function with safe bash step",
			function: Function{
				Name: "processData",
				Input: []Input{
					{Name: "userData"},
					{Name: "config"},
				},
				Steps: []Step{
					{
						Name:   "process",
						Action: "bash",
						With: map[string]interface{}{
							"linux": `echo "$userData" | jq --argjson cfg "$CFG" '.x'`,
						},
					},
				},
			},
			toolName:     "TestTool",
			wantWarnings: 0,
			description:  "Bash step with no conflicts",
		},
		{
			name: "function with conflicting bash step",
			function: Function{
				Name: "analyzeData",
				Input: []Input{
					{Name: "analysisResult"},
					{Name: "config"},
				},
				Steps: []Step{
					{
						Name:   "analyze",
						Action: "bash",
						With: map[string]interface{}{
							"linux": `jq --argjson analysisResult "$AR" '. + $analysisResult'`,
						},
					},
				},
			},
			toolName:     "TestTool",
			wantWarnings: 1,
			description:  "Bash step with jq conflict",
		},
		{
			name: "function with terminal action (same as bash)",
			function: Function{
				Name: "runTerminal",
				Input: []Input{
					{Name: "command"},
					{Name: "params"},
				},
				Steps: []Step{
					{
						Name:   "run",
						Action: "terminal",
						With: map[string]interface{}{
							"linux": `jq --argjson params "$P" '. + $params'`,
						},
					},
				},
			},
			toolName:     "TestTool",
			wantWarnings: 1,
			description:  "Terminal action also checked for jq conflicts",
		},
		{
			name: "function with multiple steps, one conflict",
			function: Function{
				Name: "multiStep",
				Input: []Input{
					{Name: "data"},
					{Name: "config"},
				},
				Steps: []Step{
					{
						Name:   "step1",
						Action: "bash",
						With: map[string]interface{}{
							"linux": `echo "safe step"`,
						},
					},
					{
						Name:   "step2",
						Action: "bash",
						With: map[string]interface{}{
							"linux": `jq --argjson data "$D" '. + $data'`,
						},
					},
					{
						Name:   "step3",
						Action: "api_call",
						With: map[string]interface{}{
							"url": "https://api.example.com",
						},
					},
				},
			},
			toolName:     "TestTool",
			wantWarnings: 1,
			description:  "Conflict in one of multiple steps",
		},
		{
			name: "function with no inputs",
			function: Function{
				Name:  "noInputs",
				Input: []Input{},
				Steps: []Step{
					{
						Name:   "process",
						Action: "bash",
						With: map[string]interface{}{
							"linux": `jq --argjson data "$D" '. + $data'`,
						},
					},
				},
			},
			toolName:     "TestTool",
			wantWarnings: 0,
			description:  "No inputs means no possible conflicts",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			warnings := ValidateFunctionJqVariableConflicts(tt.function, tt.toolName)

			if len(warnings) != tt.wantWarnings {
				t.Errorf("%s: Expected %d warnings, got %d: %v",
					tt.description, tt.wantWarnings, len(warnings), warnings)
			}
		})
	}
}

// BenchmarkDetectJqVariableConflicts ensures the detection is fast enough
// for use in the parser validation loop
func BenchmarkDetectJqVariableConflicts(b *testing.B) {
	inputParams := []string{"scoredFlights", "userPrefs", "connectionRisks", "seatAvailability", "infantPolicies"}
	bashScript := `
		SCORED=$(cat << 'JSONINPUT'
		$scoredFlights
		JSONINPUT
		)

		echo "$SCORED" | jq --argjson prefs "$USER_PREFS" \
			--argjson risks "$CONN_RISKS" \
			--argjson seats "$SEAT_DATA" \
			'
			. as $flights |
			($prefs.loyalty // "") as $loyalty |
			($risks // []) as $riskList |
			($seats // []) as $seatList |
			$flights.topFlights | map(
				. as $f |
				($seatList | map(select(.flightId == $f.id)) | .[0]) as $seatInfo |
				{
					price: $f.price,
					hasRisk: ($riskList | any(.code == $f.connectionAirport)),
					seatWarning: ($seatInfo.warning // null)
				}
			)
			'
	`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		DetectJqVariableConflicts(inputParams, bashScript)
	}
}
