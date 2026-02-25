package models

import (
	"github.com/bbiangul/mantis-skill/skill"
)

// FlexFunction represents a function with a flex trigger type
type FlexFunction struct {
	Tool     skill.Tool
	Function skill.Function
}
