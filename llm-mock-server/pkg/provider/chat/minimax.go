package chat

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"llm-mock-server/pkg/utils"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
)

const (
	minimaxDomain = "api.minimax.chat"
	// minimaxChatCompletionProPath represents the API path for chat completion Pro API which has a different response format from OpenAI's.
	minimaxChatCompletionProPath = "/v1/text/chatcompletion_pro"
)

type minimaxProvider struct{}

func (p *minimaxProvider) ShouldHandleRequest(ctx *gin.Context) bool {
	context, _ := getRequestContext(ctx)
	if context.Host == minimaxDomain && context.Path == minimaxChatCompletionProPath {
		return true
	}
	return false
}

func (p *minimaxProvider) HandleChatCompletions(ctx *gin.Context) {
	// Validate Authorization header
	authHeader := ctx.GetHeader("Authorization")
	if authHeader == "" {
		p.sendErrorResponse(ctx, 1004,
			"login fail: Please carry the API secret key in the 'Authorization' field of the request header")
		return
	}

	// Bind request body
	var chatRequest minimaxChatCompletionProRequest
	if err := ctx.ShouldBindJSON(&chatRequest); err != nil {
		p.sendErrorResponse(ctx, 2013,
			fmt.Sprintf("invalid params: %v", err.Error()))
		return
	}

	// Validate request body
	if err := utils.Validate.Struct(chatRequest); err != nil {
		validationErrors := err.(validator.ValidationErrors)
		for _, fieldError := range validationErrors {
			p.sendErrorResponse(ctx, 2013,
				fmt.Sprintf("invalid params: %v", fieldError.Error()))
			return
		}
	}

	senderType := chatRequest.ReplyConstraints.SenderType
	senderName := chatRequest.ReplyConstraints.SenderName
	// Generate reply based on the last message in the request
	reply := prompt2Response(chatRequest.Messages[len(chatRequest.Messages)-1].Text)

	// Handle stream or non-stream response based on the request
	if chatRequest.Stream {
		p.handleStreamResponse(ctx, chatRequest, senderType, senderName, reply)
	} else {
		p.handleNonStreamResponse(ctx, chatRequest, senderType, senderName, reply)
	}
}

func (p *minimaxProvider) sendErrorResponse(ctx *gin.Context, respCode int, respMsg string) {
	baseResp := minimaxBaseResp{
		StatusCode: int64(respCode),
		StatusMsg:  respMsg,
	}
	ctx.JSON(http.StatusOK, gin.H{
		"base_resp": baseResp,
	})
}

func (p *minimaxProvider) handleStreamResponse(ctx *gin.Context, chatRequest minimaxChatCompletionProRequest, senderType, senderName, reply string) {
	utils.SetEventStreamHeaders(ctx)
	dataChan := make(chan string)
	stopChan := make(chan bool, 1)
	streamResponse := minimaxChatCompletionProResp{
		Created: completionMockCreated,
		Model:   chatRequest.Model,
	}
	go func() {
		for _, s := range reply {
			streamResponse.Choices = []minimaxChoice{
				{
					Messages: []minimaxMessage{
						{
							SenderType: senderType,
							SenderName: senderName,
							Text:       string(s),
						},
					},
				},
			}
			jsonStr, _ := json.Marshal(streamResponse)
			dataChan <- string(jsonStr)

			// Simulate response delay
			time.Sleep(200 * time.Millisecond)
		}
		stopChan <- true
	}()

	ctx.Stream(func(w io.Writer) bool {
		select {
		case data := <-dataChan:
			ctx.Render(-1, streamEvent{Data: "data: " + data})
			return true
		case <-stopChan:
			jsonStr, _ := json.Marshal(p.createProResp(chatRequest.Model, senderType, senderName, reply))
			ctx.Render(-1, streamEvent{Data: fmt.Sprintf("data: %s", jsonStr)})
			return false
		}
	})
}

func (p *minimaxProvider) handleNonStreamResponse(ctx *gin.Context, chatRequest minimaxChatCompletionProRequest, senderType, senderName, reply string) {
	completion := p.createProResp(chatRequest.Model, senderType, senderName, reply)
	ctx.JSON(http.StatusOK, completion)
}

func (p *minimaxProvider) createProResp(model, senderType, senderName, reply string) minimaxChatCompletionProResp {
	return minimaxChatCompletionProResp{
		Created:         completionMockCreated,
		Model:           model,
		Reply:           reply,
		InputSensitive:  false,
		OutputSensitive: false,
		Choices: []minimaxChoice{
			{
				Index: 0,
				Messages: []minimaxMessage{
					{
						SenderType: senderType,
						SenderName: senderName,
						Text:       reply,
					},
				},
				FinishReason: stopReason,
			},
		},
		Usage: completionMockUsage,
		Id:    completionMockId,
		BaseResp: minimaxBaseResp{
			StatusCode: 0,
			StatusMsg:  "",
		},
	}
}

// minimaxChatCompletionProRequest represents the structure of a chat completion Pro request.
type minimaxChatCompletionProRequest struct {
	Model             string                  `json:"model" validate:"required"`
	Stream            bool                    `json:"stream,omitempty"`
	TokensToGenerate  int64                   `json:"tokens_to_generate,omitempty"`
	Temperature       float64                 `json:"temperature,omitempty"`
	TopP              float64                 `json:"top_p,omitempty"`
	MaskSensitiveInfo bool                    `json:"mask_sensitive_info"`
	Messages          []minimaxMessage        `json:"messages" validate:"required,min=1"`
	BotSettings       []minimaxBotSetting     `json:"bot_setting" validate:"required,min=1"`
	ReplyConstraints  minimaxReplyConstraints `json:"reply_constraints" validate:"required"`
}

// minimaxMessage represents a message in the conversation.
type minimaxMessage struct {
	SenderType string `json:"sender_type"`
	SenderName string `json:"sender_name"`
	Text       string `json:"text"`
}

// minimaxBotSetting represents the bot's settings.
type minimaxBotSetting struct {
	BotName string `json:"bot_name"`
	Content string `json:"content"`
}

// minimaxReplyConstraints represents requirements for model replies.
type minimaxReplyConstraints struct {
	SenderType string `json:"sender_type"`
	SenderName string `json:"sender_name"`
}

// minimaxChatCompletionProResp represents the structure of a Minimax Chat Completion Pro response.
type minimaxChatCompletionProResp struct {
	Created         int64           `json:"created"`
	Model           string          `json:"model"`
	Reply           string          `json:"reply"`
	InputSensitive  bool            `json:"input_sensitive,omitempty"`
	OutputSensitive bool            `json:"output_sensitive,omitempty"`
	Choices         []minimaxChoice `json:"choices,omitempty"`
	Usage           usage           `json:"usage,omitempty"`
	Id              string          `json:"id"`
	BaseResp        minimaxBaseResp `json:"base_resp"`
}

// minimaxBaseResp contains error status code and details.
type minimaxBaseResp struct {
	StatusCode int64  `json:"status_code"`
	StatusMsg  string `json:"status_msg"`
}

// minimaxChoice represents a result option.
type minimaxChoice struct {
	Messages     []minimaxMessage `json:"messages"`
	Index        int64            `json:"index"`
	FinishReason string           `json:"finish_reason"`
}
