package utils

import "strings"

const (
	MessageNeedsAdditionalInfo   = "i need some additional information to complete this action"
	MessageNeedsUserConfirmation = "i need the user confirmation before proceed"
	MessageNeedsTeamApproval     = "this action requires team approval"
)

type SpecialReturnType int

const (
	SpecialReturnNone SpecialReturnType = iota
	SpecialReturnNeedsInfo
	SpecialReturnNeedsConfirmation
	SpecialReturnNeedsApproval
)

type SpecialReturn struct {
	Type        SpecialReturnType
	Message     string
	TeamMessage string
}

func CheckSpecialReturn(result string) *SpecialReturn {
	lowerResult := strings.ToLower(result)

	if strings.Contains(lowerResult, MessageNeedsAdditionalInfo) {
		return &SpecialReturn{
			Type:    SpecialReturnNeedsInfo,
			Message: result,
		}
	}

	if strings.Contains(lowerResult, MessageNeedsUserConfirmation) {
		return &SpecialReturn{
			Type:    SpecialReturnNeedsConfirmation,
			Message: result,
		}
	}

	if strings.Contains(lowerResult, MessageNeedsTeamApproval) {
		parts := strings.Split(result, "|||")
		teamMsg := ""
		if len(parts) >= 2 {
			teamMsg = parts[1]
		}
		return &SpecialReturn{
			Type:        SpecialReturnNeedsApproval,
			Message:     result,
			TeamMessage: teamMsg,
		}
	}

	return &SpecialReturn{
		Type:    SpecialReturnNone,
		Message: result,
	}
}

func IsSpecialReturn(result string) bool {
	sr := CheckSpecialReturn(result)
	return sr.Type != SpecialReturnNone
}
