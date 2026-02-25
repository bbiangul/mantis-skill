package skill

import (
	"strings"
	"testing"
)

// TestFunctionOriginDotNotation tests that dot notation in 'from' field is rejected
func TestFunctionOriginDotNotation(t *testing.T) {
	tests := []struct {
		name          string
		yaml          string
		expectError   bool
		errorContains string
	}{
		{
			name: "invalid - dot notation in from field (single field)",
			yaml: `version: "v1"
author: "test"
tools:
  - name: "testTool"
    description: "Test tool"
    version: "1.0.0"
    functions:
      - name: "getAppointmentDateTime"
        operation: "terminal"
        description: "Get appointment date and time"
        triggers:
          - type: "flex_for_user"
        steps:
          - name: "process"
            action: "sh"
            with:
              linux: |
                echo '{"dataAgendamento":"16/11/2024","horarioAgendamento":"14:00","fimAgendamento":"14:15"}'
              windows: |
                echo {"dataAgendamento":"16/11/2024","horarioAgendamento":"14:00","fimAgendamento":"14:15"}

      - name: "createAppointment"
        operation: "api_call"
        description: "Create appointment"
        triggers:
          - type: "flex_for_user"
        needs: ["getAppointmentDateTime"]
        input:
          - name: "dataAgendamento"
            description: "Appointment date"
            origin: "function"
            from: "getAppointmentDateTime.dataAgendamento"
            onError:
              strategy: "requestUserInput"
              message: "Please provide date"
        steps:
          - name: "create"
            action: "POST"
            with:
              url: "http://example.com/appointment"
`,
			expectError:   true,
			errorContains: "dot notation in 'from' field",
		},
		{
			name: "invalid - dot notation in from field (multiple fields)",
			yaml: `version: "v1"
author: "test"
tools:
  - name: "testTool"
    description: "Test tool"
    version: "1.0.0"
    functions:
      - name: "getAppointmentDateTime"
        operation: "terminal"
        description: "Get appointment date and time"
        triggers:
          - type: "flex_for_user"
        steps:
          - name: "process"
            action: "sh"
            with:
              linux: |
                echo '{"dataAgendamento":"16/11/2024","horarioAgendamento":"14:00","fimAgendamento":"14:15"}'
              windows: |
                echo {"dataAgendamento":"16/11/2024","horarioAgendamento":"14:00","fimAgendamento":"14:15"}

      - name: "createAppointment"
        operation: "api_call"
        description: "Create appointment"
        triggers:
          - type: "flex_for_user"
        needs: ["getAppointmentDateTime"]
        input:
          - name: "dataAgendamento"
            description: "Appointment date"
            origin: "function"
            from: "getAppointmentDateTime.dataAgendamento"
            onError:
              strategy: "requestUserInput"
              message: "Please provide date"
          - name: "horarioAgendamento"
            description: "Appointment time"
            origin: "function"
            from: "getAppointmentDateTime.horarioAgendamento"
            onError:
              strategy: "requestUserInput"
              message: "Please provide time"
          - name: "fimAgendamento"
            description: "Appointment end time"
            origin: "function"
            from: "getAppointmentDateTime.fimAgendamento"
            onError:
              strategy: "requestUserInput"
              message: "Please provide end time"
        steps:
          - name: "create"
            action: "POST"
            with:
              url: "http://example.com/appointment"
`,
			expectError:   true,
			errorContains: "dot notation in 'from' field",
		},
		{
			name: "valid - whole object in input, field access in steps",
			yaml: `version: "v1"
author: "test"
tools:
  - name: "testTool"
    description: "Test tool"
    version: "1.0.0"
    functions:
      - name: "getAppointmentDateTime"
        operation: "terminal"
        description: "Get appointment date and time"
        triggers:
          - type: "flex_for_user"
        steps:
          - name: "process"
            action: "sh"
            with:
              linux: |
                echo '{"dataAgendamento":"16/11/2024","horarioAgendamento":"14:00","fimAgendamento":"14:15"}'
              windows: |
                echo {"dataAgendamento":"16/11/2024","horarioAgendamento":"14:00","fimAgendamento":"14:15"}

      - name: "createAppointment"
        operation: "api_call"
        description: "Create appointment"
        triggers:
          - type: "flex_for_user"
        needs: ["getAppointmentDateTime"]
        input:
          - name: "appointmentDateTime"
            description: "Appointment date time object"
            origin: "function"
            from: "getAppointmentDateTime"
            onError:
              strategy: "requestUserInput"
              message: "Please provide date and time"
        steps:
          - name: "create"
            action: "POST"
            with:
              url: "http://example.com/appointment"
              requestBody:
                type: "application/json"
                with:
                  dataAgendamento: "$appointmentDateTime.dataAgendamento"
                  horarioAgendamento: "$appointmentDateTime.horarioAgendamento"
                  fimAgendamento: "$appointmentDateTime.fimAgendamento"
`,
			expectError: false,
		},
		{
			name: "valid - without dot notation (whole object)",
			yaml: `version: "v1"
author: "test"
tools:
  - name: "testTool"
    description: "Test tool"
    version: "1.0.0"
    functions:
      - name: "getUser"
        operation: "terminal"
        description: "Get user"
        triggers:
          - type: "flex_for_user"
        steps:
          - name: "fetch"
            action: "sh"
            with:
              linux: |
                echo '{"id": 123}'
              windows: |
                echo {"id": 123}

      - name: "process"
        operation: "api_call"
        description: "Process"
        triggers:
          - type: "flex_for_user"
        needs: ["getUser"]
        input:
          - name: "userData"
            description: "User data"
            origin: "function"
            from: "getUser"
            onError:
              strategy: "requestUserInput"
              message: "Please provide data"
        steps:
          - name: "save"
            action: "POST"
            with:
              url: "http://example.com/save"
`,
			expectError: false,
		},
		{
			name: "invalid - dot notation in from field with non-existent function",
			yaml: `version: "v1"
author: "test"
tools:
  - name: "testTool"
    description: "Test tool"
    version: "1.0.0"
    functions:
      - name: "process"
        operation: "api_call"
        description: "Process"
        triggers:
          - type: "flex_for_user"
        input:
          - name: "email"
            description: "Email"
            origin: "function"
            from: "nonExistentFunction.email"
            onError:
              strategy: "requestUserInput"
              message: "Please provide email"
        steps:
          - name: "save"
            action: "POST"
            with:
              url: "http://example.com/save"
`,
			expectError:   true,
			errorContains: "dot notation in 'from' field",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := CreateTool(tt.yaml)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
					return
				}
				if tt.errorContains != "" && !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("expected error to contain '%s', but got: %s", tt.errorContains, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}
