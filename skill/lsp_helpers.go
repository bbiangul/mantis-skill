package skill

import (
	"regexp"
	"strconv"
)

// ExtractInputReferences extracts all input variable references from a string
// Returns input names that are found in the provided inputNames list
func ExtractInputReferences(value string, inputNames []string) []string {
	if value == "" || len(inputNames) == 0 {
		return nil
	}

	// Build a set of input names for quick lookup
	inputSet := make(map[string]bool)
	for _, name := range inputNames {
		inputSet[name] = true
	}

	pattern := regexp.MustCompile(`\$([a-zA-Z_][a-zA-Z0-9_]*)`)
	matches := pattern.FindAllStringSubmatch(value, -1)

	var refs []string
	seen := make(map[string]bool)
	for _, match := range matches {
		if len(match) > 1 {
			name := match[1]
			if inputSet[name] && !seen[name] {
				refs = append(refs, name)
				seen[name] = true
			}
		}
	}
	return refs
}

// ExtractResultReferences extracts result[N] references from a string
// Returns the indices of referenced results
func ExtractResultReferences(value string) []int {
	if value == "" {
		return nil
	}

	pattern := regexp.MustCompile(`result\[(\d+)\]`)
	matches := pattern.FindAllStringSubmatch(value, -1)

	var refs []int
	seen := make(map[int]bool)
	for _, match := range matches {
		if len(match) > 1 {
			if idx, err := strconv.Atoi(match[1]); err == nil && !seen[idx] {
				refs = append(refs, idx)
				seen[idx] = true
			}
		}
	}
	return refs
}

// GetAvailableSystemVars returns system variables available for a trigger type
func GetAvailableSystemVars(triggerType string) []string {
	if triggerType == "" {
		// Return all system variables if no trigger type is specified
		return []string{SysVarMe, SysVarMessage, SysVarNow, SysVarUser, SysVarAdmin, SysVarCompany, SysVarUUID, SysVarFile, SysVarMeeting}
	}

	availability, ok := triggerVarAvailability[triggerType]
	if !ok {
		return nil
	}

	var vars []string
	for v, available := range availability {
		if available {
			vars = append(vars, v)
		}
	}
	return vars
}

// GetAvailableSystemFuncs returns system functions available for a trigger type
func GetAvailableSystemFuncs(triggerType string) []string {
	if triggerType == "" {
		// Return common system functions if no trigger type is specified
		return []string{SysFuncAsk, SysFuncLearn, SysFuncAskHuman, SysFuncNotifyHuman}
	}

	availability, ok := triggerFuncAvailability[triggerType]
	if !ok {
		return nil
	}

	var funcs []string
	for f, available := range availability {
		if available {
			funcs = append(funcs, f)
		}
	}
	return funcs
}

// GetSystemVarFields returns valid fields for a system variable
func GetSystemVarFields(sysVar string) []string {
	fields, ok := systemVarFields[sysVar]
	if !ok {
		return nil
	}

	var fieldNames []string
	for field := range fields {
		fieldNames = append(fieldNames, field)
	}
	return fieldNames
}

// BuildFunctionDependencyMap returns a map of function name -> direct dependencies
func BuildFunctionDependencyMap(functions []Function) map[string][]string {
	depMap := make(map[string][]string)

	for _, fn := range functions {
		var deps []string

		// Add needs dependencies
		for _, need := range fn.Needs {
			deps = append(deps, need.Name)
		}

		// Add onSuccess dependencies
		for _, call := range fn.OnSuccess {
			deps = append(deps, call.Name)
		}

		// Add onFailure dependencies
		for _, call := range fn.OnFailure {
			deps = append(deps, call.Name)
		}

		// Add onSkip dependencies
		for _, call := range fn.OnSkip {
			deps = append(deps, call.Name)
		}

		depMap[fn.Name] = deps
	}

	return depMap
}

// GetReachableFunctions returns all functions reachable from a given function
func GetReachableFunctions(funcName string, dependencyMap map[string][]string) []string {
	visited := make(map[string]bool)
	var result []string

	var visit func(name string)
	visit = func(name string) {
		if visited[name] {
			return
		}
		visited[name] = true

		if name != funcName { // Don't include the starting function
			result = append(result, name)
		}

		for _, dep := range dependencyMap[name] {
			visit(dep)
		}
	}

	visit(funcName)
	return result
}

// GetAllFunctionNames returns all function names from a list of functions
func GetAllFunctionNames(functions []Function) []string {
	var names []string
	for _, fn := range functions {
		names = append(names, fn.Name)
	}
	return names
}

// GetFunctionInputNames returns all input names for a function
func GetFunctionInputNames(fn Function) []string {
	var names []string
	for _, input := range fn.Input {
		names = append(names, input.Name)
	}
	return names
}

// IsSystemFunctionLSP checks if a name is a system function (for LSP use)
// Alias for systemFunctions map access
func IsSystemFunctionLSP(name string) bool {
	return systemFunctions[name]
}

// GetAllSystemFunctions returns all system function names
func GetAllSystemFunctions() []string {
	var funcs []string
	for name := range systemFunctions {
		funcs = append(funcs, name)
	}
	return funcs
}

// GetAllSystemVariables returns all system variable names (without $ prefix)
func GetAllSystemVariables() []string {
	return []string{
		"ME", "MESSAGE", "NOW", "USER", "ADMIN", "COMPANY", "UUID", "FILE", "MEETING",
		"AUTH_TOKEN", // Runtime variable set by extractAuthToken in isAuthentication steps
	}
}
