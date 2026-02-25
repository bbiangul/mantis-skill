package skill

const (
	// Func Operations
	OperationWeb              = "web_browse"
	OperationAPI              = "api_call"
	OperationDesktop          = "desktop_use"
	OperationMCP              = "mcp"
	OperationFormat           = "format"
	OperationDB               = "db"
	OperationTerminal         = "terminal"
	OperationInitiateWorkflow = "initiate_workflow"
	OperationPolicy           = "policy"
	OperationPDF              = "pdf"
	OperationCode             = "code"
	OperationGDrive           = "gdrive"

	// MCP protocol types as string constants
	MCPProtocolTypeStdio = "stdio"
	MCPProtocolTypeSSE   = "sse"

	// PDF Page Sizes
	PDFPageSizeA4     = "A4"
	PDFPageSizeLetter = "Letter"
	PDFPageSizeLegal  = "Legal"
	PDFPageSizeA3     = "A3"
	PDFPageSizeA5     = "A5"

	// PDF Orientations
	PDFOrientationPortrait  = "portrait"
	PDFOrientationLandscape = "landscape"

	// PDF Alignments
	PDFAlignLeft   = "left"
	PDFAlignCenter = "center"
	PDFAlignRight  = "right"

	// Func Triggers
	TriggerTime                   = "time_based"
	TriggerOnUserMessage          = "always_on_user_message"           // Executes at workflow START (blocking)
	TriggerOnTeamMessage          = "always_on_team_message"           // Executes at workflow START (blocking)
	TriggerOnCompletedUserMessage = "always_on_completed_user_message" // Executes at workflow COMPLETION
	TriggerOnCompletedTeamMessage = "always_on_completed_team_message" // Executes at workflow COMPLETION
	TriggerFlexForUser            = "flex_for_user"
	TriggerFlexForTeam            = "flex_for_team"
	TriggerOnMeetingStart         = "on_meeting_start"
	TriggerOnMeetingEnd           = "on_meeting_end"

	// Email Triggers (analogous to message triggers, fire only when channel is "email")
	TriggerOnUserEmail          = "always_on_user_email"           // Executes at workflow START (blocking) for emails
	TriggerOnTeamEmail          = "always_on_team_email"           // Executes at workflow START (blocking) for emails
	TriggerOnCompletedUserEmail = "always_on_completed_user_email" // Executes at workflow COMPLETION for emails
	TriggerOnCompletedTeamEmail = "always_on_completed_team_email" // Executes at workflow COMPLETION for emails
	TriggerFlexForUserEmail     = "flex_for_user_email"            // Visible to AI only for email channel
	TriggerFlexForTeamEmail     = "flex_for_team_email"            // Visible to AI only for email channel

	// Data Origin for Input Fulfillment
	DataOriginChat      = "chat"
	DataOriginFunction  = "function"
	DataOriginKnowledge = "knowledge"
	DataOriginSearch    = "search"
	DataOriginInference = "inference"
	DataOriginMemory    = "memory"

	// OnError Strategies
	OnErrorRequestUserInput          = "requestUserInput"
	OnErrorRequestN1Support          = "requestN1Support"
	OnErrorRequestN2Support          = "requestN2Support"
	OnErrorRequestN3Support          = "requestN3Support"
	OnErrorRequestApplicationSupport = "requestApplicationSupport"
	OnErrorSearch                    = "search"
	OnErrorInference                 = "inference"

	With    = "with"
	Headers = "headers"
	Key     = "key"
	Value   = "value"

	// Steps
	StepActionURL         = "open_url"
	StepWithURL           = "url"
	StepWithIncognitoMode = "incognitoMode"

	StepActionApp = "open_app"
	StepWithApp   = "app_name"

	StepActionExtractText = "extract_text"

	StepActionFill    = "fill"
	StepActionSubmit  = "submit"
	StepWithFillValue = "fillValue"

	StepActionFindClick           = "find_and_click"
	StepWithFindBy                = "findBy"
	StepWithFindById              = "id"
	StepWithFindByLabel           = "label"
	StepWithFindByType            = "type"
	StepWithFindByVisual          = "visual_prominence"
	StepWithFindBySemanticContext = "semantic_context"

	StepWithFindByTypeTextArea  = "textarea"
	StepWithFindByTypeButton    = "button"
	StepWithFindByTypeTextInput = "input"
	StepWithFindByTypeCheckbox  = "checkbox"
	StepWithFindByTypeSelectbox = "selectbox"
	StepWithFindByTypeSearchbar = "searchbar"
	StepWithFindByTypeLink      = "link"
	StepWithFindByTypeIcon      = "icon"
	StepWithFindByTypeTextarea  = "textarea"
	StepWithListItem            = "listItem"
	StepWithFindByTypeText      = "text"

	StepWithFindValue = "findValue"

	StepActionFindFillTab    = "find_fill_and_tab"
	StepActionFindFillReturn = "find_fill_and_return"

	StepActionGET    = "GET"
	StepActionPOST   = "POST"
	StepActionPUT    = "PUT"
	StepActionPATCH  = "PATCH"
	StepActionDELETE = "DELETE"

	// Terminal Step Actions
	StepActionSh   = "sh"
	StepActionBash = "bash"

	// Code Step Actions
	StepActionPrompt = "prompt"

	// GDrive Step Actions
	StepActionGDriveList         = "list"
	StepActionGDriveUpload       = "upload"
	StepActionGDriveDownload     = "download"
	StepActionGDriveCreateFolder = "create_folder"
	StepActionGDriveDelete       = "delete"
	StepActionGDriveMove         = "move"
	StepActionGDriveSearch       = "search"
	StepActionGDriveGetMetadata  = "get_metadata"
	StepActionGDriveUpdate       = "update"
	StepActionGDriveExport       = "export"

	// GDrive Defaults
	DefaultGDrivePageSize = 100
	MaxGDrivePageSize     = 1000

	// Code Step With fields
	StepWithPrompt          = "prompt"
	StepWithCwd             = "cwd"
	StepWithSystemPrompt    = "systemPrompt"
	StepWithModel           = "model"
	StepWithMaxTurns        = "maxTurns"
	StepWithAllowedTools    = "allowedTools"
	StepWithDisallowedTools = "disallowedTools"
	StepWithIsPlanMode      = "isPlanMode"
	StepWithAdditionalDirs  = "additionalDirs"

	// Code Step With fields (taskComplexityLevel)
	StepWithTaskComplexityLevel = "taskComplexityLevel"

	// Task complexity levels
	TaskComplexityLow    = "low"
	TaskComplexityMedium = "medium"
	TaskComplexityHigh   = "high"

	// Default models per complexity level
	DefaultModelLow    = "claude-haiku-4-5-20251001"
	DefaultModelMedium = "claude-sonnet-4-5-20250929"
	DefaultModelHigh   = "claude-opus-4-6"

	// Code Step Output Suffix — appended to all code step prompts for structured output
	CodeStepOutputSuffix = `

CRITICAL — OUTPUT FORMAT:
Your final output MUST be a valid JSON object containing at minimum these two fields:
- "success": boolean — true if you completed the task, false if you encountered an error or could not complete it
- "error": string or null — if success is false, describe what went wrong

Include any other output fields requested above alongside these two fields. Example:
{"success": true, "error": null, "components": [...], "overallSummary": "..."}
Or on failure:
{"success": false, "error": "Could not access repository — permission denied"}`

	// Code Operation Defaults
	DefaultCodeModel    = "claude-sonnet-4-5-20250929"
	DefaultCodeMaxTurns = 300
	DefaultCodeTimeout  = 3600 // 1 hour in seconds
	MaxCodeMaxTurns     = 300
	MaxCodeTimeout      = 7200 // 2 hours in seconds

	StepWithRequestBody      = "requestBody"
	StepWithRequestBodyType  = "type"
	StepWithResponse         = "response"
	StepWithExtractAuthToken = "extractAuthToken"

	// ExtractAuthToken 'from' values
	ExtractAuthTokenFromHeader       = "header"
	ExtractAuthTokenFromResponseBody = "responseBody"

	// ExtractAuthToken defaults
	ExtractAuthTokenDefaultCacheTTL = 7200 // 2 hours in seconds
	StepBodyJSON                    = "application/json"
	StepBodyForm                    = "form-data"
	StepBodyMultipart               = "multipart/form-data"
	StepBodyUrlEncoded              = "application/x-www-form-urlencoded"

	// DB Operation Steps
	StepActionSelect = "select"
	StepActionWrite  = "write"

	// Initiate Workflow Operation Steps
	StepActionStartWorkflow = "start_workflow"

	// DB Operation With fields
	WithInit       = "init"
	WithMigrations = "migrations"

	// Terminal Operation With fields
	StepWithLinux   = "linux"
	StepWithMacOS   = "macos"
	StepWithWindows = "windows"
	StepWithTimeout = "timeout"

	// ForEach Operation fields
	ForEachItems     = "items"
	ForEachSeparator = "separator"
	ForEachIndexVar  = "indexVar"
	ForEachItemVar   = "itemVar"

	// Default ForEach values
	DefaultForEachSeparator = ","
	DefaultForEachIndexVar  = "index"
	DefaultForEachItemVar   = "item"

	// Default ForEach waitFor values
	DefaultForEachPollIntervalSeconds = 5
	DefaultForEachMaxWaitingSeconds   = 60

	StepOutputObject       = "object"
	StepOutputListOfObject = "list[object]"
	StepOutputListString   = "list[string]"
	StepOutputListNumber   = "list[number]"
	FieldTypeString        = "string"
	FieldTypeNumber        = "number"
	FieldTypeBoolean       = "boolean"

	// File Output Types
	OutputTypeFile     = "file"
	OutputTypeListFile = "list[file]"

	// Default file retention in seconds (10 minutes)
	DefaultFileRetention = 600

	// File size limits for saveAsFile
	MaxFileSizeDefault = 25 * 1024 * 1024  // 25MB default max file size
	MaxFileSizeHardCap = 100 * 1024 * 1024 // 100MB absolute maximum
)

// System Variables
const (
	SysVarMe      = "$ME"
	SysVarMessage = "$MESSAGE"
	SysVarNow     = "$NOW"
	SysVarUser    = "$USER"
	SysVarAdmin   = "$ADMIN"
	SysVarCompany = "$COMPANY"
	SysVarUUID    = "$UUID"
	SysVarFile    = "$FILE"
	SysVarMeeting = "$MEETING"
	SysVarEmail   = "$EMAIL" // Email-specific fields (available only for email triggers)
	SysVarTempDir = "$TEMP_DIR"
)

// System Functions
const (
	SysFuncAsk                          = "Ask"
	SysFuncLearn                        = "Learn"
	SysFuncAskHuman                     = "AskHuman"
	SysFuncNotifyHuman                  = "NotifyHuman"
	SysFuncSendTeamMessage              = "sendTeamMessage"
	SysFuncAskUser                      = "AskUser"
	SysFuncAskKnowledgeBase             = "askToKnowledgeBase"
	SysFuncRegisterKpi                  = "registerKpi"
	SysFuncRegisterAbTest               = "registerAbTest"
	SysFuncUpdateAbTestResult           = "updateAbTestResult"
	SysFuncGetAbTestStrategyPerformance = "getAbTestStrategyPerformance"

	// Meeting Bot System Functions (Recall.ai integration)
	SysFuncJoinMeetingBot         = "JoinMeetingBot"
	SysFuncSaveMeetingBot         = "saveMeetingBot"
	SysFuncListMeetingBots        = "listMeetingBots"
	SysFuncGetMeetingBotStatus    = "getMeetingBotStatus"
	SysFuncSendMeetingChatMessage = "sendMeetingChatMessage"

	// Hook System Functions
	SysFuncRegisterHook   = "registerHook"
	SysFuncUnregisterHook = "unregisterHook"
	SysFuncListHooks      = "listHooks"

	// Memory System Functions
	SysFuncCreateMemory  = "createMemory"
	SysFuncQueryMemories = "queryMemories"

	// Callback-only System Functions (NOT available to LLM coordinator, only from onsuccess/onFailure/needs)
	SysFuncSendMessageToUser = "sendMessageToUser"
)

const (
	StatusComplete = "complete"
	StatusFailed   = "failed"
	StatusPending  = "pending"
	StatusSkipped  = "skipped"
)

// CallRule types
const (
	CallRuleTypeOnce     = "once"
	CallRuleTypeUnique   = "unique"
	CallRuleTypeMultiple = "multiple"
)

// CallRule scopes
const (
	CallRuleScopeUser            = "user"
	CallRuleScopeMessage         = "message"
	CallRuleScopeMinimumInterval = "minimumInterval"
	CallRuleScopeCompany         = "company"
)

// CallRule status filter values
// Note: These must match the actual status values stored in the database (StatusComplete, StatusFailed, etc.)
const (
	CallRuleStatusFilterCompleted = "complete" // matches StatusComplete
	CallRuleStatusFilterFailed    = "failed"   // matches StatusFailed
	CallRuleStatusFilterAll       = "all"      // special value meaning all statuses
)

// ValidCallRuleStatusFilters contains all valid values for CallRule.StatusFilter
var ValidCallRuleStatusFilters = []string{
	CallRuleStatusFilterCompleted,
	CallRuleStatusFilterFailed,
	CallRuleStatusFilterAll,
}

// Concurrency control strategies for time-based triggers
const (
	ConcurrencyStrategyParallel = "parallel"
	ConcurrencyStrategySkip     = "skip"
	ConcurrencyStrategyKill     = "kill"
)

// Concurrency control defaults
const (
	DefaultConcurrencyStrategy = ConcurrencyStrategyParallel
	DefaultMaxParallel         = 3
	MaxAllowedParallel         = 10
	DefaultKillTimeoutSeconds  = 600
)

// Special filter values for 'from' field
const (
	FilterScratchpad = "scratchpad"
)

// System variable availability by trigger type
var triggerVarAvailability = map[string]map[string]bool{
	TriggerOnUserMessage: {
		SysVarMe:      true,
		SysVarMessage: true,
		SysVarNow:     true,
		SysVarUser:    true,
		SysVarAdmin:   true,
		SysVarCompany: true,
		SysVarUUID:    true,
		SysVarFile:    true,
		SysVarTempDir: true,
	},
	TriggerOnTeamMessage: {
		SysVarMe:      true,
		SysVarMessage: true,
		SysVarNow:     true,
		SysVarUser:    true,
		SysVarAdmin:   true,
		SysVarCompany: true,
		SysVarUUID:    true,
		SysVarFile:    true,
		SysVarTempDir: true,
	},
	TriggerOnCompletedUserMessage: {
		SysVarMe:      true,
		SysVarMessage: true,
		SysVarNow:     true,
		SysVarUser:    true,
		SysVarAdmin:   true,
		SysVarCompany: true,
		SysVarUUID:    true,
		SysVarFile:    true,
		SysVarTempDir: true,
	},
	TriggerOnCompletedTeamMessage: {
		SysVarMe:      true,
		SysVarMessage: true,
		SysVarNow:     true,
		SysVarUser:    true,
		SysVarAdmin:   true,
		SysVarCompany: true,
		SysVarUUID:    true,
		SysVarFile:    true,
		SysVarTempDir: true,
	},
	TriggerFlexForUser: {
		SysVarMe:      true,
		SysVarMessage: true,
		SysVarNow:     true,
		SysVarUser:    true,
		SysVarAdmin:   true,
		SysVarCompany: true,
		SysVarUUID:    true,
		SysVarFile:    true,
		SysVarTempDir: true,
	},
	TriggerFlexForTeam: {
		SysVarMe:      true,
		SysVarMessage: true,
		SysVarNow:     true,
		SysVarUser:    true,
		SysVarAdmin:   true,
		SysVarCompany: true,
		SysVarUUID:    true,
		SysVarFile:    true,
		SysVarTempDir: true,
	},
	TriggerTime: {
		SysVarMe:      true,
		SysVarNow:     true,
		SysVarAdmin:   true,
		SysVarCompany: true,
		SysVarUUID:    true,
		SysVarFile:    true,
		SysVarTempDir: true,
	},
	TriggerOnMeetingStart: {
		SysVarMe:      true,
		SysVarNow:     true,
		SysVarAdmin:   true,
		SysVarCompany: true,
		SysVarUUID:    true,
		SysVarFile:    true,
		SysVarMeeting: true,
		SysVarTempDir: true,
	},
	TriggerOnMeetingEnd: {
		SysVarMe:      true,
		SysVarNow:     true,
		SysVarAdmin:   true,
		SysVarCompany: true,
		SysVarUUID:    true,
		SysVarFile:    true,
		SysVarMeeting: true,
		SysVarTempDir: true,
	},
	// Email triggers - have access to $EMAIL instead of $MESSAGE
	TriggerOnUserEmail: {
		SysVarMe:      true,
		SysVarEmail:   true,
		SysVarNow:     true,
		SysVarUser:    true,
		SysVarAdmin:   true,
		SysVarCompany: true,
		SysVarUUID:    true,
		SysVarFile:    true,
	},
	TriggerOnTeamEmail: {
		SysVarMe:      true,
		SysVarEmail:   true,
		SysVarNow:     true,
		SysVarUser:    true,
		SysVarAdmin:   true,
		SysVarCompany: true,
		SysVarUUID:    true,
		SysVarFile:    true,
	},
	TriggerOnCompletedUserEmail: {
		SysVarMe:      true,
		SysVarEmail:   true,
		SysVarNow:     true,
		SysVarUser:    true,
		SysVarAdmin:   true,
		SysVarCompany: true,
		SysVarUUID:    true,
		SysVarFile:    true,
	},
	TriggerOnCompletedTeamEmail: {
		SysVarMe:      true,
		SysVarEmail:   true,
		SysVarNow:     true,
		SysVarUser:    true,
		SysVarAdmin:   true,
		SysVarCompany: true,
		SysVarUUID:    true,
		SysVarFile:    true,
	},
	TriggerFlexForUserEmail: {
		SysVarMe:      true,
		SysVarEmail:   true,
		SysVarNow:     true,
		SysVarUser:    true,
		SysVarAdmin:   true,
		SysVarCompany: true,
		SysVarUUID:    true,
		SysVarFile:    true,
	},
	TriggerFlexForTeamEmail: {
		SysVarMe:      true,
		SysVarEmail:   true,
		SysVarNow:     true,
		SysVarUser:    true,
		SysVarAdmin:   true,
		SysVarCompany: true,
		SysVarUUID:    true,
		SysVarFile:    true,
	},
}

// System function availability by trigger type
var triggerFuncAvailability = map[string]map[string]bool{
	TriggerOnUserMessage: {
		SysFuncAsk:         true,
		SysFuncLearn:       true,
		SysFuncAskHuman:    true,
		SysFuncNotifyHuman: true,
		SysFuncAskUser:     true,
	},
	TriggerOnTeamMessage: {
		SysFuncAsk:         true,
		SysFuncLearn:       true,
		SysFuncAskHuman:    true,
		SysFuncNotifyHuman: true,
		SysFuncAskUser:     true,
	},
	TriggerOnCompletedUserMessage: {
		SysFuncAsk:         true,
		SysFuncLearn:       true,
		SysFuncAskHuman:    true,
		SysFuncNotifyHuman: true,
		SysFuncAskUser:     true,
	},
	TriggerOnCompletedTeamMessage: {
		SysFuncAsk:         true,
		SysFuncLearn:       true,
		SysFuncAskHuman:    true,
		SysFuncNotifyHuman: true,
		SysFuncAskUser:     true,
	},
	TriggerFlexForUser: {
		SysFuncAsk:         true,
		SysFuncLearn:       true,
		SysFuncAskHuman:    true,
		SysFuncNotifyHuman: true,
	},
	TriggerFlexForTeam: {
		SysFuncAsk:         true,
		SysFuncLearn:       true,
		SysFuncAskHuman:    true,
		SysFuncNotifyHuman: true,
	},
	TriggerTime: {
		SysFuncAsk:         true,
		SysFuncLearn:       true,
		SysFuncAskHuman:    true,
		SysFuncNotifyHuman: true,
	},
	TriggerOnMeetingStart: {
		SysFuncAsk:         true,
		SysFuncLearn:       true,
		SysFuncAskHuman:    true,
		SysFuncNotifyHuman: true,
	},
	TriggerOnMeetingEnd: {
		SysFuncAsk:         true,
		SysFuncLearn:       true,
		SysFuncAskHuman:    true,
		SysFuncNotifyHuman: true,
	},
	// Email triggers - same functions available as message triggers
	TriggerOnUserEmail: {
		SysFuncAsk:         true,
		SysFuncLearn:       true,
		SysFuncAskHuman:    true,
		SysFuncNotifyHuman: true,
		SysFuncAskUser:     true,
	},
	TriggerOnTeamEmail: {
		SysFuncAsk:         true,
		SysFuncLearn:       true,
		SysFuncAskHuman:    true,
		SysFuncNotifyHuman: true,
		SysFuncAskUser:     true,
	},
	TriggerOnCompletedUserEmail: {
		SysFuncAsk:         true,
		SysFuncLearn:       true,
		SysFuncAskHuman:    true,
		SysFuncNotifyHuman: true,
		SysFuncAskUser:     true,
	},
	TriggerOnCompletedTeamEmail: {
		SysFuncAsk:         true,
		SysFuncLearn:       true,
		SysFuncAskHuman:    true,
		SysFuncNotifyHuman: true,
		SysFuncAskUser:     true,
	},
	TriggerFlexForUserEmail: {
		SysFuncAsk:         true,
		SysFuncLearn:       true,
		SysFuncAskHuman:    true,
		SysFuncNotifyHuman: true,
	},
	TriggerFlexForTeamEmail: {
		SysFuncAsk:         true,
		SysFuncLearn:       true,
		SysFuncAskHuman:    true,
		SysFuncNotifyHuman: true,
	},
}

var systemVarFields = map[string]map[string]bool{
	SysVarMe: {
		"name":        true,
		"version":     true,
		"description": true,
	},
	SysVarMessage: {
		"id":        true,
		"text":      true,
		"from":      true,
		"channel":   true,
		"timestamp": true,
		"hasMedia":  true,
		"humor":     true,
		"client_id": true,
	},
	SysVarUser: {
		"id":             true,
		"first_name":     true,
		"last_name":      true,
		"email":          true,
		"phone":          true,
		"gender":         true,
		"address":        true,
		"company_id":     true,
		"company_name":   true,
		"language":       true,
		"messages_count": true,
		"birth_date":     true,
	},
	SysVarAdmin: {
		"id":           true,
		"first_name":   true,
		"last_name":    true,
		"email":        true,
		"phone":        true,
		"gender":       true,
		"address":      true,
		"company_id":   true,
		"company_name": true,
		"language":     true,
		"birth_date":   true,
	},
	SysVarCompany: {
		"id":                true,
		"name":              true,
		"fantasy_name":      true,
		"tax_code":          true,
		"industry":          true,
		"email":             true,
		"instagram_profile": true,
		"website":           true,
		"ai_session_id":     true,
	},
	SysVarNow: {
		"date":    true, // YYYY-MM-DD format
		"time":    true, // HH:MM:SS format
		"hour":    true, // Hour (0-23)
		"unix":    true, // Unix timestamp
		"iso8601": true, // ISO 8601 / RFC3339 format
	},
	SysVarUUID:    {}, // UUID doesn't support any fields
	SysVarTempDir: {}, // Resolves to temporary workspace directory path
	SysVarFile: {
		"url":      true, // URL of the file
		"path":     true, // Local file path (triggers file upload in multipart requests)
		"mimetype": true, // MIME type of the file
		"filename": true, // Name of the file
	},
	SysVarMeeting: {
		"bot_id":    true, // The Recall.ai bot ID
		"event":     true, // "start" or "end"
		"timestamp": true, // ISO8601 timestamp of the event
	},
	SysVarEmail: {
		"thread_id":       true, // Email thread/conversation identifier
		"message_id":      true, // Unique email message ID
		"subject":         true, // Email subject line
		"sender":          true, // Sender email address (from)
		"recipients":      true, // Comma-separated list of TO addresses
		"cc":              true, // Comma-separated list of CC addresses
		"bcc":             true, // Comma-separated list of BCC addresses
		"in_reply_to":     true, // Message ID of the email being replied to
		"references":      true, // Related message IDs for threading
		"date":            true, // Email date/timestamp (ISO8601)
		"text_body":       true, // Plain text body content
		"has_attachments": true, // Whether email has attachments
	},
}
var systemFunctions = map[string]bool{
	SysFuncAsk:                          true,
	SysFuncLearn:                        true,
	SysFuncAskHuman:                     true,
	SysFuncNotifyHuman:                  true,
	SysFuncSendTeamMessage:              true,
	SysFuncAskUser:                      true,
	SysFuncAskKnowledgeBase:             true,
	SysFuncRegisterKpi:                  true,
	SysFuncRegisterAbTest:               true,
	SysFuncUpdateAbTestResult:           true,
	SysFuncGetAbTestStrategyPerformance: true,

	// Meeting Bot System Functions
	SysFuncJoinMeetingBot:         true,
	SysFuncSaveMeetingBot:         true,
	SysFuncListMeetingBots:        true,
	SysFuncGetMeetingBotStatus:    true,
	SysFuncSendMeetingChatMessage: true,

	// Hook System Functions
	SysFuncRegisterHook:   true,
	SysFuncUnregisterHook: true,
	SysFuncListHooks:      true,

	// Memory System Functions
	SysFuncCreateMemory:  true,
	SysFuncQueryMemories: true,
}

// systemFunctionsWithParams defines which system functions can accept parameters
// These are typically system functions loaded from external YAML files at runtime
// These functions can be called from onSuccess, onFailure, and needs with params
var systemFunctionsWithParams = map[string]bool{
	SysFuncLearn:                        true,
	SysFuncSendTeamMessage:              true,
	SysFuncRegisterKpi:                  true,
	SysFuncRegisterAbTest:               true,
	SysFuncUpdateAbTestResult:           true,
	SysFuncGetAbTestStrategyPerformance: true,

	// Meeting Bot System Functions
	SysFuncJoinMeetingBot:         true,
	SysFuncSaveMeetingBot:         true,
	SysFuncListMeetingBots:        true,
	SysFuncGetMeetingBotStatus:    true,
	SysFuncSendMeetingChatMessage: true,

	// Hook System Functions
	SysFuncRegisterHook:   true,
	SysFuncUnregisterHook: true,
	SysFuncListHooks:      true,

	// Memory System Functions
	SysFuncCreateMemory:  true,
	SysFuncQueryMemories: true,

	// Callback-only System Functions (NOT in systemFunctions - prevents LLM from calling directly)
	SysFuncSendMessageToUser: true,
}

// SharedTool is the argument passed to CreateTool to indicate a shared tool.
// Shared tools are embedded at compile time and visible to all tools via needs/onSuccess/onFailure.
// Unlike system tools (which use connectai.db), shared tools get their own database per tool name.
const SharedTool = "sharedTool"

// NOTE: Shared function maps (sharedFunctions, sharedFunctionsWithParams) are now
// dynamically populated from embedded YAML files in pkg/shared_tools.
// Import shared_tools package and use SharedFunctions and SharedFunctionsWithParams maps instead.
