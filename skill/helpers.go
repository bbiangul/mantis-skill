package skill

import (
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"
)

// ParseRunOnlyIf converts the RunOnlyIf field to a RunOnlyIfObject
func ParseRunOnlyIf(runOnlyIf interface{}) (*RunOnlyIfObject, error) {
	if runOnlyIf == nil {
		return nil, nil
	}

	switch v := runOnlyIf.(type) {
	case string:
		// Backward compatible: string = inference mode
		if v == "" {
			return nil, nil
		}
		return &RunOnlyIfObject{
			Condition: v,
		}, nil

	case map[interface{}]interface{}:
		return parseRunOnlyIfFromInterfaceMap(v)

	case map[string]interface{}:
		return parseRunOnlyIfFromStringMap(v)

	case *RunOnlyIfObject:
		// Validate the object
		return validateRunOnlyIfObject(v)

	default:
		return nil, fmt.Errorf("runOnlyIf must be a string or object, got %v", reflect.TypeOf(runOnlyIf))
	}
}

// parseRunOnlyIfFromInterfaceMap handles map[interface{}]interface{} type
func parseRunOnlyIfFromInterfaceMap(v map[interface{}]interface{}) (*RunOnlyIfObject, error) {
	// Convert to map[string]interface{} for easier handling
	stringMap := make(map[string]interface{})
	for k, val := range v {
		if kStr, ok := k.(string); ok {
			stringMap[kStr] = val
		} else {
			return nil, fmt.Errorf("runOnlyIf map keys must be strings")
		}
	}
	return parseRunOnlyIfFromStringMap(stringMap)
}

// parseRunOnlyIfFromStringMap parses RunOnlyIf from a string map
func parseRunOnlyIfFromStringMap(v map[string]interface{}) (*RunOnlyIfObject, error) {
	result := &RunOnlyIfObject{}

	// Parse deterministic field
	if deterministic, ok := v["deterministic"]; ok {
		if detStr, ok := deterministic.(string); ok {
			result.Deterministic = detStr
		} else {
			return nil, fmt.Errorf("runOnlyIf.deterministic must be a string")
		}
	}

	// Parse condition field (backward compatible - simple inference mode)
	if condition, ok := v["condition"]; ok {
		if condStr, ok := condition.(string); ok {
			result.Condition = condStr
		} else {
			return nil, fmt.Errorf("runOnlyIf.condition must be a string")
		}
	}

	// Parse inference field (advanced inference mode)
	if inference, ok := v["inference"]; ok {
		inferenceObj, err := parseInferenceCondition(inference)
		if err != nil {
			return nil, fmt.Errorf("error parsing runOnlyIf.inference: %w", err)
		}
		result.Inference = inferenceObj
	}

	// Parse combineWith field
	if combineWith, ok := v["combineWith"]; ok {
		if combineStr, ok := combineWith.(string); ok {
			// Validate against constants
			if combineStr != CombineModeAND && combineStr != CombineModeOR {
				return nil, fmt.Errorf("runOnlyIf.combineWith must be '%s' or '%s', got '%s'",
					CombineModeAND, CombineModeOR, combineStr)
			}
			result.CombineMode = combineStr
		} else {
			return nil, fmt.Errorf("runOnlyIf.combineWith must be a string")
		}
	}

	// Parse from field (for backward compatible condition mode)
	if from, ok := v["from"]; ok {
		fromList, err := parseStringArray(from, "runOnlyIf.from")
		if err != nil {
			return nil, err
		}
		result.From = fromList
	}

	// Parse allowedSystemFunctions field (for backward compatible condition mode)
	if allowedSystemFunctions, ok := v["allowedSystemFunctions"]; ok {
		funcList, err := parseStringArray(allowedSystemFunctions, "runOnlyIf.allowedSystemFunctions")
		if err != nil {
			return nil, err
		}
		result.AllowedSystemFunctions = funcList
	}

	// Parse disableAllSystemFunctions field (for backward compatible condition mode)
	if disableAllSystemFunctions, ok := v["disableAllSystemFunctions"]; ok {
		if disableBool, ok := disableAllSystemFunctions.(bool); ok {
			result.DisableAllSystemFunctions = disableBool
		} else {
			return nil, fmt.Errorf("runOnlyIf.disableAllSystemFunctions must be a boolean")
		}
	}

	// Parse clientIds field (for queryCustomerServiceChats system function)
	if clientIds, ok := v["clientIds"]; ok {
		if clientIdsStr, ok := clientIds.(string); ok {
			result.ClientIds = clientIdsStr
		} else {
			return nil, fmt.Errorf("runOnlyIf.clientIds must be a string (variable reference like $clientIds)")
		}
	}

	// Parse codebaseDirs field (for searchCodebase system function)
	if codebaseDirs, ok := v["codebaseDirs"]; ok {
		if codebaseDirsStr, ok := codebaseDirs.(string); ok {
			result.CodebaseDirs = codebaseDirsStr
		} else {
			return nil, fmt.Errorf("runOnlyIf.codebaseDirs must be a string (variable reference like $getProjectRepos)")
		}
	}

	// Parse documentDbName field (for queryDocuments system function)
	if documentDbName, ok := v["documentDbName"]; ok {
		if documentDbNameStr, ok := documentDbName.(string); ok {
			result.DocumentDbName = documentDbNameStr
		} else {
			return nil, fmt.Errorf("runOnlyIf.documentDbName must be a string (variable reference or literal DB name)")
		}
	}

	// Parse documentEnableGraph field (for queryDocuments system function)
	if documentEnableGraph, ok := v["documentEnableGraph"]; ok {
		if enableBool, ok := documentEnableGraph.(bool); ok {
			result.DocumentEnableGraph = enableBool
		} else {
			return nil, fmt.Errorf("runOnlyIf.documentEnableGraph must be a boolean")
		}
	}

	// Parse memoryFilters field (for queryMemories system function)
	if memoryFilters, ok := v["memoryFilters"]; ok {
		filters, err := parseMemoryFiltersFromInterface(memoryFilters)
		if err != nil {
			return nil, fmt.Errorf("error parsing runOnlyIf.memoryFilters: %w", err)
		}
		result.MemoryFilters = filters
	}

	// Parse onError field
	if onError, ok := v["onError"]; ok {
		onErrorMap, ok := onError.(map[string]interface{})
		if !ok {
			onErrorMapIface, ok := onError.(map[interface{}]interface{})
			if !ok {
				return nil, fmt.Errorf("runOnlyIf.onError must be an object")
			}
			onErrorMap = make(map[string]interface{})
			for k, val := range onErrorMapIface {
				if kStr, ok := k.(string); ok {
					onErrorMap[kStr] = val
				}
			}
		}

		parsedOnError, err := parseOnErrorFromStringMap(onErrorMap)
		if err != nil {
			return nil, fmt.Errorf("error parsing runOnlyIf.onError: %w", err)
		}
		result.OnError = parsedOnError
	}

	// Validate the result
	return validateRunOnlyIfObject(result)
}

// parseInferenceCondition parses an InferenceCondition object
func parseInferenceCondition(inference interface{}) (*InferenceCondition, error) {
	var inferenceMap map[string]interface{}

	switch v := inference.(type) {
	case map[string]interface{}:
		inferenceMap = v
	case map[interface{}]interface{}:
		inferenceMap = make(map[string]interface{})
		for k, val := range v {
			if kStr, ok := k.(string); ok {
				inferenceMap[kStr] = val
			}
		}
	default:
		return nil, fmt.Errorf("inference must be an object")
	}

	result := &InferenceCondition{}

	// condition is required
	condition, ok := inferenceMap["condition"]
	if !ok {
		return nil, fmt.Errorf("inference.condition is required")
	}
	if condStr, ok := condition.(string); ok {
		result.Condition = condStr
	} else {
		return nil, fmt.Errorf("inference.condition must be a string")
	}

	// Parse from field
	if from, ok := inferenceMap["from"]; ok {
		fromList, err := parseStringArray(from, "inference.from")
		if err != nil {
			return nil, err
		}
		result.From = fromList
	}

	// Parse allowedSystemFunctions field
	if allowedSystemFunctions, ok := inferenceMap["allowedSystemFunctions"]; ok {
		funcList, err := parseStringArray(allowedSystemFunctions, "inference.allowedSystemFunctions")
		if err != nil {
			return nil, err
		}
		result.AllowedSystemFunctions = funcList
	}

	// Parse disableAllSystemFunctions field
	if disableAllSystemFunctions, ok := inferenceMap["disableAllSystemFunctions"]; ok {
		if disableBool, ok := disableAllSystemFunctions.(bool); ok {
			result.DisableAllSystemFunctions = disableBool
		} else {
			return nil, fmt.Errorf("inference.disableAllSystemFunctions must be a boolean")
		}
	}

	// Parse clientIds field (for queryCustomerServiceChats system function)
	if clientIds, ok := inferenceMap["clientIds"]; ok {
		if clientIdsStr, ok := clientIds.(string); ok {
			result.ClientIds = clientIdsStr
		} else {
			return nil, fmt.Errorf("inference.clientIds must be a string (variable reference like $clientIds)")
		}
	}

	// Parse codebaseDirs field (for searchCodebase system function)
	if codebaseDirs, ok := inferenceMap["codebaseDirs"]; ok {
		if codebaseDirsStr, ok := codebaseDirs.(string); ok {
			result.CodebaseDirs = codebaseDirsStr
		} else {
			return nil, fmt.Errorf("inference.codebaseDirs must be a string (variable reference like $getProjectRepos)")
		}
	}

	// Parse documentDbName field (for queryDocuments system function)
	if documentDbName, ok := inferenceMap["documentDbName"]; ok {
		if documentDbNameStr, ok := documentDbName.(string); ok {
			result.DocumentDbName = documentDbNameStr
		} else {
			return nil, fmt.Errorf("inference.documentDbName must be a string (variable reference or literal DB name)")
		}
	}

	// Parse documentEnableGraph field (for queryDocuments system function)
	if documentEnableGraph, ok := inferenceMap["documentEnableGraph"]; ok {
		if enableBool, ok := documentEnableGraph.(bool); ok {
			result.DocumentEnableGraph = enableBool
		} else {
			return nil, fmt.Errorf("inference.documentEnableGraph must be a boolean")
		}
	}

	// Validate contradictory settings
	if result.DisableAllSystemFunctions && len(result.AllowedSystemFunctions) > 0 {
		return nil, fmt.Errorf("inference cannot have both disableAllSystemFunctions=true and allowedSystemFunctions specified")
	}

	return result, nil
}

// validateRunOnlyIfObject validates a RunOnlyIfObject
func validateRunOnlyIfObject(obj *RunOnlyIfObject) (*RunOnlyIfObject, error) {
	if obj == nil {
		return nil, nil
	}

	// Check if at least one condition type is specified
	hasCondition := obj.Condition != ""
	hasDeterministic := obj.Deterministic != ""
	hasInference := obj.Inference != nil

	if !hasCondition && !hasDeterministic && !hasInference {
		return nil, fmt.Errorf("runOnlyIf must specify at least one of: condition, deterministic, or inference")
	}

	// Validate: condition and inference cannot both be set (use inference object instead)
	if hasCondition && hasInference {
		return nil, fmt.Errorf("runOnlyIf cannot have both 'condition' and 'inference' fields. Use only 'inference' for advanced inference mode")
	}

	// Validate: disableAllSystemFunctions and allowedSystemFunctions are contradictory
	if obj.DisableAllSystemFunctions && len(obj.AllowedSystemFunctions) > 0 {
		return nil, fmt.Errorf("runOnlyIf cannot have both disableAllSystemFunctions=true and allowedSystemFunctions specified")
	}

	// Validate: combineWith is only valid when multiple condition types are specified
	if obj.CombineMode != "" {
		conditionCount := 0
		if hasDeterministic {
			conditionCount++
		}
		if hasCondition || hasInference {
			conditionCount++
		}

		if conditionCount < 2 {
			return nil, fmt.Errorf("runOnlyIf.combineWith is only valid when both deterministic and inference conditions are specified")
		}
	}

	// Set default combineWith if not specified and both modes are present
	if obj.CombineMode == "" && hasDeterministic && (hasCondition || hasInference) {
		obj.CombineMode = CombineModeAND // Default to AND
	}

	return obj, nil
}

// parseStringArray is a helper to parse arrays of strings from interface{}
func parseStringArray(value interface{}, fieldName string) ([]string, error) {
	switch v := value.(type) {
	case []interface{}:
		result := make([]string, 0, len(v))
		for _, item := range v {
			if str, ok := item.(string); ok {
				result = append(result, str)
			} else {
				return nil, fmt.Errorf("%s must be an array of strings", fieldName)
			}
		}
		return result, nil
	case []string:
		return v, nil
	default:
		return nil, fmt.Errorf("%s must be an array of strings", fieldName)
	}
}

// parseMetadataValue parses a metadata filter value which can be:
// - A string: exact match (e.g., "ABC123")
// - An object with value and operation: { value: "John", operation: "contains" }
func parseMetadataValue(key string, value interface{}) (interface{}, error) {
	switch v := value.(type) {
	case string:
		// Simple string value - will be exact match
		return v, nil
	case map[string]interface{}:
		return parseMetadataFilterValue(key, v)
	case map[interface{}]interface{}:
		// Convert to map[string]interface{}
		converted := make(map[string]interface{})
		for k, val := range v {
			if kStr, ok := k.(string); ok {
				converted[kStr] = val
			}
		}
		return parseMetadataFilterValue(key, converted)
	default:
		// Other types (int, bool, etc.) - use as-is
		return v, nil
	}
}

// parseMetadataFilterValue parses the object form of metadata filter value
func parseMetadataFilterValue(key string, m map[string]interface{}) (*MetadataFilterValue, error) {
	result := &MetadataFilterValue{}

	// Get "value" field (required)
	if val, ok := m["value"]; ok {
		if strVal, ok := val.(string); ok {
			result.Value = strVal
		} else {
			return nil, fmt.Errorf("memoryFilters.metadata.%s.value must be a string", key)
		}
	} else {
		return nil, fmt.Errorf("memoryFilters.metadata.%s object form requires 'value' field", key)
	}

	// Get "operation" field (optional, defaults to "exact")
	if op, ok := m["operation"]; ok {
		if opStr, ok := op.(string); ok {
			switch MetadataFilterOperation(opStr) {
			case MetadataFilterOperationExact:
				result.Operation = MetadataFilterOperationExact
			case MetadataFilterOperationContains:
				// Validate that this key supports contains
				if !SupportsContainsOperation(key) {
					return nil, fmt.Errorf("memoryFilters.metadata.%s does not support 'contains' operation. Only meeting_with_person and meeting_topic support it", key)
				}
				result.Operation = MetadataFilterOperationContains
			default:
				return nil, fmt.Errorf("memoryFilters.metadata.%s.operation must be 'exact' or 'contains', got '%s'", key, opStr)
			}
		} else {
			return nil, fmt.Errorf("memoryFilters.metadata.%s.operation must be a string", key)
		}
	} else {
		result.Operation = MetadataFilterOperationExact
	}

	return result, nil
}

// parseTopicField parses the topic field which must be an array of topic strings
// Each topic must be a valid memory topic unless it's a variable reference (starts with $)
func parseTopicField(value interface{}) ([]string, error) {
	var topics []string

	switch v := value.(type) {
	case []interface{}:
		topics = make([]string, 0, len(v))
		for _, item := range v {
			if str, ok := item.(string); ok {
				topics = append(topics, str)
			} else {
				return nil, fmt.Errorf("memoryFilters.topic must be an array of strings")
			}
		}
	case []string:
		topics = v
	default:
		return nil, fmt.Errorf("memoryFilters.topic must be an array of strings, got %T", value)
	}

	// Validate each topic
	for _, topic := range topics {
		// Skip validation for variable references (e.g., "$topic")
		if strings.HasPrefix(topic, "$") {
			continue
		}
		if !IsValidMemoryTopic(topic) {
			return nil, fmt.Errorf("memoryFilters.topic '%s' is not valid. Allowed topics: %v", topic, GetAllowedMemoryTopicsList())
		}
	}

	return topics, nil
}

// parseMemoryFiltersFromInterface parses a MemoryFilters object from interface{}
// Supports memoryFilters YAML structure:
//
//	memoryFilters:
//	  topic: ["meeting_transcript", "meeting_chat"]  # Array of topics
//	  metadata:
//	    company_id: "$companyId"                     # Simple string (exact match)
//	    meeting_with_person:                         # Object form with operation
//	      value: "$clientName"
//	      operation: "contains"
func parseMemoryFiltersFromInterface(value interface{}) (*MemoryFilters, error) {
	if value == nil {
		return nil, nil
	}

	var filterMap map[string]interface{}

	switch v := value.(type) {
	case map[string]interface{}:
		filterMap = v
	case map[interface{}]interface{}:
		filterMap = make(map[string]interface{})
		for k, val := range v {
			if kStr, ok := k.(string); ok {
				filterMap[kStr] = val
			}
		}
	default:
		return nil, fmt.Errorf("memoryFilters must be an object, got %T", value)
	}

	result := &MemoryFilters{}

	// Parse and validate topic field (now an array)
	if topic, ok := filterMap["topic"]; ok {
		topics, err := parseTopicField(topic)
		if err != nil {
			return nil, err
		}
		result.Topic = topics
	}

	// Parse and validate metadata field
	if metadata, ok := filterMap["metadata"]; ok {
		var metadataMap map[string]interface{}
		switch m := metadata.(type) {
		case map[string]interface{}:
			metadataMap = m
		case map[interface{}]interface{}:
			metadataMap = make(map[string]interface{})
			for k, val := range m {
				if kStr, ok := k.(string); ok {
					metadataMap[kStr] = val
				}
			}
		default:
			return nil, fmt.Errorf("memoryFilters.metadata must be an object")
		}

		// Validate all metadata keys
		var invalidKeys []string
		for key := range metadataMap {
			if !IsValidMemoryMetadataKey(key) {
				invalidKeys = append(invalidKeys, key)
			}
		}
		if len(invalidKeys) > 0 {
			return nil, fmt.Errorf("memoryFilters.metadata contains invalid key(s): %v. Allowed keys: %v", invalidKeys, GetAllowedMemoryMetadataKeysList())
		}

		// Parse metadata values - can be string (exact match) or object with value/operation
		parsedMetadata := make(map[string]interface{})
		for key, val := range metadataMap {
			parsedVal, err := parseMetadataValue(key, val)
			if err != nil {
				return nil, err
			}
			parsedMetadata[key] = parsedVal
		}
		result.Metadata = parsedMetadata
	}

	// Parse and validate timeRange field
	if timeRange, ok := filterMap["timeRange"]; ok {
		var timeRangeMap map[string]interface{}
		switch tr := timeRange.(type) {
		case map[string]interface{}:
			timeRangeMap = tr
		case map[interface{}]interface{}:
			timeRangeMap = make(map[string]interface{})
			for k, val := range tr {
				if kStr, ok := k.(string); ok {
					timeRangeMap[kStr] = val
				}
			}
		default:
			return nil, fmt.Errorf("memoryFilters.timeRange must be an object")
		}

		result.TimeRange = &MemoryTimeRange{}

		if after, ok := timeRangeMap["after"]; ok {
			if afterStr, ok := after.(string); ok {
				// Validate date format (skip for variable references)
				if !IsValidMemoryDateFormat(afterStr) {
					return nil, fmt.Errorf("memoryFilters.timeRange.after '%s' is not in valid format. Expected: YYYY-MM-DD (e.g., 2024-01-15)", afterStr)
				}
				result.TimeRange.After = afterStr
			} else {
				return nil, fmt.Errorf("memoryFilters.timeRange.after must be a string")
			}
		}

		if before, ok := timeRangeMap["before"]; ok {
			if beforeStr, ok := before.(string); ok {
				// Validate date format (skip for variable references)
				if !IsValidMemoryDateFormat(beforeStr) {
					return nil, fmt.Errorf("memoryFilters.timeRange.before '%s' is not in valid format. Expected: YYYY-MM-DD (e.g., 2024-12-31)", beforeStr)
				}
				result.TimeRange.Before = beforeStr
			} else {
				return nil, fmt.Errorf("memoryFilters.timeRange.before must be a string")
			}
		}

		// timeRange must have at least one of after or before
		if result.TimeRange.After == "" && result.TimeRange.Before == "" {
			return nil, fmt.Errorf("memoryFilters.timeRange must specify at least 'after' or 'before'")
		}
	}

	// Validate that at least one filter is specified
	if len(result.Topic) == 0 && len(result.Metadata) == 0 && result.TimeRange == nil {
		return nil, fmt.Errorf("memoryFilters must specify at least 'topic', 'metadata', or 'timeRange'")
	}

	return result, nil
}

// ParseSuccessCriteria converts the SuccessCriteria field to a SuccessCriteriaObject
func ParseSuccessCriteria(successCriteria interface{}) (*SuccessCriteriaObject, error) {
	if successCriteria == nil {
		return nil, nil
	}

	switch v := successCriteria.(type) {
	case string:
		if v == "" {
			return nil, nil
		}
		return &SuccessCriteriaObject{
			Condition: v,
		}, nil
	case map[interface{}]interface{}:
		result := &SuccessCriteriaObject{}

		if condition, ok := v["condition"]; ok {
			if condStr, ok := condition.(string); ok {
				result.Condition = condStr
			} else {
				return nil, fmt.Errorf("successCriteria.condition must be a string")
			}
		} else {
			return nil, fmt.Errorf("successCriteria object must have a 'condition' field")
		}

		if from, ok := v["from"]; ok {
			switch fromVal := from.(type) {
			case []interface{}:
				for _, f := range fromVal {
					if fromStr, ok := f.(string); ok {
						result.From = append(result.From, fromStr)
					} else {
						return nil, fmt.Errorf("successCriteria.from must be an array of strings")
					}
				}
			case []string:
				result.From = fromVal
			default:
				return nil, fmt.Errorf("successCriteria.from must be an array of strings")
			}
		}

		if allowedSystemFunctions, ok := v["allowedSystemFunctions"]; ok {
			switch allowedVal := allowedSystemFunctions.(type) {
			case []interface{}:
				for _, f := range allowedVal {
					if funcStr, ok := f.(string); ok {
						result.AllowedSystemFunctions = append(result.AllowedSystemFunctions, funcStr)
					} else {
						return nil, fmt.Errorf("successCriteria.allowedSystemFunctions must be an array of strings")
					}
				}
			case []string:
				result.AllowedSystemFunctions = allowedVal
			default:
				return nil, fmt.Errorf("successCriteria.allowedSystemFunctions must be an array of strings")
			}
		}

		if disableAllSystemFunctions, ok := v["disableAllSystemFunctions"]; ok {
			if disableBool, ok := disableAllSystemFunctions.(bool); ok {
				result.DisableAllSystemFunctions = disableBool
			} else {
				return nil, fmt.Errorf("successCriteria.disableAllSystemFunctions must be a boolean")
			}
		}

		// Parse clientIds field (for queryCustomerServiceChats system function)
		if clientIds, ok := v["clientIds"]; ok {
			if clientIdsStr, ok := clientIds.(string); ok {
				result.ClientIds = clientIdsStr
			} else {
				return nil, fmt.Errorf("successCriteria.clientIds must be a string (variable reference like $clientIds)")
			}
		}

		// Parse codebaseDirs field (for searchCodebase system function)
		if codebaseDirs, ok := v["codebaseDirs"]; ok {
			if codebaseDirsStr, ok := codebaseDirs.(string); ok {
				result.CodebaseDirs = codebaseDirsStr
			} else {
				return nil, fmt.Errorf("successCriteria.codebaseDirs must be a string (variable reference like $getProjectRepos)")
			}
		}

		// Parse documentDbName field (for queryDocuments system function)
		if documentDbName, ok := v["documentDbName"]; ok {
			if documentDbNameStr, ok := documentDbName.(string); ok {
				result.DocumentDbName = documentDbNameStr
			} else {
				return nil, fmt.Errorf("successCriteria.documentDbName must be a string (variable reference or literal DB name)")
			}
		}

		// Parse documentEnableGraph field (for queryDocuments system function)
		if documentEnableGraph, ok := v["documentEnableGraph"]; ok {
			if enableBool, ok := documentEnableGraph.(bool); ok {
				result.DocumentEnableGraph = enableBool
			} else {
				return nil, fmt.Errorf("successCriteria.documentEnableGraph must be a boolean")
			}
		}

		// Parse memoryFilters field (for queryMemories system function)
		if memoryFilters, ok := v["memoryFilters"]; ok {
			filters, err := parseMemoryFiltersFromInterface(memoryFilters)
			if err != nil {
				return nil, fmt.Errorf("error parsing successCriteria.memoryFilters: %w", err)
			}
			result.MemoryFilters = filters
		}

		// Validate: disableAllSystemFunctions and allowedSystemFunctions are contradictory
		if result.DisableAllSystemFunctions && len(result.AllowedSystemFunctions) > 0 {
			return nil, fmt.Errorf("successCriteria cannot have both disableAllSystemFunctions=true and allowedSystemFunctions specified")
		}

		return result, nil
	case map[string]interface{}:
		result := &SuccessCriteriaObject{}

		if condition, ok := v["condition"]; ok {
			if condStr, ok := condition.(string); ok {
				result.Condition = condStr
			} else {
				return nil, fmt.Errorf("successCriteria.condition must be a string")
			}
		} else {
			return nil, fmt.Errorf("successCriteria object must have a 'condition' field")
		}

		if from, ok := v["from"]; ok {
			switch fromVal := from.(type) {
			case []interface{}:
				for _, f := range fromVal {
					if fromStr, ok := f.(string); ok {
						result.From = append(result.From, fromStr)
					} else {
						return nil, fmt.Errorf("successCriteria.from must be an array of strings")
					}
				}
			case []string:
				result.From = fromVal
			default:
				return nil, fmt.Errorf("successCriteria.from must be an array of strings")
			}
		}

		if allowedSystemFunctions, ok := v["allowedSystemFunctions"]; ok {
			switch allowedVal := allowedSystemFunctions.(type) {
			case []interface{}:
				for _, f := range allowedVal {
					if funcStr, ok := f.(string); ok {
						result.AllowedSystemFunctions = append(result.AllowedSystemFunctions, funcStr)
					} else {
						return nil, fmt.Errorf("successCriteria.allowedSystemFunctions must be an array of strings")
					}
				}
			case []string:
				result.AllowedSystemFunctions = allowedVal
			default:
				return nil, fmt.Errorf("successCriteria.allowedSystemFunctions must be an array of strings")
			}
		}

		if disableAllSystemFunctions, ok := v["disableAllSystemFunctions"]; ok {
			if disableBool, ok := disableAllSystemFunctions.(bool); ok {
				result.DisableAllSystemFunctions = disableBool
			} else {
				return nil, fmt.Errorf("successCriteria.disableAllSystemFunctions must be a boolean")
			}
		}

		// Parse clientIds field (for queryCustomerServiceChats system function)
		if clientIds, ok := v["clientIds"]; ok {
			if clientIdsStr, ok := clientIds.(string); ok {
				result.ClientIds = clientIdsStr
			} else {
				return nil, fmt.Errorf("successCriteria.clientIds must be a string (variable reference like $clientIds)")
			}
		}

		// Parse codebaseDirs field (for searchCodebase system function)
		if codebaseDirs, ok := v["codebaseDirs"]; ok {
			if codebaseDirsStr, ok := codebaseDirs.(string); ok {
				result.CodebaseDirs = codebaseDirsStr
			} else {
				return nil, fmt.Errorf("successCriteria.codebaseDirs must be a string (variable reference like $getProjectRepos)")
			}
		}

		// Parse documentDbName field (for queryDocuments system function)
		if documentDbName, ok := v["documentDbName"]; ok {
			if documentDbNameStr, ok := documentDbName.(string); ok {
				result.DocumentDbName = documentDbNameStr
			} else {
				return nil, fmt.Errorf("successCriteria.documentDbName must be a string (variable reference or literal DB name)")
			}
		}

		// Parse documentEnableGraph field (for queryDocuments system function)
		if documentEnableGraph, ok := v["documentEnableGraph"]; ok {
			if enableBool, ok := documentEnableGraph.(bool); ok {
				result.DocumentEnableGraph = enableBool
			} else {
				return nil, fmt.Errorf("successCriteria.documentEnableGraph must be a boolean")
			}
		}

		// Parse memoryFilters field (for queryMemories system function)
		if memoryFilters, ok := v["memoryFilters"]; ok {
			filters, err := parseMemoryFiltersFromInterface(memoryFilters)
			if err != nil {
				return nil, fmt.Errorf("error parsing successCriteria.memoryFilters: %w", err)
			}
			result.MemoryFilters = filters
		}

		// Validate: disableAllSystemFunctions and allowedSystemFunctions are contradictory
		if result.DisableAllSystemFunctions && len(result.AllowedSystemFunctions) > 0 {
			return nil, fmt.Errorf("successCriteria cannot have both disableAllSystemFunctions=true and allowedSystemFunctions specified")
		}

		return result, nil
	case *SuccessCriteriaObject:
		return v, nil
	default:
		return nil, fmt.Errorf("successCriteria must be a string or object, got %v", reflect.TypeOf(successCriteria))
	}
}

// parseOnError converts a map to an OnError object
func parseOnError(onErrorMap map[interface{}]interface{}) (*OnError, error) {
	result := &OnError{}

	if strategy, ok := onErrorMap["strategy"]; ok {
		if strategyStr, ok := strategy.(string); ok {
			result.Strategy = strategyStr
		} else {
			return nil, fmt.Errorf("onError.strategy must be a string")
		}
	}

	if message, ok := onErrorMap["message"]; ok {
		if messageStr, ok := message.(string); ok {
			result.Message = messageStr
		} else {
			return nil, fmt.Errorf("onError.message must be a string")
		}
	}

	// Handle call field - can be string or object
	if call, ok := onErrorMap["call"]; ok {
		switch v := call.(type) {
		case string:
			if v != "" {
				result.Call = &FunctionCall{Name: v}
			}
		case map[interface{}]interface{}:
			fc := &FunctionCall{}
			if name, ok := v["name"].(string); ok {
				fc.Name = name
			}
			if params, ok := v["params"].(map[interface{}]interface{}); ok {
				fc.Params = make(map[string]interface{})
				for k, val := range params {
					if keyStr, ok := k.(string); ok {
						fc.Params[keyStr] = val
					}
				}
			}
			if fc.Name != "" {
				result.Call = fc
			}
		default:
			return nil, fmt.Errorf("onError.call must be a string or object with name field")
		}
	}

	// Handle with field
	if withVal, ok := onErrorMap["with"]; ok {
		if withMap, ok := withVal.(map[interface{}]interface{}); ok {
			parsedWith, err := parseInputWith(withMap)
			if err != nil {
				return nil, fmt.Errorf("error parsing onError.with: %w", err)
			}
			result.With = parsedWith
		}
	}

	// Validate: must have at least strategy or call
	if result.Strategy == "" && result.Call == nil {
		return nil, fmt.Errorf("onError must have either 'strategy' or 'call' field")
	}

	return result, nil
}

// parseOnErrorFromStringMap is similar to parseOnError but for map[string]interface{}
func parseOnErrorFromStringMap(onErrorMap map[string]interface{}) (*OnError, error) {
	result := &OnError{}

	if strategy, ok := onErrorMap["strategy"]; ok {
		if strategyStr, ok := strategy.(string); ok {
			result.Strategy = strategyStr
		} else {
			return nil, fmt.Errorf("onError.strategy must be a string")
		}
	}

	if message, ok := onErrorMap["message"]; ok {
		if messageStr, ok := message.(string); ok {
			result.Message = messageStr
		} else {
			return nil, fmt.Errorf("onError.message must be a string")
		}
	}

	// Handle call field - can be string or object
	if call, ok := onErrorMap["call"]; ok {
		switch v := call.(type) {
		case string:
			if v != "" {
				result.Call = &FunctionCall{Name: v}
			}
		case map[string]interface{}:
			fc := &FunctionCall{}
			if name, ok := v["name"].(string); ok {
				fc.Name = name
			}
			if params, ok := v["params"].(map[string]interface{}); ok {
				fc.Params = params
			}
			if fc.Name != "" {
				result.Call = fc
			}
		default:
			return nil, fmt.Errorf("onError.call must be a string or object with name field")
		}
	}

	// Handle with field
	if withVal, ok := onErrorMap["with"]; ok {
		if withMap, ok := withVal.(map[string]interface{}); ok {
			parsedWith, err := parseInputWithFromStringMap(withMap)
			if err != nil {
				return nil, fmt.Errorf("error parsing onError.with: %w", err)
			}
			result.With = parsedWith
		}
	}

	// Validate: must have at least strategy or call
	if result.Strategy == "" && result.Call == nil {
		return nil, fmt.Errorf("onError must have either 'strategy' or 'call' field")
	}

	return result, nil
}

// parseInputWith converts a map to an InputWithOptions object
func parseInputWith(withMap map[interface{}]interface{}) (*InputWithOptions, error) {
	result := &InputWithOptions{}

	if oneOf, ok := withMap["oneOf"]; ok {
		if oneOfStr, ok := oneOf.(string); ok {
			result.OneOf = oneOfStr
		}
	}

	if manyOf, ok := withMap["manyOf"]; ok {
		if manyOfStr, ok := manyOf.(string); ok {
			result.ManyOf = manyOfStr
		}
	}

	if notOneOf, ok := withMap["notOneOf"]; ok {
		if notOneOfStr, ok := notOneOf.(string); ok {
			result.NotOneOf = notOneOfStr
		}
	}

	if ttl, ok := withMap["ttl"]; ok {
		if ttlInt, ok := ttl.(int); ok {
			result.TTL = ttlInt
		}
	}

	return result, nil
}

// parseInputWithFromStringMap is similar to parseInputWith but for map[string]interface{}
func parseInputWithFromStringMap(withMap map[string]interface{}) (*InputWithOptions, error) {
	result := &InputWithOptions{}

	if oneOf, ok := withMap["oneOf"]; ok {
		if oneOfStr, ok := oneOf.(string); ok {
			result.OneOf = oneOfStr
		}
	}

	if manyOf, ok := withMap["manyOf"]; ok {
		if manyOfStr, ok := manyOf.(string); ok {
			result.ManyOf = manyOfStr
		}
	}

	if notOneOf, ok := withMap["notOneOf"]; ok {
		if notOneOfStr, ok := notOneOf.(string); ok {
			result.NotOneOf = notOneOfStr
		}
	}

	if ttl, ok := withMap["ttl"]; ok {
		if ttlInt, ok := ttl.(int); ok {
			result.TTL = ttlInt
		}
	}

	return result, nil
}

// GetRunOnlyIfCondition returns the condition string from RunOnlyIf field
func GetRunOnlyIfCondition(runOnlyIf interface{}) string {
	if runOnlyIf == nil {
		return ""
	}

	switch v := runOnlyIf.(type) {
	case string:
		return v
	case *RunOnlyIfObject:
		if v != nil {
			return v.Condition
		}
	}

	// Try to parse it
	obj, err := ParseRunOnlyIf(runOnlyIf)
	if err == nil && obj != nil {
		return obj.Condition
	}

	return ""
}

// GetSuccessCriteriaCondition returns the condition string from SuccessCriteria field
func GetSuccessCriteriaCondition(successCriteria interface{}) string {
	if successCriteria == nil {
		return ""
	}

	switch v := successCriteria.(type) {
	case string:
		return v
	case *SuccessCriteriaObject:
		if v != nil {
			return v.Condition
		}
	}

	// Try to parse it
	obj, err := ParseSuccessCriteria(successCriteria)
	if err == nil && obj != nil {
		return obj.Condition
	}

	return ""
}

// ParseBreakIf converts the BreakIf field to either a string or SuccessCriteriaObject
func ParseBreakIf(breakIf interface{}) (string, *SuccessCriteriaObject, error) {
	if breakIf == nil {
		return "", nil, nil
	}

	switch v := breakIf.(type) {
	case string:
		if v == "" {
			return "", nil, nil
		}
		// Deterministic break condition (simple string comparison)
		return v, nil, nil
	case map[interface{}]interface{}:
		// Inference break condition (agentic)
		obj, err := ParseSuccessCriteria(breakIf)
		if err != nil {
			return "", nil, fmt.Errorf("error parsing breakIf object: %w", err)
		}
		return "", obj, nil
	case map[string]interface{}:
		// Inference break condition (agentic) - already parsed
		obj, err := ParseSuccessCriteria(breakIf)
		if err != nil {
			return "", nil, fmt.Errorf("error parsing breakIf object: %w", err)
		}
		return "", obj, nil
	case *SuccessCriteriaObject:
		return "", v, nil
	default:
		return "", nil, fmt.Errorf("breakIf must be a string or object, got %v", reflect.TypeOf(breakIf))
	}
}

// GetBreakIfCondition returns the condition string from BreakIf field
func GetBreakIfCondition(breakIf interface{}) string {
	if breakIf == nil {
		return ""
	}

	switch v := breakIf.(type) {
	case string:
		return v
	case *SuccessCriteriaObject:
		if v != nil {
			return v.Condition
		}
	}

	// Try to parse it
	strCond, objCond, err := ParseBreakIf(breakIf)
	if err == nil {
		if strCond != "" {
			return strCond
		}
		if objCond != nil {
			return objCond.Condition
		}
	}

	return ""
}

// ================================================================================
// Deterministic Expression Evaluation
// ================================================================================

// EvaluateDeterministicExpression evaluates a deterministic expression with && and || operators
// Supports: ==, !=, >, <, >=, <=
// Examples:
//   - "true == true && 150 > 100"
//   - "'premium' == 'premium' || true == true"
//
// IMPORTANT: This function expects literal values only. All variable references
// ($functionName.field) should be replaced by yaml_defined_tool using VariableReplacer
// BEFORE calling this function. This maintains proper separation of concerns:
// - tool-protocol: owns expression parsing and evaluation logic
// - tool_engine: owns variable replacement and data access logic
//
// Parameters:
//   - expression: the condition string with logical operators and literal values
//
// Returns: (result bool, error)
func EvaluateDeterministicExpression(expression string) (bool, error) {
	if expression == "" {
		return false, fmt.Errorf("expression cannot be empty")
	}

	// Split by || (OR has lower precedence than AND)
	orParts := splitByLogicalOperator(expression, "||")

	for _, orPart := range orParts {
		// Split by && (AND has higher precedence than OR)
		andParts := splitByLogicalOperator(orPart, "&&")

		allTrue := true
		for _, andPart := range andParts {
			// Evaluate individual condition
			result, err := evaluateSingleCondition(strings.TrimSpace(andPart))
			if err != nil {
				return false, fmt.Errorf("error evaluating condition '%s': %w", andPart, err)
			}
			if !result {
				allTrue = false
				break
			}
		}

		// If all AND conditions are true, the entire OR succeeds
		if allTrue {
			return true, nil
		}
	}

	// None of the OR branches succeeded
	return false, nil
}

// splitByLogicalOperator splits a string by a logical operator (&&  or ||)
// while respecting parentheses
func splitByLogicalOperator(expression string, operator string) []string {
	var parts []string
	var currentPart strings.Builder
	i := 0
	parenDepth := 0

	for i < len(expression) {
		char := expression[i]

		// Track parenthesis depth
		if char == '(' {
			parenDepth++
			currentPart.WriteByte(char)
			i++
			continue
		} else if char == ')' {
			parenDepth--
			currentPart.WriteByte(char)
			i++
			continue
		}

		// Only split if we're not inside parentheses
		if parenDepth == 0 && i+len(operator) <= len(expression) && expression[i:i+len(operator)] == operator {
			// Found operator at depth 0 - save current part and move past operator
			parts = append(parts, strings.TrimSpace(currentPart.String()))
			currentPart.Reset()
			i += len(operator)
		} else {
			// Regular character
			currentPart.WriteByte(char)
			i++
		}
	}

	// Don't forget the last part
	if currentPart.Len() > 0 {
		parts = append(parts, strings.TrimSpace(currentPart.String()))
	}

	return parts
}

// evaluateSingleCondition evaluates a single comparison condition with literal values
// Supports: ==, !=, >, <, >=, <=
// Also supports built-in functions: len(), isEmpty(), contains(), exists()
// Example: "true == true", "'premium' == 'premium'", "150 > 100", "len([1,2,3]) > 0"
func evaluateSingleCondition(condition string) (bool, error) {
	// Strip outer parentheses if present
	trimmed := strings.TrimSpace(condition)
	if strings.HasPrefix(trimmed, "(") && strings.HasSuffix(trimmed, ")") {
		// Remove outer parentheses and recursively evaluate
		innerCondition := trimmed[1 : len(trimmed)-1]
		return EvaluateDeterministicExpression(innerCondition)
	}

	// Operators in order of precedence (check longest first to avoid partial matches)
	operators := []struct {
		op   string
		eval func(left, right string) (bool, error)
	}{
		{"==", compareEqual},
		{"!=", compareNotEqual},
		{">=", compareGreaterOrEqual},
		{"<=", compareLessOrEqual},
		{">", compareGreater},
		{"<", compareLess},
	}

	for _, opInfo := range operators {
		// Find operator position, but be careful with operators inside function arguments
		opIndex := findOperatorOutsideFunctions(condition, opInfo.op)
		if opIndex >= 0 {
			left := strings.TrimSpace(condition[:opIndex])
			right := strings.TrimSpace(condition[opIndex+len(opInfo.op):])

			// Evaluate built-in functions on both sides if present
			leftValue, err := evaluateOperand(left)
			if err != nil {
				return false, fmt.Errorf("error evaluating left operand '%s': %w", left, err)
			}

			rightValue, err := evaluateOperand(right)
			if err != nil {
				return false, fmt.Errorf("error evaluating right operand '%s': %w", right, err)
			}

			// Evaluate the comparison
			return opInfo.eval(leftValue, rightValue)
		}
	}

	return false, fmt.Errorf("no valid operator found in condition: %s", condition)
}

// findOperatorOutsideFunctions finds the position of an operator that is not inside function parentheses
// Returns -1 if not found
func findOperatorOutsideFunctions(condition string, operator string) int {
	parenDepth := 0
	bracketDepth := 0
	inString := false
	stringChar := byte(0)

	for i := 0; i < len(condition); i++ {
		char := condition[i]

		// Handle string delimiters
		if char == '\'' || char == '"' {
			if !inString {
				inString = true
				stringChar = char
			} else if char == stringChar && (i == 0 || condition[i-1] != '\\') {
				inString = false
			}
			continue
		}

		// Skip everything inside strings
		if inString {
			continue
		}

		switch char {
		case '(':
			parenDepth++
		case ')':
			parenDepth--
		case '[':
			bracketDepth++
		case ']':
			bracketDepth--
		}

		// Only check for operator when we're at depth 0 (not inside parentheses or brackets)
		if parenDepth == 0 && bracketDepth == 0 {
			if i+len(operator) <= len(condition) && condition[i:i+len(operator)] == operator {
				return i
			}
		}
	}

	return -1
}

// evaluateOperand evaluates an operand which may be a literal value or a built-in function call
// Returns the evaluated value as a string
func evaluateOperand(operand string) (string, error) {
	operand = strings.TrimSpace(operand)

	// Check if it's a built-in function call
	if result, handled, err := tryEvaluateBuiltinFunction(operand); handled {
		if err != nil {
			return "", err
		}
		return result, nil
	}

	// Not a function call, return as literal (remove quotes if present)
	return strings.Trim(operand, "'\""), nil
}

// tryEvaluateBuiltinFunction attempts to evaluate a built-in function
// Returns (result, handled, error) where handled indicates if it was a function call
func tryEvaluateBuiltinFunction(expr string) (string, bool, error) {
	expr = strings.TrimSpace(expr)

	// Check for supported built-in functions
	builtinFunctions := []struct {
		name string
		eval func(args string) (string, error)
	}{
		{"len", evaluateLenFunction},
		{"isEmpty", evaluateIsEmptyFunction},
		{"contains", evaluateContainsFunction},
		{"exists", evaluateExistsFunction},
		{"coalesce", evaluateCoalesceFunction},
		{"date", evaluateDateFunction},
	}

	for _, fn := range builtinFunctions {
		prefix := fn.name + "("
		if strings.HasPrefix(expr, prefix) && strings.HasSuffix(expr, ")") {
			// Extract arguments (everything between the parentheses)
			argsStart := len(prefix)
			argsEnd := len(expr) - 1
			args := expr[argsStart:argsEnd]
			result, err := fn.eval(args)
			return result, true, err
		}
	}

	return "", false, nil
}

// EvaluateBuiltinFunction attempts to evaluate a built-in function expression.
// This is the public API for evaluating functions like len(), isEmpty(), contains(), exists(), coalesce().
// Returns (result, handled, error) where handled indicates if it was a recognized function call.
// If handled is false, the expression was not a function call and should be used as-is.
func EvaluateBuiltinFunction(expr string) (string, bool, error) {
	return tryEvaluateBuiltinFunction(expr)
}

// ================================================================================
// Built-in Functions for Deterministic Expressions
// ================================================================================

// evaluateLenFunction evaluates len(value) and returns the length as a string
// Supports: arrays (JSON), strings, and objects (returns number of keys)
// Examples:
//   - len([1,2,3]) → "3"
//   - len("hello") → "5"
//   - len([]) → "0"
//   - len(null) → "0"
func evaluateLenFunction(args string) (string, error) {
	args = strings.TrimSpace(args)

	// Handle null
	if args == "null" || args == "" {
		return "0", nil
	}

	// Handle Go's empty slice/map string representations from fmt.Sprintf("%v", value)
	// These come from VariableReplacer when replacing $functionName with actual values
	if args == "map[]" || args == "[]" {
		return "0", nil
	}

	// Handle Go's non-empty map representation: map[key1:value1 key2:value2]
	if strings.HasPrefix(args, "map[") && strings.HasSuffix(args, "]") {
		// Count the number of key:value pairs by counting colons at depth 0
		inner := args[4 : len(args)-1] // Remove "map[" and "]"
		if inner == "" {
			return "0", nil
		}
		// Count entries by splitting on spaces (simplified - works for most cases)
		count := countMapEntries(inner)
		return strconv.Itoa(count), nil
	}

	// Handle quoted strings - measure the string length
	if (strings.HasPrefix(args, "'") && strings.HasSuffix(args, "'")) ||
		(strings.HasPrefix(args, "\"") && strings.HasSuffix(args, "\"")) {
		// Remove quotes and return string length
		inner := args[1 : len(args)-1]
		return strconv.Itoa(len(inner)), nil
	}

	// Handle JSON arrays
	if strings.HasPrefix(args, "[") {
		// Parse as JSON array
		var arr []interface{}
		if err := parseJSONValue(args, &arr); err != nil {
			// If parsing as array fails, it might be an empty or malformed array
			// Try to count elements manually for simple cases
			if args == "[]" {
				return "0", nil
			}
			return "", fmt.Errorf("len(): invalid array format: %s", args)
		}
		return strconv.Itoa(len(arr)), nil
	}

	// Handle JSON objects - return number of keys
	if strings.HasPrefix(args, "{") {
		var obj map[string]interface{}
		if err := parseJSONValue(args, &obj); err != nil {
			if args == "{}" {
				return "0", nil
			}
			return "", fmt.Errorf("len(): invalid object format: %s", args)
		}
		return strconv.Itoa(len(obj)), nil
	}

	// Handle plain strings (unquoted)
	return strconv.Itoa(len(args)), nil
}

// countMapEntries counts the number of entries in a Go map string representation
// e.g., "key1:value1 key2:value2" → 2
func countMapEntries(inner string) int {
	if inner == "" {
		return 0
	}

	count := 0
	depth := 0
	inEntry := false

	for i := 0; i < len(inner); i++ {
		char := inner[i]

		switch char {
		case '[', '{':
			depth++
			inEntry = true
		case ']', '}':
			depth--
		case ':':
			if depth == 0 && !inEntry {
				inEntry = true
			}
		case ' ':
			if depth == 0 && inEntry {
				count++
				inEntry = false
			}
		default:
			if depth == 0 && !inEntry {
				inEntry = true
			}
		}
	}

	// Count the last entry if we were in one
	if inEntry {
		count++
	}

	return count
}

// evaluateIsEmptyFunction evaluates isEmpty(value) and returns "true" or "false"
// Returns true for: null, empty string, empty array [], empty object {}
// Examples:
//   - isEmpty([]) → "true"
//   - isEmpty("") → "true"
//   - isEmpty(null) → "true"
//   - isEmpty([1,2]) → "false"
//   - isEmpty("hello") → "false"
func evaluateIsEmptyFunction(args string) (string, error) {
	args = strings.TrimSpace(args)

	// Handle null
	if args == "null" || args == "" {
		return "true", nil
	}

	// Handle Go's empty slice/map string representations from fmt.Sprintf("%v", value)
	if args == "map[]" || args == "[]" {
		return "true", nil
	}

	// Handle Go's non-empty map representation: map[key1:value1 key2:value2]
	if strings.HasPrefix(args, "map[") && strings.HasSuffix(args, "]") {
		inner := args[4 : len(args)-1]
		if inner == "" {
			return "true", nil
		}
		return "false", nil
	}

	// Handle quoted empty strings
	if args == "''" || args == "\"\"" {
		return "true", nil
	}

	// Handle quoted non-empty strings
	if (strings.HasPrefix(args, "'") && strings.HasSuffix(args, "'")) ||
		(strings.HasPrefix(args, "\"") && strings.HasSuffix(args, "\"")) {
		inner := args[1 : len(args)-1]
		if inner == "" {
			return "true", nil
		}
		return "false", nil
	}

	// Handle empty array
	if args == "[]" {
		return "true", nil
	}

	// Handle empty object
	if args == "{}" {
		return "true", nil
	}

	// Handle JSON arrays
	if strings.HasPrefix(args, "[") {
		var arr []interface{}
		if err := parseJSONValue(args, &arr); err != nil {
			return "", fmt.Errorf("isEmpty(): invalid array format: %s", args)
		}
		if len(arr) == 0 {
			return "true", nil
		}
		return "false", nil
	}

	// Handle JSON objects
	if strings.HasPrefix(args, "{") {
		var obj map[string]interface{}
		if err := parseJSONValue(args, &obj); err != nil {
			return "", fmt.Errorf("isEmpty(): invalid object format: %s", args)
		}
		if len(obj) == 0 {
			return "true", nil
		}
		return "false", nil
	}

	// Non-empty value
	return "false", nil
}

// evaluateContainsFunction evaluates contains(haystack, needle) and returns "true" or "false"
// For strings: checks if haystack contains needle as substring
// For arrays: checks if array contains the needle element
// Examples:
//   - contains("hello world", "world") → "true"
//   - contains([1,2,3], 2) → "true"
//   - contains(["a","b","c"], "b") → "true"
//   - contains("hello", "xyz") → "false"
func evaluateContainsFunction(args string) (string, error) {
	// Parse the two arguments: contains(haystack, needle)
	haystack, needle, err := parseTwoArguments(args)
	if err != nil {
		return "", fmt.Errorf("contains(): %w", err)
	}

	haystack = strings.TrimSpace(haystack)
	needle = strings.TrimSpace(needle)

	// Handle null haystack
	if haystack == "null" {
		return "false", nil
	}

	// Handle JSON arrays
	if strings.HasPrefix(haystack, "[") {
		var arr []interface{}
		if err := parseJSONValue(haystack, &arr); err != nil {
			return "", fmt.Errorf("contains(): invalid array format: %s", haystack)
		}

		// Clean up needle for comparison
		needleClean := strings.Trim(needle, "'\"")

		for _, item := range arr {
			itemStr := fmt.Sprintf("%v", item)
			if itemStr == needleClean {
				return "true", nil
			}
			// Also check if item is a string that matches
			if str, ok := item.(string); ok && str == needleClean {
				return "true", nil
			}
		}
		return "false", nil
	}

	// Handle strings - check if haystack contains needle as substring
	haystackClean := strings.Trim(haystack, "'\"")
	needleClean := strings.Trim(needle, "'\"")

	if strings.Contains(haystackClean, needleClean) {
		return "true", nil
	}
	return "false", nil
}

// evaluateExistsFunction evaluates exists(value) and returns "true" or "false"
// Returns true if the value is not null and not an empty string
// Examples:
//   - exists(null) → "false"
//   - exists("") → "false"
//   - exists("hello") → "true"
//   - exists([]) → "true" (empty array exists, it's not null)
//   - exists(0) → "true" (zero is a valid value)
func evaluateExistsFunction(args string) (string, error) {
	args = strings.TrimSpace(args)

	// Handle null - does not exist
	if args == "null" {
		return "false", nil
	}

	// Handle empty unquoted string - does not exist
	if args == "" {
		return "false", nil
	}

	// Handle quoted empty string - does not exist
	if args == "''" || args == "\"\"" {
		return "false", nil
	}

	// Everything else exists (including empty arrays, empty objects, zero, false, etc.)
	return "true", nil
}

// evaluateCoalesceFunction evaluates coalesce(val1, val2, ...) and returns the first non-null, non-empty value
// This is useful for providing fallback values in deterministic expressions.
// Examples:
//   - coalesce(null, "default") → "default"
//   - coalesce("value", "fallback") → "value"
//   - coalesce(null, null, "third") → "third"
//   - coalesce("", "fallback") → "fallback" (empty string treated as null)
//   - coalesce(null, null) → "null" (all values null)
func evaluateCoalesceFunction(args string) (string, error) {
	args = strings.TrimSpace(args)

	// Parse arguments respecting quotes, parentheses, brackets, and braces
	parsedArgs := parseCoalesceArgs(args)

	for _, arg := range parsedArgs {
		trimmed := strings.TrimSpace(arg)

		// Evaluate the argument (handles nested function calls)
		evaluated, err := evaluateOperand(trimmed)
		if err != nil {
			return "", fmt.Errorf("coalesce: error evaluating argument '%s': %w", trimmed, err)
		}

		// Skip null, NULL, and empty values
		if evaluated == "" || evaluated == "null" || evaluated == "NULL" {
			continue
		}

		// Found a non-null value
		return evaluated, nil
	}

	// All values were null/empty
	return "null", nil
}

// parseCoalesceArgs splits coalesce arguments respecting quotes, parentheses, brackets, and braces
func parseCoalesceArgs(argsStr string) []string {
	var args []string
	var current strings.Builder
	parenDepth := 0
	bracketDepth := 0
	braceDepth := 0
	inQuote := false
	quoteChar := rune(0)

	for i, ch := range argsStr {
		switch {
		case (ch == '"' || ch == '\'') && (i == 0 || argsStr[i-1] != '\\'):
			if !inQuote {
				inQuote = true
				quoteChar = ch
			} else if ch == quoteChar {
				inQuote = false
			}
			current.WriteRune(ch)
		case ch == '(' && !inQuote:
			parenDepth++
			current.WriteRune(ch)
		case ch == ')' && !inQuote:
			parenDepth--
			current.WriteRune(ch)
		case ch == '[' && !inQuote:
			bracketDepth++
			current.WriteRune(ch)
		case ch == ']' && !inQuote:
			bracketDepth--
			current.WriteRune(ch)
		case ch == '{' && !inQuote:
			braceDepth++
			current.WriteRune(ch)
		case ch == '}' && !inQuote:
			braceDepth--
			current.WriteRune(ch)
		case ch == ',' && parenDepth == 0 && bracketDepth == 0 && braceDepth == 0 && !inQuote:
			args = append(args, current.String())
			current.Reset()
		default:
			current.WriteRune(ch)
		}
	}

	// Add the last argument if there is one
	if current.Len() > 0 || len(argsStr) > 0 {
		args = append(args, current.String())
	}

	return args
}

// evaluateDateFunction extracts the date portion from a datetime string
// Auto-detects format and preserves original separator and order
// Examples:
//   - date("31/12/2024 14:30:00") → "31/12/2024"
//   - date("2024-12-31 14:30") → "2024-12-31"
//   - date("24/12/31 10:00:00") → "24/12/31"
//   - date("31.12.2024 14:30") → "31.12.2024"
//   - date("31/12/2024") → "31/12/2024" (already date only)
func evaluateDateFunction(args string) (string, error) {
	args = strings.TrimSpace(args)

	// Handle null or empty
	if args == "" || args == "null" {
		return "", nil
	}

	// Remove surrounding quotes if present
	if (strings.HasPrefix(args, "'") && strings.HasSuffix(args, "'")) ||
		(strings.HasPrefix(args, "\"") && strings.HasSuffix(args, "\"")) {
		args = args[1 : len(args)-1]
	}

	// Handle empty after removing quotes
	if args == "" {
		return "", nil
	}

	// Try to find the separator between date and time
	// Common separators: space, 'T' (ISO format)
	separators := []string{" ", "T"}

	for _, sep := range separators {
		if idx := strings.Index(args, sep); idx != -1 {
			datePart := args[:idx]
			// Validate that we have a date-like structure (contains / - or .)
			if strings.ContainsAny(datePart, "/-.") || isNumericDate(datePart) {
				return datePart, nil
			}
		}
	}

	// No time separator found - check if it's already a date-only value
	// A date-only value should contain date separators (/, -, .) or be numeric
	if strings.ContainsAny(args, "/-.") || isNumericDate(args) {
		return args, nil
	}

	// Return as-is if we can't determine the format
	return args, nil
}

// isNumericDate checks if a string looks like a numeric date (e.g., "20241231")
func isNumericDate(s string) bool {
	if len(s) < 6 || len(s) > 8 {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// ================================================================================
// Helper functions for built-in function evaluation
// ================================================================================

// parseJSONValue parses a JSON string into the provided target
func parseJSONValue(jsonStr string, target interface{}) error {
	// Use encoding/json for proper parsing
	decoder := strings.NewReader(jsonStr)
	return decodeJSON(decoder, target)
}

// decodeJSON is a helper that decodes JSON from a reader
func decodeJSON(reader *strings.Reader, target interface{}) error {
	// Simple JSON parsing using standard library approach
	jsonBytes := make([]byte, reader.Len())
	_, err := reader.Read(jsonBytes)
	if err != nil {
		return err
	}

	// Use reflect to handle different target types
	switch v := target.(type) {
	case *[]interface{}:
		return parseJSONArray(string(jsonBytes), v)
	case *map[string]interface{}:
		return parseJSONObject(string(jsonBytes), v)
	default:
		return fmt.Errorf("unsupported target type")
	}
}

// parseJSONArray parses a JSON array string
func parseJSONArray(jsonStr string, target *[]interface{}) error {
	jsonStr = strings.TrimSpace(jsonStr)
	if !strings.HasPrefix(jsonStr, "[") || !strings.HasSuffix(jsonStr, "]") {
		return fmt.Errorf("not a valid JSON array")
	}

	// Remove brackets
	inner := strings.TrimSpace(jsonStr[1 : len(jsonStr)-1])
	if inner == "" {
		*target = []interface{}{}
		return nil
	}

	// Parse elements (simple parsing, handles nested structures)
	elements := splitJSONElements(inner)
	result := make([]interface{}, 0, len(elements))

	for _, elem := range elements {
		elem = strings.TrimSpace(elem)
		if elem == "" {
			continue
		}

		// Parse the element value
		parsed := parseJSONElement(elem)
		result = append(result, parsed)
	}

	*target = result
	return nil
}

// parseJSONObject parses a JSON object string
func parseJSONObject(jsonStr string, target *map[string]interface{}) error {
	jsonStr = strings.TrimSpace(jsonStr)
	if !strings.HasPrefix(jsonStr, "{") || !strings.HasSuffix(jsonStr, "}") {
		return fmt.Errorf("not a valid JSON object")
	}

	// Remove braces
	inner := strings.TrimSpace(jsonStr[1 : len(jsonStr)-1])
	if inner == "" {
		*target = map[string]interface{}{}
		return nil
	}

	// Parse key-value pairs
	result := make(map[string]interface{})
	pairs := splitJSONElements(inner)

	for _, pair := range pairs {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}

		// Find the colon separator (outside of quotes)
		colonIdx := findColonOutsideQuotes(pair)
		if colonIdx < 0 {
			continue
		}

		key := strings.TrimSpace(pair[:colonIdx])
		value := strings.TrimSpace(pair[colonIdx+1:])

		// Remove quotes from key
		key = strings.Trim(key, "\"'")

		// Parse the value
		result[key] = parseJSONElement(value)
	}

	*target = result
	return nil
}

// splitJSONElements splits JSON elements by comma, respecting nesting
func splitJSONElements(s string) []string {
	var elements []string
	var current strings.Builder
	depth := 0
	inString := false
	stringChar := rune(0)

	for i, char := range s {
		switch {
		case char == '"' || char == '\'':
			if !inString {
				inString = true
				stringChar = char
			} else if char == stringChar && (i == 0 || s[i-1] != '\\') {
				inString = false
			}
			current.WriteRune(char)
		case inString:
			current.WriteRune(char)
		case char == '[' || char == '{':
			depth++
			current.WriteRune(char)
		case char == ']' || char == '}':
			depth--
			current.WriteRune(char)
		case char == ',' && depth == 0:
			elements = append(elements, current.String())
			current.Reset()
		default:
			current.WriteRune(char)
		}
	}

	if current.Len() > 0 {
		elements = append(elements, current.String())
	}

	return elements
}

// findColonOutsideQuotes finds the first colon that's not inside quotes
func findColonOutsideQuotes(s string) int {
	inString := false
	stringChar := rune(0)

	for i, char := range s {
		switch {
		case char == '"' || char == '\'':
			if !inString {
				inString = true
				stringChar = char
			} else if char == stringChar {
				inString = false
			}
		case char == ':' && !inString:
			return i
		}
	}
	return -1
}

// parseJSONElement parses a single JSON element and returns its Go value
func parseJSONElement(elem string) interface{} {
	elem = strings.TrimSpace(elem)

	// Handle null
	if elem == "null" {
		return nil
	}

	// Handle booleans
	if elem == "true" {
		return true
	}
	if elem == "false" {
		return false
	}

	// Handle strings (quoted)
	if (strings.HasPrefix(elem, "\"") && strings.HasSuffix(elem, "\"")) ||
		(strings.HasPrefix(elem, "'") && strings.HasSuffix(elem, "'")) {
		return elem[1 : len(elem)-1]
	}

	// Handle numbers
	if num, err := strconv.ParseFloat(elem, 64); err == nil {
		// Check if it's an integer
		if num == float64(int64(num)) {
			return int64(num)
		}
		return num
	}

	// Handle nested arrays
	if strings.HasPrefix(elem, "[") {
		var arr []interface{}
		if parseJSONArray(elem, &arr) == nil {
			return arr
		}
	}

	// Handle nested objects
	if strings.HasPrefix(elem, "{") {
		var obj map[string]interface{}
		if parseJSONObject(elem, &obj) == nil {
			return obj
		}
	}

	// Return as string if nothing else matches
	return elem
}

// parseTwoArguments parses function arguments like "arg1, arg2" handling nested structures
func parseTwoArguments(args string) (string, string, error) {
	args = strings.TrimSpace(args)

	// Find the comma separator that's not inside quotes, brackets, or braces
	depth := 0
	inString := false
	stringChar := rune(0)

	for i, char := range args {
		switch {
		case char == '"' || char == '\'':
			if !inString {
				inString = true
				stringChar = char
			} else if char == stringChar && (i == 0 || args[i-1] != '\\') {
				inString = false
			}
		case inString:
			continue
		case char == '[' || char == '{' || char == '(':
			depth++
		case char == ']' || char == '}' || char == ')':
			depth--
		case char == ',' && depth == 0:
			return strings.TrimSpace(args[:i]), strings.TrimSpace(args[i+1:]), nil
		}
	}

	return "", "", fmt.Errorf("expected two arguments separated by comma, got: %s", args)
}

// ================================================================================
// Comparison functions
// ================================================================================

func compareEqual(left, right string) (bool, error) {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)

	// Handle null semantics: null == null is true
	if left == "null" && right == "null" {
		return true, nil
	}

	// null == '' should be true (null is semantically empty)
	if (left == "null" && right == "") || (left == "" && right == "null") {
		return true, nil
	}

	// null == anything else (non-empty) is false
	if left == "null" || right == "null" {
		return false, nil
	}

	return left == right, nil
}

func compareNotEqual(left, right string) (bool, error) {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)

	// Handle null semantics: null != null is false
	if left == "null" && right == "null" {
		return false, nil
	}

	// null != '' should be false (null is semantically empty)
	if (left == "null" && right == "") || (left == "" && right == "null") {
		return false, nil
	}

	// null != anything else (non-empty) is true
	if left == "null" || right == "null" {
		return true, nil
	}

	return left != right, nil
}

func compareGreater(left, right string) (bool, error) {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)

	// Handle null semantics: null > anything is false
	if left == "null" || right == "null" {
		return false, nil
	}

	lVal, lErr := strconv.ParseFloat(left, 64)
	rVal, rErr := strconv.ParseFloat(right, 64)

	if lErr != nil || rErr != nil {
		return false, fmt.Errorf("cannot compare non-numeric values with >: '%s' and '%s'", left, right)
	}

	return lVal > rVal, nil
}

func compareLess(left, right string) (bool, error) {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)

	// Handle null semantics: null < anything is false
	if left == "null" || right == "null" {
		return false, nil
	}

	lVal, lErr := strconv.ParseFloat(left, 64)
	rVal, rErr := strconv.ParseFloat(right, 64)

	if lErr != nil || rErr != nil {
		return false, fmt.Errorf("cannot compare non-numeric values with <: '%s' and '%s'", left, right)
	}

	return lVal < rVal, nil
}

func compareGreaterOrEqual(left, right string) (bool, error) {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)

	// Handle null semantics: null >= anything is false
	if left == "null" || right == "null" {
		return false, nil
	}

	lVal, lErr := strconv.ParseFloat(left, 64)
	rVal, rErr := strconv.ParseFloat(right, 64)

	if lErr != nil || rErr != nil {
		return false, fmt.Errorf("cannot compare non-numeric values with >=: '%s' and '%s'", left, right)
	}

	return lVal >= rVal, nil
}

func compareLessOrEqual(left, right string) (bool, error) {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)

	// Handle null semantics: null <= anything is false
	if left == "null" || right == "null" {
		return false, nil
	}

	lVal, lErr := strconv.ParseFloat(left, 64)
	rVal, rErr := strconv.ParseFloat(right, 64)

	if lErr != nil || rErr != nil {
		return false, fmt.Errorf("cannot compare non-numeric values with <=: '%s' and '%s'", left, right)
	}

	return lVal <= rVal, nil
}

// ================================================================================
// Function Reference Validation (for parse-time checking)
// ================================================================================

// SystemVariables contains all recognized system variable names (without the $ prefix)
// These are handled by VariableReplacer at runtime, not as function dependencies
var SystemVariables = map[string]bool{
	"ME":      true,
	"MESSAGE": true,
	"NOW":     true,
	"USER":    true,
	"ADMIN":   true,
	"COMPANY": true,
	"UUID":    true,
	"FILE":    true,
}

// IsSystemVariable checks if a variable name (without $) is a system variable
func IsSystemVariable(name string) bool {
	return SystemVariables[name]
}

// ExtractFunctionReferences extracts all function references (starting with $) from a deterministic expression
// Returns a list of unique function names that are referenced
// Used for parse-time validation to ensure all referenced functions are in the 'needs' block
// System variables like $NOW, $USER, $ME, etc. are excluded as they are handled by VariableReplacer
// Example: "$getUserStatus.isActive == true && $getBalance.amount > 100 && $NOW.hour >= 8"
// Returns: ["getUserStatus", "getBalance"] (excludes "NOW" as it's a system variable)
func ExtractFunctionReferences(expression string) []string {
	// Pattern to match $functionName (captures function name after $)
	// Matches: $functionName, $functionName.field, $functionName[0], etc.
	pattern := regexp.MustCompile(`\$([a-zA-Z_][a-zA-Z0-9_]*)`)
	matches := pattern.FindAllStringSubmatch(expression, -1)

	// Use map to get unique function names
	funcMap := make(map[string]bool)
	for _, match := range matches {
		if len(match) > 1 {
			varName := match[1]
			// Skip system variables - they're handled by VariableReplacer, not as function dependencies
			if !IsSystemVariable(varName) {
				funcMap[varName] = true
			}
		}
	}

	// Convert to slice
	result := make([]string, 0, len(funcMap))
	for funcName := range funcMap {
		result = append(result, funcName)
	}

	return result
}

// SupportedDeterministicFunctions contains all built-in functions supported in deterministic expressions
var SupportedDeterministicFunctions = map[string]bool{
	"len":      true,
	"isEmpty":  true,
	"contains": true,
	"exists":   true,
	"coalesce": true, // Returns first non-null argument
	"date":     true, // Extracts date portion from datetime string
}

// SupportedExpressionFunctions contains all functions supported in expression evaluation (string fields)
// These functions are evaluated after variable replacement in output.value, params, etc.
// This should match SupportedDeterministicFunctions since both use the same underlying evaluators.
var SupportedExpressionFunctions = map[string]bool{
	"len":      true, // Returns length of array/string/object
	"isEmpty":  true, // Returns true if null/empty
	"contains": true, // Checks if value contains element
	"exists":   true, // Returns true if not null/empty string
	"coalesce": true, // Returns first non-null, non-empty argument
	"date":     true, // Extracts date portion from datetime string
}

// SupportedDeterministicOperators contains all operators supported in deterministic expressions
var SupportedDeterministicOperators = []string{
	"==", "!=", ">=", "<=", ">", "<", "&&", "||",
}

// ValidateDeterministicExpression validates that a deterministic expression only uses supported
// functions and operators. Returns an error if unsupported operations are found.
// This is meant for parse-time validation to provide early feedback.
func ValidateDeterministicExpression(expression string) error {
	// Extract all function calls (name followed by parenthesis)
	// Pattern matches: functionName(
	funcPattern := regexp.MustCompile(`([a-zA-Z_][a-zA-Z0-9_]*)\s*\(`)
	matches := funcPattern.FindAllStringSubmatch(expression, -1)

	var unsupportedFuncs []string
	for _, match := range matches {
		if len(match) > 1 {
			funcName := match[1]
			if !SupportedDeterministicFunctions[funcName] {
				unsupportedFuncs = append(unsupportedFuncs, funcName)
			}
		}
	}

	if len(unsupportedFuncs) > 0 {
		// Remove duplicates
		seen := make(map[string]bool)
		var unique []string
		for _, f := range unsupportedFuncs {
			if !seen[f] {
				seen[f] = true
				unique = append(unique, f)
			}
		}

		supportedList := make([]string, 0, len(SupportedDeterministicFunctions))
		for fn := range SupportedDeterministicFunctions {
			supportedList = append(supportedList, fn+"()")
		}

		return fmt.Errorf("deterministic expression contains unsupported function(s): %v. Supported functions are: %v",
			unique, supportedList)
	}

	// Check for unsupported keyword operators (and/or instead of &&/||)
	if err := ValidateNoKeywordOperators(expression); err != nil {
		return err
	}

	return nil
}

// ValidateNoKeywordOperators checks for 'and'/'or' keywords used as logical operators
// instead of the supported '&&'/'||' operators. This catches common mistakes like:
//   - "x == true and y == false" (should be "x == true && y == false")
//   - "a == 1 or b == 2" (should be "a == 1 || b == 2")
//
// It avoids false positives by:
//   - Ignoring content inside single or double quotes
//   - Requiring word boundaries (spaces/parens/start/end) around the keywords
func ValidateNoKeywordOperators(expression string) error {
	if expression == "" {
		return nil
	}

	// Track quote state
	inSingleQuote := false
	inDoubleQuote := false

	// Convert to lowercase for case-insensitive matching
	exprLower := strings.ToLower(expression)
	n := len(exprLower)

	for i := 0; i < n; i++ {
		char := expression[i]

		// Handle quote state transitions (using original case)
		if char == '\'' && !inDoubleQuote {
			inSingleQuote = !inSingleQuote
			continue
		}
		if char == '"' && !inSingleQuote {
			inDoubleQuote = !inDoubleQuote
			continue
		}

		// Skip if inside quotes
		if inSingleQuote || inDoubleQuote {
			continue
		}

		// Check for word boundaries before the keyword
		// Word boundary: start of string, space, or closing paren
		isWordBoundaryBefore := i == 0 || exprLower[i-1] == ' ' || exprLower[i-1] == ')'

		if isWordBoundaryBefore {
			// Check for "and" keyword
			if i+3 <= n && exprLower[i:i+3] == "and" {
				// Check word boundary after: space, open paren, or end
				if i+3 == n || exprLower[i+3] == ' ' || exprLower[i+3] == '(' {
					return fmt.Errorf("deterministic expression uses unsupported keyword 'and'. Use '&&' instead. Expression: %s", expression)
				}
			}

			// Check for "or" keyword
			if i+2 <= n && exprLower[i:i+2] == "or" {
				// Check word boundary after: space, open paren, or end
				if i+2 == n || exprLower[i+2] == ' ' || exprLower[i+2] == '(' {
					return fmt.Errorf("deterministic expression uses unsupported keyword 'or'. Use '||' instead. Expression: %s", expression)
				}
			}
		}
	}

	return nil
}

// ValidateExpressionFunctions validates that a string value only uses supported expression functions.
// Expression functions are evaluated after variable replacement (e.g., coalesce in output.value, params).
// Returns an error if unsupported functions are found.
// This is meant for parse-time validation to provide early feedback.
func ValidateExpressionFunctions(value string) error {
	// Extract all function calls (name followed by parenthesis)
	// Pattern matches: functionName( with content up to closing paren
	// We need to capture both the function name and the argument to filter false positives
	funcPattern := regexp.MustCompile(`([a-zA-Z_][a-zA-Z0-9_]*)\s*\(([^)]*)\)`)
	matches := funcPattern.FindAllStringSubmatch(value, -1)

	var unsupportedFuncs []string
	for _, match := range matches {
		if len(match) > 1 {
			funcName := match[1]
			funcArgs := ""
			if len(match) > 2 {
				funcArgs = match[2]
			}

			// Check if it's a supported expression function
			if !SupportedExpressionFunctions[funcName] {
				// Also allow if it looks like a variable reference that happens to have parens
				// e.g., $result.data(something) - this isn't a function call
				// Skip if the match is preceded by $ or .
				matchIdx := findMatchIndex(value, match[0])
				if matchIdx > 0 {
					prevChar := value[matchIdx-1]
					if prevChar == '$' || prevChar == '.' {
						continue // Not a function call, it's a variable path
					}
				}

				// Skip gendered word patterns common in Portuguese/Spanish
				// e.g., cuidado(a), querido(a), filho(a), amigo(a), amigo(s)
				// These are single/double-letter arguments that indicate grammatical gender/number
				if funcArgs == "a" || funcArgs == "o" || funcArgs == "s" || funcArgs == "as" || funcArgs == "os" {
					continue // Grammatical gender/number marker, not a function call
				}

				unsupportedFuncs = append(unsupportedFuncs, funcName)
			}
		}
	}

	if len(unsupportedFuncs) > 0 {
		// Remove duplicates
		seen := make(map[string]bool)
		var unique []string
		for _, f := range unsupportedFuncs {
			if !seen[f] {
				seen[f] = true
				unique = append(unique, f)
			}
		}

		supportedList := make([]string, 0, len(SupportedExpressionFunctions))
		for fn := range SupportedExpressionFunctions {
			supportedList = append(supportedList, fn+"()")
		}

		return fmt.Errorf("expression contains unsupported function(s): %v. Supported expression functions are: %v",
			unique, supportedList)
	}

	return nil
}

// findMatchIndex finds the starting index of a substring in a string
func findMatchIndex(s, substr string) int {
	idx := 0
	for {
		i := strings.Index(s[idx:], substr)
		if i == -1 {
			return -1
		}
		return idx + i
	}
}

// ================================================================================
// Sanitize Configuration Parsing
// ================================================================================

// ParseSanitize converts the Sanitize field (bool, string, or object) to a *SanitizeConfig.
// Returns nil if sanitization is explicitly disabled (false) or not configured.
//
// Supported formats:
//   - true or "true"        → {Strategy: "fence"}
//   - false or "false"      → nil (explicitly disabled, overrides auto-sanitize)
//   - "fence"               → {Strategy: "fence"}
//   - "strict"              → {Strategy: "strict"}
//   - "llm_extract"         → {Strategy: "llm_extract"}
//   - map with strategy key → full SanitizeConfig parsing
func ParseSanitize(sanitize interface{}) (*SanitizeConfig, error) {
	if sanitize == nil {
		return nil, nil
	}

	switch v := sanitize.(type) {
	case bool:
		if !v {
			return nil, nil // Explicitly disabled
		}
		return &SanitizeConfig{Strategy: SanitizeStrategyFence}, nil

	case string:
		return parseSanitizeString(v)

	case map[interface{}]interface{}:
		stringMap := make(map[string]interface{})
		for k, val := range v {
			if kStr, ok := k.(string); ok {
				stringMap[kStr] = val
			} else {
				return nil, fmt.Errorf("sanitize map keys must be strings")
			}
		}
		return parseSanitizeFromMap(stringMap)

	case map[string]interface{}:
		return parseSanitizeFromMap(v)

	default:
		return nil, fmt.Errorf("sanitize must be a boolean, string, or object, got %v", reflect.TypeOf(sanitize))
	}
}

// parseSanitizeString parses sanitize from a string value
func parseSanitizeString(v string) (*SanitizeConfig, error) {
	switch v {
	case "true":
		return &SanitizeConfig{Strategy: SanitizeStrategyFence}, nil
	case "false":
		return nil, nil
	case SanitizeStrategyFence, SanitizeStrategyStrict, SanitizeStrategyLLMExtract:
		return &SanitizeConfig{Strategy: v}, nil
	default:
		return nil, fmt.Errorf("sanitize string must be 'fence', 'strict', 'llm_extract', 'true', or 'false', got '%s'", v)
	}
}

// parseSanitizeFromMap parses a full SanitizeConfig from a map
func parseSanitizeFromMap(m map[string]interface{}) (*SanitizeConfig, error) {
	config := &SanitizeConfig{}

	// Parse strategy (required)
	if strategy, ok := m["strategy"]; ok {
		if stratStr, ok := strategy.(string); ok {
			config.Strategy = stratStr
		} else {
			return nil, fmt.Errorf("sanitize.strategy must be a string")
		}
	} else {
		return nil, fmt.Errorf("sanitize object must have a 'strategy' field")
	}

	// Parse maxLength (optional)
	if maxLength, ok := m["maxLength"]; ok {
		switch v := maxLength.(type) {
		case int:
			config.MaxLength = v
		case float64:
			config.MaxLength = int(v)
		default:
			return nil, fmt.Errorf("sanitize.maxLength must be a number")
		}
	}

	// Parse customPatterns (optional, strict only)
	if customPatterns, ok := m["customPatterns"]; ok {
		patterns, err := parseStringArray(customPatterns, "sanitize.customPatterns")
		if err != nil {
			return nil, err
		}
		config.CustomPatterns = patterns
	}

	// Parse extract fields (optional, llm_extract only)
	if extract, ok := m["extract"]; ok {
		fields, err := parseExtractFields(extract)
		if err != nil {
			return nil, err
		}
		config.Extract = fields
	}

	return config, nil
}

// parseExtractFields parses the extract field array for llm_extract strategy
func parseExtractFields(extract interface{}) ([]ExtractField, error) {
	arr, ok := extract.([]interface{})
	if !ok {
		return nil, fmt.Errorf("sanitize.extract must be an array")
	}

	fields := make([]ExtractField, 0, len(arr))
	for i, item := range arr {
		var fieldMap map[string]interface{}

		switch v := item.(type) {
		case map[string]interface{}:
			fieldMap = v
		case map[interface{}]interface{}:
			fieldMap = make(map[string]interface{})
			for k, val := range v {
				if kStr, ok := k.(string); ok {
					fieldMap[kStr] = val
				}
			}
		default:
			return nil, fmt.Errorf("sanitize.extract[%d] must be an object", i)
		}

		field := ExtractField{}
		if f, ok := fieldMap["field"].(string); ok {
			field.Field = f
		} else {
			return nil, fmt.Errorf("sanitize.extract[%d].field is required and must be a string", i)
		}

		if d, ok := fieldMap["description"].(string); ok {
			field.Description = d
		} else {
			return nil, fmt.Errorf("sanitize.extract[%d].description is required and must be a string", i)
		}

		fields = append(fields, field)
	}

	return fields, nil
}

// IsSanitizeExplicitlyDisabled checks if the sanitize field is explicitly set to false.
// This is used to determine if auto-sanitize should be overridden.
func IsSanitizeExplicitlyDisabled(sanitize interface{}) bool {
	if sanitize == nil {
		return false
	}
	switch v := sanitize.(type) {
	case bool:
		return !v
	case string:
		return v == "false"
	}
	return false
}
