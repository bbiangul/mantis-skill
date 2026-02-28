package engine

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/bbiangul/mantis-skill/engine/models"
	"github.com/bbiangul/mantis-skill/types"

	"github.com/henomis/lingoose/thread"
)

// WorkflowService handles workflow-related operations
type WorkflowService struct {
	workflowRepo   types.WorkflowRepository
	toolEngine     models.IToolEngine
	inputFulfiller models.IInputFulfiller
	llm            LLMProvider
}

// NewWorkflowService creates a new WorkflowService
func NewWorkflowService(workflowRepo types.WorkflowRepository, toolEngine models.IToolEngine, inputFulfiller models.IInputFulfiller, llm LLMProvider) *WorkflowService {
	return &WorkflowService{
		workflowRepo:   workflowRepo,
		toolEngine:     toolEngine,
		inputFulfiller: inputFulfiller,
		llm:            llm,
	}
}

var _ models.IWorkflowService = (*WorkflowService)(nil)

// generateAndAppend calls the LLM provider with the thread and appends the response
// as an assistant message to the thread (mirroring the lingoose Generate pattern).
func (s *WorkflowService) generateAndAppend(ctx context.Context, messages *thread.Thread, opts LLMOptions) error {
	result, err := s.llm.GenerateWithThread(ctx, messages, opts)
	if err != nil {
		return err
	}
	if result != "" {
		messages.AddMessages(
			thread.NewAssistantMessage().AddContent(thread.NewTextContent(result)),
		)
	}
	return nil
}

func (s *WorkflowService) CategorizeUserMessage(ctx context.Context, userMessage, contextForAgent string, workflowType types.WorkflowType, allowWorkflowGeneration bool) (string, error) {
	// Get categories filtered by workflow type
	categories, err := s.workflowRepo.GetCategoriesByType(ctx, workflowType)
	if err != nil {
		return "", fmt.Errorf("failed to get categories: %w", err)
	}

	if len(categories) == 0 {
		// No categories yet, return NONE to trigger workflow generation
		return "NONE", nil
	}

	// Create a map of valid category names for quick validation
	validCategories := make(map[string]bool)
	var categoriesText strings.Builder
	for _, category := range categories {
		validCategories[category.CategoryName] = true
		categoriesText.WriteString(fmt.Sprintf("- %s: %s\n", category.CategoryName, category.Description))
	}

	// Create thread for LLM categorization
	messages := thread.New()

	// Build categorization instruction based on whether workflow generation is allowed
	var categorizationInstruction string
	if allowWorkflowGeneration {
		categorizationInstruction = `Based on the user message and conversation context, select the MOST appropriate category that matches the user's CURRENT intent. You must respond with ONLY the category_name or "NONE" if no category fits well. Do not add any explanation or additional text.`
	} else {
		categorizationInstruction = `Based on the user message and conversation context, select the MOST appropriate category that matches the user's CURRENT intent.
IMPORTANT: You MUST select one of the available categories. Only respond with "NONE" if the user message is completely unrelated to ALL categories (completely off-topic).
For most messages, pick the category that BEST fits, even if it's not a perfect match. Respond with ONLY the category_name. Do not add any explanation or additional text.`
	}

	systemPrompt := fmt.Sprintf(`You are a Task Categorization Assistant. Your job is to match a user message, given the context, to the most appropriate category from a predefined list, if you found one that makes sense.

**CRITICAL: ANALYZE THE CONVERSATION CONTEXT AND MESSAGES CAREFULLY**
The conversation context shows the recent conversation history. You MUST use this context to understand if:
1. There is an ONGOING workflow/operation that has NOT been completed yet
2. The user is modifying their choice WITHIN an ongoing workflow vs starting a COMPLETELY DIFFERENT workflow

**IMPORTANT DISTINCTION:**
- If the assistant is in the middle of a workflow (e.g., asking for confirmation, collecting information) and the user changes their mind about a detail BEFORE the operation is finalized, this is still part of the SAME workflow category - the user is just correcting/adjusting within the current flow.
- A "modification" workflow (like update, edit, change) typically applies to something that ALREADY EXISTS and was previously completed.
- A "creation" workflow includes the entire process until completion, even if the user adjusts details during the process.

**Examples:**
- Assistant asks "Confirm order of 3 items?" → User says "actually, make it 4" → Still the SAME workflow (creating order, not modifying existing order)
- Assistant asks "Which size do you want?" → User says "medium... wait, actually large" → Still the SAME workflow (user is adjusting before completion)
- User completed a purchase yesterday → User says "I want to change my order" → This is a DIFFERENT workflow (modifying something that already exists)

<available_categories>
%s
</available_categories>
<message_context>
%s
</message_context>

%s`, categoriesText.String(), contextForAgent, categorizationInstruction)

	messages.AddMessages(
		thread.NewSystemMessage().AddContent(thread.NewTextContent(systemPrompt)),
	)

	messages.AddMessages(
		thread.NewUserMessage().AddContent(thread.NewTextContent(userMessage)),
	)

	maxAttempts := 3
	var categoryName string

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		err = s.generateAndAppend(ctx, messages, LLMOptions{})
		if err != nil {
			return "", fmt.Errorf("error generating LLM categorization (attempt %d): %w", attempt, err)
		}

		if len(messages.LastMessage().Contents) == 0 {
			if attempt == maxAttempts {
				return "", errors.New("no categorization generated by LLM after multiple attempts")
			}

			// Add feedback and try again
			messages.AddMessages(
				thread.NewUserMessage().AddContent(thread.NewTextContent(
					"You did not provide a response. Please respond with only a category name from the list or 'NONE' if no category fits.")),
			)
			continue
		}

		// Extract and clean the category name
		categoryName = messages.LastMessage().Contents[0].AsString()
		categoryName = strings.TrimSpace(categoryName)

		// Validate the response
		if strings.ToUpper(categoryName) == "NONE" {
			break
		} else if validCategories[categoryName] {
			break
		} else {
			if attempt == maxAttempts {
				// After max attempts, default to NONE
				if logger != nil {
					logger.Warnf("LLM failed to return a valid category after %d attempts. Defaulting to 'NONE'", maxAttempts)
				}
				return "NONE", nil
			}

			// Add feedback for invalid category and try again
			var validCategoryNames []string
			for name := range validCategories {
				validCategoryNames = append(validCategoryNames, name)
			}

			feedback := fmt.Sprintf(
				"Your response '%s' is not a valid category. Please respond with ONLY one of the following categories: %s, or 'NONE' if no category fits.",
				categoryName,
				strings.Join(validCategoryNames, ", "),
			)

			messages.AddMessages(
				thread.NewUserMessage().AddContent(thread.NewTextContent(feedback)),
			)
		}
	}

	return categoryName, nil
}

// GenerateWorkflow creates a new workflow based on user message and available tools
// It handles cases where a workflow with the same category already exists
func (s *WorkflowService) GenerateWorkflow(ctx context.Context, userMessage, contextForAgent string, availableTools []string, workflowType types.WorkflowType) (*types.Workflow, error) {
	// First, generate the workflow without any existing context
	workflow, err := s.generateWorkflowInternal(ctx, userMessage, contextForAgent, availableTools, nil, workflowType)
	if err != nil {
		return nil, err
	}

	// Check if a workflow with this category already exists
	existingWorkflow, err := s.workflowRepo.GetWorkflowByCategory(ctx, workflow.CategoryName)
	if err == nil && existingWorkflow != nil {
		// Category already exists - reuse it instead of creating a new one
		if logger != nil {
			logger.Infof("Category '%s' already exists. Reusing existing workflow.", workflow.CategoryName)
		}

		if existingWorkflow.HumanVerified {
			// For human-verified workflows, we should not modify them
			if logger != nil {
				logger.Infof("Existing workflow for category '%s' is human-verified. Returning it as-is.", workflow.CategoryName)
			}
			return existingWorkflow, nil
		} else {
			// For non-human-verified workflows, ask the LLM to merge with the existing one
			if logger != nil {
				logger.Infof("Category '%s' already exists but is not human-verified. Merging workflows.",
					workflow.CategoryName)
			}

			// Get existing workflow steps
			existingSteps, err := s.workflowRepo.GetWorkflowSteps(ctx, existingWorkflow.ID)
			if err != nil {
				return nil, fmt.Errorf("failed to get existing workflow steps: %w", err)
			}
			existingWorkflow.Steps = existingSteps

			// Generate a merged workflow by providing both workflows to the LLM
			mergedWorkflow, err := s.mergeWorkflows(ctx, userMessage, contextForAgent, availableTools, existingWorkflow, workflow, workflowType)
			if err != nil {
				return nil, fmt.Errorf("failed to merge workflows: %w", err)
			}

			// Preserve the ID and update the workflow
			mergedWorkflow.ID = existingWorkflow.ID
			mergedWorkflow.Version = existingWorkflow.Version // The version will be incremented on update

			// Update the workflow in the database
			err = s.workflowRepo.UpdateWorkflow(ctx, mergedWorkflow)
			if err != nil {
				return nil, fmt.Errorf("failed to update existing workflow: %w", err)
			}

			// Delete all existing steps
			for _, step := range existingSteps {
				err = s.workflowRepo.DeleteStep(ctx, step.ID)
				if err != nil {
					return nil, fmt.Errorf("failed to delete existing workflow step: %w", err)
				}
			}

			// Add the new steps
			for i := range mergedWorkflow.Steps {
				err = s.workflowRepo.AddStep(ctx, existingWorkflow.ID, &mergedWorkflow.Steps[i])
				if err != nil {
					return nil, fmt.Errorf("failed to add merged workflow step: %w", err)
				}
			}

			// Fetch the updated workflow to ensure we have the correct state
			updatedWorkflow, err := s.workflowRepo.GetWorkflowByCategory(ctx, mergedWorkflow.CategoryName)
			if err != nil {
				return nil, fmt.Errorf("failed to fetch updated workflow: %w", err)
			}

			return updatedWorkflow, nil
		}
	}

	// If we got here, it's a new workflow or we've generated a unique name for it
	return workflow, nil
}

// mergeWorkflows uses the LLM to intelligently merge an existing workflow with a newly generated one
func (s *WorkflowService) mergeWorkflows(ctx context.Context, userMessage, contextForAgent string, availableTools []string,
	existingWorkflow, newWorkflow *types.Workflow, workflowType types.WorkflowType) (*types.Workflow, error) {

	// Create thread for LLM workflow merging
	messages := thread.New()

	// Build system prompt for workflow merging
	systemPrompt := `You are a Workflow Optimization Assistant. Your job is to merge two workflows into a single, coherent workflow that addresses the user request effectively.

You have:
1. An EXISTING WORKFLOW that was previously created but not yet verified by a human
2. A NEW WORKFLOW that was just generated for the current user request

Your task is to create a MERGED WORKFLOW that:
- Maintains the category name of the existing workflow
- Incorporates the best elements from both workflows - Or, if you think its the case, keep all the steps from both workflows
- Eliminates redundant or unnecessary steps
- Ensures all necessary steps are included to fulfill a complete workflow, given the category name. Remember, this is a guide, not necessarily the agent will execute all of them.
- Handles both success and exception paths
- Prioritizes steps in a logical sequence

A workflow consists of an ordered sequence of tool executions. Each step specifies:
1. The order number (starting from 0)
2. The tool to execute (action)
3. The reason why this tool should be executed at this step (rationale)
4. Expected outcome description

Format your response exactly as follows:

Category: <existing_category_name>
Human Category: <human_readable_category_name>
Description: <merged_description> (Improve or combine the descriptions)

Workflow:
[order:0] - [action:tool_name] - [human_action:Human Readable Action Name] - [rationale:detailed reason] - [expected_outcome:brief description]
[order:1] - [action:tool_name] - [human_action:Human Readable Action Name] - [rationale:detailed reason] - [expected_outcome:brief description]
...

Do NOT include any additional explanation. Output ONLY the category, human category, description, and workflow steps in exactly the format specified above.`

	messages.AddMessages(
		thread.NewSystemMessage().AddContent(thread.NewTextContent(systemPrompt)),
	)

	// Format both workflows as text
	existingWorkflowText := s.formatWorkflowAsText(existingWorkflow)
	newWorkflowText := s.formatWorkflowAsText(newWorkflow)

	// Build user message with both workflows and context
	userPrompt := fmt.Sprintf(`
<current_user_message> %s </current_user_message>

<message_context>
%s
</message_context>

<existing_workflow>
%s
</existing_workflow>

<new_workflow>
%s
</new_workflow>
<available_tools>
%s
</available_tools>

Please merge these workflows into a single, coherent workflow that effectively addresses the user request.`,
		userMessage, contextForAgent, existingWorkflowText, newWorkflowText, strings.Join(availableTools, "\n"))

	messages.AddMessages(
		thread.NewUserMessage().AddContent(thread.NewTextContent(userPrompt)),
	)

	// Validation loop
	maxAttempts := 3
	var mergedWorkflow *types.Workflow
	var err error

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		// Generate merged workflow
		err = s.generateAndAppend(ctx, messages, LLMOptions{})
		if err != nil {
			return nil, fmt.Errorf("error generating merged workflow (attempt %d): %w", attempt, err)
		}

		if len(messages.LastMessage().Contents) == 0 {
			if attempt == maxAttempts {
				return nil, errors.New("no merged workflow generated by LLM after multiple attempts")
			}

			// Add feedback and try again
			messages.AddMessages(
				thread.NewUserMessage().AddContent(thread.NewTextContent(
					"You did not provide a merged workflow. Please generate a workflow in the specified format that combines elements from both the existing and new workflows.")),
			)
			continue
		}

		// Parse the workflow from the LLM response
		mergedWorkflowText := messages.LastMessage().Contents[0].AsString()
		mergedWorkflow, err = s.parseWorkflowFromText(ctx, mergedWorkflowText, workflowType)

		if err != nil {
			if attempt == maxAttempts {
				// After max attempts, default to the new workflow
				if logger != nil {
					logger.Warnf("Failed to parse valid merged workflow after %d attempts, defaulting to new workflow", maxAttempts)
				}
				return newWorkflow, nil
			}

			// Add feedback about the parsing error and try again
			feedback := fmt.Sprintf(
				"Your response could not be parsed into a valid workflow: %s. Please provide a merged workflow in the specified format with category, description, and properly formatted steps.",
				err.Error(),
			)

			messages.AddMessages(
				thread.NewUserMessage().AddContent(thread.NewTextContent(feedback)),
			)
			continue
		}

		// Ensure category name matches the existing workflow
		if mergedWorkflow.CategoryName != existingWorkflow.CategoryName {
			if attempt == maxAttempts {
				// Force the correct category name
				mergedWorkflow.CategoryName = existingWorkflow.CategoryName
			} else {
				// Add feedback and try again
				messages.AddMessages(
					thread.NewUserMessage().AddContent(thread.NewTextContent(
						fmt.Sprintf("The merged workflow must keep the exact category name '%s'. Please fix this and try again.", existingWorkflow.CategoryName))),
				)
				continue
			}
		}

		// Additional validation for workflow content
		if len(mergedWorkflow.Steps) == 0 {
			if attempt == maxAttempts {
				// Default to the new workflow
				if logger != nil {
					logger.Warnf("Merged workflow generated without any steps after multiple attempts, defaulting to new workflow")
				}
				return newWorkflow, nil
			}

			messages.AddMessages(
				thread.NewUserMessage().AddContent(thread.NewTextContent(
					"The merged workflow must include at least one step. Please add steps in the format [order:0] - [action:tool_name] - [rationale:reason] - [expected_outcome:description]")),
			)
			continue
		}

		// Validate that tools in the workflow are in the available tools list
		var invalidTools []string
		availableToolsMap := make(map[string]bool)

		// Extract tool names from the available tools
		for _, toolDesc := range availableTools {
			parts := strings.Split(toolDesc, " - ")
			if len(parts) >= 1 {
				toolName := strings.TrimPrefix(parts[0], "tool_name: ")
				availableToolsMap[toolName] = true
			}
		}

		// Check system tools that are always available
		systemTools := []string{"requestInternalTeamInfo", "requestInternalTeamAction",
			"askToTheConversationHistoryWithCustomer", "askToKnowledgeBase", "findProperImages", "casualAnswering"}
		for _, tool := range systemTools {
			availableToolsMap[tool] = true
		}

		for _, step := range mergedWorkflow.Steps {
			if !availableToolsMap[step.Action] {
				invalidTools = append(invalidTools, step.Action)
			}
		}

		if len(invalidTools) > 0 {
			if attempt == maxAttempts {
				if logger != nil {
					logger.Warnf("Merged workflow contains unavailable tools %v, but will proceed", invalidTools)
				}
				break
			}

			feedback := fmt.Sprintf(
				"Your merged workflow contains tools that are not available: %s. Please use only the available tools: %s",
				strings.Join(invalidTools, ", "),
				strings.Join(availableTools, ", "),
			)

			messages.AddMessages(
				thread.NewUserMessage().AddContent(thread.NewTextContent(feedback)),
			)
			continue
		}

		// If we got here, the merged workflow is valid
		break
	}

	return mergedWorkflow, nil
}

// generateWorkflowInternal is the internal implementation of workflow generation
func (s *WorkflowService) generateWorkflowInternal(ctx context.Context, userMessage, contextForAgent string, availableTools []string, existingWorkflow *types.Workflow, workflowType types.WorkflowType) (*types.Workflow, error) {
	messages := thread.New()

	systemPrompt := `You are a Workflow Generator Assistant. Your job is to design an optimal step-by-step workflow to fulfill a user request.

A workflow consists of an ordered sequence of tool executions. Each step should specify:
1. The order number (starting from 0)
2. The tool to execute (action)
3. A human-readable name for the action
4. The reason why this tool should be executed at this step (rationale)
5. Expected outcome description

Your workflow should handle both success and exception paths. For each step, consider:
- What information or state change this step is trying to achieve
- What should happen if this step fails or returns unexpected results
- Alternative paths that might be needed depending on different scenarios
- How to handle missing information or errors

Format your response EXACTLY as follows:

Category: <suggested_category_name>
Human Category: <human_readable_category_name>
Description: <brief_description_of_this_workflow>

Workflow:
[order:0] - [action:tool_name] - [human_action:Human Readable Action Name] - [rationale:reason why this tool is executed first, considering possible exceptions] - [expected_outcome:brief description]
[order:1] - [action:tool_name] - [human_action:Human Readable Action Name] - [rationale:reason why this tool is executed second, addressing different possible outcomes from step 0] - [expected_outcome:brief description]
...

IMPORTANT:
- Category should be lowercase with underscores (e.g., "customer_support_inquiry")
- Human Category should be title case with spaces (e.g., "Customer Support Inquiry")
- Human Readable Action Names should be clear, descriptive titles (e.g., "Check Order Details", "Request Team Assistance")

Do NOT include any additional explanation. Output ONLY the category, human category, description, and workflow steps in exactly the format specified above. Think harder step by step, and do not skip any steps. Do not include parameters that belong to this user_message or context. This workflow will be used for future requests that might be similar to this one.`

	// If we have an existing workflow, add information about it
	if existingWorkflow != nil {
		systemPrompt += fmt.Sprintf(`

IMPORTANT: There is already a workflow with category "%s" that hasn't been verified by a human yet. You should generate a workflow that either:
1. Improves this existing workflow while keeping the same category name, or
2. Is completely different with a more appropriate category name.

Here is the existing workflow:
%s`, existingWorkflow.CategoryName, s.formatWorkflowAsText(existingWorkflow))
	}

	messages.AddMessages(
		thread.NewSystemMessage().AddContent(thread.NewTextContent(systemPrompt)),
	)

	// Build user message with available tools
	userPrompt := fmt.Sprintf(`<user_message> %s </user_message>
<message_context>
%s
</message_context>

<available_actions>
%s
</available_actions>

Generate an optimal workflow for handling this request. Make sure to include steps for handling errors, exceptions, and alternative paths.`, userMessage, contextForAgent, strings.Join(availableTools, "\n"))

	messages.AddMessages(
		thread.NewUserMessage().AddContent(thread.NewTextContent(userPrompt)),
	)

	maxAttempts := 3
	var workflow *types.Workflow
	var err error

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		// Generate workflow
		err = s.generateAndAppend(ctx, messages, LLMOptions{})
		if err != nil {
			return nil, fmt.Errorf("error generating workflow (attempt %d): %w", attempt, err)
		}

		if len(messages.LastMessage().Contents) == 0 {
			if attempt == maxAttempts {
				return nil, errors.New("no workflow generated by LLM after multiple attempts")
			}

			// Add feedback and try again
			messages.AddMessages(
				thread.NewUserMessage().AddContent(thread.NewTextContent(
					"You did not provide a workflow. Please generate a workflow in the specified format.")),
			)
			continue
		}

		// Parse the workflow from the LLM response
		workflowText := messages.LastMessage().Contents[0].AsString()
		workflow, err = s.parseWorkflowFromText(ctx, workflowText, workflowType)

		if err != nil {
			if attempt == maxAttempts {
				return nil, fmt.Errorf("failed to parse valid workflow after %d attempts: %w", maxAttempts, err)
			}

			feedback := fmt.Sprintf(
				"Your response could not be parsed into a valid workflow: %s. Please provide a workflow in the specified format with category, description, and properly formatted steps.",
				err.Error(),
			)

			messages.AddMessages(
				thread.NewUserMessage().AddContent(thread.NewTextContent(feedback)),
			)
			continue
		}

		// Additional validation for workflow content
		if len(workflow.Steps) == 0 {
			if attempt == maxAttempts {
				return nil, errors.New("workflow generated without any steps after multiple attempts")
			}

			messages.AddMessages(
				thread.NewUserMessage().AddContent(thread.NewTextContent(
					"The workflow must include at least one step. Please add steps in the format [order:0] - [action:tool_name] - [rationale:reason] - [expected_outcome:description]")),
			)
			continue
		}

		// Validate that tools in the workflow are in the available tools list
		var invalidTools []string
		availableToolsMap := make(map[string]bool)

		for _, toolDesc := range availableTools {
			parts := strings.Split(toolDesc, " - ")
			if len(parts) >= 1 {
				toolName := strings.TrimPrefix(parts[0], "tool_name: ")
				availableToolsMap[toolName] = true
			}
		}

		systemTools := []string{"requestInternalTeamInfo", "requestInternalTeamAction",
			"askToTheConversationHistoryWithCustomer", "askToKnowledgeBase", "findProperImages", "casualAnswering"}
		for _, tool := range systemTools {
			availableToolsMap[tool] = true
		}

		for _, step := range workflow.Steps {
			if !availableToolsMap[step.Action] {
				invalidTools = append(invalidTools, step.Action)
			}
		}

		if len(invalidTools) > 0 {
			if attempt == maxAttempts {
				if logger != nil {
					logger.Warnf("Workflow contains unavailable tools %v, but will proceed", invalidTools)
				}
				break
			}

			feedback := fmt.Sprintf(
				"Your workflow contains tools that are not available: %s. Please use only the available tools: %s \n Remember, if you need to use a tool that is not available, you can use requestInternalTeamAction to delegate to a human to do.",
				strings.Join(invalidTools, ", "),
				strings.Join(availableTools, ", "),
			)

			messages.AddMessages(
				thread.NewUserMessage().AddContent(thread.NewTextContent(feedback)),
			)
			continue
		}

		// If we got here, the workflow is valid
		break
	}

	return workflow, nil
}

// GetWorkflowByCategory retrieves a workflow by its category name
func (s *WorkflowService) GetWorkflowByCategory(ctx context.Context, categoryName string) (*types.Workflow, error) {
	return s.workflowRepo.GetWorkflowByCategory(ctx, categoryName)
}

// GetAllCategories retrieves all available workflow categories
func (s *WorkflowService) GetAllCategories(ctx context.Context) ([]types.Workflow, error) {
	return s.workflowRepo.GetAllCategories(ctx)
}

// GetCategoriesByType retrieves workflow categories filtered by workflow type
func (s *WorkflowService) GetCategoriesByType(ctx context.Context, workflowType types.WorkflowType) ([]types.Workflow, error) {
	return s.workflowRepo.GetCategoriesByType(ctx, workflowType)
}

// SaveWorkflow saves a new workflow to the database
func (s *WorkflowService) SaveWorkflow(ctx context.Context, workflow *types.Workflow) error {
	return s.workflowRepo.CreateWorkflow(ctx, workflow)
}

// UpdateWorkflow updates an existing workflow
func (s *WorkflowService) UpdateWorkflow(ctx context.Context, workflow *types.Workflow) error {
	return s.workflowRepo.UpdateWorkflow(ctx, workflow)
}

// AddStepToWorkflow adds a new step to an existing workflow
func (s *WorkflowService) AddStepToWorkflow(ctx context.Context, workflowID int64, step *types.WorkflowStep) error {
	return s.workflowRepo.AddStep(ctx, workflowID, step)
}

// UpdateWorkflowStep updates an existing workflow step
func (s *WorkflowService) UpdateWorkflowStep(ctx context.Context, step *types.WorkflowStep) error {
	return s.workflowRepo.UpdateStep(ctx, step)
}

// DeleteWorkflowStep deletes a workflow step
func (s *WorkflowService) DeleteWorkflowStep(ctx context.Context, stepID int64) error {
	return s.workflowRepo.DeleteStep(ctx, stepID)
}

// parseWorkflowFromText parses a workflow from the text format generated by the LLM
func (s *WorkflowService) parseWorkflowFromText(ctx context.Context, workflowText string, workflowType types.WorkflowType) (*types.Workflow, error) {
	lines := strings.Split(workflowText, "\n")

	workflow := &types.Workflow{
		HumanVerified: false,
		Version:       1,
		WorkflowType:  workflowType,
		Steps:         make([]types.WorkflowStep, 0),
	}

	// Parse category, human category, and description
	for _, line := range lines {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "Category:") {
			workflow.CategoryName = strings.TrimSpace(strings.TrimPrefix(line, "Category:"))
		} else if strings.HasPrefix(line, "Human Category:") {
			workflow.HumanReadableCategoryName = strings.TrimSpace(strings.TrimPrefix(line, "Human Category:"))
		} else if strings.HasPrefix(line, "Description:") {
			workflow.Description = strings.TrimSpace(strings.TrimPrefix(line, "Description:"))
		} else if strings.HasPrefix(line, "[order:") && strings.Contains(line, "] - [action:") {
			// Parse workflow step
			step, err := s.parseWorkflowStep(line)
			if err != nil {
				if logger != nil {
					logger.Warnf("Failed to parse workflow step: %v", err)
				}
				continue
			}

			workflow.Steps = append(workflow.Steps, *step)
		}
	}

	// Validate the parsed workflow
	if workflow.CategoryName == "" {
		return nil, errors.New("no category name found in workflow text")
	}

	if len(workflow.Steps) == 0 {
		return nil, errors.New("no steps found in workflow text")
	}

	return workflow, nil
}

// parseWorkflowStep parses a single workflow step from a text line
func (s *WorkflowService) parseWorkflowStep(stepText string) (*types.WorkflowStep, error) {
	step := &types.WorkflowStep{}

	// Parse order
	orderStart := strings.Index(stepText, "[order:") + 7
	orderEnd := strings.Index(stepText[orderStart:], "]")
	if orderEnd == -1 {
		return nil, fmt.Errorf("invalid step format, missing order end: %s", stepText)
	}
	orderText := stepText[orderStart : orderStart+orderEnd]
	order, err := strconv.Atoi(orderText)
	if err != nil {
		return nil, fmt.Errorf("invalid order number: %w", err)
	}
	step.Order = order

	// Parse action
	actionStart := strings.Index(stepText, "[action:") + 8
	if actionStart == 7 {
		return nil, fmt.Errorf("invalid step format, missing action: %s", stepText)
	}
	actionEnd := strings.Index(stepText[actionStart:], "]")
	if actionEnd == -1 {
		return nil, fmt.Errorf("invalid step format, missing action end: %s", stepText)
	}
	step.Action = stepText[actionStart : actionStart+actionEnd]

	// Parse human_action
	humanActionStart := strings.Index(stepText, "[human_action:") + 14
	if humanActionStart != 13 { // 14 - 1 = 13, if found
		humanActionEnd := strings.Index(stepText[humanActionStart:], "]")
		if humanActionEnd != -1 {
			step.HumanReadableActionName = stepText[humanActionStart : humanActionStart+humanActionEnd]
		}
	}

	// Parse rationale
	rationaleStart := strings.Index(stepText, "[rationale:") + 11
	if rationaleStart == 10 {
		return nil, fmt.Errorf("invalid step format, missing rationale: %s", stepText)
	}
	rationaleEnd := strings.Index(stepText[rationaleStart:], "]")
	if rationaleEnd == -1 {
		return nil, fmt.Errorf("invalid step format, missing rationale end: %s", stepText)
	}
	step.Rationale = stepText[rationaleStart : rationaleStart+rationaleEnd]

	// Parse expected outcome (optional)
	expectedStart := strings.Index(stepText, "[expected_outcome:")
	if expectedStart != -1 {
		expectedStart += 18
		expectedEnd := strings.Index(stepText[expectedStart:], "]")
		if expectedEnd != -1 {
			step.ExpectedOutcomeDescription = stepText[expectedStart : expectedStart+expectedEnd]
		}
	}

	return step, nil
}

// UpdateWorkflowFromFeedback updates a workflow based on human feedback
func (s *WorkflowService) UpdateWorkflowFromFeedback(ctx context.Context, categoryName, feedback string) (*types.Workflow, error) {
	// Get the current workflow
	originalWorkflow, err := s.workflowRepo.GetWorkflowByCategory(ctx, categoryName)
	if err != nil {
		return nil, fmt.Errorf("failed to get workflow: %w", err)
	}

	// Format the current workflow as text
	workflowText := s.formatWorkflowAsText(originalWorkflow)

	// Create thread for LLM workflow adaptation
	messages := thread.New()

	systemPrompt := `You are a Workflow Optimization Assistant. Your job is to refine and improve existing workflows based on human feedback.

A workflow consists of an ordered sequence of tool executions. Each step specifies:
1. The order number (starting from 0)
2. The tool to execute (action)
3. The reason why this tool should be executed at this step (rationale)
4. (Optional) Expected outcome description

You will be given an existing workflow and human feedback about it. Your task is to revise the workflow according to the feedback while ensuring it remains coherent and complete. Make sure the updated workflow still handles any error cases and exception paths.

Format your response exactly as follows:

Category: <category_name> (keep the original unless feedback indicates it should change)
Human Category: <human_readable_category_name>
Description: <brief_description_of_this_workflow> (keep or improve the original)

Workflow:
[order:0] - [action:tool_name] - [human_action:Human Readable Action Name] - [rationale:detailed reason] - [expected_outcome:brief description]
[order:1] - [action:tool_name] - [human_action:Human Readable Action Name] - [rationale:detailed reason] - [expected_outcome:brief description]
...

Do NOT include any additional explanation. Output ONLY the category, human category, description, and workflow steps in exactly the format specified above.`

	messages.AddMessages(
		thread.NewSystemMessage().AddContent(thread.NewTextContent(systemPrompt)),
	)

	// Build user message with current workflow and feedback
	userPrompt := fmt.Sprintf(`<current_workflow>
%s
</current_workflow>

<human_feedback>
%s
</human_feedback>

Please revise the workflow based on this feedback.`, workflowText, feedback)

	messages.AddMessages(
		thread.NewUserMessage().AddContent(thread.NewTextContent(userPrompt)),
	)

	maxAttempts := 3
	var updatedWorkflow *types.Workflow

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		// Generate updated workflow
		err = s.generateAndAppend(ctx, messages, LLMOptions{})
		if err != nil {
			return nil, fmt.Errorf("error generating updated workflow (attempt %d): %w", attempt, err)
		}

		if len(messages.LastMessage().Contents) == 0 {
			if attempt == maxAttempts {
				return nil, errors.New("no updated workflow generated by LLM after multiple attempts")
			}

			// Add feedback and try again
			messages.AddMessages(
				thread.NewUserMessage().AddContent(thread.NewTextContent(
					"You did not provide an updated workflow. Please provide a revised workflow in the specified format.")),
			)
			continue
		}

		// Parse the updated workflow
		updatedWorkflowText := messages.LastMessage().Contents[0].AsString()
		updatedWorkflow, err = s.parseWorkflowFromText(ctx, updatedWorkflowText, originalWorkflow.WorkflowType)

		if err != nil {
			if attempt == maxAttempts {
				return nil, fmt.Errorf("failed to parse valid updated workflow after %d attempts: %w", maxAttempts, err)
			}

			parseFeedback := fmt.Sprintf(
				"Your response could not be parsed into a valid workflow: %s. Please provide a workflow in the specified format with category, description, and properly formatted steps.",
				err.Error(),
			)

			messages.AddMessages(
				thread.NewUserMessage().AddContent(thread.NewTextContent(parseFeedback)),
			)
			continue
		}

		// Additional validation for workflow content
		if len(updatedWorkflow.Steps) == 0 {
			if attempt == maxAttempts {
				return nil, errors.New("updated workflow generated without any steps after multiple attempts")
			}

			messages.AddMessages(
				thread.NewUserMessage().AddContent(thread.NewTextContent(
					"The updated workflow must include at least one step. Please add steps in the format [order:0] - [action:tool_name] - [rationale:reason] - [expected_outcome:description]")),
			)
			continue
		}

		// Make sure the category name is preserved unless explicitly changed in feedback
		if updatedWorkflow.CategoryName != originalWorkflow.CategoryName &&
			!strings.Contains(strings.ToLower(feedback), strings.ToLower("category")) {
			if attempt == maxAttempts {
				// Revert to original category name on final attempt
				updatedWorkflow.CategoryName = originalWorkflow.CategoryName
				break
			}

			categoryFeedback := fmt.Sprintf(
				"You changed the category name from '%s' to '%s', but the feedback did not request a category change. Please keep the original category name unless the feedback explicitly requests to change it.",
				originalWorkflow.CategoryName,
				updatedWorkflow.CategoryName,
			)

			messages.AddMessages(
				thread.NewUserMessage().AddContent(thread.NewTextContent(categoryFeedback)),
			)
			continue
		}

		// If we got here, the updated workflow is valid
		break
	}

	// Set ID and mark as human verified
	updatedWorkflow.ID = originalWorkflow.ID
	updatedWorkflow.HumanVerified = true

	// Save the updated workflow
	err = s.workflowRepo.UpdateWorkflow(ctx, updatedWorkflow)
	if err != nil {
		return nil, fmt.Errorf("failed to update workflow: %w", err)
	}

	// Delete old steps
	oldSteps, err := s.workflowRepo.GetWorkflowSteps(ctx, originalWorkflow.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get old workflow steps: %w", err)
	}

	for _, step := range oldSteps {
		err = s.workflowRepo.DeleteStep(ctx, step.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to delete old workflow step: %w", err)
		}
	}

	// Add new steps
	for i := range updatedWorkflow.Steps {
		err = s.workflowRepo.AddStep(ctx, originalWorkflow.ID, &updatedWorkflow.Steps[i])
		if err != nil {
			return nil, fmt.Errorf("failed to add updated workflow step: %w", err)
		}
	}

	return updatedWorkflow, nil
}

// formatWorkflowAsText formats a workflow as text
func (s *WorkflowService) formatWorkflowAsText(workflow *types.Workflow) string {
	var builder strings.Builder

	builder.WriteString(fmt.Sprintf("Category: %s\n", workflow.CategoryName))
	if workflow.HumanReadableCategoryName != "" {
		builder.WriteString(fmt.Sprintf("Human Category: %s\n", workflow.HumanReadableCategoryName))
	}
	builder.WriteString(fmt.Sprintf("Description: %s\n\n", workflow.Description))
	builder.WriteString("Workflow:\n")

	for _, step := range workflow.Steps {
		if step.ExpectedOutcomeDescription != "" {
			if step.HumanReadableActionName != "" {
				builder.WriteString(fmt.Sprintf("[order:%d] - [action:%s] - [human_action:%s] - [rationale:%s] - [expected_outcome:%s]\n",
					step.Order, step.Action, step.HumanReadableActionName, step.Rationale, step.ExpectedOutcomeDescription))
			} else {
				builder.WriteString(fmt.Sprintf("[order:%d] - [action:%s] - [rationale:%s] - [expected_outcome:%s]\n",
					step.Order, step.Action, step.Rationale, step.ExpectedOutcomeDescription))
			}
		} else {
			if step.HumanReadableActionName != "" {
				builder.WriteString(fmt.Sprintf("[order:%d] - [action:%s] - [human_action:%s] - [rationale:%s]\n",
					step.Order, step.Action, step.HumanReadableActionName, step.Rationale))
			} else {
				builder.WriteString(fmt.Sprintf("[order:%d] - [action:%s] - [rationale:%s]\n",
					step.Order, step.Action, step.Rationale))
			}
		}
	}

	return builder.String()
}

// GetWorkflowSteps retrieves all steps for a workflow
func (s *WorkflowService) GetWorkflowSteps(ctx context.Context, workflowID int64) ([]types.WorkflowStep, error) {
	return s.workflowRepo.GetWorkflowSteps(ctx, workflowID)
}

func (s *WorkflowService) GetWorkflowRepository(ctx context.Context) types.WorkflowRepository {
	return s.workflowRepo
}
