package models

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/bbiangul/mantis-skill/types"
)

// FilteredFunctionsInContextKey is the context key used to pass filtered function names
// for event querying. Value is []string of function names to include.
const FilteredFunctionsInContextKey = "filteredFunctionsInContextKey"

// Package-level logger (set by the engine at init time)
var logger types.Logger

// SetLogger sets the package-level logger
func SetLogger(l types.Logger) {
	logger = l
}

type CacheGroup string

const (
	CACHE_GROUPED_BY_MESSAGE_ID CacheGroup = "message_id"
	CACHE_GROUPED_BY_CLIENT_ID  CacheGroup = "client_id"
)

// todo: WorkflowEvent can be better defined to avoid data being nil. I mean, when the step is proposed we dont have the result, for example
type WorkflowPlanned struct {
	Category string
	IsNew    bool
	Steps    []types.WorkflowStep
}

type WorkflowEventType string

const (
	WorkflowEventTypePlanned                          WorkflowEventType = "planned"
	WorkflowEventTypeProposed                         WorkflowEventType = "proposed"
	WorkflowEventTypePausedDueHumanSupportRequired    WorkflowEventType = "pausedDueHumanSupportRequired"
	WorkflowEventTypePausedDueApprovalRequired        WorkflowEventType = "pausedDueApprovalRequired"
	WorkflowEventTypePausedDueMissingInput            WorkflowEventType = "pausedDueMissingInput"
	WorkflowEventTypePausedDueMissingUserConfirmation WorkflowEventType = "pausedDueMissingUserConfirmation"
	WorkflowEventTypeExecuted                         WorkflowEventType = "executed"
	WorkflowEventTypeFunctionExecuted                 WorkflowEventType = "functionExecuted"
	WorkflowEventTypeCompleted                        WorkflowEventType = "completed"
	WorkflowEventTypeDependencyExecuted               WorkflowEventType = "dependencyExecuted"
	WorkflowEventTypeInputFulfillDependencyExecuted   WorkflowEventType = "inputFulfillDependencyExecuted"
	WorkflowEventTypePaused                           WorkflowEventType = "paused"
	WorkflowEventTypeResumed                          WorkflowEventType = "resumed"
	WorkflowEventTypeStatePersisted                   WorkflowEventType = "statePersisted"
	WorkflowEventTypeStateRestored                    WorkflowEventType = "stateRestored"
	WorkflowEventTypeScratchpad                       WorkflowEventType = "scratchpad"
)

// ScratchpadEventStep is the special step name used for scratchpad events
// It can be filtered using "from: scratchpad" in YAML tool definitions
const ScratchpadEventStep = "__scratchpad__"

type WorkflowEvent struct {
	Key       string // a random key to identify the parent event
	Timestamp time.Time
	EventType WorkflowEventType
	Planned   WorkflowPlanned
	ClientID  string
	MessageID string
	Step      string
	Rationale string
	Result    string
	Error     string
	Inputs    string
	SubEvents []WorkflowEvent
	// Pause/Resume specific fields
	CheckpointID string                 `json:"checkpoint_id,omitempty"`
	PauseReason  string                 `json:"pause_reason,omitempty"`
	Details      map[string]interface{} `json:"details,omitempty"`
}

// WorkflowEventCache is an in-memory cache of workflow events
type WorkflowEventCache struct {
	eventsByMessageID map[string][]WorkflowEvent            // messageID -> events
	eventsByClientID  map[string]map[string][]WorkflowEvent // clientID -> messageID -> events
	mutex             sync.RWMutex
}

// NewWorkflowEventCache creates a new workflow event cache
func NewWorkflowEventCache() *WorkflowEventCache {
	return &WorkflowEventCache{
		eventsByMessageID: make(map[string][]WorkflowEvent),
		eventsByClientID:  make(map[string]map[string][]WorkflowEvent),
	}
}

// AddEvent adds an event to the cache, replacing any existing event with the same Step value
func (c *WorkflowEventCache) AddEvent(event WorkflowEvent) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	// Initialize maps if they don't exist
	if _, exists := c.eventsByMessageID[event.MessageID]; !exists {
		c.eventsByMessageID[event.MessageID] = make([]WorkflowEvent, 0)
	}

	if _, exists := c.eventsByClientID[event.ClientID]; !exists {
		c.eventsByClientID[event.ClientID] = make(map[string][]WorkflowEvent)
	}

	if _, exists := c.eventsByClientID[event.ClientID][event.MessageID]; !exists {
		c.eventsByClientID[event.ClientID][event.MessageID] = make([]WorkflowEvent, 0)
	}

	// Handle event replacement in messageID index
	isReplaced := false
	if event.Step != "" {
		// Check if an event with the same Step already exists in messageID index
		for i, existingEvent := range c.eventsByMessageID[event.MessageID] {
			if existingEvent.Step == event.Step && existingEvent.EventType == event.EventType {
				// Replace the existing event
				c.eventsByMessageID[event.MessageID][i] = event
				isReplaced = true
				break
			}
		}
	}

	// If not replaced, append to messageID index
	if !isReplaced {
		c.eventsByMessageID[event.MessageID] = append(c.eventsByMessageID[event.MessageID], event)
	}

	// Now handle the clientID index
	isReplacedInClient := false
	if event.Step != "" {
		// Check if an event with the same Step already exists in clientID index
		for i, existingEvent := range c.eventsByClientID[event.ClientID][event.MessageID] {
			if existingEvent.Step == event.Step && existingEvent.EventType == event.EventType {
				// Replace the existing event
				c.eventsByClientID[event.ClientID][event.MessageID][i] = event
				isReplacedInClient = true
				break
			}
		}
	}

	// If not replaced, append to clientID index
	if !isReplacedInClient {
		c.eventsByClientID[event.ClientID][event.MessageID] = append(
			c.eventsByClientID[event.ClientID][event.MessageID], event)
	}
}

// GetEvents returns events based on the grouping parameter
func (c *WorkflowEventCache) GetEvents(id string, group CacheGroup) []WorkflowEvent {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	if group == CACHE_GROUPED_BY_MESSAGE_ID {
		// Return events for a specific message ID
		if events, exists := c.eventsByMessageID[id]; exists {
			// Return a copy to avoid concurrent modification issues
			result := make([]WorkflowEvent, len(events))
			copy(result, events)
			return result
		}
	} else if group == CACHE_GROUPED_BY_CLIENT_ID {
		// Return all events for a client ID across all message IDs
		var allClientEvents []WorkflowEvent

		if messageMap, exists := c.eventsByClientID[id]; exists {
			for _, events := range messageMap {
				allClientEvents = append(allClientEvents, events...)
			}
		}

		return allClientEvents
	}

	return []WorkflowEvent{}
}

// String returns a string representation of the events based on the grouping parameter
func (c *WorkflowEventCache) String(ctx context.Context, id string, group CacheGroup, status WorkflowEventType) string {
	return c.StringWithContext(ctx, id, group, status)
}

// StringWithContext returns a string representation of the events with context-based filtering
func (c *WorkflowEventCache) StringWithContext(ctx context.Context, id string, group CacheGroup, status WorkflowEventType) string {
	events := c.GetEvents(id, group)

	if logger != nil {
		logger.Debugf("StringWithContext called: id=%s, group=%v, status=%s, retrieved %d events from cache", id, group, status, len(events))
	}

	// Check if there's a filter for specific functions
	var filteredFunctions map[string]bool
	if filteredFunctionsSlice, ok := ctx.Value(FilteredFunctionsInContextKey).([]string); ok && len(filteredFunctionsSlice) > 0 {
		if logger != nil {
			logger.Infof("Filtered functions in context: %v", filteredFunctionsSlice)
		}
		filteredFunctions = make(map[string]bool)
		for _, fn := range filteredFunctionsSlice {
			// Handle special "scratchpad" filter value - map it to the actual step name
			if fn == "scratchpad" {
				filteredFunctions[ScratchpadEventStep] = true
			} else {
				filteredFunctions[fn] = true
			}
		}
	}

	// Filter events if function filtering is active
	if filteredFunctions != nil {
		if logger != nil {
			logger.Debugf("Starting function filtering: %d total events before filtering", len(events))
		}

		// First, collect all events for the filtered functions
		var filteredEvents []WorkflowEvent
		for _, event := range events {
			// Include the event if its Step (function name) is in the filtered functions list
			if filteredFunctions[event.Step] {
				filteredEvents = append(filteredEvents, event)
			}
		}

		if logger != nil {
			logger.Debugf("After function name matching: %d events matched filter %v", len(filteredEvents), filteredFunctions)
		}

		// Keep all filtered events and sort by most recent first
		events = filteredEvents

		if logger != nil {
			logger.Debugf("Final filtered events count: %d", len(events))
		}
	}

	// Sort all events by most recent first (descending timestamp)
	sort.Slice(events, func(i, j int) bool {
		return events[i].Timestamp.After(events[j].Timestamp)
	})

	if len(events) == 0 {
		if group == CACHE_GROUPED_BY_MESSAGE_ID {
			return "No events found for message ID: " + id
		} else {
			return "No events found for client ID: " + id
		}
	}

	var builder strings.Builder
	if group == CACHE_GROUPED_BY_MESSAGE_ID {
		builder.WriteString(fmt.Sprintf("Workflow Events for Message ID: %s\n\n", id))
	} else {
		builder.WriteString(fmt.Sprintf("Workflow Events for Client ID: %s\n\n", id))
	}

	// Add note when function filtering is active
	if filteredFunctions != nil {
		builder.WriteString("NOTE: Showing all executions for filtered functions, ordered by most recent first.\n\n")
	}

	for i, event := range events {
		if status != "" && (event.EventType != status && event.EventType != WorkflowEventTypeFunctionExecuted) {
			continue
		}

		builder.WriteString(fmt.Sprintf("Event #%d - %s\n", i+1, event.Timestamp.Format(time.RFC3339)))

		builder.WriteString(fmt.Sprintf("Type: %s\n", event.EventType))
		builder.WriteString(fmt.Sprintf("Step: %s\n", event.Step))

		if event.Result != "" {
			builder.WriteString(fmt.Sprintf("Result output: %s\n", event.Result))
		}

		if event.Error != "" {
			builder.WriteString(fmt.Sprintf("Error: %s\n", event.Error))
		}

		builder.WriteString("\n_____\n")
	}

	return builder.String()
}

// GetEventsString returns a string representation of the events
func (c *WorkflowEventCache) GetEventsString(ctx context.Context, id string, group CacheGroup, status WorkflowEventType) string {
	return c.String(ctx, id, group, status)
}

// GetEventsStringWithContext returns a string representation of the events with context-based filtering
func (c *WorkflowEventCache) GetEventsStringWithContext(ctx context.Context, id string, group CacheGroup, status WorkflowEventType) string {
	return c.StringWithContext(ctx, id, group, status)
}

// Clear removes all events for a message ID
func (c *WorkflowEventCache) Clear(id string, group CacheGroup) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if group == CACHE_GROUPED_BY_MESSAGE_ID {
		// Get the client ID for this message (if it exists)
		events, exists := c.eventsByMessageID[id]
		if exists && len(events) > 0 {
			// For each client that has this message
			for _, event := range events {
				clientID := event.ClientID
				// Remove this message ID from the client's map
				if clientMap, clientExists := c.eventsByClientID[clientID]; clientExists {
					delete(clientMap, id)
				}
			}
		}
		// Remove from message ID index
		delete(c.eventsByMessageID, id)
	} else if group == CACHE_GROUPED_BY_CLIENT_ID {
		// Remove client and all its messages
		if clientMap, exists := c.eventsByClientID[id]; exists {
			// For each message ID in this client's map
			for messageID := range clientMap {
				// Remove events from this client from the message ID index
				newMessageEvents := make([]WorkflowEvent, 0)
				for _, event := range c.eventsByMessageID[messageID] {
					if event.ClientID != id {
						newMessageEvents = append(newMessageEvents, event)
					}
				}
				// If there are still events for this message, update the map
				if len(newMessageEvents) > 0 {
					c.eventsByMessageID[messageID] = newMessageEvents
				} else {
					// Otherwise, delete the message ID entry
					delete(c.eventsByMessageID, messageID)
				}
			}
		}
		// Remove from client ID index
		delete(c.eventsByClientID, id)
	}
}

// ClearAll removes all events from the cache
func (c *WorkflowEventCache) ClearAll() {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.eventsByMessageID = make(map[string][]WorkflowEvent)
	c.eventsByClientID = make(map[string]map[string][]WorkflowEvent)
}

// CleanupStaleEntries removes entries older than the specified duration
// This is called by background cleanup to free memory for completed workflows
func (c *WorkflowEventCache) CleanupStaleEntries(maxAge time.Duration) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	cutoff := time.Now().Add(-maxAge)

	// Clean up messageID entries older than maxAge
	for messageID, events := range c.eventsByMessageID {
		if len(events) > 0 {
			// Check the last event timestamp (most recent activity)
			lastEvent := events[len(events)-1]
			if lastEvent.Timestamp.Before(cutoff) {
				delete(c.eventsByMessageID, messageID)
			}
		}
	}

	// Clean up clientID entries accordingly
	for clientID, messageMap := range c.eventsByClientID {
		for messageID, events := range messageMap {
			if len(events) > 0 {
				lastEvent := events[len(events)-1]
				if lastEvent.Timestamp.Before(cutoff) {
					delete(messageMap, messageID)
				}
			}
		}
		// Remove clientID entry if all messages are cleaned up
		if len(messageMap) == 0 {
			delete(c.eventsByClientID, clientID)
		}
	}
}
