package skill

import (
	"fmt"
	"strings"
	"time"
)

// I18nString supports both plain string ("Foo") and localized object ({en: "Foo", pt: "Bar", es: "Baz"}).
// When unmarshaled from a plain string, only En is populated.
type I18nString struct {
	En string `yaml:"en,omitempty" json:"en,omitempty"`
	Pt string `yaml:"pt,omitempty" json:"pt,omitempty"`
	Es string `yaml:"es,omitempty" json:"es,omitempty"`
}

// UnmarshalYAML handles both plain string and object formats.
func (i *I18nString) UnmarshalYAML(unmarshal func(interface{}) error) error {
	// Try plain string first
	var str string
	if err := unmarshal(&str); err == nil {
		i.En = str
		return nil
	}

	// Try object format
	type rawI18n I18nString
	var raw rawI18n
	if err := unmarshal(&raw); err != nil {
		return err
	}
	*i = I18nString(raw)
	return nil
}

// Resolve returns the localized string for the given language code, falling back to En.
func (i I18nString) Resolve(lang string) string {
	switch lang {
	case "pt":
		if i.Pt != "" {
			return i.Pt
		}
	case "es":
		if i.Es != "" {
			return i.Es
		}
	}
	return i.En
}

// KPIDefinition defines a KPI metric declaratively in a tool YAML.
// The materializer reads these definitions and computes values from kpi_events.
type KPIDefinition struct {
	ID                string     `yaml:"id"`
	Label             I18nString `yaml:"label"`
	Description       I18nString `yaml:"description,omitempty"`
	Category          string     `yaml:"category"`
	CategoryLabel     I18nString `yaml:"category_label,omitempty"`
	Icon              string     `yaml:"icon,omitempty"`
	EventType         string     `yaml:"event_type,omitempty"`
	Aggregation       string     `yaml:"aggregation"`                  // count | count_unique | sum | avg | ratio
	Numerator         string     `yaml:"numerator,omitempty"`          // ratio only: event_type for numerator
	Denominator       string     `yaml:"denominator,omitempty"`        // ratio only: event_type for denominator
	ValueType         string     `yaml:"value_type"`                   // number | percentage | currency | duration | text
	PositiveDirection string     `yaml:"positive_direction,omitempty"` // up (default) | down
	Comparison        string     `yaml:"comparison,omitempty"`         // previous_period (default) | previous_day | previous_week | previous_month
	Featured          bool       `yaml:"featured,omitempty"`
	Order             int        `yaml:"order,omitempty"`
	Refresh           string     `yaml:"refresh,omitempty"` // Go duration string, default "5m"
}

// GetPositiveDirection returns the positive direction, defaulting to "up".
func (k *KPIDefinition) GetPositiveDirection() string {
	if k.PositiveDirection == "" {
		return "up"
	}
	return k.PositiveDirection
}

// GetComparison returns the comparison mode, defaulting to "previous_period".
func (k *KPIDefinition) GetComparison() string {
	if k.Comparison == "" {
		return "previous_period"
	}
	return k.Comparison
}

// GetRefreshDuration parses the refresh string into a time.Duration, defaulting to 10m.
func (k *KPIDefinition) GetRefreshDuration() time.Duration {
	if k.Refresh == "" {
		return 10 * time.Minute
	}
	d, err := time.ParseDuration(k.Refresh)
	if err != nil {
		return 10 * time.Minute
	}
	return d
}

type CustomTool struct {
	Version   string         `yaml:"version"`
	Env       []EnvVar       `yaml:"env"`
	Tools     []Tool         `yaml:"tools"`
	Workflows []WorkflowYAML `yaml:"workflows,omitempty"`
	Author    string         `yaml:"author"`
}

type EnvVar struct {
	Name        string `yaml:"name"`
	Value       string `yaml:"value"`
	Description string `yaml:"description"`
}

// Migration represents a single incremental database migration.
// Migrations are defined in the function's with.migrations block and are tracked
// per-tool in the tool_schema_migrations table to ensure they run exactly once.
type Migration struct {
	Version int    `yaml:"version"` // Positive integer, must be unique across all functions in a tool
	SQL     string `yaml:"sql"`     // The SQL statement to execute
}

type Tool struct {
	Name        string          `yaml:"name"`
	Description string          `yaml:"description"`
	Version     string          `yaml:"version"`
	Functions   []Function      `yaml:"functions"`
	Kpis        []KPIDefinition `yaml:"kpis,omitempty"`
	// IsSystemApp indicates if this tool is a system application and it should not be parsed from the yaml file.
	IsSystemApp bool `yaml:"-"`
	// IsSharedApp indicates if this tool is a shared application embedded at compile time.
	// Shared tools get their own database (unlike system tools which use connectai.db).
	// Their functions are visible to all other tools via needs/onSuccess/onFailure.
	IsSharedApp bool `yaml:"-"`
}

type Function struct {
	Name                           string                 `yaml:"name"`
	Operation                      string                 `yaml:"operation"`
	Description                    string                 `yaml:"description"`
	SuccessCriteria                string                 `yaml:"successCriteria"`
	Triggers                       []Trigger              `yaml:"triggers"`
	Input                          []Input                `yaml:"input"`
	Needs                          []NeedItem             `yaml:"needs,omitempty"`
	OnSuccess                      []FunctionCall         `yaml:"onSuccess,omitempty"`
	OnFailure                      []FunctionCall         `yaml:"onFailure,omitempty"`
	OnMissingUserInfo              []FunctionCall         `yaml:"onMissingUserInfo,omitempty"`         // Callback when returning "i need some additional information..."
	OnUserConfirmationRequest      []FunctionCall         `yaml:"onUserConfirmationRequest,omitempty"` // Callback when returning "i need the user confirmation..."
	OnTeamApprovalRequest          []FunctionCall         `yaml:"onTeamApprovalRequest,omitempty"`     // Callback when returning "this action requires team approval..."
	OnSkip                         []FunctionCall         `yaml:"onSkip,omitempty"`                    // Callback when function is skipped due to runOnlyIf
	RunOnlyIf                      interface{}            `yaml:"runOnlyIf,omitempty"`                 // Can be string or RunOnlyIfObject
	ReRunIf                        interface{}            `yaml:"reRunIf,omitempty"`                   // Can be string or ReRunIfConfig - re-execute condition after output formatting
	ZeroState                      ZeroState              `yaml:"zero-state"`
	Steps                          []Step                 `yaml:"steps"`
	With                           map[string]interface{} `yaml:"with,omitempty"`
	Output                         *Output                `yaml:"output"`
	MCP                            *MCP                   `yaml:"mcp,omitempty"`
	PDF                            *PDFConfig             `yaml:"pdf,omitempty"`                      // PDF generation configuration
	Cache                          interface{}            `yaml:"cache,omitempty"`                    // Can be int (seconds) or CacheConfig object
	RequiresUserConfirmation       interface{}            `yaml:"requiresUserConfirmation,omitempty"` // Can be bool or RequiresUserConfirmationConfig
	RequiresTeamApproval           bool                   `yaml:"requiresTeamApproval,omitempty"`
	CallRule                       *CallRule              `yaml:"callRule,omitempty"`
	SkipInputPreEvaluation         bool                   `yaml:"skipInputPreEvaluation,omitempty"`                     // Controls input pre-evaluation for onSuccess callbacks
	ShouldBeHandledAsMessageToUser bool                   `yaml:"shouldBeHandledAsMessageToUser,omitempty"`             // When true, output goes to extraInfo
	RequiresInitiateWorkflow       bool                   `yaml:"requiresInitiateWorkflow,omitempty"`                   // When true, function can only run via initiate_workflow
	Sanitize                       interface{}            `yaml:"sanitize,omitempty"`                                   // Prompt injection protection: bool, string ("fence"|"strict"|"llm_extract"), or SanitizeConfig object
	Async                          bool                   `yaml:"async,omitempty" json:"async,omitempty"`               // If true, function executes in background (coordinator continues without waiting)
	ReuseSession                   bool                   `yaml:"reuseSession,omitempty" json:"reuseSession,omitempty"` // If true, all code steps share a single Claude Code session
	ResponseLanguage               string                 `yaml:"responseLanguage,omitempty"`                           // Override user proxy language (e.g. "$responseLanguage" or "Spanish")

	EvaluatedEnvVars map[string]string `yaml:"-"` // Not serialized to YAML

	// Track system variables and functions used
	UsedSysVars  map[string]struct{ base, field string } `yaml:"-"`
	UsedSysFuncs map[string]bool                         `yaml:"-"`

	// Parsed cache config (populated during parsing, not serialized)
	ParsedCache *CacheConfig `yaml:"-"`

	// Parsed reRunIf config (populated during parsing, not serialized)
	ParsedReRunIf *ReRunIfConfig `yaml:"-"`

	// Parsed sanitize config (populated during parsing, not serialized)
	ParsedSanitize *SanitizeConfig `yaml:"-"`
}

type NeedItem struct {
	Name                           string
	Query                          string                 // Query for askToKnowledgeBase (existing)
	Params                         map[string]interface{} // Parameters to pre-fill inputs of the called function
	ShouldBeHandledAsMessageToUser *bool                  // Override callee's shouldBeHandledAsMessageToUser setting
	RequiresUserConfirmation       interface{}            // Override callee's requiresUserConfirmation setting (bool or RequiresUserConfirmationConfig)
}

func (n *NeedItem) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var str string
	if err := unmarshal(&str); err == nil {
		n.Name = str
		n.Query = ""
		n.Params = nil
		n.ShouldBeHandledAsMessageToUser = nil
		n.RequiresUserConfirmation = nil
		return nil
	}

	type rawNeed struct {
		Name                           string                 `yaml:"name"`
		Query                          string                 `yaml:"query,omitempty"`
		Params                         map[string]interface{} `yaml:"params,omitempty"`
		ShouldBeHandledAsMessageToUser *bool                  `yaml:"shouldBeHandledAsMessageToUser,omitempty"`
		RequiresUserConfirmation       interface{}            `yaml:"requiresUserConfirmation,omitempty"`
	}
	var raw rawNeed
	if err := unmarshal(&raw); err != nil {
		return err
	}

	n.Name = raw.Name
	n.Query = raw.Query
	n.Params = raw.Params
	n.ShouldBeHandledAsMessageToUser = raw.ShouldBeHandledAsMessageToUser
	n.RequiresUserConfirmation = raw.RequiresUserConfirmation
	return nil
}

// FunctionCall represents a function call that can be either a simple name string
// or an object with name and optional parameters.
// Supports both:
// - Simple: "functionName"
// - Complex: { name: "functionName", params: { inputName: "$varName" } }
// - With runOnlyIf: { name: "functionName", runOnlyIf: { deterministic: "$var == 'value'" }, params: { ... } }
// - With shouldBeHandledAsMessageToUser: { name: "functionName", shouldBeHandledAsMessageToUser: true }
// - With requiresUserConfirmation: { name: "functionName", requiresUserConfirmation: false } or { name: "functionName", requiresUserConfirmation: { enabled: true, message: "..." } }
type FunctionCall struct {
	Name                           string                 `yaml:"name" json:"name"`
	Params                         map[string]interface{} `yaml:"params,omitempty" json:"params,omitempty"`
	RunOnlyIf                      interface{}            `yaml:"runOnlyIf,omitempty" json:"runOnlyIf,omitempty"`                                           // Can be string or RunOnlyIfObject (deterministic only)
	ShouldBeHandledAsMessageToUser *bool                  `yaml:"shouldBeHandledAsMessageToUser,omitempty" json:"shouldBeHandledAsMessageToUser,omitempty"` // Override callee's shouldBeHandledAsMessageToUser setting
	RequiresUserConfirmation       interface{}            `yaml:"requiresUserConfirmation,omitempty" json:"requiresUserConfirmation,omitempty"`             // Override callee's requiresUserConfirmation setting (bool or object)
	ForEach                        *CallbackForEach       `yaml:"foreach,omitempty" json:"foreach,omitempty"`                                               // ForEach iteration for onSuccess/onFailure callbacks
}

// CallbackForEach defines forEach iteration for onSuccess/onFailure callbacks.
// Unlike step ForEach, callback ForEach does NOT support breakIf, waitFor, or shouldSkip.
// It provides simple iteration over arrays from $result or parent inputs.
type CallbackForEach struct {
	Items     string `yaml:"items"`               // Variable reference to iterate over (e.g., "$result.items" or "$inputName")
	Separator string `yaml:"separator,omitempty"` // Separator for string splitting (defaults to ",")
	IndexVar  string `yaml:"indexVar,omitempty"`  // Variable name for loop index (defaults to "index")
	ItemVar   string `yaml:"itemVar,omitempty"`   // Variable name for loop item (defaults to "item")
}

func (f *FunctionCall) UnmarshalYAML(unmarshal func(interface{}) error) error {
	// Try string first (simple format for backward compatibility)
	var str string
	if err := unmarshal(&str); err == nil {
		f.Name = str
		f.Params = nil
		f.RunOnlyIf = nil
		f.ShouldBeHandledAsMessageToUser = nil
		f.RequiresUserConfirmation = nil
		f.ForEach = nil
		return nil
	}

	// Try object format
	type rawFunctionCall struct {
		Name                           string                 `yaml:"name"`
		Params                         map[string]interface{} `yaml:"params,omitempty"`
		RunOnlyIf                      interface{}            `yaml:"runOnlyIf,omitempty"`
		ShouldBeHandledAsMessageToUser *bool                  `yaml:"shouldBeHandledAsMessageToUser,omitempty"`
		RequiresUserConfirmation       interface{}            `yaml:"requiresUserConfirmation,omitempty"`
		ForEach                        *CallbackForEach       `yaml:"foreach,omitempty"`
	}
	var raw rawFunctionCall
	if err := unmarshal(&raw); err != nil {
		return err
	}

	f.Name = raw.Name
	f.Params = raw.Params
	f.RunOnlyIf = raw.RunOnlyIf
	f.ShouldBeHandledAsMessageToUser = raw.ShouldBeHandledAsMessageToUser
	f.RequiresUserConfirmation = raw.RequiresUserConfirmation
	f.ForEach = raw.ForEach
	return nil
}

const (
	// CombineMode constants for RunOnlyIf and ReRunIf
	CombineModeAND = "AND" // Both conditions must be true
	CombineModeOR  = "OR"  // At least one condition must be true
)

const (
	// ReRunScope constants for ReRunIf
	ReRunScopeSteps = "steps" // Only re-run operation steps, keep resolved inputs (default)
	ReRunScopeFull  = "full"  // Re-evaluate inputs and dependencies on each retry
)

const (
	// Default values for ReRunIf
	ReRunIfDefaultMaxRetries = 1000
)

// CacheScope defines the scope of caching
type CacheScope string

const (
	CacheScopeGlobal  CacheScope = "global"  // Cache globally across all users/messages (default)
	CacheScopeClient  CacheScope = "client"  // Cache per client ID
	CacheScopeMessage CacheScope = "message" // Cache per message ID
)

// CacheConfig represents the cache configuration.
// Supports both simple format (int for backwards compatibility) and object format.
// Simple: cache: 300
// Object: cache: { scope: "client", ttl: 300, includeInputs: false }
type CacheConfig struct {
	Scope         CacheScope `yaml:"scope,omitempty" json:"scope,omitempty"`                 // default: "global"
	TTL           int        `yaml:"ttl,omitempty" json:"ttl,omitempty"`                     // seconds
	IncludeInputs *bool      `yaml:"includeInputs,omitempty" json:"includeInputs,omitempty"` // default: true
}

// GetIncludeInputs returns the value of IncludeInputs, defaulting to true if nil
func (c *CacheConfig) GetIncludeInputs() bool {
	if c.IncludeInputs == nil {
		return true
	}
	return *c.IncludeInputs
}

// GetScope returns the scope, defaulting to global if empty
func (c *CacheConfig) GetScope() CacheScope {
	if c.Scope == "" {
		return CacheScopeGlobal
	}
	return c.Scope
}

// Sanitization strategy constants
const (
	SanitizeStrategyFence      = "fence"
	SanitizeStrategyStrict     = "strict"
	SanitizeStrategyLLMExtract = "llm_extract"
)

// SanitizeConfig represents the sanitization configuration for prompt injection protection.
// Applied to function outputs or input values to prevent malicious content from hijacking the LLM agent.
// Supports three strategies:
//   - "fence": Wraps content with nonce-based delimiters (lightest, no content modification)
//   - "strict": Fence + regex pattern stripping of known injection patterns
//   - "llm_extract": Fence + structured extraction via separate LLM (most robust)
type SanitizeConfig struct {
	Strategy       string         `yaml:"strategy"`                 // "fence", "strict", or "llm_extract"
	MaxLength      int            `yaml:"maxLength,omitempty"`      // Optional: truncate content before sanitization
	CustomPatterns []string       `yaml:"customPatterns,omitempty"` // Additional regex patterns (strict mode only)
	Extract        []ExtractField `yaml:"extract,omitempty"`        // Fields to extract (llm_extract mode only)
}

// ExtractField defines a field to extract when using the "llm_extract" strategy.
type ExtractField struct {
	Field       string `yaml:"field"`
	Description string `yaml:"description"`
}

type RunOnlyIfObject struct {
	// Simple inference mode (backward compatible) - evaluated using LLM
	Condition string `yaml:"condition,omitempty"` // The condition to evaluate using inference

	// Deterministic mode - evaluated using simple comparisons with && and || operators
	// Supports: ==, !=, >, <, >=, <=
	// Examples: "$getUserStatus.isActive == true && $getBalance.amount > 100"
	//           "$users[0].age >= 18"
	//           "$payment.status == 'completed' && $payment.amount > 0"
	Deterministic string `yaml:"deterministic,omitempty"`

	// Advanced inference mode with its own configuration
	Inference *InferenceCondition `yaml:"inference,omitempty"`

	// How to combine deterministic and inference results (only used when both are present)
	// Values: CombineModeAND (both must be true), CombineModeOR (at least one must be true)
	// Default: CombineModeAND
	CombineMode string `yaml:"combineWith,omitempty"`

	// Shared configuration
	OnError                   *OnError       `yaml:"onError,omitempty"`                   // Optional error handling
	From                      []string       `yaml:"from,omitempty"`                      // Optional list of functions to include in context (used when Condition is set)
	AllowedSystemFunctions    []string       `yaml:"allowedSystemFunctions,omitempty"`    // Optional list of system functions allowed for agentic inference (used when Condition is set)
	DisableAllSystemFunctions bool           `yaml:"disableAllSystemFunctions,omitempty"` // If true, no system functions will be available for agentic inference (used when Condition is set)
	ClientIds                 string         `yaml:"clientIds,omitempty"`                 // Dynamic variable like "$clientIds" for queryCustomerServiceChats (staff-only)
	MemoryFilters             *MemoryFilters `yaml:"memoryFilters,omitempty"`             // Optional filters for queryMemories system function
	CodebaseDirs              string         `yaml:"codebaseDirs,omitempty"`              // Variable ref resolving to repo paths for searchCodebase
	DocumentDbName            string         `yaml:"documentDbName,omitempty"`            // Variable ref or literal DB name for queryDocuments (resolved to <connectaiDir>/goreason/<name>.db)
	DocumentEnableGraph       bool           `yaml:"documentEnableGraph,omitempty"`       // If true, enable knowledge graph for queryDocuments (default: env var GOREASON_ENABLE_GRAPH)
}

// InferenceCondition represents an inference-based condition with its own configuration
type InferenceCondition struct {
	Condition                 string   `yaml:"condition"`                           // The condition to evaluate using inference
	From                      []string `yaml:"from,omitempty"`                      // Optional list of functions to include in context
	AllowedSystemFunctions    []string `yaml:"allowedSystemFunctions,omitempty"`    // Optional list of system functions allowed
	DisableAllSystemFunctions bool     `yaml:"disableAllSystemFunctions,omitempty"` // If true, no system functions will be available
	ClientIds                 string   `yaml:"clientIds,omitempty"`                 // Dynamic variable like "$clientIds" for queryCustomerServiceChats (staff-only)
	CodebaseDirs              string   `yaml:"codebaseDirs,omitempty"`              // Variable ref resolving to repo paths for searchCodebase
	DocumentDbName            string   `yaml:"documentDbName,omitempty"`            // Variable ref or literal DB name for queryDocuments (resolved to <connectaiDir>/goreason/<name>.db)
	DocumentEnableGraph       bool     `yaml:"documentEnableGraph,omitempty"`       // If true, enable knowledge graph for queryDocuments (default: env var GOREASON_ENABLE_GRAPH)
}

// ReRunIfConfig defines the configuration for function re-execution conditions.
// This allows functions to be re-executed based on conditions evaluated after execution
// but before onSuccess callbacks.
//
// Supports multiple condition modes:
// - Deterministic: Boolean expressions like "$result.status != 'ready'"
// - Function call: Call a function that returns truthy/falsy
// - Inference: LLM-based evaluation for complex conditions
// - Hybrid: Combine multiple modes with configurable AND/OR logic
//
// Example YAML:
//
//	reRunIf:
//	  deterministic: "$result.status == 'pending'"
//	  maxRetries: 10
//	  delayMs: 1000
//	  scope: "steps"
type ReRunIfConfig struct {
	// Deterministic condition using boolean expressions
	// Example: "$result.status != 'success'" or "len($result.items) == 0"
	// Supports: ==, !=, >, <, >=, <=, &&, ||, len(), isEmpty(), contains(), exists()
	// Access to $result (function output), $RETRY.count, $inputName, result[N] (step results)
	Deterministic string `yaml:"deterministic,omitempty"`

	// Inference-based condition (LLM evaluation)
	Inference *InferenceCondition `yaml:"inference,omitempty"`

	// Simple inference condition (backward compatible)
	Condition string `yaml:"condition,omitempty"`

	// Function call that returns true/false, 0/1 to determine if re-run
	// Example: { name: "checkResult", params: { output: "$result" } }
	Call *FunctionCall `yaml:"call,omitempty"`

	// How to combine multiple conditions (AND/OR)
	// "and" = re-run only if ALL conditions are true
	// "or"  = re-run if ANY condition is true (default)
	// Default: "or"
	CombineMode string `yaml:"combineWith,omitempty"`

	// Maximum number of retries
	// Default: 1000
	MaxRetries int `yaml:"maxRetries,omitempty"`

	// Delay between retries in milliseconds
	// Default: 0 (no delay)
	DelayMs int `yaml:"delayMs,omitempty"`

	// Retry scope: what to re-execute on retry
	// "steps" (default) = only re-run operation steps, keep resolved inputs
	// "full" = re-evaluate inputs and dependencies on each retry
	Scope string `yaml:"scope,omitempty"`

	// Context filtering for inference (same as runOnlyIf)
	From                      []string `yaml:"from,omitempty"`
	AllowedSystemFunctions    []string `yaml:"allowedSystemFunctions,omitempty"`
	DisableAllSystemFunctions bool     `yaml:"disableAllSystemFunctions,omitempty"`

	// Params allows overriding input values for the next retry iteration.
	// Uses same variable replacement as onSuccess callbacks.
	// Example: { pageToken: "$result.nextPageToken", offset: "$offset + 100" }
	Params map[string]interface{} `yaml:"params,omitempty"`
}

// GetMaxRetries returns the max retries, defaulting to 1000 if not set
func (r *ReRunIfConfig) GetMaxRetries() int {
	if r.MaxRetries <= 0 {
		return ReRunIfDefaultMaxRetries
	}
	return r.MaxRetries
}

// GetScope returns the scope, defaulting to "steps" if not set
func (r *ReRunIfConfig) GetScope() string {
	if r.Scope == "" {
		return ReRunScopeSteps
	}
	return r.Scope
}

// GetCombineMode returns the combine mode, defaulting to "or" if not set
func (r *ReRunIfConfig) GetCombineMode() string {
	if r.CombineMode == "" {
		return CombineModeOR
	}
	return strings.ToUpper(r.CombineMode)
}

// HasConditions returns true if any condition is defined
func (r *ReRunIfConfig) HasConditions() bool {
	return r.Deterministic != "" || r.Condition != "" || r.Inference != nil || r.Call != nil
}

// ParseReRunIf parses the ReRunIf field which can be either a string or ReRunIfConfig
// Returns nil if reRunIf is nil or empty
func ParseReRunIf(reRunIf interface{}) (*ReRunIfConfig, error) {
	if reRunIf == nil {
		return nil, nil
	}

	// Handle simple string format (deterministic shorthand)
	if str, ok := reRunIf.(string); ok {
		if str == "" {
			return nil, nil
		}
		return &ReRunIfConfig{
			Deterministic: str,
			MaxRetries:    ReRunIfDefaultMaxRetries,
		}, nil
	}

	// Handle map[string]interface{} case
	if configMap, ok := reRunIf.(map[string]interface{}); ok {
		return parseReRunIfFromMap(configMap)
	}

	// Handle map[interface{}]interface{} case from YAML parsing
	if configMap, ok := reRunIf.(map[interface{}]interface{}); ok {
		stringMap := make(map[string]interface{})
		for k, v := range configMap {
			if keyStr, ok := k.(string); ok {
				stringMap[keyStr] = v
			}
		}
		return parseReRunIfFromMap(stringMap)
	}

	return nil, fmt.Errorf("reRunIf must be a string or an object")
}

// parseReRunIfFromMap parses ReRunIfConfig from a map
func parseReRunIfFromMap(m map[string]interface{}) (*ReRunIfConfig, error) {
	config := &ReRunIfConfig{}

	// Parse deterministic
	if v, ok := m["deterministic"].(string); ok {
		config.Deterministic = v
	}

	// Parse condition (simple inference)
	if v, ok := m["condition"].(string); ok {
		config.Condition = v
	}

	// Parse inference
	if v, ok := m["inference"]; ok && v != nil {
		inferenceConfig, err := parseInferenceConditionFromInterface(v)
		if err != nil {
			return nil, fmt.Errorf("reRunIf.inference: %w", err)
		}
		config.Inference = inferenceConfig
	}

	// Parse call
	if v, ok := m["call"]; ok && v != nil {
		call, err := parseFunctionCallFromInterface(v)
		if err != nil {
			return nil, fmt.Errorf("reRunIf.call: %w", err)
		}
		config.Call = call
	}

	// Parse combineWith
	if v, ok := m["combineWith"].(string); ok {
		config.CombineMode = v
	}

	// Parse maxRetries
	if v, ok := m["maxRetries"].(int); ok {
		config.MaxRetries = v
	} else if v, ok := m["maxRetries"].(float64); ok {
		config.MaxRetries = int(v)
	}

	// Parse delayMs
	if v, ok := m["delayMs"].(int); ok {
		config.DelayMs = v
	} else if v, ok := m["delayMs"].(float64); ok {
		config.DelayMs = int(v)
	}

	// Parse scope
	if v, ok := m["scope"].(string); ok {
		config.Scope = v
	}

	// Parse from
	if v, ok := m["from"]; ok && v != nil {
		fromSlice, err := parseStringSlice(v)
		if err != nil {
			return nil, fmt.Errorf("reRunIf.from: %w", err)
		}
		config.From = fromSlice
	}

	// Parse allowedSystemFunctions
	if v, ok := m["allowedSystemFunctions"]; ok && v != nil {
		funcsSlice, err := parseStringSlice(v)
		if err != nil {
			return nil, fmt.Errorf("reRunIf.allowedSystemFunctions: %w", err)
		}
		config.AllowedSystemFunctions = funcsSlice
	}

	// Parse disableAllSystemFunctions
	if v, ok := m["disableAllSystemFunctions"].(bool); ok {
		config.DisableAllSystemFunctions = v
	}

	// Parse params
	if v, ok := m["params"]; ok && v != nil {
		paramsMap, err := parseMapInterface(v)
		if err != nil {
			return nil, fmt.Errorf("reRunIf.params: %w", err)
		}
		config.Params = paramsMap
	}

	return config, nil
}

// parseInferenceConditionFromInterface parses InferenceCondition from interface{}
func parseInferenceConditionFromInterface(v interface{}) (*InferenceCondition, error) {
	var m map[string]interface{}

	switch t := v.(type) {
	case map[string]interface{}:
		m = t
	case map[interface{}]interface{}:
		m = make(map[string]interface{})
		for k, val := range t {
			if keyStr, ok := k.(string); ok {
				m[keyStr] = val
			}
		}
	default:
		return nil, fmt.Errorf("must be an object")
	}

	config := &InferenceCondition{}

	if v, ok := m["condition"].(string); ok {
		config.Condition = v
	}

	if v, ok := m["from"]; ok && v != nil {
		fromSlice, err := parseStringSlice(v)
		if err != nil {
			return nil, fmt.Errorf("from: %w", err)
		}
		config.From = fromSlice
	}

	if v, ok := m["allowedSystemFunctions"]; ok && v != nil {
		funcsSlice, err := parseStringSlice(v)
		if err != nil {
			return nil, fmt.Errorf("allowedSystemFunctions: %w", err)
		}
		config.AllowedSystemFunctions = funcsSlice
	}

	if v, ok := m["disableAllSystemFunctions"].(bool); ok {
		config.DisableAllSystemFunctions = v
	}

	if v, ok := m["clientIds"].(string); ok {
		config.ClientIds = v
	}

	return config, nil
}

// parseFunctionCallFromInterface parses FunctionCall from interface{}
func parseFunctionCallFromInterface(v interface{}) (*FunctionCall, error) {
	// Handle string case (simple name)
	if str, ok := v.(string); ok {
		return &FunctionCall{Name: str}, nil
	}

	var m map[string]interface{}

	switch t := v.(type) {
	case map[string]interface{}:
		m = t
	case map[interface{}]interface{}:
		m = make(map[string]interface{})
		for k, val := range t {
			if keyStr, ok := k.(string); ok {
				m[keyStr] = val
			}
		}
	default:
		return nil, fmt.Errorf("must be a string or object")
	}

	call := &FunctionCall{}

	if v, ok := m["name"].(string); ok {
		call.Name = v
	} else {
		return nil, fmt.Errorf("name is required")
	}

	if v, ok := m["params"]; ok && v != nil {
		switch t := v.(type) {
		case map[string]interface{}:
			call.Params = t
		case map[interface{}]interface{}:
			call.Params = make(map[string]interface{})
			for k, val := range t {
				if keyStr, ok := k.(string); ok {
					call.Params[keyStr] = val
				}
			}
		}
	}

	return call, nil
}

// parseStringSlice parses a string slice from interface{}
func parseStringSlice(v interface{}) ([]string, error) {
	switch t := v.(type) {
	case []string:
		return t, nil
	case []interface{}:
		result := make([]string, 0, len(t))
		for _, item := range t {
			if str, ok := item.(string); ok {
				result = append(result, str)
			} else {
				return nil, fmt.Errorf("all items must be strings")
			}
		}
		return result, nil
	default:
		return nil, fmt.Errorf("must be an array of strings")
	}
}

// parseMapInterface parses a map[string]interface{} from an interface{}
// Handles both map[string]interface{} and map[interface{}]interface{} (from YAML parsing)
func parseMapInterface(v interface{}) (map[string]interface{}, error) {
	switch t := v.(type) {
	case map[string]interface{}:
		return t, nil
	case map[interface{}]interface{}:
		result := make(map[string]interface{})
		for k, val := range t {
			if keyStr, ok := k.(string); ok {
				result[keyStr] = val
			} else {
				return nil, fmt.Errorf("all keys must be strings")
			}
		}
		return result, nil
	default:
		return nil, fmt.Errorf("must be an object")
	}
}

type CallRule struct {
	Type            string   `yaml:"type"`                      // once, unique, multiple
	Scope           string   `yaml:"scope,omitempty"`           // user, message, minimumInterval
	MinimumInterval int      `yaml:"minimumInterval,omitempty"` // seconds (only for minimumInterval scope)
	StatusFilter    []string `yaml:"statusFilter,omitempty"`    // which execution statuses count: completed, failed, all (default: all)
}

// RequiresUserConfirmationConfig represents the configuration for user confirmation
type RequiresUserConfirmationConfig struct {
	Enabled bool   `yaml:"enabled"`           // Whether user confirmation is required
	Message string `yaml:"message,omitempty"` // Custom message to display to the user
}

// ParseRequiresUserConfirmation parses the RequiresUserConfirmation field which can be either a bool or an object
// Returns: (enabled, customMessage, error)
func ParseRequiresUserConfirmation(requiresUserConfirmation interface{}) (bool, string, error) {
	if requiresUserConfirmation == nil {
		return false, "", nil
	}

	// Handle boolean case (backward compatibility)
	if boolVal, ok := requiresUserConfirmation.(bool); ok {
		return boolVal, "", nil
	}

	// Handle object case with map[string]interface{}
	if configMap, ok := requiresUserConfirmation.(map[string]interface{}); ok {
		enabledVal, hasEnabled := configMap["enabled"]
		messageVal, hasMessage := configMap["message"]

		if !hasEnabled {
			return false, "", fmt.Errorf("requiresUserConfirmation object must have an 'enabled' field")
		}

		enabledBool, ok := enabledVal.(bool)
		if !ok {
			return false, "", fmt.Errorf("requiresUserConfirmation.enabled must be a boolean")
		}

		if hasMessage && enabledBool {
			if messageStr, ok := messageVal.(string); ok {
				return enabledBool, messageStr, nil
			}
			return false, "", fmt.Errorf("requiresUserConfirmation.message must be a string")
		}

		return enabledBool, "", nil
	}

	// Handle map[interface{}]interface{} case from YAML parsing
	if configMap, ok := requiresUserConfirmation.(map[interface{}]interface{}); ok {
		enabledVal, hasEnabled := configMap["enabled"]
		messageVal, hasMessage := configMap["message"]

		if !hasEnabled {
			return false, "", fmt.Errorf("requiresUserConfirmation object must have an 'enabled' field")
		}

		enabledBool, ok := enabledVal.(bool)
		if !ok {
			return false, "", fmt.Errorf("requiresUserConfirmation.enabled must be a boolean")
		}

		if hasMessage && enabledBool {
			if messageStr, ok := messageVal.(string); ok {
				return enabledBool, messageStr, nil
			}
			return false, "", fmt.Errorf("requiresUserConfirmation.message must be a string")
		}

		return enabledBool, "", nil
	}

	return false, "", fmt.Errorf("requiresUserConfirmation must be a boolean or an object")
}

type Trigger struct {
	Type               string              `yaml:"type"`
	Cron               string              `yaml:"cron,omitempty"`
	ConcurrencyControl *ConcurrencyControl `yaml:"concurrencyControl,omitempty"`
}

// ConcurrencyControl defines how to handle overlapping cron executions
type ConcurrencyControl struct {
	Strategy    string `yaml:"strategy,omitempty"`    // "parallel" (default), "skip", or "kill"
	MaxParallel int    `yaml:"maxParallel,omitempty"` // 1-10, default: 3 (only for parallel strategy)
	KillTimeout int    `yaml:"killTimeout,omitempty"` // seconds, default: 600
}

type InputWithOptions struct {
	OneOf    string `yaml:"oneOf,omitempty" json:"oneOf,omitempty"`
	ManyOf   string `yaml:"manyOf,omitempty" json:"manyOf,omitempty"`
	NotOneOf string `yaml:"notOneOf,omitempty" json:"notOneOf,omitempty"`
	TTL      int    `yaml:"ttl,omitempty" json:"ttl,omitempty"` // TTL in minutes, defaults to 60
}

// ========================================
// Memory Filtering Constants
// ========================================

// MemoryTopic constants define allowed topic values for memory filtering
// These correspond to the Topic column in the vector_documents table
const (
	// MemoryTopicMeetingTranscript is set when persisting meeting notes from meeting bots
	MemoryTopicMeetingTranscript = "meeting_transcript"

	// MemoryTopicFunctionExecuted is set when persisting memories from function executions
	MemoryTopicFunctionExecuted = "function_executed"

	// MemoryTopicMeetingChat is set when persisting meeting chat messages
	MemoryTopicMeetingChat = "meeting_chat"
)

// AllowedMemoryTopics lists all valid topic values for memoryFilters.topic
var AllowedMemoryTopics = map[string]bool{
	MemoryTopicMeetingTranscript: true,
	MemoryTopicFunctionExecuted:  true,
	MemoryTopicMeetingChat:       true,
}

// AllowedMemoryMetadataKeys lists all valid metadata keys for memoryFilters.metadata
// These keys are stored in the metadata JSON column and can be filtered using json_extract
//
// For meeting_transcript memories:
//   - company_id: The company ID associated with the meeting
//   - meeting_url: URL of the meeting
//   - bot_name: Name of the meeting bot
//   - created_by: Who created the meeting bot
//   - meeting_topic: Topic/subject of the meeting
//   - meeting_with_person: Name of the person the meeting is with
//   - bot_id: ID of the meeting bot
//
// For function_executed memories:
//   - client_id: The client ID the function was executed for
//   - message_id: The message ID that triggered the function
//   - function_name: Name of the function that was executed
//   - tool_name: Name of the tool containing the function
//   - event_key: Event key identifier
//   - user_message: The user message that triggered the function
//   - has_error: Whether the function execution had an error (true/false)
//
// Common fields (added automatically):
//   - timestamp: When the memory was created (RFC3339 format)
//   - type: Type of memory (e.g., "agentic_memory", "meeting_transcript")
//   - datetime: Date/time field (for meeting transcripts)
var AllowedMemoryMetadataKeys = map[string]bool{
	// Meeting transcript metadata keys
	"company_id":          true,
	"meeting_url":         true,
	"bot_name":            true,
	"created_by":          true,
	"meeting_topic":       true,
	"meeting_with_person": true,
	"bot_id":              true,

	// Function execution metadata keys
	"client_id":     true,
	"message_id":    true,
	"function_name": true,
	"tool_name":     true,
	"event_key":     true,
	"user_message":  true,
	"has_error":     true,

	// Common metadata keys
	"timestamp": true,
	"type":      true,
	"datetime":  true,
}

// IsValidMemoryTopic checks if a topic is a valid memory topic
func IsValidMemoryTopic(topic string) bool {
	return AllowedMemoryTopics[topic]
}

// IsValidMemoryMetadataKey checks if a key is a valid memory metadata key
func IsValidMemoryMetadataKey(key string) bool {
	return AllowedMemoryMetadataKeys[key]
}

// GetAllowedMemoryTopicsList returns a slice of all allowed topic values
func GetAllowedMemoryTopicsList() []string {
	topics := make([]string, 0, len(AllowedMemoryTopics))
	for topic := range AllowedMemoryTopics {
		topics = append(topics, topic)
	}
	return topics
}

// GetAllowedMemoryMetadataKeysList returns a slice of all allowed metadata keys
func GetAllowedMemoryMetadataKeysList() []string {
	keys := make([]string, 0, len(AllowedMemoryMetadataKeys))
	for key := range AllowedMemoryMetadataKeys {
		keys = append(keys, key)
	}
	return keys
}

// MemoryTimeRange defines time range filters for memory queries
// Dates must be in YYYY-MM-DD format (e.g., "2024-01-15")
// Values can be variable references (e.g., "$lastWeek") that will be resolved at runtime
type MemoryTimeRange struct {
	After  string `yaml:"after,omitempty"`  // Filter memories created after this date (inclusive)
	Before string `yaml:"before,omitempty"` // Filter memories created before this date (inclusive)
}

// MetadataFilterOperation defines the operation type for metadata filtering
type MetadataFilterOperation string

const (
	// MetadataFilterOperationExact matches the exact value (default)
	MetadataFilterOperationExact MetadataFilterOperation = "exact"
	// MetadataFilterOperationContains matches values containing the substring
	MetadataFilterOperationContains MetadataFilterOperation = "contains"
)

// MetadataFilterValue represents a metadata filter with optional operation
// Can be used for keys that support "contains" operation (meeting_with_person, meeting_topic)
type MetadataFilterValue struct {
	Value     string                  `yaml:"value"`               // The value to filter by
	Operation MetadataFilterOperation `yaml:"operation,omitempty"` // "exact" (default) or "contains"
}

// AllowedContainsOperationKeys lists metadata keys that support the "contains" operation
var AllowedContainsOperationKeys = map[string]bool{
	"meeting_with_person": true,
	"meeting_topic":       true,
}

// SupportsContainsOperation checks if a metadata key supports the "contains" operation
func SupportsContainsOperation(key string) bool {
	return AllowedContainsOperationKeys[key]
}

// MemoryFilters defines filters for the queryMemories system function
// Used to filter memories by topic, metadata fields, and/or time range
type MemoryFilters struct {
	Topic     []string               `yaml:"topic,omitempty"`     // Filter by Topic column (must be values from AllowedMemoryTopics)
	Metadata  map[string]interface{} `yaml:"metadata,omitempty"`  // Filter by metadata JSON fields (keys must be in AllowedMemoryMetadataKeys)
	TimeRange *MemoryTimeRange       `yaml:"timeRange,omitempty"` // Filter by creation date range (format: YYYY-MM-DD)
}

// MemoryDateFormat is the expected date format for timeRange filters
const MemoryDateFormat = "2006-01-02"

// IsValidMemoryDateFormat checks if a date string is in YYYY-MM-DD format
// Returns false for variable references (starting with $) as they need runtime resolution
func IsValidMemoryDateFormat(dateStr string) bool {
	if dateStr == "" {
		return true // Empty is valid (no filter)
	}
	if strings.HasPrefix(dateStr, "$") {
		return true // Variable reference, will be resolved at runtime
	}
	_, err := time.Parse(MemoryDateFormat, dateStr)
	return err == nil
}

type SuccessCriteriaObject struct {
	Condition                 string         `yaml:"condition"`                           // The success criteria condition
	From                      []string       `yaml:"from,omitempty"`                      // Optional list of functions to include in context
	AllowedSystemFunctions    []string       `yaml:"allowedSystemFunctions,omitempty"`    // Optional list of system functions allowed for agentic inference
	DisableAllSystemFunctions bool           `yaml:"disableAllSystemFunctions,omitempty"` // If true, no system functions will be available for agentic inference
	ClientIds                 string         `yaml:"clientIds,omitempty"`                 // Dynamic variable like "$clientIds" for queryCustomerServiceChats (staff-only)
	MemoryFilters             *MemoryFilters `yaml:"memoryFilters,omitempty"`             // Optional filters for queryMemories system function
	CodebaseDirs              string         `yaml:"codebaseDirs,omitempty"`              // Variable ref resolving to repo paths for searchCodebase
	DocumentDbName            string         `yaml:"documentDbName,omitempty"`            // Variable ref or literal DB name for queryDocuments (resolved to <connectaiDir>/goreason/<name>.db)
	DocumentEnableGraph       bool           `yaml:"documentEnableGraph,omitempty"`       // If true, enable knowledge graph for queryDocuments (default: env var GOREASON_ENABLE_GRAPH)
}

type Input struct {
	Name                           string                 `yaml:"name" json:"name"`
	IsOptional                     bool                   `yaml:"isOptional" json:"isOptional"`
	Description                    string                 `yaml:"description" json:"description"`
	Origin                         string                 `yaml:"origin" json:"origin,omitempty"`
	From                           string                 `yaml:"from,omitempty" json:"from,omitempty"`
	Params                         map[string]interface{} `yaml:"params,omitempty" json:"params,omitempty"`                   // Params to inject when using origin: function with from
	SuccessCriteria                interface{}            `yaml:"successCriteria,omitempty" json:"successCriteria,omitempty"` // Can be string or SuccessCriteriaObject
	Value                          string                 `yaml:"value,omitempty" json:"value,omitempty"`
	With                           *InputWithOptions      `yaml:"with,omitempty" json:"with,omitempty"`
	OnError                        *OnError               `yaml:"onError,omitempty" json:"onError,omitempty"`
	RegexValidator                 string                 `yaml:"regexValidator,omitempty" json:"regexValidator,omitempty"`
	JsonSchemaValidator            string                 `yaml:"jsonSchemaValidator,omitempty" json:"jsonSchemaValidator,omitempty"`
	Cache                          int                    `yaml:"cache,omitempty" json:"cache,omitempty"`
	ShouldBeHandledAsMessageToUser bool                   `yaml:"shouldBeHandledAsMessageToUser,omitempty" json:"shouldBeHandledAsMessageToUser,omitempty"` // Only for format operation
	MemoryRetrievalMode            string                 `yaml:"memoryRetrievalMode,omitempty" json:"memoryRetrievalMode,omitempty"`                       // Only for memory origin: "all" or "latest" (default: "latest")
	RunOnlyIf                      interface{}            `yaml:"runOnlyIf,omitempty" json:"runOnlyIf,omitempty"`                                           // Deterministic-only condition for conditional input evaluation
	Async                          bool                   `yaml:"async,omitempty" json:"async,omitempty"`                                                   // If true, input can be resolved in parallel with other async inputs
	Sanitize                       interface{}            `yaml:"sanitize,omitempty" json:"sanitize,omitempty"`                                             // Prompt injection protection: bool, string, or SanitizeConfig object
	ParsedSanitize                 *SanitizeConfig        `yaml:"-" json:"-"`                                                                               // Parsed sanitize config (populated during parsing)
}

// OnError defines how to handle errors during input fulfillment
// Supports three modes:
// 1. Strategy + Message: Handle error with predefined strategy (requestUserInput, requestN1Support, etc.)
// 2. Call only: Execute a function transparently without surfacing error to user (e.g., to provide context)
// 3. Strategy + Message + Call: Execute function AND handle error (both behaviors applied)
type OnError struct {
	Strategy string            `yaml:"strategy"`                             // Error handling strategy (optional if Call is specified)
	Message  string            `yaml:"message"`                              // Message to display/send (optional if Call is specified)
	With     *InputWithOptions `yaml:"with,omitempty" json:"with,omitempty"` // Additional options for certain strategies
	Call     *FunctionCall     `yaml:"-" json:"call,omitempty"`              // Optional function to call on error (executed before strategy handling)
}

// UnmarshalYAML implements yaml.Unmarshaler to support both string and object formats for Call
func (o *OnError) UnmarshalYAML(unmarshal func(interface{}) error) error {
	// Use a type alias to avoid infinite recursion
	type rawOnError struct {
		Strategy string            `yaml:"strategy"`
		Message  string            `yaml:"message"`
		With     *InputWithOptions `yaml:"with,omitempty"`
		Call     interface{}       `yaml:"call,omitempty"` // Can be string or object
	}

	var raw rawOnError
	if err := unmarshal(&raw); err != nil {
		return err
	}

	o.Strategy = raw.Strategy
	o.Message = raw.Message
	o.With = raw.With

	// Handle Call field which can be string or object
	if raw.Call != nil {
		switch v := raw.Call.(type) {
		case string:
			// Simple string format (backward compatible): call: "functionName"
			if v != "" {
				o.Call = &FunctionCall{Name: v}
			}
		case map[string]interface{}:
			// Object format: call: { name: "functionName", params: { ... } }
			fc := &FunctionCall{}
			if name, ok := v["name"].(string); ok {
				fc.Name = name
			}
			if params, ok := v["params"].(map[string]interface{}); ok {
				fc.Params = params
			}
			// Handle map[interface{}]interface{} for params (from YAML parsing)
			if params, ok := v["params"].(map[interface{}]interface{}); ok {
				fc.Params = make(map[string]interface{})
				for k, val := range params {
					if keyStr, ok := k.(string); ok {
						fc.Params[keyStr] = val
					}
				}
			}
			if fc.Name != "" {
				o.Call = fc
			}
		case map[interface{}]interface{}:
			// Handle map[interface{}]interface{} case from YAML parsing
			fc := &FunctionCall{}
			if name, ok := v["name"].(string); ok {
				fc.Name = name
			}
			if params, ok := v["params"].(map[string]interface{}); ok {
				fc.Params = params
			}
			if params, ok := v["params"].(map[interface{}]interface{}); ok {
				fc.Params = make(map[string]interface{})
				for k, val := range params {
					if keyStr, ok := k.(string); ok {
						fc.Params[keyStr] = val
					}
				}
			}
			if fc.Name != "" {
				o.Call = fc
			}
		}
	}

	return nil
}

type ZeroState struct {
	Steps []Step `yaml:"steps"`
}

type Step struct {
	Name             string                 `yaml:"name"`
	Action           string                 `yaml:"action"`
	Goal             string                 `yaml:"goal,omitempty"`
	IsAuthentication bool                   `yaml:"isAuthentication,omitempty"`
	With             map[string]interface{} `yaml:"with,omitempty"`
	OnError          *OnError               `yaml:"onError,omitempty"`
	ResultIndex      int                    `yaml:"resultIndex,omitempty"`
	ForEach          *ForEach               `yaml:"foreach,omitempty"`
	ForEachCamelCase interface{}            `yaml:"forEach,omitempty"` // Captures common misspelling to provide helpful error
	Response         map[string]interface{} `yaml:"response,omitempty"`
	RunOnlyIf        interface{}            `yaml:"runOnlyIf,omitempty"` // Deterministic condition to evaluate before executing step (api_call, terminal, db operations only)
	SaveAsFile       *SaveAsFileConfig      `yaml:"saveAsFile,omitempty" json:"saveAsFile,omitempty"`
}

// SaveAsFileConfig configures an api_call step to treat the HTTP response as a file download.
// When present, the raw response bytes are captured as a FileResult instead of being parsed as JSON/text.
// The file can then be uploaded to agent-proxy (via output.upload: true) and/or sent to the user
// (via shouldBeHandledAsMessageToUser: true).
type SaveAsFileConfig struct {
	FileName    string `yaml:"fileName" json:"fileName"`                           // Output filename (supports $variables)
	MimeType    string `yaml:"mimeType,omitempty" json:"mimeType,omitempty"`       // Explicit MIME type override (auto-detected from response Content-Type if empty)
	MaxFileSize int64  `yaml:"maxFileSize,omitempty" json:"maxFileSize,omitempty"` // Max file size in bytes (default: 25MB, hard cap: 100MB)
}

// ExtractAuthToken configures authentication token extraction from API responses.
// Only valid for steps with isAuthentication: true.
// The extracted token is made available as $AUTH_TOKEN for subsequent steps.
type ExtractAuthToken struct {
	From  string `yaml:"from"`            // "header" or "responseBody" - where to extract the token from
	Key   string `yaml:"key"`             // Header name (e.g., "Authorization") or JSON path (e.g., "data.access_token")
	Cache int    `yaml:"cache,omitempty"` // TTL in seconds, default: 7200 (2 hours), 0 means no caching
}

type ForEach struct {
	Items      string      `yaml:"items"`               // Variable reference to iterate over (e.g., "$itemList")
	Separator  string      `yaml:"separator,omitempty"` // Separator for string splitting (defaults to ",")
	IndexVar   string      `yaml:"indexVar,omitempty"`  // Variable name for loop index (defaults to "index")
	ItemVar    string      `yaml:"itemVar,omitempty"`   // Variable name for loop item (defaults to "item")
	BreakIf    interface{} `yaml:"breakIf,omitempty"`   // Optional break condition - can be string (deterministic: "$item.status == 'completed'") or SuccessCriteriaObject (inference)
	WaitFor    *WaitFor    `yaml:"waitFor,omitempty"`   // Optional wait condition - makes foreach sequential and waits for function to return true between iterations
	ShouldSkip interface{} `yaml:"-"`                   // Parsed shouldSkip conditions - use GetShouldSkipConditions() to access
}

// forEachRaw is used for YAML unmarshaling to handle shouldSkip polymorphism
type forEachRaw struct {
	Items      string      `yaml:"items"`
	Separator  string      `yaml:"separator,omitempty"`
	IndexVar   string      `yaml:"indexVar,omitempty"`
	ItemVar    string      `yaml:"itemVar,omitempty"`
	BreakIf    interface{} `yaml:"breakIf,omitempty"`
	WaitFor    *WaitFor    `yaml:"waitFor,omitempty"`
	ShouldSkip interface{} `yaml:"shouldSkip,omitempty"`
}

// UnmarshalYAML implements custom unmarshaling to handle shouldSkip as either single object or array
func (f *ForEach) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var raw forEachRaw
	if err := unmarshal(&raw); err != nil {
		return err
	}

	f.Items = raw.Items
	f.Separator = raw.Separator
	f.IndexVar = raw.IndexVar
	f.ItemVar = raw.ItemVar
	f.BreakIf = raw.BreakIf
	f.WaitFor = raw.WaitFor

	// Parse shouldSkip - can be single object or array
	if raw.ShouldSkip != nil {
		conditions, err := parseShouldSkipConditions(raw.ShouldSkip)
		if err != nil {
			return err
		}
		f.ShouldSkip = conditions
	}

	return nil
}

// parseShouldSkipConditions parses shouldSkip from interface{} into []*ShouldSkip
// Handles both single object and array formats for backwards compatibility
func parseShouldSkipConditions(raw interface{}) ([]*ShouldSkip, error) {
	if raw == nil {
		return nil, nil
	}

	// Try as array first
	if arr, ok := raw.([]interface{}); ok {
		conditions := make([]*ShouldSkip, 0, len(arr))
		for i, item := range arr {
			cond, err := parseSingleShouldSkip(item)
			if err != nil {
				return nil, fmt.Errorf("shouldSkip[%d]: %w", i, err)
			}
			conditions = append(conditions, cond)
		}
		return conditions, nil
	}

	// Try as single object (map)
	cond, err := parseSingleShouldSkip(raw)
	if err != nil {
		return nil, err
	}
	return []*ShouldSkip{cond}, nil
}

// parseSingleShouldSkip parses a single shouldSkip condition from interface{}
func parseSingleShouldSkip(raw interface{}) (*ShouldSkip, error) {
	cond := &ShouldSkip{}

	// Handle map[string]interface{} (common in JSON)
	if m, ok := raw.(map[string]interface{}); ok {
		if name, ok := m["name"].(string); ok {
			cond.Name = name
		}

		if params, ok := m["params"].(map[string]interface{}); ok {
			cond.Params = params
		} else if params, ok := m["params"].(map[interface{}]interface{}); ok {
			cond.Params = convertToStringKeyMap(params)
		}
		return cond, nil
	}

	// Handle map[interface{}]interface{} (common in YAML)
	if m, ok := raw.(map[interface{}]interface{}); ok {
		if name, ok := m["name"].(string); ok {
			cond.Name = name
		}

		if params, ok := m["params"].(map[string]interface{}); ok {
			cond.Params = params
		} else if params, ok := m["params"].(map[interface{}]interface{}); ok {
			cond.Params = convertToStringKeyMap(params)
		}
		return cond, nil
	}

	return nil, fmt.Errorf("shouldSkip must be an object with 'name' field, got %T", raw)
}

// convertToStringKeyMap converts map[interface{}]interface{} to map[string]interface{}
func convertToStringKeyMap(m map[interface{}]interface{}) map[string]interface{} {
	result := make(map[string]interface{})
	for k, v := range m {
		if ks, ok := k.(string); ok {
			result[ks] = v
		}
	}
	return result
}

// GetShouldSkipConditions returns the shouldSkip conditions as a normalized slice.
// Returns nil if no shouldSkip is configured.
func (f *ForEach) GetShouldSkipConditions() []*ShouldSkip {
	if f.ShouldSkip == nil {
		return nil
	}
	if conditions, ok := f.ShouldSkip.([]*ShouldSkip); ok {
		return conditions
	}
	return nil
}

// HasShouldSkip returns true if any shouldSkip conditions are configured
func (f *ForEach) HasShouldSkip() bool {
	return len(f.GetShouldSkipConditions()) > 0
}

// WaitFor defines the wait condition for sequential foreach execution.
// When present, foreach runs sequentially and calls the specified function after each iteration.
// The function must return true (bool) or "true" (string) to proceed to the next iteration.
type WaitFor struct {
	Name                string                 `yaml:"name"`                          // Function name to call after each iteration
	Params              map[string]interface{} `yaml:"params,omitempty"`              // Optional params ($item and $index are auto-injected)
	PollIntervalSeconds int                    `yaml:"pollIntervalSeconds,omitempty"` // Polling interval in seconds (default: 5)
	MaxWaitingSeconds   int                    `yaml:"maxWaitingSeconds,omitempty"`   // Maximum wait time in seconds before error (default: 60)
}

// ShouldSkip defines a function to call at the start of each iteration to determine if it should be skipped.
// When the function returns true (bool) or "true" (string), the iteration is skipped and execution continues with the next item.
// When present, foreach runs sequentially (same as breakIf/waitFor).
// If the function returns an error, the iteration is skipped (treated as if shouldSkip returned true).
type ShouldSkip struct {
	Name   string                 `yaml:"name"`             // Function name to call at start of each iteration
	Params map[string]interface{} `yaml:"params,omitempty"` // Optional params ($item and $index are auto-injected)
}

type Output struct {
	Type           string        `yaml:"type"`
	Fields         []OutputField `yaml:"fields"`
	Value          string        `yaml:"value,omitempty"`          // Used when type is "string"
	Flatten        bool          `yaml:"flatten,omitempty"`        // Flatten nested arrays (for foreach results)
	AllowInference bool          `yaml:"allowInference,omitempty"` // Enable LLM fallback for output formatting (default: false)
	// File handling fields (for type: "file" or "list[file]")
	Upload    bool `yaml:"upload,omitempty"`    // Upload file to agent-proxy (default: true for PDF operations)
	Retention int  `yaml:"retention,omitempty"` // Temp file retention in seconds (default: 600)
}

// FileResult represents a generated file (e.g., from PDF operation)
// This is used as step result when operations produce files
type FileResult struct {
	FileName string `json:"fileName"`           // Name of the generated file
	MimeType string `json:"mimeType"`           // MIME type (e.g., "application/pdf")
	Size     int64  `json:"size"`               // File size in bytes
	TempPath string `json:"tempPath,omitempty"` // Local temp file path (internal use)
	URL      string `json:"url,omitempty"`      // Uploaded URL (set when uploaded to agent-proxy)
}

type OutputField struct {
	Name   string        `yaml:"name,omitempty"`   // If empty, Value is a simple field name
	Value  string        `yaml:"value,omitempty"`  // Simple field name if Name is empty
	Type   string        `yaml:"type,omitempty"`   // Optional: "object", "list[object]", "string", etc.
	Fields []OutputField `yaml:"fields,omitempty"` // Nested fields for objects
}

// UnmarshalYAML implements the yaml.Unmarshaler interface
func (f *OutputField) UnmarshalYAML(unmarshal func(interface{}) error) error {
	// Try unmarshaling as a simple string first
	var str string
	if err := unmarshal(&str); err == nil {
		f.Value = str
		return nil
	}

	// If that fails, try as a complex field
	type rawField OutputField
	var raw rawField
	if err := unmarshal(&raw); err != nil {
		return err
	}

	*f = OutputField(raw)
	return nil
}

// MCPProtocolType defines the supported MCP protocol types
type MCPProtocolType string

const (
	MCPProtocolStdio MCPProtocolType = "stdio"
	MCPProtocolSSE   MCPProtocolType = "sse"
)

type MCPStdioConfig struct {
	Command string            `yaml:"command"`        // The command to execute (e.g., "python", "node")
	Args    []string          `yaml:"args,omitempty"` // Arguments to pass to the command
	Env     map[string]string `yaml:"env,omitempty"`  // Environment variables
}

type MCPSSEConfig struct {
	URL string `yaml:"url"` // The SSE endpoint URL
}

type MCP struct {
	Protocol MCPProtocolType `yaml:"protocol"`        // Protocol type: "stdio" or "sse"
	Stdio    *MCPStdioConfig `yaml:"stdio,omitempty"` // STDIO configuration (required if protocol is "stdio")
	SSE      *MCPSSEConfig   `yaml:"sse,omitempty"`   // SSE configuration (required if protocol is "sse")
	Function string          `yaml:"function"`        // MCP function/tool name to call
}

// =====================================
// PDF Configuration Types
// =====================================

// PDFConfig defines the configuration for PDF generation operations
type PDFConfig struct {
	FileName    string      `yaml:"fileName"`              // Output filename (supports variables like $var.field)
	PageSize    string      `yaml:"pageSize,omitempty"`    // A4, Letter, Legal, A3, A5 (default: A4)
	Orientation string      `yaml:"orientation,omitempty"` // portrait, landscape (default: portrait)
	Margins     *PDFMargins `yaml:"margins,omitempty"`     // Page margins in mm
	Header      *PDFSection `yaml:"header,omitempty"`      // Repeated header on each page
	Body        *PDFSection `yaml:"body,omitempty"`        // Main content
	Footer      *PDFSection `yaml:"footer,omitempty"`      // Repeated footer on each page
}

// PDFMargins defines page margins in millimeters
type PDFMargins struct {
	Top    float64 `yaml:"top,omitempty"`
	Bottom float64 `yaml:"bottom,omitempty"`
	Left   float64 `yaml:"left,omitempty"`
	Right  float64 `yaml:"right,omitempty"`
}

// PDFSection represents a section (header, body, or footer) containing rows
type PDFSection struct {
	Rows []PDFRow `yaml:"rows"`
}

// PDFRow represents a row in the PDF containing columns
type PDFRow struct {
	Cols   []PDFCol `yaml:"cols"`
	Height float64  `yaml:"height,omitempty"` // Row height in mm (default: 8)
}

// PDFCol represents a column in a row using a 12-unit grid system
type PDFCol struct {
	Size      int           `yaml:"size"`                // 1-12 grid units (must sum to 12 per row)
	Text      *PDFText      `yaml:"text,omitempty"`      // Text component
	Image     *PDFImage     `yaml:"image,omitempty"`     // Image component
	Table     *PDFTable     `yaml:"table,omitempty"`     // Table component
	Barcode   *PDFBarcode   `yaml:"barcode,omitempty"`   // Barcode component
	QRCode    *PDFQRCode    `yaml:"qrcode,omitempty"`    // QR code component
	Line      *PDFLine      `yaml:"line,omitempty"`      // Horizontal line component
	Signature *PDFSignature `yaml:"signature,omitempty"` // Signature placeholder component
}

// PDFText defines text content and styling
type PDFText struct {
	Content string        `yaml:"content"`         // Text content (supports $variable substitution)
	Style   *PDFTextStyle `yaml:"style,omitempty"` // Text styling
}

// PDFTextStyle defines styling options for text
type PDFTextStyle struct {
	Size            float64 `yaml:"size,omitempty"`            // Font size in points (default: 10)
	Bold            bool    `yaml:"bold,omitempty"`            // Bold text
	Italic          bool    `yaml:"italic,omitempty"`          // Italic text
	Underline       bool    `yaml:"underline,omitempty"`       // Underlined text
	Align           string  `yaml:"align,omitempty"`           // left, center, right (default: left)
	Color           string  `yaml:"color,omitempty"`           // Hex color like "#000000"
	BackgroundColor string  `yaml:"backgroundColor,omitempty"` // Hex color like "#FFFF00"
}

// PDFImage defines an image component
type PDFImage struct {
	URL    string  `yaml:"url"`              // Image URL (supports $variable substitution)
	Height float64 `yaml:"height,omitempty"` // Image height in mm
	Align  string  `yaml:"align,omitempty"`  // left, center, right
}

// PDFTable defines a table with headers and dynamic data
type PDFTable struct {
	Headers     []string         `yaml:"headers"`               // Column headers
	Data        string           `yaml:"data"`                  // Variable reference to data array (e.g., "$invoiceData.items")
	Columns     []PDFTableColumn `yaml:"columns"`               // Column definitions
	HeaderStyle *PDFTextStyle    `yaml:"headerStyle,omitempty"` // Header text styling
	ShowBorders bool             `yaml:"showBorders,omitempty"` // Show table borders
}

// PDFTableColumn defines a column in a table
type PDFTableColumn struct {
	Field  string `yaml:"field"`            // Field name to extract from data items
	Align  string `yaml:"align,omitempty"`  // Column alignment
	Format string `yaml:"format,omitempty"` // Value format: currency, date, number
}

// PDFBarcode defines a barcode component
type PDFBarcode struct {
	Value  string  `yaml:"value"`            // Barcode value (supports $variable substitution)
	Type   string  `yaml:"type,omitempty"`   // code128, code39, ean13 (default: code128)
	Height float64 `yaml:"height,omitempty"` // Barcode height in mm
}

// PDFQRCode defines a QR code component
type PDFQRCode struct {
	Value string  `yaml:"value"`           // QR code value (supports $variable substitution)
	Size  float64 `yaml:"size,omitempty"`  // QR code size in mm
	Align string  `yaml:"align,omitempty"` // left, center, right
}

// PDFLine defines a horizontal line component
type PDFLine struct {
	Thickness float64 `yaml:"thickness,omitempty"` // Line thickness in mm (default: 0.5)
	Color     string  `yaml:"color,omitempty"`     // Hex color
}

// PDFSignature defines a signature placeholder component
type PDFSignature struct {
	Label string  `yaml:"label,omitempty"` // Label above the signature line
	Width float64 `yaml:"width,omitempty"` // Signature line width in mm
}
type FunctionDescription struct {
	ToolName     string `json:"toolName"`
	FunctionName string `json:"functionName"`
	Description  string `json:"description"`
	Version      string `json:"version"`
}

// ToolExecutionState represents the state of a tool execution
type ToolExecutionState struct {
	ID                int64     `json:"id"`
	MessageID         string    `json:"message_id"`
	ClientID          string    `json:"client_id"` // Persistent conversation identifier
	FunctionName      string    `json:"function_name"`
	ToolName          string    `json:"tool_name"`
	StoppedAt         time.Time `json:"stopped_at"`
	InputName         string    `json:"input_name"`
	InputValue        string    `json:"input_value"`
	InputDescription  string    `json:"input_description"`
	Strategy          string    `json:"strategy"`
	Status            string    `json:"status"`
	ResponseMessageID string    `json:"response_message_id"`
	AttemptCount      int       `json:"attempt_count"` // Tracks how many times this input has been asked for
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

func (ToolExecutionState) TableName() string {
	return "tool_execution_state"
}

type MissingInputInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Message     string `json:"message"`
}

// WorkflowContextParam represents a function-specific parameter injection for initiate_workflow
// Used to pre-fill inputs of specific functions when the workflow is initiated
type WorkflowContextParam struct {
	Function string                 `yaml:"function"` // Function key: "toolName.functionName" or "functionName" (resolved at runtime)
	Inputs   map[string]interface{} `yaml:"inputs"`   // Input name -> value mapping
}

// WorkflowContextConfig represents the context configuration for initiate_workflow
// Supports both simple string format (backward compatible) and object format with params
type WorkflowContextConfig struct {
	Value  string                 `yaml:"value"`            // Context string for agent
	Params []WorkflowContextParam `yaml:"params,omitempty"` // Function-targeted params
}

// ParseWorkflowContext parses the context field which can be string or WorkflowContextConfig
func ParseWorkflowContext(contextRaw interface{}) (*WorkflowContextConfig, error) {
	if contextRaw == nil {
		return nil, fmt.Errorf("context is required")
	}

	// Handle string (backwards compatible)
	if str, ok := contextRaw.(string); ok {
		return &WorkflowContextConfig{Value: str}, nil
	}

	// Handle map (new object format)
	if contextMap, ok := contextRaw.(map[string]interface{}); ok {
		config := &WorkflowContextConfig{}

		// Parse value (required)
		if value, ok := contextMap["value"].(string); ok {
			config.Value = value
		} else {
			return nil, fmt.Errorf("context.value is required and must be a string")
		}

		// Parse params (optional)
		if params, ok := contextMap["params"]; ok && params != nil {
			paramsSlice, ok := params.([]interface{})
			if !ok {
				return nil, fmt.Errorf("context.params must be an array")
			}

			for i, paramRaw := range paramsSlice {
				paramMap, ok := paramRaw.(map[string]interface{})
				if !ok {
					// Try map[interface{}]interface{} from YAML parsing
					if paramMapIface, ok := paramRaw.(map[interface{}]interface{}); ok {
						paramMap = make(map[string]interface{})
						for k, v := range paramMapIface {
							if keyStr, ok := k.(string); ok {
								paramMap[keyStr] = v
							}
						}
					} else {
						return nil, fmt.Errorf("context.params[%d] must be an object", i)
					}
				}

				param := WorkflowContextParam{}

				// Parse function (required)
				if function, ok := paramMap["function"].(string); ok {
					param.Function = function
				} else {
					return nil, fmt.Errorf("context.params[%d].function is required and must be a string", i)
				}

				// Parse inputs (required)
				if inputs, ok := paramMap["inputs"].(map[string]interface{}); ok {
					param.Inputs = inputs
				} else if inputs, ok := paramMap["inputs"].(map[interface{}]interface{}); ok {
					param.Inputs = make(map[string]interface{})
					for k, v := range inputs {
						if keyStr, ok := k.(string); ok {
							param.Inputs[keyStr] = v
						}
					}
				} else {
					return nil, fmt.Errorf("context.params[%d].inputs is required and must be an object", i)
				}

				config.Params = append(config.Params, param)
			}
		}

		return config, nil
	}

	// Handle map[interface{}]interface{} from YAML parsing
	if contextMap, ok := contextRaw.(map[interface{}]interface{}); ok {
		stringMap := make(map[string]interface{})
		for k, v := range contextMap {
			if keyStr, ok := k.(string); ok {
				stringMap[keyStr] = v
			}
		}
		return ParseWorkflowContext(stringMap)
	}

	return nil, fmt.Errorf("context must be a string or an object with value and params fields")
}

// WorkflowYAML represents a workflow definition in the YAML file
type WorkflowYAML struct {
	CategoryName              string             `yaml:"category"`                // Required: snake_case category name
	HumanReadableCategoryName string             `yaml:"human_category"`          // Required: Title Case friendly name
	Description               string             `yaml:"description"`             // Required: Brief description of workflow purpose
	WorkflowType              string             `yaml:"workflow_type,omitempty"` // Optional: 'user' or 'team' (default: 'user')
	Steps                     []WorkflowStepYAML `yaml:"steps"`                   // Required: List of steps in the workflow
}

// WorkflowStepYAML represents a single step in a workflow
type WorkflowStepYAML struct {
	Order                      int    `yaml:"order"`                      // Required: Step order (starting from 0)
	Action                     string `yaml:"action"`                     // Required: Function name (PUBLIC function or system function)
	HumanReadableActionName    string `yaml:"human_action"`               // Required: Friendly action name
	Instructions               string `yaml:"instructions"`               // Required: When to use this step (rationale)
	ExpectedOutcomeDescription string `yaml:"expected_outcome,omitempty"` // Optional: Expected outcome
}
