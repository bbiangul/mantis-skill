package engine

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/bbiangul/mantis-skill/engine/models"
	"github.com/bbiangul/mantis-skill/skill"
)

// ---------------------------------------------------------------------------
// Provider interfaces for human response handling
// ---------------------------------------------------------------------------

// HumanResponseToolRepo provides access to pending tool execution states.
type HumanResponseToolRepo interface {
	GetPendingToolExecutionStates(ctx context.Context) ([]*skill.ToolExecutionState, error)
}

// HumanResponseMessageManager provides message retrieval and updating.
type HumanResponseMessageManager interface {
	GetMessageByID(ctx context.Context, messageID string) (*HumanResponseMessage, error)
	UpdateMessage(ctx context.Context, message HumanResponseMessage) error
}

// HumanResponseMessage is a lightweight message representation used by the handler.
type HumanResponseMessage struct {
	ID        string
	UserID    string
	Message   string
	Channel   string
	ChatID    string
	SessionID string
}

// HumanResponseLLM provides LLM capabilities for extraction and evaluation.
type HumanResponseLLM interface {
	// ExtractParameterValue uses LLM to extract a parameter value from a response message.
	// Returns the extracted value or error if extraction failed.
	ExtractParameterValue(ctx context.Context, paramName, paramDescription, originalRequest, responseMessage string) (string, error)

	// IsMessageRelated uses LLM to determine if a response is related to the original request.
	// Returns (isRelated, rationale).
	IsMessageRelated(ctx context.Context, question, answer string) (bool, string)

	// GenerateClarification generates a follow-up clarification message.
	GenerateClarification(ctx context.Context, originalRequest, responseMessage, errorReason, paramName, paramDescription, language string) (string, error)
}

// HumanResponseMessageSender sends messages back to users.
type HumanResponseMessageSender interface {
	SendMessage(ctx context.Context, userID, channel, chatID, sessionID, message string) error
}

// ---------------------------------------------------------------------------
// ToolResponseHandler
// ---------------------------------------------------------------------------

// ToolResponseHandler manages responses from human tools and updates tool execution state.
type ToolResponseHandler struct {
	repository     HumanResponseToolRepo
	messageManager HumanResponseMessageManager
	inputFulfiller models.IInputFulfiller
	llm            HumanResponseLLM
	messageSender  HumanResponseMessageSender
	language       string // default language for clarification messages
}

// NewToolResponseHandler creates a new ToolResponseHandler.
func NewToolResponseHandler(
	repository HumanResponseToolRepo,
	messageManager HumanResponseMessageManager,
	inputFulfiller models.IInputFulfiller,
	llm HumanResponseLLM,
	messageSender HumanResponseMessageSender,
) *ToolResponseHandler {
	return &ToolResponseHandler{
		repository:     repository,
		messageManager: messageManager,
		inputFulfiller: inputFulfiller,
		llm:            llm,
		messageSender:  messageSender,
		language:       "English",
	}
}

// SetLanguage configures the default language for clarification messages.
func (h *ToolResponseHandler) SetLanguage(lang string) {
	if lang != "" {
		h.language = lang
	}
}

// CheckForToolResponse checks if a message is a response to a pending tool execution.
func (h *ToolResponseHandler) CheckForToolResponse(ctx context.Context, msg HumanResponseMessage) (isToolResponse bool, err error) {
	pendingStates, err := h.repository.GetPendingToolExecutionStates(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to get pending tool execution states: %w", err)
	}

	if len(pendingStates) == 0 {
		return false, nil
	}

	var matchingStates []*skill.ToolExecutionState

	for _, state := range pendingStates {
		originalMsg, err := h.messageManager.GetMessageByID(ctx, state.ResponseMessageID)
		if err != nil {
			if logger != nil {
				logger.Warnf("Failed to get original message %s: %v", state.ResponseMessageID, err)
			}
			continue
		}

		isRelated, rationale := h.llm.IsMessageRelated(ctx, originalMsg.Message, msg.Message)
		if isRelated {
			if logger != nil {
				logger.Infof("Found matching request: %s. Rationale: %s", state.ResponseMessageID, rationale)
			}
			matchingStates = append(matchingStates, state)
		}
	}

	if len(matchingStates) == 0 {
		return false, nil
	}

	for _, state := range matchingStates {
		originalMsg, err := h.messageManager.GetMessageByID(ctx, state.ResponseMessageID)
		if err != nil {
			if logger != nil {
				logger.Warnf("Failed to get original message %s: %v", state.ResponseMessageID, err)
			}
			continue
		}

		value, err := h.llm.ExtractParameterValue(ctx, state.InputName, state.InputDescription, originalMsg.Message, msg.Message)
		if err != nil {
			clarErr := h.requestClarification(ctx, msg, state, originalMsg, err.Error())
			if clarErr != nil {
				if logger != nil {
					logger.Warnf("Failed to request clarification for state %d: %v", state.ID, clarErr)
				}
			}
			continue
		}

		resolveErr := h.inputFulfiller.ResolveToolExecution(ctx, state.ID, value)
		if resolveErr != nil {
			if logger != nil {
				logger.Warnf("Failed to resolve tool execution for state %d: %v", state.ID, resolveErr)
			}
			continue
		}

		ackErr := h.sendAcknowledgment(ctx, msg, state, value)
		if ackErr != nil {
			if logger != nil {
				logger.Warnf("Failed to send acknowledgment for state %d: %v", state.ID, ackErr)
			}
		}
	}

	return len(matchingStates) > 0, nil
}

func (h *ToolResponseHandler) requestClarification(
	ctx context.Context,
	responseMsg HumanResponseMessage,
	state *skill.ToolExecutionState,
	originalMsg *HumanResponseMessage,
	errorReason string,
) error {
	if h.llm == nil {
		return errors.New("LLM provider not configured")
	}

	clarificationMsg, err := h.llm.GenerateClarification(
		ctx,
		originalMsg.Message,
		responseMsg.Message,
		errorReason,
		state.InputName,
		state.InputDescription,
		h.language,
	)
	if err != nil {
		// Use a fallback message
		clarificationMsg = fmt.Sprintf("Thank you for your response, but I still need more specific information about %s. Could you please provide exactly the %s?",
			state.InputName,
			state.InputDescription)
	}

	return h.sendClarificationMessage(ctx, responseMsg, clarificationMsg)
}

func (h *ToolResponseHandler) sendClarificationMessage(
	ctx context.Context,
	responseMsg HumanResponseMessage,
	clarificationMsg string,
) error {
	if h.messageSender == nil {
		return errors.New("message sender not configured")
	}

	return h.messageSender.SendMessage(
		ctx,
		responseMsg.UserID,
		responseMsg.Channel,
		responseMsg.ChatID,
		responseMsg.SessionID,
		clarificationMsg,
	)
}

func (h *ToolResponseHandler) sendAcknowledgment(
	ctx context.Context,
	responseMsg HumanResponseMessage,
	state *skill.ToolExecutionState,
	extractedValue string,
) error {
	acknowledgmentMsg := fmt.Sprintf("Thank you! I've received the %s: %s. I'll continue with processing the request.",
		state.InputName,
		extractedValue,
	)

	if h.messageSender == nil {
		return errors.New("message sender not configured")
	}

	return h.messageSender.SendMessage(
		ctx,
		responseMsg.UserID,
		responseMsg.Channel,
		responseMsg.ChatID,
		responseMsg.SessionID,
		acknowledgmentMsg,
	)
}

// ---------------------------------------------------------------------------
// Default LLM implementations (stubs — host overrides with real LLM calls)
// ---------------------------------------------------------------------------

// SimpleHumanResponseLLM provides basic keyword-based matching without LLM calls.
// Production systems should provide a real LLM-backed implementation.
type SimpleHumanResponseLLM struct{}

func (s *SimpleHumanResponseLLM) ExtractParameterValue(_ context.Context, _, _, _, responseMessage string) (string, error) {
	msg := strings.TrimSpace(responseMessage)
	if msg == "" {
		return "", errors.New("empty response message")
	}
	return msg, nil
}

func (s *SimpleHumanResponseLLM) IsMessageRelated(_ context.Context, _, answer string) (bool, string) {
	if strings.TrimSpace(answer) == "" {
		return false, "empty answer"
	}
	return true, "default: assuming all non-empty responses are related"
}

func (s *SimpleHumanResponseLLM) GenerateClarification(_ context.Context, _, _, _, paramName, paramDescription, _ string) (string, error) {
	_ = time.Now() // keep time import
	return fmt.Sprintf("Could you please provide more specific information about %s (%s)?", paramName, paramDescription), nil
}
