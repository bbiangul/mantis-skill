package skill

import (
	"strings"
	"testing"
)

func TestDetectRedundantDependencies_NoRedundancy(t *testing.T) {
	yamlContent := `
version: "v1"
author: "test"
tools:
  - name: "testTool"
    description: "Test tool"
    version: "1.0.0"
    functions:
      - name: "funcA"
        operation: "format"
        description: "Function A"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "output"
            origin: "inference"
            description: "test"
            successCriteria:
              condition: "test"
            onError:
              strategy: "requestUserInput"
              message: "Error"

      - name: "funcB"
        operation: "format"
        description: "Function B"
        triggers:
          - type: "flex_for_user"
        needs:
          - name: "funcA"
        input:
          - name: "output"
            origin: "inference"
            description: "test"
            successCriteria:
              condition: "test"
            onError:
              strategy: "requestUserInput"
              message: "Error"

      - name: "funcC"
        operation: "format"
        description: "Function C"
        triggers:
          - type: "flex_for_user"
        needs:
          - name: "funcB"
        input:
          - name: "output"
            origin: "inference"
            description: "test"
            successCriteria:
              condition: "test"
            onError:
              strategy: "requestUserInput"
              message: "Error"
`

	result, err := CreateToolWithWarnings(yamlContent)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if len(result.Warnings) != 0 {
		t.Errorf("Expected no warnings, got %d warnings: %v", len(result.Warnings), result.Warnings)
	}
}

func TestDetectRedundantDependencies_SimpleRedundancy(t *testing.T) {
	yamlContent := `
version: "v1"
author: "test"
tools:
  - name: "testTool"
    description: "Test tool"
    version: "1.0.0"
    functions:
      - name: "funcA"
        operation: "format"
        description: "Function A"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "output"
            origin: "inference"
            description: "test"
            successCriteria:
              condition: "test"
            onError:
              strategy: "requestUserInput"
              message: "Error"

      - name: "funcB"
        operation: "format"
        description: "Function B"
        triggers:
          - type: "flex_for_user"
        needs:
          - name: "funcA"
        input:
          - name: "output"
            origin: "inference"
            description: "test"
            successCriteria:
              condition: "test"
            onError:
              strategy: "requestUserInput"
              message: "Error"

      - name: "funcC"
        operation: "format"
        description: "Function C"
        triggers:
          - type: "flex_for_user"
        needs:
          - name: "funcA"
          - name: "funcB"
        input:
          - name: "output"
            origin: "inference"
            description: "test"
            successCriteria:
              condition: "test"
            onError:
              strategy: "requestUserInput"
              message: "Error"
`

	result, err := CreateToolWithWarnings(yamlContent)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if len(result.Warnings) != 1 {
		t.Fatalf("Expected 1 warning, got %d: %v", len(result.Warnings), result.Warnings)
	}

	warning := result.Warnings[0]
	if !strings.Contains(warning, "funcC") {
		t.Errorf("Expected warning to mention funcC, got: %s", warning)
	}
	if !strings.Contains(warning, "funcA") {
		t.Errorf("Expected warning to mention funcA as redundant, got: %s", warning)
	}
	if !strings.Contains(warning, "funcB") {
		t.Errorf("Expected warning to mention funcB as covering dependency, got: %s", warning)
	}
}

func TestDetectRedundantDependencies_MultipleRedundancies(t *testing.T) {
	yamlContent := `
version: "v1"
author: "test"
tools:
  - name: "testTool"
    description: "Test tool"
    version: "1.0.0"
    functions:
      - name: "funcA"
        operation: "format"
        description: "Function A"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "output"
            origin: "inference"
            description: "test"
            successCriteria:
              condition: "test"
            onError:
              strategy: "requestUserInput"
              message: "Error"

      - name: "funcB"
        operation: "format"
        description: "Function B"
        triggers:
          - type: "flex_for_user"
        needs:
          - name: "funcA"
        input:
          - name: "output"
            origin: "inference"
            description: "test"
            successCriteria:
              condition: "test"
            onError:
              strategy: "requestUserInput"
              message: "Error"

      - name: "funcC"
        operation: "format"
        description: "Function C"
        triggers:
          - type: "flex_for_user"
        needs:
          - name: "funcB"
        input:
          - name: "output"
            origin: "inference"
            description: "test"
            successCriteria:
              condition: "test"
            onError:
              strategy: "requestUserInput"
              message: "Error"

      - name: "funcD"
        operation: "format"
        description: "Function D"
        triggers:
          - type: "flex_for_user"
        needs:
          - name: "funcA"
          - name: "funcB"
          - name: "funcC"
        input:
          - name: "output"
            origin: "inference"
            description: "test"
            successCriteria:
              condition: "test"
            onError:
              strategy: "requestUserInput"
              message: "Error"
`

	result, err := CreateToolWithWarnings(yamlContent)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// funcD should have 2 redundant dependencies: funcA and funcB (both covered by funcC)
	if len(result.Warnings) != 2 {
		t.Fatalf("Expected 2 warnings, got %d: %v", len(result.Warnings), result.Warnings)
	}

	warningsText := strings.Join(result.Warnings, " ")
	if !strings.Contains(warningsText, "funcA") || !strings.Contains(warningsText, "funcB") {
		t.Errorf("Expected warnings to mention funcA and funcB as redundant, got: %v", result.Warnings)
	}
}

func TestDetectRedundantDependencies_ComplexChain(t *testing.T) {
	// Test case similar to the oralunic.yaml scenario:
	// - getAllAgendas is a standalone function
	// - validateAppointmentSlot needs getAllAgendas
	// - NewAppointment needs both getAllAgendas and validateAppointmentSlot
	// getAllAgendas in NewAppointment should be flagged as redundant

	yamlContent := `
version: "v1"
author: "test"
tools:
  - name: "testTool"
    description: "Test tool"
    version: "1.0.0"
    functions:
      - name: "getAllAgendas"
        operation: "format"
        description: "Get all agendas"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "output"
            origin: "inference"
            description: "test"
            successCriteria:
              condition: "test"
            onError:
              strategy: "requestUserInput"
              message: "Error"

      - name: "validateAppointmentSlot"
        operation: "format"
        description: "Validate appointment slot"
        triggers:
          - type: "flex_for_user"
        needs:
          - name: "getAllAgendas"
        input:
          - name: "output"
            origin: "inference"
            description: "test"
            successCriteria:
              condition: "test"
            onError:
              strategy: "requestUserInput"
              message: "Error"

      - name: "NewAppointment"
        operation: "format"
        description: "Create new appointment"
        triggers:
          - type: "flex_for_user"
        needs:
          - name: "getAllAgendas"
          - name: "validateAppointmentSlot"
        input:
          - name: "output"
            origin: "inference"
            description: "test"
            successCriteria:
              condition: "test"
            onError:
              strategy: "requestUserInput"
              message: "Error"
`

	result, err := CreateToolWithWarnings(yamlContent)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if len(result.Warnings) != 1 {
		t.Fatalf("Expected 1 warning, got %d: %v", len(result.Warnings), result.Warnings)
	}

	warning := result.Warnings[0]
	if !strings.Contains(warning, "NewAppointment") {
		t.Errorf("Expected warning to mention NewAppointment, got: %s", warning)
	}
	if !strings.Contains(warning, "getAllAgendas") {
		t.Errorf("Expected warning to mention getAllAgendas as redundant, got: %s", warning)
	}
	if !strings.Contains(warning, "validateAppointmentSlot") {
		t.Errorf("Expected warning to mention validateAppointmentSlot as covering dependency, got: %s", warning)
	}
	if !strings.Contains(warning, "redundant dependency") {
		t.Errorf("Expected warning to mention 'redundant dependency', got: %s", warning)
	}
}

func TestDetectRedundantDependencies_NoRedundancyWithSystemFunctions(t *testing.T) {
	// System functions should not be considered for redundancy detection
	yamlContent := `
version: "v1"
author: "test"
tools:
  - name: "testTool"
    description: "Test tool"
    version: "1.0.0"
    functions:
      - name: "funcA"
        operation: "format"
        description: "Function A"
        triggers:
          - type: "flex_for_user"
        needs:
          - name: "askToKnowledgeBase"
            query: "test query"
        input:
          - name: "output"
            origin: "inference"
            description: "test"
            successCriteria:
              condition: "test"
            onError:
              strategy: "requestUserInput"
              message: "Error"
              allowedSystemFunctions: ["askToKnowledgeBase"]
`

	result, err := CreateToolWithWarnings(yamlContent)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// System functions should not trigger redundancy warnings
	if len(result.Warnings) != 0 {
		t.Errorf("Expected no warnings for system functions, got %d: %v", len(result.Warnings), result.Warnings)
	}
}

func TestDetectRedundantDependencies_IndependentBranches(t *testing.T) {
	// Test that independent dependency branches don't trigger false positives
	yamlContent := `
version: "v1"
author: "test"
tools:
  - name: "testTool"
    description: "Test tool"
    version: "1.0.0"
    functions:
      - name: "funcA"
        operation: "format"
        description: "Function A"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "output"
            origin: "inference"
            description: "test"
            successCriteria:
              condition: "test"
            onError:
              strategy: "requestUserInput"
              message: "Error"

      - name: "funcB"
        operation: "format"
        description: "Function B"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "output"
            origin: "inference"
            description: "test"
            successCriteria:
              condition: "test"
            onError:
              strategy: "requestUserInput"
              message: "Error"

      - name: "funcC"
        operation: "format"
        description: "Function C"
        triggers:
          - type: "flex_for_user"
        needs:
          - name: "funcA"
          - name: "funcB"
        input:
          - name: "output"
            origin: "inference"
            description: "test"
            successCriteria:
              condition: "test"
            onError:
              strategy: "requestUserInput"
              message: "Error"
`

	result, err := CreateToolWithWarnings(yamlContent)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// funcA and funcB are independent, so no redundancy
	if len(result.Warnings) != 0 {
		t.Errorf("Expected no warnings for independent branches, got %d: %v", len(result.Warnings), result.Warnings)
	}
}
