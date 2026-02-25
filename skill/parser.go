package skill

import (
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/santhosh-tekuri/jsonschema"
	"gopkg.in/yaml.v2"
)

// ExternalFunctionRegistry allows registering external/shared function names
// that the parser should recognize during validation.
type ExternalFunctionRegistry struct {
	// Functions is the set of known external function names
	Functions map[string]bool
	// FunctionsWithParams is the subset that accepts parameters
	FunctionsWithParams map[string]bool
}

// defaultRegistry is used when no custom registry is provided
var defaultRegistry = &ExternalFunctionRegistry{
	Functions:           make(map[string]bool),
	FunctionsWithParams: make(map[string]bool),
}

// SetExternalFunctionRegistry sets the global external function registry.
// This should be called at init time by applications that provide shared tools.
func SetExternalFunctionRegistry(registry *ExternalFunctionRegistry) {
	if registry != nil {
		defaultRegistry = registry
	}
}

// GetExternalFunctionRegistry returns the current external function registry.
func GetExternalFunctionRegistry() *ExternalFunctionRegistry {
	return defaultRegistry
}

const (
	SystemTool = "systemTool"

	// Workflow type constants
	WorkflowTypeUser = "user"
	WorkflowTypeTeam = "team"
)

// ValidationResult contains the result of tool validation including warnings
type ValidationResult struct {
	Tool     CustomTool
	Warnings []string
}

func CreateTool(yamlToolDefinition string, args ...string) (CustomTool, error) {
	result, err := CreateToolWithWarnings(yamlToolDefinition, args...)
	return result.Tool, err
}

func CreateToolWithWarnings(yamlToolDefinition string, args ...string) (ValidationResult, error) {
	var (
		config       CustomTool
		isSystemTool bool
		isSharedTool bool
		warnings     []string
	)

	for _, arg := range args {
		if arg == SystemTool {
			isSystemTool = true
		}
		if arg == SharedTool {
			isSharedTool = true
		}
	}

	// Pre-validate to catch common mistakes with better error messages
	if err := preValidateInputCacheFormat(yamlToolDefinition); err != nil {
		return ValidationResult{}, err
	}

	err := yaml.Unmarshal([]byte(yamlToolDefinition), &config)
	if err != nil {
		return ValidationResult{}, fmt.Errorf("error parsing YAML: %w", err)
	}

	// Check for misplaced workflows inside tool definitions
	misplacedWarnings := checkForMisplacedWorkflows(yamlToolDefinition)
	warnings = append(warnings, misplacedWarnings...)

	configWarnings, err := validateConfig(&config, isSystemTool, isSharedTool)
	if err != nil {
		return ValidationResult{}, fmt.Errorf("validation error: %w", err)
	}
	warnings = append(warnings, configWarnings...)

	if isSystemTool {
		for i := range config.Tools {
			config.Tools[i].IsSystemApp = true
		}
	}

	if isSharedTool {
		for i := range config.Tools {
			config.Tools[i].IsSharedApp = true
		}
	}

	return ValidationResult{Tool: config, Warnings: warnings}, nil
}

func validateConfig(toolConfig *CustomTool, isSystemTool bool, isSharedTool bool) ([]string, error) {
	var allWarnings []string

	if err := validateGlobalSettings(toolConfig); err != nil {
		return nil, err
	}

	if err := validateEnvironmentVariables(toolConfig.Env); err != nil {
		return nil, err
	}

	warnings, err := validateTools(toolConfig.Tools, toolConfig.Env, isSystemTool, isSharedTool, toolConfig.Workflows)
	if err != nil {
		return nil, err
	}
	allWarnings = append(allWarnings, warnings...)

	if err := validateWorkflows(toolConfig.Workflows, toolConfig.Tools); err != nil {
		return nil, err
	}

	// Validate all init scripts per tool together (for non-system tools)
	if err := validateToolInitScripts(toolConfig.Tools, isSystemTool); err != nil {
		return nil, err
	}

	// Validate all migrations per tool together (for non-system tools)
	if err := validateToolMigrations(toolConfig.Tools, isSystemTool); err != nil {
		return nil, err
	}

	// Validate init + migrations work together in ephemeral DB (catches column conflicts, etc.)
	if err := validateInitAndMigrationsTogether(toolConfig.Tools, isSystemTool); err != nil {
		return nil, err
	}

	return allWarnings, nil
}

// validateGlobalSettings validates the top-level configuration settings
func validateGlobalSettings(config *CustomTool) error {
	if config.Version == "" {
		return errors.New("global 'version' is required. 'v1' is the only supported")
	}
	if config.Version != "v1" {
		return errors.New("'v1' is the only yaml definition version supported")
	}
	if config.Author == "" {
		return errors.New("global 'author' is required")
	}
	return nil
}

// validateEnvironmentVariables validates the environment variables
func validateEnvironmentVariables(envVars []EnvVar) error {
	envNames := make(map[string]bool)

	for i, env := range envVars {
		if env.Name == "" {
			return fmt.Errorf("env variable at index %d has an empty name", i)
		}

		if envNames[env.Name] {
			return fmt.Errorf("duplicate environment variable name '%s'", env.Name)
		}
		envNames[env.Name] = true

		// todo: create a method to create the tool. at this moment, we will prompt the dev if that env var
		// which has empty value should be prompted to the user or if the dev wants to set a default value like a secret
		if env.Value == "" && env.Description == "" {
			return fmt.Errorf("env variable '%s' has an empty value with an empty description. please provide at leat the description", env.Name)
		}

		if !isValidEnvVarName(env.Name) {
			return fmt.Errorf("env variable name '%s' is not a valid environment variable name", env.Name)
		}
	}
	return nil
}

// validateTools validates all tools in the configuration
func validateTools(tools []Tool, configEnvVars []EnvVar, isSystemTool bool, isSharedTool bool, rootWorkflows []WorkflowYAML) ([]string, error) {
	var allWarnings []string

	if len(tools) == 0 {
		return nil, errors.New("at least one tool must be defined")
	}

	// Check for unique tool names
	toolNames := make(map[string]bool)
	for _, tool := range tools {
		if toolNames[tool.Name] {
			return nil, fmt.Errorf("duplicate tool name '%s'", tool.Name)
		}
		toolNames[tool.Name] = true
	}

	// Validate each tool
	for _, tool := range tools {
		warnings, err := validateTool(tool, configEnvVars, isSystemTool, isSharedTool, rootWorkflows)
		if err != nil {
			return nil, err
		}
		allWarnings = append(allWarnings, warnings...)
	}

	return allWarnings, nil
}

// validateTool validates a single tool
func validateTool(tool Tool, configEnvVars []EnvVar, isSystemTool bool, isSharedTool bool, rootWorkflows []WorkflowYAML) ([]string, error) {
	var warnings []string

	if tool.Name == "" {
		return nil, errors.New("tool has an empty name")
	}

	if tool.Description == "" {
		return nil, fmt.Errorf("tool '%s' has an empty description", tool.Name)
	}

	if tool.Version == "" {
		return nil, fmt.Errorf("tool '%s' has an empty version", tool.Name)
	}

	// Validate semantic versioning
	if !isValidSemanticVersion(tool.Version) {
		return nil, fmt.Errorf("tool '%s' version '%s' is invalid; must match semantic versioning (e.g., 1.0.0)",
			tool.Name, tool.Version)
	}

	// Check for function name uniqueness
	funcNames := make(map[string]bool)
	for _, fn := range tool.Functions {
		if funcNames[fn.Name] {
			return nil, fmt.Errorf("duplicate function name '%s' in tool '%s'", fn.Name, tool.Name)
		}
		funcNames[fn.Name] = true
	}

	// Validate functions
	for _, function := range tool.Functions {
		if err := validateFunction(function, tool, configEnvVars, isSystemTool, rootWorkflows); err != nil {
			return nil, err
		}

		// Check for jq variable conflicts with input parameters (returns warnings)
		jqWarnings := ValidateFunctionJqVariableConflicts(function, tool.Name)
		warnings = append(warnings, jqWarnings...)
	}

	// Check waitFor/shouldSkip callback output types (returns warnings)
	callbackWarnings := validateWaitForAndShouldSkipOutputs(tool)
	warnings = append(warnings, callbackWarnings...)

	// Parse cache configurations on the actual slice (validateFunction works on copies)
	// This ensures ParsedCache is populated on the original functions for later warning checks
	for i := range tool.Functions {
		if err := parseFunctionCache(&tool.Functions[i], tool.Name); err != nil {
			return nil, err // Should not happen since validateFunction already validated
		}
	}

	// Parse reRunIf configurations on the actual slice
	// This ensures ParsedReRunIf is populated on the original functions for later execution
	for i := range tool.Functions {
		if err := parseFunctionReRunIf(&tool.Functions[i], tool.Name); err != nil {
			return nil, err // Should not happen since validateFunction already validated
		}
	}

	// Parse sanitize configurations on the actual slice and apply auto-sanitize rules
	for i := range tool.Functions {
		if err := parseFunctionSanitize(&tool.Functions[i], tool.Name); err != nil {
			return nil, err
		}
		// Parse input-level sanitize configs
		for j := range tool.Functions[i].Input {
			if err := parseInputSanitize(&tool.Functions[i].Input[j], tool.Functions[i].Name, tool.Name); err != nil {
				return nil, err
			}
		}
	}

	// Check for circular dependencies in functions
	if err := checkFunctionDependencyCycles(tool.Functions); err != nil {
		return nil, fmt.Errorf("in tool '%s': %w", tool.Name, err)
	}

	// Check for potential hook cycles (returns warnings, not errors)
	hookCycleWarnings := checkStaticHookCycles([]Tool{tool})
	warnings = append(warnings, hookCycleWarnings...)

	// Validate trigger reachability - functions without triggers must be reachable from triggered functions
	if err := validateTriggerReachability(tool.Functions, tool.Name); err != nil {
		return nil, err
	}

	// Validate orphan private functions - private functions must be reachable from entry points
	// Skip this check for system tools since their functions are designed to be called from external tools
	// via qualified names (e.g., team_analytics.registerHook in onSuccess/needs)
	// Skip for shared tools since their functions are designed to be called from other tools
	// via qualified names (e.g., utils_shared.addDaysToDate in needs/onSuccess)
	if !isSystemTool && !isSharedTool {
		if err := validateOrphanPrivateFunctions(tool.Functions, tool.Name); err != nil {
			return nil, err
		}
	}

	// Check that needs params referencing function inputs are provided by callers (non-fatal warnings)
	needsParamsWarnings := checkNeedsParamsFromCallers(tool.Functions)
	for _, w := range needsParamsWarnings {
		warnings = append(warnings, fmt.Sprintf("in tool '%s': %s", tool.Name, w))
	}

	// Check for input.from without corresponding needs (non-fatal warnings)
	inputFromWarnings := checkInputFromWithoutNeeds(tool.Functions)
	for _, w := range inputFromWarnings {
		warnings = append(warnings, fmt.Sprintf("in tool '%s': %s", tool.Name, w))
	}

	// Check for optional inputs without origin/value (caller injection pattern - INFO warning)
	callerInjectionWarnings := checkOptionalInputsWithoutOriginOrValue(tool.Functions)
	for _, w := range callerInjectionWarnings {
		warnings = append(warnings, fmt.Sprintf("in tool '%s': %s", tool.Name, w))
	}

	// Check for redundant dependencies (non-fatal warnings)
	redundancyWarnings := checkRedundantDependencies(tool.Functions)
	for _, w := range redundancyWarnings {
		warnings = append(warnings, fmt.Sprintf("in tool '%s': %s", tool.Name, w))
	}

	// Check for cache configuration warnings (non-fatal)
	cacheWarnings := checkCacheConfigWarnings(tool.Functions)
	for _, w := range cacheWarnings {
		warnings = append(warnings, fmt.Sprintf("in tool '%s': %s", tool.Name, w))
	}

	// Check for public functions with workflow variable references (non-fatal warning)
	publicFuncWarnings := checkPublicFunctionsWithWorkflowVars(tool.Functions)
	for _, w := range publicFuncWarnings {
		warnings = append(warnings, fmt.Sprintf("in tool '%s': %s", tool.Name, w))
	}

	// Check for expression syntax in non-evaluating fields (non-fatal warning)
	expressionWarnings := checkVariablesInsideNestedStrings(tool.Functions)
	for _, w := range expressionWarnings {
		warnings = append(warnings, fmt.Sprintf("in tool '%s': %s", tool.Name, w))
	}

	// Check for public functions with needs params referencing input variables (FATAL ERROR)
	// This pattern will fail at runtime because public functions are called directly
	// and cannot inject input values into their needs params
	if publicNeedsErrors := checkPublicFunctionsNeedsParamsFromInputs(tool.Functions, configEnvVars); len(publicNeedsErrors) > 0 {
		for _, err := range publicNeedsErrors {
			return nil, fmt.Errorf("in tool '%s': %w", tool.Name, err)
		}
	}

	// Check for input variables that reference non-accumulated inputs (FATAL ERROR)
	// Input values can only reference previous inputs, ENV vars, or system vars
	if inputRefErrors := checkInputVariableReferencesAccumulated(tool.Functions, configEnvVars); len(inputRefErrors) > 0 {
		for _, err := range inputRefErrors {
			return nil, fmt.Errorf("in tool '%s': %w", tool.Name, err)
		}
	}

	// Check for quoted variables in SQL queries (this is an error, not a warning)
	if err := validateNoQuotedSQLVariables(tool); err != nil {
		return nil, fmt.Errorf("in tool '%s': %w", tool.Name, err)
	}

	// Validate that functions with requiresInitiateWorkflow are referenced by initiate_workflow functions
	if err := validateRequiresInitiateWorkflowReferences(tool); err != nil {
		return nil, err
	}

	// Check for output references to step results that have runOnlyIf (non-fatal warning)
	outputRunOnlyIfWarnings := checkOutputReferencesToRunOnlyIfSteps(tool.Functions)
	for _, w := range outputRunOnlyIfWarnings {
		warnings = append(warnings, fmt.Sprintf("in tool '%s': %s", tool.Name, w))
	}

	// Validate KPI definitions if present
	if len(tool.Kpis) > 0 {
		if err := validateKPIDefinitions(tool.Kpis, tool.Name); err != nil {
			return nil, err
		}
	}

	return warnings, nil
}

// validateKPIDefinitions validates the KPI definitions declared in a tool.
func validateKPIDefinitions(kpis []KPIDefinition, toolName string) error {
	validAggregations := map[string]bool{
		"count":        true,
		"count_unique": true,
		"sum":          true,
		"avg":          true,
		"ratio":        true,
	}
	validValueTypes := map[string]bool{
		"number":     true,
		"percentage": true,
		"currency":   true,
		"duration":   true,
		"text":       true,
	}

	seenIDs := make(map[string]bool)

	for i, kpi := range kpis {
		// id is required
		if kpi.ID == "" {
			return fmt.Errorf("in tool '%s': kpis[%d] is missing required field 'id'", toolName, i)
		}

		// id must be unique within tool
		if seenIDs[kpi.ID] {
			return fmt.Errorf("in tool '%s': duplicate kpi id '%s'", toolName, kpi.ID)
		}
		seenIDs[kpi.ID] = true

		// label.En is required
		if kpi.Label.En == "" {
			return fmt.Errorf("in tool '%s': kpi '%s' is missing required field 'label' (at least English text is required)", toolName, kpi.ID)
		}

		// aggregation must be valid
		if !validAggregations[kpi.Aggregation] {
			return fmt.Errorf("in tool '%s': kpi '%s' has invalid aggregation '%s'; must be one of: count, count_unique, sum, avg, ratio", toolName, kpi.ID, kpi.Aggregation)
		}

		// event_type is required for non-ratio aggregations
		if kpi.Aggregation != "ratio" && kpi.EventType == "" {
			return fmt.Errorf("in tool '%s': kpi '%s' requires 'event_type' for aggregation '%s'", toolName, kpi.ID, kpi.Aggregation)
		}

		// ratio requires numerator + denominator
		if kpi.Aggregation == "ratio" {
			if kpi.Numerator == "" || kpi.Denominator == "" {
				return fmt.Errorf("in tool '%s': kpi '%s' with aggregation 'ratio' requires both 'numerator' and 'denominator'", toolName, kpi.ID)
			}
		}

		// value_type must be valid
		if !validValueTypes[kpi.ValueType] {
			return fmt.Errorf("in tool '%s': kpi '%s' has invalid value_type '%s'; must be one of: number, percentage, currency, duration, text", toolName, kpi.ID, kpi.ValueType)
		}

		// refresh must be a valid Go duration if set
		if kpi.Refresh != "" {
			if _, err := time.ParseDuration(kpi.Refresh); err != nil {
				return fmt.Errorf("in tool '%s': kpi '%s' has invalid refresh duration '%s': %w", toolName, kpi.ID, kpi.Refresh, err)
			}
		}

		// positive_direction must be valid if set
		if kpi.PositiveDirection != "" && kpi.PositiveDirection != "up" && kpi.PositiveDirection != "down" {
			return fmt.Errorf("in tool '%s': kpi '%s' has invalid positive_direction '%s'; must be 'up' or 'down'", toolName, kpi.ID, kpi.PositiveDirection)
		}

		// comparison must be valid if set
		validComparisons := map[string]bool{
			"previous_period": true,
			"previous_day":    true,
			"previous_week":   true,
			"previous_month":  true,
		}
		if kpi.Comparison != "" && !validComparisons[kpi.Comparison] {
			return fmt.Errorf("in tool '%s': kpi '%s' has invalid comparison '%s'; must be one of: previous_period, previous_day, previous_week, previous_month", toolName, kpi.ID, kpi.Comparison)
		}

		// category is required
		if kpi.Category == "" {
			return fmt.Errorf("in tool '%s': kpi '%s' is missing required field 'category'", toolName, kpi.ID)
		}
	}

	return nil
}

// validateFunction validates a single function within a tool
func validateFunction(function Function, tool Tool, configEnvVars []EnvVar, isSystemTool bool, rootWorkflows []WorkflowYAML) error {
	if function.Name == "" {
		return fmt.Errorf("function in tool '%s' has an empty name", tool.Name)
	}

	validOperations := map[string]bool{
		OperationWeb:              true,
		OperationAPI:              true,
		OperationDesktop:          false,
		OperationMCP:              true,
		OperationFormat:           true,
		OperationDB:               true,
		OperationTerminal:         true,
		OperationInitiateWorkflow: true,
		OperationPolicy:           true,
		OperationPDF:              true,
		OperationCode:             true,
		OperationGDrive:           true,
	}

	if !validOperations[function.Operation] {
		return fmt.Errorf("function '%s' in tool '%s' has invalid operation '%s'; must be one of web_browse, api_call, mcp, format, db, terminal, initiate_workflow, policy, pdf, code, and gdrive. desktop_use will be supported in the future",
			function.Name, tool.Name, function.Operation)
	}

	if function.Description == "" {
		return fmt.Errorf("function '%s' in tool '%s' has an empty description", function.Name, tool.Name)
	}

	if function.Operation == OperationWeb && GetSuccessCriteriaCondition(function.SuccessCriteria) == "" {
		return fmt.Errorf("function '%s' in tool '%s' with operation 'web_browse' must have a success criteria", function.Name, tool.Name)
	}

	if function.Operation == OperationFormat && FindLastInferenceInputName(function.Input) == "" {
		return fmt.Errorf("function '%s' in tool '%s' with operation 'format' must have an inference input", function.Name, tool.Name)
	}

	if function.Operation == OperationPolicy {
		if function.Output == nil || function.Output.Value == "" {
			return fmt.Errorf("function '%s' in tool '%s' with operation 'policy' must have output.value defined", function.Name, tool.Name)
		}
	}

	// Parse and validate cache configuration
	if err := parseFunctionCache(&function, tool.Name); err != nil {
		return err
	}

	// Parse reRunIf configuration
	if err := parseFunctionReRunIf(&function, tool.Name); err != nil {
		return err
	}

	if err := validateTriggers(function.Triggers, function.Name, tool.Name); err != nil {
		return err
	}

	if err := validateInputs(function.Input, function, tool, configEnvVars); err != nil {
		return err
	}

	if len(function.Needs) > 0 {
		if err := validateFunctionNeeds(function, tool, configEnvVars); err != nil {
			return fmt.Errorf("in function '%s' of tool '%s': %w", function.Name, tool.Name, err)
		}
	}

	if len(function.OnSuccess) > 0 {
		if err := validateFunctionOnSuccess(function, tool, configEnvVars); err != nil {
			return fmt.Errorf("in function '%s' of tool '%s': %w", function.Name, tool.Name, err)
		}
	}

	if len(function.OnFailure) > 0 {
		if err := validateFunctionOnFailure(function, tool, configEnvVars); err != nil {
			return fmt.Errorf("in function '%s' of tool '%s': %w", function.Name, tool.Name, err)
		}
	}

	if len(function.OnMissingUserInfo) > 0 {
		if err := validateFunctionOnMissingUserInfo(function, tool, configEnvVars); err != nil {
			return fmt.Errorf("in function '%s' of tool '%s': %w", function.Name, tool.Name, err)
		}
	}

	if len(function.OnUserConfirmationRequest) > 0 {
		if err := validateFunctionOnUserConfirmationRequest(function, tool, configEnvVars); err != nil {
			return fmt.Errorf("in function '%s' of tool '%s': %w", function.Name, tool.Name, err)
		}
	}

	if len(function.OnTeamApprovalRequest) > 0 {
		if err := validateFunctionOnTeamApprovalRequest(function, tool, configEnvVars); err != nil {
			return fmt.Errorf("in function '%s' of tool '%s': %w", function.Name, tool.Name, err)
		}
	}

	if len(function.OnSkip) > 0 {
		if err := validateFunctionOnSkip(function, tool, configEnvVars); err != nil {
			return fmt.Errorf("in function '%s' of tool '%s': %w", function.Name, tool.Name, err)
		}
	}

	// Validate runOnlyIf if present
	if function.RunOnlyIf != nil {
		if err := validateRunOnlyIf(function.RunOnlyIf, function, tool); err != nil {
			return fmt.Errorf("in function '%s' of tool '%s': %w", function.Name, tool.Name, err)
		}
	}

	// Validate reRunIf if present
	if function.ReRunIf != nil {
		if err := validateReRunIf(function.ReRunIf, function, tool); err != nil {
			return fmt.Errorf("in function '%s' of tool '%s': %w", function.Name, tool.Name, err)
		}
	}

	if function.CallRule != nil {
		if err := validateCallRule(function.CallRule, function.Name, tool.Name); err != nil {
			return fmt.Errorf("in function '%s' of tool '%s': %w", function.Name, tool.Name, err)
		}
	}

	// Validate RequiresUserConfirmation if present
	if function.RequiresUserConfirmation != nil {
		if err := validateRequiresUserConfirmation(function.RequiresUserConfirmation, function.Name, tool.Name); err != nil {
			return fmt.Errorf("in function '%s' of tool '%s': %w", function.Name, tool.Name, err)
		}
	}

	// Validate function-level shouldBeHandledAsMessageToUser conflicts with input-level
	if function.ShouldBeHandledAsMessageToUser {
		for _, input := range function.Input {
			if input.ShouldBeHandledAsMessageToUser {
				return fmt.Errorf("function '%s' in tool '%s' has shouldBeHandledAsMessageToUser at both function and input level; use only one",
					function.Name, tool.Name)
			}
		}
	}

	// Validate requiresInitiateWorkflow can only be used on public functions
	if function.RequiresInitiateWorkflow {
		if isPrivateFunction(function.Name) {
			return fmt.Errorf("function '%s' in tool '%s' has requiresInitiateWorkflow but is private (starts with lowercase); this field only applies to public functions",
				function.Name, tool.Name)
		}
	}

	if err := validateFunctionVariablesAndExpressions(function, tool.Name, configEnvVars); err != nil {
		return err
	}

	if function.Operation == OperationTerminal {
		if err := validateTerminalOperation(function, tool.Name); err != nil {
			return err
		}
	}

	if function.Operation == OperationMCP {
		if err := validateMCPOperation(function, tool.Name); err != nil {
			return err
		}
	}

	if function.Operation == OperationInitiateWorkflow {
		if err := validateInitiateWorkflowOperation(function, tool, rootWorkflows); err != nil {
			return err
		}
	}

	// Validate DB operation specific requirements
	if function.Operation == OperationDB {
		if err := validateDBOperation(function, tool.Name, isSystemTool); err != nil {
			return err
		}
	}

	// Validate PDF operation specific requirements
	if function.Operation == OperationPDF {
		if err := validatePDFOperation(function, tool.Name); err != nil {
			return err
		}
	}

	// Validate code operation specific requirements
	if function.Operation == OperationCode {
		if err := validateCodeOperation(function, tool.Name); err != nil {
			return err
		}
	}

	// Validate gdrive operation specific requirements
	if function.Operation == OperationGDrive {
		if err := validateGDriveOperation(function, tool.Name); err != nil {
			return err
		}
	}

	// Validate reuseSession is only used with code operations
	if function.ReuseSession && function.Operation != OperationCode {
		return fmt.Errorf("function '%s' in tool '%s' has reuseSession: true but operation '%s' is not 'code'; reuseSession is only supported for code operations",
			function.Name, tool.Name, function.Operation)
	}

	if err := ValidateSteps(function, tool, configEnvVars); err != nil {
		return err
	}

	if function.Output != nil {
		if err := validateOutput(function.Output, function, tool.Name, configEnvVars); err != nil {
			return err
		}
	}

	return nil
}

func validateFunctionNeeds(function Function, tool Tool, configEnvVars []EnvVar) error {
	availableFunctions := make(map[string]bool)
	for _, fn := range tool.Functions {
		availableFunctions[fn.Name] = true
	}

	queryAllowedFunctions := map[string]bool{
		"Ask":                true,
		"Learn":              true,
		"AskHuman":           true,
		"NotifyHuman":        true,
		"askToKnowledgeBase": true,
		"queryMemories":      true,
	}

	for _, need := range function.Needs {
		// Extract base function name (handle dot notation like "utils_shared.markConversationAsImportant")
		baseFuncName := extractBaseFunctionName(need.Name)

		isSysFunc := systemFunctions[need.Name] || systemFunctions[baseFuncName]
		isSharedFunc := defaultRegistry.Functions[need.Name] || defaultRegistry.Functions[baseFuncName]

		if !isSysFunc && !isSharedFunc && !availableFunctions[need.Name] {
			return fmt.Errorf("function '%s' in 'needs' is not available in the tool, as a system function, or as a shared function", need.Name)
		}

		if need.Name == function.Name {
			return fmt.Errorf("function cannot include itself in its 'needs' list")
		}

		if need.Query != "" {
			if !queryAllowedFunctions[need.Name] {
				return fmt.Errorf("function '%s' cannot have a query parameter in 'needs', only Ask, Learn, AskHuman, NotifyHuman, askToKnowledgeBase, and queryMemories support queries", need.Name)
			}

			if need.Name != "askToKnowledgeBase" && need.Name != "queryMemories" {
				return fmt.Errorf("currently only 'askToKnowledgeBase' and 'queryMemories' are allowed in 'needs' with a query parameter")
			}
		}

		if need.Name == "askToKnowledgeBase" && need.Query == "" {
			return fmt.Errorf("'askToKnowledgeBase' in 'needs' must have a query parameter")
		}

		if need.Name == "queryMemories" && need.Query == "" && len(need.Params) == 0 {
			return fmt.Errorf("'queryMemories' in 'needs' must have a query parameter or params with a query field")
		}

		// System functions in needs: allow askToKnowledgeBase/queryMemories (with query) and systemFunctionsWithParams (with params)
		if isSysFunc && need.Name != "askToKnowledgeBase" && need.Name != "queryMemories" && !systemFunctionsWithParams[need.Name] && !systemFunctionsWithParams[baseFuncName] {
			return fmt.Errorf("system function '%s' is not allowed in 'needs', only 'askToKnowledgeBase', 'queryMemories', and system functions with params support are allowed", need.Name)
		}

		// Shared functions in needs: only allow shared functions that accept params
		if isSharedFunc && len(need.Params) > 0 && !defaultRegistry.FunctionsWithParams[need.Name] && !defaultRegistry.FunctionsWithParams[baseFuncName] {
			return fmt.Errorf("shared function '%s' in 'needs' cannot have params", need.Name)
		}

		// Validate requiresUserConfirmation override if present
		if need.RequiresUserConfirmation != nil {
			if err := validateRequiresUserConfirmation(need.RequiresUserConfirmation, need.Name, tool.Name); err != nil {
				return fmt.Errorf("in needs function '%s': %w", need.Name, err)
			}
		}

		// Validate params if present
		if len(need.Params) > 0 {
			// askToKnowledgeBase uses query, not params
			if need.Name == "askToKnowledgeBase" {
				return fmt.Errorf("'askToKnowledgeBase' in 'needs' cannot have params, use 'query' instead")
			}

			// queryMemories: if it has a query field, it should not also have the query param
			if need.Name == "queryMemories" && need.Query != "" {
				return fmt.Errorf("'queryMemories' in 'needs' cannot have both 'query' and 'params', use one or the other")
			}

			// System functions with params: skip validation (they're loaded from system YAML at runtime)
			if isSysFunc && (systemFunctionsWithParams[need.Name] || systemFunctionsWithParams[baseFuncName]) {
				// Skip validation - the function is loaded from system YAML at runtime
				continue
			}

			// Shared functions with params: skip validation (they're loaded from embedded YAML at runtime)
			if isSharedFunc && (defaultRegistry.FunctionsWithParams[need.Name] || defaultRegistry.FunctionsWithParams[baseFuncName]) {
				// Skip validation - the function is loaded from shared YAML at runtime
				continue
			}

			// For non-system/non-shared functions, find the target function and validate params
			targetFunc := findFunctionByName(need.Name, tool.Functions)
			if targetFunc == nil {
				return fmt.Errorf("target function '%s' not found for params validation in 'needs'", need.Name)
			}

			// Convert NeedItem to FunctionCall for validation
			needCall := FunctionCall{Name: need.Name, Params: need.Params}
			if err := validateFunctionCallParams(needCall, *targetFunc, function, tool, configEnvVars, "needs"); err != nil {
				return err
			}
		}
	}

	return nil
}

// validateFunctionCallRunOnlyIf validates a runOnlyIf condition on a FunctionCall (onSuccess/onFailure)
// Only deterministic mode is supported for FunctionCall runOnlyIf.
func validateFunctionCallRunOnlyIf(runOnlyIf interface{}, sourceFunc Function, tool Tool, context string) error {
	if runOnlyIf == nil {
		return nil
	}

	// Parse the runOnlyIf
	runOnlyIfObj, err := ParseRunOnlyIf(runOnlyIf)
	if err != nil {
		return fmt.Errorf("invalid %s runOnlyIf: %w", context, err)
	}

	if runOnlyIfObj == nil {
		return nil
	}

	// For FunctionCall runOnlyIf, only deterministic mode is supported
	if runOnlyIfObj.Condition != "" {
		return fmt.Errorf("%s runOnlyIf only supports 'deterministic' mode, not 'condition' (inference)", context)
	}
	if runOnlyIfObj.Inference != nil {
		return fmt.Errorf("%s runOnlyIf only supports 'deterministic' mode, not 'inference'", context)
	}

	if runOnlyIfObj.Deterministic == "" {
		return fmt.Errorf("%s runOnlyIf must specify 'deterministic' expression", context)
	}

	// Validate that the deterministic expression only uses supported functions
	if err := ValidateDeterministicExpression(runOnlyIfObj.Deterministic); err != nil {
		return fmt.Errorf("%s %w", context, err)
	}

	// Validate that referenced functions exist in:
	// 1. Parent function inputs (accumulated values like $dealId)
	// 2. Parent function's needs (dependency results like $getDeal.result.stage)
	// 3. System variables ($NOW, $USER, etc.) - handled by ExtractFunctionReferences
	// 4. "result" - the parent function's output
	referencedFuncs := ExtractFunctionReferences(runOnlyIfObj.Deterministic)

	if len(referencedFuncs) > 0 {
		// Build set of available names
		availableNames := make(map[string]bool)

		// Add parent function's input names (they are available as $inputName)
		for _, input := range sourceFunc.Input {
			availableNames[input.Name] = true
		}

		// Add "result" as a valid reference (parent function's output)
		availableNames["result"] = true

		// Build transitive dependencies from parent function
		transitiveDeps := BuildTransitiveDependencies(tool.Functions)

		for _, funcName := range referencedFuncs {
			// Check if it's an input variable (which is valid)
			if availableNames[funcName] {
				continue
			}

			// Check if it's transitively reachable through needs
			if transitiveDeps[sourceFunc.Name] != nil && transitiveDeps[sourceFunc.Name][funcName] {
				continue
			}

			return fmt.Errorf("%s runOnlyIf references '%s' which is not available (valid: parent inputs, 'result', or dependencies from 'needs')", context, funcName)
		}
	}

	return nil
}

// validateFunctionOnSuccess validates the "onSuccess" field of a function
func validateFunctionOnSuccess(function Function, tool Tool, configEnvVars []EnvVar) error {
	availableFunctions := make(map[string]bool)
	for _, fn := range tool.Functions {
		availableFunctions[fn.Name] = true
	}

	// Track previous sibling function names for chaining support
	// e.g., if JoinMeetingBot runs before storeAppointmentInCache,
	// storeAppointmentInCache can reference $JoinMeetingBot.botId
	var previousSiblings []string

	// Check each onSuccess function
	for _, successCall := range function.OnSuccess {
		funcName := successCall.Name

		// Extract base function name (handle dot notation like "utils_shared.markConversationAsImportant")
		baseFuncName := extractBaseFunctionName(funcName)

		// Check if it's a system or shared function
		isSysFunc := systemFunctions[funcName] || systemFunctions[baseFuncName]
		isSharedFunc := defaultRegistry.Functions[funcName] || defaultRegistry.Functions[baseFuncName]

		// If it's not a system/shared function, check if it exists in the tool
		if !isSysFunc && !isSharedFunc && !availableFunctions[funcName] {
			return fmt.Errorf("function '%s' in 'onSuccess' is not available in the tool, as a system function, or as a shared function", funcName)
		}

		// Ensure the function doesn't call itself on success
		if funcName == function.Name {
			return fmt.Errorf("function cannot include itself in its 'onSuccess' list")
		}

		// Validate runOnlyIf if present
		if successCall.RunOnlyIf != nil {
			if err := validateFunctionCallRunOnlyIf(successCall.RunOnlyIf, function, tool, "onSuccess"); err != nil {
				return fmt.Errorf("in onSuccess function '%s': %w", funcName, err)
			}
		}

		// Validate requiresUserConfirmation override if present
		if successCall.RequiresUserConfirmation != nil {
			if err := validateRequiresUserConfirmation(successCall.RequiresUserConfirmation, funcName, tool.Name); err != nil {
				return fmt.Errorf("in onSuccess function '%s': %w", funcName, err)
			}
		}

		// Validate forEach if present
		if successCall.ForEach != nil {
			if err := validateCallbackForEach(successCall.ForEach, funcName, function.Name, tool.Name, "onSuccess"); err != nil {
				return fmt.Errorf("in onSuccess function '%s': %w", funcName, err)
			}
		}

		// Validate params if present
		if len(successCall.Params) > 0 {
			// Most system functions cannot have params, except those in systemFunctionsWithParams
			if isSysFunc && !systemFunctionsWithParams[funcName] && !systemFunctionsWithParams[baseFuncName] {
				return fmt.Errorf("system function '%s' in 'onSuccess' cannot have params", funcName)
			}

			// Shared functions with params: check if allowed
			if isSharedFunc && !defaultRegistry.FunctionsWithParams[funcName] && !defaultRegistry.FunctionsWithParams[baseFuncName] {
				return fmt.Errorf("shared function '%s' in 'onSuccess' cannot have params", funcName)
			}

			// For system functions with params, skip target function validation (they're loaded at runtime)
			if isSysFunc && (systemFunctionsWithParams[funcName] || systemFunctionsWithParams[baseFuncName]) {
				// Add this function to siblings for subsequent functions
				previousSiblings = append(previousSiblings, funcName)
				continue
			}

			// For shared functions with params, skip target function validation (they're loaded at runtime)
			if isSharedFunc && (defaultRegistry.FunctionsWithParams[funcName] || defaultRegistry.FunctionsWithParams[baseFuncName]) {
				// Add this function to siblings for subsequent functions
				previousSiblings = append(previousSiblings, funcName)
				continue
			}

			// Find the target function and validate params
			targetFunc := findFunctionByName(funcName, tool.Functions)
			if targetFunc == nil {
				return fmt.Errorf("target function '%s' not found for params validation in 'onSuccess'", funcName)
			}

			// Build extra sources: previous sibling function names + forEach loop variables if present
			extraSources := make([]string, 0, len(previousSiblings)+2)
			extraSources = append(extraSources, previousSiblings...)
			if successCall.ForEach != nil {
				// Add loop variables as valid sources
				itemVar := successCall.ForEach.ItemVar
				if itemVar == "" {
					itemVar = DefaultForEachItemVar
				}
				indexVar := successCall.ForEach.IndexVar
				if indexVar == "" {
					indexVar = DefaultForEachIndexVar
				}
				extraSources = append(extraSources, itemVar, indexVar)
			}

			// Pass extra sources for variable resolution
			if err := validateFunctionCallParams(successCall, *targetFunc, function, tool, configEnvVars, "onSuccess", extraSources...); err != nil {
				return err
			}
		}

		// Add this function to siblings for subsequent functions
		previousSiblings = append(previousSiblings, funcName)
	}

	return nil
}

// validateFunctionOnFailure validates the "onFailure" field of a function
func validateFunctionOnFailure(function Function, tool Tool, configEnvVars []EnvVar) error {
	availableFunctions := make(map[string]bool)
	for _, fn := range tool.Functions {
		availableFunctions[fn.Name] = true
	}

	// Check each onFailure function
	for _, failureCall := range function.OnFailure {
		funcName := failureCall.Name

		// Extract base function name (handle dot notation like "utils_shared.markConversationAsImportant")
		baseFuncName := extractBaseFunctionName(funcName)

		// Check if it's a system or shared function
		isSysFunc := systemFunctions[funcName] || systemFunctions[baseFuncName]
		isSharedFunc := defaultRegistry.Functions[funcName] || defaultRegistry.Functions[baseFuncName]

		// If it's not a system/shared function, check if it exists in the tool
		if !isSysFunc && !isSharedFunc && !availableFunctions[funcName] {
			return fmt.Errorf("function '%s' in 'onFailure' is not available in the tool, as a system function, or as a shared function", funcName)
		}

		// Ensure the function doesn't call itself on failure
		if funcName == function.Name {
			return fmt.Errorf("function cannot include itself in its 'onFailure' list")
		}

		// Validate runOnlyIf if present
		if failureCall.RunOnlyIf != nil {
			if err := validateFunctionCallRunOnlyIf(failureCall.RunOnlyIf, function, tool, "onFailure"); err != nil {
				return fmt.Errorf("in onFailure function '%s': %w", funcName, err)
			}
		}

		// Validate requiresUserConfirmation override if present
		if failureCall.RequiresUserConfirmation != nil {
			if err := validateRequiresUserConfirmation(failureCall.RequiresUserConfirmation, funcName, tool.Name); err != nil {
				return fmt.Errorf("in onFailure function '%s': %w", funcName, err)
			}
		}

		// Validate forEach if present
		if failureCall.ForEach != nil {
			if err := validateCallbackForEach(failureCall.ForEach, funcName, function.Name, tool.Name, "onFailure"); err != nil {
				return fmt.Errorf("in onFailure function '%s': %w", funcName, err)
			}
		}

		// Validate params if present
		if len(failureCall.Params) > 0 {
			// Most system functions cannot have params, except those in systemFunctionsWithParams
			if isSysFunc && !systemFunctionsWithParams[funcName] && !systemFunctionsWithParams[baseFuncName] {
				return fmt.Errorf("system function '%s' in 'onFailure' cannot have params", funcName)
			}

			// Shared functions with params: check if allowed
			if isSharedFunc && !defaultRegistry.FunctionsWithParams[funcName] && !defaultRegistry.FunctionsWithParams[baseFuncName] {
				return fmt.Errorf("shared function '%s' in 'onFailure' cannot have params", funcName)
			}

			// For system functions with params, skip target function validation (they're loaded at runtime)
			if isSysFunc && (systemFunctionsWithParams[funcName] || systemFunctionsWithParams[baseFuncName]) {
				// Skip validation - the function is loaded from system YAML at runtime
				continue
			}

			// For shared functions with params, skip target function validation (they're loaded at runtime)
			if isSharedFunc && (defaultRegistry.FunctionsWithParams[funcName] || defaultRegistry.FunctionsWithParams[baseFuncName]) {
				// Skip validation - the function is loaded from shared YAML at runtime
				continue
			}

			// Find the target function and validate params
			targetFunc := findFunctionByName(funcName, tool.Functions)
			if targetFunc == nil {
				return fmt.Errorf("target function '%s' not found for params validation in 'onFailure'", funcName)
			}

			// Build extra sources: forEach loop variables if present
			var extraSources []string
			if failureCall.ForEach != nil {
				itemVar := failureCall.ForEach.ItemVar
				if itemVar == "" {
					itemVar = DefaultForEachItemVar
				}
				indexVar := failureCall.ForEach.IndexVar
				if indexVar == "" {
					indexVar = DefaultForEachIndexVar
				}
				extraSources = []string{itemVar, indexVar}
			}

			if err := validateFunctionCallParams(failureCall, *targetFunc, function, tool, configEnvVars, "onFailure", extraSources...); err != nil {
				return err
			}
		}
	}

	return nil
}

// validateFunctionOnMissingUserInfo validates the "onMissingUserInfo" field of a function
func validateFunctionOnMissingUserInfo(function Function, tool Tool, configEnvVars []EnvVar) error {
	return validateCallbackFunctions(function.OnMissingUserInfo, function, tool, configEnvVars, "onMissingUserInfo")
}

// validateFunctionOnUserConfirmationRequest validates the "onUserConfirmationRequest" field of a function
func validateFunctionOnUserConfirmationRequest(function Function, tool Tool, configEnvVars []EnvVar) error {
	return validateCallbackFunctions(function.OnUserConfirmationRequest, function, tool, configEnvVars, "onUserConfirmationRequest")
}

// validateFunctionOnTeamApprovalRequest validates the "onTeamApprovalRequest" field of a function
func validateFunctionOnTeamApprovalRequest(function Function, tool Tool, configEnvVars []EnvVar) error {
	return validateCallbackFunctions(function.OnTeamApprovalRequest, function, tool, configEnvVars, "onTeamApprovalRequest")
}

// validateFunctionOnSkip validates the "onSkip" field of a function.
// onSkip callbacks are triggered when a function is skipped due to runOnlyIf evaluation.
// Unlike onFailure, onSkip supports sibling chaining (like onSuccess) - each callback can
// reference outputs from previously executed onSkip callbacks using $FuncName.field syntax.
func validateFunctionOnSkip(function Function, tool Tool, configEnvVars []EnvVar) error {
	availableFunctions := make(map[string]bool)
	for _, fn := range tool.Functions {
		availableFunctions[fn.Name] = true
	}

	// Track previous sibling function names for chaining support
	// e.g., if logSkippedPayment runs before notifySkipEvent,
	// notifySkipEvent can reference $logSkippedPayment.result
	var previousSiblings []string

	// Check each onSkip function
	for _, skipCall := range function.OnSkip {
		funcName := skipCall.Name

		// Extract base function name (handle dot notation like "utils_shared.markConversationAsImportant")
		baseFuncName := extractBaseFunctionName(funcName)

		// Check if it's a system or shared function
		isSysFunc := systemFunctions[funcName] || systemFunctions[baseFuncName]
		isSharedFunc := defaultRegistry.Functions[funcName] || defaultRegistry.Functions[baseFuncName]

		// If it's not a system/shared function, check if it exists in the tool
		if !isSysFunc && !isSharedFunc && !availableFunctions[funcName] {
			return fmt.Errorf("function '%s' in 'onSkip' is not available in the tool, as a system function, or as a shared function", funcName)
		}

		// Ensure the function doesn't call itself on skip
		if funcName == function.Name {
			return fmt.Errorf("function cannot include itself in its 'onSkip' list")
		}

		// Validate runOnlyIf if present
		if skipCall.RunOnlyIf != nil {
			if err := validateFunctionCallRunOnlyIf(skipCall.RunOnlyIf, function, tool, "onSkip"); err != nil {
				return fmt.Errorf("in onSkip function '%s': %w", funcName, err)
			}
		}

		// Validate requiresUserConfirmation override if present
		if skipCall.RequiresUserConfirmation != nil {
			if err := validateRequiresUserConfirmation(skipCall.RequiresUserConfirmation, funcName, tool.Name); err != nil {
				return fmt.Errorf("in onSkip function '%s': %w", funcName, err)
			}
		}

		// Validate params if present
		if len(skipCall.Params) > 0 {
			// Most system functions cannot have params, except those in systemFunctionsWithParams
			if isSysFunc && !systemFunctionsWithParams[funcName] && !systemFunctionsWithParams[baseFuncName] {
				return fmt.Errorf("system function '%s' in 'onSkip' cannot have params", funcName)
			}

			// Shared functions with params: check if allowed
			if isSharedFunc && !defaultRegistry.FunctionsWithParams[funcName] && !defaultRegistry.FunctionsWithParams[baseFuncName] {
				return fmt.Errorf("shared function '%s' in 'onSkip' cannot have params", funcName)
			}

			// For system functions with params, skip target function validation (they're loaded at runtime)
			if isSysFunc && (systemFunctionsWithParams[funcName] || systemFunctionsWithParams[baseFuncName]) {
				// Add this function to siblings for subsequent functions
				previousSiblings = append(previousSiblings, funcName)
				continue
			}

			// For shared functions with params, skip target function validation (they're loaded at runtime)
			if isSharedFunc && (defaultRegistry.FunctionsWithParams[funcName] || defaultRegistry.FunctionsWithParams[baseFuncName]) {
				// Add this function to siblings for subsequent functions
				previousSiblings = append(previousSiblings, funcName)
				continue
			}

			// Find the target function and validate params
			targetFunc := findFunctionByName(funcName, tool.Functions)
			if targetFunc == nil {
				return fmt.Errorf("target function '%s' not found for params validation in 'onSkip'", funcName)
			}

			// Pass previous sibling function names as extra sources for variable resolution
			// Include "skipReason" as a special variable available in onSkip callbacks
			extraSources := append([]string{"skipReason"}, previousSiblings...)
			if err := validateFunctionCallParams(skipCall, *targetFunc, function, tool, configEnvVars, "onSkip", extraSources...); err != nil {
				return err
			}
		}

		// Add this function to siblings for subsequent functions
		previousSiblings = append(previousSiblings, funcName)
	}

	return nil
}

// validateCallbackFunctions is a shared helper that validates a list of callback function calls.
// Used by onSuccess, onFailure, onMissingUserInfo, onUserConfirmationRequest, and onTeamApprovalRequest.
func validateCallbackFunctions(callbacks []FunctionCall, function Function, tool Tool, configEnvVars []EnvVar, callbackType string) error {
	availableFunctions := make(map[string]bool)
	for _, fn := range tool.Functions {
		availableFunctions[fn.Name] = true
	}

	for _, callbackCall := range callbacks {
		funcName := callbackCall.Name

		// Extract base function name for dot notation (e.g., "utils_shared.markConversationAsImportant" -> "markConversationAsImportant")
		baseFuncName := extractBaseFunctionName(funcName)

		// Check if it's a system function (using both full name and base name for dot notation support)
		isSysFunc := systemFunctions[funcName] || systemFunctions[baseFuncName]

		// Check if it's a shared function (using both full name and base name for dot notation support)
		isSharedFunc := defaultRegistry.Functions[funcName] || defaultRegistry.Functions[baseFuncName]

		// If it's not a system function, shared function, or available in tool, error out
		if !isSysFunc && !isSharedFunc && !availableFunctions[funcName] {
			return fmt.Errorf("function '%s' in '%s' is not available in the tool, as a system function, or as a shared function", funcName, callbackType)
		}

		// Ensure the function doesn't call itself
		if funcName == function.Name {
			return fmt.Errorf("function cannot include itself in its '%s' list", callbackType)
		}

		// Validate runOnlyIf if present
		if callbackCall.RunOnlyIf != nil {
			if err := validateFunctionCallRunOnlyIf(callbackCall.RunOnlyIf, function, tool, callbackType); err != nil {
				return fmt.Errorf("in %s function '%s': %w", callbackType, funcName, err)
			}
		}

		// Validate requiresUserConfirmation override if present
		if callbackCall.RequiresUserConfirmation != nil {
			if err := validateRequiresUserConfirmation(callbackCall.RequiresUserConfirmation, funcName, tool.Name); err != nil {
				return fmt.Errorf("in %s function '%s': %w", callbackType, funcName, err)
			}
		}

		// Validate params if present
		if len(callbackCall.Params) > 0 {
			// Most system functions cannot have params, except those in systemFunctionsWithParams
			if isSysFunc && !systemFunctionsWithParams[funcName] && !systemFunctionsWithParams[baseFuncName] {
				return fmt.Errorf("system function '%s' in '%s' cannot have params", funcName, callbackType)
			}

			// Shared functions with params: check if allowed
			if isSharedFunc && !defaultRegistry.FunctionsWithParams[funcName] && !defaultRegistry.FunctionsWithParams[baseFuncName] {
				return fmt.Errorf("shared function '%s' in '%s' cannot have params", funcName, callbackType)
			}

			// For system functions with params, skip target function validation (they're loaded at runtime)
			if isSysFunc && (systemFunctionsWithParams[funcName] || systemFunctionsWithParams[baseFuncName]) {
				// Skip validation - the function is loaded from system YAML at runtime
				continue
			}

			// For shared functions with params, skip target function validation (they're loaded at runtime)
			if isSharedFunc && (defaultRegistry.FunctionsWithParams[funcName] || defaultRegistry.FunctionsWithParams[baseFuncName]) {
				// Skip validation - the function is loaded from shared YAML at runtime
				continue
			}

			// Find the target function and validate params
			targetFunc := findFunctionByName(funcName, tool.Functions)
			if targetFunc == nil {
				return fmt.Errorf("target function '%s' not found for params validation in '%s'", funcName, callbackType)
			}

			if err := validateFunctionCallParams(callbackCall, *targetFunc, function, tool, configEnvVars, callbackType); err != nil {
				return err
			}
		}
	}

	return nil
}

// validateCallbackForEach validates forEach configuration for onSuccess/onFailure callbacks.
// Callback forEach is simpler than step forEach - it does NOT support breakIf, waitFor, or shouldSkip.
func validateCallbackForEach(forEach *CallbackForEach, callbackFuncName, parentFuncName, toolName, callbackType string) error {
	if forEach.Items == "" {
		return fmt.Errorf("forEach in %s callback '%s' for function '%s' of tool '%s' has empty items field",
			callbackType, callbackFuncName, parentFuncName, toolName)
	}

	// Set defaults
	if forEach.Separator == "" {
		forEach.Separator = DefaultForEachSeparator
	}

	if forEach.IndexVar == "" {
		forEach.IndexVar = DefaultForEachIndexVar
	}

	if forEach.ItemVar == "" {
		forEach.ItemVar = DefaultForEachItemVar
	}

	// Validate indexVar != itemVar
	if forEach.IndexVar == forEach.ItemVar {
		return fmt.Errorf("forEach in %s callback '%s' for function '%s' of tool '%s' has same indexVar and itemVar '%s'",
			callbackType, callbackFuncName, parentFuncName, toolName, forEach.IndexVar)
	}

	// Validate variable names match valid identifier pattern
	varRegex := regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)
	if !varRegex.MatchString(forEach.IndexVar) {
		return fmt.Errorf("forEach in %s callback '%s' for function '%s' of tool '%s' has invalid indexVar '%s'",
			callbackType, callbackFuncName, parentFuncName, toolName, forEach.IndexVar)
	}

	if !varRegex.MatchString(forEach.ItemVar) {
		return fmt.Errorf("forEach in %s callback '%s' for function '%s' of tool '%s' has invalid itemVar '%s'",
			callbackType, callbackFuncName, parentFuncName, toolName, forEach.ItemVar)
	}

	return nil
}

// findFunctionByName finds a function by name in the list of functions
func findFunctionByName(name string, functions []Function) *Function {
	for i := range functions {
		if functions[i].Name == name {
			return &functions[i]
		}
	}
	return nil
}

// extractBaseFunctionName extracts the function name from a potentially qualified name.
// For "utils_shared.markConversationAsImportant", returns "markConversationAsImportant".
// For "markConversationAsImportant", returns "markConversationAsImportant" (unchanged).
func extractBaseFunctionName(name string) string {
	parts := strings.Split(name, ".")
	if len(parts) > 1 {
		return parts[len(parts)-1]
	}
	return name
}

// isFunctionAvailable checks if a function name is available as a system function,
// shared function, or local tool function. Handles cross-tool references (e.g., utils_shared.funcName).
func isFunctionAvailable(name string, toolFunctions []Function) bool {
	baseFuncName := extractBaseFunctionName(name)

	// Check system functions
	if systemFunctions[name] || systemFunctions[baseFuncName] {
		return true
	}

	// Check shared functions
	if defaultRegistry.Functions[name] || defaultRegistry.Functions[baseFuncName] {
		return true
	}

	// Check local tool functions
	for _, fn := range toolFunctions {
		if fn.Name == name {
			return true
		}
	}

	return false
}

// / validateFunctionCallParams validates that parameters in a FunctionCall:
// - Reference inputs that exist in the target function
// - Have values that can be resolved (accumulated inputs, env vars, system vars, fixed values)
// extraSources allows adding additional valid sources (e.g., previous sibling onSuccess function names)
func validateFunctionCallParams(call FunctionCall, targetFunc Function, sourceFunc Function, tool Tool, configEnvVars []EnvVar, contextDescription string, extraSources ...string) error {
	if len(call.Params) == 0 {
		return nil
	}

	// Build map of target function's input names
	targetInputNames := make(map[string]bool)
	for _, input := range targetFunc.Input {
		targetInputNames[input.Name] = true
	}

	// Build set of available sources for parameter values
	availableSources := buildAvailableSources(sourceFunc, tool, configEnvVars)

	// Add extra sources (e.g., previous sibling onSuccess function names for chaining)
	for _, src := range extraSources {
		availableSources[src] = true
	}

	for paramName, paramValue := range call.Params {
		// Validate parameter name exists in target function inputs
		if !targetInputNames[paramName] {
			return fmt.Errorf("%s function '%s' has parameter '%s' but target function does not have an input with that name. Available inputs: %v",
				contextDescription, call.Name, paramName, getInputNames(targetFunc.Input))
		}

		// Validate parameter value can be resolved
		if err := validateParamValueResolvable(paramValue, availableSources, paramName, call.Name, contextDescription); err != nil {
			return err
		}
	}

	return nil
}

// buildAvailableSources builds the set of available variable sources
func buildAvailableSources(sourceFunc Function, tool Tool, configEnvVars []EnvVar) map[string]bool {
	sources := make(map[string]bool)

	// Source function's inputs (will be accumulated during execution)
	for _, input := range sourceFunc.Input {
		sources[input.Name] = true
	}

	// Env vars from config
	for _, envVar := range configEnvVars {
		sources[envVar.Name] = true
	}

	// System variables (available everywhere)
	systemVars := []string{
		"USER", "NOW", "MESSAGE", "ADMIN", "COMPANY", "UUID", "FILE", "ME",
	}
	for _, sv := range systemVars {
		sources[sv] = true
	}

	// Functions from needs (their results will be available)
	for _, need := range sourceFunc.Needs {
		sources[need.Name] = true
	}

	// "result" is available in onSuccess/onFailure callbacks (parent function's output)
	sources["result"] = true

	return sources
}

// validateParamValueResolvable checks if a parameter value can be resolved
// Supports: $varName, $varName.field, or fixed values
func validateParamValueResolvable(value interface{}, availableSources map[string]bool, paramName, functionName, contextDescription string) error {
	strValue, ok := value.(string)
	if !ok {
		// Non-string values (numbers, bools) are fixed values - always valid
		return nil
	}

	// Check for variable references starting with $
	varPattern := regexp.MustCompile(`\$([a-zA-Z_][a-zA-Z0-9_]*)`)
	matches := varPattern.FindAllStringSubmatch(strValue, -1)

	if len(matches) == 0 {
		// Fixed string value (no $ references) - always valid
		return nil
	}

	for _, match := range matches {
		varRef := match[1] // The variable name without $
		// For $varName.field, we only need to check the base variable name
		baseVar := strings.Split(varRef, ".")[0]

		if !availableSources[baseVar] {
			return fmt.Errorf("%s function '%s' parameter '%s' references '$%s' which is not available. Available sources: function inputs, env vars, system vars ($USER, $NOW, etc.), function results from needs, or use fixed values",
				contextDescription, functionName, paramName, varRef)
		}
	}

	// Validate expression functions in parameter value
	if err := ValidateExpressionFunctions(strValue); err != nil {
		return fmt.Errorf("%s function '%s' parameter '%s': %w",
			contextDescription, functionName, paramName, err)
	}

	return nil
}

// getInputNames returns a slice of input names for error messages
func getInputNames(inputs []Input) []string {
	names := make([]string, len(inputs))
	for i, input := range inputs {
		names[i] = input.Name
	}
	return names
}

// BuildTransitiveDependencies computes the transitive closure of function dependencies
// using the Floyd-Warshall algorithm. Returns a map where dependencies[functionA][functionB] = true
// means functionB is transitively reachable from functionA through the dependency chain.
// Also includes all system functions as reachable from every function.
func BuildTransitiveDependencies(functions []Function) map[string]map[string]bool {
	dependencies := make(map[string]map[string]bool)

	for _, fn := range functions {
		dependencies[fn.Name] = make(map[string]bool)
		for _, need := range fn.Needs {
			dependencies[fn.Name][need.Name] = true
		}
	}

	for _, fn := range functions {
		for sysFunc := range systemFunctions {
			dependencies[fn.Name][sysFunc] = true
		}
	}

	for k := range dependencies {
		for i := range dependencies {
			for j := range dependencies {
				if dependencies[i][k] && dependencies[k][j] {
					dependencies[i][j] = true
				}
			}
		}
	}

	return dependencies
}

// buildTransitiveDependencies is deprecated, use BuildTransitiveDependencies instead
func buildTransitiveDependencies(functions []Function) map[string]map[string]bool {
	return BuildTransitiveDependencies(functions)
}

// RedundantDependency represents a redundant dependency that can be optimized
type RedundantDependency struct {
	FunctionName string
	RedundantDep string
	CoveringDep  string
}

// detectRedundantDependencies finds redundant direct dependencies that are already
// covered transitively through other direct dependencies.
// Returns a slice of RedundantDependency structs.
func detectRedundantDependencies(functions []Function) []RedundantDependency {
	var redundancies []RedundantDependency

	// Build a map of direct dependencies for each function (excluding system functions)
	directDeps := make(map[string]map[string]bool)
	for _, fn := range functions {
		directDeps[fn.Name] = make(map[string]bool)
		for _, need := range fn.Needs {
			// Only track user-defined functions, not system functions
			if !systemFunctions[need.Name] {
				directDeps[fn.Name][need.Name] = true
			}
		}
	}

	// For each function, check if any direct dependency is also reachable
	// transitively through another direct dependency
	for _, fn := range functions {
		direct := directDeps[fn.Name]
		if len(direct) <= 1 {
			// Can't have redundancy with 0 or 1 dependencies
			continue
		}

		// For each direct dependency, check if it's also reachable through another path
		for depName := range direct {
			// Check if depName is transitively reachable through any other direct dependency
			for otherDepName := range direct {
				if depName == otherDepName {
					continue
				}

				// Check if otherDepName transitively depends on depName
				if isTransitivelyReachable(otherDepName, depName, directDeps, make(map[string]bool)) {
					// depName is redundant because it's already covered by otherDepName
					redundancies = append(redundancies, RedundantDependency{
						FunctionName: fn.Name,
						RedundantDep: depName,
						CoveringDep:  otherDepName,
					})
					break // Only report each redundancy once
				}
			}
		}
	}

	return redundancies
}

// isTransitivelyReachable checks if 'target' is reachable from 'source' through dependencies
func isTransitivelyReachable(source, target string, deps map[string]map[string]bool, visited map[string]bool) bool {
	if source == target {
		return true
	}

	if visited[source] {
		return false
	}
	visited[source] = true

	// Check direct dependencies of source
	for dep := range deps[source] {
		if dep == target {
			return true
		}
		if isTransitivelyReachable(dep, target, deps, visited) {
			return true
		}
	}

	return false
}

// checkRedundantDependencies validates that functions don't have redundant dependencies
// and returns warnings if found (does not return errors, as redundancies are non-fatal)
func checkRedundantDependencies(functions []Function) []string {
	redundancies := detectRedundantDependencies(functions)

	if len(redundancies) == 0 {
		return nil
	}

	// Build warning messages
	var warnings []string
	for _, r := range redundancies {
		warnings = append(warnings, fmt.Sprintf(
			"Function '%s' has redundant dependency '%s' (already covered transitively by '%s'). Consider removing '%s' from the needs list for better efficiency.",
			r.FunctionName, r.RedundantDep, r.CoveringDep, r.RedundantDep))
	}

	return warnings
}

// checkNeedsParamsFromCallers validates that when a function's needs block uses params
// referencing its own inputs (e.g., $orgId), callers of that function provide those params.
// Returns warnings (non-fatal) for any missing param injections.
func checkNeedsParamsFromCallers(functions []Function) []string {
	var warnings []string

	// Build a map of function name -> inputs that are used in needs params
	// These are inputs that MUST be provided by callers via params
	funcRequiredParams := make(map[string]map[string]bool) // funcName -> set of required param names

	// First pass: identify functions that use $inputName in their needs params
	for _, fn := range functions {
		// Build set of this function's input names
		inputNames := make(map[string]bool)
		for _, input := range fn.Input {
			inputNames[input.Name] = true
		}

		// Check needs params for references to function inputs
		for _, need := range fn.Needs {
			for _, paramValue := range need.Params {
				strValue, ok := paramValue.(string)
				if !ok {
					continue
				}

				// Extract variable references from the param value
				varPattern := regexp.MustCompile(`\$([a-zA-Z_][a-zA-Z0-9_]*)`)
				matches := varPattern.FindAllStringSubmatch(strValue, -1)

				for _, match := range matches {
					varName := match[1]
					// Check if this variable references one of the function's own inputs
					if inputNames[varName] {
						// This function requires 'varName' to be passed by callers
						if funcRequiredParams[fn.Name] == nil {
							funcRequiredParams[fn.Name] = make(map[string]bool)
						}
						funcRequiredParams[fn.Name][varName] = true
					}
				}
			}
		}
	}

	// Second pass: check that all callers of these functions provide the required params
	for _, fn := range functions {
		// Check onSuccess calls
		for _, call := range fn.OnSuccess {
			if requiredParams, hasRequired := funcRequiredParams[call.Name]; hasRequired {
				for paramName := range requiredParams {
					if !callProvidesParam(call.Params, paramName) {
						warnings = append(warnings, fmt.Sprintf(
							"function '%s' calls '%s' in onSuccess but doesn't provide param '%s' which is required by '%s' for its needs block. Add 'params: { %s: \"$%s\" }' to the onSuccess call.",
							fn.Name, call.Name, paramName, call.Name, paramName, paramName))
					}
				}
			}
		}

		// Check onFailure calls
		for _, call := range fn.OnFailure {
			if requiredParams, hasRequired := funcRequiredParams[call.Name]; hasRequired {
				for paramName := range requiredParams {
					if !callProvidesParam(call.Params, paramName) {
						warnings = append(warnings, fmt.Sprintf(
							"function '%s' calls '%s' in onFailure but doesn't provide param '%s' which is required by '%s' for its needs block. Add 'params: { %s: \"$%s\" }' to the onFailure call.",
							fn.Name, call.Name, paramName, call.Name, paramName, paramName))
					}
				}
			}
		}

		// Check needs calls (needs can also call functions that have their own needs with params)
		for _, need := range fn.Needs {
			if requiredParams, hasRequired := funcRequiredParams[need.Name]; hasRequired {
				for paramName := range requiredParams {
					if !callProvidesParam(need.Params, paramName) {
						warnings = append(warnings, fmt.Sprintf(
							"function '%s' calls '%s' in needs but doesn't provide param '%s' which is required by '%s' for its needs block. Add 'params: { %s: \"$%s\" }' to the needs entry.",
							fn.Name, need.Name, paramName, need.Name, paramName, paramName))
					}
				}
			}
		}
	}

	return warnings
}

// callProvidesParam checks if a params map includes a specific param name
func callProvidesParam(params map[string]interface{}, paramName string) bool {
	if params == nil {
		return false
	}
	_, exists := params[paramName]
	return exists
}

// checkInputFromWithoutNeeds warns when a function has input.from referencing a function
// that is not in its needs block. This could lead to unexpected behavior depending on
// whether the function was already executed (possibly with different params) or not.
func checkInputFromWithoutNeeds(functions []Function) []string {
	var warnings []string

	for _, fn := range functions {
		// Build set of functions in this function's needs
		needsSet := make(map[string]bool)
		for _, need := range fn.Needs {
			needsSet[need.Name] = true
		}

		// Check each input with origin "function"
		for _, input := range fn.Input {
			if input.Origin != DataOriginFunction || input.From == "" {
				continue
			}

			// Extract the function name from the "from" field (handle dot notation like "funcName.field")
			fromFuncName := ExtractFunctionNameFromFrom(input.From)

			// Skip system functions - they're handled specially
			if systemFunctions[fromFuncName] {
				continue
			}

			// Check if this function is in the needs block
			if !needsSet[fromFuncName] {
				warnings = append(warnings, fmt.Sprintf(
					"function '%s' has input '%s' with from='%s' but '%s' is not in its needs block. "+
						"If '%s' was already executed elsewhere (possibly with different params), this input will use that result. "+
						"If not executed yet, '%s' will run with its own input definitions. "+
						"Consider adding '%s' to needs for explicit control over execution.",
					fn.Name, input.Name, input.From, fromFuncName, fromFuncName, fromFuncName, fromFuncName))
			}
		}
	}

	return warnings
}

// checkOptionalInputsWithoutOriginOrValue checks for optional inputs that have no origin AND no value.
// This is the "caller injection" pattern where values are expected to be injected via params
// from onSuccess/onFailure/needs callers. If not injected, the input is treated as NULL (DB ops)
// or empty string (terminal ops). Returns warnings to inform users about this pattern.
func checkOptionalInputsWithoutOriginOrValue(functions []Function) []string {
	var warnings []string

	for _, fn := range functions {
		for _, input := range fn.Input {
			// Check for caller injection pattern: optional, no origin, no value
			if input.IsOptional && input.Origin == "" && input.Value == "" {
				warnings = append(warnings, fmt.Sprintf(
					"function '%s' has optional input '%s' with no origin and no value (caller injection pattern). "+
						"This input expects values to be injected via params from onSuccess/onFailure/needs callers. "+
						"If not injected, it will be NULL for DB operations or empty for other operations.",
					fn.Name, input.Name))
			}
		}
	}

	return warnings
}

// validateRunOnlyIf validates the "runOnlyIf" field of a function
func validateRunOnlyIf(runOnlyIf interface{}, function Function, tool Tool) error {
	// Parse the runOnlyIf to get the object
	runOnlyIfObj, err := ParseRunOnlyIf(runOnlyIf)
	if err != nil {
		return fmt.Errorf("invalid runOnlyIf: %w", err)
	}

	if runOnlyIfObj == nil {
		return nil // No runOnlyIf specified
	}

	// Check that at least one condition type is specified (already validated in ParseRunOnlyIf, but double-check)
	hasCondition := runOnlyIfObj.Condition != ""
	hasDeterministic := runOnlyIfObj.Deterministic != ""
	hasInference := runOnlyIfObj.Inference != nil

	if !hasCondition && !hasDeterministic && !hasInference {
		return fmt.Errorf("runOnlyIf must specify at least one of: condition, deterministic, or inference")
	}

	// Validate the From field if present
	if len(runOnlyIfObj.From) > 0 {
		// Build transitive dependency graph
		transitiveDeps := buildTransitiveDependencies(tool.Functions)

		// Check each function in the From list
		for _, fromFunc := range runOnlyIfObj.From {
			// Special case: "scratchpad" is always valid - it's a system-provided filter
			// for accessing accumulated workflow context (extraInfoRequested + done rationale)
			if fromFunc == FilterScratchpad {
				continue
			}

			// Check for self-reference
			if fromFunc == function.Name {
				return fmt.Errorf("function cannot reference itself in runOnlyIf.from")
			}

			// Check if fromFunc is transitively reachable from current function
			if !transitiveDeps[function.Name][fromFunc] {
				return fmt.Errorf("function '%s' in runOnlyIf.from is not transitively reachable through the dependency chain of function '%s'", fromFunc, function.Name)
			}
		}
	}

	// Validate allowedSystemFunctions if present
	if len(runOnlyIfObj.AllowedSystemFunctions) > 0 {
		validSystemFunctions := map[string]bool{
			SystemFunctionAskConversationHistory:    true,
			SystemFunctionAskKnowledgeBase:          true,
			SystemFunctionAskToContext:              true,
			SystemFunctionDoDeepWebResearch:         true,
			SystemFunctionDoSimpleWebSearch:         true,
			SystemFunctionGetWeekdayFromDate:        true,
			SystemFunctionQueryMemories:             true,
			SystemFunctionFetchWebContent:           true,
			SystemFunctionQueryCustomerServiceChats: true,
			SystemFunctionAnalyzeImage:              true,
			SystemFunctionSearchCodebase:            true,
			SystemFunctionQueryDocuments:            true,
		}

		for _, funcName := range runOnlyIfObj.AllowedSystemFunctions {
			if !validSystemFunctions[funcName] {
				return fmt.Errorf("invalid system function '%s' in runOnlyIf.allowedSystemFunctions. Valid options are: %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s",
					funcName,
					SystemFunctionAskConversationHistory,
					SystemFunctionAskKnowledgeBase,
					SystemFunctionAskToContext,
					SystemFunctionDoDeepWebResearch,
					SystemFunctionDoSimpleWebSearch,
					SystemFunctionGetWeekdayFromDate,
					SystemFunctionQueryMemories,
					SystemFunctionFetchWebContent,
					SystemFunctionQueryCustomerServiceChats,
					SystemFunctionAnalyzeImage,
					SystemFunctionSearchCodebase,
					SystemFunctionQueryDocuments)
			}
		}

		// Validate queryCustomerServiceChats requires clientIds
		hasQueryCustomerServiceChats := false
		for _, funcName := range runOnlyIfObj.AllowedSystemFunctions {
			if funcName == SystemFunctionQueryCustomerServiceChats {
				hasQueryCustomerServiceChats = true
				break
			}
		}
		if hasQueryCustomerServiceChats && runOnlyIfObj.ClientIds == "" {
			return fmt.Errorf("allowedSystemFunctions includes '%s' but 'clientIds' field is not specified. "+
				"You must provide a clientIds variable reference (e.g., clientIds: \"$targetClientIds\")",
				SystemFunctionQueryCustomerServiceChats)
		}

		// Validate searchCodebase requires codebaseDirs
		hasSearchCodebase := false
		for _, funcName := range runOnlyIfObj.AllowedSystemFunctions {
			if funcName == SystemFunctionSearchCodebase {
				hasSearchCodebase = true
				break
			}
		}
		if hasSearchCodebase && runOnlyIfObj.CodebaseDirs == "" {
			return fmt.Errorf("allowedSystemFunctions includes '%s' but 'codebaseDirs' field is not specified. "+
				"You must provide a codebaseDirs variable reference (e.g., codebaseDirs: \"$getProjectRepos\")",
				SystemFunctionSearchCodebase)
		}

		// If 'from' is specified AND allowedSystemFunctions is restricted,
		// then askToContext MUST be in the allowed list because it's required to query function outputs
		if len(runOnlyIfObj.From) > 0 {
			hasAskToContext := false
			for _, funcName := range runOnlyIfObj.AllowedSystemFunctions {
				if funcName == SystemFunctionAskToContext {
					hasAskToContext = true
					break
				}
			}
			if !hasAskToContext {
				return fmt.Errorf("runOnlyIf.allowedSystemFunctions must include '%s' when 'from' field is specified, as it is required to query the context of other functions", SystemFunctionAskToContext)
			}
		}
	}

	// Validate queryCustomerServiceChats in inference object requires clientIds
	// This is separate from the above block because when using advanced inference mode,
	// allowedSystemFunctions is on the inference object, not the runOnlyIfObj directly
	if runOnlyIfObj.Inference != nil && len(runOnlyIfObj.Inference.AllowedSystemFunctions) > 0 {
		hasQueryCustomerServiceChatsInInference := false
		for _, funcName := range runOnlyIfObj.Inference.AllowedSystemFunctions {
			if funcName == SystemFunctionQueryCustomerServiceChats {
				hasQueryCustomerServiceChatsInInference = true
				break
			}
		}
		if hasQueryCustomerServiceChatsInInference && runOnlyIfObj.Inference.ClientIds == "" {
			return fmt.Errorf("runOnlyIf.inference.allowedSystemFunctions includes '%s' but 'clientIds' field is not specified. "+
				"You must provide a clientIds variable reference (e.g., clientIds: \"$targetClientIds\")",
				SystemFunctionQueryCustomerServiceChats)
		}
	}

	// Validate deterministic expression references at parse time for better error messages
	// Note: VariableReplacer will perform the actual replacement at runtime
	if runOnlyIfObj.Deterministic != "" {
		// Validate that the deterministic expression only uses supported functions
		if err := ValidateDeterministicExpression(runOnlyIfObj.Deterministic); err != nil {
			return err
		}

		// Extract all function references from the deterministic expression
		referencedFuncs := ExtractFunctionReferences(runOnlyIfObj.Deterministic)

		if len(referencedFuncs) > 0 {
			// Build transitive dependency map to check if referenced functions are reachable
			transitiveDeps := buildTransitiveDependencies(tool.Functions)

			// Check each referenced function is transitively reachable
			var missingFuncs []string
			for _, funcName := range referencedFuncs {
				// Check if funcName is transitively reachable from current function
				if !transitiveDeps[function.Name][funcName] {
					missingFuncs = append(missingFuncs, funcName)
				}
			}

			if len(missingFuncs) > 0 {
				return fmt.Errorf("runOnlyIf.deterministic expression references functions not transitively reachable through 'needs': %v. All referenced functions must be in the dependency chain",
					missingFuncs)
			}
		}
	}

	// Validate combineWith mode if present
	if runOnlyIfObj.CombineMode != "" {
		if runOnlyIfObj.CombineMode != CombineModeAND && runOnlyIfObj.CombineMode != CombineModeOR {
			return fmt.Errorf("invalid runOnlyIf.combineWith value '%s'. Valid options are: '%s' or '%s'",
				runOnlyIfObj.CombineMode, CombineModeAND, CombineModeOR)
		}

		// CombineMode only makes sense when both deterministic and inference are present
		hasDeterministic := runOnlyIfObj.Deterministic != ""
		hasInference := runOnlyIfObj.Condition != "" || runOnlyIfObj.Inference != nil

		if !hasDeterministic || !hasInference {
			return fmt.Errorf("runOnlyIf.combineWith can only be used when both 'deterministic' and 'inference' (or 'condition') are specified")
		}
	}

	// Validate onError if present
	if runOnlyIfObj.OnError != nil {
		if err := validateOnErrorStrategy(runOnlyIfObj.OnError); err != nil {
			return fmt.Errorf("invalid runOnlyIf.onError: %w", err)
		}
	}

	return nil
}

// validateReRunIf validates the "reRunIf" field of a function
func validateReRunIf(reRunIf interface{}, function Function, tool Tool) error {
	// Parse the reRunIf to get the config
	config, err := ParseReRunIf(reRunIf)
	if err != nil {
		return fmt.Errorf("invalid reRunIf: %w", err)
	}

	if config == nil {
		return nil // No reRunIf specified
	}

	// Check that at least one condition type is specified
	hasCondition := config.Condition != ""
	hasDeterministic := config.Deterministic != ""
	hasInference := config.Inference != nil
	hasCall := config.Call != nil

	if !hasCondition && !hasDeterministic && !hasInference && !hasCall {
		return fmt.Errorf("reRunIf must specify at least one of: deterministic, condition, inference, or call")
	}

	// Validate maxRetries
	if config.MaxRetries < 0 {
		return fmt.Errorf("reRunIf.maxRetries must be a positive integer, got %d", config.MaxRetries)
	}
	if config.MaxRetries > 10000 {
		// This is a warning-level issue, but we'll allow it with a log
		// Note: The actual warning would be logged elsewhere
	}

	// Validate delayMs
	if config.DelayMs < 0 {
		return fmt.Errorf("reRunIf.delayMs must be a non-negative integer, got %d", config.DelayMs)
	}

	// Validate scope
	if config.Scope != "" && config.Scope != ReRunScopeSteps && config.Scope != ReRunScopeFull {
		return fmt.Errorf("reRunIf.scope must be '%s' or '%s', got '%s'", ReRunScopeSteps, ReRunScopeFull, config.Scope)
	}

	// Validate combineWith
	if config.CombineMode != "" {
		normalizedMode := strings.ToUpper(config.CombineMode)
		if normalizedMode != CombineModeAND && normalizedMode != CombineModeOR {
			return fmt.Errorf("reRunIf.combineWith must be 'and' or 'or', got '%s'", config.CombineMode)
		}
	}

	// Validate call function if present
	if config.Call != nil {
		if config.Call.Name == "" {
			return fmt.Errorf("reRunIf.call.name is required")
		}

		// Check if the called function exists in the tool
		availableFunctions := make(map[string]bool)
		for _, fn := range tool.Functions {
			availableFunctions[fn.Name] = true
		}

		// Extract base function name (handle dot notation)
		baseFuncName := extractBaseFunctionName(config.Call.Name)

		// Check if it's a system function, shared function, or local function
		isSysFunc := systemFunctions[config.Call.Name] || systemFunctions[baseFuncName]
		isSharedFunc := defaultRegistry.Functions[config.Call.Name] || defaultRegistry.Functions[baseFuncName]

		if !isSysFunc && !isSharedFunc && !availableFunctions[config.Call.Name] {
			return fmt.Errorf("reRunIf.call.name '%s' is not available in the tool, as a system function, or as a shared function", config.Call.Name)
		}

		// Function cannot call itself
		if config.Call.Name == function.Name {
			return fmt.Errorf("function cannot call itself in reRunIf.call")
		}
	}

	// Validate the From field if present
	if len(config.From) > 0 {
		// Build transitive dependency graph
		transitiveDeps := buildTransitiveDependencies(tool.Functions)

		// Check each function in the From list
		for _, fromFunc := range config.From {
			// Special case: "scratchpad" is always valid
			if fromFunc == FilterScratchpad {
				continue
			}

			// Check for self-reference
			if fromFunc == function.Name {
				return fmt.Errorf("function cannot reference itself in reRunIf.from")
			}

			// Check if fromFunc is transitively reachable from current function
			if !transitiveDeps[function.Name][fromFunc] {
				return fmt.Errorf("function '%s' in reRunIf.from is not transitively reachable through the dependency chain of function '%s'", fromFunc, function.Name)
			}
		}
	}

	// Validate allowedSystemFunctions if present
	if len(config.AllowedSystemFunctions) > 0 {
		validSystemFunctions := map[string]bool{
			SystemFunctionAskConversationHistory:    true,
			SystemFunctionAskKnowledgeBase:          true,
			SystemFunctionAskToContext:              true,
			SystemFunctionDoDeepWebResearch:         true,
			SystemFunctionDoSimpleWebSearch:         true,
			SystemFunctionGetWeekdayFromDate:        true,
			SystemFunctionQueryMemories:             true,
			SystemFunctionFetchWebContent:           true,
			SystemFunctionQueryCustomerServiceChats: true,
			SystemFunctionAnalyzeImage:              true,
			SystemFunctionSearchCodebase:            true,
			SystemFunctionQueryDocuments:            true,
		}

		for _, funcName := range config.AllowedSystemFunctions {
			if !validSystemFunctions[funcName] {
				return fmt.Errorf("invalid system function '%s' in reRunIf.allowedSystemFunctions", funcName)
			}
		}
	}

	// Validate deterministic expression if present
	if config.Deterministic != "" {
		if err := ValidateDeterministicExpression(config.Deterministic); err != nil {
			return fmt.Errorf("reRunIf.deterministic: %w", err)
		}

		// Extract all function references from the deterministic expression
		referencedFuncs := ExtractFunctionReferences(config.Deterministic)

		if len(referencedFuncs) > 0 {
			// Build transitive dependency map
			transitiveDeps := buildTransitiveDependencies(tool.Functions)

			// "result" is always valid as it references the function's own output
			// $RETRY is always valid as it's the retry context
			var missingFuncs []string
			for _, funcName := range referencedFuncs {
				if funcName == "result" || funcName == "RETRY" {
					continue // Always valid
				}
				// Check if funcName is transitively reachable from current function
				if !transitiveDeps[function.Name][funcName] {
					missingFuncs = append(missingFuncs, funcName)
				}
			}

			if len(missingFuncs) > 0 {
				return fmt.Errorf("reRunIf.deterministic expression references functions not transitively reachable through 'needs': %v", missingFuncs)
			}
		}
	}

	// Validate params if present (param names must match function input names)
	if len(config.Params) > 0 {
		// Build set of valid input names from function.Input
		validInputNames := make(map[string]bool)
		for _, input := range function.Input {
			validInputNames[input.Name] = true
		}

		// Check each param name
		var invalidParams []string
		for paramName := range config.Params {
			if !validInputNames[paramName] {
				invalidParams = append(invalidParams, paramName)
			}
		}

		if len(invalidParams) > 0 {
			return fmt.Errorf("reRunIf.params contains unknown input names: %v (valid inputs: %v)",
				invalidParams, getInputNames(function.Input))
		}
	}

	return nil
}

// parseFunctionReRunIf parses the reRunIf field and populates function.ParsedReRunIf
func parseFunctionReRunIf(function *Function, toolName string) error {
	if function.ReRunIf == nil {
		function.ParsedReRunIf = nil
		return nil
	}

	config, err := ParseReRunIf(function.ReRunIf)
	if err != nil {
		return fmt.Errorf("function '%s' in tool '%s' has invalid reRunIf: %w",
			function.Name, toolName, err)
	}

	function.ParsedReRunIf = config
	return nil
}

// parseFunctionSanitize parses the sanitize field and populates function.ParsedSanitize.
// Also applies auto-sanitize rules for functions using web-related system functions.
func parseFunctionSanitize(function *Function, toolName string) error {
	// Parse explicit sanitize config if present
	if function.Sanitize != nil {
		config, err := ParseSanitize(function.Sanitize)
		if err != nil {
			return fmt.Errorf("function '%s' in tool '%s' has invalid sanitize: %w",
				function.Name, toolName, err)
		}
		function.ParsedSanitize = config

		// Validate the parsed config
		if config != nil {
			if err := validateSanitizeConfig(config, function.Name, toolName); err != nil {
				return err
			}
		}
		return nil
	}

	// No explicit sanitize field — check for auto-sanitize rules.
	// Functions using web-fetching system functions get auto-sanitize "fence".
	webSystemFunctions := map[string]bool{
		SystemFunctionFetchWebContent:   true,
		SystemFunctionDoDeepWebResearch: true,
		SystemFunctionDoSimpleWebSearch: true,
	}

	// Check successCriteria allowedSystemFunctions, runOnlyIf allowedSystemFunctions, and input-level
	usesWebFunc := false
	for _, input := range function.Input {
		if scObj, err := ParseSuccessCriteria(input.SuccessCriteria); err == nil && scObj != nil {
			for _, fn := range scObj.AllowedSystemFunctions {
				if webSystemFunctions[fn] {
					usesWebFunc = true
					break
				}
			}
		}
		if usesWebFunc {
			break
		}
	}

	if !usesWebFunc {
		if roiObj, err := ParseRunOnlyIf(function.RunOnlyIf); err == nil && roiObj != nil {
			for _, fn := range roiObj.AllowedSystemFunctions {
				if webSystemFunctions[fn] {
					usesWebFunc = true
					break
				}
			}
			if !usesWebFunc && roiObj.Inference != nil {
				for _, fn := range roiObj.Inference.AllowedSystemFunctions {
					if webSystemFunctions[fn] {
						usesWebFunc = true
						break
					}
				}
			}
		}
	}

	// Auto-apply fence for functions using web system functions
	if usesWebFunc {
		function.ParsedSanitize = &SanitizeConfig{Strategy: SanitizeStrategyFence}
	}

	return nil
}

// parseInputSanitize parses the sanitize field on an input and applies auto-sanitize rules.
func parseInputSanitize(input *Input, functionName, toolName string) error {
	// Parse explicit sanitize config if present
	if input.Sanitize != nil {
		config, err := ParseSanitize(input.Sanitize)
		if err != nil {
			return fmt.Errorf("input '%s' in function '%s' of tool '%s' has invalid sanitize: %w",
				input.Name, functionName, toolName, err)
		}
		input.ParsedSanitize = config

		// Validate the parsed config
		if config != nil {
			if err := validateSanitizeConfig(config, functionName, toolName); err != nil {
				return err
			}
		}
		return nil
	}

	// Auto-sanitize: inputs with origin: "search" get fence automatically
	if input.Origin == DataOriginSearch && !IsSanitizeExplicitlyDisabled(input.Sanitize) {
		input.ParsedSanitize = &SanitizeConfig{Strategy: SanitizeStrategyFence}
	}

	return nil
}

// validateSanitizeConfig validates a parsed SanitizeConfig for consistency
func validateSanitizeConfig(config *SanitizeConfig, functionName, toolName string) error {
	// Validate strategy
	validStrategies := map[string]bool{
		SanitizeStrategyFence:      true,
		SanitizeStrategyStrict:     true,
		SanitizeStrategyLLMExtract: true,
	}
	if !validStrategies[config.Strategy] {
		return fmt.Errorf("function '%s' in tool '%s' has invalid sanitize strategy '%s'; must be 'fence', 'strict', or 'llm_extract'",
			functionName, toolName, config.Strategy)
	}

	// extract fields only valid with llm_extract
	if len(config.Extract) > 0 && config.Strategy != SanitizeStrategyLLMExtract {
		return fmt.Errorf("function '%s' in tool '%s': sanitize.extract is only valid with strategy 'llm_extract'",
			functionName, toolName)
	}

	// extract fields required for llm_extract
	if config.Strategy == SanitizeStrategyLLMExtract && len(config.Extract) == 0 {
		return fmt.Errorf("function '%s' in tool '%s': sanitize strategy 'llm_extract' requires at least one extract field",
			functionName, toolName)
	}

	// customPatterns only valid with strict
	if len(config.CustomPatterns) > 0 && config.Strategy != SanitizeStrategyStrict {
		return fmt.Errorf("function '%s' in tool '%s': sanitize.customPatterns is only valid with strategy 'strict'",
			functionName, toolName)
	}

	// Validate each customPattern is a valid regex
	for i, pattern := range config.CustomPatterns {
		if _, err := regexp.Compile(pattern); err != nil {
			return fmt.Errorf("function '%s' in tool '%s': sanitize.customPatterns[%d] is not a valid regex: %w",
				functionName, toolName, i, err)
		}
	}

	// Validate maxLength
	if config.MaxLength < 0 {
		return fmt.Errorf("function '%s' in tool '%s': sanitize.maxLength must be non-negative",
			functionName, toolName)
	}

	return nil
}

// parseFunctionCache parses the cache field which can be either an int (backwards compatible)
// or a CacheConfig object. It populates function.ParsedCache with the parsed configuration.
func parseFunctionCache(function *Function, toolName string) error {
	if function.Cache == nil {
		function.ParsedCache = nil
		return nil
	}

	// Handle int (backwards compatible format: cache: 300)
	switch v := function.Cache.(type) {
	case int:
		if v < 0 {
			return fmt.Errorf("function '%s' in tool '%s' has an invalid cache value '%d'; must be greater than 0 or omitted",
				function.Name, toolName, v)
		}
		if v == 0 {
			function.ParsedCache = nil
			return nil
		}
		includeInputs := true
		function.ParsedCache = &CacheConfig{
			Scope:         CacheScopeGlobal,
			TTL:           v,
			IncludeInputs: &includeInputs,
		}
		return nil

	case float64:
		// YAML unmarshals numbers as float64
		intVal := int(v)
		if intVal < 0 {
			return fmt.Errorf("function '%s' in tool '%s' has an invalid cache value '%d'; must be greater than 0 or omitted",
				function.Name, toolName, intVal)
		}
		if intVal == 0 {
			function.ParsedCache = nil
			return nil
		}
		includeInputs := true
		function.ParsedCache = &CacheConfig{
			Scope:         CacheScopeGlobal,
			TTL:           intVal,
			IncludeInputs: &includeInputs,
		}
		return nil

	case map[string]interface{}:
		return parseCacheConfigFromMap(function, v, toolName)

	case map[interface{}]interface{}:
		// Convert to map[string]interface{}
		converted := make(map[string]interface{})
		for k, val := range v {
			if keyStr, ok := k.(string); ok {
				converted[keyStr] = val
			}
		}
		return parseCacheConfigFromMap(function, converted, toolName)

	default:
		return fmt.Errorf("function '%s' in tool '%s' has invalid cache type '%T'; must be int or object with scope/ttl/includeInputs",
			function.Name, toolName, function.Cache)
	}
}

// parseCacheConfigFromMap parses a cache config from a map representation
func parseCacheConfigFromMap(function *Function, configMap map[string]interface{}, toolName string) error {
	config := &CacheConfig{}

	// Parse scope
	if scopeVal, ok := configMap["scope"]; ok {
		scopeStr, ok := scopeVal.(string)
		if !ok {
			return fmt.Errorf("function '%s' in tool '%s' has invalid cache.scope type '%T'; must be string",
				function.Name, toolName, scopeVal)
		}
		scope := CacheScope(scopeStr)
		if scope != CacheScopeGlobal && scope != CacheScopeClient && scope != CacheScopeMessage {
			return fmt.Errorf("function '%s' in tool '%s' has invalid cache.scope '%s'; must be one of: global, client, message",
				function.Name, toolName, scopeStr)
		}
		config.Scope = scope
	} else {
		config.Scope = CacheScopeGlobal
	}

	// Parse ttl
	if ttlVal, ok := configMap["ttl"]; ok {
		switch ttl := ttlVal.(type) {
		case int:
			config.TTL = ttl
		case float64:
			config.TTL = int(ttl)
		default:
			return fmt.Errorf("function '%s' in tool '%s' has invalid cache.ttl type '%T'; must be int",
				function.Name, toolName, ttlVal)
		}
	}

	if config.TTL <= 0 {
		return fmt.Errorf("function '%s' in tool '%s' has invalid cache.ttl '%d'; must be greater than 0",
			function.Name, toolName, config.TTL)
	}

	// Parse includeInputs
	if includeVal, ok := configMap["includeInputs"]; ok {
		includeBool, ok := includeVal.(bool)
		if !ok {
			return fmt.Errorf("function '%s' in tool '%s' has invalid cache.includeInputs type '%T'; must be bool",
				function.Name, toolName, includeVal)
		}
		config.IncludeInputs = &includeBool
	} else {
		// Default to true
		includeInputs := true
		config.IncludeInputs = &includeInputs
	}

	function.ParsedCache = config
	return nil
}

// checkCacheConfigWarnings checks for potentially problematic cache configurations
// and returns non-fatal warnings
func checkCacheConfigWarnings(functions []Function) []string {
	var warnings []string

	for _, fn := range functions {
		if fn.ParsedCache == nil {
			continue
		}

		// Warning: global scope with includeInputs: false means ONE cache entry for ALL users and ALL inputs
		if fn.ParsedCache.GetScope() == CacheScopeGlobal && !fn.ParsedCache.GetIncludeInputs() {
			warnings = append(warnings, fmt.Sprintf(
				"function '%s' has cache with scope 'global' and includeInputs: false - "+
					"this means ALL users will share a single cached result regardless of inputs. "+
					"This is rarely intended. Consider using scope 'client' or 'message', or set includeInputs: true",
				fn.Name))
		}

		// Warning: message scope with very long TTL (> 1 hour)
		if fn.ParsedCache.GetScope() == CacheScopeMessage && fn.ParsedCache.TTL > 3600 {
			warnings = append(warnings, fmt.Sprintf(
				"function '%s' has cache with scope 'message' and TTL of %d seconds (> 1 hour) - "+
					"message-scoped caches typically don't need long TTLs since messages are short-lived. "+
					"Consider using scope 'client' for longer caching periods",
				fn.Name, fn.ParsedCache.TTL))
		}
	}

	return warnings
}

// checkVariablesInsideNestedStrings detects expression-like syntax in non-evaluating fields.
// Only 'deterministic:' fields are evaluated as expressions. Other fields like 'value:', 'params:', etc.
// just do string replacement. If someone writes expression syntax (like ==, !=, .length, etc.) in these
// fields, the expression won't be evaluated - it will remain as a literal string with variables replaced.
// This is almost always a mistake.
func checkVariablesInsideNestedStrings(functions []Function) []string {
	var warnings []string

	// Regex to detect expression-like syntax that suggests the user expects evaluation
	// Matches: ==, !=, >=, <=, &&, ||, .length, .includes, .indexOf, etc.
	// Note: We use == and != but not standalone > or < since those can appear in other contexts
	expressionPatternRegex := regexp.MustCompile(`(==|!=|>=|<=|&&|\|\||\.length|\.includes|\.indexOf|\.startsWith|\.endsWith|\.match)`)

	for _, fn := range functions {
		// Check input.value fields
		for _, input := range fn.Input {
			if input.Value != "" {
				if expressionPatternRegex.MatchString(input.Value) {
					warnings = append(warnings, fmt.Sprintf(
						"function '%s' input '%s' has value '%s' which contains expression syntax (like ==, !=, .length, etc.). "+
							"This value will NOT be evaluated - only string replacement happens. The expression will remain as a literal string. "+
							"If you need expression evaluation, use 'deterministic:' in runOnlyIf or breakIf instead.",
						fn.Name, input.Name, truncateForWarning(input.Value, 80)))
				}
			}
		}

		// Check step.with fields (excluding deterministic-aware fields)
		for _, step := range fn.Steps {
			for paramName, paramValue := range step.With {
				strValue, ok := paramValue.(string)
				if !ok {
					continue
				}
				// Skip fields that ARE evaluated
				if paramName == "deterministic" {
					continue
				}
				if expressionPatternRegex.MatchString(strValue) {
					warnings = append(warnings, fmt.Sprintf(
						"function '%s' step '%s' param '%s' has value '%s' which contains expression syntax (like ==, !=, .length, etc.). "+
							"This value will NOT be evaluated - only string replacement happens. The expression will remain as a literal string. "+
							"If you need expression evaluation, use 'deterministic:' in runOnlyIf or breakIf instead.",
						fn.Name, step.Name, paramName, truncateForWarning(strValue, 80)))
				}
			}
		}

		// Check output.value field
		if fn.Output != nil && fn.Output.Value != "" {
			if expressionPatternRegex.MatchString(fn.Output.Value) {
				warnings = append(warnings, fmt.Sprintf(
					"function '%s' output.value '%s' contains expression syntax (like ==, !=, .length, etc.). "+
						"This value will NOT be evaluated - only string replacement happens. The expression will remain as a literal string. "+
						"If you need expression evaluation, consider using a different approach.",
					fn.Name, truncateForWarning(fn.Output.Value, 80)))
			}
		}

		// Check onSuccess/onFailure params
		for _, call := range fn.OnSuccess {
			for paramName, paramValue := range call.Params {
				strValue, ok := paramValue.(string)
				if !ok {
					continue
				}
				if expressionPatternRegex.MatchString(strValue) {
					warnings = append(warnings, fmt.Sprintf(
						"function '%s' onSuccess call to '%s' param '%s' has value '%s' which contains expression syntax. "+
							"This value will NOT be evaluated - only string replacement happens.",
						fn.Name, call.Name, paramName, truncateForWarning(strValue, 80)))
				}
			}
		}
		for _, call := range fn.OnFailure {
			for paramName, paramValue := range call.Params {
				strValue, ok := paramValue.(string)
				if !ok {
					continue
				}
				if expressionPatternRegex.MatchString(strValue) {
					warnings = append(warnings, fmt.Sprintf(
						"function '%s' onFailure call to '%s' param '%s' has value '%s' which contains expression syntax. "+
							"This value will NOT be evaluated - only string replacement happens.",
						fn.Name, call.Name, paramName, truncateForWarning(strValue, 80)))
				}
			}
		}

		// Check needs params
		for _, need := range fn.Needs {
			for paramName, paramValue := range need.Params {
				strValue, ok := paramValue.(string)
				if !ok {
					continue
				}
				if expressionPatternRegex.MatchString(strValue) {
					warnings = append(warnings, fmt.Sprintf(
						"function '%s' needs '%s' param '%s' has value '%s' which contains expression syntax. "+
							"This value will NOT be evaluated - only string replacement happens.",
						fn.Name, need.Name, paramName, truncateForWarning(strValue, 80)))
				}
			}
		}
	}

	return warnings
}

// truncateForWarning truncates a string for display in warning messages
func truncateForWarning(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// checkPublicFunctionsWithWorkflowVars checks for public functions that have inputs with workflow variable references.
// This is a WARNING (not an error) because there's a valid use case: public functions called via initiate_workflow
// with context.params injection. However, if the function is NOT called this way, it will fail at runtime.
// Returns non-fatal warnings to alert the user to verify this is intentional.
func checkPublicFunctionsWithWorkflowVars(functions []Function) []string {
	var warnings []string

	for _, fn := range functions {
		// Skip private functions - they can have workflow vars
		if isPrivateFunction(fn.Name) {
			continue
		}

		// Check each input for workflow variable references
		for _, input := range fn.Input {
			if input.Value != "" && isWorkflowVariableReference(input.Value) {
				warnings = append(warnings, fmt.Sprintf(
					"public function '%s' has input '%s' with workflow variable reference '%s'. "+
						"Public functions are typically called directly via the agentic loop and cannot receive workflow parameters. "+
						"This is ONLY valid if this function is called via 'initiate_workflow' with 'context.params' injection. "+
						"If not, either make the function private (start with lowercase) or use a different input source (origin: chat, inference, function, etc.)",
					fn.Name, input.Name, input.Value))
			}
		}
	}

	return warnings
}

// checkPublicFunctionsNeedsParamsFromInputs checks for public functions that have needs
// with params referencing their own input variables that DON'T have proper origins.
// If an input has origin: "chat", "inference", or "function", it will be resolved BEFORE
// the needs are evaluated, so it's safe to reference.
// Only inputs with value: "$something" (no proper origin) are problematic because they
// rely on external injection which won't happen for direct public function calls.
// Returns errors for any violations.
func checkPublicFunctionsNeedsParamsFromInputs(functions []Function, configEnvVars []EnvVar) []error {
	var errors []error

	// Build set of env var names
	envVarNames := make(map[string]bool)
	for _, envVar := range configEnvVars {
		envVarNames[envVar.Name] = true
	}

	// System variables that are always available
	systemVars := map[string]bool{
		"USER": true, "NOW": true, "MESSAGE": true,
		"ADMIN": true, "COMPANY": true, "UUID": true,
		"FILE": true, "ME": true,
	}

	// Pattern to match both $var and ${var} syntax
	varPattern := regexp.MustCompile(`\$\{?([a-zA-Z_][a-zA-Z0-9_]*)`)

	for _, fn := range functions {
		// Skip private functions - they're called via onSuccess/onFailure/needs with params
		if isPrivateFunction(fn.Name) {
			continue
		}

		// Build map of input names to their origins
		// Inputs with proper origins (chat, inference, function) are resolved before needs
		inputsWithoutOrigin := make(map[string]bool)
		for _, input := range fn.Input {
			// If input has no proper origin but has a value with $var reference, it's problematic
			if input.Origin == "" && input.Value != "" && strings.Contains(input.Value, "$") {
				inputsWithoutOrigin[input.Name] = true
			}
		}

		// If no inputs without origin, nothing to check
		if len(inputsWithoutOrigin) == 0 {
			continue
		}

		// Build set of needs function names (their results are available)
		needsNames := make(map[string]bool)
		for _, need := range fn.Needs {
			needsNames[need.Name] = true
		}

		// Check needs params for references to inputs without proper origins
		for _, need := range fn.Needs {
			for paramName, paramValue := range need.Params {
				strValue, ok := paramValue.(string)
				if !ok {
					continue
				}

				matches := varPattern.FindAllStringSubmatch(strValue, -1)

				for _, match := range matches {
					varName := match[1]
					baseVar := strings.Split(varName, ".")[0]

					// Skip if it's an env var, system var, or result from needs
					if envVarNames[baseVar] || systemVars[baseVar] || needsNames[baseVar] {
						continue
					}

					// If it references an input WITHOUT a proper origin -> ERROR
					if inputsWithoutOrigin[baseVar] {
						errors = append(errors, fmt.Errorf(
							"public function '%s' has needs '%s' with param '%s' referencing input variable '$%s'. "+
								"This input has no origin (chat/inference/function) and uses value: \"$%s\" which requires external injection. "+
								"Either: (1) add origin: \"chat\" to the input so it's resolved before needs, "+
								"(2) make the function private (lowercase first letter) and call it via onSuccess/onFailure/needs, "+
								"or (3) call it via initiate_workflow with context.params injection",
							fn.Name, need.Name, paramName, varName, varName))
					}
				}
			}
		}
	}

	return errors
}

// checkInputVariableReferencesAccumulated checks that input variables with value references
// only reference accumulated inputs (previous inputs in order), ENV vars, or system vars.
// For PRIVATE functions (lowercase first letter), skip this validation entirely because
// they receive values via params injection from onSuccess/needs calls.
// Returns errors for any violations.
func checkInputVariableReferencesAccumulated(functions []Function, configEnvVars []EnvVar) []error {
	var errors []error

	// Build set of env var names
	envVarNames := make(map[string]bool)
	for _, envVar := range configEnvVars {
		envVarNames[envVar.Name] = true
	}

	// System variables that are always available
	systemVars := map[string]bool{
		"USER": true, "NOW": true, "MESSAGE": true,
		"ADMIN": true, "COMPANY": true, "UUID": true,
		"FILE": true, "ME": true,
	}

	// Pattern to match both $var and ${var} syntax
	varPattern := regexp.MustCompile(`\$\{?([a-zA-Z_][a-zA-Z0-9_]*)`)

	for _, fn := range functions {
		// Skip private functions - they receive values via params injection from workflows
		if isPrivateFunction(fn.Name) {
			continue
		}

		// Skip functions with requiresInitiateWorkflow - they can only be called via workflows
		// which inject params, so $varName references will be resolved at runtime
		if fn.RequiresInitiateWorkflow {
			continue
		}

		// Track accumulated inputs (inputs seen so far in order)
		accumulatedInputs := make(map[string]bool)

		for _, input := range fn.Input {
			// Check if this input's value references variables
			if input.Value != "" {
				matches := varPattern.FindAllStringSubmatch(input.Value, -1)

				for _, match := range matches {
					varName := match[1]
					baseVar := strings.Split(varName, ".")[0]

					// Valid sources: env vars, system vars, or accumulated inputs
					if envVarNames[baseVar] || systemVars[baseVar] || accumulatedInputs[baseVar] {
						continue
					}

					// Invalid reference
					errors = append(errors, fmt.Errorf(
						"function '%s' input '%s' references '$%s' which is not available. "+
							"Input values can only reference: previous inputs (accumulated), ENV vars, or system vars ($USER, $NOW, etc.)",
						fn.Name, input.Name, varName))
				}
			}

			// Add this input to accumulated (for next inputs to reference)
			accumulatedInputs[input.Name] = true
		}
	}

	return errors
}

// preValidateInputCacheFormat checks if any input uses the object format for cache (scope/ttl/includeInputs)
// which is only supported at the function level, not at the input level.
// This pre-validation provides a clear error message before the YAML parser fails with a cryptic error.
func preValidateInputCacheFormat(yamlContent string) error {
	// Parse into a generic structure to inspect input cache fields
	var raw map[string]interface{}
	if err := yaml.Unmarshal([]byte(yamlContent), &raw); err != nil {
		// Let the main parser handle this error
		return nil
	}

	tools, ok := raw["tools"].([]interface{})
	if !ok {
		return nil
	}

	for _, toolRaw := range tools {
		tool, ok := toolRaw.(map[interface{}]interface{})
		if !ok {
			continue
		}

		toolName := "unknown"
		if name, ok := tool["name"].(string); ok {
			toolName = name
		}

		functions, ok := tool["functions"].([]interface{})
		if !ok {
			continue
		}

		for _, funcRaw := range functions {
			fn, ok := funcRaw.(map[interface{}]interface{})
			if !ok {
				continue
			}

			funcName := "unknown"
			if name, ok := fn["name"].(string); ok {
				funcName = name
			}

			inputs, ok := fn["input"].([]interface{})
			if !ok {
				continue
			}

			for _, inputRaw := range inputs {
				input, ok := inputRaw.(map[interface{}]interface{})
				if !ok {
					continue
				}

				inputName := "unknown"
				if name, ok := input["name"].(string); ok {
					inputName = name
				}

				// Check if cache is a map (object format) instead of int
				if cache, exists := input["cache"]; exists {
					if _, isMap := cache.(map[interface{}]interface{}); isMap {
						return fmt.Errorf("input '%s' in function '%s' of tool '%s' has invalid cache format: "+
							"input-level cache only supports integer values (TTL in seconds). "+
							"The object format with scope/ttl/includeInputs is only supported at the function level. "+
							"Use 'cache: 300' instead of 'cache: {scope: ..., ttl: ...}'",
							inputName, funcName, toolName)
					}
				}
			}
		}
	}

	return nil
}

// validateCallRule validates the "callRule" field of a function
func validateCallRule(callRule *CallRule, functionName, toolName string) error {
	// Validate rule type
	validRuleTypes := map[string]bool{
		CallRuleTypeOnce:     true,
		CallRuleTypeUnique:   true,
		CallRuleTypeMultiple: true,
	}

	if callRule.Type == "" {
		return fmt.Errorf("function '%s' in tool '%s' has callRule with empty type", functionName, toolName)
	}

	if !validRuleTypes[callRule.Type] {
		return fmt.Errorf("function '%s' in tool '%s' has invalid callRule type '%s'; must be one of: once, unique, multiple",
			functionName, toolName, callRule.Type)
	}

	// If type is "multiple", no scope is needed
	if callRule.Type == CallRuleTypeMultiple {
		if callRule.Scope != "" {
			return fmt.Errorf("function '%s' in tool '%s' has callRule type 'multiple' but defines a scope; scope is not needed for 'multiple' type",
				functionName, toolName)
		}
		return nil
	}

	// For "once" and "unique" types, validate scope
	validScopes := map[string]bool{
		CallRuleScopeUser:            true,
		CallRuleScopeMessage:         true,
		CallRuleScopeMinimumInterval: true,
		CallRuleScopeCompany:         true,
	}

	// Default scope to "message" if not specified
	if callRule.Scope == "" {
		callRule.Scope = CallRuleScopeMessage
	}

	if !validScopes[callRule.Scope] {
		return fmt.Errorf("function '%s' in tool '%s' has invalid callRule scope '%s'; must be one of: user, message, minimumInterval, company",
			functionName, toolName, callRule.Scope)
	}

	// Validate minimumInterval for minimumInterval scope
	if callRule.Scope == CallRuleScopeMinimumInterval {
		if callRule.MinimumInterval <= 0 {
			return fmt.Errorf("function '%s' in tool '%s' has callRule with scope 'minimumInterval' but minimumInterval is %d; must be greater than 0",
				functionName, toolName, callRule.MinimumInterval)
		}
	} else {
		// For other scopes, minimumInterval should not be set
		if callRule.MinimumInterval > 0 {
			return fmt.Errorf("function '%s' in tool '%s' has callRule with minimumInterval set but scope is '%s'; minimumInterval is only valid for 'minimumInterval' scope",
				functionName, toolName, callRule.Scope)
		}
	}

	// Validate statusFilter if specified
	if len(callRule.StatusFilter) > 0 {
		validStatusFilters := map[string]bool{
			CallRuleStatusFilterCompleted: true,
			CallRuleStatusFilterFailed:    true,
			CallRuleStatusFilterAll:       true,
		}

		for _, status := range callRule.StatusFilter {
			if !validStatusFilters[status] {
				return fmt.Errorf("function '%s' in tool '%s' has invalid callRule statusFilter value '%s'; must be one of: complete, failed, all",
					functionName, toolName, status)
			}
		}

		// If "all" is present with other values, normalize to just ["all"]
		hasAll := false
		for _, status := range callRule.StatusFilter {
			if status == CallRuleStatusFilterAll {
				hasAll = true
				break
			}
		}
		if hasAll && len(callRule.StatusFilter) > 1 {
			callRule.StatusFilter = []string{CallRuleStatusFilterAll}
		}
	}

	return nil
}

// validateRequiresUserConfirmation validates the RequiresUserConfirmation field
func validateRequiresUserConfirmation(requiresUserConfirmation interface{}, functionName, toolName string) error {
	// Handle boolean case (backward compatibility)
	if _, ok := requiresUserConfirmation.(bool); ok {
		// Boolean is always valid
		return nil
	}

	// Handle object case
	if configMap, ok := requiresUserConfirmation.(map[string]interface{}); ok {
		enabled, hasEnabled := configMap["enabled"]
		message, hasMessage := configMap["message"]

		if !hasEnabled {
			return fmt.Errorf("requiresUserConfirmation object must have an 'enabled' field")
		}

		enabledBool, ok := enabled.(bool)
		if !ok {
			return fmt.Errorf("requiresUserConfirmation.enabled must be a boolean")
		}

		// If enabled is false, message should not be present
		if !enabledBool && hasMessage {
			return fmt.Errorf("requiresUserConfirmation.message cannot be set when enabled is false")
		}

		// If message is present, it must be a non-empty string
		if hasMessage {
			messageStr, ok := message.(string)
			if !ok {
				return fmt.Errorf("requiresUserConfirmation.message must be a string")
			}
			if strings.TrimSpace(messageStr) == "" {
				return fmt.Errorf("requiresUserConfirmation.message cannot be empty")
			}
			// If message is present, enabled must be true
			if !enabledBool {
				return fmt.Errorf("requiresUserConfirmation.enabled must be true when message is provided")
			}
		}

		return nil
	}

	// Handle map[interface{}]interface{} case from YAML parsing
	if configMap, ok := requiresUserConfirmation.(map[interface{}]interface{}); ok {
		enabled, hasEnabled := configMap["enabled"]
		message, hasMessage := configMap["message"]

		if !hasEnabled {
			return fmt.Errorf("requiresUserConfirmation object must have an 'enabled' field")
		}

		enabledBool, ok := enabled.(bool)
		if !ok {
			return fmt.Errorf("requiresUserConfirmation.enabled must be a boolean")
		}

		// If enabled is false, message should not be present
		if !enabledBool && hasMessage {
			return fmt.Errorf("requiresUserConfirmation.message cannot be set when enabled is false")
		}

		// If message is present, it must be a non-empty string
		if hasMessage {
			messageStr, ok := message.(string)
			if !ok {
				return fmt.Errorf("requiresUserConfirmation.message must be a string")
			}
			if strings.TrimSpace(messageStr) == "" {
				return fmt.Errorf("requiresUserConfirmation.message cannot be empty")
			}
			// If message is present, enabled must be true
			if !enabledBool {
				return fmt.Errorf("requiresUserConfirmation.enabled must be true when message is provided")
			}
		}

		return nil
	}

	return fmt.Errorf("requiresUserConfirmation must be a boolean or an object with 'enabled' and optional 'message' fields")
}

// validateTriggers validates the triggers for a function
// Note: Functions without triggers are allowed if they are reachable from triggered functions
// (checked at tool level via validateTriggerReachability)
func validateTriggers(triggers []Trigger, functionName, toolName string) error {
	// Skip validation if no triggers - reachability check is done at tool level
	if len(triggers) == 0 {
		return nil
	}

	validTriggerTypes := map[string]bool{
		TriggerTime:                   true,
		TriggerOnTeamMessage:          true,
		TriggerOnUserMessage:          true,
		TriggerOnCompletedUserMessage: true,
		TriggerOnCompletedTeamMessage: true,
		TriggerFlexForTeam:            true,
		TriggerFlexForUser:            true,
		TriggerOnMeetingStart:         true,
		TriggerOnMeetingEnd:           true,
		// Email triggers (fire only when channel is "email")
		TriggerOnUserEmail:          true,
		TriggerOnTeamEmail:          true,
		TriggerOnCompletedUserEmail: true,
		TriggerOnCompletedTeamEmail: true,
		TriggerFlexForUserEmail:     true,
		TriggerFlexForTeamEmail:     true,
	}

	for i, trigger := range triggers {
		if trigger.Type == "" {
			return fmt.Errorf("trigger at index %d in function '%s' of tool '%s' has an empty type",
				i, functionName, toolName)
		}

		if !validTriggerTypes[trigger.Type] {
			return fmt.Errorf("trigger at index %d in function '%s' of tool '%s' has invalid type '%s'; must be one of the options: %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s",
				i, functionName, toolName, trigger.Type,
				TriggerTime, TriggerOnTeamMessage, TriggerOnUserMessage, TriggerOnCompletedUserMessage, TriggerOnCompletedTeamMessage, TriggerFlexForTeam, TriggerFlexForUser, TriggerOnMeetingStart, TriggerOnMeetingEnd,
				TriggerOnUserEmail, TriggerOnTeamEmail, TriggerOnCompletedUserEmail, TriggerOnCompletedTeamEmail, TriggerFlexForUserEmail, TriggerFlexForTeamEmail)
		}

		if trigger.Type == TriggerTime {
			if trigger.Cron == "" {
				return fmt.Errorf("trigger of type 'time_based' in function '%s' of tool '%s' must have a cron expression",
					functionName, toolName)
			}

			if !isValidCronExpression(trigger.Cron) {
				return fmt.Errorf("trigger of type 'time_based' in function '%s' of tool '%s' has an invalid cron expression: '%s' (must have exactly 6 fields: second minute hour day-of-month month day-of-week)",
					functionName, toolName, trigger.Cron)
			}

			// Validate concurrency control if present
			if err := validateConcurrencyControl(trigger.ConcurrencyControl, functionName, toolName); err != nil {
				return err
			}
		} else if trigger.ConcurrencyControl != nil {
			// ConcurrencyControl only valid for time_based triggers
			return fmt.Errorf("concurrencyControl in function '%s' of tool '%s' is only valid for time_based triggers",
				functionName, toolName)
		}
	}

	return nil
}

// validateTriggerReachability ensures all functions without triggers are reachable from triggered functions
// A function is "reachable" if it's transitively referenced via:
// - needs (dependency)
// - input.from with origin "function"
// - onSuccess callback
// - onFailure callback
func validateTriggerReachability(functions []Function, toolName string) error {
	// First, identify functions that have triggers
	triggeredFuncs := make(map[string]bool)
	for _, fn := range functions {
		if len(fn.Triggers) > 0 {
			triggeredFuncs[fn.Name] = true
		}
	}

	// If no triggered functions exist, all functions must have triggers
	if len(triggeredFuncs) == 0 {
		for _, fn := range functions {
			if len(fn.Triggers) == 0 {
				return fmt.Errorf("function '%s' in tool '%s' must have at least one trigger (no triggered functions found to reach it)", fn.Name, toolName)
			}
		}
		return nil
	}

	// Build a forward dependency graph: funcA -> [funcB, funcC] means funcA depends on/calls funcB and funcC
	forwardDeps := BuildForwardDependencyGraph(functions)

	// Compute transitive reachability: which functions can be reached from each triggered function
	reachableFromTriggered := make(map[string]bool)
	for triggeredFunc := range triggeredFuncs {
		// Mark the triggered function itself as reachable
		reachableFromTriggered[triggeredFunc] = true
		// Mark all functions transitively reachable from it
		markReachable(triggeredFunc, forwardDeps, reachableFromTriggered)
	}

	// Check if all functions without triggers are reachable
	// System functions are excluded from this check as they're loaded externally
	for _, fn := range functions {
		if len(fn.Triggers) == 0 && !reachableFromTriggered[fn.Name] && !systemFunctions[fn.Name] {
			return fmt.Errorf("function '%s' in tool '%s' must have at least one trigger or be reachable (via needs, input.from, onSuccess, or onFailure) from a triggered function", fn.Name, toolName)
		}
	}

	return nil
}

// validateOrphanPrivateFunctions ensures private functions are reachable from entry points.
// Entry points are: public functions (uppercase start), cron-triggered functions, or event-triggered functions (on_meeting_end, on_meeting_start).
// A private function is an orphan if it has no system trigger AND is not reachable from any entry point.
func validateOrphanPrivateFunctions(functions []Function, toolName string) error {
	// Build forward dependency graph (includes all callback types)
	forwardDeps := BuildForwardDependencyGraph(functions)

	// Identify entry points and system-triggered private functions
	entryPoints := make(map[string]bool)
	systemTriggeredPrivate := make(map[string]bool)

	for _, fn := range functions {
		// Public functions are entry points
		if !isPrivateFunction(fn.Name) {
			entryPoints[fn.Name] = true
			continue
		}
		// Private functions with system triggers (cron, meeting events, always_on_* messages/emails) are also entry points
		for _, trigger := range fn.Triggers {
			if trigger.Type == TriggerTime ||
				trigger.Type == TriggerOnMeetingEnd ||
				trigger.Type == TriggerOnMeetingStart ||
				trigger.Type == TriggerOnUserMessage ||
				trigger.Type == TriggerOnTeamMessage ||
				trigger.Type == TriggerOnCompletedUserMessage ||
				trigger.Type == TriggerOnCompletedTeamMessage ||
				trigger.Type == TriggerOnUserEmail ||
				trigger.Type == TriggerOnTeamEmail ||
				trigger.Type == TriggerOnCompletedUserEmail ||
				trigger.Type == TriggerOnCompletedTeamEmail {
				entryPoints[fn.Name] = true
				systemTriggeredPrivate[fn.Name] = true
				break
			}
		}
	}

	// Compute reachability from all entry points
	reachable := make(map[string]bool)
	for entryPoint := range entryPoints {
		reachable[entryPoint] = true
		markReachable(entryPoint, forwardDeps, reachable)
	}

	// Check private functions (excluding system-triggered ones) are reachable
	var orphans []string
	for _, fn := range functions {
		if isPrivateFunction(fn.Name) && !systemTriggeredPrivate[fn.Name] && !reachable[fn.Name] {
			orphans = append(orphans, fn.Name)
		}
	}

	if len(orphans) > 0 {
		return fmt.Errorf("orphan private function(s) in tool '%s': %v. Private functions must be reachable from public functions, cron-triggered, or event-triggered functions via dependency chains (needs, callbacks, or input.from)",
			toolName, orphans)
	}

	return nil
}

// BuildForwardDependencyGraph builds a graph where edges represent "can reach" relationships
// If funcA needs funcB, then funcA can reach funcB
// If funcA has input.from = funcB, then funcA can reach funcB
// If funcA has onSuccess = funcB, then funcA can reach funcB
// If funcA has onFailure = funcB, then funcA can reach funcB
// If funcA has onSkip = funcB, then funcA can reach funcB
// If funcA has onMissingUserInfo = funcB, then funcA can reach funcB
// If funcA has onUserConfirmationRequest = funcB, then funcA can reach funcB
// If funcA has onTeamApprovalRequest = funcB, then funcA can reach funcB
// If funcA has a step with foreach.waitFor = funcB, then funcA can reach funcB
// If funcA has a step with foreach.shouldSkip = funcB, then funcA can reach funcB
func BuildForwardDependencyGraph(functions []Function) map[string][]string {
	graph := make(map[string][]string)

	for _, fn := range functions {
		graph[fn.Name] = []string{}

		// Add needs dependencies
		for _, need := range fn.Needs {
			if !systemFunctions[need.Name] {
				graph[fn.Name] = append(graph[fn.Name], need.Name)
			}
		}

		// Add input.from dependencies (function origin)
		for _, input := range fn.Input {
			if input.Origin == DataOriginFunction && input.From != "" {
				// Extract function name from "functionName" or "functionName.field"
				funcName := ExtractFunctionNameFromFrom(input.From)
				if funcName != "" && !systemFunctions[funcName] {
					graph[fn.Name] = append(graph[fn.Name], funcName)
				}
			}
		}

		// Add onSuccess callbacks
		for _, successCall := range fn.OnSuccess {
			if !systemFunctions[successCall.Name] {
				graph[fn.Name] = append(graph[fn.Name], successCall.Name)
			}
		}

		// Add onFailure callbacks
		for _, failureCall := range fn.OnFailure {
			if !systemFunctions[failureCall.Name] {
				graph[fn.Name] = append(graph[fn.Name], failureCall.Name)
			}
		}

		// Add onSkip callbacks
		for _, skipCall := range fn.OnSkip {
			if !systemFunctions[skipCall.Name] {
				graph[fn.Name] = append(graph[fn.Name], skipCall.Name)
			}
		}

		// Add onMissingUserInfo callbacks
		for _, missingInfoCall := range fn.OnMissingUserInfo {
			if !systemFunctions[missingInfoCall.Name] {
				graph[fn.Name] = append(graph[fn.Name], missingInfoCall.Name)
			}
		}

		// Add onUserConfirmationRequest callbacks
		for _, confirmationCall := range fn.OnUserConfirmationRequest {
			if !systemFunctions[confirmationCall.Name] {
				graph[fn.Name] = append(graph[fn.Name], confirmationCall.Name)
			}
		}

		// Add onTeamApprovalRequest callbacks
		for _, approvalCall := range fn.OnTeamApprovalRequest {
			if !systemFunctions[approvalCall.Name] {
				graph[fn.Name] = append(graph[fn.Name], approvalCall.Name)
			}
		}

		// Add step-level foreach callbacks (waitFor and shouldSkip)
		for _, step := range fn.Steps {
			if step.ForEach != nil {
				// Add waitFor callback
				if step.ForEach.WaitFor != nil && step.ForEach.WaitFor.Name != "" {
					waitForFunc := ExtractFunctionNameFromFrom(step.ForEach.WaitFor.Name)
					if waitForFunc != "" && !systemFunctions[waitForFunc] {
						graph[fn.Name] = append(graph[fn.Name], waitForFunc)
					}
				}
				// Add shouldSkip callbacks
				for _, skipCond := range step.ForEach.GetShouldSkipConditions() {
					if skipCond.Name != "" {
						skipFunc := ExtractFunctionNameFromFrom(skipCond.Name)
						if skipFunc != "" && !systemFunctions[skipFunc] {
							graph[fn.Name] = append(graph[fn.Name], skipFunc)
						}
					}
				}
			}
		}
	}

	return graph
}

// ExtractFunctionNameFromFrom extracts the function name from a "from" field
// which can be "functionName" or "functionName.field" or "functionName[0].field"
func ExtractFunctionNameFromFrom(from string) string {
	// Find first occurrence of . or [
	for i, ch := range from {
		if ch == '.' || ch == '[' {
			return from[:i]
		}
	}
	return from
}

// markReachable performs DFS to mark all functions reachable from the starting function
func markReachable(start string, graph map[string][]string, reachable map[string]bool) {
	for _, dep := range graph[start] {
		if !reachable[dep] {
			reachable[dep] = true
			markReachable(dep, graph, reachable)
		}
	}
}

// validateConcurrencyControl validates the concurrency control configuration
func validateConcurrencyControl(cc *ConcurrencyControl, functionName, toolName string) error {
	if cc == nil {
		return nil
	}

	// Validate strategy
	if cc.Strategy != "" {
		validStrategies := map[string]bool{
			ConcurrencyStrategyParallel: true,
			ConcurrencyStrategySkip:     true,
			ConcurrencyStrategyKill:     true,
		}
		if !validStrategies[cc.Strategy] {
			return fmt.Errorf("invalid concurrency strategy '%s' in function '%s' of tool '%s'; must be one of: %s, %s, %s",
				cc.Strategy, functionName, toolName,
				ConcurrencyStrategyParallel, ConcurrencyStrategySkip, ConcurrencyStrategyKill)
		}
	}

	// Validate maxParallel
	if cc.MaxParallel < 0 {
		return fmt.Errorf("maxParallel cannot be negative in function '%s' of tool '%s'",
			functionName, toolName)
	}
	if cc.MaxParallel > MaxAllowedParallel {
		return fmt.Errorf("maxParallel cannot exceed %d in function '%s' of tool '%s' (got %d)",
			MaxAllowedParallel, functionName, toolName, cc.MaxParallel)
	}

	// Validate killTimeout
	if cc.KillTimeout < 0 {
		return fmt.Errorf("killTimeout cannot be negative in function '%s' of tool '%s'",
			functionName, toolName)
	}

	return nil
}

func validateInputs(inputs []Input, function Function, tool Tool, configEnvVars []EnvVar) error {
	// Check for duplicate input names
	inputNames := make(map[string]bool)

	for i, input := range inputs {
		if input.Name == "" {
			return fmt.Errorf("input at index %d in function '%s' of tool '%s' has an empty name",
				i, function.Name, tool.Name)
		}

		if input.Name == strings.ToUpper(input.Name) {
			return fmt.Errorf("input '%s' in function '%s' of tool '%s' must be lowercase",
				input.Name, function.Name, tool.Name)
		}

		if inputNames[input.Name] {
			return fmt.Errorf("duplicate input name '%s' in function '%s' of tool '%s'",
				input.Name, function.Name, tool.Name)
		}
		inputNames[input.Name] = true

		if input.Description == "" {
			return fmt.Errorf("input '%s' in function '%s' of tool '%s' must have a description",
				input.Name, function.Name, tool.Name)
		}

		// Either origin or value must be specified (unless input is optional)
		if input.Origin == "" && input.Value == "" && !input.IsOptional {
			return fmt.Errorf("input '%s' in function '%s' of tool '%s' must have an origin or a static value",
				input.Name, function.Name, tool.Name)
		}

		if input.Origin != "" {
			validOrigins := map[string]bool{
				DataOriginChat:      true,
				DataOriginFunction:  true,
				DataOriginKnowledge: true,
				DataOriginInference: true,
				DataOriginSearch:    true,
				DataOriginMemory:    true,
			}

			if !validOrigins[input.Origin] {
				return fmt.Errorf("input '%s' in function '%s' of tool '%s' has invalid origin '%s'; must be one of chat, function, knowledge, inference, search",
					input.Name, function.Name, tool.Name, input.Origin)
			}

			if input.Origin == DataOriginMemory {
				if input.From == "" {
					return fmt.Errorf("input '%s' in function '%s' of tool '%s' is missing the 'from' field which is required when origin is set to memory",
						input.Name, function.Name, tool.Name)
				}

				if GetSuccessCriteriaCondition(input.SuccessCriteria) == "" {
					return fmt.Errorf("input '%s' in function '%s' of tool '%s' with origin 'memory' must have a success criteria",
						input.Name, function.Name, tool.Name)
				}

				// Ensure 'from' refers to a valid function (can be private or public)
				if !functionExists(input.From, tool.Functions) {
					return fmt.Errorf("the function '%s' referenced in input '%s' of function '%s' in tool '%s' was not found",
						input.From, input.Name, function.Name, tool.Name)
				}

				// Validate memoryRetrievalMode if specified
				if input.MemoryRetrievalMode != "" {
					validModes := map[string]bool{
						"all":    true,
						"latest": true,
					}
					if !validModes[input.MemoryRetrievalMode] {
						return fmt.Errorf("input '%s' in function '%s' of tool '%s' has invalid memoryRetrievalMode '%s'; must be either 'all' or 'latest'",
							input.Name, function.Name, tool.Name, input.MemoryRetrievalMode)
					}
				}
			}

			// Validate that memoryRetrievalMode is only used with memory origin
			if input.MemoryRetrievalMode != "" && input.Origin != DataOriginMemory {
				return fmt.Errorf("input '%s' in function '%s' of tool '%s' has 'memoryRetrievalMode' set but origin is '%s'; this field is only valid for inputs with origin 'memory'",
					input.Name, function.Name, tool.Name, input.Origin)
			}

			// For function origin, validate 'from' field
			if input.Origin == DataOriginFunction {
				if input.From == "" {
					return fmt.Errorf("input '%s' in function '%s' of tool '%s' is missing the 'from' field which is required when origin is set to function",
						input.Name, function.Name, tool.Name)
				}

				// Check if it's a shared function reference (supports dot notation like "utils_shared.functionName")
				baseFuncName := input.From
				if idx := strings.LastIndex(input.From, "."); idx != -1 {
					baseFuncName = input.From[idx+1:]
				}
				isSharedFunc := defaultRegistry.Functions[input.From] || defaultRegistry.Functions[baseFuncName]

				// Reject dot notation if it's NOT a shared function reference
				if strings.Contains(input.From, ".") && !isSharedFunc {
					return fmt.Errorf("input '%s' in function '%s' of tool '%s' has dot notation in 'from' field ('%s'). "+
						"Dot notation for field access is NOT supported in 'from'. "+
						"You must: 1) Get the whole JSON object in the input (e.g., from: 'functionName'), "+
						"2) Then access specific fields in steps using $variableName.field syntax. "+
						"Note: Dot notation IS allowed for shared function references (e.g., 'utils_shared.functionName')",
						input.Name, function.Name, tool.Name, input.From)
				}

				// Check if the referenced function exists (either in tool, as system function, or as shared function)
				isSysFunc := systemFunctions[input.From]
				if !isSysFunc && !isSharedFunc && !functionExists(input.From, tool.Functions) {
					return fmt.Errorf("the function '%s' referenced in input '%s' of function '%s' in tool '%s' was not found",
						input.From, input.Name, function.Name, tool.Name)
				}
			}

			if input.Origin == DataOriginInference && GetSuccessCriteriaCondition(input.SuccessCriteria) == "" {
				return fmt.Errorf("input '%s' in function '%s' of tool '%s' with origin 'inference' must have a success criteria",
					input.Name, function.Name, tool.Name)
			}
		}

		// Validate shouldBeHandledAsMessageToUser field
		if input.ShouldBeHandledAsMessageToUser {
			if input.Origin != DataOriginInference {
				return fmt.Errorf("input '%s' in function '%s' of tool '%s' has 'shouldBeHandledAsMessageToUser' set to true but origin is '%s'; this field is only valid for inputs with origin 'inference'",
					input.Name, function.Name, tool.Name, input.Origin)
			}
		}

		// Validate cache field
		if input.Cache < 0 {
			return fmt.Errorf("input '%s' in function '%s' of tool '%s' has an invalid cache value '%d'; must be greater than 0 or omitted",
				input.Name, function.Name, tool.Name, input.Cache)
		}
		if input.Cache == 0 {
			input.Cache = -1
		}

		// Validate 'with' field if specified
		if input.With != nil {
			if input.Origin != DataOriginChat {
				return fmt.Errorf("input '%s' in function '%s' of tool '%s' with 'with' field must have origin 'chat'",
					input.Name, function.Name, tool.Name)
			}

			if err := validateInputWithOptions(input, function, tool); err != nil {
				return err
			}
		}

		if input.RegexValidator != "" {
			_, err := regexp.Compile(input.RegexValidator)
			if err != nil {
				return fmt.Errorf("input '%s' in function '%s' of tool '%s' has invalid regex validator '%s': %w",
					input.Name, function.Name, tool.Name, input.RegexValidator, err)
			}
		}

		if input.JsonSchemaValidator != "" {
			compiler := jsonschema.NewCompiler()
			if err := compiler.AddResource("schema.json", strings.NewReader(input.JsonSchemaValidator)); err != nil {
				return fmt.Errorf("input '%s' in function '%s' of tool '%s' has invalid JSON schema validator: %w",
					input.Name, function.Name, tool.Name, err)
			}
			_, err := compiler.Compile("schema.json")
			if err != nil {
				return fmt.Errorf("input '%s' in function '%s' of tool '%s' has invalid JSON schema validator: %w",
					input.Name, function.Name, tool.Name, err)
			}
		}

		if input.RegexValidator != "" && input.JsonSchemaValidator != "" {
			return fmt.Errorf("input '%s' in function '%s' of tool '%s' cannot have both regexValidator and jsonSchemaValidator specified",
				input.Name, function.Name, tool.Name)
		}

		// Validate successCriteria if present
		if input.SuccessCriteria != nil && input.Origin == DataOriginInference {
			if err := validateSuccessCriteria(input.SuccessCriteria, input, function, tool); err != nil {
				return fmt.Errorf("in input '%s' of function '%s' in tool '%s': %w",
					input.Name, function.Name, tool.Name, err)
			}
		}

		// Validate onError if required
		if !input.IsOptional && input.OnError == nil && input.Value == "" && function.Operation != OperationMCP {
			return fmt.Errorf("input '%s' in function '%s' of tool '%s' is required but missing onError definition",
				input.Name, function.Name, tool.Name)
		}

		if input.OnError != nil {
			if err := validateOnError(input.OnError, input.Name, function, tool, configEnvVars); err != nil {
				return err
			}

			if input.OnError.Strategy != OnErrorRequestUserInput && input.OnError.With != nil {
				return fmt.Errorf("input '%s' in function '%s' of tool '%s' with 'onError' strategy '%s' cannot have 'with' field. only allowed for %s strategy",
					input.Name, function.Name, tool.Name, input.OnError.Strategy, OnErrorRequestUserInput)
			}

			if input.OnError.Strategy == OnErrorRequestUserInput && input.OnError.With != nil {
				// Create temporary Input to reuse existing validation logic
				tempInput := Input{
					Name:        input.Name,
					Description: input.Description,
					With:        input.OnError.With,
				}
				if err := validateInputWithOptions(tempInput, function, tool); err != nil {
					return fmt.Errorf("in onError: %w", err)
				}
			}
		}

		// Validate runOnlyIf if present
		if input.RunOnlyIf != nil {
			if err := validateInputRunOnlyIf(input.RunOnlyIf, input, i, inputs, function, tool); err != nil {
				return err
			}
		}

		// Validate async field - async inputs cannot reference other inputs
		if input.Async {
			if err := validateAsyncInput(input, inputs, function, tool); err != nil {
				return err
			}
		}
	}

	return nil
}

// validateAsyncInput checks that async inputs don't reference other inputs in the same function
func validateAsyncInput(input Input, inputs []Input, function Function, tool Tool) error {
	// Build set of other input names in this function
	otherInputNames := make(map[string]bool)
	for _, otherInput := range inputs {
		if otherInput.Name != input.Name {
			otherInputNames[otherInput.Name] = true
		}
	}

	// Collect text to check for variable references
	// Check description, successCriteria, AND value field
	textToCheck := input.Description
	if input.SuccessCriteria != nil {
		textToCheck += " " + GetSuccessCriteriaCondition(input.SuccessCriteria)
	}
	if input.Value != "" {
		textToCheck += " " + input.Value
	}

	// Pattern: $varName or ${varName}
	inputVarPattern := regexp.MustCompile(`\$\{?([a-zA-Z_][a-zA-Z0-9_]*)\}?`)
	matches := inputVarPattern.FindAllStringSubmatch(textToCheck, -1)

	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		varName := match[1]

		// Skip system variables (USER, NOW, COMPANY, etc.)
		if isAsyncSafeVariable(varName) {
			continue
		}

		// Error if referencing another input in this function
		if otherInputNames[varName] {
			return fmt.Errorf("input '%s' in function '%s' of tool '%s' has async: true but references input '$%s'; async inputs cannot depend on other inputs",
				input.Name, function.Name, tool.Name, varName)
		}
	}

	return nil
}

// isAsyncSafeVariable checks if a variable name is a system variable that's safe for async inputs
func isAsyncSafeVariable(varName string) bool {
	// System variables that don't depend on other inputs and are safe for async
	safeVars := map[string]bool{
		"USER":    true,
		"NOW":     true,
		"ME":      true,
		"MESSAGE": true,
		"ADMIN":   true,
		"COMPANY": true,
		"UUID":    true,
		"FILE":    true,
		"MEETING": true,
	}

	// Check direct match
	if safeVars[varName] {
		return true
	}

	// Check if it's a prefix (e.g., USER.name, NOW.date)
	for sv := range safeVars {
		if strings.HasPrefix(varName, sv+".") || strings.HasPrefix(varName, sv+"_") {
			return true
		}
	}

	// Also check if it looks like an env var (ALL_CAPS)
	if varName == strings.ToUpper(varName) && len(varName) > 0 {
		return true
	}

	return false
}

// Add a new function to validate the 'with' field options:
func validateInputWithOptions(input Input, function Function, tool Tool) error {
	// Check for mutual exclusivity of constraint options
	constraintCount := 0
	if input.With.OneOf != "" {
		constraintCount++
	}
	if input.With.ManyOf != "" {
		constraintCount++
	}
	if input.With.NotOneOf != "" {
		constraintCount++
	}

	if constraintCount > 1 {
		return fmt.Errorf("input '%s' in function '%s' of tool '%s' cannot have multiple constraint options (oneOf, manyOf, notOneOf) specified simultaneously",
			input.Name, function.Name, tool.Name)
	}

	// Validate that the referenced function exists
	var funcName string
	if input.With.OneOf != "" {
		funcName = input.With.OneOf
	} else if input.With.ManyOf != "" {
		funcName = input.With.ManyOf
	} else if input.With.NotOneOf != "" {
		funcName = input.With.NotOneOf
	} else {
		return nil // No selection options specified
	}

	//if !isPrivateFunction(funcName) {
	//	return fmt.Errorf("input '%s' in function '%s' of tool '%s' must refer to a private function in 'oneOf', 'manyOf', or 'notOneOf' field (must start with lowercase)",
	//		input.Name, function.Name, tool.Name)
	//}

	if !functionExists(funcName, tool.Functions) {
		return fmt.Errorf("the function '%s' referenced in input '%s' of function '%s' in tool '%s' was not found",
			funcName, input.Name, function.Name, tool.Name)
	}

	return nil
}

// validateOnError validates onError configuration
func validateOnError(onError *OnError, inputName string, function Function, tool Tool, configEnvVars []EnvVar) error {
	functionName := function.Name
	toolName := tool.Name

	// Define allowed fields for onError
	allowedOnErrorFields := map[string]bool{
		"strategy": true,
		"message":  true,
		"with":     true,
		"call":     true,
	}

	// Check for any invalid fields in onError
	onErrorMap := make(map[string]interface{})
	onErrorBytes, _ := yaml.Marshal(onError)
	yaml.Unmarshal(onErrorBytes, &onErrorMap)

	for field := range onErrorMap {
		if !allowedOnErrorFields[field] {
			return fmt.Errorf("input '%s' in function '%s' of tool '%s' has invalid field '%s' in onError. Allowed fields are: strategy, message, with, call",
				inputName, functionName, toolName, field)
		}
	}

	// Validate Call if present
	if onError.Call != nil {
		callName := onError.Call.Name

		// Check if function exists
		isSysFunc := systemFunctions[callName]
		funcExists := functionExists(callName, tool.Functions)

		if !isSysFunc && !funcExists {
			return fmt.Errorf("input '%s' in function '%s' references non-existent function '%s' in onError.call",
				inputName, functionName, callName)
		}

		// Validate params if present
		if len(onError.Call.Params) > 0 {
			// System functions cannot have params
			if isSysFunc {
				return fmt.Errorf("input '%s' in function '%s' cannot pass params to system function '%s' in onError.call",
					inputName, functionName, callName)
			}

			// Find the target function and validate params
			targetFunc := findFunctionByName(callName, tool.Functions)
			if targetFunc == nil {
				return fmt.Errorf("target function '%s' not found for params validation in onError.call", callName)
			}

			if err := validateFunctionCallParams(*onError.Call, *targetFunc, function, tool, configEnvVars, "onError.call"); err != nil {
				return fmt.Errorf("input '%s' in function '%s': %w", inputName, functionName, err)
			}
		}
	}

	// Validate strategy
	validStrategies := map[string]bool{
		OnErrorRequestUserInput:          true,
		OnErrorRequestN1Support:          true,
		OnErrorRequestN2Support:          true,
		OnErrorRequestN3Support:          true,
		OnErrorRequestApplicationSupport: true,
		OnErrorSearch:                    true,
		OnErrorInference:                 true,
	}

	if !validStrategies[onError.Strategy] {
		return fmt.Errorf("input '%s' in function '%s' of tool '%s' has invalid onError strategy '%s'",
			inputName, functionName, toolName, onError.Strategy)
	}

	// Note: onError.Message is now optional - if empty, the system will auto-generate
	// a message from the extraction rationale (for origin: chat and origin: inference)

	// Validate 'with' field if present
	if onError.With != nil {
		// 'with' is only allowed with requestUserInput strategy
		if onError.Strategy != OnErrorRequestUserInput {
			return fmt.Errorf("input '%s' in function '%s' of tool '%s' has 'with' field in onError but strategy is not 'requestUserInput'",
				inputName, functionName, toolName)
		}

		// Define allowed fields for with inside onError
		allowedWithFields := map[string]bool{
			"oneOf":    true,
			"manyOf":   true,
			"notOneOf": true,
			"ttl":      true,
		}

		// Check for any invalid fields in with
		withMap := make(map[string]interface{})
		withBytes, _ := yaml.Marshal(onError.With)
		yaml.Unmarshal(withBytes, &withMap)

		for field := range withMap {
			if !allowedWithFields[field] {
				return fmt.Errorf("input '%s' in function '%s' of tool '%s' has invalid field '%s' in onError.with. Allowed fields are: oneOf, manyOf, notOneOf, ttl",
					inputName, functionName, toolName, field)
			}
		}
	}

	return nil
}

// validateInputRunOnlyIf validates runOnlyIf at the input level
// Input-level runOnlyIf only supports deterministic evaluation (no inference/condition)
// and can only reference inputs defined earlier in the list (order matters)
func validateInputRunOnlyIf(runOnlyIf interface{}, input Input, inputIndex int, allInputs []Input, function Function, tool Tool) error {
	// Parse the runOnlyIf
	runOnlyIfObj, err := ParseRunOnlyIf(runOnlyIf)
	if err != nil {
		return fmt.Errorf("input '%s' in function '%s' of tool '%s' has invalid runOnlyIf: %w",
			input.Name, function.Name, tool.Name, err)
	}

	if runOnlyIfObj == nil {
		return nil
	}

	// Enforce deterministic-only: input-level runOnlyIf cannot use condition or inference
	if runOnlyIfObj.Condition != "" {
		return fmt.Errorf("input '%s' in function '%s' of tool '%s' has runOnlyIf with 'condition' field. Input-level runOnlyIf only supports deterministic evaluation. Use 'deterministic' field instead",
			input.Name, function.Name, tool.Name)
	}

	if runOnlyIfObj.Inference != nil {
		return fmt.Errorf("input '%s' in function '%s' of tool '%s' has runOnlyIf with 'inference' field. Input-level runOnlyIf only supports deterministic evaluation. Use 'deterministic' field instead",
			input.Name, function.Name, tool.Name)
	}

	// If no deterministic expression, nothing more to validate
	if runOnlyIfObj.Deterministic == "" {
		return fmt.Errorf("input '%s' in function '%s' of tool '%s' has empty runOnlyIf. Must specify 'deterministic' field",
			input.Name, function.Name, tool.Name)
	}

	// Build set of input names that appear BEFORE the current input
	previousInputNames := make(map[string]bool)
	for i := 0; i < inputIndex; i++ {
		previousInputNames[allInputs[i].Name] = true
	}

	// Build set of all input names (for detecting forward references)
	allInputNames := make(map[string]bool)
	for _, inp := range allInputs {
		allInputNames[inp.Name] = true
	}

	// Build set of dependency function names from transitive dependencies
	// (same logic as runtime uses in evaluateInputRunOnlyIf)
	allTransitiveDeps := BuildTransitiveDependencies(tool.Functions)
	transitiveDepsForFunc := allTransitiveDeps[function.Name]

	// Extract variable references from deterministic expression
	refs := ExtractFunctionReferences(runOnlyIfObj.Deterministic)

	for _, ref := range refs {
		// Check if it's a forward reference (input defined AFTER this input)
		if allInputNames[ref] && !previousInputNames[ref] {
			return fmt.Errorf("input '%s' in function '%s' of tool '%s' has runOnlyIf that references '$%s' which is defined later in the input list. runOnlyIf can only reference inputs defined earlier",
				input.Name, function.Name, tool.Name, ref)
		}

		// Check if it's a valid reference (previous input, needs function, or system variable)
		isPreviousInput := previousInputNames[ref]
		isNeedsFunction := transitiveDepsForFunc[ref]
		isSystemVar := IsSystemVariable(ref)

		if !isPreviousInput && !isNeedsFunction && !isSystemVar {
			return fmt.Errorf("input '%s' in function '%s' of tool '%s' has runOnlyIf that references unknown variable '$%s'. Must be a previous input name, a function in 'needs', or a system variable",
				input.Name, function.Name, tool.Name, ref)
		}
	}

	return nil
}

// validateSuccessCriteria validates the successCriteria field of an input
func validateSuccessCriteria(successCriteria interface{}, input Input, function Function, tool Tool) error {
	// Parse the successCriteria to get the object
	successCriteriaObj, err := ParseSuccessCriteria(successCriteria)
	if err != nil {
		return fmt.Errorf("invalid successCriteria: %w", err)
	}

	if successCriteriaObj == nil {
		return nil // No successCriteria specified
	}

	if successCriteriaObj.Condition == "" {
		return fmt.Errorf("successCriteria must have a non-empty condition")
	}

	// Validate the From field if present
	if len(successCriteriaObj.From) > 0 {
		// Build transitive dependency graph
		transitiveDeps := buildTransitiveDependencies(tool.Functions)

		// Check each function in the From list
		for _, fromFunc := range successCriteriaObj.From {
			// Special case: "scratchpad" is always valid - it's a system-provided filter
			// for accessing accumulated workflow context (extraInfoRequested + done rationale)
			if fromFunc == FilterScratchpad {
				continue
			}

			// Check for self-reference
			if fromFunc == function.Name {
				return fmt.Errorf("function cannot reference itself in successCriteria.from")
			}

			// Check if fromFunc is transitively reachable from current function
			if !transitiveDeps[function.Name][fromFunc] {
				return fmt.Errorf("function '%s' in successCriteria.from is not transitively reachable through the dependency chain of function '%s'", fromFunc, function.Name)
			}
		}
	}

	// Validate allowedSystemFunctions if present
	if len(successCriteriaObj.AllowedSystemFunctions) > 0 {
		validSystemFunctions := map[string]bool{
			SystemFunctionAskConversationHistory:    true,
			SystemFunctionAskKnowledgeBase:          true,
			SystemFunctionAskToContext:              true,
			SystemFunctionDoDeepWebResearch:         true,
			SystemFunctionDoSimpleWebSearch:         true,
			SystemFunctionGetWeekdayFromDate:        true,
			SystemFunctionQueryMemories:             true,
			SystemFunctionFetchWebContent:           true,
			SystemFunctionQueryCustomerServiceChats: true,
			SystemFunctionAnalyzeImage:              true,
			SystemFunctionSearchCodebase:            true,
			SystemFunctionQueryDocuments:            true,
		}

		for _, funcName := range successCriteriaObj.AllowedSystemFunctions {
			if !validSystemFunctions[funcName] {
				return fmt.Errorf("invalid system function '%s' in successCriteria.allowedSystemFunctions. Valid options are: %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s",
					funcName,
					SystemFunctionAskConversationHistory,
					SystemFunctionAskKnowledgeBase,
					SystemFunctionAskToContext,
					SystemFunctionDoDeepWebResearch,
					SystemFunctionDoSimpleWebSearch,
					SystemFunctionGetWeekdayFromDate,
					SystemFunctionQueryMemories,
					SystemFunctionFetchWebContent,
					SystemFunctionQueryCustomerServiceChats,
					SystemFunctionAnalyzeImage,
					SystemFunctionSearchCodebase,
					SystemFunctionQueryDocuments)
			}
		}

		// Validate queryCustomerServiceChats requires clientIds
		hasQueryCustomerServiceChats := false
		for _, funcName := range successCriteriaObj.AllowedSystemFunctions {
			if funcName == SystemFunctionQueryCustomerServiceChats {
				hasQueryCustomerServiceChats = true
				break
			}
		}
		if hasQueryCustomerServiceChats && successCriteriaObj.ClientIds == "" {
			return fmt.Errorf("allowedSystemFunctions includes '%s' but 'clientIds' field is not specified. "+
				"You must provide a clientIds variable reference (e.g., clientIds: \"$targetClientIds\")",
				SystemFunctionQueryCustomerServiceChats)
		}

		// Validate searchCodebase requires codebaseDirs
		hasSearchCodebase := false
		for _, funcName := range successCriteriaObj.AllowedSystemFunctions {
			if funcName == SystemFunctionSearchCodebase {
				hasSearchCodebase = true
				break
			}
		}
		if hasSearchCodebase && successCriteriaObj.CodebaseDirs == "" {
			return fmt.Errorf("allowedSystemFunctions includes '%s' but 'codebaseDirs' field is not specified. "+
				"You must provide a codebaseDirs variable reference (e.g., codebaseDirs: \"$getProjectRepos\")",
				SystemFunctionSearchCodebase)
		}

		// If 'from' is specified AND allowedSystemFunctions is restricted,
		// then askToContext MUST be in the allowed list because it's required to query function outputs
		if len(successCriteriaObj.From) > 0 {
			hasAskToContext := false
			for _, funcName := range successCriteriaObj.AllowedSystemFunctions {
				if funcName == SystemFunctionAskToContext {
					hasAskToContext = true
					break
				}
			}
			if !hasAskToContext {
				return fmt.Errorf("successCriteria.allowedSystemFunctions must include '%s' when 'from' field is specified, as it is required to query the context of other functions", SystemFunctionAskToContext)
			}
		}
	}

	return nil
}

// validateOnErrorStrategy validates the OnError strategy
func validateOnErrorStrategy(onError *OnError) error {
	if onError.Strategy == "" {
		return fmt.Errorf("onError must have a strategy")
	}

	// Validate the strategy is one of the valid ones
	validStrategies := map[string]bool{
		OnErrorRequestUserInput:          true,
		OnErrorRequestN1Support:          true,
		OnErrorRequestN2Support:          true,
		OnErrorRequestN3Support:          true,
		OnErrorRequestApplicationSupport: true,
		OnErrorSearch:                    true,
		OnErrorInference:                 true,
	}

	if !validStrategies[onError.Strategy] {
		return fmt.Errorf("invalid onError strategy: %s", onError.Strategy)
	}

	return nil
}

// validateOutput validates function output configuration
func validateOutput(output *Output, function Function, toolName string, configEnvVars []EnvVar) error {
	validOutputTypes := map[string]bool{
		StepOutputObject:       true,
		StepOutputListOfObject: true,
		StepOutputListString:   true,
		StepOutputListNumber:   true,
		FieldTypeString:        true,
		OutputTypeFile:         true,
		OutputTypeListFile:     true,
	}

	if !validOutputTypes[output.Type] {
		return fmt.Errorf("function '%s' in tool '%s' has invalid output type '%s'",
			function.Name, toolName, output.Type)
	}

	// Validate file output types
	if output.Type == OutputTypeFile || output.Type == OutputTypeListFile {
		// Validate retention is non-negative
		if output.Retention < 0 {
			return fmt.Errorf("function '%s' in tool '%s' has negative retention value",
				function.Name, toolName)
		}

		// File output types with PDF operation are valid without explicit upload setting
		// The upload defaults to true for PDF operations
		if function.Operation != OperationPDF && !output.Upload {
			// Warn but don't error - non-uploaded files are still valid (will be base64 accessible)
		}

		return nil
	}

	// For list[string] and list[number], fields should be empty
	if (output.Type == StepOutputListString || output.Type == StepOutputListNumber) && len(output.Fields) > 0 {
		return fmt.Errorf("function '%s' in tool '%s' with output type '%s' should not have fields",
			function.Name, toolName, output.Type)
	}

	// For object and list[object], fields are required
	if (output.Type == StepOutputObject || output.Type == StepOutputListOfObject) && len(output.Fields) == 0 {
		return fmt.Errorf("function '%s' in tool '%s' output must have at least one field",
			function.Name, toolName)
	}

	if output.Type == FieldTypeString {
		if output.Value == "" {
			return fmt.Errorf("function '%s' in tool '%s' with output type 'string' must have a 'value' property",
				function.Name, toolName)
		}
		if len(output.Fields) > 0 {
			return fmt.Errorf("function '%s' in tool '%s' with output type 'string' should not have fields",
				function.Name, toolName)
		}

		// Validate variables in the string output value
		if err := extractAndValidateAllVariables(output.Value, function, toolName, configEnvVars, nil); err != nil {
			return fmt.Errorf("in output value: %w", err)
		}

		// Validate result[X] references in output value have corresponding resultIndex in steps
		if err := validateOutputResultReferences(output.Value, function, toolName); err != nil {
			return err
		}

		// Validate expression functions in output value
		if err := ValidateExpressionFunctions(output.Value); err != nil {
			return fmt.Errorf("function '%s' in tool '%s' output.value: %w", function.Name, toolName, err)
		}

		return nil
	}

	// Validate fields if we have them
	for _, field := range output.Fields {
		if err := validateOutputField(field, function.Name, toolName, 0); err != nil {
			return err
		}
	}

	return nil
}

// validateOutputResultReferences checks if result[X] references in output value have corresponding resultIndex in steps
func validateOutputResultReferences(outputValue string, function Function, toolName string) error {
	// Build map of valid result indices from steps
	validIndices := make(map[int]string)
	for _, step := range function.Steps {
		if step.ResultIndex != 0 {
			validIndices[step.ResultIndex] = step.Name
		}
	}

	// Find all result[X] references in the output value
	resultRefRegex := regexp.MustCompile(`result\[(\d+)\]`)
	matches := resultRefRegex.FindAllStringSubmatch(outputValue, -1)

	for _, match := range matches {
		if len(match) >= 2 {
			indexStr := match[1]
			index, err := strconv.Atoi(indexStr)
			if err != nil {
				continue // Not a valid index, skip
			}

			// Ensure the referenced result exists
			if _, exists := validIndices[index]; !exists {
				return fmt.Errorf("function '%s' in tool '%s' output references result[%d] but no step has resultIndex: %d. "+
					"Add 'resultIndex: %d' to the step that should provide this result",
					function.Name, toolName, index, index, index)
			}
		}
	}

	return nil
}

func validateOutputField(field OutputField, functionName, toolName string, depth int) error {
	// Check for empty name and value in complex fields
	if field.Name == "" && field.Value == "" {
		return fmt.Errorf("field in function '%s' of tool '%s' must have either a name or value",
			functionName, toolName)
	}

	// Max nesting depth check
	if depth > 2 {
		fieldName := field.Name
		if fieldName == "" {
			fieldName = field.Value
		}
		return fmt.Errorf("field '%s' in function '%s' of tool '%s' exceeds maximum nesting depth",
			fieldName, functionName, toolName)
	}

	// Type validation if specified
	if field.Type != "" {
		validFieldTypes := map[string]bool{
			StepOutputObject:       true,
			StepOutputListOfObject: true,
			StepOutputListString:   true,
			StepOutputListNumber:   true,
			FieldTypeString:        true,
			FieldTypeNumber:        true,
			FieldTypeBoolean:       true,
		}

		if !validFieldTypes[field.Type] {
			fieldName := field.Name
			if fieldName == "" {
				fieldName = field.Value
			}
			return fmt.Errorf("field '%s' in function '%s' of tool '%s' has invalid type '%s'",
				fieldName, functionName, toolName, field.Type)
		}
	}

	// Validate nested fields for complex types
	if field.Type == StepOutputObject || field.Type == StepOutputListOfObject {
		if len(field.Fields) == 0 {
			fieldName := field.Name
			if fieldName == "" {
				fieldName = field.Value
			}
			return fmt.Errorf("complex field '%s' in function '%s' of tool '%s' must define nested fields",
				fieldName, functionName, toolName)
		}

		for _, nestedField := range field.Fields {
			if err := validateOutputField(nestedField, functionName, toolName, depth+1); err != nil {
				return err
			}
		}
	}

	return nil
}

// checkFunctionDependencyCycles detects circular dependencies between functions
func checkFunctionDependencyCycles(functions []Function) error {
	// Build dependency graph
	dependencyGraph := make(map[string][]string)

	// Collect all dependencies
	for _, function := range functions {
		dependencyGraph[function.Name] = []string{}

		for _, input := range function.Input {
			if input.Origin == DataOriginFunction && input.From != "" {
				dependencyGraph[function.Name] = append(dependencyGraph[function.Name], input.From)
			}
		}
		for _, needed := range function.Needs {
			if !systemFunctions[needed.Name] {
				dependencyGraph[function.Name] = append(dependencyGraph[function.Name], needed.Name)
			}
		}
		for _, successFunc := range function.OnSuccess {
			// Only add if it's not a system function
			if !systemFunctions[successFunc.Name] {
				dependencyGraph[function.Name] = append(dependencyGraph[function.Name], successFunc.Name)
			}
		}
		for _, skipFunc := range function.OnSkip {
			// Only add if it's not a system function
			if !systemFunctions[skipFunc.Name] {
				dependencyGraph[function.Name] = append(dependencyGraph[function.Name], skipFunc.Name)
			}
		}
	}

	for functionName := range dependencyGraph {
		visited := make(map[string]bool)
		path := make(map[string]bool)

		if hasCycle(functionName, dependencyGraph, visited, path) {
			return fmt.Errorf("circular dependency detected involving function '%s'", functionName)
		}
	}

	return nil
}

// hasCycle is a DFS helper to detect cycles in the dependency graph
func hasCycle(current string, graph map[string][]string, visited, path map[string]bool) bool {
	// If we've already checked this node completely, we can skip
	if visited[current] {
		return false
	}

	// If we find this node again in the current path, we have a cycle
	if path[current] {
		return true
	}

	// Mark as part of current path
	path[current] = true

	// Visit all dependencies
	for _, dependency := range graph[current] {
		if hasCycle(dependency, graph, visited, path) {
			return true
		}
	}

	// Remove from current path as we backtrack
	path[current] = false

	// Mark as fully visited
	visited[current] = true

	return false
}

// checkStaticHookCycles detects potential circular dependencies in statically-defined registerHook calls.
// This is a best-effort validation - it can only detect cycles when both functionName and callbackFunction
// are static strings (not variables like $varName). Returns warnings, not errors, since we can't catch all cases.
func checkStaticHookCycles(tools []Tool) []string {
	var warnings []string

	// hookEdge represents a hook registration: functionName -> callbackFunction
	type hookEdge struct {
		toolName         string
		functionName     string
		callbackFunction string
		triggerType      string
		location         string // Where the registerHook was found (for better error messages)
	}

	var edges []hookEdge

	// Helper to extract static string value from params
	getStaticParam := func(params map[string]interface{}, key string) string {
		if params == nil {
			return ""
		}
		if val, ok := params[key]; ok {
			if strVal, ok := val.(string); ok {
				// Skip if it's a variable (starts with $)
				if strings.HasPrefix(strVal, "$") {
					return ""
				}
				return strVal
			}
		}
		return ""
	}

	// Helper to check if a name/params pair is a registerHook and extract edge
	checkAndExtractHook := func(name string, params map[string]interface{}, location string) {
		if name == SysFuncRegisterHook {
			toolName := getStaticParam(params, "toolName")
			funcName := getStaticParam(params, "functionName")
			callbackFunc := getStaticParam(params, "callbackFunction")
			triggerType := getStaticParam(params, "triggerType")

			// Only add edge if both are static (non-empty after variable check)
			if funcName != "" && callbackFunc != "" {
				edges = append(edges, hookEdge{
					toolName:         toolName,
					functionName:     funcName,
					callbackFunction: callbackFunc,
					triggerType:      triggerType,
					location:         location,
				})
			}
		}
	}

	// Scan all tools and functions for registerHook calls
	for _, tool := range tools {
		for _, function := range tool.Functions {
			funcLocation := fmt.Sprintf("%s.%s", tool.Name, function.Name)

			// Check needs ([]NeedItem)
			for _, need := range function.Needs {
				checkAndExtractHook(need.Name, need.Params, funcLocation+" (needs)")
			}

			// Check onSuccess ([]FunctionCall)
			for _, call := range function.OnSuccess {
				checkAndExtractHook(call.Name, call.Params, funcLocation+" (onSuccess)")
			}

			// Check onFailure ([]FunctionCall)
			for _, call := range function.OnFailure {
				checkAndExtractHook(call.Name, call.Params, funcLocation+" (onFailure)")
			}
		}
	}

	if len(edges) == 0 {
		return nil // No static hooks found
	}

	// Build dependency graph: functionName -> [callbackFunctions]
	// Key format: "toolName.functionName" or just "functionName" if toolName is empty
	graph := make(map[string][]string)
	edgeLocations := make(map[string]string) // Track where each edge was defined

	for _, edge := range edges {
		var sourceKey, targetKey string

		if edge.toolName != "" {
			sourceKey = edge.toolName + "." + edge.functionName
		} else {
			sourceKey = edge.functionName
		}

		// For callback, check if it contains a dot (tool.function format)
		if strings.Contains(edge.callbackFunction, ".") {
			targetKey = edge.callbackFunction
		} else if edge.toolName != "" {
			// Assume same tool if no dot notation
			targetKey = edge.toolName + "." + edge.callbackFunction
		} else {
			targetKey = edge.callbackFunction
		}

		graph[sourceKey] = append(graph[sourceKey], targetKey)
		edgeLocations[sourceKey+"->"+targetKey] = edge.location
	}

	// Detect cycles using DFS
	// Track reported cycles to avoid duplicates (A→B→A and B→A→B are the same cycle)
	reportedCycles := make(map[string]bool)

	for startNode := range graph {
		visited := make(map[string]bool)
		path := make(map[string]bool)
		pathList := []string{} // Track actual path for warning message

		if detectHookCycle(startNode, graph, visited, path, &pathList) {
			// Extract cycle nodes (the cycle starts where we found the back-edge)
			cycleStart := pathList[len(pathList)-1]
			var cycleNodes []string
			inCycle := false
			for _, node := range pathList {
				if node == cycleStart && !inCycle {
					inCycle = true
				}
				if inCycle {
					cycleNodes = append(cycleNodes, node)
				}
			}
			// Remove the duplicate cycleStart at the end (it's added by detectHookCycle)
			if len(cycleNodes) > 1 && cycleNodes[len(cycleNodes)-1] == cycleStart {
				cycleNodes = cycleNodes[:len(cycleNodes)-1]
			}

			// Create canonical cycle key by sorting unique nodes (to dedupe A→B vs B→A)
			uniqueNodes := make(map[string]bool)
			for _, node := range cycleNodes {
				uniqueNodes[node] = true
			}
			var sortedNodes []string
			for node := range uniqueNodes {
				sortedNodes = append(sortedNodes, node)
			}
			sort.Strings(sortedNodes)
			cycleKey := strings.Join(sortedNodes, "|")

			// Skip if we've already reported this cycle
			if reportedCycles[cycleKey] {
				continue
			}
			reportedCycles[cycleKey] = true

			// Build cycle path string for the warning
			cyclePathStr := strings.Join(cycleNodes, " → ") + " → " + cycleStart + " (cycle)"

			warning := fmt.Sprintf("Potential hook cycle detected: %s. "+
				"This could cause infinite recursion at runtime. "+
				"Consider using different trigger types or restructuring your hooks.",
				cyclePathStr)
			warnings = append(warnings, warning)
		}
	}

	return warnings
}

// detectHookCycle is a DFS helper to detect cycles in the hook dependency graph
func detectHookCycle(current string, graph map[string][]string, visited, path map[string]bool, pathList *[]string) bool {
	if visited[current] {
		return false
	}

	if path[current] {
		*pathList = append(*pathList, current)
		return true
	}

	path[current] = true
	*pathList = append(*pathList, current)

	for _, target := range graph[current] {
		if detectHookCycle(target, graph, visited, path, pathList) {
			return true
		}
	}

	path[current] = false
	*pathList = (*pathList)[:len(*pathList)-1]
	visited[current] = true

	return false
}

// Helper functions

// isValidSemanticVersion checks if a version string follows semantic versioning format
func isValidSemanticVersion(version string) bool {
	semverRegex := regexp.MustCompile(`^\d+\.\d+\.\d+$`)
	return semverRegex.MatchString(version)
}

// isValidEnvVarName checks if an environment variable name is valid
func isValidEnvVarName(name string) bool {
	// Valid env var names: start with letter or underscore, followed by letters, numbers, or underscores
	envVarRegex := regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)
	return envVarRegex.MatchString(name)
}

// isValidCronExpression validates a cron expression
// Requires exactly 6 fields (with seconds): second minute hour day-of-month month day-of-week
func isValidCronExpression(cron string) bool {
	fields := strings.Fields(cron)
	return len(fields) == 6
}

// isPrivateFunction checks if a function name follows private function naming convention
func isPrivateFunction(name string) bool {
	// Private functions start with a lowercase letter
	if len(name) == 0 {
		return false
	}
	return name[0] >= 'a' && name[0] <= 'z'
}

// isWorkflowVariableReference checks if a value is a workflow variable reference (e.g., $companyName).
// Workflow variables start with $ followed by a lowercase letter.
// Environment variables (e.g., $API_KEY) start with uppercase and are NOT considered workflow variables.
func isWorkflowVariableReference(value string) bool {
	if len(value) < 2 || value[0] != '$' {
		return false
	}
	// Check if the first character after $ is lowercase (workflow param)
	return value[1] >= 'a' && value[1] <= 'z'
}

func functionExists(name string, functions []Function) bool {
	for _, function := range functions {
		if function.Name == name {
			return true
		}
	}
	return false
}

// validateStep validates an individual step based on the function operation type
func validateStep(step Step, functionName, toolName, operation string) error {
	if step.Name == "" {
		return fmt.Errorf("a step in function '%s' of tool '%s' has an empty name", functionName, toolName)
	}
	if step.Action == "" {
		return fmt.Errorf("step '%s' in function '%s' of tool '%s' has an empty action", step.Name, functionName, toolName)
	}

	if step.ResultIndex != 0 {
		if step.ResultIndex < 0 {
			return fmt.Errorf("step '%s' in function '%s' of tool '%s' has negative resultIndex %d",
				step.Name, functionName, toolName, step.ResultIndex)
		}
	}

	// Validate isAuthentication field
	if step.IsAuthentication && operation != OperationAPI {
		return fmt.Errorf("step '%s' in function '%s' of tool '%s' has 'isAuthentication' set to true, but this feature is only applicable to 'api_call' operations",
			step.Name, functionName, toolName)
	}

	// Check for common misspelling of 'foreach' as 'forEach'
	if step.ForEachCamelCase != nil {
		return fmt.Errorf("step '%s' in function '%s' of tool '%s' uses 'forEach' (camelCase). The correct field name is 'foreach' (lowercase). Please use 'foreach:' instead of 'forEach:'",
			step.Name, functionName, toolName)
	}

	// Validate foreach field
	if step.ForEach != nil {
		if err := validateForEach(step.ForEach, step.Name, functionName, toolName); err != nil {
			return err
		}
	}

	// Validate runOnlyIf field (deterministic only, for api_call, terminal, db operations)
	if step.RunOnlyIf != nil {
		if err := validateStepRunOnlyIf(step.RunOnlyIf, step.Name, functionName, toolName, operation); err != nil {
			return err
		}
	}

	switch operation {
	case OperationWeb:
		return validateWebStep(step, functionName, toolName)
	case OperationDesktop:
		return validateDesktopStep(step, functionName, toolName)
	case OperationAPI:
		return validateAPIStep(step, functionName, toolName)
	case OperationDB:
		return validateDBStep(step, functionName, toolName)
	case OperationTerminal:
		return validateTerminalStep(step, functionName, toolName)
	case OperationCode:
		return validateCodeStep(step, functionName, toolName)
	case OperationGDrive:
		return validateGDriveStep(step, functionName, toolName)
	case OperationInitiateWorkflow:
		// initiate_workflow validation is done in validateInitiateWorkflowOperation
		return nil
	default:
		return fmt.Errorf("unsupported operation '%s' for step '%s' in function '%s' of tool '%s'",
			operation, step.Name, functionName, toolName)
	}
}

// validateForEach validates foreach configuration
func validateForEach(forEach *ForEach, stepName, functionName, toolName string) error {
	if forEach.Items == "" {
		return fmt.Errorf("step '%s' in function '%s' of tool '%s' has foreach with empty items field",
			stepName, functionName, toolName)
	}

	if forEach.Separator == "" {
		forEach.Separator = DefaultForEachSeparator
	}

	if forEach.IndexVar == "" {
		forEach.IndexVar = DefaultForEachIndexVar
	}

	if forEach.ItemVar == "" {
		forEach.ItemVar = DefaultForEachItemVar
	}

	if forEach.IndexVar == forEach.ItemVar {
		return fmt.Errorf("step '%s' in function '%s' of tool '%s' has foreach with same indexVar and itemVar '%s'",
			stepName, functionName, toolName, forEach.IndexVar)
	}

	varRegex := regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)
	if !varRegex.MatchString(forEach.IndexVar) {
		return fmt.Errorf("step '%s' in function '%s' of tool '%s' has foreach with invalid indexVar '%s'",
			stepName, functionName, toolName, forEach.IndexVar)
	}

	if !varRegex.MatchString(forEach.ItemVar) {
		return fmt.Errorf("step '%s' in function '%s' of tool '%s' has foreach with invalid itemVar '%s'",
			stepName, functionName, toolName, forEach.ItemVar)
	}

	// Validate breakIf if present
	if forEach.BreakIf != nil {
		strCond, objCond, err := ParseBreakIf(forEach.BreakIf)
		if err != nil {
			return fmt.Errorf("step '%s' in function '%s' of tool '%s' has invalid foreach breakIf: %w",
				stepName, functionName, toolName, err)
		}

		// If it's a string (deterministic), validate syntax and variables
		if strCond != "" {
			if err := validateDeterministicBreakCondition(strCond, forEach.ItemVar, forEach.IndexVar, stepName, functionName, toolName); err != nil {
				return err
			}
		} else if objCond != nil {
			// Inference break condition - validate it has a condition
			if objCond.Condition == "" {
				return fmt.Errorf("step '%s' in function '%s' of tool '%s' has foreach breakIf object with empty condition",
					stepName, functionName, toolName)
			}
		} else {
			return fmt.Errorf("step '%s' in function '%s' of tool '%s' has invalid foreach breakIf: must be string or object",
				stepName, functionName, toolName)
		}
	}

	// Validate waitFor if present
	if forEach.WaitFor != nil {
		if forEach.WaitFor.Name == "" {
			return fmt.Errorf("step '%s' in function '%s' of tool '%s' has foreach waitFor with empty name field",
				stepName, functionName, toolName)
		}

		// Set default values
		if forEach.WaitFor.PollIntervalSeconds <= 0 {
			forEach.WaitFor.PollIntervalSeconds = DefaultForEachPollIntervalSeconds
		}
		if forEach.WaitFor.MaxWaitingSeconds <= 0 {
			forEach.WaitFor.MaxWaitingSeconds = DefaultForEachMaxWaitingSeconds
		}

		// Validate maxWaitingSeconds > pollIntervalSeconds
		if forEach.WaitFor.MaxWaitingSeconds <= forEach.WaitFor.PollIntervalSeconds {
			return fmt.Errorf("step '%s' in function '%s' of tool '%s' has foreach waitFor with maxWaitingSeconds (%d) <= pollIntervalSeconds (%d)",
				stepName, functionName, toolName, forEach.WaitFor.MaxWaitingSeconds, forEach.WaitFor.PollIntervalSeconds)
		}
	}

	// Validate shouldSkip conditions if present (supports both single and array format)
	conditions := forEach.GetShouldSkipConditions()
	for i, cond := range conditions {
		if cond.Name == "" {
			if len(conditions) > 1 {
				return fmt.Errorf("step '%s' in function '%s' of tool '%s' has foreach shouldSkip[%d] with empty name field",
					stepName, functionName, toolName, i)
			}
			return fmt.Errorf("step '%s' in function '%s' of tool '%s' has foreach shouldSkip with empty name field",
				stepName, functionName, toolName)
		}
	}

	return nil
}

// validateStepRunOnlyIf validates the runOnlyIf condition for a step
// Step-level runOnlyIf only supports deterministic mode (no inference/LLM)
// and is only allowed for api_call, terminal, and db operations
func validateStepRunOnlyIf(runOnlyIf interface{}, stepName, functionName, toolName, operation string) error {
	// Validate operation type - only api_call, terminal, db, and code are supported
	if operation != OperationAPI && operation != OperationTerminal && operation != OperationDB && operation != OperationCode && operation != OperationGDrive {
		return fmt.Errorf("step '%s' in function '%s' of tool '%s' has runOnlyIf but operation '%s' is not supported - only 'api_call', 'terminal', 'db', 'code', and 'gdrive' operations support step-level runOnlyIf",
			stepName, functionName, toolName, operation)
	}

	// Parse the runOnlyIf
	runOnlyIfObj, err := ParseRunOnlyIf(runOnlyIf)
	if err != nil {
		return fmt.Errorf("step '%s' in function '%s' of tool '%s' has invalid runOnlyIf: %w",
			stepName, functionName, toolName, err)
	}

	if runOnlyIfObj == nil {
		return nil // Empty runOnlyIf is allowed
	}

	// Step-level runOnlyIf only supports deterministic mode
	if runOnlyIfObj.Condition != "" {
		return fmt.Errorf("step '%s' in function '%s' of tool '%s' has runOnlyIf with 'condition' field - step-level runOnlyIf only supports 'deterministic' mode, not inference. Use 'runOnlyIf: { deterministic: \"your_expression\" }' instead",
			stepName, functionName, toolName)
	}

	if runOnlyIfObj.Inference != nil {
		return fmt.Errorf("step '%s' in function '%s' of tool '%s' has runOnlyIf with 'inference' field - step-level runOnlyIf only supports 'deterministic' mode, not inference. Use 'runOnlyIf: { deterministic: \"your_expression\" }' instead",
			stepName, functionName, toolName)
	}

	if runOnlyIfObj.Deterministic == "" {
		return fmt.Errorf("step '%s' in function '%s' of tool '%s' has runOnlyIf without 'deterministic' field - step-level runOnlyIf requires a deterministic expression. Use 'runOnlyIf: { deterministic: \"your_expression\" }'",
			stepName, functionName, toolName)
	}

	// Validate the deterministic expression syntax (basic syntax check)
	// The expression should contain at least one comparison operator
	expr := runOnlyIfObj.Deterministic
	if !containsComparisonOperator(expr) && !containsBuiltInFunction(expr) {
		return fmt.Errorf("step '%s' in function '%s' of tool '%s' has runOnlyIf.deterministic with invalid expression '%s' - must contain a comparison operator (==, !=, >, <, >=, <=) or built-in function (len(), isEmpty(), contains(), exists())",
			stepName, functionName, toolName, expr)
	}

	return nil
}

// containsComparisonOperator checks if the expression contains a comparison operator
func containsComparisonOperator(expr string) bool {
	operators := []string{"==", "!=", ">=", "<=", ">", "<"}
	for _, op := range operators {
		if strings.Contains(expr, op) {
			return true
		}
	}
	return false
}

// containsBuiltInFunction checks if the expression contains a built-in function
func containsBuiltInFunction(expr string) bool {
	functions := []string{"len(", "isEmpty(", "contains(", "exists("}
	for _, fn := range functions {
		if strings.Contains(expr, fn) {
			return true
		}
	}
	return false
}

// checkOutputReferencesToRunOnlyIfSteps checks if function outputs reference result[X]
// where step X has runOnlyIf. This is a warning because the result may be null if skipped.
func checkOutputReferencesToRunOnlyIfSteps(functions []Function) []string {
	var warnings []string
	resultRefRegex := regexp.MustCompile(`result\[(\d+)\]`)

	for _, function := range functions {
		if function.Output == nil || function.Output.Value == "" {
			continue
		}

		// Build map of steps with runOnlyIf indexed by resultIndex
		stepsWithRunOnlyIf := make(map[int]string) // resultIndex -> step name
		for _, step := range function.Steps {
			if step.ResultIndex != 0 && step.RunOnlyIf != nil {
				stepsWithRunOnlyIf[step.ResultIndex] = step.Name
			}
		}

		if len(stepsWithRunOnlyIf) == 0 {
			continue
		}

		// Check output value for result[X] references
		matches := resultRefRegex.FindAllStringSubmatch(function.Output.Value, -1)
		for _, match := range matches {
			if len(match) >= 2 {
				indexStr := match[1]
				index, err := strconv.Atoi(indexStr)
				if err != nil {
					continue
				}

				if stepName, exists := stepsWithRunOnlyIf[index]; exists {
					warnings = append(warnings, fmt.Sprintf(
						"function '%s' output references result[%d] from step '%s' which has runOnlyIf - "+
							"result may be null if step is skipped. Consider using coalesce() to handle null values. "+
							"See documentation for coalesce usage.",
						function.Name, index, stepName))
				}
			}
		}
	}

	return warnings
}

// validateWaitForFunctionExists validates that the waitFor function exists in the tool
func validateWaitForFunctionExists(waitFor *WaitFor, tool Tool, stepName, functionName string) error {
	if waitFor == nil || waitFor.Name == "" {
		return nil
	}

	// Check if the function exists in the tool
	for _, fn := range tool.Functions {
		if fn.Name == waitFor.Name {
			return nil
		}
	}

	return fmt.Errorf("step '%s' in function '%s' of tool '%s' has foreach waitFor referencing non-existent function '%s'",
		stepName, functionName, tool.Name, waitFor.Name)
}

// validateShouldSkipFunctionsExist validates that all shouldSkip functions exist in the tool
// Supports both single shouldSkip and array of shouldSkip conditions
// Also supports cross-tool references (e.g., utils_shared.shouldSkipIfContains)
func validateShouldSkipFunctionsExist(forEach *ForEach, tool Tool, stepName, functionName string) error {
	if forEach == nil {
		return nil
	}

	conditions := forEach.GetShouldSkipConditions()
	for i, cond := range conditions {
		if cond == nil || cond.Name == "" {
			continue
		}

		if !isFunctionAvailable(cond.Name, tool.Functions) {
			if len(conditions) > 1 {
				return fmt.Errorf("step '%s' in function '%s' of tool '%s' has foreach shouldSkip[%d] referencing non-existent function '%s'",
					stepName, functionName, tool.Name, i, cond.Name)
			}
			return fmt.Errorf("step '%s' in function '%s' of tool '%s' has foreach shouldSkip referencing non-existent function '%s'",
				stepName, functionName, tool.Name, cond.Name)
		}
	}

	return nil
}

// validateWaitForAndShouldSkipOutputs checks that functions used as waitFor or shouldSkip callbacks
// have output types that clearly return true/false. Emits warnings (not errors) to help developers
// avoid hard-to-debug issues where callbacks time out or silently fail.
// Accepted output types: "string" (should return "true"/"false") or "boolean".
func validateWaitForAndShouldSkipOutputs(tool Tool) []string {
	var warnings []string

	// Build function lookup
	funcMap := make(map[string]*Function)
	for i := range tool.Functions {
		funcMap[tool.Functions[i].Name] = &tool.Functions[i]
	}

	for _, function := range tool.Functions {
		for _, step := range function.Steps {
			if step.ForEach == nil {
				continue
			}

			// Check waitFor callback output
			if step.ForEach.WaitFor != nil && step.ForEach.WaitFor.Name != "" {
				callbackName := step.ForEach.WaitFor.Name
				if targetFunc, ok := funcMap[callbackName]; ok {
					if targetFunc.Output != nil && targetFunc.Output.Type != "" {
						outType := strings.ToLower(targetFunc.Output.Type)
						if outType != "string" && outType != "boolean" && outType != "bool" {
							warnings = append(warnings, fmt.Sprintf(
								"[%s] function '%s' is used as waitFor callback in '%s.%s' but has output type '%s'. "+
									"waitFor callbacks must return 'true' or 'false' (string or boolean). "+
									"Current output may cause timeouts.",
								tool.Name, callbackName, function.Name, step.Name, targetFunc.Output.Type))
						}
					}
				}
			}

			// Check shouldSkip callback outputs
			for _, cond := range step.ForEach.GetShouldSkipConditions() {
				if cond == nil || cond.Name == "" {
					continue
				}
				callbackName := cond.Name
				if targetFunc, ok := funcMap[callbackName]; ok {
					if targetFunc.Output != nil && targetFunc.Output.Type != "" {
						outType := strings.ToLower(targetFunc.Output.Type)
						if outType != "string" && outType != "boolean" && outType != "bool" {
							warnings = append(warnings, fmt.Sprintf(
								"[%s] function '%s' is used as shouldSkip callback in '%s.%s' but has output type '%s'. "+
									"shouldSkip callbacks must return 'true' or 'false' (string or boolean).",
								tool.Name, callbackName, function.Name, step.Name, targetFunc.Output.Type))
						}
					}
				}
			}
		}
	}

	return warnings
}

// validateDeterministicBreakCondition validates deterministic break condition syntax and variables
func validateDeterministicBreakCondition(condition, itemVar, indexVar, stepName, functionName, toolName string) error {
	// Basic syntax validation - must contain comparison operator
	validOperators := []string{"==", "!=", ">", "<", ">=", "<="}
	hasOperator := false
	for _, op := range validOperators {
		if strings.Contains(condition, op) {
			hasOperator = true
			break
		}
	}

	if !hasOperator {
		return fmt.Errorf("step '%s' in function '%s' of tool '%s' has foreach breakIf with invalid syntax: '%s'. Must contain a comparison operator (==, !=, >, <, >=, <=)",
			stepName, functionName, toolName, condition)
	}

	// Extract all variables used in the condition
	varRegex := regexp.MustCompile(`\$[a-zA-Z0-9_]+(?:\.[a-zA-Z0-9_\[\]]+)*`)
	variables := varRegex.FindAllString(condition, -1)

	if len(variables) == 0 {
		return fmt.Errorf("step '%s' in function '%s' of tool '%s' has foreach breakIf without any variables: '%s'",
			stepName, functionName, toolName, condition)
	}

	// Validate that variables reference the loop variables (item or index)
	validLoopVars := map[string]bool{
		"$" + itemVar:  true,
		"$" + indexVar: true,
	}

	for _, variable := range variables {
		// Extract base variable (before any dot or bracket)
		baseVar := variable
		if idx := strings.IndexAny(baseVar, ".["); idx > 0 {
			baseVar = baseVar[:idx]
		}

		// Check if it's a valid loop variable
		if !validLoopVars[baseVar] {
			return fmt.Errorf("step '%s' in function '%s' of tool '%s' has foreach breakIf using invalid variable '%s'. Only loop variables '$%s' and '$%s' are allowed",
				stepName, functionName, toolName, baseVar, itemVar, indexVar)
		}
	}

	return nil
}

// validateWebStep validates steps for web_browse operation
func validateWebStep(step Step, functionName, toolName string) error {
	// Define allowed fields for each action type
	allowedFields := map[string]map[string]bool{
		StepActionURL: {
			StepWithURL:           true,
			StepWithIncognitoMode: true,
		},
		StepActionExtractText: {
			StepWithFindBy:    true,
			StepWithFindValue: true,
			"limit":           true,
			"format":          true,
		},
		StepActionFill: {
			StepWithFillValue: true,
		},
		StepActionFindClick: {
			StepWithFindBy:    true,
			StepWithFindValue: true,
		},
		StepActionFindFillTab: {
			StepWithFindBy:    true,
			StepWithFindValue: true,
			StepWithFillValue: true,
		},
		StepActionFindFillReturn: {
			StepWithFindBy:    true,
			StepWithFindValue: true,
			StepWithFillValue: true,
		},
		StepActionSubmit: {}, // No fields required
	}

	// Validate that only allowed fields are present
	if step.With != nil {
		if allowed, ok := allowedFields[step.Action]; ok {
			if err := validateAllowedFields(step.With, allowed, step.Name, functionName, toolName); err != nil {
				return err
			}
		}
	}

	switch step.Action {
	case StepActionURL:
		if step.With == nil {
			return fmt.Errorf("step '%s' in function '%s' of tool '%s' with action 'open_url' must have a 'with' block",
				step.Name, functionName, toolName)
		}
		if _, ok := step.With[StepWithURL]; !ok {
			return fmt.Errorf("step '%s' in function '%s' of tool '%s' with action 'open_url' must provide a 'url' parameter",
				step.Name, functionName, toolName)
		}

		if step.With[StepWithIncognitoMode] != nil {
			if _, ok := step.With[StepWithIncognitoMode].(bool); !ok {
				step.With[StepWithIncognitoMode] = true // Default to true if not specified
			}
		}
		// incognitoMode is optional with default value true, no need to validate

	case StepActionExtractText:
		if step.Goal == "" {
			return fmt.Errorf("step '%s' in function '%s' of tool '%s' with action 'extract_text' must have a 'goal'",
				step.Name, functionName, toolName)
		}
		if step.With == nil {
			return fmt.Errorf("step '%s' in function '%s' of tool '%s' with action 'extract_text' must have a 'with' block",
				step.Name, functionName, toolName)
		}
		if err := validateFindStep(step, functionName, toolName); err != nil {
			return err
		}

	case StepActionFill:
		if step.With == nil {
			return fmt.Errorf("step '%s' in function '%s' of tool '%s' with action 'fill' must have a 'with' block",
				step.Name, functionName, toolName)
		}
		if _, ok := step.With[StepWithFillValue]; !ok {
			return fmt.Errorf("step '%s' in function '%s' of tool '%s' with action 'fill' must provide a 'fillValue' parameter",
				step.Name, functionName, toolName)
		}

	case StepActionFindClick, StepActionFindFillTab, StepActionFindFillReturn:
		if err := validateFindStep(step, functionName, toolName); err != nil {
			return err
		}

		// Additional validation for fill actions
		if step.Action == StepActionFindFillTab || step.Action == StepActionFindFillReturn {
			if _, ok := step.With[StepWithFillValue]; !ok {
				return fmt.Errorf("step '%s' in function '%s' of tool '%s' with action '%s' must provide a 'fillValue' parameter",
					step.Name, functionName, toolName, step.Action)
			}
		}

	case StepActionSubmit:
		// no additional validation needed for submit action

	default:
		return fmt.Errorf("unsupported action '%s' for web_browse in step '%s' in function '%s' of tool '%s'",
			step.Action, step.Name, functionName, toolName)
	}

	return nil
}

// validateDesktopStep validates steps for desktop_use operation
func validateDesktopStep(step Step, functionName, toolName string) error {
	allowedFields := map[string]map[string]bool{
		StepActionApp: {
			StepWithApp: true,
		},
		StepActionExtractText: {
			StepWithFindBy:    true,
			StepWithFindValue: true,
		},
		StepActionFill: {
			StepWithFillValue: true,
		},
		StepActionFindClick: {
			StepWithFindBy:    true,
			StepWithFindValue: true,
		},
		StepActionFindFillTab: {
			StepWithFindBy:    true,
			StepWithFindValue: true,
			StepWithFillValue: true,
		},
		StepActionFindFillReturn: {
			StepWithFindBy:    true,
			StepWithFindValue: true,
			StepWithFillValue: true,
		},
		StepActionSubmit: {}, // No fields required
	}

	// Validate that only allowed fields are present
	if step.With != nil {
		if allowed, ok := allowedFields[step.Action]; ok {
			if err := validateAllowedFields(step.With, allowed, step.Name, functionName, toolName); err != nil {
				return err
			}
		}
	}

	switch step.Action {
	case StepActionApp:
		if step.With == nil {
			return fmt.Errorf("step '%s' in function '%s' of tool '%s' with action 'open_app' must have a 'with' block",
				step.Name, functionName, toolName)
		}
		if _, ok := step.With[StepWithApp]; !ok {
			return fmt.Errorf("step '%s' in function '%s' of tool '%s' with action 'open_app' must provide an 'app_name' parameter",
				step.Name, functionName, toolName)
		}

	case StepActionExtractText:
		if step.Goal == "" {
			return fmt.Errorf("step '%s' in function '%s' of tool '%s' with action 'extract_text' must have a 'goal'",
				step.Name, functionName, toolName)
		}
		if step.With == nil {
			return fmt.Errorf("step '%s' in function '%s' of tool '%s' with action 'extract_text' must have a 'with' block",
				step.Name, functionName, toolName)
		}
		if err := validateFindStep(step, functionName, toolName); err != nil {
			return err
		}

	case StepActionFill:
		if step.With == nil {
			return fmt.Errorf("step '%s' in function '%s' of tool '%s' with action 'fill' must have a 'with' block",
				step.Name, functionName, toolName)
		}
		if _, ok := step.With[StepWithFillValue]; !ok {
			return fmt.Errorf("step '%s' in function '%s' of tool '%s' with action 'fill' must provide a 'fillValue' parameter",
				step.Name, functionName, toolName)
		}

	case StepActionFindClick, StepActionFindFillTab, StepActionFindFillReturn:
		if err := validateFindStep(step, functionName, toolName); err != nil {
			return err
		}

		// Additional validation for fill actions
		if step.Action == StepActionFindFillTab || step.Action == StepActionFindFillReturn {
			if _, ok := step.With[StepWithFillValue]; !ok {
				return fmt.Errorf("step '%s' in function '%s' of tool '%s' with action '%s' must provide a 'fillValue' parameter",
					step.Name, functionName, toolName, step.Action)
			}
		}

	default:
		return fmt.Errorf("unsupported action '%s' for desktop_use in step '%s' in function '%s' of tool '%s'",
			step.Action, step.Name, functionName, toolName)
	}

	return nil
}

// validateAPIStep validates steps for api_call operation
func validateAPIStep(step Step, functionName, toolName string) error {
	allowedFields := map[string]bool{
		StepWithURL:              true,
		Headers:                  true,
		StepWithRequestBody:      true,
		StepWithResponse:         true,
		StepWithExtractAuthToken: true,
	}

	// Validate allowed fields
	if step.With != nil {
		if err := validateAllowedFields(step.With, allowedFields, step.Name, functionName, toolName); err != nil {
			return err
		}
	}

	// API call actions are HTTP methods
	validHTTPMethods := map[string]bool{
		StepActionGET:    true,
		StepActionPOST:   true,
		StepActionPUT:    true,
		StepActionPATCH:  true,
		StepActionDELETE: true,
	}

	if !validHTTPMethods[step.Action] {
		return fmt.Errorf("step '%s' in function '%s' of tool '%s' has an invalid HTTP method: %s",
			step.Name, functionName, toolName, step.Action)
	}

	// isAuthentication is only allowed for API calls
	// No additional validation needed here as it's already an API step

	// Validate request body if present
	if reqBody, ok := step.With[StepWithRequestBody]; ok {
		bodyMap, ok := reqBody.(map[string]interface{})
		if !ok {
			interfaceMap, ok := reqBody.(map[interface{}]interface{})
			if !ok {
				return fmt.Errorf("step '%s' in function '%s' of tool '%s' has an invalid requestBody format",
					step.Name, functionName, toolName)
			}

			// Convert to map[string]interface{}
			bodyMap = make(map[string]interface{})
			for k, v := range interfaceMap {
				kStr, ok := k.(string)
				if !ok {
					return fmt.Errorf("step '%s' in function '%s' of tool '%s' has a non-string key in requestBody",
						step.Name, functionName, toolName)
				}
				bodyMap[kStr] = v
			}
		}

		contentType, ok := bodyMap[StepWithRequestBodyType].(string)
		if !ok {
			return fmt.Errorf("step '%s' in function '%s' of tool '%s' requestBody must have a 'type' field",
				step.Name, functionName, toolName)
		}

		validContentTypes := map[string]bool{
			StepBodyJSON:       true,
			StepBodyForm:       true,
			StepBodyMultipart:  true,
			StepBodyUrlEncoded: true,
		}

		if !validContentTypes[contentType] {
			return fmt.Errorf("step '%s' in function '%s' of tool '%s' has an invalid content type: %s",
				step.Name, functionName, toolName, contentType)
		}

		// Check that with field exists for the body
		if _, ok := bodyMap[With]; !ok {
			return fmt.Errorf("step '%s' in function '%s' of tool '%s' requestBody must have a 'with' field",
				step.Name, functionName, toolName)
		}
	}

	// Validate headers if present
	if headers, ok := step.With[Headers]; ok {
		headersList, ok := headers.([]interface{})
		if !ok {
			return fmt.Errorf("step '%s' in function '%s' of tool '%s' has an invalid headers format",
				step.Name, functionName, toolName)
		}

		for i, header := range headersList {
			headerMap, ok := header.(map[string]interface{})
			if !ok {
				interfaceMap, ok := header.(map[interface{}]interface{})
				if !ok {
					return fmt.Errorf("step '%s' in function '%s' of tool '%s' has an invalid header at index %d",
						step.Name, functionName, toolName, i)
				}

				headerMap = make(map[string]interface{})
				for k, v := range interfaceMap {
					kStr, ok := k.(string)
					if !ok {
						return fmt.Errorf("step '%s' in function '%s' of tool '%s' has a non-string key in header at index %d",
							step.Name, functionName, toolName, i)
					}
					headerMap[kStr] = v
				}
			}

			if _, ok := headerMap[Key]; !ok {
				return fmt.Errorf("step '%s' in function '%s' of tool '%s' header at index %d must have a 'key' field",
					step.Name, functionName, toolName, i)
			}

			if _, ok := headerMap[Value]; !ok {
				return fmt.Errorf("step '%s' in function '%s' of tool '%s' header at index %d must have a 'value' field",
					step.Name, functionName, toolName, i)
			}
		}
	}

	// Validate extractAuthToken if present
	if extractAuthToken, ok := step.With[StepWithExtractAuthToken]; ok {
		// extractAuthToken is only valid when isAuthentication is true
		if !step.IsAuthentication {
			return fmt.Errorf("step '%s' in function '%s' of tool '%s' has 'extractAuthToken' but 'isAuthentication' is not set to true. extractAuthToken is only valid for authentication steps",
				step.Name, functionName, toolName)
		}

		authTokenMap, ok := extractAuthToken.(map[string]interface{})
		if !ok {
			interfaceMap, ok := extractAuthToken.(map[interface{}]interface{})
			if !ok {
				return fmt.Errorf("step '%s' in function '%s' of tool '%s' has an invalid extractAuthToken format",
					step.Name, functionName, toolName)
			}

			// Convert to map[string]interface{}
			authTokenMap = make(map[string]interface{})
			for k, v := range interfaceMap {
				kStr, ok := k.(string)
				if !ok {
					return fmt.Errorf("step '%s' in function '%s' of tool '%s' has a non-string key in extractAuthToken",
						step.Name, functionName, toolName)
				}
				authTokenMap[kStr] = v
			}
		}

		// Validate 'from' field - required
		fromVal, ok := authTokenMap["from"]
		if !ok {
			return fmt.Errorf("step '%s' in function '%s' of tool '%s' extractAuthToken must have a 'from' field",
				step.Name, functionName, toolName)
		}
		fromStr, ok := fromVal.(string)
		if !ok {
			return fmt.Errorf("step '%s' in function '%s' of tool '%s' extractAuthToken 'from' must be a string",
				step.Name, functionName, toolName)
		}
		if fromStr != ExtractAuthTokenFromHeader && fromStr != ExtractAuthTokenFromResponseBody {
			return fmt.Errorf("step '%s' in function '%s' of tool '%s' extractAuthToken 'from' must be '%s' or '%s', got: %s",
				step.Name, functionName, toolName, ExtractAuthTokenFromHeader, ExtractAuthTokenFromResponseBody, fromStr)
		}

		// Validate 'key' field - required and non-empty
		keyVal, ok := authTokenMap["key"]
		if !ok {
			return fmt.Errorf("step '%s' in function '%s' of tool '%s' extractAuthToken must have a 'key' field",
				step.Name, functionName, toolName)
		}
		keyStr, ok := keyVal.(string)
		if !ok {
			return fmt.Errorf("step '%s' in function '%s' of tool '%s' extractAuthToken 'key' must be a string",
				step.Name, functionName, toolName)
		}
		if strings.TrimSpace(keyStr) == "" {
			return fmt.Errorf("step '%s' in function '%s' of tool '%s' extractAuthToken 'key' cannot be empty",
				step.Name, functionName, toolName)
		}

		// Validate 'cache' field - optional, must be >= 0 if present
		if cacheVal, ok := authTokenMap["cache"]; ok {
			var cacheInt int
			switch v := cacheVal.(type) {
			case int:
				cacheInt = v
			case int64:
				cacheInt = int(v)
			case float64:
				cacheInt = int(v)
			default:
				return fmt.Errorf("step '%s' in function '%s' of tool '%s' extractAuthToken 'cache' must be an integer",
					step.Name, functionName, toolName)
			}
			if cacheInt < 0 {
				return fmt.Errorf("step '%s' in function '%s' of tool '%s' extractAuthToken 'cache' must be >= 0, got: %d",
					step.Name, functionName, toolName, cacheInt)
			}
		}
	}

	// Validate saveAsFile if present
	if step.SaveAsFile != nil {
		if strings.TrimSpace(step.SaveAsFile.FileName) == "" {
			return fmt.Errorf("step '%s' in function '%s' of tool '%s' saveAsFile must have a non-empty 'fileName'",
				step.Name, functionName, toolName)
		}

		// Enforce resultIndex requirement - the file result needs to be stored
		if step.ResultIndex == 0 {
			return fmt.Errorf("step '%s' in function '%s' of tool '%s' with saveAsFile must have a 'resultIndex' to store the FileResult",
				step.Name, functionName, toolName)
		}

		// Apply default and cap for maxFileSize
		if step.SaveAsFile.MaxFileSize <= 0 {
			step.SaveAsFile.MaxFileSize = MaxFileSizeDefault
		}
		if step.SaveAsFile.MaxFileSize > MaxFileSizeHardCap {
			return fmt.Errorf("step '%s' in function '%s' of tool '%s' saveAsFile.maxFileSize (%d bytes) exceeds hard cap of %d bytes (100MB)",
				step.Name, functionName, toolName, step.SaveAsFile.MaxFileSize, MaxFileSizeHardCap)
		}
	}

	return nil
}

// testSQLIdempotency tests if SQL can be run multiple times without errors
func testSQLIdempotency(initSQL, functionName, toolName string) error {
	// Create an in-memory SQLite database
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		return fmt.Errorf("function '%s' in tool '%s' failed to create test database: %w",
			functionName, toolName, err)
	}
	defer db.Close()

	// First execution
	if _, err := db.Exec(initSQL); err != nil {
		return fmt.Errorf("function '%s' in tool '%s' has invalid init SQL (first execution failed): %w",
			functionName, toolName, err)
	}

	// Second execution - this must succeed for idempotency
	if _, err := db.Exec(initSQL); err != nil {
		return fmt.Errorf("function '%s' in tool '%s' has init SQL that is not idempotent (second execution failed): %w. Use CREATE TABLE IF NOT EXISTS, INSERT OR IGNORE, INSERT OR REPLACE, or similar constructs",
			functionName, toolName, err)
	}

	return nil
}

// validateToolInitScripts joins all init scripts from all functions in a tool
// and validates them together in an ephemeral database.
// This catches cross-function conflicts like duplicate table definitions.
func validateToolInitScripts(tools []Tool, isSystemTool bool) error {
	// Skip validation for system tools
	if isSystemTool {
		return nil
	}

	for _, tool := range tools {
		var allInitSQL []string
		var functionsWithInit []string

		for _, fn := range tool.Functions {
			if fn.With != nil && fn.With[WithInit] != nil {
				if initSQL, ok := fn.With[WithInit].(string); ok && initSQL != "" {
					allInitSQL = append(allInitSQL, initSQL)
					functionsWithInit = append(functionsWithInit, fn.Name)
				}
			}
		}

		if len(allInitSQL) == 0 {
			continue
		}

		// Join all init scripts and test together
		combinedSQL := strings.Join(allInitSQL, "\n")
		if err := testCombinedSQLValidity(combinedSQL, tool.Name, functionsWithInit); err != nil {
			return err
		}
	}
	return nil
}

// testCombinedSQLValidity tests if combined SQL scripts can be run together without conflicts
func testCombinedSQLValidity(combinedSQL, toolName string, functionNames []string) error {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		return fmt.Errorf("tool '%s' failed to create test database: %w", toolName, err)
	}
	defer db.Close()

	functionsInfo := strings.Join(functionNames, ", ")

	// Execute all init scripts together
	if _, err := db.Exec(combinedSQL); err != nil {
		return fmt.Errorf("tool '%s' has conflicting init SQL across functions [%s]: %w", toolName, functionsInfo, err)
	}

	// Test idempotency - running again should succeed
	if _, err := db.Exec(combinedSQL); err != nil {
		return fmt.Errorf("tool '%s' has combined init SQL that is not idempotent (functions: %s): %w", toolName, functionsInfo, err)
	}

	return nil
}

// parseMigrations extracts and validates migrations from a function's With block.
// Returns nil slice if no migrations are defined.
func parseMigrations(withBlock map[string]interface{}, functionName, toolName string) ([]Migration, error) {
	if withBlock == nil || withBlock[WithMigrations] == nil {
		return nil, nil
	}

	raw, ok := withBlock[WithMigrations].([]interface{})
	if !ok {
		return nil, fmt.Errorf("function '%s' in tool '%s' has invalid 'migrations' field: must be a list",
			functionName, toolName)
	}

	var migrations []Migration
	seenVersions := make(map[int]bool)

	for i, item := range raw {
		// Handle both map[string]interface{} and map[interface{}]interface{}
		var m map[string]interface{}
		switch v := item.(type) {
		case map[string]interface{}:
			m = v
		case map[interface{}]interface{}:
			m = make(map[string]interface{})
			for k, val := range v {
				if kStr, ok := k.(string); ok {
					m[kStr] = val
				}
			}
		default:
			return nil, fmt.Errorf("function '%s' in tool '%s' migration at index %d: must be an object with 'version' and 'sql' fields",
				functionName, toolName, i)
		}

		version, hasVersion := m["version"]
		sqlVal, hasSQL := m["sql"]

		if !hasVersion || !hasSQL {
			return nil, fmt.Errorf("function '%s' in tool '%s' migration at index %d: must have 'version' and 'sql' fields",
				functionName, toolName, i)
		}

		// Parse version (YAML unmarshals numbers as int or float64)
		var versionInt int
		switch v := version.(type) {
		case int:
			versionInt = v
		case float64:
			versionInt = int(v)
		default:
			return nil, fmt.Errorf("function '%s' in tool '%s' migration at index %d: 'version' must be a positive integer",
				functionName, toolName, i)
		}

		if versionInt <= 0 {
			return nil, fmt.Errorf("function '%s' in tool '%s' migration at index %d: 'version' must be a positive integer (got %d)",
				functionName, toolName, i, versionInt)
		}

		sqlStr, ok := sqlVal.(string)
		if !ok || sqlStr == "" {
			return nil, fmt.Errorf("function '%s' in tool '%s' migration at index %d: 'sql' must be a non-empty string",
				functionName, toolName, i)
		}

		if seenVersions[versionInt] {
			return nil, fmt.Errorf("function '%s' in tool '%s' has duplicate migration version %d",
				functionName, toolName, versionInt)
		}
		seenVersions[versionInt] = true

		migrations = append(migrations, Migration{Version: versionInt, SQL: sqlStr})
	}

	return migrations, nil
}

// validateToolMigrations validates migrations across all functions in a tool.
// Ensures that the same version number across different functions has identical SQL.
// This allows multiple functions to declare the same migration (like init scripts),
// as long as the SQL is identical.
func validateToolMigrations(tools []Tool, isSystemTool bool) error {
	// Skip validation for system tools (like validateToolInitScripts)
	if isSystemTool {
		return nil
	}

	for _, tool := range tools {
		// Collect all migrations by version across all functions
		// Map: version -> {sql: string, functions: []string}
		type migrationInfo struct {
			sql       string
			functions []string
		}
		versionMap := make(map[int]*migrationInfo)

		for _, fn := range tool.Functions {
			migrations, err := parseMigrations(fn.With, fn.Name, tool.Name)
			if err != nil {
				return err
			}

			for _, m := range migrations {
				existing, exists := versionMap[m.Version]
				if exists {
					// Same version exists - SQL must be identical
					if existing.sql != m.SQL {
						return fmt.Errorf(
							"tool '%s' has conflicting migration v%d: "+
								"function '%s' has different SQL than function '%s'. "+
								"Same version across functions must have identical SQL",
							tool.Name, m.Version, fn.Name, existing.functions[0])
					}
					existing.functions = append(existing.functions, fn.Name)
				} else {
					versionMap[m.Version] = &migrationInfo{
						sql:       m.SQL,
						functions: []string{fn.Name},
					}
				}
			}
		}
	}

	return nil
}

// validateInitAndMigrationsTogether validates that init scripts and migrations work together
// by running them in an ephemeral in-memory SQLite database.
//
// This performs TWO validations to match runtime behavior:
// 1. Combined validation: All inits + all migrations (catches column conflicts)
// 2. Per-function validation: Each function's init + all migrations (matches runtime)
//
// At runtime, when a function executes:
// - Only THAT function's init script runs
// - But ALL migrations from ALL functions are collected and applied
//
// This means a migration in function A that references a table from function B's init
// will fail at runtime if function A runs first. We catch this at parse time.
func validateInitAndMigrationsTogether(tools []Tool, isSystemTool bool) error {
	// Skip validation for system tools
	if isSystemTool {
		return nil
	}

	for _, tool := range tools {
		// Collect all unique migrations across functions
		versionMap := make(map[int]string) // version -> SQL
		for _, fn := range tool.Functions {
			migrations, err := parseMigrations(fn.With, fn.Name, tool.Name)
			if err != nil {
				continue // Already validated in validateToolMigrations
			}
			for _, m := range migrations {
				if _, exists := versionMap[m.Version]; !exists {
					versionMap[m.Version] = m.SQL
				}
			}
		}

		// If no migrations, nothing extra to validate
		if len(versionMap) == 0 {
			continue
		}

		// Sort migrations by version
		var versions []int
		for v := range versionMap {
			versions = append(versions, v)
		}
		sort.Ints(versions)

		// VALIDATION 1: Combined validation (all inits + all migrations)
		// This catches column conflicts where migration adds a column already in init
		if err := validateCombinedInitAndMigrations(tool, versions, versionMap); err != nil {
			return err
		}

		// VALIDATION 2: Per-function validation (simulates runtime behavior)
		// At runtime, only the current function's init runs, but ALL migrations run.
		// This catches cases where a migration references a table from another function's init.
		if err := validatePerFunctionInitWithAllMigrations(tool, versions, versionMap); err != nil {
			return err
		}
	}

	return nil
}

// validateCombinedInitAndMigrations tests all init scripts together with all migrations.
// This catches column conflicts (migration adds column that already exists in init).
func validateCombinedInitAndMigrations(tool Tool, versions []int, versionMap map[int]string) error {
	// Collect all init scripts
	var allInitSQL []string
	for _, fn := range tool.Functions {
		if fn.With != nil && fn.With[WithInit] != nil {
			if initSQL, ok := fn.With[WithInit].(string); ok && initSQL != "" {
				allInitSQL = append(allInitSQL, initSQL)
			}
		}
	}

	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		return fmt.Errorf("tool '%s' failed to create test database for migration validation: %w", tool.Name, err)
	}
	defer db.Close()

	// Run all init scripts first
	if len(allInitSQL) > 0 {
		combinedInit := strings.Join(allInitSQL, "\n")
		if _, err := db.Exec(combinedInit); err != nil {
			return fmt.Errorf("tool '%s' init SQL failed during migration validation: %w", tool.Name, err)
		}
	}

	// Run migrations in version order
	for _, version := range versions {
		migrationSQL := versionMap[version]
		if _, err := db.Exec(migrationSQL); err != nil {
			if strings.Contains(err.Error(), "duplicate column name") {
				return fmt.Errorf("tool '%s' migration v%d failed: %w. "+
					"This column likely already exists in the init script. "+
					"Either remove it from init (for existing deployments) or remove the migration (for new deployments only)",
					tool.Name, version, err)
			}
			return fmt.Errorf("tool '%s' migration v%d failed: %w", tool.Name, version, err)
		}
	}

	return nil
}

// validatePerFunctionInitWithAllMigrations simulates runtime behavior.
// At runtime, only the current function's init runs, but ALL migrations are collected and applied.
// This catches cases where a migration references a table that only exists in another function's init.
func validatePerFunctionInitWithAllMigrations(tool Tool, versions []int, versionMap map[int]string) error {
	// For each function with a db operation, test its init + all migrations
	for _, fn := range tool.Functions {
		// Skip functions without db operation
		if fn.Operation != OperationDB {
			continue
		}

		// Get this function's init SQL (may be empty)
		var initSQL string
		if fn.With != nil && fn.With[WithInit] != nil {
			if sql, ok := fn.With[WithInit].(string); ok {
				initSQL = sql
			}
		}

		// Create ephemeral database
		db, err := sql.Open("sqlite3", ":memory:")
		if err != nil {
			return fmt.Errorf("tool '%s' function '%s' failed to create test database: %w", tool.Name, fn.Name, err)
		}

		// Run ONLY this function's init
		if initSQL != "" {
			if _, err := db.Exec(initSQL); err != nil {
				db.Close()
				return fmt.Errorf("tool '%s' function '%s' init SQL failed: %w", tool.Name, fn.Name, err)
			}
		}

		// Run ALL migrations (simulating runtime behavior)
		for _, version := range versions {
			migrationSQL := versionMap[version]
			if _, err := db.Exec(migrationSQL); err != nil {
				db.Close()
				if strings.Contains(err.Error(), "no such table") {
					return fmt.Errorf("tool '%s' function '%s': migration v%d failed: %w. "+
						"This migration references a table that doesn't exist in this function's init. "+
						"At runtime, if this function runs first, the migration will fail. "+
						"Solution: Ensure all functions with db operations have the same init script, "+
						"or ensure migrations only reference tables created in their own function's init",
						tool.Name, fn.Name, version, err)
				}
				return fmt.Errorf("tool '%s' function '%s': migration v%d failed: %w", tool.Name, fn.Name, version, err)
			}
		}

		db.Close()
	}

	return nil
}

// extractTableNamesFromSQL extracts table names from SQL statements
func extractTableNamesFromSQL(sql string) []string {
	tables := make(map[string]bool)
	upperSQL := strings.ToUpper(sql)

	// Regular expressions to match table names in various SQL contexts
	patterns := []string{
		`CREATE\s+TABLE\s+(?:IF\s+NOT\s+EXISTS\s+)?([^\s\(]+)`,
		`FROM\s+([^\s,]+)`,
		`JOIN\s+([^\s]+)`,
		`INSERT\s+(?:OR\s+\w+\s+)?INTO\s+([^\s\(]+)`,
		`UPDATE\s+([^\s]+)`, // Will capture table name; reserved keywords filtered later
		`DELETE\s+FROM\s+([^\s]+)`,
	}

	// Reserved SQL keywords that should not be considered as table names
	reservedKeywords := map[string]bool{
		"SET":     true,
		"WHERE":   true,
		"VALUES":  true,
		"SELECT":  true,
		"ORDER":   true,
		"GROUP":   true,
		"HAVING":  true,
		"LIMIT":   true,
		"OFFSET":  true,
		"UNION":   true,
		"EXCEPT":  true,
		"CASE":    true,
		"WHEN":    true,
		"THEN":    true,
		"ELSE":    true,
		"END":     true,
		"AS":      true,
		"ON":      true,
		"USING":   true,
		"NATURAL": true,
		"INNER":   true,
		"OUTER":   true,
		"LEFT":    true,
		"RIGHT":   true,
		"FULL":    true,
		"CROSS":   true,
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindAllStringSubmatch(upperSQL, -1)
		for _, match := range matches {
			if len(match) > 1 {
				tableName := strings.Trim(match[1], "`\"[];")
				// Skip reserved keywords
				if !reservedKeywords[tableName] {
					// Skip SQLite table-valued functions (e.g., json_each, json_tree)
					// These look like function calls: NAME(...) and should not be treated as tables
					if strings.Contains(tableName, "(") {
						continue
					}
					tables[tableName] = true
				}
			}
		}
	}

	result := make([]string, 0, len(tables))
	for table := range tables {
		result = append(result, table)
	}
	return result
}

// validateDBOperation validates DB operation specific requirements at the function level
func validateDBOperation(function Function, toolName string, isSystemTool bool) error {
	// Extract initialized tables from init SQL
	var initializedTables []string
	var initSQL string

	if function.With != nil && function.With[WithInit] != nil {
		var ok bool
		initSQL, ok = function.With[WithInit].(string)
		if !ok {
			return fmt.Errorf("function '%s' in tool '%s' has invalid 'init' field in 'with' block: must be a string",
				function.Name, toolName)
		}

		// Test idempotency by running the SQL twice in an ephemeral database
		if initSQL != "" {
			if err := testSQLIdempotency(initSQL, function.Name, toolName); err != nil {
				return err
			}
		}

		// Extract table names from init SQL
		initializedTables = extractTableNamesFromSQL(initSQL)
	}

	// Validate that all DB steps only query initialized tables -- not for system tools
	for _, step := range function.Steps {
		if (step.Action == StepActionSelect || step.Action == StepActionWrite) && !isSystemTool {
			if step.With != nil && step.With[step.Action] != nil {
				sqlQuery, ok := step.With[step.Action].(string)
				if ok {
					queriedTables := extractTableNamesFromSQL(sqlQuery)
					for _, table := range queriedTables {
						found := false
						for _, initTable := range initializedTables {
							if strings.EqualFold(table, initTable) {
								found = true
								break
							}
						}
						if !found && len(initializedTables) > 0 {
							return fmt.Errorf("step '%s' in function '%s' of tool '%s' queries table '%s' which was not initialized in the 'init' field",
								step.Name, function.Name, toolName, table)
						}
					}
				}
			}
		}
	}

	return nil
}

// validateDBStep validates steps for db operation
func validateDBStep(step Step, functionName, toolName string) error {
	validActions := map[string]bool{
		StepActionSelect: true,
		StepActionWrite:  true,
	}

	if !validActions[step.Action] {
		return fmt.Errorf("step '%s' in function '%s' of tool '%s' has invalid action '%s' for db operation; must be one of: select, write",
			step.Name, functionName, toolName, step.Action)
	}

	// For db operations, the SQL query should be provided in the step's with block
	if step.With == nil {
		return fmt.Errorf("step '%s' in function '%s' of tool '%s' must have a 'with' block containing the SQL query",
			step.Name, functionName, toolName)
	}

	// The SQL query should be provided as a string value in the with block
	// The key for the SQL depends on the action type
	sqlKey := step.Action // "select" or "write"
	sqlQuery, exists := step.With[sqlKey]
	if !exists {
		return fmt.Errorf("step '%s' in function '%s' of tool '%s' must have a '%s' field in the 'with' block containing the SQL query",
			step.Name, functionName, toolName, sqlKey)
	}

	// Ensure the SQL query is a string
	if _, ok := sqlQuery.(string); !ok {
		return fmt.Errorf("step '%s' in function '%s' of tool '%s' has invalid '%s' field: must be a string",
			step.Name, functionName, toolName, sqlKey)
	}

	return nil
}

// validateNoQuotedSQLVariables checks for INPUT variables wrapped in quotes in SQL queries
// This is a common mistake that causes variables to be inserted as literal strings ('?')
// instead of being properly parameterized. Returns an error if any quoted input variables are found.
//
// NOTE: Environment variables ($UPPERCASE_VAR) are NOT flagged because they are replaced
// with their literal values BEFORE parameterization, so '$ENV_VAR' correctly becomes 'value'.
// Only input variables ($lowercase_var) are problematic because they become parameterized
// placeholders, turning '$input_var' into '?' which is a literal string, not a placeholder.
func validateNoQuotedSQLVariables(tool Tool) error {
	// Regex to find quoted INPUT variables only: '$varName' or "$varName"
	// Input variables start with lowercase letter: $[a-z]
	// Environment variables start with uppercase: $[A-Z] - these are OK to quote
	quotedInputVarPattern := regexp.MustCompile(`['"](\$[a-z][a-zA-Z0-9_]*)['"]`)

	for _, function := range tool.Functions {
		if function.Operation != OperationDB {
			continue
		}

		for _, step := range function.Steps {
			if step.With == nil {
				continue
			}

			// Check both 'select' and 'write' SQL queries
			for _, sqlKey := range []string{StepActionSelect, StepActionWrite} {
				sqlQuery, ok := step.With[sqlKey].(string)
				if !ok {
					continue
				}

				matches := quotedInputVarPattern.FindAllStringSubmatch(sqlQuery, -1)
				if len(matches) > 0 {
					varName := matches[0][1]
					return fmt.Errorf(
						"function '%s', step '%s': Input variable %s is wrapped in quotes in SQL query. "+
							"This will insert the literal string '?' instead of the actual value. "+
							"Remove the quotes around the variable for proper parameterized query binding. "+
							"Example: Use VALUES(%s, ...) instead of VALUES('%s', ...)",
						function.Name, step.Name, varName, varName, varName)
				}
			}
		}
	}

	return nil
}

// validateFindStep validates the common parameters for steps that find elements
func validateFindStep(step Step, functionName, toolName string) error {
	if step.With == nil {
		return fmt.Errorf("step '%s' in function '%s' of tool '%s' with action '%s' must have a 'with' block",
			step.Name, functionName, toolName, step.Action)
	}

	// Check findBy parameter
	findBy, ok := step.With[StepWithFindBy]
	if !ok {
		return fmt.Errorf("step '%s' in function '%s' of tool '%s' with action '%s' must provide a 'findBy' parameter",
			step.Name, functionName, toolName, step.Action)
	}

	// Check findBy is one of the allowed values
	findByStr, ok := findBy.(string)
	if !ok {
		return fmt.Errorf("step '%s' in function '%s' of tool '%s' has an invalid 'findBy' format, must be a string",
			step.Name, functionName, toolName)
	}

	validFindByOptions := map[string]bool{
		StepWithFindById:              true,
		StepWithFindByLabel:           true,
		StepWithFindByType:            true,
		StepWithFindByVisual:          true,
		StepWithFindBySemanticContext: true,
	}

	if !validFindByOptions[findByStr] {
		return fmt.Errorf("step '%s' in function '%s' of tool '%s' has an invalid 'findBy' value: %s",
			step.Name, functionName, toolName, findByStr)
	}

	// If findBy is "type", validate the type value
	if findByStr == StepWithFindByType {
		findValue, ok := step.With[StepWithFindValue]
		if !ok {
			return fmt.Errorf("step '%s' in function '%s' of tool '%s' must provide a 'findValue' parameter",
				step.Name, functionName, toolName)
		}

		findValueStr, ok := findValue.(string)
		if !ok {
			return fmt.Errorf("step '%s' in function '%s' of tool '%s' has an invalid 'findValue' format, must be a string",
				step.Name, functionName, toolName)
		}

		validElementTypes := map[string]bool{
			StepWithFindByTypeButton:    true,
			StepWithFindByTypeTextInput: true,
			StepWithFindByTypeCheckbox:  true,
			StepWithFindByTypeSelectbox: true,
			StepWithFindByTypeSearchbar: true,
			StepWithFindByTypeLink:      true,
			StepWithFindByTypeIcon:      true,
			StepWithFindByTypeTextarea:  true,
			StepWithListItem:            true,
			StepWithFindByTypeText:      true,
		}

		if !validElementTypes[findValueStr] {
			return fmt.Errorf("step '%s' in function '%s' of tool '%s' has an invalid element type: %s",
				step.Name, functionName, toolName, findValueStr)
		}
	} else {
		// For other findBy options, just make sure findValue is provided
		if _, ok := step.With[StepWithFindValue]; !ok {
			return fmt.Errorf("step '%s' in function '%s' of tool '%s' must provide a 'findValue' parameter",
				step.Name, functionName, toolName)
		}
	}

	return nil
}

func ValidateSteps(function Function, tool Tool, configEnvVars []EnvVar) error {
	var (
		hasFillAction, hasSubmitAction bool
	)

	// Validate that isAuthentication step must be at index 0 (first step)
	for i, step := range function.Steps {
		if step.IsAuthentication && i > 0 {
			return fmt.Errorf("step '%s' in function '%s' of tool '%s' has 'isAuthentication' set to true but is not the first step. Authentication steps must be at index 0",
				step.Name, function.Name, tool.Name)
		}
	}

	for _, step := range function.Steps {
		if strings.Contains(step.Action, "fill") {
			hasFillAction = true
		}
		if strings.Contains(step.Action, "submit") || strings.Contains(step.Action, "return") {
			hasSubmitAction = true
		}

		if err := validateStep(step, function.Name, tool.Name, function.Operation); err != nil {
			return err
		}

		// Validate waitFor function exists in the tool
		if step.ForEach != nil && step.ForEach.WaitFor != nil {
			if err := validateWaitForFunctionExists(step.ForEach.WaitFor, tool, step.Name, function.Name); err != nil {
				return err
			}
		}

		// Validate shouldSkip functions exist in the tool (supports both single and array format)
		if step.ForEach != nil && step.ForEach.HasShouldSkip() {
			if err := validateShouldSkipFunctionsExist(step.ForEach, tool, step.Name, function.Name); err != nil {
				return err
			}
		}

		// Validate that step variables have corresponding input definitions
		// Skip for terminal operations because shell scripts use $var syntax for both shell variables AND YAML inputs
		// Skip for code operations because prompts may contain $variable references and result[N] patterns
		if step.With != nil && function.Operation != OperationTerminal && function.Operation != OperationCode {
			if err := validateStepVariableReferences(step.With, function, tool, configEnvVars, step); err != nil {
				return err
			}
		}
	}

	if hasFillAction && (!hasSubmitAction) {
		return fmt.Errorf("function '%s' in tool '%s' has fill actions but no submit or return action",
			function.Name, tool.Name)
	}

	if err := validateResultReferences(function, tool.Name); err != nil {
		return err
	}

	for _, step := range function.ZeroState.Steps {
		if err := validateStep(step, function.Name, tool.Name, function.Operation); err != nil {
			return fmt.Errorf("in zero-state: %w", err)
		}

		// Validate step variables for zero-state steps too
		// Skip for terminal and code operations (same reason as above)
		if step.With != nil && function.Operation != OperationTerminal && function.Operation != OperationCode {
			if err := validateStepVariableReferences(step.With, function, tool, configEnvVars, step); err != nil {
				return fmt.Errorf("in zero-state: %w", err)
			}
		}
	}

	return nil
}

// extractSystemVarsAndFuncs scans through text to find system variables and functions
func extractSystemVarsAndFuncs(text string, vars map[string]struct{ base, field string }, funcs map[string]bool) {
	// Extract system variables (prefixed with $)
	varRegex := regexp.MustCompile(`\$([A-Z_]+)(?:\.([a-zA-Z0-9_]+))?`)
	matches := varRegex.FindAllStringSubmatch(text, -1)
	for _, match := range matches {
		if len(match) >= 2 {
			baseVar := "$" + match[1]
			field := ""
			if len(match) >= 3 {
				field = match[2]
			}
			vars[baseVar] = struct{ base, field string }{base: baseVar, field: field}
		}
	}

	funcRegex := regexp.MustCompile(`(Ask|Learn|AskHuman|NotifyHuman|AskUser)\([^)]*\)`)
	funcMatches := funcRegex.FindAllString(text, -1)
	for _, match := range funcMatches {
		// Extract function name (before the parenthesis)
		parts := strings.Split(match, "(")
		funcName := parts[0]
		funcs[funcName] = true
	}
}

// scanFunctionForSystemUsage scans all fields of a function for system variables and functions
func scanFunctionForSystemUsage(function *Function) {
	function.UsedSysVars = make(map[string]struct{ base, field string })
	function.UsedSysFuncs = make(map[string]bool)

	runOnlyIfStr := GetRunOnlyIfCondition(function.RunOnlyIf)
	if runOnlyIfStr != "" {
		extractSystemVarsAndFuncs(runOnlyIfStr, function.UsedSysVars, function.UsedSysFuncs)
	}

	for _, step := range function.Steps {
		extractSystemVarsAndFuncs(step.Goal, function.UsedSysVars, function.UsedSysFuncs)

		// Skip step.With for terminal and code operations because they use $var syntax for shell variables / prompt templates
		if function.Operation != OperationTerminal && function.Operation != OperationCode {
			for _, value := range step.With {
				if strValue, ok := value.(string); ok {
					extractSystemVarsAndFuncs(strValue, function.UsedSysVars, function.UsedSysFuncs)
				}
			}
		}

		if step.OnError != nil {
			extractSystemVarsAndFuncs(step.OnError.Message, function.UsedSysVars, function.UsedSysFuncs)
		}
	}

	for _, input := range function.Input {
		// Scan input value field
		if input.Value != "" {
			extractSystemVarsAndFuncs(input.Value, function.UsedSysVars, function.UsedSysFuncs)
		}

		// Scan success criteria if it's a string
		if input.SuccessCriteria != nil {
			if successStr, ok := input.SuccessCriteria.(string); ok && successStr != "" {
				extractSystemVarsAndFuncs(successStr, function.UsedSysVars, function.UsedSysFuncs)
			}
		}

		// Scan error message
		if input.OnError != nil {
			extractSystemVarsAndFuncs(input.OnError.Message, function.UsedSysVars, function.UsedSysFuncs)
		}
	}
}

// verifyParamVariables checks for invalid variable references in text
func verifyParamVariables(text string, function Function, toolName string, inputs []Input) error {
	// Remove {$$...} escape sequences before validation - these produce literal $... in output
	// and should not be validated as variable references
	escapedDollarRegex := regexp.MustCompile(`\{\$\$[^}]+\}`)
	text = escapedDollarRegex.ReplaceAllString(text, "")

	// Extract all variable references
	// Supports optional date transformation functions: .toISO() and .toDDMMYYYY()
	varRegex := regexp.MustCompile(`\$[A-Za-z0-9_]+(\.[a-z0-9_]+)?(?:\.(toISO|toDDMMYYYY)\(\))?`)
	matches := varRegex.FindAllString(text, -1)

	for _, match := range matches {
		baseVar := strings.Split(match, ".")[0]

		if isSysVar(baseVar) {
			continue // Already validated by validateSystemVarsAndFuncs
		}

		if isEnvVar(baseVar) {
			continue // Environment variables are available globally
		}

		isInput := false
		for _, input := range inputs {
			if "$"+input.Name == baseVar {
				isInput = true
				break
			}
		}

		if !isInput {
			return fmt.Errorf("function '%s' in tool '%s' references undefined variable '%s'",
				function.Name, toolName, baseVar)
		}
	}

	return nil
}

// Helper function to check if a variable is a system variable
func isSysVar(varName string) bool {
	systemVars := map[string]bool{
		SysVarMe:      true,
		SysVarMessage: true,
		SysVarNow:     true,
		SysVarUser:    true,
		SysVarAdmin:   true,
		SysVarCompany: true,
		SysVarFile:    true,
	}

	return systemVars[varName]
}

// Helper function to check if a variable is an environment variable
func isEnvVar(varName string) bool {
	// Remove the $ prefix if present
	if strings.HasPrefix(varName, "$") {
		varName = varName[1:]
	}

	return varName == strings.ToUpper(varName)
}

func shouldSkipVariableValidation(stepAction, paramName string) bool {
	if stepAction == StepActionSh || stepAction == StepActionBash {
		return paramName == StepWithLinux || paramName == StepWithMacOS || paramName == StepWithWindows
	}
	return false
}

func extractAndValidateAllVariables(text string, function Function, toolName string, configEnvVars []EnvVar, step *Step) error {
	// Remove {$$...} escape sequences before validation - these produce literal $... in output
	// and should not be validated as variable references
	escapedDollarRegex := regexp.MustCompile(`\{\$\$[^}]+\}`)
	text = escapedDollarRegex.ReplaceAllString(text, "")

	// Updated regex to also match array access patterns like $var[0], $var.field[0], etc.
	// Also supports optional date transformation functions: .toISO() and .toDDMMYYYY() at the end
	varRegex := regexp.MustCompile(`\$[A-Za-z0-9_]+(?:\[[0-9]+\])?(?:\.[A-Za-z0-9_]+(?:\[[0-9]+\])?)*(?:\.(toISO|toDDMMYYYY)\(\))?`)
	matches := varRegex.FindAllString(text, -1)

	for _, match := range matches {
		baseParts := strings.Split(match, ".")
		// Extract base variable name, removing any array access brackets
		baseVar := baseParts[0]
		if idx := strings.Index(baseVar, "["); idx > 0 {
			baseVar = baseVar[:idx]
		}

		if baseVar == strings.ToUpper(baseVar) {
			continue
		}

		isValidInput := false
		for _, input := range function.Input {
			if "$"+input.Name == baseVar {
				isValidInput = true
				break
			}
		}

		// Check if variable references a needs function output
		if !isValidInput {
			for _, need := range function.Needs {
				if "$"+need.Name == baseVar {
					isValidInput = true
					break
				}
			}
		}

		if !isValidInput && step != nil && step.ForEach != nil {
			itemVar := step.ForEach.ItemVar
			if itemVar == "" {
				itemVar = DefaultForEachItemVar
			}
			indexVar := step.ForEach.IndexVar
			if indexVar == "" {
				indexVar = DefaultForEachIndexVar
			}

			if baseVar == "$"+itemVar || baseVar == "$"+indexVar {
				isValidInput = true
			}
		}

		if !isValidInput {
			return fmt.Errorf("function '%s' in tool '%s' references undefined variable '%s'",
				function.Name, toolName, baseVar)
		}

		// Check for unsupported direct array access pattern: $var[0] without dot notation
		// Pattern like $items[0].field is NOT supported (array at root level)
		// Supported patterns: $data.items[0].field, $obj.users[0].name
		directArrayPattern := regexp.MustCompile(`^\$[a-zA-Z0-9_]+\[`)
		if directArrayPattern.MatchString(match) {
			inputName := strings.TrimPrefix(strings.Split(match, "[")[0], "$")
			return fmt.Errorf("function '%s' in tool '%s' uses unsupported pattern '%s'. Direct array access on variable '%s' is not supported. The variable must be an object with the array as a property. Supported: $data.items[0].field. Not supported: $items[0].field",
				function.Name, toolName, match, inputName)
		}
	}

	return nil
}

func validateRunOnlyIfVariables(text string, function Function, toolName string, configEnvVars []EnvVar) error {
	// Supports optional date transformation functions: .toISO() and .toDDMMYYYY()
	varRegex := regexp.MustCompile(`\$[A-Za-z0-9_]+(\.[a-z0-9_]+)?(?:\.(toISO|toDDMMYYYY)\(\))?`)
	matches := varRegex.FindAllString(text, -1)

	for _, match := range matches {
		baseParts := strings.Split(match, ".")
		baseVar := baseParts[0]

		if baseVar == strings.ToUpper(baseVar) {
			continue
		}

		inputName := strings.TrimPrefix(baseVar, "$")
		return fmt.Errorf("function '%s' in tool '%s' references input '%s' in runOnlyIf, but inputs are not accessible in runOnlyIf conditions. Tip: Consider creating another function that has these inputs and add this function as a dependency (using 'needs:'). Make the output include the values you need (operations like 'db' or 'format' can help here). These values will be available in the context, allowing you to check the context properly",
			function.Name, toolName, inputName)
	}

	return nil
}

// validateFunctionVariablesAndExpressions validates all variable references and function calls
func validateFunctionVariablesAndExpressions(function Function, toolName string, configEnvVars []EnvVar) error {
	// First scan the function for all system variable and function usage
	scanFunctionForSystemUsage(&function)

	// Validate system vars and functions based on trigger types
	if err := validateSystemVarsAndFuncs(function, toolName, configEnvVars); err != nil {
		return err
	}

	// Now validate all variables including inputs (both cases)
	runOnlyIfStr := GetRunOnlyIfCondition(function.RunOnlyIf)
	if runOnlyIfStr != "" {
		// Special validation for RunOnlyIf - inputs are not accessible
		if err := validateRunOnlyIfVariables(runOnlyIfStr, function, toolName, configEnvVars); err != nil {
			return err
		}
	}

	for _, step := range function.Steps {
		if step.Goal != "" {
			if err := extractAndValidateAllVariables(step.Goal, function, toolName, configEnvVars, &step); err != nil {
				return err
			}
		}

		for paramName, paramValue := range step.With {
			if strValue, ok := paramValue.(string); ok {
				if shouldSkipVariableValidation(step.Action, paramName) {
					continue
				}
				if err := extractAndValidateAllVariables(strValue, function, toolName, configEnvVars, &step); err != nil {
					return fmt.Errorf("in parameter '%s': %w", paramName, err)
				}
			} else if paramName == "requestBody" {
				// Handle nested requestBody.with fields
				if bodyMap, ok := paramValue.(map[string]interface{}); ok {
					if withFields, ok := bodyMap["with"].(map[string]interface{}); ok {
						for fieldName, fieldValue := range withFields {
							if strValue, ok := fieldValue.(string); ok {
								if err := extractAndValidateAllVariables(strValue, function, toolName, configEnvVars, &step); err != nil {
									return fmt.Errorf("in requestBody field '%s': %w", fieldName, err)
								}
							}
						}
					}
				} else if bodyMap, ok := paramValue.(map[interface{}]interface{}); ok {
					// Handle map[interface{}]interface{} from YAML parsing
					if withFields, ok := bodyMap["with"].(map[interface{}]interface{}); ok {
						for fieldName, fieldValue := range withFields {
							fieldNameStr, ok := fieldName.(string)
							if !ok {
								continue
							}
							if strValue, ok := fieldValue.(string); ok {
								if err := extractAndValidateAllVariables(strValue, function, toolName, configEnvVars, &step); err != nil {
									return fmt.Errorf("in requestBody field '%s': %w", fieldNameStr, err)
								}
							}
						}
					}
				}
			}
		}

		if step.OnError != nil && step.OnError.Message != "" {
			if err := extractAndValidateAllVariables(step.OnError.Message, function, toolName, configEnvVars, &step); err != nil {
				return fmt.Errorf("in error message: %w", err)
			}
		}
	}

	for _, input := range function.Input {
		if input.OnError != nil && input.OnError.Message != "" {
			if err := extractAndValidateAllVariables(input.OnError.Message, function, toolName, configEnvVars, nil); err != nil {
				return fmt.Errorf("in input '%s' error message: %w", input.Name, err)
			}
		}
	}

	return nil
}

// validateSystemVarsAndFuncs validates system variables and functions specifically
func validateSystemVarsAndFuncs(function Function, toolName string, configEnvVars []EnvVar) error {
	validEnvVars := make(map[string]bool)
	for _, env := range configEnvVars {
		validEnvVars["$"+env.Name] = true
	}

	// Determine which trigger types the function has
	triggerTypes := make(map[string]bool)
	for _, trigger := range function.Triggers {
		triggerTypes[trigger.Type] = true
	}

	// Skip trigger-based validation for functions without triggers
	// These functions are called via onSuccess/onFailure and inherit context from their callers
	if len(triggerTypes) == 0 {
		return nil
	}

	// Check each used system variable
	for _, varInfo := range function.UsedSysVars {
		baseVar := varInfo.base
		field := varInfo.field

		// If it's an environment variable, it's always valid
		if validEnvVars[baseVar] {
			continue
		}

		// If it's a system variable, check trigger availability
		isAllowed := false
		for triggerType := range triggerTypes {
			if availability, ok := triggerVarAvailability[triggerType]; ok {
				if availability[baseVar] {
					isAllowed = true
					break
				}
			}
		}

		if !isAllowed {
			return fmt.Errorf("function '%s' in tool '%s' uses system variable '%s' which is not available for its trigger types",
				function.Name, toolName, baseVar)
		}

		// If a field is specified, validate that it exists
		if field != "" {
			if fields, ok := systemVarFields[baseVar]; ok {
				if !fields[field] {
					return fmt.Errorf("function '%s' in tool '%s' references invalid field '%s' for system variable '%s'",
						function.Name, toolName, field, baseVar)
				}
			}
		}
	}

	// Check each used system function
	for funcName := range function.UsedSysFuncs {
		isAllowed := false

		// Check if function is allowed in any of the function's trigger types
		for triggerType := range triggerTypes {
			if availability, ok := triggerFuncAvailability[triggerType]; ok {
				if availability[funcName] {
					isAllowed = true
					break
				}
			}
		}

		if !isAllowed {
			return fmt.Errorf("function '%s' in tool '%s' uses system function '%s' which is not available for its trigger types",
				function.Name, toolName, funcName)
		}
	}

	return nil
}

// validateResultReferences checks if result references in parameters are valid
func validateResultReferences(function Function, toolName string) error {
	resultSteps := make(map[int]string)

	// First pass: identify all steps that store results
	for _, step := range function.Steps {
		if step.ResultIndex != 0 {
			// Check if this index is already used
			if existingStep, exists := resultSteps[step.ResultIndex]; exists {
				return fmt.Errorf("steps '%s' and '%s' in function '%s' of tool '%s' use the same resultIndex %d",
					existingStep, step.Name, function.Name, toolName, step.ResultIndex)
			}

			resultSteps[step.ResultIndex] = step.Name
		}
	}

	// Second pass: validate references to results in parameters
	for _, step := range function.Steps {
		if step.With != nil {
			if err := checkForResultReferences(step.With, resultSteps, step.Name, function.Name, toolName); err != nil {
				return err
			}
		}
	}

	return nil
}

// checkForResultReferences recursively checks for result references in parameters
func checkForResultReferences(params interface{}, validIndices map[int]string, stepName, functionName, toolName string) error {
	switch v := params.(type) {
	case string:
		// Check string for result references
		resultRefRegex := regexp.MustCompile(`result\[(\d+)\](\.[a-zA-Z0-9_]+)*`)
		matches := resultRefRegex.FindAllStringSubmatch(v, -1)

		for _, match := range matches {
			if len(match) >= 2 {
				indexStr := match[1]
				index, err := strconv.Atoi(indexStr)
				if err != nil {
					continue // Not a valid index, skip
				}

				// Ensure the referenced result exists
				if _, exists := validIndices[index]; !exists {
					return fmt.Errorf("step '%s' in function '%s' of tool '%s' references resultIndex %d which is not defined",
						stepName, functionName, toolName, index)
				}
			}
		}

	case map[string]interface{}:
		// Recursively check all values in the map
		for _, value := range v {
			if err := checkForResultReferences(value, validIndices, stepName, functionName, toolName); err != nil {
				return err
			}
		}

	case map[interface{}]interface{}:
		// Handle map[interface{}]interface{} which can occur in YAML parsing
		for _, value := range v {
			if err := checkForResultReferences(value, validIndices, stepName, functionName, toolName); err != nil {
				return err
			}
		}

	case []interface{}:
		// Recursively check all items in the array
		for _, item := range v {
			if err := checkForResultReferences(item, validIndices, stepName, functionName, toolName); err != nil {
				return err
			}
		}
	}

	return nil
}

// validateMCPOperation validates MCP-specific requirements
func validateMCPOperation(function Function, toolName string) error {
	if function.MCP == nil {
		return fmt.Errorf("function '%s' in tool '%s' has operation 'mcp' but missing mcp configuration",
			function.Name, toolName)
	}

	switch function.MCP.Protocol {
	case MCPProtocolStdio:
		if function.MCP.Stdio == nil {
			return fmt.Errorf("function '%s' in tool '%s' has MCP protocol 'stdio' but missing stdio configuration",
				function.Name, toolName)
		}
		if function.MCP.Stdio.Command == "" {
			return fmt.Errorf("function '%s' in tool '%s' MCP stdio configuration missing required 'command' field",
				function.Name, toolName)
		}
		// Args and Env are optional

	case MCPProtocolSSE:
		if function.MCP.SSE == nil {
			return fmt.Errorf("function '%s' in tool '%s' has MCP protocol 'sse' but missing sse configuration",
				function.Name, toolName)
		}
		if function.MCP.SSE.URL == "" {
			return fmt.Errorf("function '%s' in tool '%s' MCP sse configuration missing required 'url' field",
				function.Name, toolName)
		}

	default:
		return fmt.Errorf("function '%s' in tool '%s' has invalid MCP protocol '%s'; must be one of 'stdio' or 'sse'",
			function.Name, toolName, function.MCP.Protocol)
	}

	// Validate function name is specified
	if function.MCP.Function == "" {
		return fmt.Errorf("function '%s' in tool '%s' MCP configuration missing required 'function' field",
			function.Name, toolName)
	}

	// Validate inputs for MCP function
	// TODO: List tools and check the input schema. the user must fill all the required fields
	for i, input := range function.Input {
		// For MCP operations, all inputs should have onError with requestN1Support by default
		// if not explicitly defined
		if !input.IsOptional && input.OnError == nil {
			// Create default onError for MCP operations
			function.Input[i].OnError = &OnError{
				Strategy: OnErrorRequestN1Support,
				Message:  fmt.Sprintf("Required input '%s' for MCP function '%s' is missing", input.Name, function.Name),
			}
		}
	}

	return nil
}

// validatePDFOperation validates PDF-specific requirements
func validatePDFOperation(function Function, toolName string) error {
	if function.PDF == nil {
		return fmt.Errorf("function '%s' in tool '%s' has operation 'pdf' but missing pdf configuration",
			function.Name, toolName)
	}

	// Validate fileName is present
	if function.PDF.FileName == "" {
		return fmt.Errorf("function '%s' in tool '%s' pdf configuration missing required 'fileName' field",
			function.Name, toolName)
	}

	// Validate pageSize if specified
	if function.PDF.PageSize != "" {
		validPageSizes := map[string]bool{
			PDFPageSizeA4:     true,
			PDFPageSizeLetter: true,
			PDFPageSizeLegal:  true,
			PDFPageSizeA3:     true,
			PDFPageSizeA5:     true,
		}
		if !validPageSizes[function.PDF.PageSize] {
			return fmt.Errorf("function '%s' in tool '%s' pdf configuration has invalid pageSize '%s'; must be one of A4, Letter, Legal, A3, A5",
				function.Name, toolName, function.PDF.PageSize)
		}
	}

	// Validate orientation if specified
	if function.PDF.Orientation != "" {
		if function.PDF.Orientation != PDFOrientationPortrait && function.PDF.Orientation != PDFOrientationLandscape {
			return fmt.Errorf("function '%s' in tool '%s' pdf configuration has invalid orientation '%s'; must be 'portrait' or 'landscape'",
				function.Name, toolName, function.PDF.Orientation)
		}
	}

	// Validate at least one section exists
	if function.PDF.Header == nil && function.PDF.Body == nil && function.PDF.Footer == nil {
		return fmt.Errorf("function '%s' in tool '%s' pdf configuration must have at least one section (header, body, or footer)",
			function.Name, toolName)
	}

	// Validate each section
	if function.PDF.Header != nil {
		if err := validatePDFSection(function.PDF.Header, "header", function.Name, toolName); err != nil {
			return err
		}
	}
	if function.PDF.Body != nil {
		if err := validatePDFSection(function.PDF.Body, "body", function.Name, toolName); err != nil {
			return err
		}
	}
	if function.PDF.Footer != nil {
		if err := validatePDFSection(function.PDF.Footer, "footer", function.Name, toolName); err != nil {
			return err
		}
	}

	// Note: shouldBeHandledAsMessageToUser no longer requires upload: true.
	// Files can be sent to users via base64 fallback when upload is not configured.
	// Using upload: true is recommended for larger files (provides URL instead of inline base64).

	return nil
}

// validatePDFSection validates a PDF section (header, body, or footer)
func validatePDFSection(section *PDFSection, sectionName, functionName, toolName string) error {
	if len(section.Rows) == 0 {
		return fmt.Errorf("function '%s' in tool '%s' pdf %s must have at least one row",
			functionName, toolName, sectionName)
	}

	for rowIdx, row := range section.Rows {
		if len(row.Cols) == 0 {
			return fmt.Errorf("function '%s' in tool '%s' pdf %s row %d must have at least one column",
				functionName, toolName, sectionName, rowIdx+1)
		}

		// Validate column sizes sum to 12
		totalSize := 0
		for _, col := range row.Cols {
			if col.Size < 1 || col.Size > 12 {
				return fmt.Errorf("function '%s' in tool '%s' pdf %s row %d has column with invalid size %d; must be between 1 and 12",
					functionName, toolName, sectionName, rowIdx+1, col.Size)
			}
			totalSize += col.Size
		}
		if totalSize != 12 {
			return fmt.Errorf("function '%s' in tool '%s' pdf %s row %d column sizes sum to %d; must equal 12",
				functionName, toolName, sectionName, rowIdx+1, totalSize)
		}

		// Validate each column has at most one component
		for colIdx, col := range row.Cols {
			componentCount := 0
			if col.Text != nil {
				componentCount++
			}
			if col.Image != nil {
				componentCount++
			}
			if col.Table != nil {
				componentCount++
			}
			if col.Barcode != nil {
				componentCount++
			}
			if col.QRCode != nil {
				componentCount++
			}
			if col.Line != nil {
				componentCount++
			}
			if col.Signature != nil {
				componentCount++
			}

			if componentCount > 1 {
				return fmt.Errorf("function '%s' in tool '%s' pdf %s row %d column %d has %d components; each column must have at most one component",
					functionName, toolName, sectionName, rowIdx+1, colIdx+1, componentCount)
			}

			// Validate table if present
			if col.Table != nil {
				if len(col.Table.Headers) == 0 {
					return fmt.Errorf("function '%s' in tool '%s' pdf %s row %d column %d table must have headers",
						functionName, toolName, sectionName, rowIdx+1, colIdx+1)
				}
				if col.Table.Data == "" {
					return fmt.Errorf("function '%s' in tool '%s' pdf %s row %d column %d table must have data reference",
						functionName, toolName, sectionName, rowIdx+1, colIdx+1)
				}
				if len(col.Table.Columns) == 0 {
					return fmt.Errorf("function '%s' in tool '%s' pdf %s row %d column %d table must have column definitions",
						functionName, toolName, sectionName, rowIdx+1, colIdx+1)
				}
			}
		}
	}

	return nil
}

// validateGDriveOperation validates gdrive-specific operation requirements
func validateGDriveOperation(function Function, toolName string) error {
	if len(function.Steps) == 0 {
		return fmt.Errorf("function '%s' in tool '%s' has operation 'gdrive' but no steps defined",
			function.Name, toolName)
	}

	validActions := map[string]bool{
		StepActionGDriveList:         true,
		StepActionGDriveUpload:       true,
		StepActionGDriveDownload:     true,
		StepActionGDriveCreateFolder: true,
		StepActionGDriveDelete:       true,
		StepActionGDriveMove:         true,
		StepActionGDriveSearch:       true,
		StepActionGDriveGetMetadata:  true,
		StepActionGDriveUpdate:       true,
		StepActionGDriveExport:       true,
	}

	for _, step := range function.Steps {
		if step.Action == "" {
			return fmt.Errorf("step '%s' in function '%s' of tool '%s' is missing required 'action' field for gdrive operation",
				step.Name, function.Name, toolName)
		}
		if !validActions[step.Action] {
			return fmt.Errorf("step '%s' in function '%s' of tool '%s' has invalid gdrive action '%s'; must be one of list, upload, download, create_folder, delete, move, search, get_metadata, update, export",
				step.Name, function.Name, toolName, step.Action)
		}
	}

	return nil
}

// validateGDriveStep validates a single gdrive step's with block
func validateGDriveStep(step Step, functionName, toolName string) error {
	withBlock := step.With
	if withBlock == nil {
		withBlock = make(map[string]interface{})
	}

	switch step.Action {
	case StepActionGDriveList:
		// No required fields; optional: folderId, query, pageSize, orderBy
		if err := validateGDrivePageSize(withBlock, step.Name, functionName, toolName); err != nil {
			return err
		}

	case StepActionGDriveUpload:
		if err := requireGDriveWithField(withBlock, "fileName", step.Name, functionName, toolName); err != nil {
			return err
		}
		if err := requireGDriveWithField(withBlock, "content", step.Name, functionName, toolName); err != nil {
			return err
		}

	case StepActionGDriveDownload:
		if err := requireGDriveWithField(withBlock, "fileId", step.Name, functionName, toolName); err != nil {
			return err
		}

	case StepActionGDriveCreateFolder:
		if err := requireGDriveWithField(withBlock, "fileName", step.Name, functionName, toolName); err != nil {
			return err
		}

	case StepActionGDriveDelete:
		if err := requireGDriveWithField(withBlock, "fileId", step.Name, functionName, toolName); err != nil {
			return err
		}

	case StepActionGDriveMove:
		if err := requireGDriveWithField(withBlock, "fileId", step.Name, functionName, toolName); err != nil {
			return err
		}
		if err := requireGDriveWithField(withBlock, "targetFolderId", step.Name, functionName, toolName); err != nil {
			return err
		}

	case StepActionGDriveSearch:
		if err := requireGDriveWithField(withBlock, "query", step.Name, functionName, toolName); err != nil {
			return err
		}
		if err := validateGDrivePageSize(withBlock, step.Name, functionName, toolName); err != nil {
			return err
		}

	case StepActionGDriveGetMetadata:
		if err := requireGDriveWithField(withBlock, "fileId", step.Name, functionName, toolName); err != nil {
			return err
		}

	case StepActionGDriveUpdate:
		if err := requireGDriveWithField(withBlock, "fileId", step.Name, functionName, toolName); err != nil {
			return err
		}

	case StepActionGDriveExport:
		if err := requireGDriveWithField(withBlock, "fileId", step.Name, functionName, toolName); err != nil {
			return err
		}
		if err := requireGDriveWithField(withBlock, "mimeType", step.Name, functionName, toolName); err != nil {
			return err
		}
	}

	return nil
}

// requireGDriveWithField checks that a required field exists in the with block (may be a variable reference)
func requireGDriveWithField(withBlock map[string]interface{}, field, stepName, functionName, toolName string) error {
	val, exists := withBlock[field]
	if !exists {
		return fmt.Errorf("gdrive step '%s' in function '%s' of tool '%s' is missing required 'with.%s' field",
			stepName, functionName, toolName, field)
	}
	// Allow variable references ($varName) — they'll be resolved at runtime
	if str, ok := val.(string); ok && str == "" {
		return fmt.Errorf("gdrive step '%s' in function '%s' of tool '%s' has empty 'with.%s' field",
			stepName, functionName, toolName, field)
	}
	return nil
}

// validateGDrivePageSize validates pageSize if present in the with block
func validateGDrivePageSize(withBlock map[string]interface{}, stepName, functionName, toolName string) error {
	if ps, exists := withBlock["pageSize"]; exists {
		switch v := ps.(type) {
		case int:
			if v < 1 || v > MaxGDrivePageSize {
				return fmt.Errorf("gdrive step '%s' in function '%s' of tool '%s' has invalid pageSize %d; must be between 1 and %d",
					stepName, functionName, toolName, v, MaxGDrivePageSize)
			}
		case float64:
			iv := int(v)
			if iv < 1 || iv > MaxGDrivePageSize {
				return fmt.Errorf("gdrive step '%s' in function '%s' of tool '%s' has invalid pageSize %d; must be between 1 and %d",
					stepName, functionName, toolName, iv, MaxGDrivePageSize)
			}
		case string:
			// Variable reference — skip numeric validation
		}
	}
	return nil
}

func FindLastInferenceInputName(inputs []Input) string {
	for i := len(inputs) - 1; i >= 0; i-- {
		if inputs[i].Origin == DataOriginInference {
			return inputs[i].Name
		}
	}
	return ""
}

func validateTerminalOperation(function Function, toolName string) error {
	if len(function.Steps) == 0 {
		return fmt.Errorf("function '%s' in tool '%s' with operation 'terminal' must have at least one step",
			function.Name, toolName)
	}

	for _, step := range function.Steps {
		if step.Action != StepActionSh && step.Action != StepActionBash {
			return fmt.Errorf("function '%s' in tool '%s' with operation 'terminal' has invalid step action '%s'; must be 'sh' or 'bash'",
				function.Name, toolName, step.Action)
		}

		if step.With == nil {
			return fmt.Errorf("function '%s' in tool '%s' terminal step '%s' must have a 'with' block",
				function.Name, toolName, step.Name)
		}

		hasLinux := step.With[StepWithLinux] != nil
		hasWindows := step.With[StepWithWindows] != nil

		if !hasLinux || !hasWindows {
			return fmt.Errorf("function '%s' in tool '%s' terminal step '%s' must have both 'linux' and 'windows' scripts",
				function.Name, toolName, step.Name)
		}

		if err := validateTerminalScripts(step.With, function.Name, toolName, step.Name); err != nil {
			return err
		}
	}

	return nil
}

func validateTerminalScripts(stepWith map[string]interface{}, functionName, toolName, stepName string) error {
	osScripts := map[string]string{
		StepWithLinux:   "linux",
		StepWithMacOS:   "macos",
		StepWithWindows: "windows",
	}

	for field, osName := range osScripts {
		if script, exists := stepWith[field]; exists {
			if scriptStr, ok := script.(string); ok {
				if err := validateScriptSafety(scriptStr, osName, functionName, toolName, stepName); err != nil {
					return err
				}
			} else {
				return fmt.Errorf("function '%s' in tool '%s' step '%s' has invalid '%s' script: must be a string",
					functionName, toolName, stepName, osName)
			}
		}
	}

	return nil
}

// stripQuotedStrings removes all single and double quoted strings and bash comments
// from script content to prevent false positives in unsafe command detection.
// For example, "At Risk" should not trigger the "at " command detection.
// Comments like "# Get flight at rank" should also not trigger false positives.
// Handles escaped quotes within double-quoted strings.
func stripQuotedStrings(script string) string {
	// Remove bash comments (# to end of line)
	// This must be done first before quote stripping to handle comments correctly
	// Pattern matches: # followed by anything until end of line
	commentRegex := regexp.MustCompile(`(?m)#.*$`)
	script = commentRegex.ReplaceAllString(script, "")

	// Remove double-quoted strings (handling escaped quotes like \")
	// Pattern matches: opening ", then any chars except " or \, or escaped sequences, then closing "
	doubleQuoteRegex := regexp.MustCompile(`"(?:[^"\\]|\\.)*"`)
	script = doubleQuoteRegex.ReplaceAllString(script, `""`)

	// Remove single-quoted strings (in bash/sh, no escape sequences in single quotes)
	// Pattern matches: opening ', any chars except ', then closing '
	singleQuoteRegex := regexp.MustCompile(`'[^']*'`)
	script = singleQuoteRegex.ReplaceAllString(script, `''`)

	return script
}

func validateScriptSafety(script, osName, functionName, toolName, stepName string) error {
	// Strip quoted strings before checking for unsafe patterns
	// This prevents false positives for strings like "At Risk" or "do not use curl"
	scriptForCheck := stripQuotedStrings(script)

	unsafePatterns := []struct {
		pattern string
		command string
	}{
		{`\brm\s+-rf\b`, "rm -rf"},
		{`\bdel\s+/s\b`, "del /s"},
		//{`\bformat\b`, "format"},
		//{`\bmkfs\b`, "mkfs"},
		//{`\bdd\s+if=`, "dd if="},
		{`>\s*/dev/(sd|hd|nvme|loop|mapper)`, "> /dev/(disk devices)"}, // Block dangerous device writes, allow /dev/null, /dev/stderr, etc.
		{`\bchmod\s+-R\s+777\b`, "chmod -R 777"},
		{`\bwget\b`, "wget"},
		{`\bcurl\b`, "curl"},
		{`\bnc\b`, "nc"},
		{`\bnetcat\b`, "netcat"},
		{`\bssh\b`, "ssh"},
		{`\bftp\b`, "ftp"},
		{`\btelnet\b`, "telnet"},
		{`\bsudo\b`, "sudo"},
		{`\bsu\b`, "su"},
		{`\bchmod\s+\+s\b`, "chmod +s"},
		{`\bchown\b`, "chown"},
		{`\busermod\b`, "usermod"},
		{`\bpasswd\b`, "passwd"},
		{`\bkill\s+-9\b`, "kill -9"},
		{`\bkillall\b`, "killall"},
		{`\bpkill\b`, "pkill"},
		{`\btaskkill\s+/f\b`, "taskkill /f"},
		{`\bapt\s+install\b`, "apt install"},
		{`\byum\s+install\b`, "yum install"},
		{`\bpip\s+install\b`, "pip install"},
		{`\bnpm\s+install\s+-g\b`, "npm install -g"},
		{`\bexport\s+path=`, "export PATH="},
		{`\bunset\b`, "unset"},
		{`\bsource\s+/etc/`, "source /etc/"},
		{`>\s*/etc/`, "> /etc/"},
		{`>\s*C:\\Windows\\`, "> C:\\Windows\\"},
		{`\bshutdown\b`, "shutdown"},
		{`\breboot\b`, "reboot"},
		{`\bhalt\b`, "halt"},
		{`\bpoweroff\b`, "poweroff"},
		{`\bfdisk\b`, "fdisk"},
		{`\bparted\b`, "parted"},
		{`\bdiskpart\b`, "diskpart"},
		{`\bbcdedit\b`, "bcdedit"},
		{`\biptables\b`, "iptables"},
		{`\bnetsh\b`, "netsh"},
		{`\broute\s+add\b`, "route add"},
		{`\broute\s+del\b`, "route del"},
		{`\bcrontab\b`, "crontab"},
		{`\bschtasks\b`, "schtasks"},
		{`\bsystemctl\s+start\b`, "systemctl start"},
		{`\bsystemctl\s+stop\b`, "systemctl stop"},
		{`\bmount\b`, "mount"},
		{`\bumount\b`, "umount"},
		{`\bnet\s+use\b`, "net use"},
		{`\bnet\s+share\b`, "net share"},
		{`\bsqlite3\b`, "sqlite3"},
	}

	scriptLower := strings.ToLower(scriptForCheck)
	for _, pattern := range unsafePatterns {
		matched, err := regexp.MatchString(pattern.pattern, scriptLower)
		if err != nil {
			continue
		}
		if matched {
			return fmt.Errorf("function '%s' in tool '%s' step '%s' contains unsafe command '%s' in %s script",
				functionName, toolName, stepName, pattern.command, osName)
		}
	}

	return nil
}

// extractVariableNames extracts all variable names from a string (e.g., $varName, $varName.field)
// Returns the base variable names without the $ prefix or field accessors
func extractVariableNames(text string) []string {
	// First, strip escaped dollar patterns {$...} - these are literal dollar signs, not variables
	// Example: {$file} should become empty, not extract "file" as a variable
	escapedDollarRegex := regexp.MustCompile(`\{\$\$[^}]+\}`)
	text = escapedDollarRegex.ReplaceAllString(text, "")

	// Match $varName or $varName.field or $varName[0].field
	// Also supports optional date transformation functions: .toISO() and .toDDMMYYYY() at the end
	varRegex := regexp.MustCompile(`\$([a-zA-Z0-9_]+)(?:\.[a-zA-Z0-9_]+|\[[^\]]+\])*(?:\.(toISO|toDDMMYYYY)\(\))?`)
	matches := varRegex.FindAllStringSubmatch(text, -1)

	seen := make(map[string]bool)
	var vars []string

	for _, match := range matches {
		if len(match) > 1 {
			varName := match[1]
			// Skip system variables (these are always available)
			systemVars := map[string]bool{
				"ME": true, "MESSAGE": true, "NOW": true, "USER": true,
				"ADMIN": true, "COMPANY": true, "UUID": true, "FILE": true,
				"MEETING": true,
			}
			if systemVars[varName] {
				continue
			}
			// Skip result[] references as they're handled separately
			if varName == "result" {
				continue
			}
			if !seen[varName] {
				seen[varName] = true
				vars = append(vars, varName)
			}
		}
	}

	return vars
}

// validateStepVariableReferences validates that all variables referenced in step parameters
// have corresponding input definitions. This applies to ALL operations (terminal, db, api, etc.)
func validateStepVariableReferences(stepWith map[string]interface{}, function Function, tool Tool, configEnvVars []EnvVar, step Step) error {
	// Build a map of valid input names
	validInputs := make(map[string]bool)
	for _, input := range function.Input {
		validInputs[input.Name] = true
	}

	// Build a map of valid environment variables from config
	validEnvVars := make(map[string]bool)
	for _, env := range configEnvVars {
		validEnvVars[env.Name] = true
	}

	// Check if any step in the function has extractAuthToken configured
	// If so, AUTH_TOKEN becomes a valid variable for subsequent steps
	hasExtractAuthToken := false
	for _, s := range function.Steps {
		if s.With != nil {
			if _, hasConfig := s.With[StepWithExtractAuthToken]; hasConfig {
				hasExtractAuthToken = true
				break
			}
		}
	}
	if hasExtractAuthToken {
		validInputs["AUTH_TOKEN"] = true
	}

	// If this step has a foreach, add the itemVar and indexVar as valid variables
	if step.ForEach != nil {
		// Default itemVar is "item" if not specified
		itemVar := step.ForEach.ItemVar
		if itemVar == "" {
			itemVar = "item"
		}
		validInputs[itemVar] = true

		// Default indexVar is "index" if not specified
		indexVar := step.ForEach.IndexVar
		if indexVar == "" {
			indexVar = "index"
		}
		validInputs[indexVar] = true
	}

	// Extract variables from all step parameters recursively
	vars := extractVariablesFromStepParams(stepWith)

	// Check each variable has a corresponding input or is a defined environment variable
	for _, varName := range vars {
		// Check if it's a valid input (or foreach variable)
		if validInputs[varName] {
			continue
		}

		// Check if it's a defined environment variable
		if validEnvVars[varName] {
			continue
		}

		// Variable is neither an input nor a defined environment variable - error
		return fmt.Errorf(
			"function '%s' in tool '%s' step '%s' references variable '$%s' which has no corresponding input definition.\n"+
				"Variables in steps can only reference inputs, not functions from 'needs' directly.\n"+
				"To use a function result, add an input with origin: 'function' and from: '%s'",
			function.Name, tool.Name, step.Name, varName, varName)
	}

	return nil
}

// extractVariablesFromStepParams recursively extracts variables from step parameters
func extractVariablesFromStepParams(params map[string]interface{}) []string {
	var allVars []string
	seen := make(map[string]bool)

	var extract func(interface{})
	extract = func(value interface{}) {
		switch v := value.(type) {
		case string:
			vars := extractVariableNames(v)
			for _, varName := range vars {
				if !seen[varName] {
					seen[varName] = true
					allVars = append(allVars, varName)
				}
			}
		case map[string]interface{}:
			for _, val := range v {
				extract(val)
			}
		case map[interface{}]interface{}:
			for _, val := range v {
				extract(val)
			}
		case []interface{}:
			for _, item := range v {
				extract(item)
			}
		}
	}

	extract(params)
	return allVars
}

func validateTerminalStep(step Step, functionName, toolName string) error {
	allowedFields := map[string]bool{
		StepWithLinux:   true,
		StepWithMacOS:   true,
		StepWithWindows: true,
		StepWithTimeout: true,
	}

	if step.With != nil {
		if err := validateAllowedFields(step.With, allowedFields, step.Name, functionName, toolName); err != nil {
			return err
		}
	}

	validActions := map[string]bool{
		StepActionSh:   true,
		StepActionBash: true,
	}

	if !validActions[step.Action] {
		return fmt.Errorf("step '%s' in function '%s' of tool '%s' has invalid action '%s' for terminal operation; must be 'sh' or 'bash'",
			step.Name, functionName, toolName, step.Action)
	}

	if step.With == nil {
		return fmt.Errorf("step '%s' in function '%s' of tool '%s' must have a 'with' block containing scripts",
			step.Name, functionName, toolName)
	}

	hasLinux := step.With[StepWithLinux] != nil
	hasWindows := step.With[StepWithWindows] != nil

	if !hasLinux || !hasWindows {
		return fmt.Errorf("step '%s' in function '%s' of tool '%s' must have both 'linux' and 'windows' scripts",
			step.Name, functionName, toolName)
	}

	if timeout, exists := step.With[StepWithTimeout]; exists {
		if timeoutInt, ok := timeout.(int); ok {
			if timeoutInt <= 0 {
				return fmt.Errorf("step '%s' in function '%s' of tool '%s' has invalid timeout value %d; must be greater than 0",
					step.Name, functionName, toolName, timeoutInt)
			}
		} else {
			return fmt.Errorf("step '%s' in function '%s' of tool '%s' has invalid timeout type; must be an integer",
				step.Name, functionName, toolName)
		}
	}

	if err := validateTerminalScripts(step.With, functionName, toolName, step.Name); err != nil {
		return err
	}

	// If action is "sh", check for bash-specific syntax that won't work in POSIX sh
	if step.Action == StepActionSh {
		if err := validateNoBashisms(step.With, functionName, toolName, step.Name); err != nil {
			return err
		}
	}

	return nil
}

// validateCodeOperation validates a code operation's function-level configuration
func validateCodeOperation(function Function, toolName string) error {
	if len(function.Steps) == 0 {
		return fmt.Errorf("function '%s' in tool '%s' with operation 'code' must have at least one step",
			function.Name, toolName)
	}

	for _, step := range function.Steps {
		if step.Action != StepActionPrompt {
			return fmt.Errorf("function '%s' in tool '%s' with operation 'code' has invalid step action '%s'; must be 'prompt'",
				function.Name, toolName, step.Action)
		}

		// reuseSession is incompatible with forEach — forEach creates independent processes per iteration
		if function.ReuseSession && step.ForEach != nil {
			return fmt.Errorf("function '%s' in tool '%s' code step '%s' has forEach but reuseSession is true; forEach is not supported with reuseSession because each iteration needs an independent session",
				function.Name, toolName, step.Name)
		}

		if step.With == nil {
			return fmt.Errorf("function '%s' in tool '%s' code step '%s' must have a 'with' block",
				function.Name, toolName, step.Name)
		}

		prompt, hasPrompt := step.With[StepWithPrompt]
		if !hasPrompt {
			return fmt.Errorf("function '%s' in tool '%s' code step '%s' must have a 'prompt' field in with block",
				function.Name, toolName, step.Name)
		}
		if promptStr, ok := prompt.(string); !ok || promptStr == "" {
			return fmt.Errorf("function '%s' in tool '%s' code step '%s' prompt must be a non-empty string",
				function.Name, toolName, step.Name)
		}

		// Validate maxTurns bounds
		if maxTurns, exists := step.With[StepWithMaxTurns]; exists {
			maxTurnsVal, ok := toInt(maxTurns)
			if !ok {
				return fmt.Errorf("function '%s' in tool '%s' code step '%s' maxTurns must be an integer",
					function.Name, toolName, step.Name)
			}
			if maxTurnsVal < 1 || maxTurnsVal > MaxCodeMaxTurns {
				return fmt.Errorf("function '%s' in tool '%s' code step '%s' maxTurns must be between 1 and %d",
					function.Name, toolName, step.Name, MaxCodeMaxTurns)
			}
		}

		// Validate timeout bounds
		if timeout, exists := step.With[StepWithTimeout]; exists {
			timeoutVal, ok := toInt(timeout)
			if !ok {
				return fmt.Errorf("function '%s' in tool '%s' code step '%s' timeout must be an integer",
					function.Name, toolName, step.Name)
			}
			if timeoutVal < 1 || timeoutVal > MaxCodeTimeout {
				return fmt.Errorf("function '%s' in tool '%s' code step '%s' timeout must be between 1 and %d",
					function.Name, toolName, step.Name, MaxCodeTimeout)
			}
		}

		// Validate isPlanMode is bool
		if isPlanMode, exists := step.With[StepWithIsPlanMode]; exists {
			if _, ok := isPlanMode.(bool); !ok {
				return fmt.Errorf("function '%s' in tool '%s' code step '%s' isPlanMode must be a boolean",
					function.Name, toolName, step.Name)
			}
		}

		// Validate taskComplexityLevel is a valid string
		if level, exists := step.With[StepWithTaskComplexityLevel]; exists {
			levelStr, ok := level.(string)
			if !ok {
				return fmt.Errorf("function '%s' in tool '%s' code step '%s' taskComplexityLevel must be a string",
					function.Name, toolName, step.Name)
			}
			validLevels := map[string]bool{TaskComplexityLow: true, TaskComplexityMedium: true, TaskComplexityHigh: true}
			if !validLevels[strings.ToLower(levelStr)] {
				return fmt.Errorf("function '%s' in tool '%s' code step '%s' taskComplexityLevel must be 'low', 'medium', or 'high' (got '%s')",
					function.Name, toolName, step.Name, levelStr)
			}
		}

		// Validate tool lists
		if err := validateCodeToolsList(step.With, StepWithAllowedTools, function.Name, toolName, step.Name); err != nil {
			return err
		}
		if err := validateCodeToolsList(step.With, StepWithDisallowedTools, function.Name, toolName, step.Name); err != nil {
			return err
		}
		if err := validateCodeToolsList(step.With, StepWithAdditionalDirs, function.Name, toolName, step.Name); err != nil {
			return err
		}
	}

	return nil
}

// validateCodeStep validates a single code operation step
func validateCodeStep(step Step, functionName, toolName string) error {
	allowedFields := map[string]bool{
		StepWithPrompt:              true,
		StepWithCwd:                 true,
		StepWithModel:               true,
		StepWithMaxTurns:            true,
		StepWithSystemPrompt:        true,
		StepWithAllowedTools:        true,
		StepWithDisallowedTools:     true,
		StepWithIsPlanMode:          true,
		StepWithAdditionalDirs:      true,
		StepWithTimeout:             true,
		StepWithTaskComplexityLevel: true,
	}

	if step.With != nil {
		if err := validateAllowedFields(step.With, allowedFields, step.Name, functionName, toolName); err != nil {
			return err
		}
	}

	if step.Action != StepActionPrompt {
		return fmt.Errorf("step '%s' in function '%s' of tool '%s' has invalid action '%s' for code operation; must be 'prompt'",
			step.Name, functionName, toolName, step.Action)
	}

	if step.With == nil {
		return fmt.Errorf("step '%s' in function '%s' of tool '%s' must have a 'with' block containing a prompt",
			step.Name, functionName, toolName)
	}

	prompt, hasPrompt := step.With[StepWithPrompt]
	if !hasPrompt {
		return fmt.Errorf("step '%s' in function '%s' of tool '%s' must have a 'prompt' field",
			step.Name, functionName, toolName)
	}
	if promptStr, ok := prompt.(string); !ok || promptStr == "" {
		return fmt.Errorf("step '%s' in function '%s' of tool '%s' prompt must be a non-empty string",
			step.Name, functionName, toolName)
	}

	return nil
}

// validateCodeToolsList validates that a tools list field (allowedTools, disallowedTools, additionalDirs) is a []string
func validateCodeToolsList(stepWith map[string]interface{}, fieldName, functionName, toolName, stepName string) error {
	raw, exists := stepWith[fieldName]
	if !exists || raw == nil {
		return nil
	}

	switch v := raw.(type) {
	case []interface{}:
		for i, item := range v {
			if _, ok := item.(string); !ok {
				return fmt.Errorf("function '%s' in tool '%s' code step '%s' %s[%d] must be a string",
					functionName, toolName, stepName, fieldName, i)
			}
		}
	case []string:
		// Already valid
	default:
		return fmt.Errorf("function '%s' in tool '%s' code step '%s' %s must be an array of strings",
			functionName, toolName, stepName, fieldName)
	}

	return nil
}

// toInt converts an interface{} to int, handling both int and float64 (from YAML parsing)
func toInt(v interface{}) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case float64:
		return int(n), true
	case int64:
		return int(n), true
	default:
		return 0, false
	}
}

// validateNoBashisms checks if scripts use bash-specific syntax when action is "sh"
// These features work in bash but not in POSIX sh (like Alpine's ash/busybox sh)
func validateNoBashisms(stepWith map[string]interface{}, functionName, toolName, stepName string) error {
	bashisms := []struct {
		pattern     string
		description string
	}{
		{`<<<`, "here-strings (<<<)"},
		{`\[\[`, "extended test ([[ ]])"},
		{`\$\{[^}]+//`, "pattern substitution (${var//pattern/replacement})"},
		{`\$\{[^}]+:[0-9]`, "substring expansion (${var:offset:length})"},
		{`<\(`, "process substitution (<(cmd))"},
		{`>\(`, "process substitution (>(cmd))"},
		{`&>`, "combined redirect (&>)"},
		{`\|&`, "pipe with stderr (|&)"},
		{`\bdeclare\s`, "declare keyword"},
		{`\blocal\s+-[aA]`, "local arrays (local -a/-A)"},
		{`\bshopt\s`, "shopt command"},
		{`\bread\s+-a`, "read into array (read -a)"},
		{`\$'[^']*'`, "ANSI-C quoting ($'...')"},
	}

	osScripts := map[string]string{
		StepWithLinux: "linux",
		StepWithMacOS: "macos",
	}

	for field, osName := range osScripts {
		if script, exists := stepWith[field]; exists {
			if scriptStr, ok := script.(string); ok {
				for _, bashism := range bashisms {
					matched, err := regexp.MatchString(bashism.pattern, scriptStr)
					if err != nil {
						continue
					}
					if matched {
						return fmt.Errorf("step '%s' in function '%s' of tool '%s' uses bash-specific syntax %s in %s script but action is 'sh'; change action to 'bash' or use POSIX-compatible syntax",
							stepName, functionName, toolName, bashism.description, osName)
					}
				}
			}
		}
	}

	return nil
}

// validateAllowedFields checks that only allowed fields are present in the given map
func validateAllowedFields(fields map[string]interface{}, allowedFields map[string]bool, stepName, functionName, toolName string) error {
	for field := range fields {
		if !allowedFields[field] {
			return fmt.Errorf("step '%s' in function '%s' of tool '%s' has an invalid field in 'with' block: '%s'",
				stepName, functionName, toolName, field)
		}
	}
	return nil
}

// checkForMisplacedWorkflows checks if workflows are incorrectly placed inside tool definitions
// and returns warnings for any found. This catches a common mistake where users define
// workflows inside the tools array instead of at the top level.
func checkForMisplacedWorkflows(yamlContent string) []string {
	var warnings []string

	// Parse into a raw map structure to detect unknown fields
	var rawConfig map[string]interface{}
	if err := yaml.Unmarshal([]byte(yamlContent), &rawConfig); err != nil {
		// If we can't parse, let the main parser handle the error
		return warnings
	}

	// Check each tool in the tools array
	toolsRaw, ok := rawConfig["tools"]
	if !ok {
		return warnings
	}

	toolsList, ok := toolsRaw.([]interface{})
	if !ok {
		return warnings
	}

	for i, toolRaw := range toolsList {
		// yaml.v2 returns map[interface{}]interface{} for nested maps
		toolMap, ok := toolRaw.(map[interface{}]interface{})
		if !ok {
			continue
		}

		// Get tool name for better error messages
		toolName := fmt.Sprintf("tool at index %d", i)
		if name, ok := toolMap["name"].(string); ok && name != "" {
			toolName = fmt.Sprintf("tool '%s'", name)
		}

		// Check if workflows field exists inside the tool
		if _, hasWorkflows := toolMap["workflows"]; hasWorkflows {
			warnings = append(warnings, fmt.Sprintf(
				"WARNING: 'workflows' found inside %s - workflows should be defined at the TOP LEVEL of the YAML file, "+
					"not inside tool definitions. Move the workflows section to be a sibling of 'version', 'author', and 'tools'. "+
					"The correct structure is:\n"+
					"  version: \"v1\"\n"+
					"  author: \"...\"\n"+
					"  tools:\n"+
					"    - name: \"...\"\n"+
					"      ...\n"+
					"  workflows:  # <-- workflows at top level\n"+
					"    - category: \"...\"\n"+
					"      ...",
				toolName))
		}
	}

	return warnings
}

// validateWorkflows validates all workflow definitions
func validateWorkflows(workflows []WorkflowYAML, tools []Tool) error {
	if len(workflows) == 0 {
		// Workflows are optional
		return nil
	}

	// Build map of available functions (PUBLIC functions from tools + system functions)
	availableFunctions := make(map[string]bool)

	// Add public functions from all tools
	// A PUBLIC function is one that starts with an uppercase letter
	for _, tool := range tools {
		for _, fn := range tool.Functions {
			// Check if function name starts with uppercase letter
			if len(fn.Name) > 0 && fn.Name[0] >= 'A' && fn.Name[0] <= 'Z' {
				availableFunctions[fn.Name] = true
			}
		}
	}

	// Add system functions
	systemFunctions := []string{
		"requestInternalTeamInfo",
		"requestInternalTeamAction",
		"askToTheConversationHistoryWithCustomer",
		"askToKnowledgeBase",
		"findProperImages",
		"casualAnswering",
	}
	for _, sysFunc := range systemFunctions {
		availableFunctions[sysFunc] = true
	}

	// Check for unique workflow category names
	categoryNames := make(map[string]bool)
	for i, workflow := range workflows {
		if err := validateWorkflow(workflow, i, availableFunctions, categoryNames); err != nil {
			return err
		}
	}

	return nil
}

// validateWorkflow validates a single workflow definition
func validateWorkflow(workflow WorkflowYAML, index int, availableFunctions map[string]bool, categoryNames map[string]bool) error {
	// Validate category name
	if workflow.CategoryName == "" {
		return fmt.Errorf("workflow at index %d has an empty category name", index)
	}

	// Check for duplicate category names
	if categoryNames[workflow.CategoryName] {
		return fmt.Errorf("duplicate workflow category name '%s'", workflow.CategoryName)
	}
	categoryNames[workflow.CategoryName] = true

	// Validate category name follows snake_case convention
	if !isValidSnakeCase(workflow.CategoryName) {
		return fmt.Errorf("workflow category '%s' must be in snake_case format (lowercase with underscores)", workflow.CategoryName)
	}

	// Validate human-readable category name
	if workflow.HumanReadableCategoryName == "" {
		return fmt.Errorf("workflow '%s' has an empty human_category", workflow.CategoryName)
	}

	// Validate description
	if workflow.Description == "" {
		return fmt.Errorf("workflow '%s' has an empty description", workflow.CategoryName)
	}

	// Validate workflow_type (required field)
	if workflow.WorkflowType == "" {
		return fmt.Errorf("workflow '%s' is missing required field 'workflow_type'; must be '%s' or '%s'",
			workflow.CategoryName, WorkflowTypeUser, WorkflowTypeTeam)
	}
	if workflow.WorkflowType != WorkflowTypeUser && workflow.WorkflowType != WorkflowTypeTeam {
		return fmt.Errorf("workflow '%s' has invalid workflow_type '%s'; must be '%s' or '%s'",
			workflow.CategoryName, workflow.WorkflowType, WorkflowTypeUser, WorkflowTypeTeam)
	}

	// Validate steps
	if len(workflow.Steps) == 0 {
		return fmt.Errorf("workflow '%s' must have at least one step", workflow.CategoryName)
	}

	// Validate each step
	stepOrders := make(map[int]bool)
	for _, step := range workflow.Steps {
		if err := validateWorkflowStep(step, workflow.CategoryName, availableFunctions, stepOrders); err != nil {
			return err
		}
	}

	return nil
}

// validateWorkflowStep validates a single workflow step
func validateWorkflowStep(step WorkflowStepYAML, workflowCategory string, availableFunctions map[string]bool, stepOrders map[int]bool) error {
	// Validate order
	if step.Order < 0 {
		return fmt.Errorf("workflow '%s' has a step with invalid order %d; order must be >= 0", workflowCategory, step.Order)
	}

	// Check for duplicate orders
	if stepOrders[step.Order] {
		return fmt.Errorf("workflow '%s' has duplicate step order %d", workflowCategory, step.Order)
	}
	stepOrders[step.Order] = true

	// Validate action
	if step.Action == "" {
		return fmt.Errorf("workflow '%s' has a step at order %d with an empty action", workflowCategory, step.Order)
	}

	// Verify action is a valid function (PUBLIC or system)
	if !availableFunctions[step.Action] {
		return fmt.Errorf("workflow '%s' step %d references unknown action '%s'; must be a PUBLIC function or system function",
			workflowCategory, step.Order, step.Action)
	}

	// Validate human-readable action name
	if step.HumanReadableActionName == "" {
		return fmt.Errorf("workflow '%s' step %d has an empty human_action", workflowCategory, step.Order)
	}

	// Validate instructions
	if step.Instructions == "" {
		return fmt.Errorf("workflow '%s' step %d has empty instructions", workflowCategory, step.Order)
	}

	return nil
}

// isValidSnakeCase checks if a string follows snake_case convention
func isValidSnakeCase(s string) bool {
	if s == "" {
		return false
	}

	// Must start with lowercase letter
	if s[0] < 'a' || s[0] > 'z' {
		return false
	}

	// Can only contain lowercase letters, digits, and underscores
	for _, ch := range s {
		if !((ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '_') {
			return false
		}
	}

	// Cannot have consecutive underscores
	if strings.Contains(s, "__") {
		return false
	}

	// Cannot end with underscore
	if s[len(s)-1] == '_' {
		return false
	}

	return true
}

// validateInitiateWorkflowOperation validates initiate_workflow-specific requirements
func validateInitiateWorkflowOperation(function Function, tool Tool, rootWorkflows []WorkflowYAML) error {
	toolName := tool.Name

	// Must be used with time_based or flex_for_user triggers
	// - time_based: for cron-based workflow initiation
	// - flex_for_user: for hook callbacks that initiate workflows (e.g., escalation notifications)
	hasValidTrigger := false
	for _, trigger := range function.Triggers {
		if trigger.Type == TriggerTime || trigger.Type == TriggerFlexForUser || trigger.Type == TriggerFlexForTeam {
			hasValidTrigger = true
		} else {
			return fmt.Errorf("function '%s' in tool '%s' with operation 'initiate_workflow' can only be used with time_based, flex_for_user, or flex_for_team triggers, found trigger type '%s'",
				function.Name, toolName, trigger.Type)
		}
	}

	if !hasValidTrigger {
		return fmt.Errorf("function '%s' in tool '%s' with operation 'initiate_workflow' must have at least one time_based, flex_for_user, or flex_for_team trigger",
			function.Name, toolName)
	}

	// Must have at least one step
	if len(function.Steps) == 0 {
		return fmt.Errorf("function '%s' in tool '%s' with operation 'initiate_workflow' must have at least one step",
			function.Name, toolName)
	}

	// Validate each step
	for _, step := range function.Steps {
		if step.Action != StepActionStartWorkflow {
			return fmt.Errorf("function '%s' in tool '%s' with operation 'initiate_workflow' has invalid step action '%s'; must be 'start_workflow'",
				function.Name, toolName, step.Action)
		}

		if step.With == nil {
			return fmt.Errorf("function '%s' in tool '%s' initiate_workflow step '%s' must have a 'with' block",
				function.Name, toolName, step.Name)
		}

		// Check if using userId or user object - must have one or the other
		_, hasUserId := step.With["userId"]
		_, hasUser := step.With["user"]

		if !hasUserId && !hasUser {
			return fmt.Errorf("function '%s' in tool '%s' initiate_workflow step '%s' must have either 'userId' or 'user' field",
				function.Name, toolName, step.Name)
		}

		if hasUserId && hasUser {
			return fmt.Errorf("function '%s' in tool '%s' initiate_workflow step '%s' cannot have both 'userId' and 'user' fields; use one or the other",
				function.Name, toolName, step.Name)
		}

		// Validate user object if present
		if hasUser {
			if err := validateInitiateWorkflowUserObject(step.With["user"], function.Name, toolName, step.Name); err != nil {
				return err
			}
		}

		// Validate other required fields
		requiredFields := []string{"workflowType", "message", "context"}
		for _, field := range requiredFields {
			if _, exists := step.With[field]; !exists {
				return fmt.Errorf("function '%s' in tool '%s' initiate_workflow step '%s' missing required field: '%s'",
					function.Name, toolName, step.Name, field)
			}
		}

		// Validate context field structure (can be string or object with value+params)
		if err := validateInitiateWorkflowContext(step.With["context"], function.Name, toolName, step.Name); err != nil {
			return err
		}

		// Validate workflowType if it's a literal value (not a variable reference)
		workflowType, workflowTypeIsString := step.With["workflowType"].(string)
		workflowTypeIsLiteral := workflowTypeIsString && !strings.HasPrefix(workflowType, "$")
		if workflowTypeIsLiteral {
			if workflowType != "user" && workflowType != "team" {
				return fmt.Errorf("function '%s' in tool '%s' initiate_workflow step '%s' has invalid workflowType '%s'; must be 'user' or 'team'",
					function.Name, toolName, step.Name, workflowType)
			}
		}

		// Validate context.params function references against workflowType and tool functions
		if workflowTypeIsLiteral {
			if err := validateContextParamsFunctionReferences(step.With["context"], workflowType, function.Name, tool, step.Name); err != nil {
				return err
			}
		}

		// Validate shouldSend if present
		if shouldSend, exists := step.With["shouldSend"]; exists {
			if _, ok := shouldSend.(bool); !ok {
				// Check if it's a string that could be a variable
				if strVal, ok := shouldSend.(string); !ok || !strings.HasPrefix(strVal, "$") {
					return fmt.Errorf("function '%s' in tool '%s' initiate_workflow step '%s' has invalid shouldSend value; must be a boolean or variable reference",
						function.Name, toolName, step.Name)
				}
			}
		}

		// Validate workflow field if present - must match a workflow defined in the tool
		if workflowRaw, exists := step.With["workflow"]; exists {
			workflowName, ok := workflowRaw.(string)
			if !ok {
				return fmt.Errorf("function '%s' in tool '%s' initiate_workflow step '%s' has invalid workflow value; must be a string or variable reference",
					function.Name, toolName, step.Name)
			}
			// Skip validation for variable references (e.g., "$item.workflow")
			if !strings.HasPrefix(workflowName, "$") {
				// Validate the workflow exists in the root-level workflows section
				if err := validateInitiateWorkflowWorkflowField(workflowName, function.Name, toolName, step.Name, rootWorkflows); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

// validateInitiateWorkflowUserObject validates the user object in initiate_workflow
func validateInitiateWorkflowUserObject(userObj interface{}, functionName, toolName, stepName string) error {
	// Handle variable reference (e.g., "$item.user")
	if strVal, ok := userObj.(string); ok {
		if strings.HasPrefix(strVal, "$") {
			return nil // Variable reference, will be resolved at runtime
		}
		return fmt.Errorf("function '%s' in tool '%s' initiate_workflow step '%s' has invalid user value; must be an object or variable reference",
			functionName, toolName, stepName)
	}

	// Handle object - YAML may parse as map[interface{}]interface{} or map[string]interface{}
	userMap, ok := userObj.(map[string]interface{})
	if !ok {
		// Try map[interface{}]interface{} which YAML uses for nested objects
		interfaceMap, ok := userObj.(map[interface{}]interface{})
		if !ok {
			return fmt.Errorf("function '%s' in tool '%s' initiate_workflow step '%s' has invalid user value; must be an object with 'channel' field",
				functionName, toolName, stepName)
		}
		// Convert to map[string]interface{}
		userMap = make(map[string]interface{})
		for k, v := range interfaceMap {
			kStr, ok := k.(string)
			if !ok {
				return fmt.Errorf("function '%s' in tool '%s' initiate_workflow step '%s' user object has a non-string key",
					functionName, toolName, stepName)
			}
			userMap[kStr] = v
		}
	}

	// Channel is required
	channel, hasChannel := userMap["channel"]
	if !hasChannel {
		return fmt.Errorf("function '%s' in tool '%s' initiate_workflow step '%s' user object missing required field: 'channel'",
			functionName, toolName, stepName)
	}

	// Validate channel value if it's a literal (not a variable)
	if channelStr, ok := channel.(string); ok && !strings.HasPrefix(channelStr, "$") {
		validChannels := map[string]bool{
			"Waha":             true, // WhatsApp via Waha (case-sensitive!)
			"Widget":           true,
			"API_OFFICIAL":     true,
			"instagram":        true,
			"Teams":            true,
			"whatsapp_cloud":   true,
			"messenger":        true,
			"telegram":         true,
			"email":            true,
			"webchat":          true,
			"slack":            true,
			"facebook":         true,
			"none":             true,
			"WebClient":        true,
			"local_agent_chat": true,
			"synthetic":        true, // Allow synthetic for internal workflows
		}
		if !validChannels[channelStr] {
			// Check for common case-sensitivity mistakes
			for valid := range validChannels {
				if strings.EqualFold(channelStr, valid) {
					return fmt.Errorf("function '%s' in tool '%s' initiate_workflow step '%s' user object has invalid channel '%s'; did you mean '%s'? (channel values are case-sensitive)",
						functionName, toolName, stepName, channelStr, valid)
				}
			}
			return fmt.Errorf("function '%s' in tool '%s' initiate_workflow step '%s' user object has invalid channel '%s'; valid channels are: Waha, Widget, API_OFFICIAL, instagram, Teams, whatsapp_cloud, messenger, telegram, email, webchat, slack, facebook, none, WebClient, local_agent_chat, synthetic",
				functionName, toolName, stepName, channelStr)
		}
	}

	// Validate optional fields types if present (they can also be variables)
	optionalStringFields := []string{"email", "phoneNumber", "firstName", "lastName", "sessionId"}
	for _, field := range optionalStringFields {
		if val, exists := userMap[field]; exists {
			if strVal, ok := val.(string); ok {
				// Either a literal string or variable reference - both are valid
				_ = strVal
			} else {
				return fmt.Errorf("function '%s' in tool '%s' initiate_workflow step '%s' user object field '%s' must be a string",
					functionName, toolName, stepName, field)
			}
		}
	}

	return nil
}

// validateInitiateWorkflowWorkflowField validates the workflow field in initiate_workflow
// The workflow must match one of the workflows defined in the root-level workflows section
func validateInitiateWorkflowWorkflowField(workflowName, functionName, toolName, stepName string, rootWorkflows []WorkflowYAML) error {
	// Build a map of valid workflow categories from the root-level workflows
	validWorkflows := make(map[string]bool)
	for _, wf := range rootWorkflows {
		validWorkflows[wf.CategoryName] = true
	}

	// Check if the workflow exists
	if !validWorkflows[workflowName] {
		// Collect valid workflow names for error message
		validNames := make([]string, 0, len(validWorkflows))
		for name := range validWorkflows {
			validNames = append(validNames, name)
		}
		if len(validNames) == 0 {
			return fmt.Errorf("function '%s' in tool '%s' initiate_workflow step '%s' specifies workflow '%s', but no workflows are defined in the YAML file",
				functionName, toolName, stepName, workflowName)
		}
		return fmt.Errorf("function '%s' in tool '%s' initiate_workflow step '%s' specifies unknown workflow '%s'; valid workflows are: %v",
			functionName, toolName, stepName, workflowName, validNames)
	}

	return nil
}

// validateInitiateWorkflowContext validates the context field in initiate_workflow
// Context can be either a simple string or an object with value and params fields
func validateInitiateWorkflowContext(contextRaw interface{}, functionName, toolName, stepName string) error {
	if contextRaw == nil {
		return fmt.Errorf("function '%s' in tool '%s' initiate_workflow step '%s' context cannot be nil",
			functionName, toolName, stepName)
	}

	// Simple string format (backward compatible)
	if _, ok := contextRaw.(string); ok {
		return nil
	}

	// Object format: { value: "...", params: [...] }
	contextMap, ok := contextRaw.(map[string]interface{})
	if !ok {
		// Try map[interface{}]interface{} from YAML parsing
		if ifaceMap, ok := contextRaw.(map[interface{}]interface{}); ok {
			contextMap = make(map[string]interface{})
			for k, v := range ifaceMap {
				if keyStr, ok := k.(string); ok {
					contextMap[keyStr] = v
				}
			}
		} else {
			return fmt.Errorf("function '%s' in tool '%s' initiate_workflow step '%s' context must be a string or an object with 'value' field",
				functionName, toolName, stepName)
		}
	}

	// Value is required in object format
	value, hasValue := contextMap["value"]
	if !hasValue {
		return fmt.Errorf("function '%s' in tool '%s' initiate_workflow step '%s' context object must have a 'value' field",
			functionName, toolName, stepName)
	}
	if _, ok := value.(string); !ok {
		return fmt.Errorf("function '%s' in tool '%s' initiate_workflow step '%s' context.value must be a string",
			functionName, toolName, stepName)
	}

	// Params is optional
	params, hasParams := contextMap["params"]
	if !hasParams || params == nil {
		return nil
	}

	// Params must be an array
	paramsSlice, ok := params.([]interface{})
	if !ok {
		return fmt.Errorf("function '%s' in tool '%s' initiate_workflow step '%s' context.params must be an array",
			functionName, toolName, stepName)
	}

	// Validate each param entry
	for i, paramRaw := range paramsSlice {
		paramMap, ok := paramRaw.(map[string]interface{})
		if !ok {
			// Try map[interface{}]interface{} from YAML parsing
			if ifaceMap, ok := paramRaw.(map[interface{}]interface{}); ok {
				paramMap = make(map[string]interface{})
				for k, v := range ifaceMap {
					if keyStr, ok := k.(string); ok {
						paramMap[keyStr] = v
					}
				}
			} else {
				return fmt.Errorf("function '%s' in tool '%s' initiate_workflow step '%s' context.params[%d] must be an object",
					functionName, toolName, stepName, i)
			}
		}

		// Function is required
		functionField, hasFunction := paramMap["function"]
		if !hasFunction {
			return fmt.Errorf("function '%s' in tool '%s' initiate_workflow step '%s' context.params[%d] must have a 'function' field",
				functionName, toolName, stepName, i)
		}
		funcStr, ok := functionField.(string)
		if !ok || funcStr == "" {
			return fmt.Errorf("function '%s' in tool '%s' initiate_workflow step '%s' context.params[%d].function must be a non-empty string",
				functionName, toolName, stepName, i)
		}

		// Inputs is required
		inputs, hasInputs := paramMap["inputs"]
		if !hasInputs {
			return fmt.Errorf("function '%s' in tool '%s' initiate_workflow step '%s' context.params[%d] must have an 'inputs' field",
				functionName, toolName, stepName, i)
		}

		// Inputs must be a map
		_, isMap1 := inputs.(map[string]interface{})
		_, isMap2 := inputs.(map[interface{}]interface{})
		if !isMap1 && !isMap2 {
			return fmt.Errorf("function '%s' in tool '%s' initiate_workflow step '%s' context.params[%d].inputs must be an object",
				functionName, toolName, stepName, i)
		}

		// Note: We don't validate input names here because:
		// Input validation requires knowing the target function's input definitions
		// These validations happen at runtime when all tools are loaded
		//
		// However, we CAN validate function existence and trigger compatibility
		// for functions within the same tool (local references) - this is done
		// separately in validateContextParamsFunctionReferences
	}

	return nil
}

// validateContextParamsFunctionReferences validates that functions referenced in context.params
// exist in the tool and have triggers compatible with the workflow type.
// This catches errors like referencing "ToolName.FunctionName" where FunctionName doesn't exist
// or has incompatible triggers (e.g., flex_for_user when workflowType is "team").
func validateContextParamsFunctionReferences(contextRaw interface{}, workflowType, functionName string, tool Tool, stepName string) error {
	if contextRaw == nil {
		return nil
	}

	// Simple string context has no params to validate
	if _, ok := contextRaw.(string); ok {
		return nil
	}

	// Get context map
	contextMap, ok := contextRaw.(map[string]interface{})
	if !ok {
		// Try map[interface{}]interface{} from YAML parsing
		if ifaceMap, ok := contextRaw.(map[interface{}]interface{}); ok {
			contextMap = make(map[string]interface{})
			for k, v := range ifaceMap {
				if keyStr, ok := k.(string); ok {
					contextMap[keyStr] = v
				}
			}
		} else {
			return nil // Already validated structure in validateInitiateWorkflowContext
		}
	}

	// Get params array
	params, hasParams := contextMap["params"]
	if !hasParams || params == nil {
		return nil
	}

	paramsSlice, ok := params.([]interface{})
	if !ok {
		return nil // Already validated in validateInitiateWorkflowContext
	}

	toolName := tool.Name

	// Build a map of functions in this tool for quick lookup
	toolFunctions := make(map[string]*Function)
	for i := range tool.Functions {
		toolFunctions[tool.Functions[i].Name] = &tool.Functions[i]
	}

	// Determine which triggers are compatible with this workflow type
	var compatibleTriggers []string
	if workflowType == WorkflowTypeUser {
		compatibleTriggers = []string{TriggerFlexForUser, TriggerOnUserMessage, TriggerOnCompletedUserMessage}
	} else if workflowType == WorkflowTypeTeam {
		compatibleTriggers = []string{TriggerFlexForTeam, TriggerOnTeamMessage, TriggerOnCompletedTeamMessage}
	}

	for i, paramRaw := range paramsSlice {
		paramMap, ok := paramRaw.(map[string]interface{})
		if !ok {
			if ifaceMap, ok := paramRaw.(map[interface{}]interface{}); ok {
				paramMap = make(map[string]interface{})
				for k, v := range ifaceMap {
					if keyStr, ok := k.(string); ok {
						paramMap[keyStr] = v
					}
				}
			} else {
				continue // Already validated in validateInitiateWorkflowContext
			}
		}

		functionField, ok := paramMap["function"].(string)
		if !ok || functionField == "" {
			continue // Already validated in validateInitiateWorkflowContext
		}

		// Parse the function reference: "ToolName.FunctionName" or just "FunctionName"
		refToolName, refFuncName := ParseContextParamFunctionKey(functionField)

		// Only validate if this is a reference to a function in the same tool
		if refToolName != "" && refToolName != toolName {
			// Reference to another tool - we can't validate it here
			// as the other tool may not be loaded yet
			continue
		}

		// Look up the function in this tool
		targetFunc, exists := toolFunctions[refFuncName]
		if !exists {
			return fmt.Errorf("function '%s' in tool '%s' initiate_workflow step '%s' context.params[%d] references function '%s' which does not exist in tool '%s'",
				functionName, toolName, stepName, i, refFuncName, toolName)
		}

		// Check if the target function has a compatible trigger for the workflow type
		hasCompatibleTrigger := false
		var actualTriggers []string
		for _, trigger := range targetFunc.Triggers {
			actualTriggers = append(actualTriggers, trigger.Type)
			for _, compatibleTrigger := range compatibleTriggers {
				if trigger.Type == compatibleTrigger {
					hasCompatibleTrigger = true
					break
				}
			}
			if hasCompatibleTrigger {
				break
			}
		}

		if !hasCompatibleTrigger {
			var expectedTriggersStr string
			if workflowType == WorkflowTypeUser {
				expectedTriggersStr = "flex_for_user or always_on_user_message"
			} else {
				expectedTriggersStr = "flex_for_team or always_on_team_message"
			}

			actualTriggersStr := "none"
			if len(actualTriggers) > 0 {
				actualTriggersStr = strings.Join(actualTriggers, ", ")
			}

			return fmt.Errorf("function '%s' in tool '%s' initiate_workflow step '%s' context.params[%d] references function '%s' which has triggers [%s], but workflowType '%s' requires one of: %s. Add the appropriate trigger to function '%s'",
				functionName, toolName, stepName, i, refFuncName, actualTriggersStr, workflowType, expectedTriggersStr, refFuncName)
		}
	}

	return nil
}

// ParseContextParamFunctionKey parses a function reference like "ToolName.FunctionName" or "FunctionName"
// Returns (toolName, functionName) where toolName may be empty for local references
func ParseContextParamFunctionKey(functionKey string) (toolName string, functionName string) {
	parts := strings.SplitN(functionKey, ".", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return "", functionKey
}

// validateRequiresInitiateWorkflowReferences validates that functions with requiresInitiateWorkflow: true
// are referenced by at least one initiate_workflow function via context.params
func validateRequiresInitiateWorkflowReferences(tool Tool) error {
	// Collect all functions with requiresInitiateWorkflow: true
	requiresWorkflow := make(map[string]bool)
	for _, fn := range tool.Functions {
		if fn.RequiresInitiateWorkflow {
			requiresWorkflow[fn.Name] = true
		}
	}

	if len(requiresWorkflow) == 0 {
		return nil // No functions require workflow context
	}

	// Scan all initiate_workflow functions for context.params references
	referencedFunctions := make(map[string]bool)
	for _, fn := range tool.Functions {
		if fn.Operation != OperationInitiateWorkflow {
			continue
		}
		for _, step := range fn.Steps {
			if step.With == nil {
				continue
			}
			contextRaw := step.With["context"]
			// Extract function references from context.params
			refs := extractContextParamsFunctionRefs(contextRaw, tool.Name)
			for _, ref := range refs {
				referencedFunctions[ref] = true
			}
		}
	}

	// Check each requiresWorkflow function is referenced
	for funcName := range requiresWorkflow {
		if !referencedFunctions[funcName] {
			return fmt.Errorf("function '%s' in tool '%s' has requiresInitiateWorkflow: true but is not referenced by any initiate_workflow function via context.params. Add a context.params entry like: params: [{function: \"%s.%s\", inputs: {...}}]",
				funcName, tool.Name, tool.Name, funcName)
		}
	}

	return nil
}

// extractContextParamsFunctionRefs extracts function names from context.params
// Returns slice of function names (without tool prefix) for functions in this tool
func extractContextParamsFunctionRefs(contextRaw interface{}, toolName string) []string {
	var refs []string

	if contextRaw == nil {
		return refs
	}

	// Simple string context has no params
	if _, ok := contextRaw.(string); ok {
		return refs
	}

	// Get context map
	contextMap, ok := contextRaw.(map[string]interface{})
	if !ok {
		// Try map[interface{}]interface{} from YAML parsing
		if ifaceMap, ok := contextRaw.(map[interface{}]interface{}); ok {
			contextMap = make(map[string]interface{})
			for k, v := range ifaceMap {
				if keyStr, ok := k.(string); ok {
					contextMap[keyStr] = v
				}
			}
		} else {
			return refs
		}
	}

	// Get params array
	params, hasParams := contextMap["params"]
	if !hasParams || params == nil {
		return refs
	}

	paramsSlice, ok := params.([]interface{})
	if !ok {
		return refs
	}

	for _, paramRaw := range paramsSlice {
		paramMap, ok := paramRaw.(map[string]interface{})
		if !ok {
			if ifaceMap, ok := paramRaw.(map[interface{}]interface{}); ok {
				paramMap = make(map[string]interface{})
				for k, v := range ifaceMap {
					if keyStr, ok := k.(string); ok {
						paramMap[keyStr] = v
					}
				}
			} else {
				continue
			}
		}

		functionField, ok := paramMap["function"].(string)
		if !ok || functionField == "" {
			continue
		}

		// Parse the function reference: "ToolName.FunctionName" or just "FunctionName"
		refToolName, refFuncName := ParseContextParamFunctionKey(functionField)

		// Only include functions from this tool (or local references without tool prefix)
		if refToolName == "" || refToolName == toolName {
			refs = append(refs, refFuncName)
		}
	}

	return refs
}
