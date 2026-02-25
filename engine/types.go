// Package engine re-exports core domain types from the types package.
// This file exists so that existing code in the engine package can continue
// to reference these types without a package prefix (e.g. Logger, Message).
// The canonical definitions live in github.com/bbiangul/mantis-skill/types.
package engine

import "github.com/bbiangul/mantis-skill/types"

// -----------------------------------------------------------------------
// Type aliases — these make engine.X identical to types.X everywhere.
// -----------------------------------------------------------------------

// Core domain types
type Message = types.Message
type User = types.User
type MediaType = types.MediaType
type HumanSupportType = types.HumanSupportType
type HumanSupportMessage = types.HumanSupportMessage
type App = types.App

// Constants (re-exported via variables since const aliases aren't supported in Go)
var (
	RequiresHumanAnswer   = types.RequiresHumanAnswer
	RequiresHumanAction   = types.RequiresHumanAction
	RequiresHumanApproval = types.RequiresHumanApproval
)

// Workflow & checkpoint types
type WorkflowType = types.WorkflowType
type Workflow = types.Workflow
type WorkflowStep = types.WorkflowStep
type WorkflowCompletionType = types.WorkflowCompletionType
type StepExecutionSummary = types.StepExecutionSummary
type WorkflowSummary = types.WorkflowSummary
type CheckpointStatus = types.CheckpointStatus
type ExecutionCheckpoint = types.ExecutionCheckpoint

// Constants (re-exported via variables)
var (
	WorkflowTypeUser = types.WorkflowTypeUser
	WorkflowTypeTeam = types.WorkflowTypeTeam
)

// Provider interfaces
type Logger = types.Logger
type DatabaseProvider = types.DatabaseProvider
type HTTPClient = types.HTTPClient
type BrowserProvider = types.BrowserProvider
type BrowserStep = types.BrowserStep
type BrowserResult = types.BrowserResult
type TerminalProvider = types.TerminalProvider
type TerminalResult = types.TerminalResult
type MCPProvider = types.MCPProvider
type LLMProvider = types.LLMProvider
type LLMOptions = types.LLMOptions
type LLMTool = types.LLMTool
type AuthProvider = types.AuthProvider
type FileProvider = types.FileProvider
type PDFProvider = types.PDFProvider
type GDriveProvider = types.GDriveProvider
type GDriveFile = types.GDriveFile
type CodeExecutor = types.CodeExecutor
type CodeExecutorOptions = types.CodeExecutorOptions
type CodeExecutorResult = types.CodeExecutorResult

// Data access types
type UserRepository = types.UserRepository
type ToolRepository = types.ToolRepository
type FunctionExecutionRecord = types.FunctionExecutionRecord
type CacheProvider = types.CacheProvider
type WorkflowRepository = types.WorkflowRepository
type CheckpointRepository = types.CheckpointRepository
type MessageSender = types.MessageSender

// Configuration
type Config = types.Config
