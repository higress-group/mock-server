package responses

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// mockCreatedAt is fixed (mirroring the chat simulation's created field) so
// conformance assertions can compare the full response body deterministically.
const mockCreatedAt = 10

// responsesRequest mirrors the OpenAI Responses API request body; only the
// fields needed by the simulation are modeled.
type responsesRequest struct {
	Model string      `json:"model"`
	Input interface{} `json:"input"`
}

type responsesResponse struct {
	Id        string           `json:"id"`
	Object    string           `json:"object"`
	CreatedAt int64            `json:"created_at"`
	Status    string           `json:"status"`
	Model     string           `json:"model"`
	Output    []responseOutput `json:"output"`
	Usage     responsesUsage   `json:"usage"`
}

type responseOutput struct {
	Type    string          `json:"type"`
	Id      string          `json:"id"`
	Status  string          `json:"status"`
	Role    string          `json:"role"`
	Content []outputContent `json:"content"`
}

type outputContent struct {
	Type        string        `json:"type"`
	Text        string        `json:"text"`
	Annotations []interface{} `json:"annotations"`
}

type responsesUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

// HandleResponses simulates the OpenAI Responses API endpoint, served by
// groq under /openai/v1/responses and by OpenAI under /v1/responses. The
// assistant message echoes the input text so conformance cases can assert
// the full response body deterministically.
func HandleResponses(ctx *gin.Context) {
	var request responsesRequest
	if err := ctx.ShouldBindJSON(&request); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	text := inputText(request.Input)
	if text == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "input must not be empty"})
		return
	}

	inputTokens := len([]rune(text))
	if inputTokens == 0 {
		inputTokens = 1
	}

	ctx.JSON(http.StatusOK, responsesResponse{
		Id:        "resp_123",
		Object:    "response",
		CreatedAt: mockCreatedAt,
		Status:    "completed",
		Model:     request.Model,
		Output: []responseOutput{{
			Type:   "message",
			Id:     "msg_123",
			Status: "completed",
			Role:   "assistant",
			Content: []outputContent{{
				Type:        "output_text",
				Text:        text,
				Annotations: []interface{}{},
			}},
		}},
		Usage: responsesUsage{
			InputTokens:  inputTokens,
			OutputTokens: len([]rune(text)),
			TotalTokens:  2 * inputTokens,
		},
	})
}

// inputText flattens a Responses API input, which may be a string or an
// array of message objects with string content parts.
func inputText(input interface{}) string {
	switch v := input.(type) {
	case string:
		return v
	case []interface{}:
		text := ""
		for _, item := range v {
			switch m := item.(type) {
			case string:
				text += m
			case map[string]interface{}:
				if content, ok := m["content"].(string); ok {
					text += content
				}
				if parts, ok := m["content"].([]interface{}); ok {
					for _, part := range parts {
						if pm, ok := part.(map[string]interface{}); ok {
							if t, ok := pm["text"].(string); ok {
								text += t
							}
						}
					}
				}
			}
		}
		return text
	default:
		return ""
	}
}
