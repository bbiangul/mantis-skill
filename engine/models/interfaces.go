package models

import (
	"context"
	"time"

	"github.com/bbiangul/mantis-skill/types"
	"github.com/bbiangul/mantis-skill/skill"
	"github.com/henomis/lingoose/thread"
	"github.com/pemistahl/lingua-go"
)

// LangChainTool defines the interface for langchain-compatible tools.
// Replaces the external github.com/IT-Tech-Company/langchaingo/tools.Tool dependency.
type LangChainTool interface {
	Name() string
	Description() string
	Call(ctx context.Context, input string) (string, error)
}

type LoopContext struct {
	IndexVar string
	ItemVar  string
	Index    int
	Item     interface{}
	Result   interface{} // Result of the current iteration (available in waitFor)
}

// UserProxyFinalResponseFunc is the function type for generating user-facing messages through LLM proxy.
// Used to avoid import cycles between tool_engine and agents packages.
type UserProxyFinalResponseFunc func(ctx context.Context, retrievedMsg types.Message, agentsMessage string, messages *thread.Thread, language lingua.Language) (string, error)

// AdminProxyFinalResponseFunc is the function type for generating admin/staff-facing messages through LLM proxy.
// Used to avoid import cycles between tool_engine and agents packages.
type AdminProxyFinalResponseFunc func(ctx context.Context, agentsMessage string, messages *thread.Thread, language lingua.Language) (string, error)

// IWorkflowInitiator defines the interface for initiating workflows
type IWorkflowInitiator interface {
	// InitiateWorkflow queues a synthetic message for workflow execution
	// functionParams is a map of function keys ("toolName.functionName") to pre-filled input values (can be nil)
	// enforcedWorkflow is the workflow category name to bypass LLM categorization (can be empty)
	InitiateWorkflow(ctx context.Context, msg types.Message, contextForAgent string, functionParams map[string]map[string]interface{}, enforcedWorkflow string) error
	UserExists(ctx context.Context, userID string) (bool, error)
	GetUser(ctx context.Context, userID string) (*types.User, error)
	CreateUser(ctx context.Context, user types.User) error
	// GetUserLastSession returns the session ID from the user's most recent message
	// Returns empty string if no previous messages exist
	GetUserLastSession(ctx context.Context, userID string) (string, error)
}

// ITempFileManager defines the interface for managing temporary files with retention
type ITempFileManager interface {
	// SaveTempFile saves bytes to a temp file with the specified retention period
	// Returns the full path to the saved file
	SaveTempFile(companyID, filename string, data []byte, retentionSeconds int) (string, error)
	// GetTempFile reads and returns the contents of a temp file
	GetTempFile(path string) ([]byte, error)
	// GetTempFileBase64 reads a temp file and returns it as base64-encoded string
	GetTempFileBase64(path string) (string, error)
	// IsFileTracked checks if a file path is being tracked (and not expired)
	IsFileTracked(path string) bool
}

type IToolEngine interface {
	Initialize(ctx context.Context, inputFulfiller IInputFulfiller, systemApps []types.App) error
	GetToolByName(name, version string) (skill.Tool, bool)
	GetLangChainFunctionByName(toolName, name, version string) (LangChainTool, bool)
	GetAllTools() []skill.Tool
	GetAllFunctionsDescription(triggerType skill.Trigger) string
	GetFunctionsWithFlexTrigger(isUserContext bool, channel string) []FlexFunction
	GetFlexFunctionsForMap(
		toolFuncMap map[string][]string,
	) []FlexFunction
	UpdateToolEngine(ctx context.Context, inputFulfiller IInputFulfiller) error
	ForceReloadTools(ctx context.Context, inputFulfiller IInputFulfiller) error
	ExecuteFunctionByNameAndVersion(
		ctx context.Context,
		messageID string,
		toolName string,
		toolVersion string,
		functionName string,
		inputFulfiller IInputFulfiller,
	) (string, error)

	FindFunctionDefinition(tool *skill.Tool, functionName string) (*skill.Function, error)
	FindFunctionAndTool(tool *skill.Tool, functionName string) (*skill.Function, *skill.Tool, error)
	ExecutionTracker() IFunctionExecutionTracker

	ExecuteDependencies(
		ctx context.Context,
		messageID string,
		clientID string,
		tool *skill.Tool,
		needs []skill.NeedItem,
		inputFulfiller IInputFulfiller,
		callback AgenticWorkflowCallback,
		parentFunc string,
	) (bool, []string, error)
	GetVariableReplacer() IVariableReplacer
	GetAuthProvider() types.AuthProvider
	GetWorkflowInitiator() IWorkflowInitiator
	GetTempFileManager() ITempFileManager
	SetAskHumanCallback(callback func(ctx context.Context, query string) (string, error))
	SetToolFunctions(tf ToolFunctions)
	GetToolFunctions() ToolFunctions
}

type IInputFulfiller interface {
	FulfillInputs(ctx context.Context, messageID, clientID string, tool *skill.Tool, functionName string, inputs []skill.Input) (map[string]interface{}, error)
	ResolveToolExecution(ctx context.Context, stateID int64, value string) error
	AgenticInfer(ctx context.Context,
		messageID,
		clientID string,
		input skill.Input,
		tool *skill.Tool) (string, error)
	RequestUserInput(
		ctx context.Context,
		messageID string,
		paramName string,
		additionalInfo string,
	) (string, error)
}

type IAgenticCoordinator interface {
	ExecuteWorkflow(ctx context.Context, userMessage, contextForAgent string, toolsAndFunctions map[string][]string) (string, error)
	SetMaxTurns(maxTurns int)
	GetAvailableTools(ctx context.Context, toolsAndFunctions map[string][]string) ([]string, []string)
	SetCallback(callback AgenticWorkflowCallback)
	StateStore() *StateStore

	// Checkpoint methods for pause/resume functionality
	CreateCheckpoint(ctx context.Context, messageID, clientID, sessionID string, pauseReason string, toolName string, functionName string, stepKey string, toolsAndFunctions map[string][]string, state CoordinatorState) (*types.ExecutionCheckpoint, error)
	RestoreFromCheckpoint(ctx context.Context, checkpoint *types.ExecutionCheckpoint, currentMessage types.Message) (context.Context, error)
	ContinueExecution(ctx context.Context, sessionID string) (string, string, error)
	ApproveCheckPoint(ctx context.Context, sessionID string) error
	MarkCheckPointAsResumed(ctx context.Context, sessionID string) error
	MarkCheckPointAsComplete(ctx context.Context, sessionID string) error
}

type IFunctionExecutionTracker interface {
	RecordExecution(ctx context.Context, messageID, clientID, toolName, functionName, functionDescription string, inputs map[string]interface{}, inputsHash string, output string, originalOutput *string, startTime time.Time, status string, function *skill.Function) error
	RecordStepResult(ctx context.Context, messageID string, functionName string, resultIndex int, resultData interface{}) error
	GetStepResult(ctx context.Context, messageID string, resultIndex int) (*StepResult, error)
	GetStepResultField(ctx context.Context, messageID string, resultIndex int, fieldPath string) (interface{}, error)
	HasExecuted(ctx context.Context, messageID, functionName string) (bool, string, error)
	LoadFunctionsForMessage(ctx context.Context, messageID string) error
	LoadStepResultsForMessage(ctx context.Context, messageID string) error
	CheckNeeds(ctx context.Context, messageID string, needs []string) (allMet bool, missingNeeds []string, executionResults []string, err error)
	GetExecutionResult(ctx context.Context, messageID string, functionName string) (string, error)
	ClearCache()
	ClearCacheForMessage(messageID string)

	GetFromGlobalCache(ctx context.Context, toolName, functionName string, inputs map[string]interface{}) (string, bool, error)
	GetSummaryFromGlobalCache(ctx context.Context, toolName, functionName string, inputs map[string]interface{}) (string, bool, error)
	AddToGlobalCache(ctx context.Context, toolName, functionName string, inputs map[string]interface{}, output, summary string, ttl int) error
	StartCacheCleanup(interval time.Duration)

	// Scoped cache methods - support for client/message scoped caching
	// scope: "global", "client", or "message"
	// scopeID: clientID for client scope, messageID for message scope, empty for global
	// inputs: can be nil if includeInputs is false
	GetFromScopedCache(ctx context.Context, toolName, functionName string, scope skill.CacheScope, scopeID string, inputs map[string]interface{}) (string, bool, error)
	AddToScopedCache(ctx context.Context, toolName, functionName string, scope skill.CacheScope, scopeID string, inputs map[string]interface{}, output string, ttl int) error

	// CallRule related methods
	CheckCallRuleViolationWithoutInputs(ctx context.Context, messageID, clientID, toolName, functionName string, callRule *skill.CallRule) (bool, string, error)
	CheckCallRuleViolation(ctx context.Context, messageID, clientID, toolName, functionName string, inputs map[string]interface{}, callRule *skill.CallRule) (bool, string, string, error)

	// GatherInfo cache methods - for shouldBeHandledAsMessageToUser functionality
	// AddGatherInfo stores information that should be surfaced to the user
	AddGatherInfo(messageID string, info string)
	// GetAndClearGatherInfo retrieves all gathered info for a message and clears the cache
	GetAndClearGatherInfo(messageID string) []string

	// FilesToShare cache methods - for shouldBeHandledAsMessageToUser functionality with file outputs
	// AddFileToShare stores a file (e.g., PDF) that should be attached to the user response
	AddFileToShare(messageID string, file *types.MediaType)
	// GetAndClearFilesToShare retrieves all files to share for a message and clears the cache
	GetAndClearFilesToShare(messageID string) []*types.MediaType

	// ResponseLanguage override - set by YAML functions via responseLanguage field
	// Used by GenerateFinalResponse to override language detection for user proxy
	SetResponseLanguage(messageID string, language string)
	GetResponseLanguage(messageID string) string
}

// AgenticWorkflowCallback defines the interface for workflow execution callbacks
type AgenticWorkflowCallback interface {
	OnWorkflowPlanned(ctx context.Context, messageID string, clientID string, workflow *CoordinatorState, humanVerified bool, isNewMessage bool) (interface{}, error)

	// OnStepProposed is called when the LLM proposes a new step
	OnStepProposed(ctx context.Context, stepKey string, messageID string, clientID string, toolName string, rationale string)

	OnStepPausedDueHumanSupportRequired(ctx context.Context, eventKey, messageID, clientID, step string, humanSupportType types.HumanSupportType, humanQaId string, humanSupportMessage types.HumanSupportMessage)

	// OnStepPausedDueApprovalRequired is called when a step requires team approval
	OnStepPausedDueApprovalRequired(ctx context.Context, stepKey, messageID, clientID, toolName, functionName string, checkpoint *types.ExecutionCheckpoint)

	// OnStepPausedDueMissingInput is called when a step requires missing input from user
	OnStepPausedDueMissingInput(ctx context.Context, stepKey, messageID, clientID, toolName, functionName string, checkpoint *types.ExecutionCheckpoint)

	OnStepPausedDueRequiringUserConfirmation(ctx context.Context, stepKey, messageID, clientID, toolName, functionName string, checkpoint *types.ExecutionCheckpoint)

	// OnWorkflowPaused is called when a workflow is paused
	OnWorkflowPaused(ctx context.Context, messageID, clientID string, currentStep string, reason string, checkpoint *types.ExecutionCheckpoint)

	// OnWorkflowResumed is called when a workflow is resumed
	OnWorkflowResumed(ctx context.Context, messageID, clientID string, checkpoint *types.ExecutionCheckpoint)

	// OnStatePersisted is called when execution state is saved
	OnStatePersisted(ctx context.Context, messageID, clientID string, checkpointID string)

	// OnStateRestored is called when execution state is loaded
	OnStateRestored(ctx context.Context, messageID, clientID string, checkpointID string)

	// OnStepExecuted is called after a tool is executed
	OnStepExecuted(ctx context.Context, stepKey string, messageID string, clientID string, toolname string, output string, error error)

	OnFunctionExecuted(ctx context.Context, stepKey string, messageID string, clientID string, functionName, toolName, functionDescription string, inputs, output string, err error, function *skill.Function)

	OnInputsFulfilled(ctx context.Context, messageID, toolName, functionName string, inputs map[string]interface{})

	GetFulfilledInputs(messageID, toolName, functionName string) (map[string]interface{}, bool)

	// GetAllFulfilledInputsForMessage returns all fulfilled inputs for all functions in a message
	// Returns a map with keys in format "toolName.functionName" -> inputs map
	GetAllFulfilledInputsForMessage(messageID string) map[string]map[string]interface{}

	// OnWorkflowCompleted is called when the workflow is complete
	OnWorkflowCompleted(ctx context.Context, messageID string, clientID string, summary types.WorkflowSummary, stateStore StateStore, isNewMessage bool)

	OnDependencyExecuted(ctx context.Context, messageID string, clientID string, functionName, dependencyFunctionName string, result string, err error)

	OnInputFulfillDependencyExecuted(ctx context.Context, messageID, clientID string, inputFieldName, inputFieldDescription, functionExecuted string, result string, err error)

	GetEvents(id string, group CacheGroup) []WorkflowEvent

	GetEventsString(ctx context.Context, id string, group CacheGroup, status WorkflowEventType) string

	GetEventsStringWithContext(ctx context.Context, id string, group CacheGroup, status WorkflowEventType) string

	// OnScratchpadUpdated is called when the scratchpad (accumulated workflow context) is updated
	OnScratchpadUpdated(ctx context.Context, messageID, clientID string, event WorkflowEvent)

	RestoreEvents(ctx context.Context, events []interface{}) error

	SetTaskService(taskService interface{})
}

// StateStore defines the interface for workflow state storage operations
type StateStore interface {
	GetState(messageID string) *CoordinatorState
	RemoveState(messageID string)
	GetActiveWorkflows() []string
}

type ToolFunctions interface {
	Ask(ctx context.Context, query string) (string, error)
	Learn(ctx context.Context, textOrMediaLink string) (string, error)
	AskHuman(ctx context.Context, query string) (string, error)
	RequestHumanAction(ctx context.Context, text string) (string, error)
	RequestHumanApproval(ctx context.Context, text string) error
	AskKnowledge(ctx context.Context, query string) (string, error)
	AskStructuredKnowledge(ctx context.Context, query string, knowledgeKeys []string) (string, error)
	FindProperImages(ctx context.Context, query string) (string, error)
	AskBasedOnConversation(ctx context.Context, query string) (string, error)
	NotifyHuman(ctx context.Context, text, role string) (string, error)
	SendTeamMessage(ctx context.Context, message, role string) (string, error)
	SendMessageToUser(ctx context.Context, message string) error
	AskAboutAgenticEvents(ctx context.Context, query string) (string, error)
	AskAboutAgenticEventsWithRag(ctx context.Context, query string) (string, error)
	DeepResearch(ctx context.Context, query, finalResponse string) (string, error)
	Search(ctx context.Context, query string) (string, error)
	ChangeAIPersona(ctx context.Context, rationale string) (string, error)
	QueryMetrics(ctx context.Context) (string, error)
	ChangeAnswerMode(ctx context.Context) (string, error)
	Teach(ctx context.Context, rationale string) (string, error)
	GetWeekdayFromDate(ctx context.Context, dateStr string) (string, error)
	QueryMemories(ctx context.Context, query string) (string, error)
	QueryMemoriesWithFilters(ctx context.Context, query string, filters *skill.MemoryFilters) (string, error)
	CreateMemory(ctx context.Context, content string, topic string) (string, error)
	FetchWebContent(ctx context.Context, url string) (string, error)
	ProcessEventHistoryInChunksWithLLM(ctx context.Context, eventHistory string, query string, llmToUse interface{}) (string, error)
	ExtractSnippetsFromEventHistoryInChunksWithLLM(ctx context.Context, eventHistory string, query string, llmToUse interface{}) (string, error)
	QueryCustomerServiceChats(ctx context.Context, query string, clientIds []string) (string, error)
	AnalyzeImage(ctx context.Context, rationale string) (string, error)
	SearchCodebase(ctx context.Context, query string, dirs []string) (string, error)
	QueryDocuments(ctx context.Context, query string, dbName string, enableGraph bool) (string, error)
}

// ReplaceOptions configures variable replacement behavior
type ReplaceOptions struct {
	// DBContext controls NULL handling: true = SQL NULL keyword, false = empty string
	DBContext bool
	// KeepUnresolvedPlaceholders controls what happens when a variable path doesn't exist:
	// - false (default): unresolved $var becomes "" (empty string) - safe for API calls
	// - true: unresolved $var stays as "$var" - useful for LLM prompts where readability matters
	KeepUnresolvedPlaceholders bool
	// ShellEscape escapes single quotes in replaced values for safe embedding in bash scripts.
	// Uses the standard technique: ' -> '\'' (close quote, escaped quote, reopen quote).
	// Only used by terminal operations to prevent shell injection from LLM/user-generated content.
	ShellEscape bool
}

type IVariableReplacer interface {
	ReplaceVariables(ctx context.Context, text string, inputs map[string]interface{}) (string, error)
	ReplaceVariablesWithContext(ctx context.Context, text string, inputs map[string]interface{}, dbContext bool) (string, error)
	ReplaceVariablesWithOptions(ctx context.Context, text string, inputs map[string]interface{}, opts ReplaceOptions) (string, error)
	ReplaceVariablesForDB(ctx context.Context, text string, inputs map[string]interface{}) (string, []interface{}, error)
	ReplaceVariablesWithLoop(ctx context.Context, text string, inputs map[string]interface{}, loopContext *LoopContext) (string, error)
	SetEnvironmentVariables(envVars []skill.EnvVar)
	GetEnvironmentVariables() map[string]string
	NavigatePath(data interface{}, path string) (interface{}, error)
	NavigatePathWithTransformation(inputs map[string]interface{}, path string) (interface{}, error)
	// Auth token methods for $AUTH_TOKEN support
	SetAuthToken(token string)
	GetAuthToken() string
	ClearAuthToken()
}

type IWorkflowService interface {
	CategorizeUserMessage(ctx context.Context, userMessage, contextForAgent string, workflowType types.WorkflowType, allowWorkflowGeneration bool) (string, error)
	GenerateWorkflow(ctx context.Context, userMessage, contextForAgent string, availableTools []string, workflowType types.WorkflowType) (*types.Workflow, error)
	GetWorkflowByCategory(ctx context.Context, categoryName string) (*types.Workflow, error)
	GetAllCategories(ctx context.Context) ([]types.Workflow, error)
	GetCategoriesByType(ctx context.Context, workflowType types.WorkflowType) ([]types.Workflow, error)
	SaveWorkflow(ctx context.Context, workflow *types.Workflow) error
	UpdateWorkflow(ctx context.Context, workflow *types.Workflow) error
	AddStepToWorkflow(ctx context.Context, workflowID int64, step *types.WorkflowStep) error
	UpdateWorkflowStep(ctx context.Context, step *types.WorkflowStep) error
	DeleteWorkflowStep(ctx context.Context, stepID int64) error
	UpdateWorkflowFromFeedback(ctx context.Context, categoryName, feedback string) (*types.Workflow, error)
	GetWorkflowSteps(ctx context.Context, workflowID int64) ([]types.WorkflowStep, error)
	GetWorkflowRepository(ctx context.Context) types.WorkflowRepository
}

// ITriggerSystem defines the interface for the trigger system
// Used by the unified dispatcher to process meeting events
type ITriggerSystem interface {
	// ProcessMeetingEvent processes functions with meeting-based triggers
	// botID is the Recall.ai bot ID, eventType is "in_call_recording" or "call_ended"
	ProcessMeetingEvent(ctx context.Context, botID string, eventType string) error
}

// IMockService defines the interface for test mocking in the Tacitron testing framework.
// This interface enables tests to intercept external calls (API, terminal, etc.) without
// modifying production behavior. The context key "testMockService" is used to inject
// the mock service during test execution.
type IMockService interface {
	// ShouldMock determines if an operation should be mocked based on:
	// 1. Mock mode (full vs selective)
	// 2. Operation type (api_call, terminal, db, etc.)
	// 3. Whether a mock is defined for this function/step
	ShouldMock(operationType, functionKey, stepName string) bool

	// GetMockResponseValue returns just the response value for the given function/step
	// Returns nil, false if no mock is defined
	GetMockResponseValue(functionKey, stepName string) (interface{}, bool)

	// RecordCall records a mock call for verification
	RecordCall(functionKey, stepName, operationType string, inputs map[string]interface{}, response interface{}, err string)
}

// TestMockServiceKey is the context key for injecting IMockService during test execution.
// Production code NEVER sets this, so the mock check is always a no-op in production.
const TestMockServiceKey = "testMockService"

// IContainerExecutor defines the interface for executing commands in an isolated container.
// This is used by the Tacitron testing framework to run terminal commands in an environment
// that matches production (Alpine Linux with bash, jq, etc.).
type IContainerExecutor interface {
	// Start initializes and starts the container
	Start(ctx context.Context) error

	// ExecuteBash executes a bash script inside the container and returns the output
	ExecuteBash(ctx context.Context, script string) (string, error)

	// ExecuteBashWithEnv executes a bash script with environment variables
	ExecuteBashWithEnv(ctx context.Context, script string, env map[string]string) (string, error)

	// Stop stops and removes the container
	Stop(ctx context.Context) error

	// IsStarted returns whether the container is running
	IsStarted() bool

	// GetImage returns the container image being used
	GetImage() string
}

// TestContainerExecutorKey is the context key for injecting IContainerExecutor during test execution.
// When this key is present in context, terminal operations are executed inside the container
// instead of on the host machine.
const TestContainerExecutorKey = "testContainerExecutor"
