package skill

import (
	"strings"
	"testing"
)

// parseTestYAML is a helper function that parses YAML and returns error if any
func parseOrphanTestYAML(yaml string) error {
	_, err := CreateToolWithWarnings(yaml)
	return err
}

// ============================================================================
// VALID CASES - These should pass validation
// ============================================================================

func TestOrphanFunctions_PrivateReachableViaNeeds(t *testing.T) {
	yaml := `
version: "v1"
author: "test"
tools:
  - name: "test_tool"
    description: "Test tool"
    version: "1.0.0"
    functions:
      - name: "PublicFunc"
        operation: "format"
        description: "Public function"
        triggers:
          - type: "flex_for_user"
        needs:
          - name: "helper"
        input:
          - name: "output"
            origin: "inference"
            description: "test"
            successCriteria:
              condition: "test"
            onError:
              strategy: "requestUserInput"
              message: "Error"

      - name: "helper"
        operation: "format"
        description: "Private helper"
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
	err := parseOrphanTestYAML(yaml)
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
}

func TestOrphanFunctions_PrivateReachableViaOnSuccess(t *testing.T) {
	yaml := `
version: "v1"
author: "test"
tools:
  - name: "test_tool"
    description: "Test tool"
    version: "1.0.0"
    functions:
      - name: "PublicFunc"
        operation: "format"
        description: "Public function"
        triggers:
          - type: "flex_for_user"
        onSuccess:
          - name: "handleSuccess"
        input:
          - name: "output"
            origin: "inference"
            description: "test"
            successCriteria:
              condition: "test"
            onError:
              strategy: "requestUserInput"
              message: "Error"

      - name: "handleSuccess"
        operation: "format"
        description: "Success handler"
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
	err := parseOrphanTestYAML(yaml)
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
}

func TestOrphanFunctions_PrivateReachableViaOnFailure(t *testing.T) {
	yaml := `
version: "v1"
author: "test"
tools:
  - name: "test_tool"
    description: "Test tool"
    version: "1.0.0"
    functions:
      - name: "PublicFunc"
        operation: "format"
        description: "Public function"
        triggers:
          - type: "flex_for_user"
        onFailure:
          - name: "handleFailure"
        input:
          - name: "output"
            origin: "inference"
            description: "test"
            successCriteria:
              condition: "test"
            onError:
              strategy: "requestUserInput"
              message: "Error"

      - name: "handleFailure"
        operation: "format"
        description: "Failure handler"
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
	err := parseOrphanTestYAML(yaml)
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
}

func TestOrphanFunctions_PrivateReachableViaOnSkip(t *testing.T) {
	yaml := `
version: "v1"
author: "test"
tools:
  - name: "test_tool"
    description: "Test tool"
    version: "1.0.0"
    functions:
      - name: "PublicFunc"
        operation: "format"
        description: "Public function"
        triggers:
          - type: "flex_for_user"
        runOnlyIf:
          condition: "false"
        onSkip:
          - name: "handleSkip"
        input:
          - name: "output"
            origin: "inference"
            description: "test"
            successCriteria:
              condition: "test"
            onError:
              strategy: "requestUserInput"
              message: "Error"

      - name: "handleSkip"
        operation: "format"
        description: "Skip handler"
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
	err := parseOrphanTestYAML(yaml)
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
}

func TestOrphanFunctions_PrivateReachableViaInputFrom(t *testing.T) {
	yaml := `
version: "v1"
author: "test"
tools:
  - name: "test_tool"
    description: "Test tool"
    version: "1.0.0"
    functions:
      - name: "PublicFunc"
        operation: "format"
        description: "Public function"
        triggers:
          - type: "flex_for_user"
        needs:
          - name: "dataProvider"
        input:
          - name: "data"
            origin: "function"
            from: "dataProvider"
            description: "test"
            successCriteria:
              condition: "test"
            onError:
              strategy: "requestUserInput"
              message: "Error"
          - name: "output"
            origin: "inference"
            description: "test"
            successCriteria:
              condition: "test"
            onError:
              strategy: "requestUserInput"
              message: "Error"

      - name: "dataProvider"
        operation: "format"
        description: "Provides data"
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
	err := parseOrphanTestYAML(yaml)
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
}

func TestOrphanFunctions_PrivateReachableFromCron(t *testing.T) {
	yaml := `
version: "v1"
author: "test"
tools:
  - name: "test_tool"
    description: "Test tool"
    version: "1.0.0"
    functions:
      - name: "CronJob"
        operation: "format"
        description: "Cron job"
        triggers:
          - type: "time_based"
            cron: "0 0 0 * * *"
        needs:
          - name: "helper"
        input:
          - name: "output"
            origin: "inference"
            description: "test"
            successCriteria:
              condition: "test"
            onError:
              strategy: "requestUserInput"
              message: "Error"

      - name: "helper"
        operation: "format"
        description: "Helper function"
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
	err := parseOrphanTestYAML(yaml)
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
}

func TestOrphanFunctions_TransitiveReachability(t *testing.T) {
	yaml := `
version: "v1"
author: "test"
tools:
  - name: "test_tool"
    description: "Test tool"
    version: "1.0.0"
    functions:
      - name: "PublicFunc"
        operation: "format"
        description: "Public function"
        triggers:
          - type: "flex_for_user"
        needs:
          - name: "helperA"
        input:
          - name: "output"
            origin: "inference"
            description: "test"
            successCriteria:
              condition: "test"
            onError:
              strategy: "requestUserInput"
              message: "Error"

      - name: "helperA"
        operation: "format"
        description: "First helper"
        needs:
          - name: "helperB"
        input:
          - name: "output"
            origin: "inference"
            description: "test"
            successCriteria:
              condition: "test"
            onError:
              strategy: "requestUserInput"
              message: "Error"

      - name: "helperB"
        operation: "format"
        description: "Second helper"
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
	err := parseOrphanTestYAML(yaml)
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
}

func TestOrphanFunctions_CronTriggeredPrivate(t *testing.T) {
	yaml := `
version: "v1"
author: "test"
tools:
  - name: "test_tool"
    description: "Test tool"
    version: "1.0.0"
    functions:
      - name: "PublicFunc"
        operation: "format"
        description: "Public function"
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

      - name: "cronHelper"
        operation: "format"
        description: "Cron triggered private function"
        triggers:
          - type: "time_based"
            cron: "0 0 0 * * *"
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
	err := parseOrphanTestYAML(yaml)
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
}

func TestOrphanFunctions_AllPublic(t *testing.T) {
	yaml := `
version: "v1"
author: "test"
tools:
  - name: "test_tool"
    description: "Test tool"
    version: "1.0.0"
    functions:
      - name: "FuncA"
        operation: "format"
        description: "Public A"
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

      - name: "FuncB"
        operation: "format"
        description: "Public B"
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
`
	err := parseOrphanTestYAML(yaml)
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
}

func TestOrphanFunctions_MixedEntryPoints(t *testing.T) {
	yaml := `
version: "v1"
author: "test"
tools:
  - name: "test_tool"
    description: "Test tool"
    version: "1.0.0"
    functions:
      - name: "PublicFunc"
        operation: "format"
        description: "Public function"
        triggers:
          - type: "flex_for_user"
        needs:
          - name: "sharedHelper"
        input:
          - name: "output"
            origin: "inference"
            description: "test"
            successCriteria:
              condition: "test"
            onError:
              strategy: "requestUserInput"
              message: "Error"

      - name: "cronPrivate"
        operation: "format"
        description: "Cron triggered private"
        triggers:
          - type: "time_based"
            cron: "0 0 0 * * *"
        needs:
          - name: "sharedHelper"
        input:
          - name: "output"
            origin: "inference"
            description: "test"
            successCriteria:
              condition: "test"
            onError:
              strategy: "requestUserInput"
              message: "Error"

      - name: "sharedHelper"
        operation: "format"
        description: "Shared helper"
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
	err := parseOrphanTestYAML(yaml)
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
}

func TestOrphanFunctions_PrivateReachableViaOnMissingUserInfo(t *testing.T) {
	yaml := `
version: "v1"
author: "test"
tools:
  - name: "test_tool"
    description: "Test tool"
    version: "1.0.0"
    functions:
      - name: "PublicFunc"
        operation: "format"
        description: "Public function"
        triggers:
          - type: "flex_for_user"
        onMissingUserInfo:
          - name: "handleMissingInfo"
        input:
          - name: "output"
            origin: "inference"
            description: "test"
            successCriteria:
              condition: "test"
            onError:
              strategy: "requestUserInput"
              message: "Error"

      - name: "handleMissingInfo"
        operation: "format"
        description: "Missing info handler"
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
	err := parseOrphanTestYAML(yaml)
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
}

func TestOrphanFunctions_PrivateReachableViaOnUserConfirmationRequest(t *testing.T) {
	yaml := `
version: "v1"
author: "test"
tools:
  - name: "test_tool"
    description: "Test tool"
    version: "1.0.0"
    functions:
      - name: "PublicFunc"
        operation: "format"
        description: "Public function"
        triggers:
          - type: "flex_for_user"
        onUserConfirmationRequest:
          - name: "handleConfirmation"
        input:
          - name: "output"
            origin: "inference"
            description: "test"
            successCriteria:
              condition: "test"
            onError:
              strategy: "requestUserInput"
              message: "Error"

      - name: "handleConfirmation"
        operation: "format"
        description: "Confirmation handler"
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
	err := parseOrphanTestYAML(yaml)
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
}

func TestOrphanFunctions_PrivateReachableViaOnTeamApprovalRequest(t *testing.T) {
	yaml := `
version: "v1"
author: "test"
tools:
  - name: "test_tool"
    description: "Test tool"
    version: "1.0.0"
    functions:
      - name: "PublicFunc"
        operation: "format"
        description: "Public function"
        triggers:
          - type: "flex_for_user"
        onTeamApprovalRequest:
          - name: "handleApproval"
        input:
          - name: "output"
            origin: "inference"
            description: "test"
            successCriteria:
              condition: "test"
            onError:
              strategy: "requestUserInput"
              message: "Error"

      - name: "handleApproval"
        operation: "format"
        description: "Approval handler"
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
	err := parseOrphanTestYAML(yaml)
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
}

// ============================================================================
// INVALID CASES - These should fail validation
// ============================================================================

func TestOrphanFunctions_OrphanPrivate(t *testing.T) {
	yaml := `
version: "v1"
author: "test"
tools:
  - name: "test_tool"
    description: "Test tool"
    version: "1.0.0"
    functions:
      - name: "PublicFunc"
        operation: "format"
        description: "Public function"
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

      - name: "orphanHelper"
        operation: "format"
        description: "Orphan helper - not reachable from public"
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
`
	err := parseOrphanTestYAML(yaml)
	if err == nil {
		t.Error("Expected error for orphan private function, got nil")
	} else if !strings.Contains(err.Error(), "orphan private function") {
		t.Errorf("Expected error about orphan private function, got: %v", err)
	}
}

func TestOrphanFunctions_PrivateOnlyReachableFromPrivate(t *testing.T) {
	yaml := `
version: "v1"
author: "test"
tools:
  - name: "test_tool"
    description: "Test tool"
    version: "1.0.0"
    functions:
      - name: "PublicFunc"
        operation: "format"
        description: "Public function"
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

      - name: "orphanA"
        operation: "format"
        description: "Orphan A - has trigger but not cron, not called by public"
        triggers:
          - type: "flex_for_user"
        needs:
          - name: "orphanB"
        input:
          - name: "output"
            origin: "inference"
            description: "test"
            successCriteria:
              condition: "test"
            onError:
              strategy: "requestUserInput"
              message: "Error"

      - name: "orphanB"
        operation: "format"
        description: "Orphan B - only reachable from orphanA"
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
	err := parseOrphanTestYAML(yaml)
	if err == nil {
		t.Error("Expected error for orphan private functions, got nil")
	} else if !strings.Contains(err.Error(), "orphan private function") {
		t.Errorf("Expected error about orphan private function, got: %v", err)
	}
}

func TestOrphanFunctions_MultipleOrphans(t *testing.T) {
	yaml := `
version: "v1"
author: "test"
tools:
  - name: "test_tool"
    description: "Test tool"
    version: "1.0.0"
    functions:
      - name: "PublicFunc"
        operation: "format"
        description: "Public function"
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

      - name: "orphanA"
        operation: "format"
        description: "Orphan A"
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

      - name: "orphanB"
        operation: "format"
        description: "Orphan B"
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
`
	err := parseOrphanTestYAML(yaml)
	if err == nil {
		t.Error("Expected error for multiple orphan private functions, got nil")
	} else {
		errStr := err.Error()
		if !strings.Contains(errStr, "orphan private function") {
			t.Errorf("Expected error about orphan private function, got: %v", err)
		}
		// Should report both orphans
		if !strings.Contains(errStr, "orphanA") || !strings.Contains(errStr, "orphanB") {
			t.Errorf("Expected error to list both orphanA and orphanB, got: %v", err)
		}
	}
}

func TestOrphanFunctions_OrphanChain(t *testing.T) {
	yaml := `
version: "v1"
author: "test"
tools:
  - name: "test_tool"
    description: "Test tool"
    version: "1.0.0"
    functions:
      - name: "PublicFunc"
        operation: "format"
        description: "Public function"
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

      - name: "orphanChainA"
        operation: "format"
        description: "Orphan chain A - not reachable from public"
        triggers:
          - type: "flex_for_user"
        needs:
          - name: "orphanChainB"
        input:
          - name: "output"
            origin: "inference"
            description: "test"
            successCriteria:
              condition: "test"
            onError:
              strategy: "requestUserInput"
              message: "Error"

      - name: "orphanChainB"
        operation: "format"
        description: "Orphan chain B - reachable from orphanChainA only"
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
	err := parseOrphanTestYAML(yaml)
	if err == nil {
		t.Error("Expected error for orphan chain, got nil")
	} else if !strings.Contains(err.Error(), "orphan private function") {
		t.Errorf("Expected error about orphan private function, got: %v", err)
	}
}

// ============================================================================
// EDGE CASES
// ============================================================================

func TestOrphanFunctions_PrivateWithNonCronTrigger(t *testing.T) {
	// Private function with flex_for_user trigger but not reachable from public
	// Should be an orphan because flex_for_user doesn't make it an entry point
	yaml := `
version: "v1"
author: "test"
tools:
  - name: "test_tool"
    description: "Test tool"
    version: "1.0.0"
    functions:
      - name: "PublicFunc"
        operation: "format"
        description: "Public function"
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

      - name: "privateWithTrigger"
        operation: "format"
        description: "Private with trigger but not cron"
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
`
	err := parseOrphanTestYAML(yaml)
	if err == nil {
		t.Error("Expected error for private with non-cron trigger not reachable from public, got nil")
	} else if !strings.Contains(err.Error(), "orphan private function") {
		t.Errorf("Expected error about orphan private function, got: %v", err)
	}
}

func TestOrphanFunctions_DiamondWithOrphan(t *testing.T) {
	// Diamond pattern where one branch leads to an orphan
	// Public -> helperA -> sharedHelper (valid)
	// orphanBranch -> sharedHelper (orphan because orphanBranch is not reachable)
	yaml := `
version: "v1"
author: "test"
tools:
  - name: "test_tool"
    description: "Test tool"
    version: "1.0.0"
    functions:
      - name: "PublicFunc"
        operation: "format"
        description: "Public function"
        triggers:
          - type: "flex_for_user"
        needs:
          - name: "helperA"
        input:
          - name: "output"
            origin: "inference"
            description: "test"
            successCriteria:
              condition: "test"
            onError:
              strategy: "requestUserInput"
              message: "Error"

      - name: "helperA"
        operation: "format"
        description: "Helper A - reachable from public"
        needs:
          - name: "sharedHelper"
        input:
          - name: "output"
            origin: "inference"
            description: "test"
            successCriteria:
              condition: "test"
            onError:
              strategy: "requestUserInput"
              message: "Error"

      - name: "orphanBranch"
        operation: "format"
        description: "Orphan branch - not reachable from any entry point"
        triggers:
          - type: "flex_for_user"
        needs:
          - name: "sharedHelper"
        input:
          - name: "output"
            origin: "inference"
            description: "test"
            successCriteria:
              condition: "test"
            onError:
              strategy: "requestUserInput"
              message: "Error"

      - name: "sharedHelper"
        operation: "format"
        description: "Shared helper - reachable via helperA"
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
	err := parseOrphanTestYAML(yaml)
	if err == nil {
		t.Error("Expected error for diamond with orphan branch, got nil")
	} else if !strings.Contains(err.Error(), "orphan private function") {
		t.Errorf("Expected error about orphan private function, got: %v", err)
	}
}

func TestOrphanFunctions_ComplexCallbackChain(t *testing.T) {
	// Test a complex chain: Public -> onSuccess -> onFailure -> onSkip -> helper
	yaml := `
version: "v1"
author: "test"
tools:
  - name: "test_tool"
    description: "Test tool"
    version: "1.0.0"
    functions:
      - name: "PublicFunc"
        operation: "format"
        description: "Public function"
        triggers:
          - type: "flex_for_user"
        onSuccess:
          - name: "successHandler"
        input:
          - name: "output"
            origin: "inference"
            description: "test"
            successCriteria:
              condition: "test"
            onError:
              strategy: "requestUserInput"
              message: "Error"

      - name: "successHandler"
        operation: "format"
        description: "Success handler"
        onFailure:
          - name: "failureHandler"
        input:
          - name: "output"
            origin: "inference"
            description: "test"
            successCriteria:
              condition: "test"
            onError:
              strategy: "requestUserInput"
              message: "Error"

      - name: "failureHandler"
        operation: "format"
        description: "Failure handler"
        runOnlyIf:
          condition: "false"
        onSkip:
          - name: "skipHandler"
        input:
          - name: "output"
            origin: "inference"
            description: "test"
            successCriteria:
              condition: "test"
            onError:
              strategy: "requestUserInput"
              message: "Error"

      - name: "skipHandler"
        operation: "format"
        description: "Skip handler"
        needs:
          - name: "finalHelper"
        input:
          - name: "output"
            origin: "inference"
            description: "test"
            successCriteria:
              condition: "test"
            onError:
              strategy: "requestUserInput"
              message: "Error"

      - name: "finalHelper"
        operation: "format"
        description: "Final helper at end of chain"
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
	err := parseOrphanTestYAML(yaml)
	if err != nil {
		t.Errorf("Expected no error for complex callback chain, got: %v", err)
	}
}

// ============================================================================
// NO ENTRY POINTS EDGE CASES
// ============================================================================

func TestOrphanFunctions_NoEntryPoints_OnlyPrivateWithTrigger(t *testing.T) {
	// Tool with only private functions that have non-cron triggers
	// These should be flagged as orphans because there are no entry points
	yaml := `
version: "v1"
author: "test"
tools:
  - name: "test_tool"
    description: "Test tool"
    version: "1.0.0"
    functions:
      - name: "privateFunc"
        operation: "format"
        description: "Private function with trigger but no entry point"
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
`
	err := parseOrphanTestYAML(yaml)
	if err == nil {
		t.Error("Expected error for private function with no entry points, got nil")
	} else if !strings.Contains(err.Error(), "orphan private function") {
		t.Errorf("Expected error about orphan private function, got: %v", err)
	}
}

func TestOrphanFunctions_NoEntryPoints_OnlyCronPrivate(t *testing.T) {
	// Tool with only a cron-triggered private function
	// This should NOT be flagged as orphan because cron-triggered functions are entry points
	yaml := `
version: "v1"
author: "test"
tools:
  - name: "test_tool"
    description: "Test tool"
    version: "1.0.0"
    functions:
      - name: "cronPrivate"
        operation: "format"
        description: "Cron triggered private function"
        triggers:
          - type: "time_based"
            cron: "0 0 0 * * *"
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
	err := parseOrphanTestYAML(yaml)
	if err != nil {
		t.Errorf("Expected no error for cron-triggered private function, got: %v", err)
	}
}

func TestOrphanFunctions_NoPublic_CronPrivateWithHelper(t *testing.T) {
	// Tool with no public functions, but has a cron-triggered private + helper
	// The helper should be reachable from the cron entry point
	yaml := `
version: "v1"
author: "test"
tools:
  - name: "test_tool"
    description: "Test tool"
    version: "1.0.0"
    functions:
      - name: "cronPrivate"
        operation: "format"
        description: "Cron triggered private function"
        triggers:
          - type: "time_based"
            cron: "0 0 0 * * *"
        needs:
          - name: "helper"
        input:
          - name: "output"
            origin: "inference"
            description: "test"
            successCriteria:
              condition: "test"
            onError:
              strategy: "requestUserInput"
              message: "Error"

      - name: "helper"
        operation: "format"
        description: "Helper function"
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
	err := parseOrphanTestYAML(yaml)
	if err != nil {
		t.Errorf("Expected no error for cron-triggered private with helper, got: %v", err)
	}
}

func TestOrphanFunctions_NoEntryPoints_MultiplePrivateWithTriggers(t *testing.T) {
	// Tool with multiple private functions with triggers but no entry points
	// All should be flagged as orphans
	yaml := `
version: "v1"
author: "test"
tools:
  - name: "test_tool"
    description: "Test tool"
    version: "1.0.0"
    functions:
      - name: "privateA"
        operation: "format"
        description: "Private A"
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

      - name: "privateB"
        operation: "format"
        description: "Private B"
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
`
	err := parseOrphanTestYAML(yaml)
	if err == nil {
		t.Error("Expected error for multiple private functions with no entry points, got nil")
	} else {
		errStr := err.Error()
		if !strings.Contains(errStr, "orphan private function") {
			t.Errorf("Expected error about orphan private function, got: %v", err)
		}
		// Should report both orphans
		if !strings.Contains(errStr, "privateA") || !strings.Contains(errStr, "privateB") {
			t.Errorf("Expected error to list both privateA and privateB, got: %v", err)
		}
	}
}

// ============================================================================
// Unit test for BuildForwardDependencyGraph with new callbacks
// ============================================================================

func TestBuildForwardDependencyGraph_IncludesAllCallbacks(t *testing.T) {
	functions := []Function{
		{
			Name: "main",
			Needs: []NeedItem{
				{Name: "needsDep"},
			},
			OnSuccess: []FunctionCall{
				{Name: "successDep"},
			},
			OnFailure: []FunctionCall{
				{Name: "failureDep"},
			},
			OnSkip: []FunctionCall{
				{Name: "skipDep"},
			},
			OnMissingUserInfo: []FunctionCall{
				{Name: "missingInfoDep"},
			},
			OnUserConfirmationRequest: []FunctionCall{
				{Name: "confirmDep"},
			},
			OnTeamApprovalRequest: []FunctionCall{
				{Name: "approvalDep"},
			},
			Input: []Input{
				{
					Name:   "data",
					Origin: DataOriginFunction,
					From:   "inputFromDep.field",
				},
			},
		},
		{Name: "needsDep"},
		{Name: "successDep"},
		{Name: "failureDep"},
		{Name: "skipDep"},
		{Name: "missingInfoDep"},
		{Name: "confirmDep"},
		{Name: "approvalDep"},
		{Name: "inputFromDep"},
	}

	graph := BuildForwardDependencyGraph(functions)

	// Check that main can reach all dependencies
	mainDeps := graph["main"]
	expectedDeps := []string{
		"needsDep",
		"successDep",
		"failureDep",
		"skipDep",
		"missingInfoDep",
		"confirmDep",
		"approvalDep",
		"inputFromDep",
	}

	for _, expected := range expectedDeps {
		found := false
		for _, dep := range mainDeps {
			if dep == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected main to have dependency on '%s', but it was not found in %v", expected, mainDeps)
		}
	}
}
