package skill

import (
	"strings"
	"testing"
)

// Helper function to create ForEach with single shouldSkip condition for testing
func createForEachWithSingleShouldSkip(items string, shouldSkip *ShouldSkip) *ForEach {
	forEach := &ForEach{
		Items: items,
	}
	if shouldSkip != nil {
		forEach.ShouldSkip = []*ShouldSkip{shouldSkip}
	}
	return forEach
}

// Helper function to create ForEach with multiple shouldSkip conditions for testing
func createForEachWithMultipleShouldSkip(items string, conditions []*ShouldSkip) *ForEach {
	forEach := &ForEach{
		Items: items,
	}
	if len(conditions) > 0 {
		forEach.ShouldSkip = conditions
	}
	return forEach
}

func TestValidateShouldSkip(t *testing.T) {
	tests := []struct {
		name        string
		forEach     *ForEach
		stepName    string
		funcName    string
		toolName    string
		wantErr     bool
		expectedErr string
	}{
		{
			name:     "valid shouldSkip with name only",
			forEach:  createForEachWithSingleShouldSkip("$itemList", &ShouldSkip{Name: "checkFunction"}),
			stepName: "testStep",
			funcName: "testFunc",
			toolName: "testTool",
			wantErr:  false,
		},
		{
			name: "valid shouldSkip with params",
			forEach: createForEachWithSingleShouldSkip("$itemList", &ShouldSkip{
				Name: "checkFunction",
				Params: map[string]interface{}{
					"itemId": "$item.id",
					"status": "$item.status",
				},
			}),
			stepName: "testStep",
			funcName: "testFunc",
			toolName: "testTool",
			wantErr:  false,
		},
		{
			name:        "shouldSkip with empty name",
			forEach:     createForEachWithSingleShouldSkip("$itemList", &ShouldSkip{Name: ""}),
			stepName:    "testStep",
			funcName:    "testFunc",
			toolName:    "testTool",
			wantErr:     true,
			expectedErr: "has foreach shouldSkip with empty name field",
		},
		{
			name: "shouldSkip with breakIf - both allowed",
			forEach: func() *ForEach {
				f := createForEachWithSingleShouldSkip("$itemList", &ShouldSkip{Name: "checkFunction"})
				f.BreakIf = "$item.done == 'true'"
				return f
			}(),
			stepName: "testStep",
			funcName: "testFunc",
			toolName: "testTool",
			wantErr:  false,
		},
		{
			name: "shouldSkip with waitFor - both allowed",
			forEach: func() *ForEach {
				f := createForEachWithSingleShouldSkip("$itemList", &ShouldSkip{Name: "checkFunction"})
				f.WaitFor = &WaitFor{
					Name:                "waitFunction",
					PollIntervalSeconds: 5,
					MaxWaitingSeconds:   60,
				}
				return f
			}(),
			stepName: "testStep",
			funcName: "testFunc",
			toolName: "testTool",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateForEach(tt.forEach, tt.stepName, tt.funcName, tt.toolName)

			if tt.wantErr {
				if err == nil {
					t.Errorf("validateForEach() expected error but got none")
					return
				}
				if !strings.Contains(err.Error(), tt.expectedErr) {
					t.Errorf("validateForEach() error = %v, want error containing %v", err, tt.expectedErr)
				}
			} else {
				if err != nil {
					t.Errorf("validateForEach() unexpected error = %v", err)
				}
			}
		})
	}
}

func TestValidateShouldSkipMultiple(t *testing.T) {
	tests := []struct {
		name        string
		forEach     *ForEach
		stepName    string
		funcName    string
		toolName    string
		wantErr     bool
		expectedErr string
	}{
		{
			name: "valid multiple shouldSkip conditions",
			forEach: createForEachWithMultipleShouldSkip("$itemList", []*ShouldSkip{
				{Name: "checkProcessed"},
				{Name: "checkArchived"},
			}),
			stepName: "testStep",
			funcName: "testFunc",
			toolName: "testTool",
			wantErr:  false,
		},
		{
			name: "multiple shouldSkip with params",
			forEach: createForEachWithMultipleShouldSkip("$itemList", []*ShouldSkip{
				{Name: "checkProcessed", Params: map[string]interface{}{"id": "$item.id"}},
				{Name: "checkArchived", Params: map[string]interface{}{"status": "$item.status"}},
			}),
			stepName: "testStep",
			funcName: "testFunc",
			toolName: "testTool",
			wantErr:  false,
		},
		{
			name: "multiple shouldSkip with one empty name",
			forEach: createForEachWithMultipleShouldSkip("$itemList", []*ShouldSkip{
				{Name: "checkProcessed"},
				{Name: ""},
			}),
			stepName:    "testStep",
			funcName:    "testFunc",
			toolName:    "testTool",
			wantErr:     true,
			expectedErr: "shouldSkip[1] with empty name field",
		},
		{
			name: "three shouldSkip conditions",
			forEach: createForEachWithMultipleShouldSkip("$itemList", []*ShouldSkip{
				{Name: "checkA"},
				{Name: "checkB"},
				{Name: "checkC"},
			}),
			stepName: "testStep",
			funcName: "testFunc",
			toolName: "testTool",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateForEach(tt.forEach, tt.stepName, tt.funcName, tt.toolName)

			if tt.wantErr {
				if err == nil {
					t.Errorf("validateForEach() expected error but got none")
					return
				}
				if !strings.Contains(err.Error(), tt.expectedErr) {
					t.Errorf("validateForEach() error = %v, want error containing %v", err, tt.expectedErr)
				}
			} else {
				if err != nil {
					t.Errorf("validateForEach() unexpected error = %v", err)
				}
			}
		})
	}
}

func TestValidateShouldSkipFunctionsExist(t *testing.T) {
	// Create a test tool with some functions
	testTool := Tool{
		Name:        "testTool",
		Description: "Test tool",
		Version:     "1.0.0",
		Functions: []Function{
			{
				Name:        "existingFunction",
				Description: "An existing function",
				Operation:   "format",
			},
			{
				Name:        "anotherFunction",
				Description: "Another existing function",
				Operation:   "format",
			},
			{
				Name:        "thirdFunction",
				Description: "A third function",
				Operation:   "format",
			},
		},
	}

	tests := []struct {
		name        string
		forEach     *ForEach
		wantErr     bool
		expectedErr string
	}{
		{
			name:    "nil forEach",
			forEach: nil,
			wantErr: false,
		},
		{
			name:    "single shouldSkip with existing function",
			forEach: createForEachWithSingleShouldSkip("$items", &ShouldSkip{Name: "existingFunction"}),
			wantErr: false,
		},
		{
			name:        "single shouldSkip with non-existent function",
			forEach:     createForEachWithSingleShouldSkip("$items", &ShouldSkip{Name: "nonExistentFunction"}),
			wantErr:     true,
			expectedErr: "referencing non-existent function 'nonExistentFunction'",
		},
		{
			name:    "single shouldSkip with empty name",
			forEach: createForEachWithSingleShouldSkip("$items", &ShouldSkip{Name: ""}),
			wantErr: false, // Empty name is handled by validateForEach, not validateShouldSkipFunctionsExist
		},
		{
			name: "multiple shouldSkip all existing",
			forEach: createForEachWithMultipleShouldSkip("$items", []*ShouldSkip{
				{Name: "existingFunction"},
				{Name: "anotherFunction"},
			}),
			wantErr: false,
		},
		{
			name: "multiple shouldSkip one non-existent",
			forEach: createForEachWithMultipleShouldSkip("$items", []*ShouldSkip{
				{Name: "existingFunction"},
				{Name: "nonExistentFunction"},
			}),
			wantErr:     true,
			expectedErr: "shouldSkip[1] referencing non-existent function 'nonExistentFunction'",
		},
		{
			name: "multiple shouldSkip first non-existent",
			forEach: createForEachWithMultipleShouldSkip("$items", []*ShouldSkip{
				{Name: "nonExistentFunction"},
				{Name: "existingFunction"},
			}),
			wantErr:     true,
			expectedErr: "shouldSkip[0] referencing non-existent function 'nonExistentFunction'",
		},
		{
			name: "three shouldSkip all existing",
			forEach: createForEachWithMultipleShouldSkip("$items", []*ShouldSkip{
				{Name: "existingFunction"},
				{Name: "anotherFunction"},
				{Name: "thirdFunction"},
			}),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateShouldSkipFunctionsExist(tt.forEach, testTool, "testStep", "testFunc")

			if tt.wantErr {
				if err == nil {
					t.Errorf("validateShouldSkipFunctionsExist() expected error but got none")
					return
				}
				if !strings.Contains(err.Error(), tt.expectedErr) {
					t.Errorf("validateShouldSkipFunctionsExist() error = %v, want error containing %v", err, tt.expectedErr)
				}
			} else {
				if err != nil {
					t.Errorf("validateShouldSkipFunctionsExist() unexpected error = %v", err)
				}
			}
		})
	}
}

func TestShouldSkipStructParsing(t *testing.T) {
	yamlDef := `
version: v1
author: test

tools:
  - name: testTool
    description: Test tool with shouldSkip
    version: 1.0.0
    functions:
      - name: checkDealProcessed
        operation: format
        description: Check if deal is processed
        triggers:
          - type: flex_for_team
        input:
          - name: dealId
            description: Deal ID
            origin: inference
            shouldBeHandledAsMessageToUser: true
            successCriteria: "Generate deal ID"
            onError:
              strategy: inference
              message: Please provide deal ID
        steps: []
        output:
          type: string
          value: "true"

      - name: processDeals
        operation: db
        description: Process deals with shouldSkip
        triggers:
          - type: flex_for_team
        needs: [checkDealProcessed]
        input:
          - name: deals
            description: List of deals
            origin: chat
            onError:
              strategy: requestUserInput
              message: Please provide deals
        steps:
          - name: process
            action: select
            foreach:
              items: "$deals"
              itemVar: deal
              shouldSkip:
                name: checkDealProcessed
                params:
                  dealId: "$deal.id"
            with:
              select: "SELECT * FROM deals WHERE id = $deal.id"
            resultIndex: 1
        output:
          type: string
          value: "processing complete"
`

	result, err := CreateToolWithWarnings(yamlDef)
	if err != nil {
		t.Fatalf("CreateToolWithWarnings() error = %v", err)
	}

	// Verify the tool was parsed correctly
	if len(result.Tool.Tools) != 1 {
		t.Fatalf("Expected 1 tool, got %d", len(result.Tool.Tools))
	}

	tool := result.Tool.Tools[0]
	if len(tool.Functions) != 2 {
		t.Fatalf("Expected 2 functions, got %d", len(tool.Functions))
	}

	// Find processDeals function
	var processDealsFunc *Function
	for i := range tool.Functions {
		if tool.Functions[i].Name == "processDeals" {
			processDealsFunc = &tool.Functions[i]
			break
		}
	}

	if processDealsFunc == nil {
		t.Fatal("processDeals function not found")
	}

	if len(processDealsFunc.Steps) != 1 {
		t.Fatalf("Expected 1 step, got %d", len(processDealsFunc.Steps))
	}

	step := processDealsFunc.Steps[0]
	if step.ForEach == nil {
		t.Fatal("ForEach is nil")
	}

	// Use GetShouldSkipConditions to access the conditions
	conditions := step.ForEach.GetShouldSkipConditions()
	if len(conditions) != 1 {
		t.Fatalf("Expected 1 shouldSkip condition, got %d", len(conditions))
	}

	if conditions[0].Name != "checkDealProcessed" {
		t.Errorf("Expected shouldSkip.name = 'checkDealProcessed', got '%s'", conditions[0].Name)
	}

	if conditions[0].Params == nil {
		t.Fatal("ShouldSkip.Params is nil")
	}

	dealIdParam, ok := conditions[0].Params["dealId"]
	if !ok {
		t.Fatal("dealId param not found")
	}

	if dealIdParam != "$deal.id" {
		t.Errorf("Expected dealId param = '$deal.id', got '%v'", dealIdParam)
	}
}

func TestShouldSkipArrayParsing(t *testing.T) {
	yamlDef := `
version: v1
author: test

tools:
  - name: testTool
    description: Test tool with multiple shouldSkip conditions
    version: 1.0.0
    functions:
      - name: checkDealProcessed
        operation: format
        description: Check if deal is processed
        triggers:
          - type: flex_for_team
        input:
          - name: dealId
            description: Deal ID
            origin: inference
            successCriteria: "Extract deal ID"
            onError:
              strategy: inference
              message: Please provide deal ID
        steps: []
        output:
          type: string
          value: "true"

      - name: checkDealArchived
        operation: format
        description: Check if deal is archived
        triggers:
          - type: flex_for_team
        input:
          - name: dealId
            description: Deal ID
            origin: inference
            successCriteria: "Extract deal ID"
            onError:
              strategy: inference
              message: Please provide deal ID
        steps: []
        output:
          type: string
          value: "false"

      - name: processDeals
        operation: db
        description: Process deals with multiple shouldSkip conditions
        triggers:
          - type: flex_for_team
        needs: [checkDealProcessed, checkDealArchived]
        input:
          - name: deals
            description: List of deals
            origin: chat
            onError:
              strategy: requestUserInput
              message: Please provide deals
        steps:
          - name: process
            action: select
            foreach:
              items: "$deals"
              itemVar: deal
              shouldSkip:
                - name: checkDealProcessed
                  params:
                    dealId: "$deal.id"
                - name: checkDealArchived
                  params:
                    dealId: "$deal.id"
                    status: "$deal.status"
            with:
              select: "SELECT * FROM deals WHERE id = $deal.id"
            resultIndex: 1
        output:
          type: string
          value: "processing complete"
`

	result, err := CreateToolWithWarnings(yamlDef)
	if err != nil {
		t.Fatalf("CreateToolWithWarnings() error = %v", err)
	}

	// Verify the tool was parsed correctly
	if len(result.Tool.Tools) != 1 {
		t.Fatalf("Expected 1 tool, got %d", len(result.Tool.Tools))
	}

	tool := result.Tool.Tools[0]

	// Find processDeals function
	var processDealsFunc *Function
	for i := range tool.Functions {
		if tool.Functions[i].Name == "processDeals" {
			processDealsFunc = &tool.Functions[i]
			break
		}
	}

	if processDealsFunc == nil {
		t.Fatal("processDeals function not found")
	}

	step := processDealsFunc.Steps[0]
	if step.ForEach == nil {
		t.Fatal("ForEach is nil")
	}

	// Use GetShouldSkipConditions to access the conditions
	conditions := step.ForEach.GetShouldSkipConditions()
	if len(conditions) != 2 {
		t.Fatalf("Expected 2 shouldSkip conditions, got %d", len(conditions))
	}

	// Check first condition
	if conditions[0].Name != "checkDealProcessed" {
		t.Errorf("Expected first shouldSkip.name = 'checkDealProcessed', got '%s'", conditions[0].Name)
	}
	if conditions[0].Params["dealId"] != "$deal.id" {
		t.Errorf("Expected first shouldSkip dealId param = '$deal.id', got '%v'", conditions[0].Params["dealId"])
	}

	// Check second condition
	if conditions[1].Name != "checkDealArchived" {
		t.Errorf("Expected second shouldSkip.name = 'checkDealArchived', got '%s'", conditions[1].Name)
	}
	if conditions[1].Params["dealId"] != "$deal.id" {
		t.Errorf("Expected second shouldSkip dealId param = '$deal.id', got '%v'", conditions[1].Params["dealId"])
	}
	if conditions[1].Params["status"] != "$deal.status" {
		t.Errorf("Expected second shouldSkip status param = '$deal.status', got '%v'", conditions[1].Params["status"])
	}

	// Verify HasShouldSkip returns true
	if !step.ForEach.HasShouldSkip() {
		t.Error("HasShouldSkip() should return true")
	}
}

func TestShouldSkipWithNonExistentFunction(t *testing.T) {
	yamlDef := `
version: v1
author: test

tools:
  - name: testTool
    description: Test tool with invalid shouldSkip
    version: 1.0.0
    functions:
      - name: processDeals
        operation: db
        description: Process deals with invalid shouldSkip
        triggers:
          - type: flex_for_team
        input:
          - name: deals
            description: List of deals
            origin: chat
            onError:
              strategy: requestUserInput
              message: Please provide deals
        steps:
          - name: process
            action: select
            foreach:
              items: "$deals"
              itemVar: deal
              shouldSkip:
                name: nonExistentFunction
            with:
              select: "SELECT * FROM deals WHERE id = '$deal.id'"
            resultIndex: 1
        output:
          type: string
          value: "$result[1]"
`

	_, err := CreateToolWithWarnings(yamlDef)
	if err == nil {
		t.Fatal("Expected error for non-existent shouldSkip function, got none")
	}

	if !strings.Contains(err.Error(), "referencing non-existent function 'nonExistentFunction'") {
		t.Errorf("Expected error about non-existent function, got: %v", err)
	}
}

func TestShouldSkipArrayWithNonExistentFunction(t *testing.T) {
	yamlDef := `
version: v1
author: test

tools:
  - name: testTool
    description: Test tool with array shouldSkip having invalid function
    version: 1.0.0
    functions:
      - name: checkDealProcessed
        operation: format
        description: Check if deal is processed
        triggers:
          - type: flex_for_team
        input:
          - name: dealId
            description: Deal ID
            origin: inference
            successCriteria: "Extract deal ID"
            onError:
              strategy: inference
              message: Please provide deal ID
        steps: []
        output:
          type: string
          value: "true"

      - name: processDeals
        operation: db
        description: Process deals with shouldSkip array
        triggers:
          - type: flex_for_team
        needs: [checkDealProcessed]
        input:
          - name: deals
            description: List of deals
            origin: chat
            onError:
              strategy: requestUserInput
              message: Please provide deals
        steps:
          - name: process
            action: select
            foreach:
              items: "$deals"
              itemVar: deal
              shouldSkip:
                - name: checkDealProcessed
                  params:
                    dealId: "$deal.id"
                - name: nonExistentFunction
            with:
              select: "SELECT * FROM deals WHERE id = '$deal.id'"
            resultIndex: 1
        output:
          type: string
          value: "$result[1]"
`

	_, err := CreateToolWithWarnings(yamlDef)
	if err == nil {
		t.Fatal("Expected error for non-existent shouldSkip function in array, got none")
	}

	if !strings.Contains(err.Error(), "shouldSkip[1] referencing non-existent function 'nonExistentFunction'") {
		t.Errorf("Expected error about non-existent function at index 1, got: %v", err)
	}
}

func TestShouldSkipWithEmptyName(t *testing.T) {
	yamlDef := `
version: v1
author: test

tools:
  - name: testTool
    description: Test tool with empty shouldSkip name
    version: 1.0.0
    functions:
      - name: processDeals
        operation: db
        description: Process deals with empty shouldSkip name
        triggers:
          - type: flex_for_team
        input:
          - name: deals
            description: List of deals
            origin: chat
            onError:
              strategy: requestUserInput
              message: Please provide deals
        steps:
          - name: process
            action: select
            foreach:
              items: "$deals"
              itemVar: deal
              shouldSkip:
                name: ""
            with:
              select: "SELECT * FROM deals WHERE id = '$deal.id'"
            resultIndex: 1
        output:
          type: string
          value: "$result[1]"
`

	_, err := CreateToolWithWarnings(yamlDef)
	if err == nil {
		t.Fatal("Expected error for empty shouldSkip name, got none")
	}

	if !strings.Contains(err.Error(), "has foreach shouldSkip with empty name field") {
		t.Errorf("Expected error about empty name, got: %v", err)
	}
}

func TestShouldSkipArrayWithEmptyName(t *testing.T) {
	yamlDef := `
version: v1
author: test

tools:
  - name: testTool
    description: Test tool with array shouldSkip having empty name
    version: 1.0.0
    functions:
      - name: checkDealProcessed
        operation: format
        description: Check if deal is processed
        triggers:
          - type: flex_for_team
        input:
          - name: dealId
            description: Deal ID
            origin: inference
            successCriteria: "Extract deal ID"
            onError:
              strategy: inference
              message: Please provide deal ID
        steps: []
        output:
          type: string
          value: "true"

      - name: processDeals
        operation: db
        description: Process deals with shouldSkip array
        triggers:
          - type: flex_for_team
        needs: [checkDealProcessed]
        input:
          - name: deals
            description: List of deals
            origin: chat
            onError:
              strategy: requestUserInput
              message: Please provide deals
        steps:
          - name: process
            action: select
            foreach:
              items: "$deals"
              itemVar: deal
              shouldSkip:
                - name: checkDealProcessed
                  params:
                    dealId: "$deal.id"
                - name: ""
            with:
              select: "SELECT * FROM deals WHERE id = '$deal.id'"
            resultIndex: 1
        output:
          type: string
          value: "$result[1]"
`

	_, err := CreateToolWithWarnings(yamlDef)
	if err == nil {
		t.Fatal("Expected error for empty shouldSkip name in array, got none")
	}

	if !strings.Contains(err.Error(), "shouldSkip[1] with empty name field") {
		t.Errorf("Expected error about empty name at index 1, got: %v", err)
	}
}

func TestForEachHelperMethods(t *testing.T) {
	t.Run("GetShouldSkipConditions with nil", func(t *testing.T) {
		forEach := &ForEach{Items: "$items"}
		conditions := forEach.GetShouldSkipConditions()
		if conditions != nil {
			t.Errorf("Expected nil conditions, got %v", conditions)
		}
	})

	t.Run("GetShouldSkipConditions with single condition", func(t *testing.T) {
		forEach := createForEachWithSingleShouldSkip("$items", &ShouldSkip{Name: "check"})
		conditions := forEach.GetShouldSkipConditions()
		if len(conditions) != 1 {
			t.Errorf("Expected 1 condition, got %d", len(conditions))
		}
		if conditions[0].Name != "check" {
			t.Errorf("Expected name 'check', got '%s'", conditions[0].Name)
		}
	})

	t.Run("GetShouldSkipConditions with multiple conditions", func(t *testing.T) {
		forEach := createForEachWithMultipleShouldSkip("$items", []*ShouldSkip{
			{Name: "check1"},
			{Name: "check2"},
		})
		conditions := forEach.GetShouldSkipConditions()
		if len(conditions) != 2 {
			t.Errorf("Expected 2 conditions, got %d", len(conditions))
		}
	})

	t.Run("HasShouldSkip with nil", func(t *testing.T) {
		forEach := &ForEach{Items: "$items"}
		if forEach.HasShouldSkip() {
			t.Error("Expected HasShouldSkip() to return false for nil")
		}
	})

	t.Run("HasShouldSkip with conditions", func(t *testing.T) {
		forEach := createForEachWithSingleShouldSkip("$items", &ShouldSkip{Name: "check"})
		if !forEach.HasShouldSkip() {
			t.Error("Expected HasShouldSkip() to return true")
		}
	})
}

func TestBackwardsCompatibility(t *testing.T) {
	// Test that the old single-object syntax still works
	yamlDef := `
version: v1
author: test

tools:
  - name: testTool
    description: Test backwards compatibility
    version: 1.0.0
    functions:
      - name: checkFunc
        operation: format
        description: Check function
        triggers:
          - type: flex_for_team
        input:
          - name: itemId
            description: Item ID
            origin: inference
            successCriteria: "Extract item ID"
            onError:
              strategy: inference
              message: Please provide item ID
        steps: []
        output:
          type: string
          value: "true"

      - name: mainFunc
        operation: db
        description: Main function
        triggers:
          - type: flex_for_team
        needs: [checkFunc]
        input:
          - name: items
            description: Items
            origin: chat
            onError:
              strategy: requestUserInput
              message: Please provide items
        steps:
          - name: process
            action: select
            foreach:
              items: "$items"
              itemVar: item
              shouldSkip:
                name: checkFunc
            with:
              select: "SELECT * FROM items"
            resultIndex: 1
        output:
          type: string
          value: "done"
`

	result, err := CreateToolWithWarnings(yamlDef)
	if err != nil {
		t.Fatalf("Backwards compatibility test failed: %v", err)
	}

	// Find mainFunc
	var mainFunc *Function
	for i := range result.Tool.Tools[0].Functions {
		if result.Tool.Tools[0].Functions[i].Name == "mainFunc" {
			mainFunc = &result.Tool.Tools[0].Functions[i]
			break
		}
	}

	if mainFunc == nil {
		t.Fatal("mainFunc not found")
	}

	step := mainFunc.Steps[0]
	conditions := step.ForEach.GetShouldSkipConditions()

	if len(conditions) != 1 {
		t.Fatalf("Expected 1 condition for backwards compat, got %d", len(conditions))
	}

	if conditions[0].Name != "checkFunc" {
		t.Errorf("Expected condition name 'checkFunc', got '%s'", conditions[0].Name)
	}
}
