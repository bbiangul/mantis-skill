package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"github.com/bbiangul/mantis-skill/skill"
	"github.com/henomis/lingoose/thread"
)

// outputFormatterVarReplacer is the minimal interface required by OutputFormatter.
// It is satisfied by the concrete VariableReplacer struct (defined in variable_replacer.go)
// and by engine/models.IVariableReplacer.
type outputFormatterVarReplacer interface {
	ReplaceVariables(ctx context.Context, text string, inputs map[string]interface{}) (string, error)
}

// Package-level logger (set by the engine at init time via SetOutputFormatterLogger).
var outputFormatterLogger Logger

// SetOutputFormatterLogger sets the package-level logger for the output formatter.
func SetOutputFormatterLogger(l Logger) {
	outputFormatterLogger = l
}

// OutputFormatter handles the conversion of raw outputs to structured formats
type OutputFormatter struct {
	variableReplacer outputFormatterVarReplacer
	llmProvider      LLMProvider // optional; used as fallback when deterministic formatting fails
}

// NewOutputFormatter creates a new OutputFormatter
func NewOutputFormatter(variableReplacer outputFormatterVarReplacer) *OutputFormatter {
	return &OutputFormatter{
		variableReplacer: variableReplacer,
	}
}

// NewOutputFormatterWithLLM creates a new OutputFormatter with an LLM fallback provider
func NewOutputFormatterWithLLM(variableReplacer outputFormatterVarReplacer, llm LLMProvider) *OutputFormatter {
	return &OutputFormatter{
		variableReplacer: variableReplacer,
		llmProvider:      llm,
	}
}

// FormatOutput formats the raw output according to the specified output definition
func (f *OutputFormatter) FormatOutput(
	ctx context.Context,
	messageID string,
	rawOutput string,
	stepResults map[int]interface{},
	outputDef *skill.Output,
	inputs map[string]interface{}, // Added inputs parameter
) (string, error) {
	if outputDef == nil {
		return rawOutput, nil
	}

	// Handle based on output type
	switch outputDef.Type {
	case "string":
		return f.formatStringOutput(ctx, messageID, outputDef.Value, stepResults, inputs)

	case "object":
		return f.formatObjectOutput(ctx, messageID, rawOutput, stepResults, outputDef.Fields)

	case "list[object]":
		return f.formatListObjectOutput(ctx, messageID, rawOutput, stepResults, outputDef)

	case "list[string]":
		return f.formatListStringOutput(ctx, messageID, rawOutput, stepResults)

	case "list[number]":
		return f.formatListNumberOutput(ctx, messageID, rawOutput, stepResults)

	case skill.OutputTypeFile:
		return f.formatFileOutput(ctx, messageID, rawOutput, stepResults, outputDef.Value)

	case skill.OutputTypeListFile:
		return f.formatListFileOutput(ctx, messageID, rawOutput, stepResults, outputDef.Value)

	default:
		return "", fmt.Errorf("unsupported output type: %s", outputDef.Type)
	}
}

// formatStringOutput handles string output with variable replacement
func (f *OutputFormatter) formatStringOutput(
	ctx context.Context,
	messageID string,
	template string,
	stepResults map[int]interface{},
	inputs map[string]interface{}, // Added inputs parameter
) (string, error) {
	if template == "" {
		return "", fmt.Errorf("string output type requires a value template")
	}

	if outputFormatterLogger != nil {
		outputFormatterLogger.Debugf("formatStringOutput - messageID: %s, template: %s, stepResults count: %d", messageID, template, len(stepResults))
	}

	// Log step results for debugging
	if outputFormatterLogger != nil {
		for idx, result := range stepResults {
			outputFormatterLogger.Debugf("formatStringOutput - stepResults[%d] type: %T", idx, result)
		}
	}

	// Create a data map including step results AND inputs for variable replacement
	data := make(map[string]interface{})

	// First, add all inputs to the data map
	for key, value := range inputs {
		data[key] = value
	}

	// Then add step results (these can override inputs if there's a conflict)
	for index, result := range stepResults {
		data[fmt.Sprintf("result[%d]", index)] = result
		if outputFormatterLogger != nil {
			outputFormatterLogger.Debugf("formatStringOutput - Added to data map: result[%d]", index)
		}
	}

	// Replace variables in the template
	if outputFormatterLogger != nil {
		outputFormatterLogger.Debugf("formatStringOutput - Calling ReplaceVariables with template: %s", template)
	}
	replaced, err := f.variableReplacer.ReplaceVariables(ctx, template, data)
	if err != nil {
		if outputFormatterLogger != nil {
			outputFormatterLogger.Errorf("formatStringOutput - ReplaceVariables ERROR: %v", err)
		}
		return "", err
	}
	if outputFormatterLogger != nil {
		outputFormatterLogger.Debugf("formatStringOutput - ReplaceVariables SUCCESS: %s", replaced)
	}
	return replaced, nil
}

// formatObjectOutput formats the output as a JSON object with specific fields
func (f *OutputFormatter) formatObjectOutput(
	ctx context.Context,
	messageID string,
	rawOutput string,
	stepResults map[int]interface{},
	fields []skill.OutputField,
) (string, error) {
	// Try to parse raw output as JSON if available
	var rawData map[string]interface{}
	if rawOutput != "" {
		err := json.Unmarshal([]byte(rawOutput), &rawData)
		if err != nil {
			// Fallback: Try to extract JSON from mixed text (e.g., "prefix\n{...}\nsuffix")
			if extracted, extractErr := extractJSONFromText(rawOutput); extractErr == nil && extracted != "" {
				if err2 := json.Unmarshal([]byte(extracted), &rawData); err2 != nil {
					// Extraction found JSON but parsing as object failed, use empty map
					rawData = make(map[string]interface{})
				}
				// else: Successfully extracted and parsed JSON - rawData is set
			} else {
				// No JSON found in text, use empty map
				rawData = make(map[string]interface{})
			}
		} else if rawData == nil {
			// Handle case where rawOutput is valid JSON "null"
			rawData = make(map[string]interface{})
		}
	} else {
		rawData = make(map[string]interface{})
	}

	// Enrich rawData with step results (both formats for compatibility)
	for index, result := range stepResults {
		rawData[fmt.Sprintf("result_%d", index)] = result
		rawData[fmt.Sprintf("result[%d]", index)] = result
	}

	// Create the output object with the required fields
	output, err := f.buildStructuredObject(ctx, rawData, fields, stepResults)
	if err != nil {
		// Log the error before falling back to LLM
		if outputFormatterLogger != nil {
			outputFormatterLogger.Warnf("formatObjectOutput - Failed to build structured object, falling back to LLM. Error: %v", err)
		}
		return f.useLLMForFormatting(ctx, rawOutput, stepResults, "object", fields)
	}

	// Convert to JSON
	jsonBytes, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return "", fmt.Errorf("error serializing output to JSON: %w", err)
	}

	return string(jsonBytes), nil
}

// formatListObjectOutput formats the output as a list of objects
func (f *OutputFormatter) formatListObjectOutput(
	ctx context.Context,
	messageID string,
	rawOutput string,
	stepResults map[int]interface{},
	outputDef *skill.Output,
) (string, error) {
	fields := outputDef.Fields

	// First, try to resolve the value expression (e.g. "result[1].issues") against step results
	var rawList []interface{}
	if outputDef.Value != "" && f.variableReplacer != nil && strings.Contains(outputDef.Value, "result[") {
		data := buildStepResultsDataMap(stepResults)
		replaced, err := f.variableReplacer.ReplaceVariables(ctx, outputDef.Value, data)
		if err == nil && replaced != outputDef.Value {
			if err2 := json.Unmarshal([]byte(replaced), &rawList); err2 == nil {
				if outputFormatterLogger != nil {
					outputFormatterLogger.Debugf("formatListObjectOutput - resolved value expression '%s' to %d items", outputDef.Value, len(rawList))
				}
			}
		}
		// If expression resolved but didn't produce a JSON array, check stepResults directly
		if rawList == nil {
			for index, res := range stepResults {
				if outputDef.Value == fmt.Sprintf("result[%d]", index) {
					if arr, ok := res.([]interface{}); ok {
						rawList = arr
					}
				}
			}
		}
	}

	// Fallback: Try to parse raw output as JSON array
	if rawList == nil && rawOutput != "" {
		err := json.Unmarshal([]byte(rawOutput), &rawList)
		if err != nil {
			// Fallback: Try to extract JSON from mixed text (e.g., "prefix\n[...]\nsuffix")
			if extracted, extractErr := extractJSONFromText(rawOutput); extractErr == nil && extracted != "" {
				// Try parsing extracted text as array first
				if err2 := json.Unmarshal([]byte(extracted), &rawList); err2 != nil {
					// Not an array, try as single object
					var singleObj map[string]interface{}
					if err3 := json.Unmarshal([]byte(extracted), &singleObj); err3 == nil {
						rawList = []interface{}{singleObj}
					}
				}
			}

			// If extraction didn't work, try original logic with single object
			if rawList == nil {
				var singleObj map[string]interface{}
				err = json.Unmarshal([]byte(rawOutput), &singleObj)
				if err == nil {
					// If it's a single object, convert it to a list
					rawList = []interface{}{singleObj}
				}
			}

			// If still nil, look for arrays in step results or fallback to LLM
			if rawList == nil {
				// Look for arrays in step results
				foundArray := false
				for _, result := range stepResults {
					if arr, ok := result.([]interface{}); ok {
						rawList = arr
						foundArray = true
						break
					}
				}

				if !foundArray {
					// Log the parsing failure before falling back to LLM
					if outputFormatterLogger != nil {
						outputFormatterLogger.Warnf("formatListObjectOutput - Failed to parse JSON array, falling back to LLM. Error: %v. Raw output (first 500 chars): %s", err, truncateString(rawOutput, 500))
					}
					return f.useLLMForFormatting(ctx, rawOutput, stepResults, "list[object]", fields)
				}
			}
		}
	} else if rawList == nil {
		// Check if any step result is an array
		for _, result := range stepResults {
			if arr, ok := result.([]interface{}); ok {
				rawList = arr
				break
			}
		}

		if rawList == nil {
			// Log the issue before falling back to LLM
			if outputFormatterLogger != nil {
				outputFormatterLogger.Warnf("formatListObjectOutput - No array found in rawOutput or stepResults, falling back to LLM. stepResults count: %d", len(stepResults))
			}
			return f.useLLMForFormatting(ctx, rawOutput, stepResults, "list[object]", fields)
		}
	}

	// Parse JSON strings in array (common with foreach results from API calls)
	// Each foreach iteration might return a JSON string like "[{...}, {...}]"
	parsedList := make([]interface{}, 0, len(rawList))
	for _, item := range rawList {
		if jsonStr, ok := item.(string); ok {
			// Try to parse as JSON array
			var parsed []interface{}
			if err := json.Unmarshal([]byte(jsonStr), &parsed); err == nil {
				// Successfully parsed as array - add all items
				parsedList = append(parsedList, parsed...)
			} else {
				// Try to parse as single object
				var parsedObj map[string]interface{}
				if err := json.Unmarshal([]byte(jsonStr), &parsedObj); err == nil {
					parsedList = append(parsedList, parsedObj)
				} else {
					// Not JSON, keep as-is
					parsedList = append(parsedList, item)
				}
			}
		} else {
			// Not a string, keep as-is
			parsedList = append(parsedList, item)
		}
	}
	rawList = parsedList

	// Flatten nested arrays if requested (for foreach results)
	if outputDef.Flatten {
		rawList = flattenArray(rawList)
	}

	// Format each object in the list
	formattedList := make([]interface{}, 0, len(rawList))
	for _, item := range rawList {
		// Convert to map if needed
		itemMap, ok := item.(map[string]interface{})
		if !ok {
			// Skip items that aren't objects
			continue
		}

		// Build structured object for this item
		formattedItem, err := f.buildStructuredObject(ctx, itemMap, fields, stepResults)
		if err != nil {
			// Log the error before falling back to LLM
			if outputFormatterLogger != nil {
				outputFormatterLogger.Warnf("formatListObjectOutput - Failed to build structured object for item, falling back to LLM. Error: %v", err)
			}
			return f.useLLMForFormatting(ctx, rawOutput, stepResults, "list[object]", fields)
		}

		formattedList = append(formattedList, formattedItem)
	}

	// Convert to JSON
	jsonBytes, err := json.MarshalIndent(formattedList, "", "  ")
	if err != nil {
		return "", fmt.Errorf("error serializing output to JSON: %w", err)
	}

	return string(jsonBytes), nil
}

// formatListStringOutput formats the output as a list of strings
func (f *OutputFormatter) formatListStringOutput(
	ctx context.Context,
	messageID string,
	rawOutput string,
	stepResults map[int]interface{},
) (string, error) {
	// Try to parse raw output as JSON array
	var stringList []string

	if rawOutput != "" {
		// First try to parse as direct string array
		err := json.Unmarshal([]byte(rawOutput), &stringList)
		if err != nil {
			// If that fails, try parsing as interface array and convert
			var rawList []interface{}
			err = json.Unmarshal([]byte(rawOutput), &rawList)
			if err == nil {
				// Convert each item to string
				stringList = make([]string, 0, len(rawList))
				for _, item := range rawList {
					stringList = append(stringList, fmt.Sprintf("%v", item))
				}
			} else {
				// If all else fails, try LLM
				return f.useLLMForFormatting(ctx, rawOutput, stepResults, "list[string]", nil)
			}
		}
	} else {
		// Look in step results for string arrays
		for _, result := range stepResults {
			if arr, ok := result.([]interface{}); ok {
				stringList = make([]string, 0, len(arr))
				for _, item := range arr {
					stringList = append(stringList, fmt.Sprintf("%v", item))
				}
				break
			}
		}

		if stringList == nil {
			// If no array found, try LLM
			return f.useLLMForFormatting(ctx, rawOutput, stepResults, "list[string]", nil)
		}
	}

	// Convert to JSON
	jsonBytes, err := json.MarshalIndent(stringList, "", "  ")
	if err != nil {
		return "", fmt.Errorf("error serializing output to JSON: %w", err)
	}

	return string(jsonBytes), nil
}

// formatListNumberOutput formats the output as a list of numbers
func (f *OutputFormatter) formatListNumberOutput(
	ctx context.Context,
	messageID string,
	rawOutput string,
	stepResults map[int]interface{},
) (string, error) {
	// Try to parse raw output as JSON array
	var numberList []float64

	if rawOutput != "" {
		// First try to parse as direct number array
		err := json.Unmarshal([]byte(rawOutput), &numberList)
		if err != nil {
			// If that fails, try parsing as interface array and convert
			var rawList []interface{}
			err = json.Unmarshal([]byte(rawOutput), &rawList)
			if err == nil {
				// Convert each item to number if possible
				numberList = make([]float64, 0, len(rawList))
				for _, item := range rawList {
					switch v := item.(type) {
					case float64:
						numberList = append(numberList, v)
					case int:
						numberList = append(numberList, float64(v))
					case string:
						// Try to convert string to number
						if f, err := parseNumber(v); err == nil {
							numberList = append(numberList, f)
						}
					}
				}
			} else {
				// If all else fails, try LLM
				return f.useLLMForFormatting(ctx, rawOutput, stepResults, "list[number]", nil)
			}
		}
	} else {
		// Look in step results for number arrays
		for _, result := range stepResults {
			if arr, ok := result.([]interface{}); ok {
				numberList = make([]float64, 0, len(arr))
				for _, item := range arr {
					switch v := item.(type) {
					case float64:
						numberList = append(numberList, v)
					case int:
						numberList = append(numberList, float64(v))
					case string:
						// Try to convert string to number
						if f, err := parseNumber(v); err == nil {
							numberList = append(numberList, f)
						}
					}
				}
				break
			}
		}

		if numberList == nil {
			// If no array found, try LLM
			return f.useLLMForFormatting(ctx, rawOutput, stepResults, "list[number]", nil)
		}
	}

	// Convert to JSON
	jsonBytes, err := json.MarshalIndent(numberList, "", "  ")
	if err != nil {
		return "", fmt.Errorf("error serializing output to JSON: %w", err)
	}

	return string(jsonBytes), nil
}

// buildStepResultsDataMap creates a data map with step results for variable replacement
func buildStepResultsDataMap(stepResults map[int]interface{}) map[string]interface{} {
	data := make(map[string]interface{})
	for index, res := range stepResults {
		data[fmt.Sprintf("result[%d]", index)] = res
	}
	return data
}

// buildStructuredObject creates an object with the exact fields specified
func (f *OutputFormatter) buildStructuredObject(
	ctx context.Context,
	sourceData map[string]interface{},
	fields []skill.OutputField,
	stepResults map[int]interface{},
) (map[string]interface{}, error) {
	result := make(map[string]interface{})

	for _, field := range fields {
		fieldName := field.Name
		if fieldName == "" {
			fieldName = field.Value
		}

		if fieldName == "" {
			return nil, fmt.Errorf("field name not specified")
		}

		// Handle complex (nested) fields
		if field.Type == "object" && len(field.Fields) > 0 {
			// Try to get the nested data
			var nestedSource map[string]interface{}

			// FIRST: Check if field.Value contains an expression like "result[1][0]"
			if field.Value != "" && f.variableReplacer != nil {
				// Check if it looks like an expression (contains result[ or $)
				if strings.Contains(field.Value, "result[") || strings.Contains(field.Value, "$") {
					data := buildStepResultsDataMap(stepResults)

					// Try to evaluate the expression
					replaced, err := f.variableReplacer.ReplaceVariables(ctx, field.Value, data)
					if err == nil && replaced != field.Value {
						// Try to parse the replaced value as JSON object
						var obj map[string]interface{}
						if err := json.Unmarshal([]byte(replaced), &obj); err == nil {
							nestedSource = obj
							if outputFormatterLogger != nil {
								outputFormatterLogger.Debugf("buildStructuredObject - nested object '%s' resolved from expression '%s'", fieldName, field.Value)
							}
						}
					}
				}
			}

			// FALLBACK: Check if source data has this field
			if nestedSource == nil && sourceData[fieldName] != nil {
				if nestedMap, ok := sourceData[fieldName].(map[string]interface{}); ok {
					nestedSource = nestedMap
				} else {
					// Try to convert it to map if it's not already
					var obj map[string]interface{}
					jsonData, err := json.Marshal(sourceData[fieldName])
					if err == nil {
						if err = json.Unmarshal(jsonData, &obj); err == nil {
							nestedSource = obj
						}
					}
				}
			}

			// If we didn't find the nested source or couldn't convert, use empty map
			if nestedSource == nil {
				nestedSource = make(map[string]interface{})
			}

			// Build the nested object
			nestedObj, err := f.buildStructuredObject(ctx, nestedSource, field.Fields, stepResults)
			if err != nil {
				return nil, err
			}

			result[fieldName] = nestedObj
		} else if field.Type == "list[object]" && len(field.Fields) > 0 {
			// Handle array of objects
			var nestedList []interface{}

			// FIRST: Check if field.Value contains an expression like "result[1]"
			if field.Value != "" && f.variableReplacer != nil {
				data := buildStepResultsDataMap(stepResults)

				// Try to evaluate the expression
				replaced, err := f.variableReplacer.ReplaceVariables(ctx, field.Value, data)
				if err == nil && replaced != field.Value {
					// Try to parse the replaced value as JSON array
					var arr []interface{}
					if err := json.Unmarshal([]byte(replaced), &arr); err == nil {
						nestedList = arr
						if outputFormatterLogger != nil {
							outputFormatterLogger.Debugf("buildStructuredObject - field '%s' resolved from expression '%s' to %d items", fieldName, field.Value, len(arr))
						}
					} else {
						// Maybe it's already an interface{} that's an array
						if stepResults != nil {
							// Check if it directly references a step result
							for index, res := range stepResults {
								if field.Value == fmt.Sprintf("result[%d]", index) {
									if arr, ok := res.([]interface{}); ok {
										nestedList = arr
										if outputFormatterLogger != nil {
											outputFormatterLogger.Debugf("buildStructuredObject - field '%s' directly resolved from stepResults[%d] to %d items", fieldName, index, len(arr))
										}
									}
								}
							}
						}
					}
				}
			}

			// FALLBACK: Check if source data has this field
			if nestedList == nil && sourceData[fieldName] != nil {
				if list, ok := sourceData[fieldName].([]interface{}); ok {
					nestedList = list
				} else {
					// Try to convert it to list if it's not already
					var arr []interface{}
					jsonData, err := json.Marshal(sourceData[fieldName])
					if err == nil {
						if err = json.Unmarshal(jsonData, &arr); err == nil {
							nestedList = arr
						}
					}
				}
			}

			// If we didn't find the nested list or couldn't convert, use empty list
			if nestedList == nil {
				nestedList = []interface{}{}
			}

			// Build each object in the list
			formattedList := make([]interface{}, 0, len(nestedList))
			for _, item := range nestedList {
				itemMap, ok := item.(map[string]interface{})
				if !ok {
					// Try to convert it to map if it's not already
					var obj map[string]interface{}
					jsonData, err := json.Marshal(item)
					if err == nil {
						if err = json.Unmarshal(jsonData, &obj); err == nil {
							itemMap = obj
						} else {
							// Skip items we can't convert
							continue
						}
					} else {
						// Skip items we can't convert
						continue
					}
				}

				// Build the nested object
				nestedObj, err := f.buildStructuredObject(ctx, itemMap, field.Fields, stepResults)
				if err != nil {
					continue
				}

				formattedList = append(formattedList, nestedObj)
			}

			result[fieldName] = formattedList
		} else {
			// For simple fields, first check if field.Value contains an expression
			if field.Value != "" && f.variableReplacer != nil {
				// Check if it looks like an expression (contains result[ or len( or other patterns)
				if strings.Contains(field.Value, "result[") || strings.Contains(field.Value, "len(") || strings.Contains(field.Value, "$") {
					data := buildStepResultsDataMap(stepResults)

					// Try to evaluate the expression
					replaced, err := f.variableReplacer.ReplaceVariables(ctx, field.Value, data)
					if err == nil && replaced != field.Value {
						if outputFormatterLogger != nil {
							outputFormatterLogger.Debugf("buildStructuredObject - field '%s' resolved from expression '%s' to '%s'", fieldName, field.Value, replaced)
						}
						result[fieldName] = replaced
						continue
					}
				}
			}

			// For simple fields, just copy the value if available
			if val, ok := sourceData[fieldName]; ok {
				result[fieldName] = val
			} else {
				// For missing fields, try to find a value in source data that might match
				foundValue := f.findMatchingValue(sourceData, fieldName)
				if foundValue != nil {
					result[fieldName] = foundValue
				} else {
					// If still not found, add an empty value of appropriate type
					result[fieldName] = getDefaultValueForType(field.Type)
				}
			}
		}
	}

	return result, nil
}

// useLLMForFormatting uses an LLM to format the output when all else fails.
// If no LLMProvider was configured, it returns an error.
func (f *OutputFormatter) useLLMForFormatting(
	ctx context.Context,
	rawOutput string,
	stepResults map[int]interface{},
	outputType string,
	fields []skill.OutputField,
) (string, error) {
	if f.llmProvider == nil {
		return rawOutput, fmt.Errorf("no LLM provider configured for output formatting fallback (output type: %s)", outputType)
	}

	stepResultsJSON, err := json.MarshalIndent(stepResults, "", "  ")
	if err != nil {
		stepResultsJSON = []byte("{}")
	}

	// Create field descriptions
	fieldDesc := "N/A"
	if len(fields) > 0 {
		fieldNames := make([]string, 0, len(fields))
		for _, field := range fields {
			name := field.Name
			if name == "" {
				name = field.Value
			}
			fieldNames = append(fieldNames, name)
		}
		fieldDesc = strings.Join(fieldNames, ", ")
	}

	systemPrompt := fmt.Sprintf(`You are a data formatting assistant. Your task is to convert the raw data into the specified output format.

Output Format: %s
Fields (if applicable): %s

Raw output:
%s

Step results:
%s

Create the requested output format using ONLY the information provided above. Do not invent or hallucinate additional information.

If you don't have enough information to create a valid output, respond with: {"error": "Insufficient information to create the required output format"}

Your response must be valid JSON that matches the required output type. Return ONLY the formatted JSON without any explanations or additional text.`,
		outputType, fieldDesc, rawOutput, string(stepResultsJSON))

	messages := thread.New()
	messages.AddMessages(
		thread.NewSystemMessage().AddContent(thread.NewTextContent(systemPrompt)),
		thread.NewUserMessage().AddContent(thread.NewTextContent("please output the formatted JSON")),
	)

	generatedOutput, err := f.llmProvider.GenerateWithThread(ctx, messages, LLMOptions{})
	if err != nil {
		return rawOutput, fmt.Errorf("error generating formatted output with LLM: %w", err)
	}

	// Validate the result is valid JSON
	var testObj interface{}
	if err := json.Unmarshal([]byte(generatedOutput), &testObj); err != nil {
		// Try to extract JSON from markdown-wrapped output
		if extracted, extractErr := extractJSONFromText(generatedOutput); extractErr == nil && extracted != "" {
			return extracted, nil
		}
		return rawOutput, fmt.Errorf("LLM returned invalid JSON: %w", err)
	}

	// Check for error in LLM response
	if strings.Contains(generatedOutput, "error") {
		var errorObj map[string]interface{}
		if err := json.Unmarshal([]byte(generatedOutput), &errorObj); err == nil {
			if errorMsg, ok := errorObj["error"].(string); ok {
				return rawOutput, fmt.Errorf("LLM formatting error: %s", errorMsg)
			}
		}
	}

	return interfaceToString(testObj), nil
}

// Helper function to try to find a matching value in the data
func (f *OutputFormatter) findMatchingValue(data map[string]interface{}, fieldName string) interface{} {
	// Try exact match first
	if val, ok := data[fieldName]; ok {
		return val
	}

	// Try case-insensitive match
	lcFieldName := strings.ToLower(fieldName)
	for k, v := range data {
		if strings.ToLower(k) == lcFieldName {
			return v
		}
	}

	// Try matching based on common variations
	variations := []string{
		strings.ReplaceAll(fieldName, "_", ""), // Remove underscores
		strings.ReplaceAll(fieldName, "-", ""), // Remove hyphens
		strings.ReplaceAll(fieldName, " ", ""), // Remove spaces
		toCamelCase(fieldName),                 // camelCase
		toPascalCase(fieldName),                // PascalCase
		toSnakeCase(fieldName),                 // snake_case
	}

	for _, variant := range variations {
		for k, v := range data {
			if k == variant || strings.ToLower(k) == strings.ToLower(variant) {
				return v
			}
		}
	}

	return nil
}

// FindMatchingValuePublic exposes findMatchingValue for testing purposes
func (f *OutputFormatter) FindMatchingValuePublic(data map[string]interface{}, fieldName string) interface{} {
	return f.findMatchingValue(data, fieldName)
}

// OutputToJSON converts an Output struct to a JSON string with default/empty values
func (f *OutputFormatter) OutputToJSON(output *skill.Output) (string, error) {
	if output == nil {
		return "", fmt.Errorf("output definition is nil")
	}

	switch output.Type {
	case "string":
		// For string type, return the value or an empty string
		defaultValue := ""
		if output.Value != "" {
			defaultValue = output.Value
		}
		return fmt.Sprintf("\"%s\"", defaultValue), nil

	case "object":
		// For object type, create a map with default values for each field
		return f.objectToJSON(output.Fields)

	case "list[object]":
		// For list of objects, return an empty array or an array with a single default object
		return f.listObjectToJSON(output.Fields)

	case "list[string]":
		// For list of strings, return an empty array
		return "[]", nil

	case "list[number]":
		// For list of numbers, return an empty array
		return "[]", nil

	default:
		return "", fmt.Errorf("unsupported output type: %s", output.Type)
	}
}

// Helper function to create a JSON representation of an object with default values for each field
func (f *OutputFormatter) objectToJSON(fields []skill.OutputField) (string, error) {
	if len(fields) == 0 {
		return "{}", nil
	}

	// Create a map to hold the field values
	fieldMap := make(map[string]interface{})

	for _, field := range fields {
		fieldName := field.Name
		if fieldName == "" {
			fieldName = field.Value // If Name is empty, Value is the field name
		}

		if fieldName == "" {
			continue // Skip if both Name and Value are empty
		}

		// Handle different field types
		switch field.Type {
		case "object":
			// For nested object, recursively create defaults
			if len(field.Fields) > 0 {
				nestedJSON, err := f.objectToJSON(field.Fields)
				if err != nil {
					return "", err
				}
				var nestedObj interface{}
				if err := json.Unmarshal([]byte(nestedJSON), &nestedObj); err != nil {
					return "", err
				}
				fieldMap[fieldName] = nestedObj
			} else {
				fieldMap[fieldName] = make(map[string]interface{})
			}

		case "list[object]":
			// For list of objects, add an empty array or array with a default object
			if len(field.Fields) > 0 {
				listJSON, err := f.listObjectToJSON(field.Fields)
				if err != nil {
					return "", err
				}
				var listObj interface{}
				if err := json.Unmarshal([]byte(listJSON), &listObj); err != nil {
					return "", err
				}
				fieldMap[fieldName] = listObj
			} else {
				fieldMap[fieldName] = []interface{}{}
			}

		case "list[string]":
			// For list of strings, add an empty array
			fieldMap[fieldName] = []string{}

		case "list[number]":
			// For list of numbers, add an empty array
			fieldMap[fieldName] = []float64{}

		case "string", "":
			// For string or unspecified type, default to empty string
			fieldMap[fieldName] = ""

		case "number":
			// For number type, default to zero
			fieldMap[fieldName] = 0

		case "boolean":
			// For boolean type, default to false
			fieldMap[fieldName] = false

		default:
			// For unknown types, default to null
			fieldMap[fieldName] = nil
		}
	}

	// Convert the map to JSON
	jsonBytes, err := json.Marshal(fieldMap)
	if err != nil {
		return "", err
	}

	return string(jsonBytes), nil
}

// Helper function to create a JSON representation of a list of objects with default values
func (f *OutputFormatter) listObjectToJSON(fields []skill.OutputField) (string, error) {
	if len(fields) == 0 {
		return "[]", nil
	}

	// Create a default object with the fields
	objJSON, err := f.objectToJSON(fields)
	if err != nil {
		return "", err
	}

	// Return an array with a single default object
	return fmt.Sprintf("[%s]", objJSON), nil
}

// getDefaultValueForType returns a default value for a given type
func getDefaultValueForType(fieldType string) interface{} {
	switch fieldType {
	case "number":
		return float64(0)
	case "boolean":
		return false
	case "object":
		return map[string]interface{}{}
	case "list[object]":
		return []interface{}{}
	case "list[string]":
		return []string{}
	case "list[number]":
		return []float64{}
	default:
		return ""
	}
}

// parseNumber tries to parse a string as a float64
func parseNumber(s string) (float64, error) {
	return strconv.ParseFloat(s, 64)
}

// String case conversion utilities

func toCamelCase(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}

	// Split on spaces, underscores, hyphens
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == ' ' || r == '_' || r == '-'
	})

	// Convert first part to lowercase, the rest to title case
	result := strings.ToLower(parts[0])
	for i := 1; i < len(parts); i++ {
		result += titleCase(strings.ToLower(parts[i]))
	}

	return result
}

func toPascalCase(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}

	// Split on spaces, underscores, hyphens
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == ' ' || r == '_' || r == '-'
	})

	// Convert all parts to title case
	result := ""
	for _, part := range parts {
		result += titleCase(strings.ToLower(part))
	}

	return result
}

func toSnakeCase(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}

	// Replace spaces, hyphens with underscores
	s = strings.ReplaceAll(s, " ", "_")
	s = strings.ReplaceAll(s, "-", "_")

	// Handle camelCase and PascalCase
	var result strings.Builder
	for i, r := range s {
		if i > 0 && unicode.IsUpper(r) {
			result.WriteRune('_')
		}
		result.WriteRune(unicode.ToLower(r))
	}

	return result.String()
}

// titleCase converts the first character of s to uppercase (replaces deprecated strings.Title).
func titleCase(s string) string {
	if s == "" {
		return s
	}
	runes := []rune(s)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}

// flattenArray recursively flattens nested arrays into a single-level array
// Example: [[a,b], [c,d]] -> [a,b,c,d]
func flattenArray(arr []interface{}) []interface{} {
	result := make([]interface{}, 0)

	for _, item := range arr {
		// Check if the item is itself an array
		if nestedArr, ok := item.([]interface{}); ok {
			// Recursively flatten and append all items
			flattened := flattenArray(nestedArr)
			result = append(result, flattened...)
		} else {
			// Not an array, just append the item
			result = append(result, item)
		}
	}

	return result
}

// truncateString truncates a string to maxLen characters, adding "..." if truncated
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

// formatFileOutput handles file output type
func (f *OutputFormatter) formatFileOutput(
	ctx context.Context,
	messageID string,
	rawOutput string,
	stepResults map[int]interface{},
	valueTemplate string,
) (string, error) {
	// If there's a value template, try to resolve it
	if valueTemplate != "" {
		data := buildStepResultsDataMap(stepResults)
		replaced, err := f.variableReplacer.ReplaceVariables(ctx, valueTemplate, data)
		if err == nil && replaced != valueTemplate {
			if outputFormatterLogger != nil {
				outputFormatterLogger.Debugf("formatFileOutput - resolved template '%s' to '%s'", valueTemplate, replaced)
			}
			return replaced, nil
		}
	}

	// Look for FileResult in step results
	for idx, result := range stepResults {
		if fileResult, ok := result.(*skill.FileResult); ok {
			// Return URL if available, otherwise return JSON representation
			if fileResult.URL != "" {
				if outputFormatterLogger != nil {
					outputFormatterLogger.Debugf("formatFileOutput - returning URL from stepResults[%d]", idx)
				}
				return fileResult.URL, nil
			}
			// Return JSON representation
			jsonBytes, err := json.Marshal(fileResult)
			if err != nil {
				return "", fmt.Errorf("error serializing FileResult to JSON: %w", err)
			}
			if outputFormatterLogger != nil {
				outputFormatterLogger.Debugf("formatFileOutput - returning JSON from stepResults[%d]", idx)
			}
			return string(jsonBytes), nil
		}
	}

	// If no FileResult found, return raw output
	return rawOutput, nil
}

// formatListFileOutput handles list[file] output type
func (f *OutputFormatter) formatListFileOutput(
	ctx context.Context,
	messageID string,
	rawOutput string,
	stepResults map[int]interface{},
	valueTemplate string,
) (string, error) {
	var files []*skill.FileResult

	// If there's a value template, try to resolve it
	if valueTemplate != "" {
		data := buildStepResultsDataMap(stepResults)
		replaced, err := f.variableReplacer.ReplaceVariables(ctx, valueTemplate, data)
		if err == nil && replaced != valueTemplate {
			// Try to parse as JSON array of files
			var arr []interface{}
			if err := json.Unmarshal([]byte(replaced), &arr); err == nil {
				for _, item := range arr {
					if fileResult := interfaceToFileResult(item); fileResult != nil {
						files = append(files, fileResult)
					}
				}
			}
		}
	}

	// If no files from template, look in step results
	if len(files) == 0 {
		for _, result := range stepResults {
			// Check for single FileResult
			if fileResult, ok := result.(*skill.FileResult); ok {
				files = append(files, fileResult)
			}
			// Check for slice of FileResults
			if fileSlice, ok := result.([]*skill.FileResult); ok {
				files = append(files, fileSlice...)
			}
			// Check for slice of interfaces (might contain FileResults)
			if arr, ok := result.([]interface{}); ok {
				for _, item := range arr {
					if fileResult := interfaceToFileResult(item); fileResult != nil {
						files = append(files, fileResult)
					}
				}
			}
		}
	}

	if len(files) == 0 {
		if outputFormatterLogger != nil {
			outputFormatterLogger.Debugf("formatListFileOutput - no files found, returning raw output")
		}
		return rawOutput, nil
	}

	// Convert to JSON
	jsonBytes, err := json.MarshalIndent(files, "", "  ")
	if err != nil {
		return "", fmt.Errorf("error serializing file list to JSON: %w", err)
	}

	if outputFormatterLogger != nil {
		outputFormatterLogger.Debugf("formatListFileOutput - returning %d files", len(files))
	}
	return string(jsonBytes), nil
}

// interfaceToFileResult converts an interface{} to *skill.FileResult if possible
func interfaceToFileResult(item interface{}) *skill.FileResult {
	// Direct type assertion
	if fileResult, ok := item.(*skill.FileResult); ok {
		return fileResult
	}

	// Try to convert from map
	if itemMap, ok := item.(map[string]interface{}); ok {
		// Check if it looks like a FileResult
		if _, hasFileName := itemMap["fileName"]; hasFileName {
			if _, hasMimeType := itemMap["mimeType"]; hasMimeType {
				fileResult := &skill.FileResult{}
				if fileName, ok := itemMap["fileName"].(string); ok {
					fileResult.FileName = fileName
				}
				if mimeType, ok := itemMap["mimeType"].(string); ok {
					fileResult.MimeType = mimeType
				}
				if size, ok := itemMap["size"].(float64); ok {
					fileResult.Size = int64(size)
				} else if size, ok := itemMap["size"].(int64); ok {
					fileResult.Size = size
				}
				if tempPath, ok := itemMap["tempPath"].(string); ok {
					fileResult.TempPath = tempPath
				}
				if url, ok := itemMap["url"].(string); ok {
					fileResult.URL = url
				}
				return fileResult
			}
		}
	}

	return nil
}

// ExtractFilesFromStepResults extracts all FileResult objects from step results
// This is used when shouldBeHandledAsMessageToUser is true to collect files for attachment
func (f *OutputFormatter) ExtractFilesFromStepResults(stepResults map[int]interface{}) []*skill.FileResult {
	var files []*skill.FileResult

	for _, result := range stepResults {
		// Check for single FileResult
		if fileResult, ok := result.(*skill.FileResult); ok {
			files = append(files, fileResult)
			continue
		}

		// Check for slice of FileResults
		if fileSlice, ok := result.([]*skill.FileResult); ok {
			files = append(files, fileSlice...)
			continue
		}

		// Check for slice of interfaces (might contain FileResults)
		if arr, ok := result.([]interface{}); ok {
			for _, item := range arr {
				if fileResult := interfaceToFileResult(item); fileResult != nil {
					files = append(files, fileResult)
				}
			}
		}
	}

	return files
}

// FileResultToMediaType converts a FileResult to MediaType for message attachments
func FileResultToMediaType(fr *skill.FileResult) *MediaType {
	if fr == nil {
		return nil
	}
	return &MediaType{
		Url:      fr.URL,
		Mimetype: fr.MimeType,
		Filename: fr.FileName,
		FileSize: uint64(fr.Size),
	}
}

// FileResultsToMediaTypes converts a slice of FileResults to MediaTypes for message attachments
func FileResultsToMediaTypes(files []*skill.FileResult) []*MediaType {
	if len(files) == 0 {
		return nil
	}

	mediaTypes := make([]*MediaType, 0, len(files))
	for _, fr := range files {
		if mt := FileResultToMediaType(fr); mt != nil {
			mediaTypes = append(mediaTypes, mt)
		}
	}

	return mediaTypes
}

// -----------------------------------------------------------------------
// Inlined utility functions (from connect-ai/pkg/utils)
// -----------------------------------------------------------------------

// extractJSONFromText finds and returns the last valid JSON object/array in the given text.
func extractJSONFromText(response string) (string, error) {
	var jsonCandidates []string

	for i := 0; i < len(response); i++ {
		char := rune(response[i])
		if char == '{' || char == '[' {
			jsonStr, endIndex := extractCompleteJSONBlock(response, i)
			if jsonStr != "" {
				jsonCandidates = append(jsonCandidates, jsonStr)
				i = endIndex
			}
		}
	}

	if len(jsonCandidates) == 0 {
		return "", fmt.Errorf("no JSON content found in the response")
	}

	return strings.TrimSpace(jsonCandidates[len(jsonCandidates)-1]), nil
}

// extractCompleteJSONBlock extracts a complete JSON object/array starting at the given index.
func extractCompleteJSONBlock(text string, startIndex int) (string, int) {
	if startIndex >= len(text) {
		return "", startIndex
	}

	startChar := rune(text[startIndex])
	var endChar rune
	if startChar == '{' {
		endChar = '}'
	} else if startChar == '[' {
		endChar = ']'
	} else {
		return "", startIndex
	}

	braceCount := 1
	inString := false
	escaped := false

	for i := startIndex + 1; i < len(text); i++ {
		char := rune(text[i])

		if escaped {
			escaped = false
			continue
		}
		if char == '\\' && inString {
			escaped = true
			continue
		}
		if char == '"' {
			inString = !inString
			continue
		}
		if !inString {
			if char == startChar {
				braceCount++
			} else if char == endChar {
				braceCount--
				if braceCount == 0 {
					candidate := text[startIndex : i+1]
					var js interface{}
					if json.Unmarshal([]byte(candidate), &js) == nil {
						return candidate, i
					}
					return "", i
				}
			}
		}
	}

	return "", len(text)
}

// interfaceToString converts any value to its string representation.
func interfaceToString(obj interface{}) string {
	if obj == nil {
		return ""
	}
	switch v := obj.(type) {
	case string:
		return v
	case []byte:
		return string(v)
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64, bool:
		return fmt.Sprintf("%v", v)
	}
	jsonBytes, err := json.Marshal(obj)
	if err != nil {
		return fmt.Sprintf("%v", obj)
	}
	return string(jsonBytes)
}
