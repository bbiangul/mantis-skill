package models

import (
	"time"
)

// StepResult represents the result of a specific step with a result index
type StepResult struct {
	ID           int64     `json:"id"`
	MessageID    string    `json:"message_id"`
	FunctionName string    `json:"function_name"`
	ResultIndex  int       `json:"result_index"`
	ResultData   string    `json:"result_data"` // JSON-encoded result data
	CreatedAt    time.Time `json:"created_at"`
}
