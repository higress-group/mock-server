package chat

import (
	"encoding/json"
	"fmt"
)

const (
	completionMockId = "chatcmpl-llm-mock"

	objectChatCompletion      = "chat.completion"
	objectChatCompletionChunk = "chat.completion.chunk"

	roleAssistant = "assistant"

	stopReason = "stop"

	contentTypeText     = "text"
	contentTypeImageUrl = "image_url"
)

var (
	completionMockCreated int64 = 10
	completionMockUsage         = usage{
		PromptTokens:     9,
		CompletionTokens: 1,
		TotalTokens:      10,
	}
)

type chatCompletionRequest struct {
	Model            string                 `json:"model" validate:"required"`
	Messages         []chatMessage          `json:"messages" validate:"required,min=1"`
	MaxTokens        int                    `json:"max_tokens,omitempty"`
	FrequencyPenalty float64                `json:"frequency_penalty,omitempty"`
	N                int                    `json:"n,omitempty"`
	PresencePenalty  float64                `json:"presence_penalty,omitempty"`
	Seed             int                    `json:"seed,omitempty"`
	Stream           bool                   `json:"stream,omitempty"`
	StreamOptions    *streamOptions         `json:"stream_options,omitempty"`
	Temperature      float64                `json:"temperature,omitempty"`
	TopP             float64                `json:"top_p,omitempty"`
	Tools            []tool                 `json:"tools,omitempty"`
	ToolChoice       *toolChoice            `json:"tool_choice,omitempty"`
	User             string                 `json:"user,omitempty"`
	Stop             []string               `json:"stop,omitempty"`
	ResponseFormat   map[string]interface{} `json:"response_format,omitempty"`
}

type streamOptions struct {
	IncludeUsage bool `json:"include_usage,omitempty"`
}

type tool struct {
	Type     string   `json:"type"`
	Function function `json:"function"`
}

type function struct {
	Description string                 `json:"description,omitempty"`
	Name        string                 `json:"name"`
	Parameters  map[string]interface{} `json:"parameters,omitempty"`
}

// toolChoice represents the different types of tool choice options
// It can be a string ("auto", "none", "required") or an object with specific configurations
type toolChoice struct {
	// For string values: "auto", "none", "required"
	StringValue *string `json:"-"`

	// For allowed_tools configuration
	AllowedTools *allowedToolsChoice `json:"-"`

	// For function tool choice
	FunctionChoice *functionToolChoice `json:"-"`

	// For custom tool choice
	CustomChoice *customToolChoice `json:"-"`
}

// allowedToolsChoice represents the allowed_tools configuration
type allowedToolsChoice struct {
	Type         string        `json:"type"`          // Always "allowed_tools"
	AllowedTools []allowedTool `json:"allowed_tools"` // Constrains the tools available to the model
}

// allowedTool represents a tool in the allowed tools list
type allowedTool struct {
	Mode     string   `json:"mode"`     // Tool mode
	Function function `json:"function"` // Function definition
}

// functionToolChoice represents a specific function tool choice
type functionToolChoice struct {
	Type     string   `json:"type"`     // Always "function"
	Function function `json:"function"` // The specific function to call
}

// customToolChoice represents a custom tool choice
type customToolChoice struct {
	Type   string     `json:"type"`   // Always "custom"
	Custom customTool `json:"custom"` // The custom tool configuration
}

// customTool represents a custom tool configuration
type customTool struct {
	Name string `json:"name"` // Custom tool name
}

// UnmarshalJSON implements custom JSON unmarshaling for toolChoice
// to handle string values ("auto", "none", "required") and different object types
func (tc *toolChoice) UnmarshalJSON(data []byte) error {
	// Try to unmarshal as string first
	var str string
	if err := json.Unmarshal(data, &str); err == nil {
		// Validate string value - only allow "auto", "none", "required"
		switch str {
		case "auto", "none", "required":
			tc.StringValue = &str
			return nil
		default:
			return fmt.Errorf("invalid tool_choice string value: %q, must be one of: \"auto\", \"none\", \"required\"", str)
		}
	}

	// If not a string, try to unmarshal as object
	// First, check the type field to determine which object type it is
	var typeCheck struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &typeCheck); err != nil {
		return err
	}

	switch typeCheck.Type {
	case "allowed_tools":
		var allowedTools allowedToolsChoice
		if err := json.Unmarshal(data, &allowedTools); err != nil {
			return err
		}
		tc.AllowedTools = &allowedTools
	case "function":
		var functionChoice functionToolChoice
		if err := json.Unmarshal(data, &functionChoice); err != nil {
			return err
		}
		tc.FunctionChoice = &functionChoice
	case "custom":
		var customChoice customToolChoice
		if err := json.Unmarshal(data, &customChoice); err != nil {
			return err
		}
		tc.CustomChoice = &customChoice
	default:
		// For backward compatibility, try to unmarshal as the old format
		var functionChoice functionToolChoice
		if err := json.Unmarshal(data, &functionChoice); err != nil {
			return err
		}
		tc.FunctionChoice = &functionChoice
	}

	return nil
}

// MarshalJSON implements custom JSON marshaling for toolChoice
func (tc *toolChoice) MarshalJSON() ([]byte, error) {
	if tc.StringValue != nil {
		return json.Marshal(*tc.StringValue)
	}
	if tc.AllowedTools != nil {
		return json.Marshal(tc.AllowedTools)
	}
	if tc.FunctionChoice != nil {
		return json.Marshal(tc.FunctionChoice)
	}
	if tc.CustomChoice != nil {
		return json.Marshal(tc.CustomChoice)
	}
	return json.Marshal(nil)
}

// IsString returns true if the tool choice is a string value
func (tc *toolChoice) IsString() bool {
	return tc.StringValue != nil
}

// GetStringValue returns the string value if it exists
func (tc *toolChoice) GetStringValue() string {
	if tc.StringValue != nil {
		return *tc.StringValue
	}
	return ""
}

// IsAllowedTools returns true if the tool choice is an allowed_tools configuration
func (tc *toolChoice) IsAllowedTools() bool {
	return tc.AllowedTools != nil
}

// IsFunction returns true if the tool choice is a function configuration
func (tc *toolChoice) IsFunction() bool {
	return tc.FunctionChoice != nil
}

// IsCustom returns true if the tool choice is a custom configuration
func (tc *toolChoice) IsCustom() bool {
	return tc.CustomChoice != nil
}

type chatCompletionResponse struct {
	Id                string                 `json:"id,omitempty"`
	Choices           []chatCompletionChoice `json:"choices"`
	Created           int64                  `json:"created,omitempty"`
	Model             string                 `json:"model,omitempty"`
	SystemFingerprint string                 `json:"system_fingerprint,omitempty"`
	Object            string                 `json:"object,omitempty"`
	Usage             *usage                 `json:"usage"`
}

type chatCompletionChoice struct {
	Index        int                    `json:"index"`
	Message      *chatMessage           `json:"message,omitempty"`
	Delta        *chatMessage           `json:"delta,omitempty"`
	FinishReason *string                `json:"finish_reason"`
	Logprobs     map[string]interface{} `json:"logprobs"`
}

type usage struct {
	PromptTokens     int `json:"prompt_tokens,omitempty"`
	CompletionTokens int `json:"completion_tokens,omitempty"`
	TotalTokens      int `json:"total_tokens,omitempty"`
}

type chatMessage struct {
	Name      string     `json:"name,omitempty"`
	Role      string     `json:"role,omitempty"`
	Content   any        `json:"content,omitempty"`
	ToolCalls []toolCall `json:"tool_calls,omitempty"`
}

type messageContent struct {
	Type     string    `json:"type,omitempty"`
	Text     string    `json:"text"`
	ImageUrl *imageUrl `json:"image_url,omitempty"`
}

type imageUrl struct {
	Url    string `json:"url,omitempty"`
	Detail string `json:"detail,omitempty"`
}

func (m *chatMessage) IsEmpty() bool {
	if m.IsStringContent() && m.Content != "" {
		return false
	}
	anyList, ok := m.Content.([]any)
	if ok && len(anyList) > 0 {
		return false
	}
	if len(m.ToolCalls) != 0 {
		nonEmpty := false
		for _, toolCall := range m.ToolCalls {
			if !toolCall.Function.IsEmpty() {
				nonEmpty = true
				break
			}
		}
		if nonEmpty {
			return false
		}
	}
	return true
}

func (m *chatMessage) IsStringContent() bool {
	_, ok := m.Content.(string)
	return ok
}

func (m *chatMessage) StringContent() string {
	content, ok := m.Content.(string)
	if ok {
		return content
	}
	contentList, ok := m.Content.([]any)
	if ok {
		var contentStr string
		for _, contentItem := range contentList {
			contentMap, ok := contentItem.(map[string]any)
			if !ok {
				continue
			}
			if contentMap["type"] == contentTypeText {
				if subStr, ok := contentMap[contentTypeText].(string); ok {
					contentStr += subStr + "\n"
				}
			}
		}
		return contentStr
	}
	return ""
}

func (m *chatMessage) ParseContent() []messageContent {
	var contentList []messageContent
	content, ok := m.Content.(string)
	if ok {
		contentList = append(contentList, messageContent{
			Type: contentTypeText,
			Text: content,
		})
		return contentList
	}
	anyList, ok := m.Content.([]any)
	if ok {
		for _, contentItem := range anyList {
			contentMap, ok := contentItem.(map[string]any)
			if !ok {
				continue
			}
			switch contentMap["type"] {
			case contentTypeText:
				if subStr, ok := contentMap[contentTypeText].(string); ok {
					contentList = append(contentList, messageContent{
						Type: contentTypeText,
						Text: subStr,
					})
				}
			case contentTypeImageUrl:
				if subObj, ok := contentMap[contentTypeImageUrl].(map[string]any); ok {
					contentList = append(contentList, messageContent{
						Type: contentTypeImageUrl,
						ImageUrl: &imageUrl{
							Url: subObj["url"].(string),
						},
					})
				}
			}
		}
		return contentList
	}
	return nil
}

type toolCall struct {
	Index    int          `json:"index"`
	Id       string       `json:"id"`
	Type     string       `json:"type"`
	Function functionCall `json:"function"`
}

type functionCall struct {
	Id        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

func (m *functionCall) IsEmpty() bool {
	return m.Name == "" && m.Arguments == ""
}
