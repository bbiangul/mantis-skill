package skill

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"
)

// TestInputParamsUnmarshal tests that Input can unmarshal with params field
// when using origin: function with from
func TestInputParamsUnmarshal(t *testing.T) {
	testCases := []struct {
		name     string
		yamlData string
		expected Input
	}{
		{
			name: "Input with origin function and params",
			yamlData: `
name: "dentistId"
origin: "function"
from: "lookupDentistByGender"
params:
  gender: "$genderPreference"
  clinic: "$COMPANY.id"
  default: "DEN-001"
`,
			expected: Input{
				Name:   "dentistId",
				Origin: "function",
				From:   "lookupDentistByGender",
				Params: map[string]interface{}{
					"gender":  "$genderPreference",
					"clinic":  "$COMPANY.id",
					"default": "DEN-001",
				},
			},
		},
		{
			name: "Input with origin function without params",
			yamlData: `
name: "userId"
origin: "function"
from: "getCurrentUser"
`,
			expected: Input{
				Name:   "userId",
				Origin: "function",
				From:   "getCurrentUser",
				Params: nil,
			},
		},
		{
			name: "Input with params using nested system variable",
			yamlData: `
name: "appointmentSlots"
origin: "function"
from: "getAvailableSlots"
params:
  dentistId: "$selectedDentist"
  date: "$appointmentDate"
  patientId: "$PATIENT.id"
`,
			expected: Input{
				Name:   "appointmentSlots",
				Origin: "function",
				From:   "getAvailableSlots",
				Params: map[string]interface{}{
					"dentistId": "$selectedDentist",
					"date":      "$appointmentDate",
					"patientId": "$PATIENT.id",
				},
			},
		},
		{
			name: "Input with empty params map",
			yamlData: `
name: "data"
origin: "function"
from: "fetchData"
params: {}
`,
			expected: Input{
				Name:   "data",
				Origin: "function",
				From:   "fetchData",
				Params: map[string]interface{}{},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var result Input
			err := yaml.Unmarshal([]byte(tc.yamlData), &result)
			assert.NoError(t, err)
			assert.Equal(t, tc.expected.Name, result.Name)
			assert.Equal(t, tc.expected.Origin, result.Origin)
			assert.Equal(t, tc.expected.From, result.From)

			if tc.expected.Params != nil {
				assert.NotNil(t, result.Params)
				assert.Equal(t, len(tc.expected.Params), len(result.Params))
				for k, v := range tc.expected.Params {
					assert.Equal(t, v, result.Params[k], "param %s mismatch", k)
				}
			} else {
				assert.Nil(t, result.Params)
			}
		})
	}
}

// TestInputParamsInFunction tests parsing a complete function with input that has params
func TestInputParamsInFunction(t *testing.T) {
	// Simplified test - just test the Function struct with inputs
	yamlData := `
name: "scheduleAppointment"
description: "Schedules an appointment with a dentist"
input:
  - name: "dentistId"
    description: "The dentist ID"
    origin: "function"
    from: "lookupDentistByGender"
    params:
      gender: "$genderPreference"
      clinic: "$COMPANY.id"
  - name: "date"
    description: "Appointment date"
    origin: "chat"
`

	var scheduleFunc Function
	err := yaml.Unmarshal([]byte(yamlData), &scheduleFunc)
	assert.NoError(t, err)
	assert.Equal(t, "scheduleAppointment", scheduleFunc.Name)

	// Check the dentistId input has params
	var dentistInput *Input
	for i := range scheduleFunc.Input {
		if scheduleFunc.Input[i].Name == "dentistId" {
			dentistInput = &scheduleFunc.Input[i]
			break
		}
	}
	assert.NotNil(t, dentistInput)
	assert.Equal(t, "function", dentistInput.Origin)
	assert.Equal(t, "lookupDentistByGender", dentistInput.From)
	assert.NotNil(t, dentistInput.Params)
	assert.Equal(t, "$genderPreference", dentistInput.Params["gender"])
	assert.Equal(t, "$COMPANY.id", dentistInput.Params["clinic"])
}
