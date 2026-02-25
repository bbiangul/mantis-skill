package models

import (
	"github.com/bbiangul/mantis-skill/skill"
)

// System function names used in the agentic coordinator
const (
	// Core system functions
	SystemFunctionRequestInternalTeamInfo      = "requestInternalTeamInfo"
	SystemFunctionRequestInternalTeamAction    = "requestInternalTeamAction"
	SystemFunctionAskToStructuredKnowledgeBase = "askToStructuredKnowledgeBase"
	SystemFunctionFindProperImages             = "findProperImages"
	// SystemFunctionCasualAnswering              = "casualAnswering"
	SystemFunctionGatherUserInformation = "gatherUserInformation"
	SystemFunctionTeach                 = "teach"

	// Inference-specific system functions (imported from skill to respect DRY)
	SystemFunctionAskConversationHistory    = skill.SystemFunctionAskConversationHistory
	SystemFunctionAskKnowledgeBase          = skill.SystemFunctionAskKnowledgeBase
	SystemFunctionAskToContext              = skill.SystemFunctionAskToContext
	SystemFunctionDoDeepWebResearch         = skill.SystemFunctionDoDeepWebResearch
	SystemFunctionDoSimpleWebSearch         = skill.SystemFunctionDoSimpleWebSearch
	SystemFunctionGetWeekdayFromDate        = skill.SystemFunctionGetWeekdayFromDate
	SystemFunctionQueryMemories             = skill.SystemFunctionQueryMemories
	SystemFunctionFetchWebContent           = skill.SystemFunctionFetchWebContent
	SystemFunctionQueryCustomerServiceChats = skill.SystemFunctionQueryCustomerServiceChats
	SystemFunctionAnalyzeImage              = skill.SystemFunctionAnalyzeImage
	SystemFunctionCreateMemory              = skill.SystemFunctionCreateMemory
	SystemFunctionSearchCodebase            = skill.SystemFunctionSearchCodebase
	SystemFunctionQueryDocuments            = skill.SystemFunctionQueryDocuments

	// Staff-specific system functions
	SystemFunctionUpdateAIAgentPersona = "updateAIAgentPersona"
	SystemFunctionSwitchPlatformMode   = "switchPlatformExecutionMode"
	//SystemFunctionRetrieveCustomerServiceKPIs = "retrieveCustomerServiceKPIs"
)

// System function descriptions
const (
	DescriptionRequestInternalTeamInfo      = "Request information from the internal team. Generally, information that does not change much, such as company policies, procedures, etc."
	DescriptionRequestInternalTeamAction    = "Request the internal team to perform an action. It can not be used if there is another tool that can do the same action. Use it only if the action is not possible to be done by any other tool."
	DescriptionAskConversationHistory       = "Search through the conversation history with the user. Useful to retrieve information that was already provided by the user in this conversation."
	DescriptionAskKnowledgeBase             = "Query the knowledge base of the company for relevant information. Useful to gather information for the company, like policies. USe it before asking the internal team."
	DescriptionAskToStructuredKnowledgeBase = "Query structured knowledge data of the company that was captured and stored by the team. This accesses specific knowledge items that were collected and processed as part of completed workflow steps, including documents, attachments, and structured information."
	DescriptionFindProperImages             = "Find relevant images to include in responses to improve quality and conversion rate. Use it when you want to include images to the response.For example, in a beauty salon, if the user asks for a specific type of haircut, the agent can find images of that haircut to include in the response."
	DescriptionCasualAnswering              = "Use this tool when the message does not require any tool or function to be executed. It is suitable for casual interactions, greetings, or messages that are simply conversational or not related to a specific task - no action needed."
	DescriptionGatherUserInformation        = "Use this tool to collect optional or supplemental details that can enhance or personalize the user's experience—such as upsell opportunities, extra preferences, or contextual nuances—without blocking the completion of the current task. It should only be invoked when the information is helpful but not strictly required. If a piece of information is essential to complete the task, output `done: {rationale}` instead."
	DescriptionTeach                        = "Teach the AI new knowledge about the company, processes, policies, or other relevant information. This tool allows the system to learn and update its knowledge base with new facts that can be used in future interactions. Dynamic data should not be taught using this tool, as it is not designed for real-time updates. Use it to teach static information that does not change frequently, such as company policies, procedures, etc."

	// Inference-specific descriptions
	DescriptionAskToContext              = "Ask to the context about the results of executed tools/steps and inputs fulfillment. Useful to gather information about the current execution chain and review the output of previous steps to correlate them or to supplement the information."
	DescriptionDoDeepWebResearch         = "Perform a deep web search to find information related to the input since it is not user personal data. Useful to discover and gather information from a web research. Useful for generating new content. Use only if needed, its a long process."
	DescriptionDoSimpleWebSearch         = "Perform a simple web search to find information related to the input since it is not user personal data. Useful to discover and gather snippets from a web search. Useful for confirming some thought when required. Use only if needed"
	DescriptionGetWeekdayFromDate        = "Get the weekday name (e.g., Monday, Tuesday) from a given date. Useful when you need to know what day of the week a specific date falls on."
	DescriptionQueryMemories             = "Query past agentic execution memories to understand previous actions, decisions, and outcomes. Useful to learn from past experiences, record previous requests or actions took, understand patterns, and make informed decisions based on what was done before in similar situations. ATTENTION: If some data is expected to be DYNAMIC, like data generated by external tools, the memory is just a support for understanding and pattern recognition BUT DO NOT replace the actual tool data retrieval."
	DescriptionFetchWebContent           = "Fetch and extract clean content from a web page URL using Jina.ai Reader. Returns the page content in Markdown format with auto-generated image captions. Useful for reading articles, documentation, blog posts, or any web content. Requires explicit opt-in via allowedSystemFunctions."
	DescriptionQueryCustomerServiceChats = "Query conversation histories from specific customer service chats by clientIDs. Staff-only function for reviewing customer interactions. Requires explicit opt-in via allowedSystemFunctions and clientIds parameter must be specified."
	DescriptionAnalyzeImage              = "Analyze image content from URLs to extract information relevant to fulfilling input parameters. Useful when images shared in conversation may contain data needed for the current task (e.g., product photos, screenshots, documents). Requires explicit opt-in via allowedSystemFunctions."
	DescriptionCreateMemory              = "Create a new memory entry to store important information from the current interaction. Useful for recording decisions, preferences, outcomes, and other contextual data that should be remembered for future interactions. The content will be persisted with optional topic categorization."
	DescriptionSearchCodebase            = "Search and explore project codebases using Claude Code in read-only (plan) mode. Useful for understanding existing implementations, finding relevant code patterns, and answering architecture questions. Requires explicit opt-in via allowedSystemFunctions and codebaseDirs must be specified."
	DescriptionQueryDocuments            = "Query ingested documents using hybrid RAG retrieval (vector + FTS + knowledge graph). Accepts a natural language question and returns relevant answers with source citations. Requires explicit opt-in via allowedSystemFunctions."

	// Staff-specific descriptions
	DescriptionUpdateAIAgentPersona        = "Updates the AI agent's persona (system prompt) with new information. Useful when the task gives instructions in how the agent should behave, for example, language, tone, etc."
	DescriptionSwitchPlatformMode          = "Switches the platform execution mode. The platform can be in two modes: 'live' or 'test'.\n\nThe test mode is used for testing purposes and does not reply to all customers. The live mode replies to all the customers. Useful when the task requires to switch the platform mode."
	DescriptionRetrieveCustomerServiceKPIs = "Fetches database metrics about users, started conversations, messages by time and tasks. It retrieves the metrics for the entire data range."
)
