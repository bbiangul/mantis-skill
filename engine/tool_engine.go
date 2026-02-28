package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/bbiangul/mantis-skill/engine/files"
	tool_engine_models "github.com/bbiangul/mantis-skill/engine/models"
	tool_engine_utils "github.com/bbiangul/mantis-skill/engine/utils"
	tool_protocol "github.com/bbiangul/mantis-skill/skill"
	"github.com/bbiangul/mantis-skill/types"
)

const (
	// LockTimeout is the maximum time to wait for acquiring a lock
	LockTimeout = 30 * time.Second
	// RLockTimeout is the maximum time to wait for acquiring a read lock
	RLockTimeout = 10 * time.Second
)

// ErrLockTimeout is returned when lock acquisition times out
var ErrLockTimeout = errors.New("lock acquisition timeout")

// ToolEngine manages tool definitions and their execution
type ToolEngine struct {
	repository                      types.ToolRepository
	toolDefinitions                 map[string]tool_protocol.Tool // Map of tool name+version to definition
	langChainTools                  map[string]tool_engine_models.LangChainTool
	langChainToolsByTypeDescription map[string]string
	toolsDir                        string
	mutex                           sync.RWMutex
	executionTracker                tool_engine_models.IFunctionExecutionTracker
	variableReplacer                tool_engine_models.IVariableReplacer
	authProvider                    types.AuthProvider
	lastToolsChecksum               string
	workflowRepo                    types.WorkflowRepository
	knowledgeQuery                  tool_engine_models.LangChainTool
	askHumanCallback                func(ctx context.Context, query string) (string, error)
	workflowInitiator               tool_engine_models.IWorkflowInitiator
	selfFixer                       SelfFixerInterface
	toolFilePaths                   map[string]string // Map of tool key -> file path
	toolFunctions                   tool_engine_models.ToolFunctions
	tempFileManager                 tool_engine_models.ITempFileManager
}

// tryRLockWithTimeout attempts to acquire a read lock with timeout.
// Returns true if lock was acquired, false if timeout occurred.
// IMPORTANT: Caller MUST call RUnlock() if this returns true.
//
// Uses TryRLock polling instead of spawning a goroutine to avoid leaks
// when the timeout or context cancellation fires before the lock is acquired.
func (e *ToolEngine) tryRLockWithTimeout(ctx context.Context, timeout time.Duration, caller string) bool {
	start := time.Now()
	logger.Debugf("[ToolEngine] %s: attempting RLock with timeout %v", caller, timeout)

	deadline := time.After(timeout)
	ticker := time.NewTicker(1 * time.Millisecond)
	defer ticker.Stop()

	for {
		if e.mutex.TryRLock() {
			logger.Debugf("[ToolEngine] %s: acquired RLock (waited %v)", caller, time.Since(start))
			return true
		}

		select {
		case <-deadline:
			logger.Errorf("[ToolEngine] %s: RLock timeout after %v - possible deadlock detected", caller, timeout)
			return false
		case <-ctx.Done():
			logger.Warnf("[ToolEngine] %s: context cancelled while waiting for RLock", caller)
			return false
		case <-ticker.C:
			// retry TryRLock on next tick
		}
	}
}

// tryLockWithTimeout attempts to acquire a write lock with timeout.
// Returns true if lock was acquired, false if timeout occurred.
// IMPORTANT: Caller MUST call Unlock() if this returns true.
//
// Uses TryLock polling instead of spawning a goroutine to avoid leaks
// when the timeout or context cancellation fires before the lock is acquired.
func (e *ToolEngine) tryLockWithTimeout(ctx context.Context, timeout time.Duration, caller string) bool {
	start := time.Now()
	logger.Debugf("[ToolEngine] %s: attempting Lock (WRITE) with timeout %v", caller, timeout)

	deadline := time.After(timeout)
	ticker := time.NewTicker(1 * time.Millisecond)
	defer ticker.Stop()

	for {
		if e.mutex.TryLock() {
			logger.Debugf("[ToolEngine] %s: acquired Lock (waited %v)", caller, time.Since(start))
			return true
		}

		select {
		case <-deadline:
			logger.Errorf("[ToolEngine] %s: Lock timeout after %v - possible deadlock detected", caller, timeout)
			return false
		case <-ctx.Done():
			logger.Warnf("[ToolEngine] %s: context cancelled while waiting for Lock", caller)
			return false
		case <-ticker.C:
			// retry TryLock on next tick
		}
	}
}

// NewToolEngine creates a new ToolEngine
func NewToolEngine(
	repository types.ToolRepository,
	executionTracker tool_engine_models.IFunctionExecutionTracker,
	toolsDir string,
	variableReplacer tool_engine_models.IVariableReplacer,
	authProvider types.AuthProvider,
	workflowRepo types.WorkflowRepository,
	workflowInitiator tool_engine_models.IWorkflowInitiator,
) (tool_engine_models.IToolEngine, error) {
	if repository == nil {
		return nil, errors.New("repository cannot be nil")
	}
	if executionTracker == nil {
		return nil, errors.New("executionTracker cannot be nil")
	}
	if variableReplacer == nil {
		return nil, errors.New("variableReplacer cannot be nil")
	}

	// Initialize temp file manager using a data directory
	dataDir := os.Getenv("MANTIS_DATA_DIR")
	if dataDir == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory for temp files: %w", err)
		}
		dataDir = filepath.Join(homeDir, ".mantis")
	}
	tempFileManager := files.NewTempFileManager(dataDir)
	// Start cleanup goroutine (every 60 seconds)
	tempFileManager.StartCleanup(60 * time.Second)

	// Set temp file manager on variable replacer if it supports the setter
	type tempFileManagerSetter interface {
		SetTempFileManager(manager interface {
			GetTempFileBase64(path string) (string, error)
		})
	}
	if vr, ok := variableReplacer.(tempFileManagerSetter); ok {
		vr.SetTempFileManager(tempFileManager)
	}

	return &ToolEngine{
		repository:                      repository,
		toolDefinitions:                 make(map[string]tool_protocol.Tool),
		langChainTools:                  make(map[string]tool_engine_models.LangChainTool),
		langChainToolsByTypeDescription: make(map[string]string),
		toolsDir:                        toolsDir,
		executionTracker:                executionTracker,
		variableReplacer:                variableReplacer,
		authProvider:                    authProvider,
		workflowRepo:                    workflowRepo,
		workflowInitiator:               workflowInitiator,
		toolFilePaths:                   make(map[string]string),
		tempFileManager:                 tempFileManager,
	}, nil
}

func (e *ToolEngine) ExecutionTracker() tool_engine_models.IFunctionExecutionTracker {
	return e.executionTracker
}

func (e *ToolEngine) GetVariableReplacer() tool_engine_models.IVariableReplacer {
	return &variableReplacerAdapter{e.variableReplacer}
}

func (e *ToolEngine) GetWorkflowInitiator() tool_engine_models.IWorkflowInitiator {
	return e.workflowInitiator
}

func (e *ToolEngine) GetTempFileManager() tool_engine_models.ITempFileManager {
	return e.tempFileManager
}

func (e *ToolEngine) GetAuthProvider() types.AuthProvider {
	return e.authProvider
}

func (e *ToolEngine) GetSelfFixer() SelfFixerInterface {
	return e.selfFixer
}

func (e *ToolEngine) GetToolFilePath(toolName, toolVersion string) string {
	e.mutex.RLock()
	defer e.mutex.RUnlock()
	key := fmt.Sprintf("%s:%s", toolName, toolVersion)
	return e.toolFilePaths[key]
}

type variableReplacerAdapter struct {
	impl tool_engine_models.IVariableReplacer
}

func (v *variableReplacerAdapter) ReplaceVariables(ctx context.Context, text string, inputs map[string]interface{}) (string, error) {
	return v.impl.ReplaceVariables(ctx, text, inputs)
}

func (v *variableReplacerAdapter) ReplaceVariablesWithContext(ctx context.Context, text string, inputs map[string]interface{}, dbContext bool) (string, error) {
	return v.impl.ReplaceVariablesWithContext(ctx, text, inputs, dbContext)
}

func (v *variableReplacerAdapter) ReplaceVariablesWithOptions(ctx context.Context, text string, inputs map[string]interface{}, opts tool_engine_models.ReplaceOptions) (string, error) {
	return v.impl.ReplaceVariablesWithOptions(ctx, text, inputs, opts)
}

func (v *variableReplacerAdapter) ReplaceVariablesForDB(ctx context.Context, text string, inputs map[string]interface{}) (string, []interface{}, error) {
	return v.impl.ReplaceVariablesForDB(ctx, text, inputs)
}

func (v *variableReplacerAdapter) ReplaceVariablesWithLoop(ctx context.Context, text string, inputs map[string]interface{}, loopContext *tool_engine_models.LoopContext) (string, error) {
	return v.impl.ReplaceVariablesWithLoop(ctx, text, inputs, loopContext)
}

func (v *variableReplacerAdapter) SetEnvironmentVariables(envVars []tool_protocol.EnvVar) {
	v.impl.SetEnvironmentVariables(envVars)
}

func (v *variableReplacerAdapter) GetEnvironmentVariables() map[string]string {
	return v.impl.GetEnvironmentVariables()
}

func (v *variableReplacerAdapter) NavigatePath(data interface{}, path string) (interface{}, error) {
	return v.impl.NavigatePath(data, path)
}

func (v *variableReplacerAdapter) NavigatePathWithTransformation(inputs map[string]interface{}, path string) (interface{}, error) {
	return v.impl.NavigatePathWithTransformation(inputs, path)
}

func (v *variableReplacerAdapter) SetAuthToken(token string) {
	v.impl.SetAuthToken(token)
}

func (v *variableReplacerAdapter) GetAuthToken() string {
	return v.impl.GetAuthToken()
}

func (v *variableReplacerAdapter) ClearAuthToken() {
	v.impl.ClearAuthToken()
}

func (e *ToolEngine) SetAskHumanCallback(callback func(ctx context.Context, query string) (string, error)) {
	e.askHumanCallback = callback
}

// SetToolFunctions sets the tool functions for system function execution
func (e *ToolEngine) SetToolFunctions(tf tool_engine_models.ToolFunctions) {
	e.toolFunctions = tf
}

// GetToolFunctions returns the tool functions for system function execution
func (e *ToolEngine) GetToolFunctions() tool_engine_models.ToolFunctions {
	return e.toolFunctions
}

func (e *ToolEngine) Initialize(ctx context.Context, inputFulfiller tool_engine_models.IInputFulfiller, systemApps []types.App) error {
	if e.toolsDir != "" {
		err := e.loadToolsFromDirectory(ctx, e.toolsDir, systemApps)
		if err != nil {
			return fmt.Errorf("failed to load tools from directory: %w", err)
		}

		checksum, err := e.computeToolsChecksum()
		if err != nil {
			return fmt.Errorf("failed to compute tools checksum: %w", err)
		}
		e.lastToolsChecksum = checksum

		err = e.CreateLangChainTools(ctx, inputFulfiller)
		if err != nil {
			return fmt.Errorf("failed to create langchain tools: %w", err)
		}

		// Initialize function descriptions
		e.InitFunctionsDescription()
	}

	return nil
}

// loadToolsFromDirectory loads tool definitions from YAML files in the specified directory
func (e *ToolEngine) loadToolsFromDirectory(ctx context.Context, dir string, systemApps []types.App) error {
	if logger != nil {
		logger.Debugf("[ToolEngine] loadToolsFromDirectory: starting load from %s with %d system apps", dir, len(systemApps))
	}
	loadStart := time.Now()

	// First, load system apps if any
	if logger != nil {
		logger.Infof("[SYSTEM-APPS] Loading %d system apps from API", len(systemApps))
	}
	for _, app := range systemApps {
		if app.YamlSchema != "" {
			if logger != nil {
				logger.Infof("[SYSTEM-APPS] Processing system app: %s (ID: %s)", app.Name, app.ID)
			}
			customTool, err := tool_protocol.CreateTool(app.YamlSchema, tool_protocol.SystemTool)
			if err != nil {
				if logger != nil {
					logger.Errorf("Failed to parse system app %s: %v", app.Name, err)
				}
				continue
			}

			// Add the environment variables to our variable replacer
			e.variableReplacer.SetEnvironmentVariables(customTool.Env)

			// Store each tool in the engine
			for _, tool := range customTool.Tools {
				key := fmt.Sprintf("%s:%s", tool.Name, tool.Version)

				// Log all function names for debugging
				var funcNames []string
				for _, fn := range tool.Functions {
					funcNames = append(funcNames, fn.Name)
				}
				if logger != nil {
					logger.Infof("[SYSTEM-APPS] Tool '%s' v%s has %d functions: %v (IsSystemApp: %v)",
						tool.Name, tool.Version, len(tool.Functions), funcNames, tool.IsSystemApp)
				}

				e.mutex.Lock()
				e.toolDefinitions[key] = tool
				e.mutex.Unlock()

				if logger != nil {
					logger.Infof("Loaded system app tool '%s' version '%s'", tool.Name, tool.Version)
				}
			}

			// Sync workflows from YAML if any are defined and workflow repo is available
			if e.workflowRepo != nil && len(customTool.Workflows) > 0 {
				toolName := customTool.Tools[0].Name
				if err := e.syncWorkflowsFromYAML(ctx, customTool.Workflows, toolName); err != nil {
					if logger != nil {
						logger.Warnf("Failed to sync workflows from system app %s: %v", app.Name, err)
					}
					// Don't fail the entire load process if workflow sync fails
				}
			}
		}
	}

	// Load shared tools from the external function registry
	if err := e.loadSharedTools(ctx); err != nil {
		if logger != nil {
			logger.Warnf("Failed to load shared tools: %v", err)
		}
		// Continue without shared tools if load fails
	}

	// Then load tools from directory
	dirFiles, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("failed to read directory: %w", err)
	}

	for _, file := range dirFiles {
		if file.IsDir() || !(filepath.Ext(file.Name()) == ".yaml" || filepath.Ext(file.Name()) == ".yml") {
			continue
		}

		filePath := filepath.Join(dir, file.Name())

		content, err := os.ReadFile(filePath)
		if err != nil {
			return fmt.Errorf("failed to read file %s: %w", filePath, err)
		}

		customTool, err := tool_protocol.CreateTool(string(content))
		if err != nil {
			return fmt.Errorf("failed to parse tool from file %s: %w", filePath, err)
		}

		// Add the environment variables to our variable replacer
		e.variableReplacer.SetEnvironmentVariables(customTool.Env)

		// Store each tool in the engine
		for _, tool := range customTool.Tools {
			key := fmt.Sprintf("%s:%s", tool.Name, tool.Version)

			e.mutex.Lock()
			e.toolDefinitions[key] = tool
			e.toolFilePaths[key] = filePath // Track file path for self-fix
			e.mutex.Unlock()

			if logger != nil {
				logger.Debugf("Loaded tool '%s' version '%s' from %s\n", tool.Name, tool.Version, filePath)
			}
		}

		// Sync workflows from YAML if any are defined and workflow repo is available
		if e.workflowRepo != nil && len(customTool.Workflows) > 0 {
			toolName := customTool.Tools[0].Name
			if err := e.syncWorkflowsFromYAML(ctx, customTool.Workflows, toolName); err != nil {
				if logger != nil {
					logger.Warnf("Failed to sync workflows from %s: %v", filePath, err)
				}
				// Don't fail the entire load process if workflow sync fails
			}
		}
	}

	if logger != nil {
		logger.Debugf("[ToolEngine] loadToolsFromDirectory: completed in %v", time.Since(loadStart))
	}
	return nil
}

// SharedToolProvider provides embedded shared tool YAML content.
// Host applications implement this to supply shared tools at runtime.
type SharedToolProvider interface {
	GetSharedToolFiles() ([]string, error)
	GetSharedToolContent(fileName string) ([]byte, error)
}

// Package-level shared tool provider (set by host application at init time)
var sharedToolProvider SharedToolProvider

// SetSharedToolProvider registers the provider used to load shared tools.
func SetSharedToolProvider(p SharedToolProvider) {
	sharedToolProvider = p
}

// loadSharedTools loads tool definitions from the shared tool provider
func (e *ToolEngine) loadSharedTools(ctx context.Context) error {
	if sharedToolProvider == nil {
		// No shared tool provider configured — skip silently
		return nil
	}

	fileNames, err := sharedToolProvider.GetSharedToolFiles()
	if err != nil {
		return fmt.Errorf("failed to get shared tool files: %w", err)
	}

	for _, fileName := range fileNames {
		content, err := sharedToolProvider.GetSharedToolContent(fileName)
		if err != nil {
			if logger != nil {
				logger.Warnf("Failed to read shared tool file %s: %v", fileName, err)
			}
			continue
		}

		customTool, err := tool_protocol.CreateTool(string(content), tool_protocol.SharedTool)
		if err != nil {
			if logger != nil {
				logger.Errorf("Failed to parse shared tool %s: %v", fileName, err)
			}
			continue
		}

		// Add the environment variables to our variable replacer
		e.variableReplacer.SetEnvironmentVariables(customTool.Env)

		// Store each tool in the engine
		for _, tool := range customTool.Tools {
			key := fmt.Sprintf("%s:%s", tool.Name, tool.Version)

			e.mutex.Lock()
			e.toolDefinitions[key] = tool
			e.mutex.Unlock()

			if logger != nil {
				logger.Infof("Loaded shared tool '%s' version '%s'", tool.Name, tool.Version)
			}
		}

		// Sync workflows from YAML if any are defined and workflow repo is available
		if e.workflowRepo != nil && len(customTool.Workflows) > 0 {
			toolName := customTool.Tools[0].Name
			if err := e.syncWorkflowsFromYAML(ctx, customTool.Workflows, toolName); err != nil {
				if logger != nil {
					logger.Warnf("Failed to sync workflows from shared tool %s: %v", fileName, err)
				}
				// Don't fail the entire load process if workflow sync fails
			}
		}
	}

	return nil
}

// CreateLangChainTools converts the YAML tool definitions to LangChainTool instances
func (e *ToolEngine) CreateLangChainTools(ctx context.Context, inputFulfiller tool_engine_models.IInputFulfiller) error {
	start := time.Now()
	if logger != nil {
		logger.Debugf("[ToolEngine] CreateLangChainTools: attempting Lock (WRITE)")
	}
	e.mutex.Lock()
	if logger != nil {
		logger.Debugf("[ToolEngine] CreateLangChainTools: acquired Lock (waited %v)", time.Since(start))
	}
	defer func() {
		e.mutex.Unlock()
		if logger != nil {
			logger.Debugf("[ToolEngine] CreateLangChainTools: released Lock (total %v)", time.Since(start))
		}
	}()

	for _, tool := range e.toolDefinitions {
		for _, function := range tool.Functions {
			// check if the function is already registered in the map
			if _, exists := e.langChainTools[functionIdentification(tool.Name, function.Name, tool.Version)]; exists {
				continue
			}

			langChainTool := NewYAMLDefinedTool(
				inputFulfiller,
				&tool,
				&function,
				e.executionTracker,
				e.variableReplacer,
				e,
				e.workflowInitiator,
			)

			e.langChainTools[functionIdentification(tool.Name, function.Name, tool.Version)] = langChainTool
		}
	}

	return nil
}

// GetToolByName returns a tool definition by name and version
func (e *ToolEngine) GetToolByName(name, version string) (tool_protocol.Tool, bool) {
	ctx := context.Background()
	if !e.tryRLockWithTimeout(ctx, RLockTimeout, fmt.Sprintf("GetToolByName(%s:%s)", name, version)) {
		if logger != nil {
			logger.Errorf("[ToolEngine] GetToolByName: failed to acquire lock")
		}
		return tool_protocol.Tool{}, false
	}
	defer e.mutex.RUnlock()

	key := fmt.Sprintf("%s:%s", name, version)
	tool, exists := e.toolDefinitions[key]
	return tool, exists
}

// GetLangChainFunctionByName returns a LangChainTool by toolName, functionName and version
func (e *ToolEngine) GetLangChainFunctionByName(toolName, name, version string) (tool_engine_models.LangChainTool, bool) {
	ctx := context.Background()
	if !e.tryRLockWithTimeout(ctx, RLockTimeout, fmt.Sprintf("GetLangChainFunctionByName(%s.%s)", toolName, name)) {
		if logger != nil {
			logger.Errorf("[ToolEngine] GetLangChainFunctionByName: failed to acquire lock")
		}
		return nil, false
	}
	defer e.mutex.RUnlock()

	tool, exists := e.langChainTools[functionIdentification(toolName, name, version)]
	return tool, exists
}

// GetAllTools returns all tool definitions
func (e *ToolEngine) GetAllTools() []tool_protocol.Tool {
	ctx := context.Background()
	if !e.tryRLockWithTimeout(ctx, RLockTimeout, "GetAllTools") {
		if logger != nil {
			logger.Errorf("[ToolEngine] GetAllTools: failed to acquire lock")
		}
		return nil
	}
	defer e.mutex.RUnlock()

	var allTools []tool_protocol.Tool
	for _, tool := range e.toolDefinitions {
		allTools = append(allTools, tool)
	}

	return allTools
}

func (e *ToolEngine) InitFunctionsDescription() {
	start := time.Now()
	ctx := context.Background()
	if logger != nil {
		logger.Debugf("[ToolEngine] InitFunctionsDescription: attempting Lock (WRITE)")
	}
	e.mutex.Lock()
	if logger != nil {
		logger.Debugf("[ToolEngine] InitFunctionsDescription: acquired Lock (waited %v)", time.Since(start))
	}
	defer func() {
		e.mutex.Unlock()
		if logger != nil {
			logger.Debugf("[ToolEngine] InitFunctionsDescription: released Lock (total %v)", time.Since(start))
		}
	}()
	_ = ctx

	// Initialize (or clear) the cache.
	e.langChainToolsByTypeDescription = make(map[string]string)

	for _, tool := range e.toolDefinitions {
		// For each tool, create a temporary map to group functions by trigger.
		toolFunctionsByTrigger := make(map[string][]tool_protocol.Function)
		for _, function := range tool.Functions {
			// For each trigger that the function has, add it to the map.
			for _, triggerStr := range function.Triggers {
				toolFunctionsByTrigger[triggerStr.Type] = append(toolFunctionsByTrigger[triggerStr.Type], function)
			}
		}

		// For each trigger in this tool, build the description block.
		for trigger, functions := range toolFunctionsByTrigger {
			block := fmt.Sprintf("Agent: %s_%s\nType: conversational\nDescription: %s\nAvailable Tools:\n",
				tool.Name, tool.Version, tool.Description)
			for _, f := range functions {
				block += fmt.Sprintf("Function: %s\nDescription: %s\n", f.Name, f.Description)
			}

			if existing, ok := e.langChainToolsByTypeDescription[trigger]; ok {
				e.langChainToolsByTypeDescription[trigger] = existing + "\n" + block
			} else {
				e.langChainToolsByTypeDescription[trigger] = block
			}
		}
	}
}

func (e *ToolEngine) GetAllFunctionsDescription(triggerType tool_protocol.Trigger) string {
	ctx := context.Background()

	if !e.tryRLockWithTimeout(ctx, RLockTimeout, fmt.Sprintf("GetAllFunctionsDescription(trigger=%s)", triggerType.Type)) {
		if logger != nil {
			logger.Errorf("[ToolEngine] GetAllFunctionsDescription: failed to acquire lock for trigger=%s, returning empty", triggerType.Type)
		}
		return "[]"
	}
	start := time.Now()
	defer func() {
		e.mutex.RUnlock()
		if logger != nil {
			logger.Debugf("[ToolEngine] GetAllFunctionsDescription: released RLock (total %v)", time.Since(start))
		}
	}()

	if description, ok := e.langChainToolsByTypeDescription[triggerType.Type]; ok {
		return description
	}
	return "[]"
}

// GetFunctionsWithFlexTrigger returns all functions that have a flex or always_on trigger type
func (e *ToolEngine) GetFunctionsWithFlexTrigger(isUserContext bool, channel string) []tool_engine_models.FlexFunction {
	ctx := context.Background()

	if !e.tryRLockWithTimeout(ctx, RLockTimeout, fmt.Sprintf("GetFunctionsWithFlexTrigger(isUserContext=%v, channel=%s)", isUserContext, channel)) {
		if logger != nil {
			logger.Errorf("[ToolEngine] GetFunctionsWithFlexTrigger: failed to acquire lock, returning empty")
		}
		return nil
	}
	start := time.Now()
	defer func() {
		e.mutex.RUnlock()
		if logger != nil {
			logger.Debugf("[ToolEngine] GetFunctionsWithFlexTrigger: released RLock (total %v)", time.Since(start))
		}
	}()

	var flexFunctions []tool_engine_models.FlexFunction
	var allowedTriggers []string
	isEmailChannel := channel == "email"

	if isUserContext {
		if isEmailChannel {
			allowedTriggers = []string{tool_protocol.TriggerFlexForUserEmail, tool_protocol.TriggerOnUserEmail, tool_protocol.TriggerOnCompletedUserEmail}
		} else {
			allowedTriggers = []string{tool_protocol.TriggerFlexForUser, tool_protocol.TriggerOnUserMessage, tool_protocol.TriggerOnCompletedUserMessage}
		}
	} else {
		if isEmailChannel {
			allowedTriggers = []string{tool_protocol.TriggerFlexForTeamEmail, tool_protocol.TriggerOnTeamEmail, tool_protocol.TriggerOnCompletedTeamEmail}
		} else {
			allowedTriggers = []string{tool_protocol.TriggerFlexForTeam, tool_protocol.TriggerOnTeamMessage, tool_protocol.TriggerOnCompletedTeamMessage}
		}
	}

	for _, tool := range e.toolDefinitions {
		for _, function := range tool.Functions {
			found := false
			for _, trigger := range function.Triggers {
				for _, allowed := range allowedTriggers {
					if trigger.Type == allowed {
						flexFunctions = append(flexFunctions, tool_engine_models.FlexFunction{
							Tool:     tool,
							Function: function,
						})
						found = true
						break
					}
				}
				if found {
					break
				}
			}
		}
	}

	return flexFunctions
}

func (e *ToolEngine) GetFlexFunctionsForMap(
	toolFuncMap map[string][]string,
) []tool_engine_models.FlexFunction {
	ctx := context.Background()
	if !e.tryRLockWithTimeout(ctx, RLockTimeout, "GetFlexFunctionsForMap") {
		if logger != nil {
			logger.Errorf("[ToolEngine] GetFlexFunctionsForMap: failed to acquire lock")
		}
		return nil
	}
	defer e.mutex.RUnlock()

	var flexFunctions []tool_engine_models.FlexFunction

	for _, tool := range e.toolDefinitions {
		funcNames, ok := toolFuncMap[tool.Name]
		if !ok {
			continue
		}

		nameSet := make(map[string]struct{}, len(funcNames))
		for _, fn := range funcNames {
			nameSet[fn] = struct{}{}
		}

		for _, function := range tool.Functions {
			if _, wanted := nameSet[function.Name]; !wanted {
				continue
			}
			flexFunctions = append(flexFunctions, tool_engine_models.FlexFunction{
				Tool:     tool,
				Function: function,
			})
		}
	}

	return flexFunctions
}

// UpdateToolEngine updates the tool engine with new tools
func (e *ToolEngine) UpdateToolEngine(ctx context.Context, inputFulfiller tool_engine_models.IInputFulfiller) error {
	if e.toolsDir != "" {
		// For updates, we don't reload system apps - they're only loaded on initialization
		err := e.loadToolsFromDirectory(ctx, e.toolsDir, nil)
		if err != nil {
			return fmt.Errorf("failed to load tools from directory: %w", err)
		}

		newChecksum, err := e.computeToolsChecksum()
		if err != nil {
			return fmt.Errorf("failed to compute tools checksum: %w", err)
		}

		// If no changes, skip the update.
		if newChecksum == e.lastToolsChecksum {
			return nil
		}

		err = e.CreateLangChainTools(ctx, inputFulfiller)
		if err != nil {
			return fmt.Errorf("failed to create langchain tools: %w", err)
		}

		e.InitFunctionsDescription()
	}

	return nil
}

// ForceReloadTools forces a complete reload of all tools from disk.
func (e *ToolEngine) ForceReloadTools(ctx context.Context, inputFulfiller tool_engine_models.IInputFulfiller) error {
	if e.toolsDir == "" {
		return nil
	}

	if logger != nil {
		logger.Infof("[ForceReloadTools] Forcing complete tool reload")
	}

	// Clear existing tool caches to force recreation
	start := time.Now()
	if logger != nil {
		logger.Debugf("[ForceReloadTools] attempting Lock (WRITE) to clear caches")
	}
	e.mutex.Lock()
	if logger != nil {
		logger.Debugf("[ForceReloadTools] acquired Lock (waited %v)", time.Since(start))
	}
	e.langChainTools = make(map[string]tool_engine_models.LangChainTool)
	e.langChainToolsByTypeDescription = make(map[string]string)
	e.mutex.Unlock()
	if logger != nil {
		logger.Debugf("[ForceReloadTools] released Lock (held %v)", time.Since(start))
	}

	// Reload tools from directory (don't reload system apps)
	err := e.loadToolsFromDirectory(ctx, e.toolsDir, nil)
	if err != nil {
		return fmt.Errorf("failed to load tools from directory: %w", err)
	}

	// Update checksum
	newChecksum, err := e.computeToolsChecksum()
	if err != nil {
		return fmt.Errorf("failed to compute tools checksum: %w", err)
	}
	e.lastToolsChecksum = newChecksum

	// Recreate all LangChain tools
	err = e.CreateLangChainTools(ctx, inputFulfiller)
	if err != nil {
		return fmt.Errorf("failed to create langchain tools: %w", err)
	}

	e.InitFunctionsDescription()

	if logger != nil {
		logger.Infof("[ForceReloadTools] Tool reload complete")
	}
	return nil
}

// ExecuteFunctionByNameAndVersion executes a specific function by name and version
func (e *ToolEngine) ExecuteFunctionByNameAndVersion(
	ctx context.Context,
	messageID string,
	toolName string,
	toolVersion string,
	functionName string,
	inputFulfiller tool_engine_models.IInputFulfiller,
) (string, error) {
	// Use timeout-based lock acquisition for the lookup phase only
	if !e.tryRLockWithTimeout(ctx, RLockTimeout, fmt.Sprintf("ExecuteFunctionByNameAndVersion(%s.%s)", toolName, functionName)) {
		return "", fmt.Errorf("failed to acquire lock for tool execution: %w", ErrLockTimeout)
	}

	// Find the tool
	toolKey := fmt.Sprintf("%s:%s", toolName, toolVersion)
	customTool, ok := e.toolDefinitions[toolKey]
	if !ok {
		e.mutex.RUnlock()
		return "", fmt.Errorf("tool '%s:%s' not found", toolName, toolVersion)
	}

	// Find the function
	var foundFunction *tool_protocol.Function
	var foundTool *tool_protocol.Tool

	for _, function := range customTool.Functions {
		if function.Name == functionName {
			foundFunction = &function
			foundTool = &customTool
			break
		}
	}

	if foundFunction == nil || foundTool == nil {
		e.mutex.RUnlock()
		return "", fmt.Errorf("function '%s' not found in tool '%s:%s'", functionName, toolName, toolVersion)
	}

	var (
		yamlTool tool_engine_models.LangChainTool
		exists   bool
	)
	if yamlTool, exists = e.langChainTools[functionIdentification(toolName, functionName, toolVersion)]; !exists {
		yamlTool = NewYAMLDefinedTool(
			inputFulfiller,
			foundTool,
			foundFunction,
			e.executionTracker,
			e.variableReplacer,
			e,
			e.workflowInitiator,
		)
	}

	// Release the lock BEFORE calling the tool - tool execution can take a long time
	e.mutex.RUnlock()
	if logger != nil {
		logger.Debugf("[ToolEngine] ExecuteFunctionByNameAndVersion: released RLock before tool execution")
	}

	// Execute the tool without holding the lock
	res, err := yamlTool.Call(ctx, messageID)

	var callback *ExecutorAgenticWorkflowCallback
	if callback, ok = ctx.Value(CallbackInContextKey).(*ExecutorAgenticWorkflowCallback); !ok {
		cbErr := fmt.Errorf("callback not found in context or invalid type: %T",
			ctx.Value(CallbackInContextKey))
		if logger != nil {
			logger.Errorf("Failed to get workflow callback from context: %v", cbErr)
		}
		return res, err
	}

	var stepKey string

	stepKey, ok = ctx.Value(StepKeyInContextKey).(string)
	if !ok {
		if logger != nil {
			logger.Errorf("Error getting step key from ctx")
		}
	}

	retrievedMsg, ok := ctx.Value(MessageInContextKey).(Message)
	if !ok {
		return res, err
	}

	clientID := retrievedMsg.ClientID

	callback.OnStepExecuted(ctx, stepKey, messageID, clientID, toolName, res, err)

	return res, err
}

func (e *ToolEngine) ExecuteDependencies(
	ctx context.Context,
	messageID,
	clientID string,
	tool *tool_protocol.Tool,
	needs []tool_protocol.NeedItem,
	inputFulfiller tool_engine_models.IInputFulfiller,
	callback tool_engine_models.AgenticWorkflowCallback,
	parentFunc string,
) (bool, []string, error) {
	if len(needs) == 0 {
		return true, nil, nil
	}

	needNames := make([]string, len(needs))
	for i, need := range needs {
		needNames[i] = need.Name
	}

	allMet, missingNeedNames, executionResults, err := e.executionTracker.CheckNeeds(ctx, messageID, needNames)
	if err != nil {
		return false, executionResults, fmt.Errorf("error checking dependencies: %w", err)
	}

	// Build map of missing needs
	missingNeedMap := make(map[string]bool)
	for _, name := range missingNeedNames {
		missingNeedMap[name] = true
	}

	// Force re-execution for needs with params, even if already executed
	for _, need := range needs {
		if len(need.Params) > 0 && !missingNeedMap[need.Name] {
			missingNeedMap[need.Name] = true
			if logger != nil {
				logger.Debugf("[NEEDS] Forcing re-execution of '%s' because it has params", need.Name)
			}
		}
	}

	// Force re-execution for functions without explicit cache field
	for _, need := range needs {
		if !missingNeedMap[need.Name] {
			funcDef, err := e.FindFunctionDefinition(tool, need.Name)
			if err != nil {
				continue
			}
			if funcDef.ParsedCache == nil {
				missingNeedMap[need.Name] = true
				if logger != nil {
					logger.Debugf("[NEEDS] Forcing re-execution of '%s' (no cache field)", need.Name)
				}
			}
		}
	}

	// Check if we actually have anything to execute
	if allMet && len(missingNeedMap) == 0 {
		return true, executionResults, nil
	}

	var executeResults []string

	// Track outputs from sibling needs to enable chaining like $funcA.field in funcB's params
	siblingOutputs := make(map[string]interface{})

	for _, need := range needs {
		if !missingNeedMap[need.Name] {
			continue
		}

		if need.Name == "askToKnowledgeBase" {
			if need.Query == "" {
				return false, nil, fmt.Errorf("askToKnowledgeBase requires a query parameter")
			}

			result, err := e.executeAskToKnowledge(ctx, need.Query, messageID, clientID, need.Name, callback)

			callback.OnDependencyExecuted(ctx, messageID, clientID, parentFunc, need.Name, result, err)

			if err != nil {
				return false, missingNeedNames, fmt.Errorf("error executing dependency %s: %w", need.Name, err)
			}

			if tool_engine_utils.IsSpecialReturn(result) {
				executeResults = append(executeResults, fmt.Sprintf("%s: %s", need.Name, result))
				return false, executeResults, nil
			}

			executeResults = append(executeResults, fmt.Sprintf("%s: %s", need.Name, result))
			continue
		}

		// Handle sendTeamMessage system function
		if need.Name == tool_protocol.SysFuncSendTeamMessage {
			startTime := time.Now()
			inputs := map[string]interface{}{
				"message": need.Params["message"],
				"role":    need.Params["role"],
			}

			if e.toolFunctions == nil {
				if logger != nil {
					logger.Warnf("sendTeamMessage skipped: toolFunctions not initialized")
				}
				e.executionTracker.RecordExecution(ctx, messageID, clientID, "system", "sendTeamMessage",
					"Send team message notification", inputs, "", "skipped: toolFunctions not initialized",
					nil, startTime, tool_protocol.StatusFailed, nil)
				continue
			}

			message, _ := need.Params["message"].(string)
			role, _ := need.Params["role"].(string)

			if role == "" {
				role = "all"
			}

			// Get accumulated inputs for variable resolution
			var accumulatedInputs map[string]interface{}
			parentFunctionKey := GenerateFunctionKey(tool.Name, parentFunc)
			parentContextKey := fmt.Sprintf("pre_fulfilled_inputs_%s_%s_%s", clientID, messageID, parentFunctionKey)
			if preFilled, ok := ctx.Value(parentContextKey).(map[string]interface{}); ok && len(preFilled) > 0 {
				accumulatedInputs = preFilled
			} else if callback != nil {
				accumulatedInputs, _ = callback.GetFulfilledInputs(messageID, tool.Name, parentFunc)
			}

			resolvedMessage, err := e.variableReplacer.ReplaceVariables(ctx, message, accumulatedInputs)
			if err != nil {
				if logger != nil {
					logger.Warnf("sendTeamMessage: variable replacement failed for message '%s': %v", message, err)
				}
			} else {
				message = resolvedMessage
			}

			inputs["message"] = message
			inputs["role"] = role

			if message == "" {
				if logger != nil {
					logger.Warnf("sendTeamMessage skipped: message is empty after variable replacement for function %s", parentFunc)
				}
				e.executionTracker.RecordExecution(ctx, messageID, clientID, "system", "sendTeamMessage",
					"Send team message notification", inputs, "", "skipped: message empty after variable replacement",
					nil, startTime, tool_protocol.StatusFailed, nil)
				continue
			}

			result, err := e.toolFunctions.SendTeamMessage(ctx, message, role)

			status := tool_protocol.StatusComplete
			output := result
			if err != nil {
				status = tool_protocol.StatusFailed
				output = fmt.Sprintf("error: %v", err)
			}
			e.executionTracker.RecordExecution(ctx, messageID, clientID, "system", "sendTeamMessage",
				"Send team message notification", inputs, "", output, nil, startTime, status, nil)

			if callback != nil {
				callback.OnDependencyExecuted(ctx, messageID, clientID, parentFunc, need.Name, result, err)
			}

			if err != nil {
				if logger != nil {
					logger.Warnf("sendTeamMessage error: %v", err)
				}
				continue
			}

			executeResults = append(executeResults, fmt.Sprintf("%s: %s", need.Name, result))
			continue
		}

		// Handle Learn system function
		if need.Name == tool_protocol.SysFuncLearn {
			startTime := time.Now()
			inputs := map[string]interface{}{
				"textOrMediaLink": need.Params["textOrMediaLink"],
			}

			if e.toolFunctions == nil {
				if logger != nil {
					logger.Warnf("Learn skipped: toolFunctions not initialized")
				}
				e.executionTracker.RecordExecution(ctx, messageID, clientID, "system", "Learn",
					"Store information for future use", inputs, "", "skipped: toolFunctions not initialized",
					nil, startTime, tool_protocol.StatusFailed, nil)
				continue
			}

			textOrMediaLink, _ := need.Params["textOrMediaLink"].(string)

			var accumulatedInputs map[string]interface{}
			parentFunctionKey := GenerateFunctionKey(tool.Name, parentFunc)
			parentContextKey := fmt.Sprintf("pre_fulfilled_inputs_%s_%s_%s", clientID, messageID, parentFunctionKey)
			if preFilled, ok := ctx.Value(parentContextKey).(map[string]interface{}); ok && len(preFilled) > 0 {
				accumulatedInputs = preFilled
			} else if callback != nil {
				accumulatedInputs, _ = callback.GetFulfilledInputs(messageID, tool.Name, parentFunc)
			}

			resolvedText, err := e.variableReplacer.ReplaceVariables(ctx, textOrMediaLink, accumulatedInputs)
			if err != nil {
				if logger != nil {
					logger.Warnf("Learn: variable replacement failed for textOrMediaLink: %v", err)
				}
			} else {
				textOrMediaLink = resolvedText
			}

			inputs["textOrMediaLink"] = textOrMediaLink

			if textOrMediaLink == "" {
				if logger != nil {
					logger.Warnf("Learn skipped: textOrMediaLink is empty after variable replacement for function %s", parentFunc)
				}
				e.executionTracker.RecordExecution(ctx, messageID, clientID, "system", "Learn",
					"Store information for future use", inputs, "", "skipped: textOrMediaLink empty after variable replacement",
					nil, startTime, tool_protocol.StatusFailed, nil)
				continue
			}

			result, err := e.toolFunctions.Learn(ctx, textOrMediaLink)

			status := tool_protocol.StatusComplete
			output := result
			if err != nil {
				status = tool_protocol.StatusFailed
				output = fmt.Sprintf("error: %v", err)
			}
			e.executionTracker.RecordExecution(ctx, messageID, clientID, "system", "Learn",
				"Store information for future use", inputs, "", output, nil, startTime, status, nil)

			if callback != nil {
				callback.OnDependencyExecuted(ctx, messageID, clientID, parentFunc, need.Name, result, err)
			}

			if err != nil {
				if logger != nil {
					logger.Warnf("Learn error: %v", err)
				}
				continue
			}

			executeResults = append(executeResults, fmt.Sprintf("%s: %s", need.Name, result))
			continue
		}

		// Handle queryMemories system function
		if need.Name == tool_protocol.SysFuncQueryMemories {
			startTime := time.Now()

			query := need.Query
			if query == "" {
				query, _ = need.Params["query"].(string)
			}

			if query == "" {
				return false, nil, fmt.Errorf("queryMemories requires a query parameter")
			}

			inputs := map[string]interface{}{"query": query}

			if e.toolFunctions == nil {
				if logger != nil {
					logger.Warnf("queryMemories skipped: toolFunctions not initialized")
				}
				e.executionTracker.RecordExecution(ctx, messageID, clientID, "system", "queryMemories",
					"Query stored memories from previous executions", inputs, "", "skipped: toolFunctions not initialized",
					nil, startTime, tool_protocol.StatusFailed, nil)
				continue
			}

			var accumulatedInputs map[string]interface{}
			parentFunctionKey := GenerateFunctionKey(tool.Name, parentFunc)
			parentContextKey := fmt.Sprintf("pre_fulfilled_inputs_%s_%s_%s", clientID, messageID, parentFunctionKey)
			if preFilled, ok := ctx.Value(parentContextKey).(map[string]interface{}); ok && len(preFilled) > 0 {
				accumulatedInputs = preFilled
			} else if callback != nil {
				accumulatedInputs, _ = callback.GetFulfilledInputs(messageID, tool.Name, parentFunc)
			}

			resolvedQuery, err := e.variableReplacer.ReplaceVariables(ctx, query, accumulatedInputs)
			if err != nil {
				if logger != nil {
					logger.Warnf("queryMemories: variable replacement failed for query '%s': %v", query, err)
				}
			} else {
				query = resolvedQuery
			}

			inputs["query"] = query

			result, err := e.toolFunctions.QueryMemories(ctx, query)

			status := tool_protocol.StatusComplete
			output := result
			if err != nil {
				status = tool_protocol.StatusFailed
				output = fmt.Sprintf("error: %v", err)
			}
			e.executionTracker.RecordExecution(ctx, messageID, clientID, "system", "queryMemories",
				"Query stored memories from previous executions", inputs, "", output, nil, startTime, status, nil)

			if callback != nil {
				callback.OnDependencyExecuted(ctx, messageID, clientID, parentFunc, need.Name, result, err)
			}

			if err != nil {
				if logger != nil {
					logger.Warnf("queryMemories error: %v", err)
				}
				continue
			}

			executeResults = append(executeResults, fmt.Sprintf("%s: %s", need.Name, result))
			continue
		}

		// Handle createMemory system function
		if need.Name == tool_protocol.SysFuncCreateMemory {
			startTime := time.Now()
			inputs := map[string]interface{}{
				"content": need.Params["content"],
				"topic":   need.Params["topic"],
			}

			if e.toolFunctions == nil {
				if logger != nil {
					logger.Warnf("createMemory skipped: toolFunctions not initialized")
				}
				e.executionTracker.RecordExecution(ctx, messageID, clientID, "system", "createMemory",
					"Create a new memory entry", inputs, "", "skipped: toolFunctions not initialized",
					nil, startTime, tool_protocol.StatusFailed, nil)
				continue
			}

			content, _ := need.Params["content"].(string)
			topic, _ := need.Params["topic"].(string)

			var accumulatedInputs map[string]interface{}
			parentFunctionKey := GenerateFunctionKey(tool.Name, parentFunc)
			parentContextKey := fmt.Sprintf("pre_fulfilled_inputs_%s_%s_%s", clientID, messageID, parentFunctionKey)
			if preFilled, ok := ctx.Value(parentContextKey).(map[string]interface{}); ok && len(preFilled) > 0 {
				accumulatedInputs = preFilled
			} else if callback != nil {
				accumulatedInputs, _ = callback.GetFulfilledInputs(messageID, tool.Name, parentFunc)
			}

			// Also inject sibling outputs for variable resolution
			for k, v := range siblingOutputs {
				if accumulatedInputs == nil {
					accumulatedInputs = make(map[string]interface{})
				}
				accumulatedInputs[k] = v
			}

			resolvedContent, err := e.variableReplacer.ReplaceVariables(ctx, content, accumulatedInputs)
			if err != nil {
				if logger != nil {
					logger.Warnf("createMemory: variable replacement failed for content: %v", err)
				}
			} else {
				content = resolvedContent
			}

			if topic != "" {
				resolvedTopic, err := e.variableReplacer.ReplaceVariables(ctx, topic, accumulatedInputs)
				if err != nil {
					if logger != nil {
						logger.Warnf("createMemory: variable replacement failed for topic: %v", err)
					}
				} else {
					topic = resolvedTopic
				}
			}

			inputs["content"] = content
			inputs["topic"] = topic

			if content == "" {
				if logger != nil {
					logger.Warnf("createMemory skipped: content is empty after variable replacement for function %s", parentFunc)
				}
				e.executionTracker.RecordExecution(ctx, messageID, clientID, "system", "createMemory",
					"Create a new memory entry", inputs, "", "skipped: content empty after variable replacement",
					nil, startTime, tool_protocol.StatusFailed, nil)
				continue
			}

			result, err := e.toolFunctions.CreateMemory(ctx, content, topic)

			status := tool_protocol.StatusComplete
			output := result
			if err != nil {
				status = tool_protocol.StatusFailed
				output = fmt.Sprintf("error: %v", err)
			}
			e.executionTracker.RecordExecution(ctx, messageID, clientID, "system", "createMemory",
				"Create a new memory entry", inputs, "", output, nil, startTime, status, nil)

			if callback != nil {
				callback.OnDependencyExecuted(ctx, messageID, clientID, parentFunc, need.Name, result, err)
			}

			if err != nil {
				if logger != nil {
					logger.Warnf("createMemory error: %v", err)
				}
				continue
			}

			executeResults = append(executeResults, fmt.Sprintf("%s: %s", need.Name, result))
			continue
		}

		// Handle sendMessageToUser system function (callback-only, NOT available to LLM)
		if need.Name == tool_protocol.SysFuncSendMessageToUser {
			startTime := time.Now()
			inputs := map[string]interface{}{
				"message": need.Params["message"],
			}

			if e.toolFunctions == nil {
				if logger != nil {
					logger.Warnf("sendMessageToUser skipped: toolFunctions not initialized")
				}
				e.executionTracker.RecordExecution(ctx, messageID, clientID, "system", "sendMessageToUser",
					"Send message to user via LLM proxy", inputs, "", "skipped: toolFunctions not initialized",
					nil, startTime, tool_protocol.StatusSkipped, nil)
				continue
			}

			messageParam, _ := need.Params["message"].(string)
			if messageParam == "" {
				messageParam, _ = need.Params["content"].(string)
			}

			var accumulatedInputs map[string]interface{}
			parentFunctionKey := GenerateFunctionKey(tool.Name, parentFunc)
			parentContextKey := fmt.Sprintf("pre_fulfilled_inputs_%s_%s_%s", clientID, messageID, parentFunctionKey)
			if preFilled, ok := ctx.Value(parentContextKey).(map[string]interface{}); ok && len(preFilled) > 0 {
				accumulatedInputs = preFilled
			} else if callback != nil {
				accumulatedInputs, _ = callback.GetFulfilledInputs(messageID, tool.Name, parentFunc)
			}

			resolvedMessage, err := e.variableReplacer.ReplaceVariables(ctx, messageParam, accumulatedInputs)
			if err != nil {
				if logger != nil {
					logger.Warnf("sendMessageToUser: variable replacement failed for message '%s': %v", messageParam, err)
				}
			} else {
				messageParam = resolvedMessage
			}

			inputs["message"] = messageParam

			if messageParam == "" {
				if logger != nil {
					logger.Warnf("sendMessageToUser skipped: message is empty after variable replacement for function %s", parentFunc)
				}
				e.executionTracker.RecordExecution(ctx, messageID, clientID, "system", "sendMessageToUser",
					"Send message to user via LLM proxy", inputs, "", "skipped: message empty after variable replacement",
					nil, startTime, tool_protocol.StatusSkipped, nil)
				continue
			}

			// Execute async to avoid blocking workflow
			go func(msg string, msgID string, cID string, cb tool_engine_models.AgenticWorkflowCallback, originalCtx context.Context) {
				bgCtx := context.Background()
				if retrievedMsg := originalCtx.Value(MessageInContextKey); retrievedMsg != nil {
					bgCtx = context.WithValue(bgCtx, MessageInContextKey, retrievedMsg)
				}
				if convType := originalCtx.Value(ConversationTypeInContextKey); convType != nil {
					bgCtx = context.WithValue(bgCtx, ConversationTypeInContextKey, convType)
				}
				execErr := e.toolFunctions.SendMessageToUser(bgCtx, msg)

				execStatus := tool_protocol.StatusComplete
				execOutput := "Message sent successfully"
				if execErr != nil {
					execStatus = tool_protocol.StatusFailed
					execOutput = fmt.Sprintf("error: %v", execErr)
					if logger != nil {
						logger.Errorf("sendMessageToUser async error: %v", execErr)
					}
				}

				e.executionTracker.RecordExecution(bgCtx, msgID, cID, "system", "sendMessageToUser",
					"Send message to user via LLM proxy", inputs, "", execOutput,
					nil, startTime, execStatus, nil)

				if cb != nil {
					cb.OnDependencyExecuted(bgCtx, msgID, cID, parentFunc, "sendMessageToUser", execOutput, execErr)
				}
			}(messageParam, messageID, clientID, callback, ctx)

			if logger != nil {
				logger.Infof("sendMessageToUser: queued message for user (length: %d chars)", len(messageParam))
			}
			executeResults = append(executeResults, fmt.Sprintf("%s: queued", need.Name))
			continue
		}

		if isSystemFunction(need.Name) {
			return false, []string{need.Name}, fmt.Errorf(
				"system function '%s' cannot be auto-executed in needs, only 'askToKnowledgeBase', 'queryMemories', 'createMemory', 'sendTeamMessage', 'Learn', and 'sendMessageToUser' are supported",
				need.Name,
			)
		}

		// Find the function and its parent tool (may be from a different tool like system tools)
		function, functionTool, err := e.FindFunctionAndTool(tool, need.Name)
		if err != nil {
			return false, missingNeedNames, fmt.Errorf("error finding dependency %s: %w", need.Name, err)
		}

		// Handle params injection if the need has params
		execCtx := ctx
		// Ensure messageID is in context for variable replacement (needed for $result[n] access)
		execCtx = context.WithValue(execCtx, "message_id", messageID)
		if len(need.Params) > 0 {
			var accumulatedInputs map[string]interface{}
			parentFunctionKey := GenerateFunctionKey(tool.Name, parentFunc)
			parentContextKey := fmt.Sprintf("pre_fulfilled_inputs_%s_%s_%s", clientID, messageID, parentFunctionKey)
			if logger != nil {
				logger.Infof("[EXEC_DEPS] Looking for parent '%s' inputs - tool: %s, key: %s",
					parentFunc, tool.Name, parentContextKey)
			}
			if preFilled, ok := ctx.Value(parentContextKey).(map[string]interface{}); ok && len(preFilled) > 0 {
				accumulatedInputs = preFilled
				if logger != nil {
					logger.Infof("[EXEC_DEPS] Found %d pre-fulfilled inputs from context for parent '%s': %v",
						len(preFilled), parentFunc, preFilled)
				}
			} else if callback != nil {
				accumulatedInputs, _ = callback.GetFulfilledInputs(messageID, tool.Name, parentFunc)
				if logger != nil {
					logger.Infof("[EXEC_DEPS] Context lookup failed, callback returned: %v", accumulatedInputs)
				}
			} else {
				if logger != nil {
					logger.Warnf("[EXEC_DEPS] No parent inputs found - context lookup failed and callback is nil")
				}
			}

			// Merge sibling outputs into accumulatedInputs
			if len(siblingOutputs) > 0 {
				if accumulatedInputs == nil {
					accumulatedInputs = make(map[string]interface{})
				}
				for k, v := range siblingOutputs {
					accumulatedInputs[k] = v
				}
				if logger != nil {
					logger.Infof("[EXEC_DEPS] Merged %d sibling outputs into accumulatedInputs: %v", len(siblingOutputs), siblingOutputs)
				}
			}

			// Resolve params using variable replacer
			if logger != nil {
				logger.Infof("[EXEC_DEPS] Resolving %d params for need '%s' with accumulatedInputs: %v",
					len(need.Params), need.Name, accumulatedInputs)
			}
			resolvedParams := make(map[string]interface{})
			for paramName, paramValue := range need.Params {
				strValue, ok := paramValue.(string)
				if !ok {
					resolvedParams[paramName] = paramValue
					if logger != nil {
						logger.Debugf("[EXEC_DEPS] Param '%s' is non-string, using as-is: %v", paramName, paramValue)
					}
					continue
				}

				if logger != nil {
					logger.Infof("[EXEC_DEPS] Resolving param '%s' = '%s' with inputs: %v", paramName, strValue, accumulatedInputs)
				}
				replacedValue, replaceErr := e.variableReplacer.ReplaceVariables(execCtx, strValue, accumulatedInputs)
				if replaceErr != nil {
					if logger != nil {
						logger.Warnf("[EXEC_DEPS] Failed to resolve param '%s' for need '%s': %v (skipping pre-fill)", paramName, need.Name, replaceErr)
					}
					continue
				}

				if logger != nil {
					logger.Infof("[EXEC_DEPS] Param '%s': '%s' -> '%s'", paramName, strValue, replacedValue)
				}

				if (strings.HasPrefix(strValue, "$") && replacedValue == "") ||
					(strings.HasPrefix(replacedValue, "$") && replacedValue == strValue) {
					if logger != nil {
						logger.Warnf("[EXEC_DEPS] Param '%s' for need '%s' references unavailable variable '%s' (skipping pre-fill)", paramName, need.Name, strValue)
					}
					continue
				}

				resolvedParams[paramName] = replacedValue
			}
			if logger != nil {
				logger.Infof("[EXEC_DEPS] Final resolved params for '%s': %v", need.Name, resolvedParams)
			}

			if logger != nil {
				logger.Infof("[EXEC_DEPS-DEBUG] Resolved %d params for '%s': %v", len(resolvedParams), need.Name, resolvedParams)
			}
			if len(resolvedParams) > 0 {
				functionKey := GenerateFunctionKey(functionTool.Name, function.Name)
				contextKey := fmt.Sprintf("pre_fulfilled_inputs_%s_%s_%s", clientID, messageID, functionKey)
				if logger != nil {
					logger.Infof("[EXEC_DEPS-DEBUG] Looking for existing inputs in context with key '%s'", contextKey)
				}

				if existingInputs, hasCheckpoint := ctx.Value(contextKey).(map[string]interface{}); hasCheckpoint && len(existingInputs) > 0 {
					if logger != nil {
						logger.Infof("[EXEC_DEPS-DEBUG] Found %d existing inputs in context for '%s': %v", len(existingInputs), need.Name, existingInputs)
					}
					merged := make(map[string]interface{})
					for k, v := range existingInputs {
						merged[k] = v
					}
					paramsOverridden := 0
					paramsAdded := 0
					for k, v := range resolvedParams {
						if _, exists := merged[k]; exists {
							if logger != nil {
								logger.Infof("[EXEC_DEPS-DEBUG] Overriding existing value '%v' with resolved param '%v' for key '%s'",
									merged[k], v, k)
							}
							merged[k] = v
							paramsOverridden++
						} else {
							merged[k] = v
							paramsAdded++
						}
					}
					if logger != nil {
						logger.Infof("[EXEC_DEPS-DEBUG] Merge complete for '%s': %d overridden, %d added. Final merged: %v",
							need.Name, paramsOverridden, paramsAdded, merged)
					}
					execCtx = context.WithValue(ctx, contextKey, merged)
				} else {
					if logger != nil {
						logger.Infof("[EXEC_DEPS-DEBUG] No existing inputs in context for '%s', using resolved params directly: %v", need.Name, resolvedParams)
					}
					execCtx = context.WithValue(ctx, contextKey, resolvedParams)
				}
			} else {
				if logger != nil {
					logger.Infof("[EXEC_DEPS-DEBUG] No resolved params for '%s', skipping context injection", need.Name)
				}
			}
		}

		// Inject shouldBeHandledAsMessageToUser override if specified at call-site
		if need.ShouldBeHandledAsMessageToUser != nil {
			functionKey := GenerateFunctionKey(functionTool.Name, need.Name)
			overrideKey := fmt.Sprintf("shouldBeHandledAsMessageToUser_override_%s_%s_%s", clientID, messageID, functionKey)
			execCtx = context.WithValue(execCtx, overrideKey, *need.ShouldBeHandledAsMessageToUser)
			if logger != nil {
				logger.Debugf("[EXEC_DEPS] Injected shouldBeHandledAsMessageToUser override=%v for '%s' with key '%s'",
					*need.ShouldBeHandledAsMessageToUser, need.Name, overrideKey)
			}
		}

		// Inject requiresUserConfirmation override if specified at call-site
		if need.RequiresUserConfirmation != nil {
			functionKey := GenerateFunctionKey(functionTool.Name, need.Name)
			overrideKey := fmt.Sprintf("requiresUserConfirmation_override_%s_%s_%s", clientID, messageID, functionKey)
			execCtx = context.WithValue(execCtx, overrideKey, need.RequiresUserConfirmation)
			if logger != nil {
				logger.Debugf("[EXEC_DEPS] Injected requiresUserConfirmation override for '%s' with key '%s'",
					need.Name, overrideKey)
			}
		}

		if logger != nil {
			logger.Debugf("Auto-executing dependency: %s (from tool: %s)\n", need.Name, functionTool.Name)
		}
		result, err := e.ExecuteFunctionByNameAndVersion(
			execCtx,
			messageID,
			functionTool.Name,
			functionTool.Version,
			function.Name,
			inputFulfiller,
		)

		callback.OnDependencyExecuted(ctx, messageID, clientID, parentFunc, need.Name, result, err)

		if err != nil {
			return false, missingNeedNames, fmt.Errorf("error executing dependency %s: %w", need.Name, err)
		}

		if tool_engine_utils.IsSpecialReturn(result) {
			executeResults = append(executeResults, fmt.Sprintf("%s: %s", need.Name, result))
			return false, executeResults, nil
		}

		executeResults = append(executeResults, fmt.Sprintf("%s: %s", need.Name, result))

		// Store result in siblingOutputs for potential use by subsequent needs
		var parsedResult interface{}
		if json.Unmarshal([]byte(result), &parsedResult) == nil {
			siblingOutputs[need.Name] = parsedResult
			if logger != nil {
				logger.Infof("[EXEC_DEPS] Stored sibling output for '%s' (parsed JSON): %v", need.Name, parsedResult)
			}
		} else {
			siblingOutputs[need.Name] = result
			if logger != nil {
				logger.Infof("[EXEC_DEPS] Stored sibling output for '%s' (string): %s", need.Name, result)
			}
		}
	}

	allMet, stillMissing, executionResults, err := e.executionTracker.CheckNeeds(ctx, messageID, needNames)
	if err != nil {
		return false, nil, fmt.Errorf("error checking dependencies after execution: %w", err)
	}

	if !allMet {
		return false, stillMissing, fmt.Errorf(
			"the following dependencies still failed after auto-execution: %s",
			strings.Join(stillMissing, ", "),
		)
	}

	allResults := append(executionResults, executeResults...)
	return true, allResults, nil
}

// FindFunctionDefinition looks up a function definition by name in the provided tool only.
func (e *ToolEngine) FindFunctionDefinition(tool *tool_protocol.Tool, functionName string) (*tool_protocol.Function, error) {
	for i := range tool.Functions {
		if tool.Functions[i].Name == functionName {
			return &tool.Functions[i], nil
		}
	}

	return nil, fmt.Errorf("function %s not found in the tool", functionName)
}

// FindFunctionAndTool looks up a function definition by name and returns both the function and the tool it belongs to.
func (e *ToolEngine) FindFunctionAndTool(tool *tool_protocol.Tool, functionName string) (*tool_protocol.Function, *tool_protocol.Tool, error) {
	if logger != nil {
		logger.Infof("[FindFunctionAndTool] Looking for function: '%s'", functionName)
	}

	// Handle dot notation (e.g., "utils_shared.markConversationAsImportant")
	targetToolName := ""
	baseFunctionName := functionName
	if parts := strings.SplitN(functionName, ".", 2); len(parts) == 2 {
		targetToolName = parts[0]
		baseFunctionName = parts[1]
		if logger != nil {
			logger.Infof("[FindFunctionAndTool] Dot notation detected: tool='%s', function='%s'", targetToolName, baseFunctionName)
		}
	}

	// If dot notation was used, search in the specific tool first
	if targetToolName != "" {
		e.mutex.RLock()
		defer e.mutex.RUnlock()

		var availableTools []string
		for _, loadedTool := range e.toolDefinitions {
			availableTools = append(availableTools, loadedTool.Name)
		}
		if logger != nil {
			logger.Infof("[FindFunctionAndTool] Available tools: %v", availableTools)
		}

		for _, loadedTool := range e.toolDefinitions {
			if loadedTool.Name != targetToolName {
				continue
			}
			if logger != nil {
				logger.Infof("[FindFunctionAndTool] Found target tool '%s', searching for function '%s'", targetToolName, baseFunctionName)
			}
			var availableFuncs []string
			for i := range loadedTool.Functions {
				availableFuncs = append(availableFuncs, loadedTool.Functions[i].Name)
				if loadedTool.Functions[i].Name == baseFunctionName {
					toolCopy := loadedTool
					funcCopy := loadedTool.Functions[i]
					if logger != nil {
						logger.Infof("[FindFunctionAndTool] Found function '%s' in tool '%s'", baseFunctionName, targetToolName)
					}
					return &funcCopy, &toolCopy, nil
				}
			}
			if logger != nil {
				logger.Errorf("[FindFunctionAndTool] Function '%s' not found in tool '%s'. Available functions: %v", baseFunctionName, targetToolName, availableFuncs)
			}
			return nil, nil, fmt.Errorf("function %s not found in tool %s", baseFunctionName, targetToolName)
		}
		if logger != nil {
			logger.Errorf("[FindFunctionAndTool] Tool '%s' not found in toolDefinitions", targetToolName)
		}
		return nil, nil, fmt.Errorf("tool %s not found", targetToolName)
	}

	// First, search in the provided tool
	for i := range tool.Functions {
		if tool.Functions[i].Name == functionName {
			return &tool.Functions[i], tool, nil
		}
	}

	// Not found in the provided tool, search in system and shared tools
	e.mutex.RLock()
	defer e.mutex.RUnlock()

	for _, loadedTool := range e.toolDefinitions {
		if !loadedTool.IsSystemApp && !loadedTool.IsSharedApp {
			continue
		}

		for i := range loadedTool.Functions {
			if loadedTool.Functions[i].Name == functionName {
				toolCopy := loadedTool
				funcCopy := loadedTool.Functions[i]
				return &funcCopy, &toolCopy, nil
			}
		}
	}

	return nil, nil, fmt.Errorf("function %s not found in the tool, system tools, or shared tools", functionName)
}

func (e *ToolEngine) executeAskToKnowledge(ctx context.Context, query string, messageID string, clientID string, functionName string, callback tool_engine_models.AgenticWorkflowCallback) (string, error) {
	var stepKey string
	stepKey, ok := ctx.Value(StepKeyInContextKey).(string)
	if !ok {
		if logger != nil {
			logger.Debugf("StepKey not found in context for askToKnowledgeBase")
		}
	}

	inputs := map[string]interface{}{"query": query}
	inputsJSON, _ := json.Marshal(inputs)

	cachedResult, cacheHit, err := e.executionTracker.GetFromGlobalCache(ctx, "system", functionName, inputs)
	if err != nil {
		if logger != nil {
			logger.Warnf("Error checking cache for knowledge query: %v", err)
		}
	}

	if cacheHit {
		if logger != nil {
			logger.Debugf("Knowledge cache hit for query: %s", query)
		}

		err = e.executionTracker.RecordExecution(
			ctx,
			messageID,
			clientID,
			"system",
			functionName,
			"Query knowledge base for information",
			inputs,
			"",
			cachedResult,
			nil,
			time.Now(),
			tool_protocol.StatusComplete,
			nil,
		)
		if err != nil {
			if logger != nil {
				logger.Warnf("Failed to record cached knowledge execution: %v", err)
			}
		}

		callback.OnFunctionExecuted(ctx, stepKey, messageID, clientID, functionName, "system", "Query knowledge base for information", string(inputsJSON), cachedResult, nil, nil)

		return cachedResult, nil
	}

	// If no knowledge query tool is set, return a graceful error
	if e.knowledgeQuery == nil {
		return "", fmt.Errorf("knowledge query tool not configured")
	}

	result, err := e.knowledgeQuery.Call(ctx, query)
	if err != nil {
		callback.OnFunctionExecuted(ctx, stepKey, messageID, clientID, functionName, "system", "Query knowledge base for information", string(inputsJSON), "", err, nil)
		return "", fmt.Errorf("error querying knowledge base: %w", err)
	}

	cacheErr := e.executionTracker.AddToGlobalCache(ctx, "system", functionName, inputs, result, "", 900)
	if cacheErr != nil {
		if logger != nil {
			logger.Warnf("Failed to cache knowledge query result: %v", cacheErr)
		}
	}

	err = e.executionTracker.RecordExecution(
		ctx,
		messageID,
		clientID,
		"system",
		functionName,
		"Query knowledge base for information",
		inputs,
		"",
		result,
		nil,
		time.Now(),
		tool_protocol.StatusComplete,
		nil,
	)
	if err != nil {
		if logger != nil {
			logger.Warnf("Failed to record execution: %v", err)
		}
	}

	callback.OnFunctionExecuted(ctx, stepKey, messageID, clientID, functionName, "system", "Query knowledge base for information", string(inputsJSON), result, nil, nil)

	if e.askHumanCallback != nil {
		// Check if the LLM was unable to answer - use a simple heuristic
		lowerResult := strings.ToLower(result)
		unableToAnswer := strings.Contains(lowerResult, "i do not have enough information") ||
			strings.Contains(lowerResult, "i don't have enough information") ||
			strings.Contains(lowerResult, "no relevant information")
		if unableToAnswer {
			go e.askHumanForHelp(ctx, query)
		}
	}

	return result, nil
}

func (e *ToolEngine) askHumanForHelp(ctx context.Context, originalQuery string) {
	defer func() {
		if r := recover(); r != nil {
			if logger != nil {
				logger.Errorf("Panic in askHumanForHelp: %v", r)
			}
		}
	}()

	helpMessage := fmt.Sprintf("I tried to answer this question: %s but I was not able. Can you help me and explain it for me please?", originalQuery)

	_, err := e.askHumanCallback(ctx, helpMessage)
	if err != nil {
		if logger != nil {
			logger.Warnf("Failed to ask human for help with query '%s': %v", originalQuery, err)
		}
	}
}

func (e *ToolEngine) computeToolsChecksum() (string, error) {
	var keys []string

	for key := range e.toolDefinitions {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	h := sha256.New()
	for _, key := range keys {
		h.Write([]byte(key))
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func functionIdentification(toolName, functionName, version string) string {
	return fmt.Sprintf("%s:%s:%s", toolName, functionName, version)
}

// isSystemFunction checks if a function name is a built-in system function
func isSystemFunction(functionName string) bool {
	systemFunctions := map[string]bool{
		"Ask":               true,
		"Learn":             true,
		"AskHuman":          true,
		"NotifyHuman":       true,
		"sendTeamMessage":   true,
		"sendMessageToUser": true,
		"createMemory":      true,
		"queryMemories":     true,
	}

	return systemFunctions[functionName]
}

// syncWorkflowsFromYAML syncs workflow definitions from YAML to the database
func (e *ToolEngine) syncWorkflowsFromYAML(ctx context.Context, workflows []tool_protocol.WorkflowYAML, toolName string) error {
	start := time.Now()
	if logger != nil {
		logger.Debugf("[ToolEngine] syncWorkflowsFromYAML: starting sync for %d workflows from tool %s", len(workflows), toolName)
	}
	for _, yamlWorkflow := range workflows {
		if err := e.syncSingleWorkflow(ctx, yamlWorkflow, toolName); err != nil {
			return fmt.Errorf("failed to sync workflow '%s': %w", yamlWorkflow.CategoryName, err)
		}
	}
	if logger != nil {
		logger.Debugf("[ToolEngine] syncWorkflowsFromYAML: completed in %v", time.Since(start))
	}
	return nil
}

// syncSingleWorkflow syncs a single workflow from YAML to database
func (e *ToolEngine) syncSingleWorkflow(ctx context.Context, yamlWorkflow tool_protocol.WorkflowYAML, toolName string) error {
	start := time.Now()
	if logger != nil {
		logger.Debugf("[ToolEngine] syncSingleWorkflow: checking workflow '%s'", yamlWorkflow.CategoryName)
	}
	existing, err := e.workflowRepo.GetWorkflowByCategory(ctx, yamlWorkflow.CategoryName)
	if logger != nil {
		logger.Debugf("[ToolEngine] syncSingleWorkflow: GetWorkflowByCategory took %v", time.Since(start))
	}
	if err != nil {
		// Workflow doesn't exist, create it
		return e.createWorkflowFromYAML(ctx, yamlWorkflow, toolName)
	}

	// If workflow exists and is human-verified, skip replacement
	if existing.HumanVerified {
		if logger != nil {
			logger.Infof("Skipping workflow '%s' - human verified", yamlWorkflow.CategoryName)
		}
		return nil
	}

	// Replace the workflow
	if logger != nil {
		logger.Infof("Replacing workflow '%s' from YAML", yamlWorkflow.CategoryName)
	}
	return e.updateWorkflowFromYAML(ctx, existing.ID, yamlWorkflow, toolName)
}

// createWorkflowFromYAML creates a new workflow in the database from YAML definition
func (e *ToolEngine) createWorkflowFromYAML(ctx context.Context, yamlWorkflow tool_protocol.WorkflowYAML, toolName string) error {
	workflowType := types.WorkflowTypeUser
	if yamlWorkflow.WorkflowType != "" {
		workflowType = types.WorkflowType(yamlWorkflow.WorkflowType)
	}

	workflow := types.Workflow{
		CategoryName:              yamlWorkflow.CategoryName,
		HumanReadableCategoryName: yamlWorkflow.HumanReadableCategoryName,
		Description:               yamlWorkflow.Description,
		HumanVerified:             false,
		WorkflowType:              workflowType,
	}

	if err := e.workflowRepo.CreateWorkflow(ctx, &workflow); err != nil {
		return fmt.Errorf("failed to create workflow: %w", err)
	}

	for _, yamlStep := range yamlWorkflow.Steps {
		step := types.WorkflowStep{
			WorkflowID:                 workflow.ID,
			Order:                      yamlStep.Order,
			Action:                     yamlStep.Action,
			HumanReadableActionName:    yamlStep.HumanReadableActionName,
			Rationale:                  yamlStep.Instructions,
			ExpectedOutcomeDescription: yamlStep.ExpectedOutcomeDescription,
		}
		if err := e.workflowRepo.AddStep(ctx, workflow.ID, &step); err != nil {
			return fmt.Errorf("failed to add step to workflow: %w", err)
		}
	}

	return nil
}

// updateWorkflowFromYAML updates an existing workflow in the database from YAML definition
func (e *ToolEngine) updateWorkflowFromYAML(ctx context.Context, workflowID int64, yamlWorkflow tool_protocol.WorkflowYAML, toolName string) error {
	workflowType := types.WorkflowTypeUser
	if yamlWorkflow.WorkflowType != "" {
		workflowType = types.WorkflowType(yamlWorkflow.WorkflowType)
	}

	workflow := types.Workflow{
		ID:                        workflowID,
		CategoryName:              yamlWorkflow.CategoryName,
		HumanReadableCategoryName: yamlWorkflow.HumanReadableCategoryName,
		Description:               yamlWorkflow.Description,
		HumanVerified:             false,
		WorkflowType:              workflowType,
	}

	if err := e.workflowRepo.UpdateWorkflow(ctx, &workflow); err != nil {
		return fmt.Errorf("failed to update workflow: %w", err)
	}

	existingSteps, err := e.workflowRepo.GetWorkflowSteps(ctx, workflowID)
	if err != nil {
		return fmt.Errorf("failed to get existing steps: %w", err)
	}

	for _, step := range existingSteps {
		if err := e.workflowRepo.DeleteStep(ctx, step.ID); err != nil {
			return fmt.Errorf("failed to delete existing step: %w", err)
		}
	}

	for _, yamlStep := range yamlWorkflow.Steps {
		step := types.WorkflowStep{
			WorkflowID:                 workflowID,
			Order:                      yamlStep.Order,
			Action:                     yamlStep.Action,
			HumanReadableActionName:    yamlStep.HumanReadableActionName,
			Rationale:                  yamlStep.Instructions,
			ExpectedOutcomeDescription: yamlStep.ExpectedOutcomeDescription,
		}
		if err := e.workflowRepo.AddStep(ctx, workflowID, &step); err != nil {
			return fmt.Errorf("failed to add step to workflow: %w", err)
		}
	}

	return nil
}
