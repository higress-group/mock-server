package chat

import (
	"encoding/json"
	"testing"
)

func TestToolChoiceUnmarshalJSON(t *testing.T) {
	tests := []struct {
		name        string
		jsonData    string
		expected    func(*toolChoice) bool
		expectError bool
	}{
		{
			name:     "string value - auto",
			jsonData: `"auto"`,
			expected: func(tc *toolChoice) bool {
				return tc.IsString() && tc.GetStringValue() == "auto"
			},
		},
		{
			name:     "string value - none",
			jsonData: `"none"`,
			expected: func(tc *toolChoice) bool {
				return tc.IsString() && tc.GetStringValue() == "none"
			},
		},
		{
			name:     "string value - required",
			jsonData: `"required"`,
			expected: func(tc *toolChoice) bool {
				return tc.IsString() && tc.GetStringValue() == "required"
			},
		},
		{
			name:        "invalid string value - invalid",
			jsonData:    `"invalid"`,
			expectError: true,
		},
		{
			name:        "invalid string value - empty",
			jsonData:    `""`,
			expectError: true,
		},
		{
			name:        "invalid string value - random",
			jsonData:    `"random_value"`,
			expectError: true,
		},
		{
			name: "allowed_tools configuration",
			jsonData: `{
				"type": "allowed_tools",
				"allowed_tools": [
					{
						"mode": "function",
						"function": {
							"name": "get_weather",
							"description": "Get weather information"
						}
					}
				]
			}`,
			expected: func(tc *toolChoice) bool {
				return tc.IsAllowedTools() &&
					tc.AllowedTools.Type == "allowed_tools" &&
					len(tc.AllowedTools.AllowedTools) == 1 &&
					tc.AllowedTools.AllowedTools[0].Function.Name == "get_weather"
			},
		},
		{
			name: "function tool choice",
			jsonData: `{
				"type": "function",
				"function": {
					"name": "calculate_sum",
					"description": "Calculate sum of numbers"
				}
			}`,
			expected: func(tc *toolChoice) bool {
				return tc.IsFunction() &&
					tc.FunctionChoice.Type == "function" &&
					tc.FunctionChoice.Function.Name == "calculate_sum"
			},
		},
		{
			name: "custom tool choice",
			jsonData: `{
				"type": "custom",
				"custom": {
					"name": "my_custom_tool"
				}
			}`,
			expected: func(tc *toolChoice) bool {
				return tc.IsCustom() &&
					tc.CustomChoice.Type == "custom" &&
					tc.CustomChoice.Custom.Name == "my_custom_tool"
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var tc toolChoice
			err := json.Unmarshal([]byte(tt.jsonData), &tc)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error for %s, but got none", tt.name)
				}
				return
			}

			if err != nil {
				t.Fatalf("Failed to unmarshal JSON: %v", err)
			}

			if !tt.expected(&tc) {
				t.Errorf("Test failed for %s", tt.name)
			}
		})
	}
}

func TestToolChoiceMarshalJSON(t *testing.T) {
	tests := []struct {
		name      string
		jsonInput string
		expected  string
	}{
		{
			name:      "string value",
			jsonInput: `"auto"`,
			expected:  `"auto"`,
		},
		{
			name:      "function choice",
			jsonInput: `{"type":"function","function":{"name":"test_function","description":"A test function"}}`,
			expected:  `{"type":"function","function":{"description":"A test function","name":"test_function"}}`,
		},
		{
			name:      "custom choice",
			jsonInput: `{"type":"custom","custom":{"name":"my_tool"}}`,
			expected:  `{"type":"custom","custom":{"name":"my_tool"}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// First unmarshal to create the toolChoice
			var tc toolChoice
			err := json.Unmarshal([]byte(tt.jsonInput), &tc)
			if err != nil {
				t.Fatalf("Failed to unmarshal JSON: %v", err)
			}

			// Then marshal it back
			data, err := json.Marshal(&tc)
			if err != nil {
				t.Fatalf("Failed to marshal JSON: %v", err)
			}

			if string(data) != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, string(data))
			}
		})
	}
}
