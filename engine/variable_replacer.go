package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	tool_engine_models "github.com/bbiangul/mantis-skill/engine/models"
	"github.com/bbiangul/mantis-skill/skill"
	"github.com/google/uuid"
)

// ---------------------------------------------------------------------------
// Context keys for variable replacement (supplement triggers.go keys)
// ---------------------------------------------------------------------------

const (
	// UserInContextKey carries the types.User through the context.
	UserInContextKey contextKeyType = "userIDInContextKey"

	// AdminInContextKey carries the admin *types.User through the context.
	AdminInContextKey contextKeyType = "adminInContextKey"

	// CompanyInContextKey carries the *CompanyInfo through the context.
	CompanyInContextKey contextKeyType = "companyInContextKey"
)

// CompanyInfo holds company-level metadata for $COMPANY variable resolution.
// This replaces the connect-ai domain/models.CompanyInfo singleton.
type CompanyInfo struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	FantasyName      string `json:"fantasy_name"`
	TaxCode          string `json:"tax_code"` // CNPJ equivalent
	Industry         string `json:"industry"`
	Email            string `json:"email"`
	InstagramProfile string `json:"instagram_profile"`
	Website          string `json:"website"`
	AISessionID      string `json:"ai_session_id"`
}

// ---------------------------------------------------------------------------
// EmailContext — email channel variable resolution
// ---------------------------------------------------------------------------

// EmailContext holds email-specific information for $EMAIL variable.
// This is populated when processing email channel messages.
type EmailContext struct {
	ThreadID       string   // Email thread/conversation identifier
	MessageID      string   // Unique email message ID
	Subject        string   // Email subject line
	Sender         string   // Sender email address (from)
	Recipients     []string // List of TO addresses
	CC             []string // List of CC addresses
	BCC            []string // List of BCC addresses
	InReplyTo      string   // Message ID of the email being replied to
	References     []string // Related message IDs for threading
	Date           string   // Email date/timestamp (ISO8601)
	TextBody       string   // Plain text body content
	HasAttachments bool     // Whether email has attachments
}

// emailContextKeyType is an unexported type for the email context key.
type emailContextKeyType struct{}

// EmailContextKey is the context key for email information.
var EmailContextKey = emailContextKeyType{}

// ---------------------------------------------------------------------------
// Local narrow interfaces — avoid importing engine/models (import cycle)
// ---------------------------------------------------------------------------

// variableReplacerTracker is the subset of IFunctionExecutionTracker that
// the VariableReplacer actually calls. Any concrete tracker satisfying the
// full interface also satisfies this.
type variableReplacerTracker interface {
	GetStepResult(ctx context.Context, messageID string, resultIndex int) (*StepResult, error)
	GetStepResultField(ctx context.Context, messageID string, resultIndex int, fieldPath string) (interface{}, error)
}

// variableReplacerTempFileManager is the subset of ITempFileManager used here.
type variableReplacerTempFileManager interface {
	GetTempFileBase64(path string) (string, error)
}

// variableReplacerUserRepo is the subset of UserRepository used here.
type variableReplacerUserRepo interface {
	GetUser(ctx context.Context, userID string) (*User, error)
}

// variableReplacerMessageCounter counts user messages (optional).
type variableReplacerMessageCounter interface {
	CountUserMessages(ctx context.Context, userID string) (int, error)
}

// LoopContext is a type alias for models.LoopContext so that VariableReplacer
// satisfies models.IVariableReplacer without creating an import cycle.
type LoopContext = tool_engine_models.LoopContext

// ReplaceOptions is a type alias for models.ReplaceOptions so that
// VariableReplacer satisfies models.IVariableReplacer.
type ReplaceOptions = tool_engine_models.ReplaceOptions

// ---------------------------------------------------------------------------
// Package-level compiled regexes for performance (avoid recompilation in loops)
// ---------------------------------------------------------------------------

var (
	resultIndexRegex = regexp.MustCompile(`\$?result\[([^]]+)\]`)
)

// shellEscapeValue escapes single quotes in a string for safe embedding in bash single-quoted strings.
func shellEscapeValue(s string) string {
	return strings.ReplaceAll(s, "'", `'\''`)
}

// ---------------------------------------------------------------------------
// VariableReplacer
// ---------------------------------------------------------------------------

// Ensure VariableReplacer satisfies models.IVariableReplacer at compile time.
var _ tool_engine_models.IVariableReplacer = (*VariableReplacer)(nil)

// VariableReplacer handles the substitution of variables in strings.
type VariableReplacer struct {
	executionTracker    variableReplacerTracker
	envVars             map[string]string    // Store environment variables
	expressionEvaluator *ExpressionEvaluator // Evaluates function expressions like coalesce()
	authToken           string               // Runtime auth token set by extractAuthToken in isAuthentication steps
	tempFileManager     variableReplacerTempFileManager

	// Optional providers — host application injects these.
	userRepo       variableReplacerUserRepo
	messageCounter variableReplacerMessageCounter

	// Me identity — configurable by the host application.
	meName        string
	meVersion     string
	meDescription string
}

// NewVariableReplacer creates a new VariableReplacer.
func NewVariableReplacer(executionTracker variableReplacerTracker) *VariableReplacer {
	return &VariableReplacer{
		executionTracker:    executionTracker,
		envVars:             make(map[string]string),
		expressionEvaluator: NewExpressionEvaluator(),
		meName:              "Assistant",
		meVersion:           "1.0.0",
		meDescription:       "AI assistant",
	}
}

// SetEnvironmentVariables sets the environment variables from a tool definition.
func (r *VariableReplacer) SetEnvironmentVariables(envVars []skill.EnvVar) {
	for _, env := range envVars {
		r.envVars[env.Name] = env.Value
	}
}

// GetEnvironmentVariables returns all environment variables.
func (r *VariableReplacer) GetEnvironmentVariables() map[string]string {
	return r.envVars
}

// SetAuthToken sets the runtime authentication token (extracted from extractAuthToken).
func (r *VariableReplacer) SetAuthToken(token string) {
	r.authToken = token
}

// GetAuthToken returns the current authentication token.
func (r *VariableReplacer) GetAuthToken() string {
	return r.authToken
}

// ClearAuthToken clears the authentication token (used on unauthorized errors).
func (r *VariableReplacer) ClearAuthToken() {
	r.authToken = ""
}

// SetTempFileManager sets the temporary file manager for base64 file access.
func (r *VariableReplacer) SetTempFileManager(manager variableReplacerTempFileManager) {
	r.tempFileManager = manager
}

// SetUserRepository sets the user repository for $USER variable resolution.
func (r *VariableReplacer) SetUserRepository(repo variableReplacerUserRepo) {
	r.userRepo = repo
}

// SetMessageCounter sets the message counter for $USER.messages_count.
func (r *VariableReplacer) SetMessageCounter(counter variableReplacerMessageCounter) {
	r.messageCounter = counter
}

// SetMeIdentity configures the $ME variable.
func (r *VariableReplacer) SetMeIdentity(name, version, description string) {
	r.meName = name
	r.meVersion = version
	r.meDescription = description
}

// ---------------------------------------------------------------------------
// Public replacement API
// ---------------------------------------------------------------------------

// ReplaceVariables replaces all types of variables in a string.
// Default behavior: unresolved variables become empty string (safe for API calls).
func (r *VariableReplacer) ReplaceVariables(ctx context.Context, text string, inputs map[string]interface{}) (string, error) {
	return r.ReplaceVariablesWithOptions(ctx, text, inputs, ReplaceOptions{
		DBContext:                  false,
		KeepUnresolvedPlaceholders: false,
	})
}

// ReplaceVariablesWithContext replaces all types of variables with optional DB context for NULL handling.
func (r *VariableReplacer) ReplaceVariablesWithContext(ctx context.Context, text string, inputs map[string]interface{}, dbContext bool) (string, error) {
	return r.ReplaceVariablesWithOptions(ctx, text, inputs, ReplaceOptions{
		DBContext:                  dbContext,
		KeepUnresolvedPlaceholders: false,
	})
}

// ReplaceVariablesWithOptions replaces all types of variables with full control over behavior.
// Supports {$$var} escape: use {$$varName} to produce a literal $varName in the output.
func (r *VariableReplacer) ReplaceVariablesWithOptions(ctx context.Context, text string, inputs map[string]interface{}, opts ReplaceOptions) (string, error) {
	// Protect {$$...} escape sequences from variable replacement.
	text = r.protectEscapedDollars(text)

	// First, replace all environment variables
	text, err := r.replaceEnvVariables(text)
	if err != nil {
		return "", fmt.Errorf("error replacing environment variables: %w", err)
	}

	// Next, replace all system variables
	text, err = r.replaceSystemVariables(ctx, text)
	if err != nil {
		return "", fmt.Errorf("error replacing system variables: %w", err)
	}

	// Then replace input variables (with options for DB context and unresolved handling)
	text = r.replaceInputVariables(text, inputs, opts.DBContext, opts.KeepUnresolvedPlaceholders, opts.ShellEscape)

	// Finally, replace function results references (check inputs map first for onSuccess params)
	text, err = r.replaceFunctionResults(ctx, text, inputs, opts.ShellEscape)
	if err != nil {
		return "", fmt.Errorf("error replacing function results: %w", err)
	}

	// Evaluate function expressions like coalesce()
	text = r.expressionEvaluator.Evaluate(text)

	// Restore escaped dollar signs: sentinel -> $
	text = r.restoreEscapedDollars(text)

	return text, nil
}

// ReplaceVariablesWithLoop replaces variables including loop context variables.
// Default behavior: unresolved variables become empty string (safe for API calls).
func (r *VariableReplacer) ReplaceVariablesWithLoop(ctx context.Context, text string, inputs map[string]interface{}, loopContext *LoopContext) (string, error) {
	// Protect {$$...} escape sequences from variable replacement
	text = r.protectEscapedDollars(text)

	// First, replace all environment variables
	text, err := r.replaceEnvVariables(text)
	if err != nil {
		return "", fmt.Errorf("error replacing environment variables: %w", err)
	}

	// Next, replace all system variables
	text, err = r.replaceSystemVariables(ctx, text)
	if err != nil {
		return "", fmt.Errorf("error replacing system variables: %w", err)
	}

	// Merge loop context variables with inputs
	mergedInputs := make(map[string]interface{})
	if inputs != nil {
		for k, v := range inputs {
			mergedInputs[k] = v
		}
	}

	if loopContext != nil {
		mergedInputs[loopContext.IndexVar] = loopContext.Index
		mergedInputs[loopContext.ItemVar] = loopContext.Item
		// Add result if available (for waitFor context)
		if loopContext.Result != nil {
			mergedInputs["result"] = loopContext.Result
		}
	}

	// Then replace input variables (including loop vars)
	text = r.replaceInputVariables(text, mergedInputs, false, false, false)

	// Finally, replace function results references
	text, err = r.replaceFunctionResults(ctx, text, mergedInputs, false)
	if err != nil {
		return "", fmt.Errorf("error replacing function results: %w", err)
	}

	// Evaluate function expressions like coalesce()
	text = r.expressionEvaluator.Evaluate(text)

	// Restore escaped dollar signs
	text = r.restoreEscapedDollars(text)

	return text, nil
}

// ---------------------------------------------------------------------------
// Environment variables
// ---------------------------------------------------------------------------

func (r *VariableReplacer) replaceEnvVariables(text string) (string, error) {
	envVarRegex := regexp.MustCompile(`\$([A-Z][A-Z0-9_]*)`)
	matches := envVarRegex.FindAllStringSubmatch(text, -1)

	// Sort matches longest-first to prevent substring collisions
	sort.Slice(matches, func(i, j int) bool {
		return len(matches[i][0]) > len(matches[j][0])
	})

	for _, match := range matches {
		fullMatch := match[0]
		varName := match[1]

		// Check for special runtime variable AUTH_TOKEN first
		if varName == "AUTH_TOKEN" {
			if r.authToken != "" {
				text = strings.Replace(text, fullMatch, r.authToken, -1)
			}
			continue
		}

		// Check if we have this env var
		value, exists := r.envVars[varName]
		if !exists {
			continue
		}

		text = strings.Replace(text, fullMatch, value, -1)
	}

	return text, nil
}

// ---------------------------------------------------------------------------
// System variables
// ---------------------------------------------------------------------------

func (r *VariableReplacer) replaceSystemVariables(ctx context.Context, text string) (string, error) {
	varRegex := regexp.MustCompile(`\$([A-Z_]+)(?:\.([a-zA-Z0-9_]+))?`)
	matches := varRegex.FindAllStringSubmatch(text, -1)

	processed := make(map[string]string)

	for _, match := range matches {
		fullMatch := match[0]
		varName := "$" + match[1]

		if _, ok := processed[fullMatch]; ok {
			continue
		}

		var replacement string
		var err error

		var field string
		if len(match) > 2 {
			field = match[2]
		}

		switch varName {
		case "$ME":
			replacement, err = r.getMeVariable(field)
		case "$MESSAGE":
			replacement, err = r.getMessageVariable(ctx, field)
		case "$NOW":
			replacement, err = r.getNowVariable(field)
		case "$USER":
			replacement, err = r.getUserVariable(ctx, field)
		case "$ADMIN":
			replacement, err = r.getAdminVariable(ctx, field)
		case "$COMPANY":
			replacement, err = r.getCompanyVariable(ctx, field)
		case "$UUID":
			replacement, err = r.getUuidVariable(field)
		case "$FILE":
			replacement, err = r.getFileVariable(ctx, field)
		case "$MEETING":
			replacement, err = r.getMeetingVariable(ctx, field)
		case "$EMAIL":
			replacement, err = r.getEmailVariable(ctx, field)
		case "$TEMP_DIR":
			replacement, err = r.getTempDirVariable(field)
		default:
			continue
		}

		if err != nil {
			if logger != nil && strings.HasPrefix(fullMatch, "$USER") {
				logger.Warnf("Error processing variable %s: %v - skipping replacement", fullMatch, err)
			}
			continue
		}

		processed[fullMatch] = replacement
		text = strings.Replace(text, fullMatch, replacement, -1)
	}

	return text, nil
}

// ---------------------------------------------------------------------------
// Input variables
// ---------------------------------------------------------------------------

// replaceInputVariables replaces input variables with their values, including dot notation.
func (r *VariableReplacer) replaceInputVariables(text string, inputs map[string]interface{}, dbContext bool, keepUnresolved bool, shellEscape bool) string {
	if inputs == nil {
		return text
	}

	processedReplacements := make(map[string]string)

	// Handle complex variable references: $var.field, $var[0].field, etc.
	complexVarRegex := regexp.MustCompile(`\$([a-zA-Z0-9_]+(?:\[[^\]]+\]|\.)[\[\]a-zA-Z0-9_\.]*(?:\(\))?)`)
	matches := complexVarRegex.FindAllStringSubmatch(text, -1)

	for _, match := range matches {
		if len(match) < 2 {
			continue
		}

		fullMatch := "$" + match[1]
		refPath := match[1]

		firstDotOrBracket := strings.IndexAny(refPath, ".[")
		if firstDotOrBracket == -1 {
			continue
		}

		varName := refPath[:firstDotOrBracket]
		fieldPath := refPath[firstDotOrBracket:]

		// Skip $result[n] patterns - handled by replaceFunctionResults
		if varName == "result" && strings.HasPrefix(fieldPath, "[") {
			continue
		}

		if strings.HasPrefix(fieldPath, ".") {
			fieldPath = fieldPath[1:]
		}

		// Check for date transformation function at the end of the path
		cleanFieldPath, transformFunc, hasTransform := extractDateTransformation(fieldPath)

		baseValue, exists := inputs[varName]
		if !exists {
			if keepUnresolved {
				continue
			}
			processedReplacements[fullMatch] = ""
			continue
		}

		var strValue string
		if baseValue == nil && dbContext {
			strValue = "NULL"
		} else if baseValue == nil {
			strValue = ""
		} else {
			var navigableValue interface{} = baseValue
			if strBase, ok := baseValue.(string); ok {
				if parsedData, ok := tryParseJSON(strBase); ok {
					navigableValue = parsedData
				}
			}

			fieldValue, err := r.NavigatePath(navigableValue, cleanFieldPath)
			if err != nil {
				if keepUnresolved {
					continue
				}
				if _, isStr := baseValue.(string); isStr {
					_, isMap := navigableValue.(map[string]interface{})
					_, isSlice := navigableValue.([]interface{})
					if !isMap && !isSlice {
						processedReplacements["$"+varName] = fmt.Sprintf("%v", baseValue)
						continue
					}
				}
				fieldValue = nil
			}

			if fieldValue == nil && dbContext {
				strValue = "NULL"
			} else if fieldValue == nil {
				strValue = ""
			} else {
				switch v := fieldValue.(type) {
				case string:
					strValue = v
				case fmt.Stringer:
					strValue = v.String()
				default:
					strValue = formatValueAsString(v)
				}
			}

			if hasTransform && strValue != "" && strValue != "NULL" {
				strValue, _ = applyDateTransformation(strValue, transformFunc)
			}
		}

		processedReplacements[fullMatch] = strValue
	}

	// Handle simple variables ($var) without dot notation
	simpleVarRegex := regexp.MustCompile(`\$([a-zA-Z0-9_]+)`)
	simpleMatches := simpleVarRegex.FindAllStringSubmatchIndex(text, -1)

	for _, match := range simpleMatches {
		if len(match) < 4 {
			continue
		}

		fullMatchStart := match[0]
		fullMatchEnd := match[1]
		varNameStart := match[2]
		varNameEnd := match[3]

		fullMatch := text[fullMatchStart:fullMatchEnd]
		varName := text[varNameStart:varNameEnd]

		if fullMatchEnd < len(text) {
			nextChar := text[fullMatchEnd]
			if nextChar == '.' || nextChar == '[' {
				continue
			}
		}

		if _, alreadyProcessed := processedReplacements[fullMatch]; alreadyProcessed {
			continue
		}

		value, exists := inputs[varName]
		if !exists {
			if keepUnresolved {
				continue
			}
			processedReplacements[fullMatch] = ""
			continue
		}

		var strValue string
		if value == nil && dbContext {
			strValue = "NULL"
		} else if value == nil {
			strValue = ""
		} else {
			switch v := value.(type) {
			case string:
				strValue = v
			case fmt.Stringer:
				strValue = v.String()
			default:
				strValue = formatValueAsString(v)
			}
		}

		processedReplacements[fullMatch] = strValue
	}

	// Apply all replacements at once, longest first
	sortedRefs := make([]string, 0, len(processedReplacements))
	for varRef := range processedReplacements {
		sortedRefs = append(sortedRefs, varRef)
	}
	sort.Slice(sortedRefs, func(i, j int) bool {
		return len(sortedRefs[i]) > len(sortedRefs[j])
	})

	for _, varRef := range sortedRefs {
		value := processedReplacements[varRef]
		if shellEscape && value != "" {
			value = shellEscapeValue(value)
		}
		text = strings.Replace(text, varRef, value, -1)
	}

	return text
}

// ---------------------------------------------------------------------------
// Function results
// ---------------------------------------------------------------------------

func (r *VariableReplacer) replaceFunctionResults(ctx context.Context, text string, inputs map[string]interface{}, shellEscape bool) (string, error) {
	resultRegex := regexp.MustCompile(`\$?result\[([^]]+)\](?:\[[^]]+\]|\.[a-zA-Z0-9_]+(?:\[[^]]+\])?)*`)
	complexMatches := resultRegex.FindAllString(text, -1)

	if len(complexMatches) == 0 {
		return text, nil
	}

	if logger != nil {
		logger.Debugf("replaceFunctionResults - Found %d result references in text: %s", len(complexMatches), text)
	}

	messageID, messageIDErr := r.getMessageIDFromContext(ctx)
	if messageIDErr != nil {
		if logger != nil {
			logger.Debugf("replaceFunctionResults - No messageID in context, will use inputs map only: %v", messageIDErr)
		}
	}

	for _, fullMatch := range complexMatches {
		indexMatch := resultIndexRegex.FindStringSubmatch(fullMatch)
		if len(indexMatch) < 2 {
			continue
		}

		indexStr := indexMatch[1]
		index, err := strconv.Atoi(indexStr)
		if err != nil {
			return "", fmt.Errorf("invalid result index in '%s': %v", fullMatch, err)
		}

		path := strings.TrimPrefix(fullMatch, fmt.Sprintf("$result[%d]", index))
		if path == fullMatch {
			path = strings.TrimPrefix(fullMatch, fmt.Sprintf("result[%d]", index))
		}

		if strings.HasPrefix(path, "[") {
			// path stays as-is
		} else if strings.HasPrefix(path, ".") {
			path = path[1:]
		} else if path == "" {
			path = ""
		}

		// First, check if result[N] exists in inputs map
		var resultValue interface{}
		var fetchErr error
		inputKey := fmt.Sprintf("result[%d]", index)
		if inputs != nil {
			if inputVal, exists := inputs[inputKey]; exists {
				if logger != nil {
					logger.Debugf("replaceFunctionResults - Found result[%d] in inputs map, value type: %T", index, inputVal)
				}
				if path == "" {
					resultValue = inputVal
				} else {
					var data interface{}
					if strVal, ok := inputVal.(string); ok {
						if parsed, ok := tryParseJSON(strVal); ok {
							data = parsed
						} else {
							data = inputVal
						}
					} else {
						data = inputVal
					}
					var navErr error
					resultValue, navErr = r.NavigatePath(data, path)
					if navErr != nil {
						if logger != nil {
							logger.Errorf("replaceFunctionResults - Failed to navigate path %s in inputs: %v", path, navErr)
						}
						return "", fmt.Errorf("error navigating path %s: %w", fullMatch, navErr)
					}
				}
				goto replaceValue
			}
		}

		// Fall back to database lookup via getResultValueByPath
		if messageIDErr != nil {
			if logger != nil {
				logger.Debugf("replaceFunctionResults - result[%d] not in inputs and no messageID for DB lookup, skipping", index)
			}
			continue
		}
		resultValue, fetchErr = r.getResultValueByPath(ctx, messageID, index, path)
		if fetchErr != nil {
			if logger != nil {
				logger.Errorf("replaceFunctionResults - Failed to retrieve %s: %v", fullMatch, fetchErr)
			}
			return "", fmt.Errorf("error retrieving %s: %w", fullMatch, fetchErr)
		}

	replaceValue:
		if logger != nil {
			logger.Debugf("replaceFunctionResults - Successfully retrieved result[%d], value type: %T", index, resultValue)
		}

		strValue := formatValueAsString(resultValue)
		if shellEscape && strValue != "" {
			strValue = shellEscapeValue(strValue)
		}
		text = strings.Replace(text, fullMatch, strValue, -1)
	}

	return text, nil
}

func (r *VariableReplacer) getResultValueByPath(ctx context.Context, messageID string, index int, path string) (interface{}, error) {
	if r.executionTracker == nil {
		return nil, fmt.Errorf("execution tracker is not initialized")
	}

	stepResult, err := r.executionTracker.GetStepResult(ctx, messageID, index)
	if err != nil {
		return nil, err
	}

	var resultDataStr string
	resultValue := reflect.ValueOf(stepResult)
	if resultValue.Kind() == reflect.Ptr {
		resultValue = resultValue.Elem()
	}

	resultDataField := resultValue.FieldByName("ResultData")
	if !resultDataField.IsValid() {
		return nil, fmt.Errorf("result does not contain ResultData field")
	}
	resultDataStr = resultDataField.String()

	// If the raw string is not valid JSON, try to extract from markdown fences
	if !json.Valid([]byte(resultDataStr)) {
		if extracted, extractErr := extractJSONFromText(resultDataStr); extractErr == nil && extracted != "" {
			resultDataStr = extracted
		}
	}

	if path == "" {
		var resultObj map[string]interface{}
		err = json.Unmarshal([]byte(resultDataStr), &resultObj)
		if err == nil {
			return resultObj, nil
		}

		var resultArray []interface{}
		err = json.Unmarshal([]byte(resultDataStr), &resultArray)
		if err != nil {
			return nil, fmt.Errorf("failed to parse result data as JSON: %w", err)
		}
		return resultArray, nil
	}

	if strings.HasPrefix(path, "array[") {
		var resultArray []interface{}
		err = json.Unmarshal([]byte(resultDataStr), &resultArray)
		if err != nil {
			return nil, fmt.Errorf("failed to parse result data as JSON array: %w", err)
		}

		arrayIndexRegex := regexp.MustCompile(`array\[(\d+)\](.*)`)
		matches := arrayIndexRegex.FindStringSubmatch(path)
		if len(matches) < 3 {
			return nil, fmt.Errorf("invalid array path format: %s", path)
		}

		arrayIndex, err := strconv.Atoi(matches[1])
		if err != nil {
			return nil, fmt.Errorf("invalid array index: %s", matches[1])
		}

		if arrayIndex < 0 || arrayIndex >= len(resultArray) {
			return nil, fmt.Errorf("array index %d out of bounds", arrayIndex)
		}

		remainingPath := matches[2]
		if remainingPath == "" {
			return resultArray[arrayIndex], nil
		}

		if strings.HasPrefix(remainingPath, ".") {
			remainingPath = remainingPath[1:]
		}

		return r.NavigatePath(resultArray[arrayIndex], remainingPath)
	}

	var data map[string]interface{}
	err = json.Unmarshal([]byte(resultDataStr), &data)
	if err != nil {
		var arrayData []interface{}
		err = json.Unmarshal([]byte(resultDataStr), &arrayData)
		if err != nil {
			return nil, fmt.Errorf("failed to parse result data as JSON: %w", err)
		}

		wrapper := map[string]interface{}{
			"array": arrayData,
		}
		return r.NavigatePath(wrapper, "array."+path)
	}

	return r.NavigatePath(data, path)
}

// ---------------------------------------------------------------------------
// NavigatePath — traverse nested data structures
// ---------------------------------------------------------------------------

// NavigatePath navigates a data structure using a dot/bracket path.
func (r *VariableReplacer) NavigatePath(data interface{}, path string) (interface{}, error) {
	if data == nil {
		return nil, nil
	}

	segments := r.parsePathSegments(path)
	current := data

	for _, segment := range segments {
		if current == nil {
			return nil, nil
		}

		indexStart := strings.Index(segment, "[")

		if indexStart > 0 {
			fieldName := segment[:indexStart]
			indexEnd := strings.Index(segment, "]")

			if indexEnd == -1 || indexStart >= len(segment)-1 {
				return nil, fmt.Errorf("invalid bracket syntax in path segment: %s", segment)
			}

			bracketContent := segment[indexStart+1 : indexEnd]

			obj, ok := current.(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("cannot access field %s in non-object value", fieldName)
			}

			fieldValue, ok := obj[fieldName]
			if !ok {
				return nil, fmt.Errorf("field %s not found", fieldName)
			}

			if strings.HasPrefix(bracketContent, "\"") && strings.HasSuffix(bracketContent, "\"") {
				key := bracketContent[1 : len(bracketContent)-1]

				nestedObj, ok := fieldValue.(map[string]interface{})
				if !ok {
					return nil, fmt.Errorf("field %s is not an object (cannot use string key access)", fieldName)
				}

				value, ok := nestedObj[key]
				if !ok {
					return nil, fmt.Errorf("key '%s' not found in field %s", key, fieldName)
				}

				current = value
			} else {
				index, err := strconv.Atoi(bracketContent)
				if err != nil {
					return nil, fmt.Errorf("invalid bracket content '%s' in segment %s: not a number or quoted string", bracketContent, segment)
				}

				switch arr := fieldValue.(type) {
				case []interface{}:
					if index < 0 || index >= len(arr) {
						return nil, fmt.Errorf("array index %d out of bounds for field %s", index, fieldName)
					}
					current = arr[index]
				case []map[string]interface{}:
					if index < 0 || index >= len(arr) {
						return nil, fmt.Errorf("array index %d out of bounds for field %s", index, fieldName)
					}
					current = arr[index]
				default:
					rv := reflect.ValueOf(fieldValue)
					if rv.Kind() == reflect.Slice {
						if index < 0 || index >= rv.Len() {
							return nil, fmt.Errorf("array index %d out of bounds for field %s", index, fieldName)
						}
						current = rv.Index(index).Interface()
					} else {
						return nil, fmt.Errorf("field %s is not an array (type: %T)", fieldName, fieldValue)
					}
				}
			}
		} else if indexStart == 0 {
			indexEnd := strings.Index(segment, "]")
			if indexEnd == -1 {
				return nil, fmt.Errorf("invalid bracket syntax in path segment: %s", segment)
			}

			bracketContent := segment[1:indexEnd]

			if strings.HasPrefix(bracketContent, "\"") && strings.HasSuffix(bracketContent, "\"") {
				key := bracketContent[1 : len(bracketContent)-1]

				obj, ok := current.(map[string]interface{})
				if !ok {
					return nil, fmt.Errorf("cannot use string key access on non-object value")
				}

				value, ok := obj[key]
				if !ok {
					return nil, fmt.Errorf("key '%s' not found", key)
				}

				current = value
			} else {
				index, err := strconv.Atoi(bracketContent)
				if err != nil {
					return nil, fmt.Errorf("invalid bracket content '%s' in segment %s: not a number or quoted string", bracketContent, segment)
				}

				switch arr := current.(type) {
				case []interface{}:
					if index < 0 || index >= len(arr) {
						return nil, fmt.Errorf("array index %d out of bounds", index)
					}
					current = arr[index]
				case []map[string]interface{}:
					if index < 0 || index >= len(arr) {
						return nil, fmt.Errorf("array index %d out of bounds", index)
					}
					current = arr[index]
				default:
					rv := reflect.ValueOf(current)
					if rv.Kind() == reflect.Slice {
						if index < 0 || index >= rv.Len() {
							return nil, fmt.Errorf("array index %d out of bounds", index)
						}
						current = rv.Index(index).Interface()
					} else {
						return nil, fmt.Errorf("cannot index into non-array value (type: %T)", current)
					}
				}
			}
		} else {
			// Regular field access

			// Handle FileResult type (pointer or value)
			if fileResult, ok := current.(*skill.FileResult); ok {
				value, err := r.getFileResultField(fileResult, segment)
				if err != nil {
					return nil, err
				}
				current = value
				continue
			}

			// Also handle FileResult as map (from JSON unmarshaling)
			if fileMap, ok := current.(map[string]interface{}); ok {
				if _, hasFileName := fileMap["fileName"]; hasFileName {
					if _, hasMimeType := fileMap["mimeType"]; hasMimeType {
						value, err := r.getFileResultFieldFromMap(fileMap, segment)
						if err == nil {
							current = value
							continue
						}
					}
				}
			}

			obj, ok := current.(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("cannot access field %s in non-object value", segment)
			}

			value, ok := obj[segment]
			if !ok {
				return nil, fmt.Errorf("field %s not found", segment)
			}

			current = value
		}
	}

	return current, nil
}

// getFileResultField extracts a field from a FileResult struct.
func (r *VariableReplacer) getFileResultField(fr *skill.FileResult, field string) (interface{}, error) {
	switch field {
	case "url":
		return fr.URL, nil
	case "fileName":
		return fr.FileName, nil
	case "size":
		return fr.Size, nil
	case "mimeType":
		return fr.MimeType, nil
	case "tempPath":
		return fr.TempPath, nil
	case "base64":
		if fr.TempPath != "" && r.tempFileManager != nil {
			return r.tempFileManager.GetTempFileBase64(fr.TempPath)
		}
		return "", fmt.Errorf("file no longer available (expired or temp manager not available)")
	case "fileUpload":
		if fr.TempPath != "" {
			return fmt.Sprintf("__FILE_UPLOAD__:%s", fr.TempPath), nil
		}
		return "", fmt.Errorf("file no longer available (no tempPath)")
	default:
		return nil, fmt.Errorf("field '%s' not found in FileResult", field)
	}
}

// getFileResultFieldFromMap extracts a field from a FileResult represented as a map.
func (r *VariableReplacer) getFileResultFieldFromMap(fileMap map[string]interface{}, field string) (interface{}, error) {
	switch field {
	case "url", "fileName", "mimeType", "tempPath":
		if val, ok := fileMap[field]; ok {
			return val, nil
		}
		return "", nil
	case "size":
		if val, ok := fileMap["size"]; ok {
			return val, nil
		}
		return int64(0), nil
	case "base64":
		if tempPath, ok := fileMap["tempPath"].(string); ok && tempPath != "" && r.tempFileManager != nil {
			return r.tempFileManager.GetTempFileBase64(tempPath)
		}
		return "", fmt.Errorf("file no longer available (expired or temp manager not available)")
	case "fileUpload":
		if tempPath, ok := fileMap["tempPath"].(string); ok && tempPath != "" {
			return fmt.Sprintf("__FILE_UPLOAD__:%s", tempPath), nil
		}
		return "", fmt.Errorf("file no longer available (no tempPath)")
	default:
		return nil, fmt.Errorf("field '%s' not found in FileResult", field)
	}
}

// parsePathSegments parses a path into segments, handling array indices.
func (r *VariableReplacer) parsePathSegments(path string) []string {
	result := []string{}
	current := ""
	inBracket := false

	for _, char := range path {
		switch char {
		case '.':
			if !inBracket {
				if current != "" {
					result = append(result, current)
					current = ""
				}
			} else {
				current += string(char)
			}
		case '[':
			inBracket = true
			current += string(char)
		case ']':
			inBracket = false
			current += string(char)
		default:
			current += string(char)
		}
	}

	if current != "" {
		result = append(result, current)
	}

	return result
}

// ---------------------------------------------------------------------------
// DB parameterized queries
// ---------------------------------------------------------------------------

// ReplaceVariablesForDB replaces variables with placeholders for parameterized queries.
func (r *VariableReplacer) ReplaceVariablesForDB(ctx context.Context, text string, inputs map[string]interface{}) (string, []interface{}, error) {
	var params []interface{}

	text, err := r.replaceEnvVariables(text)
	if err != nil {
		return "", nil, fmt.Errorf("error replacing environment variables: %w", err)
	}

	text, err = r.replaceSystemVariables(ctx, text)
	if err != nil {
		return "", nil, fmt.Errorf("error replacing system variables: %w", err)
	}

	text, params = r.replaceInputVariablesWithPlaceholders(text, inputs)

	text, resultParams, err := r.replaceFunctionResultsWithPlaceholders(ctx, text)
	if err != nil {
		return "", nil, fmt.Errorf("error replacing function results: %w", err)
	}

	params = append(params, resultParams...)

	return text, params, nil
}

func (r *VariableReplacer) replaceInputVariablesWithPlaceholders(text string, inputs map[string]interface{}) (string, []interface{}) {
	if inputs == nil {
		return text, nil
	}

	var params []interface{}

	resolvedValues := make(map[string]interface{})
	processedVarNames := make(map[string]bool)

	complexVarRegex := regexp.MustCompile(`\$([a-zA-Z0-9_]+(?:\[[^\]]+\]|\.)[\[\]a-zA-Z0-9_\.]*)`)
	complexMatches := complexVarRegex.FindAllStringSubmatch(text, -1)

	for _, match := range complexMatches {
		if len(match) < 2 {
			continue
		}

		fullMatch := "$" + match[1]
		if processedVarNames[fullMatch] {
			continue
		}

		refPath := match[1]
		firstDotOrBracket := strings.IndexAny(refPath, ".[")
		if firstDotOrBracket == -1 {
			continue
		}

		varName := refPath[:firstDotOrBracket]
		fieldPath := refPath[firstDotOrBracket:]
		if strings.HasPrefix(fieldPath, ".") {
			fieldPath = fieldPath[1:]
		}

		baseValue, _ := inputs[varName]
		var value interface{}
		if baseValue == nil {
			value = nil
		} else {
			fieldValue, err := r.NavigatePath(baseValue, fieldPath)
			if err != nil {
				continue
			}
			value = fieldValue
		}

		resolvedValues[fullMatch] = value
		processedVarNames[fullMatch] = true
	}

	simpleVarRegex := regexp.MustCompile(`\$([a-zA-Z0-9_]+)`)
	simpleMatches := simpleVarRegex.FindAllStringSubmatchIndex(text, -1)

	for _, match := range simpleMatches {
		if len(match) < 4 {
			continue
		}

		fullMatchStart := match[0]
		fullMatchEnd := match[1]
		varNameStart := match[2]
		varNameEnd := match[3]

		fullMatch := text[fullMatchStart:fullMatchEnd]
		varName := text[varNameStart:varNameEnd]

		if fullMatchEnd < len(text) {
			nextChar := text[fullMatchEnd]
			if nextChar == '.' || nextChar == '[' {
				continue
			}
		}

		if processedVarNames[fullMatch] {
			continue
		}

		value, _ := inputs[varName]
		if strVal, ok := value.(string); ok && strVal == "NULL" {
			value = nil
		}

		resolvedValues[fullMatch] = value
		processedVarNames[fullMatch] = true
	}

	// Replace variables ONE AT A TIME in order of appearance
	allVarRegex := regexp.MustCompile(`\$([a-zA-Z0-9_]+(?:\[[^\]]+\]|\.[\[\]a-zA-Z0-9_\.]*)?|\$[a-zA-Z0-9_]+)`)

	for {
		match := allVarRegex.FindStringSubmatchIndex(text)
		if match == nil {
			break
		}

		fullMatchStart := match[0]
		fullMatchEnd := match[1]
		fullMatch := text[fullMatchStart:fullMatchEnd]

		if fullMatchEnd < len(text) {
			nextChar := text[fullMatchEnd]
			if nextChar == '.' || nextChar == '[' {
				complexMatch := complexVarRegex.FindStringIndex(text[fullMatchStart:])
				if complexMatch != nil {
					fullMatchEnd = fullMatchStart + complexMatch[1]
					fullMatch = text[fullMatchStart:fullMatchEnd]
				}
			}
		}

		value, exists := resolvedValues[fullMatch]
		if !exists {
			break
		}

		text = text[:fullMatchStart] + "?" + text[fullMatchEnd:]

		switch v := value.(type) {
		case []interface{}, map[string]interface{}, map[interface{}]interface{}:
			jsonBytes, err := json.Marshal(v)
			if err != nil {
				params = append(params, value)
			} else {
				params = append(params, string(jsonBytes))
			}
		default:
			params = append(params, value)
		}
	}

	return text, params
}

func (r *VariableReplacer) replaceFunctionResultsWithPlaceholders(ctx context.Context, text string) (string, []interface{}, error) {
	var params []interface{}

	resultRegex := regexp.MustCompile(`result\[([^]]+)\](?:\[[^]]+\]|\.[a-zA-Z0-9_]+(?:\[[^]]+\])?)*`)
	complexMatches := resultRegex.FindAllString(text, -1)

	if len(complexMatches) == 0 {
		return text, params, nil
	}

	messageID, err := r.getMessageIDFromContext(ctx)
	if err != nil {
		return "", nil, fmt.Errorf("error getting message ID for result replacement: %w", err)
	}

	for _, fullMatch := range complexMatches {
		indexRegex := regexp.MustCompile(`result\[([^]]+)\]`)
		indexMatch := indexRegex.FindStringSubmatch(fullMatch)
		if len(indexMatch) < 2 {
			continue
		}

		indexStr := indexMatch[1]
		index, err := strconv.Atoi(indexStr)
		if err != nil {
			return "", nil, fmt.Errorf("invalid result index in '%s': %v", fullMatch, err)
		}

		path := strings.TrimPrefix(fullMatch, fmt.Sprintf("result[%d]", index))

		if strings.HasPrefix(path, "[") {
			path = "array" + path
		} else if strings.HasPrefix(path, ".") {
			path = path[1:]
		}

		resultValue, err := r.getResultValueByPath(ctx, messageID, index, path)
		if err != nil {
			return "", nil, fmt.Errorf("error retrieving %s: %w", fullMatch, err)
		}

		count := strings.Count(text, fullMatch)
		text = strings.ReplaceAll(text, fullMatch, "?")

		for i := 0; i < count; i++ {
			params = append(params, resultValue)
		}
	}

	return text, params, nil
}

// ---------------------------------------------------------------------------
// Context helpers
// ---------------------------------------------------------------------------

func (r *VariableReplacer) getMessageIDFromContext(ctx context.Context) (string, error) {
	if msgVal := ctx.Value(MessageInContextKey); msgVal != nil {
		if msg, ok := msgVal.(Message); ok {
			return msg.Id, nil
		}
	}

	if msgIDVal := ctx.Value("message_id"); msgIDVal != nil {
		if msgID, ok := msgIDVal.(string); ok {
			return msgID, nil
		}
	}

	return "", fmt.Errorf("message ID not found in context")
}

func (r *VariableReplacer) getResultValue(ctx context.Context, messageID string, index int, fieldName string) (interface{}, error) {
	if r.executionTracker == nil {
		return nil, fmt.Errorf("execution tracker is not initialized")
	}

	return r.executionTracker.GetStepResultField(ctx, messageID, index, fieldName)
}

// ---------------------------------------------------------------------------
// System variable resolvers
// ---------------------------------------------------------------------------

func (r *VariableReplacer) getMeVariable(field string) (string, error) {
	me := map[string]string{
		"name":        r.meName,
		"version":     r.meVersion,
		"description": r.meDescription,
	}

	if field == "" {
		return r.meName, nil
	}

	if value, ok := me[field]; ok {
		return value, nil
	}

	return "", fmt.Errorf("invalid field '%s' for $ME variable", field)
}

func (r *VariableReplacer) getMessageVariable(ctx context.Context, field string) (string, error) {
	retrievedMsg, ok := ctx.Value(MessageInContextKey).(Message)
	if !ok {
		return "", fmt.Errorf("error getting message from context")
	}

	messageFields := map[string]func(Message) string{
		"id":        func(m Message) string { return m.Id },
		"text":      func(m Message) string { return m.Body },
		"from":      func(m Message) string { return m.From },
		"channel":   func(m Message) string { return m.Channel },
		"timestamp": func(m Message) string { return fmt.Sprintf("%d", m.Timestamp) },
		"hasMedia":  func(m Message) string { return fmt.Sprintf("%t", m.HasMedia) },
		"client_id": func(m Message) string { return m.ClientID },
	}

	if field == "" {
		return retrievedMsg.Body, nil
	}

	if fieldFunc, ok := messageFields[field]; ok {
		return fieldFunc(retrievedMsg), nil
	}

	return "", fmt.Errorf("invalid field '%s' for $MESSAGE variable", field)
}

func (r *VariableReplacer) getNowVariable(field string) (string, error) {
	now := time.Now().UTC()

	switch field {
	case "":
		return now.Format(time.RFC3339), nil
	case "date":
		return now.Format("2006-01-02"), nil
	case "time":
		return now.Format("15:04:05"), nil
	case "hour":
		return fmt.Sprintf("%d", now.Hour()), nil
	case "unix":
		return fmt.Sprintf("%d", now.Unix()), nil
	case "iso8601":
		return now.Format(time.RFC3339), nil
	case "weekday":
		return now.Weekday().String(), nil
	default:
		return "", fmt.Errorf("invalid field '%s' for $NOW variable", field)
	}
}

func (r *VariableReplacer) getUserVariable(ctx context.Context, field string) (string, error) {
	var err error

	// Try to get user from context first
	userInContext, ok := ctx.Value(UserInContextKey).(User)
	if !ok {
		if logger != nil {
			logger.Warnf("User not found in context when processing $USER.%s variable", field)
		}
		// Fall back to looking up user by message client ID
		retrievedMsg, msgOk := ctx.Value(MessageInContextKey).(Message)
		if !msgOk {
			return "", fmt.Errorf("error getting message from context. variable $USER.%s requires a user in context", field)
		}
		if r.userRepo == nil {
			return "", fmt.Errorf("user repository not available for $USER.%s variable", field)
		}
		userPtr, lookupErr := r.userRepo.GetUser(ctx, retrievedMsg.ClientID)
		if lookupErr != nil {
			if logger != nil {
				logger.Errorf("Failed to get user from repository: %v", lookupErr)
			}
			return "", fmt.Errorf("error getting user from repository for $USER.%s variable", field)
		}
		userInContext = *userPtr
		err = nil // clear
	}
	_ = err // suppress unused warning

	// Handle messages_count specially
	if field == "messages_count" {
		if r.messageCounter != nil {
			count, countErr := r.messageCounter.CountUserMessages(ctx, userInContext.ID)
			if countErr != nil {
				if logger != nil {
					logger.Warnf("Failed to count user messages, falling back to cached value: %v", countErr)
				}
				return fmt.Sprintf("%d", userInContext.QuantityOfMessagesReceived), nil
			}
			return fmt.Sprintf("%d", count), nil
		}
		return fmt.Sprintf("%d", userInContext.QuantityOfMessagesReceived), nil
	}

	userFields := map[string]func(User) string{
		"id":           func(u User) string { return u.ID },
		"first_name":   func(u User) string { return u.FirstName },
		"last_name":    func(u User) string { return u.LastName },
		"email":        func(u User) string { return u.Email },
		"phone":        func(u User) string { return u.Telephone },
		"gender":       func(u User) string { return u.Gender },
		"address":      func(u User) string { return u.Address },
		"company_id":   func(u User) string { return u.CompanyID },
		"company_name": func(u User) string { return u.CompanyName },
		"language":     func(u User) string { return u.Language },
		"birth_date": func(u User) string {
			if u.BirthdayDate.IsZero() {
				return ""
			}
			return u.BirthdayDate.Format("02/01/2006")
		},
	}

	if field == "" {
		return fmt.Sprintf("%s %s", userInContext.FirstName, userInContext.LastName), nil
	}

	if fieldFunc, ok := userFields[field]; ok {
		return fieldFunc(userInContext), nil
	}

	return "", fmt.Errorf("invalid field '%s' for $USER variable", field)
}

func (r *VariableReplacer) getAdminVariable(ctx context.Context, field string) (string, error) {
	// Get admin from context (replaces the ConnectAI singleton pattern)
	admin, ok := ctx.Value(AdminInContextKey).(*User)
	if !ok || admin == nil {
		return "", fmt.Errorf("admin user not found in context for $ADMIN variable")
	}

	adminFields := map[string]func(*User) string{
		"id":           func(u *User) string { return u.ID },
		"first_name":   func(u *User) string { return u.FirstName },
		"last_name":    func(u *User) string { return u.LastName },
		"email":        func(u *User) string { return u.Email },
		"phone":        func(u *User) string { return u.Telephone },
		"gender":       func(u *User) string { return u.Gender },
		"address":      func(u *User) string { return u.Address },
		"company_id":   func(u *User) string { return u.CompanyID },
		"company_name": func(u *User) string { return u.CompanyName },
		"language":     func(u *User) string { return u.Language },
		"birth_date": func(u *User) string {
			if u.BirthdayDate.IsZero() {
				return ""
			}
			return u.BirthdayDate.Format("02/01/2006")
		},
	}

	if field == "" {
		return fmt.Sprintf("%s %s", admin.FirstName, admin.LastName), nil
	}

	if fieldFunc, ok := adminFields[field]; ok {
		return fieldFunc(admin), nil
	}

	return "", fmt.Errorf("invalid field '%s' for $ADMIN variable", field)
}

func (r *VariableReplacer) getCompanyVariable(ctx context.Context, field string) (string, error) {
	// Get company from context (replaces the ConnectAI singleton pattern)
	company, ok := ctx.Value(CompanyInContextKey).(*CompanyInfo)
	if !ok || company == nil {
		return "", fmt.Errorf("company info not found in context for $COMPANY variable")
	}

	companyFields := map[string]func(*CompanyInfo) string{
		"id":                func(c *CompanyInfo) string { return c.ID },
		"name":              func(c *CompanyInfo) string { return c.Name },
		"fantasy_name":      func(c *CompanyInfo) string { return c.FantasyName },
		"tax_code":          func(c *CompanyInfo) string { return c.TaxCode },
		"industry":          func(c *CompanyInfo) string { return c.Industry },
		"email":             func(c *CompanyInfo) string { return c.Email },
		"instagram_profile": func(c *CompanyInfo) string { return c.InstagramProfile },
		"website":           func(c *CompanyInfo) string { return c.Website },
		"ai_session_id":     func(c *CompanyInfo) string { return c.AISessionID },
	}

	if field == "" {
		return company.Name, nil
	}

	if fieldFunc, ok := companyFields[field]; ok {
		return fieldFunc(company), nil
	}

	return "", fmt.Errorf("invalid field '%s' for $COMPANY variable", field)
}

func (r *VariableReplacer) getUuidVariable(field string) (string, error) {
	newUUID := uuid.New()

	if field == "" {
		return newUUID.String(), nil
	}

	return "", fmt.Errorf("$UUID variable does not support fields (got: '%s')", field)
}

func (r *VariableReplacer) getMeetingVariable(ctx context.Context, field string) (string, error) {
	meetingCtx, ok := ctx.Value(MeetingContextKey).(MeetingContext)
	if !ok {
		return "", nil
	}

	meetingFields := map[string]func(MeetingContext) string{
		"bot_id":    func(m MeetingContext) string { return m.BotID },
		"event":     func(m MeetingContext) string { return m.Event },
		"timestamp": func(m MeetingContext) string { return m.Timestamp.Format(time.RFC3339) },
	}

	if field == "" {
		return meetingCtx.BotID, nil
	}

	if fieldFunc, ok := meetingFields[field]; ok {
		return fieldFunc(meetingCtx), nil
	}

	return "", fmt.Errorf("invalid field '%s' for $MEETING variable (valid fields: bot_id, event, timestamp)", field)
}

func (r *VariableReplacer) getEmailVariable(ctx context.Context, field string) (string, error) {
	emailCtx, ok := ctx.Value(EmailContextKey).(EmailContext)
	if !ok {
		return "", nil
	}

	emailFields := map[string]func(EmailContext) string{
		"thread_id":       func(e EmailContext) string { return e.ThreadID },
		"message_id":      func(e EmailContext) string { return e.MessageID },
		"subject":         func(e EmailContext) string { return e.Subject },
		"sender":          func(e EmailContext) string { return e.Sender },
		"recipients":      func(e EmailContext) string { return strings.Join(e.Recipients, ",") },
		"cc":              func(e EmailContext) string { return strings.Join(e.CC, ",") },
		"bcc":             func(e EmailContext) string { return strings.Join(e.BCC, ",") },
		"in_reply_to":     func(e EmailContext) string { return e.InReplyTo },
		"references":      func(e EmailContext) string { return strings.Join(e.References, ",") },
		"date":            func(e EmailContext) string { return e.Date },
		"text_body":       func(e EmailContext) string { return e.TextBody },
		"has_attachments": func(e EmailContext) string { return fmt.Sprintf("%t", e.HasAttachments) },
	}

	if field == "" {
		return emailCtx.Subject, nil
	}

	if fieldFunc, ok := emailFields[field]; ok {
		return fieldFunc(emailCtx), nil
	}

	return "", fmt.Errorf("invalid field '%s' for $EMAIL variable (valid fields: thread_id, message_id, subject, sender, recipients, cc, bcc, in_reply_to, references, date, text_body, has_attachments)", field)
}

func (r *VariableReplacer) getTempDirVariable(field string) (string, error) {
	if field != "" {
		return "", fmt.Errorf("$TEMP_DIR variable does not support fields (got: '%s')", field)
	}

	tempDir := os.Getenv("TEMP_DIR")
	if tempDir == "" {
		tempDir = filepath.Join(os.TempDir(), "mantis-skill", "workspaces")
	}

	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create temp dir %s: %w", tempDir, err)
	}
	return tempDir, nil
}

func (r *VariableReplacer) getFileVariable(ctx context.Context, field string) (string, error) {
	retrievedMsg, ok := ctx.Value(MessageInContextKey).(Message)
	if !ok {
		return "", nil
	}

	if !retrievedMsg.HasMedia || retrievedMsg.Media == nil {
		return "", nil
	}

	fileFields := map[string]func(*MediaType) string{
		"url": func(m *MediaType) string {
			return m.Url
		},
		"path": func(m *MediaType) string {
			// Return URL as fallback (FilepathDir is ConnectAI-specific)
			return m.Url
		},
		"mimetype": func(m *MediaType) string {
			return m.Mimetype
		},
		"filename": func(m *MediaType) string {
			return m.Filename
		},
	}

	if field == "" {
		return retrievedMsg.Media.Url, nil
	}

	if fieldFunc, ok := fileFields[field]; ok {
		return fieldFunc(retrievedMsg.Media), nil
	}

	return "", fmt.Errorf("invalid field '%s' for $FILE variable (valid fields: url, path, mimetype, filename)", field)
}

// ---------------------------------------------------------------------------
// Date transformation functions
// ---------------------------------------------------------------------------

func toISO(dateStr string) string {
	if dateStr == "" {
		return ""
	}

	if strings.Contains(dateStr, ",") {
		dates := strings.Split(dateStr, ",")
		convertedDates := make([]string, 0, len(dates))
		for _, d := range dates {
			d = strings.TrimSpace(d)
			if converted := convertSingleDateToISO(d); converted != "" {
				convertedDates = append(convertedDates, converted)
			}
		}
		return strings.Join(convertedDates, ",")
	}

	return convertSingleDateToISO(dateStr)
}

func convertSingleDateToISO(dateStr string) string {
	dateStr = strings.TrimSpace(dateStr)
	parts := strings.Split(dateStr, "/")
	if len(parts) != 3 {
		return dateStr
	}

	day := parts[0]
	month := parts[1]
	year := parts[2]

	if len(day) == 1 {
		day = "0" + day
	}
	if len(month) == 1 {
		month = "0" + month
	}

	return fmt.Sprintf("%s-%s-%s", year, month, day)
}

func toDDMMYYYY(dateStr string) string {
	if dateStr == "" {
		return ""
	}

	if strings.Contains(dateStr, ",") {
		dates := strings.Split(dateStr, ",")
		convertedDates := make([]string, 0, len(dates))
		for _, d := range dates {
			d = strings.TrimSpace(d)
			if converted := convertSingleDateToDDMMYYYY(d); converted != "" {
				convertedDates = append(convertedDates, converted)
			}
		}
		return strings.Join(convertedDates, ",")
	}

	return convertSingleDateToDDMMYYYY(dateStr)
}

func convertSingleDateToDDMMYYYY(dateStr string) string {
	dateStr = strings.TrimSpace(dateStr)
	parts := strings.Split(dateStr, "-")
	if len(parts) != 3 {
		return dateStr
	}

	year := parts[0]
	month := parts[1]
	day := parts[2]

	if len(year) != 4 || !isNumeric(year) {
		return dateStr
	}
	if len(month) < 1 || len(month) > 2 || !isNumeric(month) {
		return dateStr
	}
	if len(day) < 1 || len(day) > 2 || !isNumeric(day) {
		return dateStr
	}

	return fmt.Sprintf("%s/%s/%s", day, month, year)
}

func isNumeric(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return len(s) > 0
}

func applyDateTransformation(value string, transformFunc string) (string, bool) {
	switch transformFunc {
	case "toISO":
		return toISO(value), true
	case "toDDMMYYYY":
		return toDDMMYYYY(value), true
	default:
		return value, false
	}
}

// ---------------------------------------------------------------------------
// Escaped dollar signs
// ---------------------------------------------------------------------------

const escapedDollarSentinel = "\x00ESCAPED_DOLLAR\x00"

var escapedDollarRegex = regexp.MustCompile(`\{\$\$([^}]+)\}`)

func (r *VariableReplacer) protectEscapedDollars(text string) string {
	if !strings.Contains(text, "{$$") {
		return text
	}
	return escapedDollarRegex.ReplaceAllString(text, escapedDollarSentinel+"$1")
}

func (r *VariableReplacer) restoreEscapedDollars(text string) string {
	if !strings.Contains(text, escapedDollarSentinel) {
		return text
	}
	return strings.ReplaceAll(text, escapedDollarSentinel, "$")
}

// ---------------------------------------------------------------------------
// Date transformation extraction
// ---------------------------------------------------------------------------

func extractDateTransformation(path string) (string, string, bool) {
	transformFuncs := []string{"toISO()", "toDDMMYYYY()"}

	for _, tf := range transformFuncs {
		if strings.HasSuffix(path, "."+tf) {
			funcName := strings.TrimSuffix(tf, "()")
			cleanPath := strings.TrimSuffix(path, "."+tf)
			return cleanPath, funcName, true
		}
		if path == tf {
			funcName := strings.TrimSuffix(tf, "()")
			return "", funcName, true
		}
	}

	return path, "", false
}

// NavigatePathWithTransformation navigates to a value and applies any date transformation.
func (r *VariableReplacer) NavigatePathWithTransformation(inputs map[string]interface{}, path string) (interface{}, error) {
	cleanPath, transformFunc, hasTransform := extractDateTransformation(path)

	rawValue, err := r.NavigatePath(inputs, cleanPath)
	if err != nil {
		return nil, err
	}

	if !hasTransform {
		return rawValue, nil
	}

	strValue, ok := rawValue.(string)
	if !ok {
		return rawValue, nil
	}

	switch transformFunc {
	case "toISO":
		return toISO(strValue), nil
	case "toDDMMYYYY":
		return toDDMMYYYY(strValue), nil
	default:
		return rawValue, nil
	}
}

// ---------------------------------------------------------------------------
// Utility functions (inlined from utils package)
// ---------------------------------------------------------------------------

// tryParseJSON attempts to parse text as JSON. If direct parsing fails,
// it falls back to extractJSONFromText to handle markdown-wrapped JSON.
func tryParseJSON(text string) (interface{}, bool) {
	var parsed interface{}
	if err := json.Unmarshal([]byte(text), &parsed); err == nil {
		return parsed, true
	}
	if extracted, err := extractJSONFromText(text); err == nil && extracted != "" {
		if err := json.Unmarshal([]byte(extracted), &parsed); err == nil {
			return parsed, true
		}
	}
	return nil, false
}

// extractJSONFromText and extractCompleteJSONBlock are defined in output_formatter.go
// and shared across the engine package. No need to redeclare them here.

// formatValueAsString converts a value to string, handling float64 specially
// to avoid scientific notation for integer-like values.
func formatValueAsString(v interface{}) string {
	switch val := v.(type) {
	case float64:
		if val == float64(int64(val)) {
			return strconv.FormatInt(int64(val), 10)
		}
		return strconv.FormatFloat(val, 'f', -1, 64)
	case float32:
		if val == float32(int32(val)) {
			return strconv.FormatInt(int64(val), 10)
		}
		return strconv.FormatFloat(float64(val), 'f', -1, 32)
	case int, int8, int16, int32, int64:
		return fmt.Sprintf("%d", val)
	case uint, uint8, uint16, uint32, uint64:
		return fmt.Sprintf("%d", val)
	case json.Number:
		return val.String()
	case string:
		return val
	case bool:
		return strconv.FormatBool(val)
	case nil:
		return "null"
	default:
		if jsonBytes, err := json.Marshal(val); err == nil {
			return string(jsonBytes)
		}
		return fmt.Sprintf("%v", val)
	}
}
