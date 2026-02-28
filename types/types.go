// Package types provides core domain types shared between engine and engine/models.
// This package exists to break the import cycle: engine -> engine/models -> engine.
// Both engine and engine/models import types instead of each other.
package types

import (
	"context"
	"time"
)

// -----------------------------------------------------------------------
// Core domain types — lightweight value objects that the engine passes around.
// These replace connect-ai/internal/models and connect-ai/pkg/domain/models.
// -----------------------------------------------------------------------

// Message represents an incoming message from a user or system.
type Message struct {
	Id              string     `json:"id"`
	ClientID        string     `json:"clientID"`
	Timestamp       int        `json:"timestamp"`
	Body            string     `json:"body"`
	From            string     `json:"from"`
	FromMe          bool       `json:"fromMe"`
	HasMedia        bool       `json:"hasMedia"`
	NotifyName      string     `json:"notifyName"`
	Channel         string     `json:"channel"`
	Session         string     `json:"session"`
	ShouldReply     bool       `json:"shouldReply"`
	IsSynthetic     bool       `json:"isSynthetic,omitempty"`
	ContextForAgent string     `json:"contextForAgent,omitempty"`
	Media           *MediaType `json:"media,omitempty"`
	Language        string     `json:"language,omitempty"`
	OriginsIA       bool       `json:"originsIA,omitempty"`
	WorkflowType    string     `json:"workflowType,omitempty"`
}

// User represents a user/contact in the system.
type User struct {
	ID                         string    `json:"id"`
	FirstName                  string    `json:"firstname"`
	LastName                   string    `json:"lastname"`
	Email                      string    `json:"email"`
	PhoneNumber                string    `json:"phone_number"`
	Telephone                  string    `json:"telephone"`
	CompanyID                  string    `json:"company_id"`
	Language                   string    `json:"language"`
	CompanyName                string    `json:"company_name"`
	Role                       string    `json:"role"`
	Gender                     string    `json:"gender"`
	Address                    string    `json:"address"`
	BirthdayDate               time.Time `json:"birthday_date"`
	QuantityOfMessagesReceived int       `json:"quantity_of_messages_received"`
	Interest                   string    `json:"interest"`
	UserName                   string    `json:"user_name"`
	CreatedAt                  time.Time `json:"created_at"`
	UpdatedAt                  time.Time `json:"updated_at"`
	SessionExpiration          time.Time `json:"session_expiration"`
}

// MediaType represents a file/media attachment.
type MediaType struct {
	Url        string `json:"url"`
	Base64Data string `json:"base64Data,omitempty"`
	Mimetype   string `json:"mimetype"`
	Filename   string `json:"filename,omitempty"`
	FileSize   uint64 `json:"filesize,omitempty"`
}

// HumanSupportType defines the type of human intervention required.
type HumanSupportType string

const (
	RequiresHumanAnswer   HumanSupportType = "requiresHumanAnswer"
	RequiresHumanAction   HumanSupportType = "requiresHumanAction"
	RequiresHumanApproval HumanSupportType = "requiresHumanApproval"
)

// HumanSupportMessage contains a Q&A exchange with a human.
type HumanSupportMessage struct {
	Question string     `json:"question"`
	Answer   string     `json:"answer"`
	Media    *MediaType `json:"media,omitempty"`
}

// App represents a tool application definition (metadata).
type App struct {
	ID            string    `json:"id"`
	Code          string    `json:"code"`
	Name          string    `json:"name"`
	InternalName  string    `json:"internal_name"`
	LatestVersion string    `json:"latest_version"`
	Description   string    `json:"description"`
	IconURL       string    `json:"icon_url"`
	Category      string    `json:"category"`
	Status        string    `json:"status"`
	YamlSchema    string    `json:"yaml_schema,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// -----------------------------------------------------------------------
// Workflow & checkpoint types — replaces connect-ai/pkg/data/database models.
// -----------------------------------------------------------------------

// WorkflowType distinguishes customer vs team workflows.
type WorkflowType string

const (
	WorkflowTypeUser WorkflowType = "user"
	WorkflowTypeTeam WorkflowType = "team"
)

// Workflow represents a predefined sequence of steps.
type Workflow struct {
	ID                        int64          `json:"id"`
	CategoryName              string         `json:"category_name"`
	Description               string         `json:"description"`
	HumanVerified             bool           `json:"human_verified"`
	Version                   int            `json:"version"`
	IsCompleted               bool           `json:"is_completed"`
	HumanReadableCategoryName string         `json:"human_readable_category_name"`
	WorkflowType              WorkflowType   `json:"workflow_type"`
	AvailableTools            string         `json:"available_tools"`
	IsAutoGenerated           bool           `json:"is_auto_generated"`
	AllowWorkflowGeneration   bool           `json:"allow_workflow_generation"`
	Steps                     []WorkflowStep `json:"steps,omitempty"`
	CreatedAt                 time.Time      `json:"created_at"`
	UpdatedAt                 time.Time      `json:"updated_at"`
}

// WorkflowStep represents a single step in a workflow.
type WorkflowStep struct {
	ID                         int64     `json:"id"`
	WorkflowID                 int64     `json:"workflow_id"`
	StepOrder                  int       `json:"step_order"`
	Order                      int       `json:"order"`
	Action                     string    `json:"action"`
	HumanReadableActionName    string    `json:"human_readable_action_name"`
	Rationale                  string    `json:"rationale"`
	ExpectedOutcomeDescription string    `json:"expected_outcome_description"`
	KnowledgeKey               string    `json:"knowledge_key"`
	ToolName                   string    `json:"tool_name"`
	FunctionName               string    `json:"function_name"`
	Description                string    `json:"description"`
	Condition                  string    `json:"condition"`
	CreatedAt                  time.Time `json:"created_at"`
	UpdatedAt                  time.Time `json:"updated_at"`
}

// WorkflowCompletionType describes how a workflow completed.
type WorkflowCompletionType string

// StepExecutionSummary contains information about a single step execution.
type StepExecutionSummary struct {
	ToolName      string
	FunctionName  string
	Rationale     string
	Result        string
	Status        string
	Output        string
	Error         string
	ExecutionTime time.Duration
}

// WorkflowSummary summarizes a completed workflow execution.
type WorkflowSummary struct {
	UserMessage    string
	ExecutedSteps  []StepExecutionSummary
	FinalResult    string
	ExecutionTime  time.Duration
	CompletionType WorkflowCompletionType
}

// CheckpointStatus represents the status of an execution checkpoint.
type CheckpointStatus string

// ExecutionCheckpoint stores the state of a paused workflow execution.
type ExecutionCheckpoint struct {
	ID           string           `json:"id"`
	SessionID    string           `json:"session_id"`
	MessageID    string           `json:"message_id"`
	ClientID     string           `json:"client_id"`
	ToolName     string           `json:"tool_name"`
	FunctionName string           `json:"function_name"`
	StepKey      string           `json:"step_key"`
	Status       CheckpointStatus `json:"status"`
	StateJSON    string           `json:"state_json"`
	CreatedAt    time.Time        `json:"created_at"`
	UpdatedAt    time.Time        `json:"updated_at"`
}

// -----------------------------------------------------------------------
// Provider interfaces — these are what host applications implement.
// -----------------------------------------------------------------------

// Logger provides structured logging.
type Logger interface {
	Debugf(format string, args ...interface{})
	Infof(format string, args ...interface{})
	Warnf(format string, args ...interface{})
	Errorf(format string, args ...interface{})
	WithError(err error) Logger
	WithFields(fields map[string]interface{}) Logger
}

// DatabaseProvider handles SQL execution for db operations.
type DatabaseProvider interface {
	Execute(ctx context.Context, dbName, query string, params ...interface{}) ([]map[string]interface{}, error)
	ExecuteRaw(ctx context.Context, dbName, query string) ([]map[string]interface{}, error)
}

// HTTPClient executes HTTP requests for api_call operations.
type HTTPClient interface {
	Do(ctx context.Context, method, url string, headers map[string]string, body interface{}) (statusCode int, responseBody []byte, responseHeaders map[string]string, err error)
}

// BrowserProvider handles web_browse operations.
type BrowserProvider interface {
	Navigate(ctx context.Context, url string, steps []BrowserStep) (*BrowserResult, error)
}

// BrowserStep represents a single browser automation action.
type BrowserStep struct {
	Action   string
	Selector string
	Value    string
	Wait     int
}

// BrowserResult contains the output of a browser operation.
type BrowserResult struct {
	Content    string
	Screenshot []byte
	URL        string
	StatusCode int
}

// TerminalProvider executes shell commands for terminal operations.
type TerminalProvider interface {
	Execute(ctx context.Context, script string, env map[string]string, workDir string, timeoutMs int) (*TerminalResult, error)
}

// TerminalResult contains the output of a terminal operation.
type TerminalResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// MCPProvider executes MCP (Model Context Protocol) tool operations.
type MCPProvider interface {
	ExecuteTool(ctx context.Context, serverName, toolName string, params map[string]interface{}) (interface{}, error)
}

// LLMProvider handles AI inference operations.
type LLMProvider interface {
	Generate(ctx context.Context, systemPrompt, userPrompt string, opts LLMOptions) (string, error)
	GenerateWithThread(ctx context.Context, thread interface{}, opts LLMOptions) (string, error)
}

// LLMOptions configures LLM generation parameters.
type LLMOptions struct {
	Model       string
	Temperature float64
	MaxTokens   int
	Tools       []LLMTool
}

// LLMTool describes a tool the LLM can call during generation.
type LLMTool struct {
	Name        string
	Description string
	Call        func(ctx context.Context, input string) (string, error)
}

// AuthProvider supplies authentication headers for API calls.
type AuthProvider interface {
	GetHeaders(ctx context.Context) (map[string]string, error)
	IsAuthenticated(ctx context.Context) bool
}

// FileProvider manages temporary file storage.
type FileProvider interface {
	SaveTemp(companyID, filename string, data []byte, retentionSeconds int) (string, error)
	ReadTemp(path string) ([]byte, error)
	ReadTempBase64(path string) (string, error)
	IsFileTracked(path string) bool
}

// PDFProvider generates PDF documents.
type PDFProvider interface {
	Generate(ctx context.Context, config interface{}) ([]byte, error)
}

// GDriveProvider handles Google Drive operations.
type GDriveProvider interface {
	Upload(ctx context.Context, filename string, data []byte, mimeType string) (string, error)
	Download(ctx context.Context, fileID string) ([]byte, error)
	List(ctx context.Context, query string) ([]GDriveFile, error)
}

// GDriveFile represents a file in Google Drive.
type GDriveFile struct {
	ID       string
	Name     string
	MimeType string
	Size     int64
}

// CodeExecutor runs code operations (Claude Code / agent SDK).
type CodeExecutor interface {
	Execute(ctx context.Context, prompt string, opts CodeExecutorOptions) (*CodeExecutorResult, error)
}

// CodeExecutorOptions configures code execution.
type CodeExecutorOptions struct {
	Model        string
	AllowedTools []string
	IsPlanMode   bool
	WorkDir      string
	MaxTurns     int
}

// CodeExecutorResult contains the output of a code execution.
type CodeExecutorResult struct {
	Output  string
	Success bool
}

// UserRepository provides user data access.
type UserRepository interface {
	GetUser(ctx context.Context, userID string) (*User, error)
	UserExists(ctx context.Context, userID string) (bool, error)
	CreateUser(ctx context.Context, user User) error
	GetUserLastSession(ctx context.Context, userID string) (string, error)
}

// ToolRepository provides function execution data access.
type ToolRepository interface {
	RecordFunctionExecution(ctx context.Context, record FunctionExecutionRecord) error
	GetFunctionExecutions(ctx context.Context, messageID string) ([]FunctionExecutionRecord, error)
	GetFunctionExecutionByName(ctx context.Context, messageID, functionName string) (*FunctionExecutionRecord, error)
}

// FunctionExecutionRecord stores the result of executing a function.
type FunctionExecutionRecord struct {
	ID                  string    `json:"id"`
	MessageID           string    `json:"message_id"`
	ClientID            string    `json:"client_id"`
	ToolName            string    `json:"tool_name"`
	FunctionName        string    `json:"function_name"`
	FunctionDescription string    `json:"function_description"`
	Inputs              string    `json:"inputs"`
	InputsHash          string    `json:"inputs_hash"`
	Output              string    `json:"output"`
	OriginalOutput      *string   `json:"original_output"`
	Status              string    `json:"status"`
	StartTime           time.Time `json:"start_time"`
	EndTime             time.Time `json:"end_time"`
}

// CacheProvider handles function result caching.
type CacheProvider interface {
	Get(ctx context.Context, key string) (string, bool, error)
	Set(ctx context.Context, key string, value string, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
}

// WorkflowRepository provides workflow persistence.
type WorkflowRepository interface {
	CreateWorkflow(ctx context.Context, workflow *Workflow) error
	GetWorkflowByCategory(ctx context.Context, categoryName string) (*Workflow, error)
	GetWorkflowByCategoryAndType(ctx context.Context, categoryName string, workflowType WorkflowType) (*Workflow, error)
	GetAllCategories(ctx context.Context) ([]Workflow, error)
	GetCategoriesByType(ctx context.Context, workflowType WorkflowType) ([]Workflow, error)
	UpdateWorkflow(ctx context.Context, workflow *Workflow) error
	GetWorkflowSteps(ctx context.Context, workflowID int64) ([]WorkflowStep, error)
	AddStep(ctx context.Context, workflowID int64, step *WorkflowStep) error
	UpdateStep(ctx context.Context, step *WorkflowStep) error
	DeleteStep(ctx context.Context, stepID int64) error
}

// CheckpointRepository provides execution checkpoint persistence.
type CheckpointRepository interface {
	SaveCheckpoint(ctx context.Context, checkpoint *ExecutionCheckpoint) error
	GetCheckpoint(ctx context.Context, sessionID string) (*ExecutionCheckpoint, error)
	UpdateCheckpointStatus(ctx context.Context, sessionID string, status CheckpointStatus) error
}

// MessageSender sends messages to users or team channels.
type MessageSender interface {
	SendToUser(ctx context.Context, message string) error
	SendToTeam(ctx context.Context, message, role string) error
}

// -----------------------------------------------------------------------
// Engine configuration — wires all providers together.
// -----------------------------------------------------------------------

// Config holds all provider implementations needed by the engine.
type Config struct {
	// Required providers
	LLM      LLMProvider
	Database DatabaseProvider
	Logger   Logger

	// Optional providers — operations that need them will fail gracefully if nil
	HTTP     HTTPClient
	Browser  BrowserProvider
	Terminal TerminalProvider
	MCP      MCPProvider
	Auth     AuthProvider
	Files    FileProvider
	PDF      PDFProvider
	GDrive   GDriveProvider
	Code     CodeExecutor
	Message  MessageSender

	// Data access
	UserRepo       UserRepository
	ToolRepo       ToolRepository
	Cache          CacheProvider
	WorkflowRepo   WorkflowRepository
	CheckpointRepo CheckpointRepository
}
