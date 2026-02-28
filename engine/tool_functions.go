package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/bbiangul/mantis-skill/engine/models"
	"github.com/bbiangul/mantis-skill/skill"

	"github.com/henomis/lingoose/thread"
)

// Ensure toolFunctionsImpl satisfies the ToolFunctions interface.
var _ models.ToolFunctions = (*toolFunctionsImpl)(nil)

// ToolFunctionDeps holds the injectable dependencies for tool functions.
// The host application provides concrete implementations of these.
type ToolFunctionDeps struct {
	// RAG provides retrieval-augmented generation capabilities
	RAG RAGProvider
	// LLM provides language model inference
	LLM LLMProvider
	// Knowledge queries the knowledge base
	Knowledge KnowledgeProvider
	// Conversation queries conversation history
	Conversation ConversationProvider
	// HumanQA handles human Q&A interactions
	HumanQA HumanInteractionProvider
	// HumanAction handles human action requests
	HumanAction HumanInteractionProvider
	// ImageFinder finds relevant images
	ImageFinder ImageProvider
	// Research performs web research
	Research ResearchProvider
	// MessageSender sends messages to users/teams
	MessageSender MessageSender
	// Memory provides memory/recall capabilities
	Memory MemoryProvider
	// WebFetcher fetches web content
	WebFetcher WebFetchProvider
	// CodeSearch searches codebases
	CodeSearch CodeSearchProvider
	// DocumentQuery queries document stores
	DocumentQuery DocumentQueryProvider
	// Callback is the agentic workflow callback for event tracking
	Callback models.AgenticWorkflowCallback
	// Metrics provides KPI/metrics queries
	Metrics MetricsProvider
	// PersonaManager manages AI persona updates
	PersonaManager PersonaProvider
	// ImageAnalyzer analyzes images
	ImageAnalyzer ImageAnalyzerProvider
}

// ---------------------------------------------------------------------------
// Narrow provider interfaces — each covers a single responsibility.
// ---------------------------------------------------------------------------

// RAGProvider provides retrieval-augmented generation.
type RAGProvider interface {
	Query(ctx context.Context, query string) (string, error)
	QueryWithConversation(ctx context.Context, conversationHistory, outputFormat, query string) (string, error)
}

// KnowledgeProvider queries the knowledge base.
type KnowledgeProvider interface {
	Query(ctx context.Context, query string) (string, error)
	QueryStructured(ctx context.Context, query string, knowledgeKeys []string) (string, error)
}

// ConversationProvider queries conversation history.
type ConversationProvider interface {
	Query(ctx context.Context, query string) (string, error)
}

// HumanInteractionProvider handles human Q&A or action requests.
type HumanInteractionProvider interface {
	Execute(ctx context.Context, query string) (string, error)
}

// ImageProvider finds relevant images.
type ImageProvider interface {
	Find(ctx context.Context, query string) (string, error)
}

// ResearchProvider performs web research.
type ResearchProvider interface {
	DeepResearch(ctx context.Context, query, successCriteria string) (string, error)
	SimpleSearch(ctx context.Context, query string) (string, error)
}

// MemoryProvider provides memory/recall capabilities.
type MemoryProvider interface {
	Query(ctx context.Context, query string) (string, error)
	QueryWithFilters(ctx context.Context, query string, filters *skill.MemoryFilters) (string, error)
	Create(ctx context.Context, content, topic string) (string, error)
}

// WebFetchProvider fetches web content from URLs.
type WebFetchProvider interface {
	Fetch(ctx context.Context, url string) (string, error)
}

// CodeSearchProvider searches project codebases.
type CodeSearchProvider interface {
	Search(ctx context.Context, query string, dirs []string) (string, error)
}

// DocumentQueryProvider queries ingested document stores.
type DocumentQueryProvider interface {
	Query(ctx context.Context, query, dbName string, enableGraph bool) (string, error)
}

// MetricsProvider queries system metrics/KPIs.
type MetricsProvider interface {
	Query(ctx context.Context) (string, error)
}

// PersonaProvider manages AI persona changes.
type PersonaProvider interface {
	Change(ctx context.Context, rationale string) (string, error)
}

// ImageAnalyzerProvider analyzes images.
type ImageAnalyzerProvider interface {
	Analyze(ctx context.Context, rationale string) (string, error)
}

// AgenticEventsResponse represents a response from analyzing agentic event chunks.
type AgenticEventsResponse struct {
	Answer      string `json:"answer"`
	Confidence  string `json:"confidence"`
	Explanation string `json:"explanation"`
}

// AgenticEventsSnippetsResponse represents snippets extracted from event chunks.
type AgenticEventsSnippetsResponse struct {
	Snippets []string `json:"snippets"`
}

// toolFunctionsImpl provides implementations of the system functions for tools.
// All external dependencies are injected via ToolFunctionDeps.
type toolFunctionsImpl struct {
	deps ToolFunctionDeps
}

// NewToolFunctions creates a new ToolFunctions instance with the given dependencies.
func NewToolFunctions(deps ToolFunctionDeps) models.ToolFunctions {
	return &toolFunctionsImpl{deps: deps}
}

// Ask queries the knowledge base combined with conversation context.
func (f *toolFunctionsImpl) Ask(ctx context.Context, query string) (string, error) {
	if f.deps.RAG == nil {
		return "", fmt.Errorf("RAG provider not configured")
	}
	result, err := f.deps.RAG.QueryWithConversation(ctx, "", "", query)
	if err != nil {
		return fmt.Sprintf("error querying knowledge base: %s", err.Error()), nil
	}
	return result, nil
}

// Learn stores new knowledge.
func (f *toolFunctionsImpl) Learn(ctx context.Context, textOrMediaLink string) (string, error) {
	if strings.TrimSpace(textOrMediaLink) == "" {
		return "No content provided to learn from.", nil
	}
	if f.deps.RAG == nil {
		return "", fmt.Errorf("RAG provider not configured")
	}
	// Delegate to RAG for learning
	return f.deps.RAG.Query(ctx, textOrMediaLink)
}

// AskHuman sends a question to a human team member.
func (f *toolFunctionsImpl) AskHuman(ctx context.Context, query string) (string, error) {
	if f.deps.HumanQA == nil {
		return "", fmt.Errorf("human QA provider not configured")
	}
	return f.deps.HumanQA.Execute(ctx, query)
}

// RequestHumanAction requests a human to perform an action.
func (f *toolFunctionsImpl) RequestHumanAction(ctx context.Context, text string) (string, error) {
	if f.deps.HumanAction == nil {
		return "", fmt.Errorf("human action provider not configured")
	}
	return f.deps.HumanAction.Execute(ctx, text)
}

// RequestHumanApproval requests approval from a human team member.
func (f *toolFunctionsImpl) RequestHumanApproval(ctx context.Context, text string) error {
	if f.deps.HumanAction == nil {
		return fmt.Errorf("human action provider not configured")
	}
	_, err := f.deps.HumanAction.Execute(ctx, text)
	return err
}

// AskKnowledge queries the company knowledge base.
func (f *toolFunctionsImpl) AskKnowledge(ctx context.Context, query string) (string, error) {
	if f.deps.Knowledge == nil {
		return "", fmt.Errorf("knowledge provider not configured")
	}
	return f.deps.Knowledge.Query(ctx, query)
}

// AskStructuredKnowledge queries structured knowledge with specific keys.
func (f *toolFunctionsImpl) AskStructuredKnowledge(ctx context.Context, query string, knowledgeKeys []string) (string, error) {
	if f.deps.Knowledge == nil {
		return "", fmt.Errorf("knowledge provider not configured")
	}
	return f.deps.Knowledge.QueryStructured(ctx, query, knowledgeKeys)
}

// FindProperImages finds relevant images for a query.
func (f *toolFunctionsImpl) FindProperImages(ctx context.Context, query string) (string, error) {
	if f.deps.ImageFinder == nil {
		return "", fmt.Errorf("image provider not configured")
	}
	return f.deps.ImageFinder.Find(ctx, query)
}

// AskBasedOnConversation queries conversation history.
func (f *toolFunctionsImpl) AskBasedOnConversation(ctx context.Context, query string) (string, error) {
	if f.deps.Conversation == nil {
		return "", fmt.Errorf("conversation provider not configured")
	}
	return f.deps.Conversation.Query(ctx, query)
}

// NotifyHuman sends a notification to a human team member.
func (f *toolFunctionsImpl) NotifyHuman(ctx context.Context, text, role string) (string, error) {
	if f.deps.MessageSender == nil {
		return "", fmt.Errorf("message sender not configured")
	}
	err := f.deps.MessageSender.SendToTeam(ctx, text, role)
	if err != nil {
		return "", err
	}
	return "Notification sent successfully", nil
}

// SendTeamMessage sends a message to the team channel.
func (f *toolFunctionsImpl) SendTeamMessage(ctx context.Context, message, role string) (string, error) {
	if f.deps.MessageSender == nil {
		return "", fmt.Errorf("message sender not configured")
	}
	err := f.deps.MessageSender.SendToTeam(ctx, message, role)
	if err != nil {
		return "", err
	}
	return "Message sent to team successfully", nil
}

// SendMessageToUser sends a message to the user.
func (f *toolFunctionsImpl) SendMessageToUser(ctx context.Context, message string) error {
	if f.deps.MessageSender == nil {
		return fmt.Errorf("message sender not configured")
	}
	return f.deps.MessageSender.SendToUser(ctx, message)
}

// AskAboutAgenticEvents queries the execution chain context.
func (f *toolFunctionsImpl) AskAboutAgenticEvents(ctx context.Context, query string) (string, error) {
	// Try to get events from callback
	if f.deps.Callback == nil {
		return "No execution context available", nil
	}

	retrievedMsg, ok := ctx.Value(MessageInContextKey).(Message)
	if !ok {
		return "No message context available", nil
	}

	eventsString := f.deps.Callback.GetEventsStringWithContext(ctx, retrievedMsg.Id, models.CACHE_GROUPED_BY_MESSAGE_ID, models.WorkflowEventTypeExecuted)
	if eventsString == "" {
		return "No execution events found for this message", nil
	}

	// Use LLM to answer the query based on events
	if f.deps.LLM != nil {
		return f.processEventHistoryWithLLM(ctx, eventsString, query)
	}

	return eventsString, nil
}

// AskAboutAgenticEventsWithRag queries events with RAG support.
func (f *toolFunctionsImpl) AskAboutAgenticEventsWithRag(ctx context.Context, query string) (string, error) {
	return f.AskAboutAgenticEvents(ctx, query)
}

// DeepResearch performs deep web research.
func (f *toolFunctionsImpl) DeepResearch(ctx context.Context, query, finalResponse string) (string, error) {
	if f.deps.Research == nil {
		return "", fmt.Errorf("research provider not configured")
	}
	return f.deps.Research.DeepResearch(ctx, query, finalResponse)
}

// Search performs a simple web search.
func (f *toolFunctionsImpl) Search(ctx context.Context, query string) (string, error) {
	if f.deps.Research == nil {
		return "", fmt.Errorf("research provider not configured")
	}
	return f.deps.Research.SimpleSearch(ctx, query)
}

// ChangeAIPersona updates the AI agent's persona.
func (f *toolFunctionsImpl) ChangeAIPersona(ctx context.Context, rationale string) (string, error) {
	if f.deps.PersonaManager == nil {
		return "", fmt.Errorf("persona manager not configured")
	}
	return f.deps.PersonaManager.Change(ctx, rationale)
}

// QueryMetrics retrieves customer service KPIs.
func (f *toolFunctionsImpl) QueryMetrics(ctx context.Context) (string, error) {
	if f.deps.Metrics == nil {
		return "", fmt.Errorf("metrics provider not configured")
	}
	return f.deps.Metrics.Query(ctx)
}

// ChangeAnswerMode switches the platform execution mode.
func (f *toolFunctionsImpl) ChangeAnswerMode(ctx context.Context) (string, error) {
	return "Mode change not implemented in this deployment", nil
}

// Teach teaches the AI new knowledge.
func (f *toolFunctionsImpl) Teach(ctx context.Context, rationale string) (string, error) {
	return f.Learn(ctx, rationale)
}

// GetWeekdayFromDate returns the weekday for a given date string.
func (f *toolFunctionsImpl) GetWeekdayFromDate(ctx context.Context, dateStr string) (string, error) {
	dateStr = strings.TrimSpace(dateStr)
	if dateStr == "" {
		return "", fmt.Errorf("date string is empty")
	}

	// Try common date formats
	formats := []string{
		"2006-01-02",
		"02/01/2006",
		"01/02/2006",
		"2006-01-02T15:04:05Z",
		"2006-01-02 15:04:05",
		time.RFC3339,
		"January 2, 2006",
		"Jan 2, 2006",
		"02-01-2006",
	}

	for _, format := range formats {
		if t, err := time.Parse(format, dateStr); err == nil {
			return t.Weekday().String(), nil
		}
	}

	// If no format matched, try to use LLM to parse the date
	if f.deps.LLM != nil {
		result, err := f.deps.LLM.Generate(ctx, "You are a date parser. Extract the date and return only the weekday name (e.g., Monday, Tuesday).", dateStr, LLMOptions{})
		if err == nil && result != "" {
			return strings.TrimSpace(result), nil
		}
	}

	return "", fmt.Errorf("could not parse date: %s", dateStr)
}

// QueryMemories queries past execution memories.
func (f *toolFunctionsImpl) QueryMemories(ctx context.Context, query string) (string, error) {
	if f.deps.Memory == nil {
		return "", fmt.Errorf("memory provider not configured")
	}
	return f.deps.Memory.Query(ctx, query)
}

// QueryMemoriesWithFilters queries memories with optional filters.
func (f *toolFunctionsImpl) QueryMemoriesWithFilters(ctx context.Context, query string, filters *skill.MemoryFilters) (string, error) {
	if f.deps.Memory == nil {
		return "", fmt.Errorf("memory provider not configured")
	}
	return f.deps.Memory.QueryWithFilters(ctx, query, filters)
}

// CreateMemory creates a new memory entry.
func (f *toolFunctionsImpl) CreateMemory(ctx context.Context, content string, topic string) (string, error) {
	if f.deps.Memory == nil {
		return "", fmt.Errorf("memory provider not configured")
	}
	return f.deps.Memory.Create(ctx, content, topic)
}

// FetchWebContent fetches and extracts content from a URL.
func (f *toolFunctionsImpl) FetchWebContent(ctx context.Context, url string) (string, error) {
	if f.deps.WebFetcher == nil {
		return "", fmt.Errorf("web fetch provider not configured")
	}
	return f.deps.WebFetcher.Fetch(ctx, url)
}

// ProcessEventHistoryInChunksWithLLM processes event history in chunks using an LLM.
func (f *toolFunctionsImpl) ProcessEventHistoryInChunksWithLLM(ctx context.Context, eventHistory string, query string, llmToUse interface{}) (string, error) {
	return f.processEventHistoryWithLLM(ctx, eventHistory, query)
}

// ExtractSnippetsFromEventHistoryInChunksWithLLM extracts relevant snippets from event history.
func (f *toolFunctionsImpl) ExtractSnippetsFromEventHistoryInChunksWithLLM(ctx context.Context, eventHistory string, query string, llmToUse interface{}) (string, error) {
	return f.processEventHistoryWithLLM(ctx, eventHistory, query)
}

// QueryCustomerServiceChats queries customer service chat histories.
func (f *toolFunctionsImpl) QueryCustomerServiceChats(ctx context.Context, query string, clientIds []string) (string, error) {
	if f.deps.Conversation == nil {
		return "", fmt.Errorf("conversation provider not configured")
	}
	// Build a combined query with client IDs
	combinedQuery := query
	if len(clientIds) > 0 {
		combinedQuery = fmt.Sprintf("For clients %s: %s", strings.Join(clientIds, ", "), query)
	}
	return f.deps.Conversation.Query(ctx, combinedQuery)
}

// AnalyzeImage analyzes image content.
func (f *toolFunctionsImpl) AnalyzeImage(ctx context.Context, rationale string) (string, error) {
	if f.deps.ImageAnalyzer == nil {
		return "", fmt.Errorf("image analyzer not configured")
	}
	return f.deps.ImageAnalyzer.Analyze(ctx, rationale)
}

// SearchCodebase searches project codebases.
func (f *toolFunctionsImpl) SearchCodebase(ctx context.Context, query string, dirs []string) (string, error) {
	if f.deps.CodeSearch == nil {
		return "", fmt.Errorf("code search provider not configured")
	}
	return f.deps.CodeSearch.Search(ctx, query, dirs)
}

// QueryDocuments queries ingested documents.
func (f *toolFunctionsImpl) QueryDocuments(ctx context.Context, query string, dbName string, enableGraph bool) (string, error) {
	if f.deps.DocumentQuery == nil {
		return "", fmt.Errorf("document query provider not configured")
	}
	return f.deps.DocumentQuery.Query(ctx, query, dbName, enableGraph)
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

// processEventHistoryWithLLM uses the LLM to answer a query based on event history.
func (f *toolFunctionsImpl) processEventHistoryWithLLM(ctx context.Context, eventHistory string, query string) (string, error) {
	if f.deps.LLM == nil {
		return eventHistory, nil
	}

	// Split into chunks if too long (approx 4000 chars per chunk)
	chunkSize := 4000
	if len(eventHistory) <= chunkSize {
		return f.analyzeSingleChunk(ctx, eventHistory, query)
	}

	chunks := splitIntoChunks(eventHistory, chunkSize)
	var partialAnswers []AgenticEventsResponse

	for i, chunk := range chunks {
		resp, err := f.analyzeChunk(ctx, chunk, query, i+1, len(chunks))
		if err != nil {
			if logger != nil {
				logger.Warnf("Error analyzing chunk %d: %v", i+1, err)
			}
			continue
		}
		partialAnswers = append(partialAnswers, resp)
	}

	if len(partialAnswers) == 0 {
		return "Could not find relevant information in the execution history", nil
	}

	if len(partialAnswers) == 1 {
		return partialAnswers[0].Answer, nil
	}

	return f.combinePartialAnswers(ctx, partialAnswers, query)
}

func (f *toolFunctionsImpl) analyzeSingleChunk(ctx context.Context, eventHistory, query string) (string, error) {
	systemPrompt := `You are an AI assistant that analyzes execution event history. Answer the query based ONLY on the provided event data. If the information is not in the events, say so clearly.`

	userPrompt := fmt.Sprintf(`<event_history>
%s
</event_history>

<query>%s</query>

Analyze the event history and answer the query.`, eventHistory, query)

	result, err := f.deps.LLM.Generate(ctx, systemPrompt, userPrompt, LLMOptions{})
	if err != nil {
		return "", err
	}
	return result, nil
}

func (f *toolFunctionsImpl) analyzeChunk(ctx context.Context, chunk, query string, chunkNum, totalChunks int) (AgenticEventsResponse, error) {
	systemPrompt := fmt.Sprintf(`You are analyzing chunk %d of %d from an execution event history. Extract information relevant to the query and respond in JSON format: {"answer": "...", "confidence": "high/medium/low", "explanation": "..."}`, chunkNum, totalChunks)

	userPrompt := fmt.Sprintf(`<chunk>%s</chunk>
<query>%s</query>`, chunk, query)

	messages := thread.New()
	messages.AddMessages(
		thread.NewSystemMessage().AddContent(thread.NewTextContent(systemPrompt)),
		thread.NewUserMessage().AddContent(thread.NewTextContent(userPrompt)),
	)

	result, err := f.deps.LLM.Generate(ctx, systemPrompt, userPrompt, LLMOptions{})
	if err != nil {
		return AgenticEventsResponse{}, err
	}

	var resp AgenticEventsResponse
	if err := json.Unmarshal([]byte(result), &resp); err != nil {
		return AgenticEventsResponse{Answer: result, Confidence: "medium"}, nil
	}
	return resp, nil
}

func (f *toolFunctionsImpl) combinePartialAnswers(ctx context.Context, answers []AgenticEventsResponse, query string) (string, error) {
	var answersText strings.Builder
	for i, ans := range answers {
		answersText.WriteString(fmt.Sprintf("Chunk %d (confidence: %s): %s\n", i+1, ans.Confidence, ans.Answer))
	}

	systemPrompt := `You are synthesizing partial answers from different chunks of execution event history into a single coherent response. Prioritize high-confidence answers.`
	userPrompt := fmt.Sprintf(`<partial_answers>
%s
</partial_answers>

<original_query>%s</original_query>

Synthesize these partial answers into a single, coherent response.`, answersText.String(), query)

	result, err := f.deps.LLM.Generate(ctx, systemPrompt, userPrompt, LLMOptions{})
	if err != nil {
		// Fallback: return the highest confidence answer
		for _, ans := range answers {
			if ans.Confidence == "high" {
				return ans.Answer, nil
			}
		}
		return answers[0].Answer, nil
	}
	return result, nil
}

// splitIntoChunks splits text into chunks of approximately the given size.
func splitIntoChunks(text string, chunkSize int) []string {
	if len(text) <= chunkSize {
		return []string{text}
	}

	var chunks []string
	lines := strings.Split(text, "\n")
	var currentChunk strings.Builder

	for _, line := range lines {
		if currentChunk.Len()+len(line)+1 > chunkSize && currentChunk.Len() > 0 {
			chunks = append(chunks, currentChunk.String())
			currentChunk.Reset()
		}
		if currentChunk.Len() > 0 {
			currentChunk.WriteString("\n")
		}
		currentChunk.WriteString(line)
	}

	if currentChunk.Len() > 0 {
		chunks = append(chunks, currentChunk.String())
	}

	return chunks
}
