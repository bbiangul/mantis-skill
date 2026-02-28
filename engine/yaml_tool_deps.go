package engine

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/bbiangul/mantis-skill/skill"
	"github.com/henomis/lingoose/thread"
)

// threadMessage is a package-level alias for thread.Message so the rest of the
// engine package can reference it without importing "thread" everywhere.
type threadMessage = thread.Message

// threadMessages wraps a slice of threadMessage for helper methods.
type threadMessages []threadMessage

// ConversationHistory returns a single string representation of the messages,
// suitable for embedding in an LLM prompt.
func (msgs threadMessages) ConversationHistory() string {
	var sb strings.Builder
	for _, m := range msgs {
		role := string(m.Role)
		for _, c := range m.Contents {
			if c == nil {
				continue
			}
			if text, ok := c.Data.(string); ok && text != "" {
				sb.WriteString(role)
				sb.WriteString(": ")
				sb.WriteString(text)
				sb.WriteString("\n")
			}
		}
	}
	return sb.String()
}

// getLatestNGroups returns the last n role-groups from the conversation.
// A "group" is a contiguous run of messages with the same role.
func getLatestNGroups(msgs []threadMessage, n int) []threadMessage {
	if len(msgs) == 0 || n <= 0 {
		return nil
	}

	// Walk backwards counting group boundaries.
	groups := 0
	prevRole := thread.Role("")
	startIdx := 0
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role != prevRole {
			groups++
			prevRole = msgs[i].Role
		}
		if groups > n {
			startIdx = i + 1
			break
		}
	}
	return msgs[startIdx:]
}

// errNotFound is a sentinel error for "not found" lookups.
var errNotFound = errors.New("not found")

// APIExecutor executes HTTP API calls for api_call operations.
type APIExecutor interface {
	ExecuteAPICall(ctx context.Context, step skill.Step, inputs map[string]interface{}, headers map[string]string) (statusCode int, responseBody string, responseHeaders map[string]string, err error)
}

// MCPExecutor extends MCPProvider with operation-level execution.
type MCPExecutor interface {
	ExecuteMCPOperation(ctx context.Context, mcpConfig *skill.MCP, inputs map[string]interface{}) (interface{}, error)
}

// SelfFixerInterface handles automatic fix attempts for failed operations.
type SelfFixerInterface interface {
	IsEnabled() bool
	AttemptFix(ctx context.Context, execCtx selfFixExecutionContext) (interface{}, error)
}

// SelfFixEnvironmentInfo contains environment details for self-fix operations.
type SelfFixEnvironmentInfo struct {
	OS             string   `json:"os"`
	DBType         string   `json:"db_type"`
	AvailableTools []string `json:"available_tools"`
}

// selfFixOperationType identifies the kind of operation being fixed.
type selfFixOperationType string

const (
	selfFixOpTerminal selfFixOperationType = "terminal"
	selfFixOpDB       selfFixOperationType = "db"
)

// selfFixExecutionContext holds all context needed for a self-fix attempt.
type selfFixExecutionContext struct {
	OperationType   selfFixOperationType   `json:"operation_type"`
	ToolFilePath    string                 `json:"tool_file_path"`
	FunctionName    string                 `json:"function_name"`
	StepName        string                 `json:"step_name"`
	Script          string                 `json:"script"`
	ErrorMessage    string                 `json:"error_message"`
	Output          string                 `json:"output"`
	Inputs          map[string]interface{} `json:"inputs"`
	Environment     SelfFixEnvironmentInfo `json:"environment"`
	MessageID       string                 `json:"message_id"`
	ToolsDBPath     string                 `json:"tools_db_path"`
	ValidateYAMLBin string                 `json:"validate_yaml_bin"`
}

// selfFixResult represents the result of a self-fix attempt.
// Used via type assertion from the interface{} returned by SelfFixerInterface.AttemptFix.
type selfFixResult struct {
	Success           bool   `json:"success"`
	RecommendedAction string `json:"recommended_action"`
	Reason            string `json:"reason"`
	Attempts          int    `json:"attempts"`
	Error             error  `json:"-"`
}

// ErrWorkflowRestartRequired signals that the workflow must be restarted
// after a successful self-fix (the YAML has been modified on disk).
var ErrWorkflowRestartRequired = errors.New("workflow restart required after self-fix")

// DatabaseManager provides access to SQL databases for db operations.
type DatabaseManager interface {
	GetDB() (*sql.DB, error)
	GetToolDB(toolName string) (*sql.DB, error)
}

// Package-level database manager (set by host application at init time)
var databaseManager DatabaseManager

// SetDatabaseManager sets the database manager for db operations.
func SetDatabaseManager(dm DatabaseManager) {
	databaseManager = dm
}

// getDB returns the system database connection.
func getDB() (*sql.DB, error) {
	if databaseManager == nil {
		return nil, fmt.Errorf("database manager not configured — call engine.SetDatabaseManager()")
	}
	return databaseManager.GetDB()
}

// getToolDBManager returns the database manager.
func getToolDBManager() (DatabaseManager, error) {
	if databaseManager == nil {
		return nil, fmt.Errorf("database manager not configured — call engine.SetDatabaseManager()")
	}
	return databaseManager, nil
}

// selfFixEnvironmentInfo is the unexported alias used internally.
type selfFixEnvironmentInfo = SelfFixEnvironmentInfo

// getSelfFixEnvironmentInfo returns default environment info.
func getSelfFixEnvironmentInfo() SelfFixEnvironmentInfo {
	return SelfFixEnvironmentInfo{
		OS:             runtime.GOOS,
		DBType:         "sqlite",
		AvailableTools: []string{"bash", "sh", "jq", "sqlite3"},
	}
}

// ─── Cookie / Session persistence ──────────────────────────────────────────

// dbCookie represents a single HTTP cookie for persistence.
type dbCookie struct {
	Name     string    `json:"name"`
	Value    string    `json:"value"`
	Domain   string    `json:"domain"`
	Path     string    `json:"path"`
	Expires  time.Time `json:"expires,omitempty"`
	Secure   bool      `json:"secure"`
	HTTPOnly bool      `json:"httpOnly"`
	SameSite string    `json:"sameSite,omitempty"`
}

// cookieRepository persists HTTP cookies between tool executions.
type cookieRepository interface {
	SaveCookies(toolName, functionName string, cookies []*dbCookie) error
	GetCookies(toolName, functionName string) ([]*dbCookie, error)
	DeleteCookies(toolName, functionName string) error
}

// ─── User confirmation state ───────────────────────────────────────────────

// confirmationState tracks whether a user has confirmed an action.
type confirmationState struct {
	ID               int64      `json:"id"`
	ClientID         string     `json:"client_id"`
	ToolName         string     `json:"tool_name"`
	FunctionName     string     `json:"function_name"`
	InputsHash       string     `json:"inputs_hash"`
	ConfirmationText string     `json:"confirmation_text"`
	PauseReason      string     `json:"pause_reason"`
	IsConfirmed      bool       `json:"is_confirmed"`
	MessageID        string     `json:"message_id"`
	TTLMinutes       int        `json:"ttl_minutes"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
	ConfirmedAt      *time.Time `json:"confirmed_at,omitempty"`
}

// ─── Checkpoint context ────────────────────────────────────────────────────

// contextSnapshot captures context information for workflow checkpoints.
type contextSnapshot struct {
	ConversationType    string                 `json:"conversation_type"`
	StepKey             string                 `json:"step_key"`
	MessageData         Message                `json:"message_data"`
	ConversationHistory []threadMessage        `json:"conversation_history"`
	AdditionalContext   map[string]interface{} `json:"additional_context"`
}

// executeWorkflowParams holds workflow execution parameters for checkpoint resume.
type executeWorkflowParams struct {
	UserMessage       string                 `json:"user_message"`
	ContextForAgent   string                 `json:"context_for_agent"`
	ToolsAndFunctions map[string][]string    `json:"tools_and_functions"`
	ContextKeys       map[string]interface{} `json:"context_keys"`
}

// ─── Tool repository (confirmation state CRUD) ────────────────────────────

// toolRepository provides CRUD for confirmation states and hooks.
type toolRepository interface {
	CreateConfirmationState(ctx context.Context, state *confirmationState) (int64, error)
	GetConfirmationState(ctx context.Context, clientID, toolName, functionName, inputsHash string) (*confirmationState, error)
	UpdateConfirmationState(ctx context.Context, state *confirmationState) error
	FindConfirmationStateByFunction(ctx context.Context, clientID, toolName, functionName string) (*confirmationState, error)
	DeleteConfirmationState(ctx context.Context, id int64) error
}

// ─── Data store (audit + settings) ────────────────────────────────────────

// auditRecord represents a single audit log entry.
type auditRecord struct {
	MessageID   string `json:"message_id"`
	Action      string `json:"action"`
	Observation string `json:"observation"`
}

// dataStore provides audit logging and settings persistence.
type dataStore interface {
	CreateAudit(ctx context.Context, record auditRecord) (int64, error)
	GetAutoApprovalSetting(ctx context.Context) (bool, error)
	GetConversationHistory(ctx context.Context, clientID, workflow string) ([]threadMessage, error)
}

// ─── GDrive client interface ──────────────────────────────────────────────

// GDriveClient abstracts Google Drive operations so the host app can provide
// its own implementation without importing the google.golang.org/api dependency.
type GDriveClient interface {
	List(ctx context.Context, params map[string]interface{}) (interface{}, error)
	Upload(ctx context.Context, params map[string]interface{}) (interface{}, error)
	Download(ctx context.Context, params map[string]interface{}) ([]byte, *skill.FileResult, error)
	CreateFolder(ctx context.Context, params map[string]interface{}) (interface{}, error)
	Delete(ctx context.Context, params map[string]interface{}) (interface{}, error)
	Move(ctx context.Context, params map[string]interface{}) (interface{}, error)
	Search(ctx context.Context, params map[string]interface{}) (interface{}, error)
	GetMetadata(ctx context.Context, params map[string]interface{}) (interface{}, error)
	Update(ctx context.Context, params map[string]interface{}) (interface{}, error)
	Export(ctx context.Context, params map[string]interface{}) ([]byte, *skill.FileResult, error)
}

// GDriveClientFactory creates a GDriveClient. Host applications register their
// factory via SetGDriveClientFactory so the engine can create clients on demand.
type GDriveClientFactory func(ctx context.Context) (GDriveClient, error)

var gdriveClientFactory GDriveClientFactory

// SetGDriveClientFactory registers the factory used to create GDrive clients.
func SetGDriveClientFactory(f GDriveClientFactory) {
	gdriveClientFactory = f
}

// createGDriveClient creates a GDrive client via the registered factory.
func (t *YAMLDefinedTool) createGDriveClient(ctx context.Context) (GDriveClient, error) {
	if gdriveClientFactory == nil {
		return nil, fmt.Errorf("GDrive client factory not configured — call engine.SetGDriveClientFactory()")
	}
	return gdriveClientFactory(ctx)
}

// ─── Auth token repository ─────────────────────────────────────────────────

// authTokenRepository persists auth tokens between tool executions.
type authTokenRepository interface {
	GetToken(toolName, functionName string) (string, error)
	SaveToken(toolName, functionName, token string, ttl time.Duration) error
	DeleteToken(toolName, functionName string) error
}

// newCookieRepository returns a cookieRepository backed by the system DB.
// Returns nil if no database manager is configured.
func newCookieRepository() (cookieRepository, error) {
	// Stub — will be provided by host application via a factory/setter.
	return nil, nil
}

// newAuthTokenRepository returns an authTokenRepository backed by the system DB.
// Returns nil if no database manager is configured.
func newAuthTokenRepository() (authTokenRepository, error) {
	// Stub — will be provided by host application via a factory/setter.
	return nil, nil
}

// ─── MCP config types ──────────────────────────────────────────────────────

// mcpClientType identifies the MCP transport type.
type mcpClientType string

const (
	mcpClientTypeStdio mcpClientType = "stdio"
	mcpClientTypeSSE   mcpClientType = "sse"
)

// mcpClientConfig holds the base MCP client configuration.
type mcpClientConfig struct {
	Type            mcpClientType `json:"type"`
	ClientName      string        `json:"client_name"`
	ClientVersion   string        `json:"client_version"`
	ProtocolVersion string        `json:"protocol_version"`
}

// mcpStdioConfig holds stdio transport configuration.
type mcpStdioConfig struct {
	ClientConfig mcpClientConfig `json:"client_config"`
	Command      string          `json:"command"`
	Args         []string        `json:"args"`
	Env          []string        `json:"env"`
}

// mcpSSEConfig holds SSE transport configuration.
type mcpSSEConfig struct {
	ClientConfig mcpClientConfig `json:"client_config"`
	URL          string          `json:"url"`
}

// mcpConfigLocal is the local MCP config assembled for the executor.
type mcpConfigLocal struct {
	Protocol string          `json:"protocol"`
	Function string          `json:"function"`
	Stdio    *mcpStdioConfig `json:"stdio,omitempty"`
	SSE      *mcpSSEConfig   `json:"sse,omitempty"`
}

// ─── LLM response helpers ──────────────────────────────────────────────────

// errNoJSONContent is returned when an LLM response contains no valid JSON.
var errNoJSONContent = errors.New("no JSON content in assistant response")

// unmarshalAssistantResponse extracts JSON from an assistant response and unmarshals it.
// If the response contains no valid JSON, returns errNoJSONContent.
func unmarshalAssistantResponse(_ context.Context, response string, _ interface{}, target interface{}) error {
	// Find JSON in response — look for last { ... }
	start := strings.LastIndex(response, "{")
	end := strings.LastIndex(response, "}")
	if start < 0 || end <= start {
		return errNoJSONContent
	}
	jsonStr := response[start : end+1]

	// Use encoding/json from the import block
	decoder := json.NewDecoder(strings.NewReader(jsonStr))
	if err := decoder.Decode(target); err != nil {
		return errNoJSONContent
	}
	return nil
}

// ─── Checkpoint / session helpers ──────────────────────────────────────────

// generateSessionID creates a deterministic session ID from its components.
func generateSessionID(parts ...string) string {
	return strings.Join(parts, "::")
}

// checkpointDB is the checkpoint database instance.
var checkpointDB *sql.DB

// SetCheckpointDB sets the database used for execution checkpoints.
func SetCheckpointDB(db *sql.DB) {
	checkpointDB = db
}

// checkpointRepository provides CRUD for execution checkpoints.
type checkpointRepository interface {
	GetCheckpoint(ctx context.Context, sessionID string) (*executionCheckpointData, error)
	SaveCheckpoint(ctx context.Context, checkpoint *executionCheckpointData) error
	DeleteCheckpoint(ctx context.Context, sessionID string) error
}

// executionCheckpointData is the internal checkpoint data (richer than types.ExecutionCheckpoint).
type executionCheckpointData struct {
	ID                    string                 `json:"id"`
	SessionID             string                 `json:"session_id"`
	MessageID             string                 `json:"message_id"`
	ClientID              string                 `json:"client_id"`
	ToolName              string                 `json:"tool_name"`
	FunctionName          string                 `json:"function_name"`
	StepKey               string                 `json:"step_key"`
	Status                string                 `json:"status"`
	ExecuteWorkflowParams interface{}            `json:"execute_workflow_params,omitempty"`
	ContextSnapshot       interface{}            `json:"context_snapshot,omitempty"`
	PauseReason           string                 `json:"pause_reason,omitempty"`
	ResumeCondition       interface{}            `json:"resume_condition,omitempty"`
	ApprovalRequested     bool                   `json:"approval_requested"`
	ApprovalGranted       bool                   `json:"approval_granted"`
	Inputs                map[string]interface{} `json:"inputs,omitempty"`
	CreatedAt             time.Time              `json:"created_at"`
	UpdatedAt             time.Time              `json:"updated_at"`
}

// checkpointStatusPaused is the status string for paused checkpoints.
const checkpointStatusPaused = "paused"

// resumeCondition describes what must happen before a checkpoint can resume.
type resumeCondition struct {
	Type       string                 `json:"type"`
	Parameters map[string]interface{} `json:"parameters,omitempty"`
}

// package-level checkpoint repository (nil until set by host).
var checkpointRepo checkpointRepository

// SetCheckpointRepository sets the checkpoint repository.
func SetCheckpointRepository(repo checkpointRepository) {
	checkpointRepo = repo
}

// getCheckpointDB returns the checkpoint repository, or an error if not configured.
func getCheckpointDB() (checkpointRepository, error) {
	if checkpointRepo == nil {
		return nil, fmt.Errorf("checkpoint repository not configured — call engine.SetCheckpointRepository()")
	}
	return checkpointRepo, nil
}

// i18nStub returns the detected language from context.
// TODO: Wire to a real i18n provider.
func i18nStub(ctx context.Context) string {
	_ = ctx
	return "en"
}

// ─── PDF generation stub ───────────────────────────────────────────────────

// pdfGenerator generates PDF documents from tool definitions.
type pdfGenerator struct {
	variableReplacer interface{}
}

// pdfPackage is a namespace for PDF-related constructors.
var pdf = struct {
	NewPDFGenerator func(vr interface{}) *pdfGenerator
}{
	NewPDFGenerator: func(vr interface{}) *pdfGenerator {
		return &pdfGenerator{variableReplacer: vr}
	},
}

// GeneratePDF generates a PDF from the given configuration and inputs.
// TODO: Wire to actual PDF provider when available.
func (g *pdfGenerator) GeneratePDF(ctx context.Context, config interface{}, inputs map[string]interface{}) ([]byte, *skill.FileResult, error) {
	return nil, nil, fmt.Errorf("PDF generation not yet configured")
}

// ─── Company / Agent proxy stubs ───────────────────────────────────────────

// companyInstance is a minimal company info struct.
type companyInstance struct {
	ID string
}

// getCompanyInstance returns the current company info.
// TODO: Wire to host application's company provider.
func getCompanyInstance() *companyInstance {
	return &companyInstance{ID: "default"}
}

// agentProxyClient handles uploads to the agent proxy service.
type agentProxyClient struct{}

// newAgentProxyClient creates a new agent proxy client.
func newAgentProxyClient() *agentProxyClient {
	return &agentProxyClient{}
}

// UploadPDFFile uploads a PDF file to the agent proxy.
func (c *agentProxyClient) UploadPDFFile(ctx context.Context, data []byte, fileName, companyID string) (string, error) {
	return "", fmt.Errorf("agent proxy client not configured")
}

// UploadFile uploads a file to the agent proxy.
func (c *agentProxyClient) UploadFile(ctx context.Context, data []byte, fileName, mimeType, companyID string) (string, error) {
	return "", fmt.Errorf("agent proxy client not configured")
}

// ─── API request details ───────────────────────────────────────────────────

// apiRequestDetails holds response metadata from API calls.
type apiRequestDetails struct {
	RawResponseBytes    []byte `json:"raw_response_bytes,omitempty"`
	ResponseContentType string `json:"response_content_type,omitempty"`
}
