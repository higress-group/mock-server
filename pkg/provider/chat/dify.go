package chat

import (
	"crypto/rand"
	"encoding/hex"
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
	difyDomain         = "api.dify.ai"
	difyCompletionPath = "/v1/completion-messages"
	BotTypeCompletion  = "Completion"
)

type difyProvider struct {
}

func (p *difyProvider) ShouldHandleRequest(ctx *gin.Context) bool {
	context, _ := getRequestContext(ctx)
	return context.Host == difyDomain && context.Path == difyCompletionPath
}

func (p *difyProvider) HandleChatCompletions(ctx *gin.Context) {
	// Validate Authorization header
	authHeader := ctx.GetHeader("Authorization")
	if authHeader == "" {
		p.sendErrorResponse(ctx, 401, "Unauthorized: Please provide an API key")
		return
	}

	// Bind request body
	var chatRequest difyChatRequest
	if err := ctx.ShouldBindJSON(&chatRequest); err != nil {
		p.sendErrorResponse(ctx, 400, fmt.Sprintf("Invalid request: %v", err.Error()))
		return
	}

	// Validate request body
	if err := utils.Validate.Struct(chatRequest); err != nil {
		validationErrors := err.(validator.ValidationErrors)
		for _, fieldError := range validationErrors {
			p.sendErrorResponse(ctx, 400, fmt.Sprintf("Invalid request: %v", fieldError.Error()))
			return
		}
	}

	// Generate reply based on the query
	reply := prompt2Response(chatRequest.Query)

	// Handle stream or non-stream response based on the request
	if chatRequest.ResponseMode == "streaming" {
		p.handleStreamResponse(ctx, chatRequest, BotTypeCompletion, reply)
	} else {
		p.handleNonStreamResponse(ctx, chatRequest, BotTypeCompletion, reply)
	}
}

func (p *difyProvider) sendErrorResponse(ctx *gin.Context, statusCode int, message string) {
	ctx.JSON(statusCode, gin.H{
		"error": gin.H{
			"message": message,
			"type":    "invalid_request_error",
			"code":    "invalid_request",
		},
	})
}

func generateUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// TODO Mode
func (p *difyProvider) handleStreamResponse(ctx *gin.Context, chatRequest difyChatRequest, botType string, reply string) {
	utils.SetEventStreamHeaders(ctx)
	dataChan := make(chan string)
	stopChan := make(chan bool, 1)

	go func() {
		for _, s := range reply {
			response := difyChunkChatResponse{
				Event:          "agent_thought",
				Answer:         string(s),
				ConversationId: chatRequest.ConversationId,
				MessageId:      generateUUID(),
			}
			jsonStr, _ := json.Marshal(response)
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
			// Send final message with metadata
			finalResponse := difyChunkChatResponse{
				Event:          "message_end",
				Answer:         reply,
				ConversationId: chatRequest.ConversationId,
				MessageId:      generateUUID(),
				MetaData: difyMetaData{
					Usage: usage{
						PromptTokens:     len(chatRequest.Query),
						CompletionTokens: len(reply),
						TotalTokens:      len(chatRequest.Query) + len(reply),
					},
				},
			}
			jsonStr, _ := json.Marshal(finalResponse)
			ctx.Render(-1, streamEvent{Data: fmt.Sprintf("data: %s", jsonStr)})
			return false
		}
	})
}

func (p *difyProvider) handleNonStreamResponse(ctx *gin.Context, chatRequest difyChatRequest, botType string, reply string) {
	response := difyChatResponse{
		Answer:         reply,
		ConversationId: chatRequest.ConversationId,
		MessageId:      generateUUID(),
		CreateAt:       time.Now().Unix(),
		MetaData: difyMetaData{
			Usage: usage{
				PromptTokens:     len(chatRequest.Query),
				CompletionTokens: len(reply),
				TotalTokens:      len(chatRequest.Query) + len(reply),
			},
		},
	}
	ctx.JSON(http.StatusOK, response)
}

type difyChatRequest struct {
	Inputs           map[string]interface{} `json:"inputs"`
	Query            string                 `json:"query"`
	ResponseMode     string                 `json:"response_mode"`
	User             string                 `json:"user"`
	AutoGenerateName bool                   `json:"auto_generate_name"`
	ConversationId   string                 `json:"conversation_id"`
}

type difyMetaData struct {
	Usage usage `json:"usage"`
}

type difyData struct {
	WorkflowId string                 `json:"workflow_id"`
	Id         string                 `json:"id"`
	Outputs    map[string]interface{} `json:"outputs"`
}

type difyChatResponse struct {
	ConversationId string       `json:"conversation_id"`
	MessageId      string       `json:"message_id"`
	Answer         string       `json:"answer"`
	CreateAt       int64        `json:"create_at"`
	Data           difyData     `json:"data"`
	MetaData       difyMetaData `json:"metadata"`
}

type difyChunkChatResponse struct {
	Event          string       `json:"event"`
	ConversationId string       `json:"conversation_id"`
	MessageId      string       `json:"message_id"`
	Answer         string       `json:"answer"`
	Data           difyData     `json:"data"`
	MetaData       difyMetaData `json:"metadata"`
}
