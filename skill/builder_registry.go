package skill

// builder_registry.go exports validation registries for the connectai-builder CLI.
// This file references existing constants and maps. It does NOT modify any existing code.
// If this file is deleted, nothing in the runtime breaks.

// GetValidOperations returns all valid operation type strings.
func GetValidOperations() []string {
	return []string{
		OperationWeb,
		OperationAPI,
		OperationDesktop,
		OperationMCP,
		OperationFormat,
		OperationDB,
		OperationTerminal,
		OperationInitiateWorkflow,
		OperationPolicy,
		OperationPDF,
		OperationGDrive,
	}
}

// GetValidTriggerTypes returns all valid trigger type strings.
func GetValidTriggerTypes() []string {
	return []string{
		TriggerTime,
		TriggerOnUserMessage,
		TriggerOnTeamMessage,
		TriggerOnCompletedUserMessage,
		TriggerOnCompletedTeamMessage,
		TriggerFlexForUser,
		TriggerFlexForTeam,
		TriggerOnMeetingStart,
		TriggerOnMeetingEnd,
	}
}

// GetValidDataOrigins returns all valid input origin strings.
func GetValidDataOrigins() []string {
	return []string{
		DataOriginChat,
		DataOriginFunction,
		DataOriginKnowledge,
		DataOriginSearch,
		DataOriginInference,
		DataOriginMemory,
		"value",  // origin: value (static/computed value)
		"system", // origin: system (system variable)
	}
}

// GetValidOnErrorStrategies returns all valid error strategy strings.
func GetValidOnErrorStrategies() []string {
	return []string{
		OnErrorRequestUserInput,
		OnErrorRequestN1Support,
		OnErrorRequestN2Support,
		OnErrorRequestN3Support,
		OnErrorRequestApplicationSupport,
		OnErrorSearch,
		OnErrorInference,
	}
}

// GetValidStepActions returns all valid step action strings grouped by operation type.
func GetValidStepActions() map[string][]string {
	return map[string][]string{
		OperationAPI: {
			StepActionGET,
			StepActionPOST,
			StepActionPUT,
			StepActionPATCH,
			StepActionDELETE,
		},
		OperationDB: {
			StepActionSelect,
			StepActionWrite,
		},
		OperationTerminal: {
			StepActionSh,
			StepActionBash,
		},
		OperationWeb: {
			StepActionURL,
			StepActionFill,
			StepActionSubmit,
			StepActionFindClick,
			StepActionFindFillTab,
			StepActionFindFillReturn,
			StepActionExtractText,
			StepActionApp,
		},
		OperationInitiateWorkflow: {
			StepActionStartWorkflow,
		},
		OperationGDrive: {
			StepActionGDriveList,
			StepActionGDriveUpload,
			StepActionGDriveDownload,
			StepActionGDriveCreateFolder,
			StepActionGDriveDelete,
			StepActionGDriveMove,
			StepActionGDriveSearch,
			StepActionGDriveGetMetadata,
			StepActionGDriveUpdate,
			StepActionGDriveExport,
		},
	}
}

// GetValidOutputTypes returns all valid output type strings.
func GetValidOutputTypes() []string {
	return []string{
		StepOutputObject,
		StepOutputListOfObject,
		StepOutputListString,
		StepOutputListNumber,
		FieldTypeString,
		OutputTypeFile,
		OutputTypeListFile,
	}
}

// GetValidCacheScopes returns all valid cache scope strings.
func GetValidCacheScopes() []string {
	return []string{
		string(CacheScopeGlobal),
		string(CacheScopeClient),
		string(CacheScopeMessage),
	}
}

// GetValidCallRuleTypes returns all valid call rule type strings.
func GetValidCallRuleTypes() []string {
	return []string{
		CallRuleTypeOnce,
		CallRuleTypeUnique,
		CallRuleTypeMultiple,
	}
}

// GetValidCallRuleScopes returns all valid call rule scope strings.
func GetValidCallRuleScopes() []string {
	return []string{
		CallRuleScopeUser,
		CallRuleScopeMessage,
		CallRuleScopeMinimumInterval,
		CallRuleScopeCompany,
	}
}

// GetValidConcurrencyStrategies returns all valid concurrency control strategies.
func GetValidConcurrencyStrategies() []string {
	return []string{
		ConcurrencyStrategyParallel,
		ConcurrencyStrategySkip,
		ConcurrencyStrategyKill,
	}
}

// GetValidCombineModes returns valid combine modes for runOnlyIf/reRunIf.
func GetValidCombineModes() []string {
	return []string{
		CombineModeAND,
		CombineModeOR,
	}
}

// GetValidReRunScopes returns valid scopes for reRunIf.
func GetValidReRunScopes() []string {
	return []string{
		ReRunScopeSteps,
		ReRunScopeFull,
	}
}

// GetValidCallbackTypes returns the valid callback event types.
func GetValidCallbackTypes() []string {
	return []string{
		"success",
		"failure",
		"skip",
		"missing-user-info",
		"user-confirmation-request",
		"team-approval-request",
	}
}

// GetValidWorkflowTypes returns valid workflow types.
func GetValidWorkflowTypes() []string {
	return []string{
		WorkflowTypeUser,
		WorkflowTypeTeam,
	}
}

// GetValidMemoryRetrievalModes returns valid memory retrieval modes.
func GetValidMemoryRetrievalModes() []string {
	return []string{
		"all",
		"latest",
	}
}

// GetTriggerVarAvailability returns which system vars are available per trigger type.
func GetTriggerVarAvailability() map[string]map[string]bool {
	return triggerVarAvailability
}

// GetTriggerFuncAvailability returns which system funcs are available per trigger type.
func GetTriggerFuncAvailability() map[string]map[string]bool {
	return triggerFuncAvailability
}

// GetSystemFunctionsMap returns the system functions map (read-only copy).
func GetSystemFunctionsMap() map[string]bool {
	result := make(map[string]bool, len(systemFunctions))
	for k, v := range systemFunctions {
		result[k] = v
	}
	return result
}

// GetSystemFunctionsWithParamsMap returns system functions that accept parameters.
func GetSystemFunctionsWithParamsMap() map[string]bool {
	result := make(map[string]bool, len(systemFunctionsWithParams))
	for k, v := range systemFunctionsWithParams {
		result[k] = v
	}
	return result
}

// GetSystemVarFieldsMap returns the fields map for all system variables.
func GetSystemVarFieldsMap() map[string][]string {
	result := make(map[string][]string, len(systemVarFields))
	for sysVar, fields := range systemVarFields {
		fieldNames := make([]string, 0, len(fields))
		for field := range fields {
			fieldNames = append(fieldNames, field)
		}
		result[sysVar] = fieldNames
	}
	return result
}
