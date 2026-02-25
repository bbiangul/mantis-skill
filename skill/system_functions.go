package skill

// System function names for agentic inference
// These are the canonical definitions of inference-specific system function names.
// pkg/tool_engine/models/constants.go imports these values to avoid duplication (DRY principle).
//
// These 7 functions are available for use in allowedSystemFunctions field in:
// - RunOnlyIfObject.AllowedSystemFunctions
// - SuccessCriteriaObject.AllowedSystemFunctions
const (
	SystemFunctionAskConversationHistory    = "askToTheConversationHistoryWithCustomer"
	SystemFunctionAskKnowledgeBase          = "askToKnowledgeBase"
	SystemFunctionAskToContext              = "askToContext"
	SystemFunctionDoDeepWebResearch         = "doDeepWebResearch"
	SystemFunctionDoSimpleWebSearch         = "doSimpleWebSearch"
	SystemFunctionGetWeekdayFromDate        = "getWeekdayFromDate"
	SystemFunctionQueryMemories             = "queryMemories"
	SystemFunctionFetchWebContent           = "fetchWebContent"
	SystemFunctionQueryCustomerServiceChats = "queryCustomerServiceChats"
	SystemFunctionAnalyzeImage              = "analyzeImage"
	SystemFunctionCreateMemory              = "createMemory"
	SystemFunctionSearchCodebase            = "searchCodebase"
	SystemFunctionQueryDocuments            = "queryDocuments"
)
