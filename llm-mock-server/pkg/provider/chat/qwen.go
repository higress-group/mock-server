package chat

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"llm-mock-server/pkg/utils"
)

const (
	qwenDomain              = "dashscope.aliyuncs.com"
	qwenChatCompletionPath  = "/api/v1/services/aigc/text-generation/generation"
	qwenResultFormatMessage = "message"
)

type qwenProvider struct {
}

func (p *qwenProvider) ShouldHandleRequest(ctx *gin.Context) bool {
	context, _ := getRequestContext(ctx)
	if context.Host == qwenDomain && context.Path == qwenChatCompletionPath {
		return true
	}
	return false
}

func (p *qwenProvider) HandleChatCompletions(ctx *gin.Context) {
	// Validate Authorization header
	authHeader := ctx.GetHeader("Authorization")
	if authHeader == "" {
		p.sendErrorResponse(ctx, http.StatusUnauthorized,
			"InvalidApiKey", "No API-key provided.")
		return
	}

	// Bind request body
	var chatRequest qwenTextGenRequest
	if err := ctx.ShouldBindJSON(&chatRequest); err != nil {
		p.sendErrorResponse(ctx, http.StatusBadRequest,
			"InvalidParameter", fmt.Sprintf("invalid params: %v", err.Error()))
		return
	}

	// Validate request body
	if err := utils.Validate.Struct(chatRequest); err != nil {
		validationErrors := err.(validator.ValidationErrors)
		for _, fieldError := range validationErrors {
			p.sendErrorResponse(ctx, http.StatusBadRequest,
				"InvalidParameter", fmt.Sprintf("invalid params: %v", fieldError.Error()))
			return
		}
	}

	prompt := ""
	messages := chatRequest.Input.Messages
	if messages[len(messages)-1].IsStringContent() {
		prompt = messages[len(messages)-1].StringContent()
	}
	response := prompt2Response(prompt)

	// Determine if the request is a stream request
	isStream := p.isStreamRequest(ctx)

	if isStream {
		// todo stream response
	} else {
		p.handleNonStreamResponse(ctx, chatRequest, response)
	}
}

func (p *qwenProvider) sendErrorResponse(ctx *gin.Context, statusCode int, errorCode, errorMsg string) {
	errorResp := qwenErrorResp{
		Code:      errorCode,
		Message:   errorMsg,
		RequestId: completionMockId,
	}
	ctx.JSON(statusCode, errorResp)
}

// isStreamRequest checks if the request is a stream request.
func (p *qwenProvider) isStreamRequest(ctx *gin.Context) bool {
	acceptHeader := ctx.GetHeader("Accept")
	sseHeader := ctx.GetHeader("X-DashScope-SSE")

	// Check if Accept header is text/event-stream or X-DashScope-SSE is set to enable
	if acceptHeader == "text/event-stream" || sseHeader == "enable" {
		return true
	}
	return false
}

func (p *qwenProvider) handleNonStreamResponse(ctx *gin.Context, chatRequest qwenTextGenRequest, response string) {
	completion := createQwenTextGenResponse(chatRequest, response)
	ctx.JSON(http.StatusOK, completion)
}

type qwenErrorResp struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestId string `json:"request_id"`
}

type qwenTextGenRequest struct {
	Model      string                `json:"model"`
	Input      qwenTextGenInput      `json:"input"`
	Parameters qwenTextGenParameters `json:"parameters,omitempty"`
}

type qwenTextGenInput struct {
	Messages []qwenMessage `json:"messages"`
}

type qwenTextGenParameters struct {
	ResultFormat      string  `json:"result_format,omitempty"`
	MaxTokens         int     `json:"max_tokens,omitempty"`
	RepetitionPenalty float64 `json:"repetition_penalty,omitempty"`
	N                 int     `json:"n,omitempty"`
	Seed              int     `json:"seed,omitempty"`
	Temperature       float64 `json:"temperature,omitempty"`
	TopP              float64 `json:"top_p,omitempty"`
	IncrementalOutput bool    `json:"incremental_output,omitempty"`
	EnableSearch      bool    `json:"enable_search,omitempty"`
	Tools             []tool  `json:"tools,omitempty"`
}

type qwenTextGenResponse struct {
	RequestId string            `json:"request_id"`
	Output    qwenTextGenOutput `json:"output"`
	Usage     qwenUsage         `json:"usage"`
}

func createQwenTextGenResponse(chatRequest qwenTextGenRequest, response string) qwenTextGenResponse {
	var output qwenTextGenOutput
	if chatRequest.Parameters.ResultFormat == qwenResultFormatMessage {
		output = qwenTextGenOutput{
			Choices: []qwenTextGenChoice{
				{
					FinishReason: stopReason,
					Message: qwenMessage{
						Role:    roleAssistant,
						Content: response,
					},
				},
			},
		}
	} else {
		output = qwenTextGenOutput{
			FinishReason: stopReason,
			Text:         response,
		}
	}
	return qwenTextGenResponse{
		Output: output,
		Usage: qwenUsage{
			InputTokens:  9,
			OutputTokens: 1,
			TotalTokens:  10,
		},
		RequestId: completionMockId,
	}
}

type qwenTextGenOutput struct {
	FinishReason string              `json:"finish_reason,omitempty"`
	Text         string              `json:"text,omitempty"`
	Choices      []qwenTextGenChoice `json:"choices,omitempty"`
}

type qwenTextGenChoice struct {
	FinishReason string      `json:"finish_reason"`
	Message      qwenMessage `json:"message"`
}

type qwenUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

type qwenMessage struct {
	Name      string     `json:"name,omitempty"`
	Role      string     `json:"role"`
	Content   any        `json:"content"`
	ToolCalls []toolCall `json:"tool_calls,omitempty"`
}

func (m *qwenMessage) IsStringContent() bool {
	_, ok := m.Content.(string)
	return ok
}

func (m *qwenMessage) StringContent() string {
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
