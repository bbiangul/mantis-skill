package engine

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

// ---------------------------------------------------------------------------
// BrowserAutomation provider — host applications inject a concrete impl.
// ---------------------------------------------------------------------------

// BrowserAutomationOptions configures a single browser automation operation.
type BrowserAutomationOptions struct {
	Task            string
	UseUserBrowser  bool
	JSONOutput      string
	Verbose         bool
	SuccessCriteria string
	KeepTempDir     bool
}

// BrowserAutomationClient executes browser automation operations.
type BrowserAutomationClient interface {
	ExecuteBrowserOperation(opts BrowserAutomationOptions) (interface{}, error)
}

// package-level browser client; must be set by the host before use.
var browserAutomationClient BrowserAutomationClient

// SetBrowserAutomationClient sets the global browser automation client.
func SetBrowserAutomationClient(client BrowserAutomationClient) {
	browserAutomationClient = client
}

// ---------------------------------------------------------------------------
// BrowserManager
// ---------------------------------------------------------------------------

// BrowserManager handles browser automation operations.
type BrowserManager struct {
	userBrowserQueue    chan browserQueueItem
	isolatedBrowserPool chan struct{}
	operationStatus     map[string]*operationStatus
	mutex               sync.RWMutex
}

type operationStatus struct {
	browser   string // "user" or "isolated"
	status    string // "queued", "running", "completed", "failed"
	queuePos  int
	queuedAt  time.Time
	startedAt time.Time
	endedAt   time.Time
}

type browserQueueItem struct {
	messageID       string
	operationID     string
	guide           string
	inputs          map[string]interface{}
	jsonSchema      string
	successCriteria string
	resultChan      chan browserQueueResult
}

type browserQueueResult struct {
	result interface{}
	err    error
}

// GlobalBrowserManager is the singleton browser manager.
// Initialised lazily via InitGlobalBrowserManager.
var GlobalBrowserManager *BrowserManager

// InitGlobalBrowserManager creates and starts the global browser manager.
// Safe to call multiple times; only the first call takes effect.
func InitGlobalBrowserManager() {
	if GlobalBrowserManager != nil {
		return
	}
	GlobalBrowserManager = NewBrowserManager(1, 5)
	go GlobalBrowserManager.Start()
}

// NewBrowserManager creates a new browser manager.
func NewBrowserManager(userWorkers, maxIsolatedBrowsers int) *BrowserManager {
	m := &BrowserManager{
		userBrowserQueue:    make(chan browserQueueItem, 100),
		isolatedBrowserPool: make(chan struct{}, maxIsolatedBrowsers),
		operationStatus:     make(map[string]*operationStatus),
	}
	go m.backgroundCleanup()
	return m
}

func (m *BrowserManager) backgroundCleanup() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		m.mutex.Lock()
		now := time.Now()
		for key, status := range m.operationStatus {
			if (status.status == "completed" || status.status == "failed") &&
				!status.endedAt.IsZero() &&
				now.Sub(status.endedAt) > 5*time.Minute {
				delete(m.operationStatus, key)
			}
		}
		m.mutex.Unlock()
	}
}

// Start begins processing browser operations.
func (m *BrowserManager) Start() {
	for i := 0; i < cap(m.userBrowserQueue); i++ {
		go m.userBrowserWorker()
	}
}

func (m *BrowserManager) userBrowserWorker() {
	for item := range m.userBrowserQueue {
		m.updateStatus(item.messageID, item.operationID, "running")

		result, err := executeUserBrowserOp(item.guide, item.inputs, item.jsonSchema, item.successCriteria)

		if err != nil {
			m.updateStatus(item.messageID, item.operationID, "failed")
		} else {
			m.updateStatus(item.messageID, item.operationID, "completed")
		}

		item.resultChan <- browserQueueResult{result: result, err: err}
		m.updateQueuePositions()
	}
}

// ExecuteBrowserOperation handles a browser operation, routing it appropriately.
func (m *BrowserManager) ExecuteBrowserOperation(
	ctx context.Context,
	messageID string,
	useUserBrowser bool,
	guide string,
	successCriteria string,
	inputs map[string]interface{},
	jsonSchema string,
) (interface{}, error) {
	operationID := fmt.Sprintf("%s-%d", messageID, time.Now().UnixNano())

	if useUserBrowser {
		return m.queueUserBrowserOperation(ctx, messageID, operationID, guide, inputs, jsonSchema, successCriteria)
	}
	return m.executeIsolatedBrowserOperation(ctx, messageID, operationID, guide, inputs, jsonSchema, successCriteria)
}

func (m *BrowserManager) queueUserBrowserOperation(
	ctx context.Context,
	messageID, operationID, guide string,
	inputs map[string]interface{},
	jsonSchema, successCriteria string,
) (interface{}, error) {
	if logger != nil {
		logger.Infof("Queueing operation for user's browser: %s", operationID)
	}

	resultChan := make(chan browserQueueResult, 1)

	item := browserQueueItem{
		messageID:       messageID,
		operationID:     operationID,
		guide:           guide,
		inputs:          inputs,
		jsonSchema:      jsonSchema,
		successCriteria: successCriteria,
		resultChan:      resultChan,
	}

	m.mutex.Lock()
	position := 0
	for _, status := range m.operationStatus {
		if status.browser == "user" && (status.status == "queued" || status.status == "running") {
			position++
		}
	}
	m.operationStatus[m.makeKey(messageID, operationID)] = &operationStatus{
		browser:  "user",
		status:   "queued",
		queuePos: position,
		queuedAt: time.Now(),
	}
	m.mutex.Unlock()

	m.userBrowserQueue <- item

	result := <-resultChan
	return result.result, result.err
}

func (m *BrowserManager) executeIsolatedBrowserOperation(
	ctx context.Context,
	messageID, operationID, guide string,
	inputs map[string]interface{},
	jsonSchema, successCriteria string,
) (interface{}, error) {
	if logger != nil {
		logger.Infof("Executing operation in isolated browser: %s", operationID)
	}

	m.mutex.Lock()
	m.operationStatus[m.makeKey(messageID, operationID)] = &operationStatus{
		browser:  "isolated",
		status:   "queued",
		queuedAt: time.Now(),
	}
	m.mutex.Unlock()

	m.isolatedBrowserPool <- struct{}{}
	m.updateStatus(messageID, operationID, "running")

	result, err := executeIsolatedBrowserOp(guide, inputs, jsonSchema, successCriteria)

	<-m.isolatedBrowserPool

	if err != nil {
		m.updateStatus(messageID, operationID, "failed")
	} else {
		m.updateStatus(messageID, operationID, "completed")
	}

	return result, err
}

// GetOperationStatus returns the current status of an operation.
func (m *BrowserManager) GetOperationStatus(messageID, operationID string) (map[string]interface{}, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	key := m.makeKey(messageID, operationID)
	status, exists := m.operationStatus[key]
	if !exists {
		return nil, errors.New("operation not found")
	}

	result := map[string]interface{}{
		"browser":  status.browser,
		"status":   status.status,
		"queuedAt": status.queuedAt,
	}

	if status.browser == "user" {
		result["queuePosition"] = status.queuePos
	}
	if !status.startedAt.IsZero() {
		result["startedAt"] = status.startedAt
		if status.status == "running" {
			result["runningDuration"] = time.Since(status.startedAt).String()
		}
	}
	if !status.endedAt.IsZero() {
		result["endedAt"] = status.endedAt
		result["totalDuration"] = status.endedAt.Sub(status.queuedAt).String()
		result["executionDuration"] = status.endedAt.Sub(status.startedAt).String()
	}

	return result, nil
}

func (m *BrowserManager) updateStatus(messageID, operationID, status string) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	key := m.makeKey(messageID, operationID)
	if opStatus, exists := m.operationStatus[key]; exists {
		opStatus.status = status
		switch status {
		case "running":
			opStatus.startedAt = time.Now()
		case "completed", "failed":
			opStatus.endedAt = time.Now()
		}
	}
}

func (m *BrowserManager) updateQueuePositions() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	position := 0
	for _, status := range m.operationStatus {
		if status.browser == "user" && status.status == "queued" {
			status.queuePos = position
			position++
		}
	}
}

func (m *BrowserManager) makeKey(messageID, operationID string) string {
	return fmt.Sprintf("%s:%s", messageID, operationID)
}

// ---------------------------------------------------------------------------
// Browser execution — delegate to the injected BrowserAutomationClient.
// ---------------------------------------------------------------------------

func executeUserBrowserOp(guide string, inputs map[string]interface{}, jsonOutputSchema string, successCriteria string) (interface{}, error) {
	if browserAutomationClient == nil {
		return nil, errors.New("browser automation client not configured")
	}

	options := BrowserAutomationOptions{
		Task:            guide,
		UseUserBrowser:  true,
		JSONOutput:      jsonOutputSchema,
		Verbose:         false,
		SuccessCriteria: successCriteria,
		KeepTempDir:     true,
	}

	result, err := browserAutomationClient.ExecuteBrowserOperation(options)
	if err != nil {
		if strings.Contains(err.Error(), "missing data in output") {
			return result, err
		}
		return nil, fmt.Errorf("failed to execute browser operation: %w", err)
	}

	return result, nil
}

func executeIsolatedBrowserOp(guide string, inputs map[string]interface{}, jsonOutputSchema string, successCriteria string) (interface{}, error) {
	if browserAutomationClient == nil {
		return nil, errors.New("browser automation client not configured")
	}

	options := BrowserAutomationOptions{
		Task:            guide,
		UseUserBrowser:  false,
		JSONOutput:      jsonOutputSchema,
		Verbose:         false,
		SuccessCriteria: successCriteria,
	}

	result, err := browserAutomationClient.ExecuteBrowserOperation(options)
	if err != nil {
		if strings.Contains(err.Error(), "missing data in output") {
			return result, err
		}
		return nil, fmt.Errorf("failed to execute browser operation: %w", err)
	}

	return result, nil
}
