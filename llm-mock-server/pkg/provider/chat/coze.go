package chat

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"llm-mock-server/pkg/log"
	"llm-mock-server/pkg/utils"

	"github.com/gin-gonic/gin"
)

const (
	cozeDomain = "api.coze.cn"
	cozePath   = "/v3/chat"

	cozeMockChatId         = "coze-mock-chat-id"
	cozeMockConversationId = "coze-mock-conv-id"
	cozeMockBotId          = "coze-mock-bot-id"
	cozeMockUserId         = "coze-mock-user-id"
	cozeMockMessageId      = "coze-mock-msg-id"
)

type cozeProvider struct{}

func (p *cozeProvider) ShouldHandleRequest(ctx *gin.Context) bool {
	context, err := getRequestContext(ctx)
	if err != nil {
		log.Errorf("get request context failed: %v", err)
		return false
	}
	return context.Host == cozeDomain && context.Path == cozePath
}

func (p *cozeProvider) HandleChatCompletions(ctx *gin.Context) {
	if !strings.HasPrefix(ctx.GetHeader("Authorization"), "Bearer ") {
		p.sendErrorResponse(ctx, http.StatusUnauthorized, "Unauthorized: Please provide a valid Bearer token")
		return
	}

	var req cozeChatRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		p.sendErrorResponse(ctx, http.StatusBadRequest, "Invalid request: "+err.Error())
		return
	}

	if req.BotId == "" {
		p.sendErrorResponse(ctx, http.StatusBadRequest, "bot_id is required")
		return
	}
	if req.UserId == "" {
		p.sendErrorResponse(ctx, http.StatusBadRequest, "user_id is required")
		return
	}

	content := p.extractLastUserContent(&req)

	if req.Stream {
		p.handleStreamResponse(ctx, &req, content)
	} else {
		p.handleNonStreamResponse(ctx, &req, content)
	}
}

func (p *cozeProvider) extractLastUserContent(req *cozeChatRequest) string {
	for i := len(req.AdditionalMessages) - 1; i >= 0; i-- {
		msg := req.AdditionalMessages[i]
		if msg.Role == "user" && msg.Content != "" {
			return msg.Content
		}
	}
	return ""
}

func (p *cozeProvider) sendErrorResponse(ctx *gin.Context, statusCode int, message string) {
	ctx.JSON(statusCode, gin.H{
		"code": statusCode,
		"msg":  message,
	})
}

func (p *cozeProvider) handleNonStreamResponse(ctx *gin.Context, req *cozeChatRequest, content string) {
	response := cozeChatResponse{
		Code: 0,
		Msg:  "Success",
		Data: cozeChatData{
			Id:             cozeMockChatId,
			ConversationId: cozeMockConversationId,
			BotId:          req.BotId,
			UserId:         req.UserId,
			CreatedAt:      completionMockCreated,
			CompletedAt:    completionMockCreated,
			Status:         "completed",
			Messages: []cozeMessage{
				{
					Id:             cozeMockMessageId,
					ConversationId: cozeMockConversationId,
					BotId:          req.BotId,
					Role:           "assistant",
					Content:        content,
					ContentType:    "text",
					Type:           "answer",
				},
			},
			Usage: cozeUsage{TokenCount: completionMockUsage.TotalTokens},
		},
	}
	ctx.JSON(http.StatusOK, response)
}

func (p *cozeProvider) handleStreamResponse(ctx *gin.Context, req *cozeChatRequest, content string) {
	utils.SetEventStreamHeaders(ctx)

	dataChan := make(chan string)
	stopChan := make(chan bool, 1)

	go func() {
		sendEvent := func(event string, data interface{}) bool {
			jsonStr, _ := json.Marshal(data)
			payload := "event:" + event + "\ndata:" + string(jsonStr) + "\n\n"
			select {
			case dataChan <- payload:
				return true
			case <-ctx.Request.Context().Done():
				return false
			}
		}

		if !sendEvent("conversation.chat.created", cozeStreamChatEvent{
			Id:             cozeMockChatId,
			ConversationId: cozeMockConversationId,
			BotId:          req.BotId,
			UserId:         req.UserId,
			CreatedAt:      completionMockCreated,
			Status:         "created",
		}) {
			return
		}

		if !sendEvent("conversation.chat.in_progress", cozeStreamChatEvent{
			Id:             cozeMockChatId,
			ConversationId: cozeMockConversationId,
			BotId:          req.BotId,
			UserId:         req.UserId,
			CreatedAt:      completionMockCreated,
			Status:         "in_progress",
		}) {
			return
		}

		for _, ch := range []rune(content) {
			if !sendEvent("conversation.message.delta", cozeStreamMessageEvent{
				Id:             cozeMockMessageId,
				ConversationId: cozeMockConversationId,
				BotId:          req.BotId,
				Role:           "assistant",
				Content:        string(ch),
				ContentType:    "text",
				Type:           "answer",
			}) {
				return
			}
			select {
			case <-ctx.Request.Context().Done():
				return
			case <-time.After(50 * time.Millisecond):
			}
		}

		if !sendEvent("conversation.message.completed", cozeStreamMessageEvent{
			Id:             cozeMockMessageId,
			ConversationId: cozeMockConversationId,
			BotId:          req.BotId,
			Role:           "assistant",
			Content:        content,
			ContentType:    "text",
			Type:           "answer",
		}) {
			return
		}

		if !sendEvent("conversation.chat.completed", cozeStreamChatEvent{
			Id:             cozeMockChatId,
			ConversationId: cozeMockConversationId,
			BotId:          req.BotId,
			UserId:         req.UserId,
			CreatedAt:      completionMockCreated,
			CompletedAt:    completionMockCreated,
			Status:         "completed",
			Usage:          cozeUsage{TokenCount: completionMockUsage.TotalTokens},
		}) {
			return
		}

		if !sendEvent("done", cozeDoneEvent{
			Code: 0,
			Msg:  "success",
		}) {
			return
		}

		stopChan <- true
	}()

	ctx.Stream(func(w io.Writer) bool {
		select {
		case data := <-dataChan:
			_, _ = w.Write([]byte(data))
			return true
		case <-stopChan:
			return false
		}
	})
}

// Coze API v3 data structures

type cozeChatRequest struct {
	BotId              string                 `json:"bot_id"`
	UserId             string                 `json:"user_id"`
	Stream             bool                   `json:"stream"`
	ConversationId     string                 `json:"conversation_id,omitempty"`
	AutoSaveHistory    bool                   `json:"auto_save_history,omitempty"`
	AdditionalMessages []cozeAdditionalMsg    `json:"additional_messages,omitempty"`
	CustomVariables    map[string]interface{} `json:"custom_variables,omitempty"`
}

type cozeAdditionalMsg struct {
	Role        string `json:"role"`
	Content     string `json:"content"`
	ContentType string `json:"content_type"`
}

type cozeChatResponse struct {
	Code int          `json:"code"`
	Msg  string       `json:"msg"`
	Data cozeChatData `json:"data"`
}

type cozeChatData struct {
	Id             string        `json:"id"`
	ConversationId string        `json:"conversation_id"`
	BotId          string        `json:"bot_id"`
	UserId         string        `json:"user_id"`
	CreatedAt      int64         `json:"created_at"`
	CompletedAt    int64         `json:"completed_at"`
	Status         string        `json:"status"`
	Messages       []cozeMessage `json:"messages"`
	Usage          cozeUsage     `json:"usage"`
}

type cozeMessage struct {
	Id             string `json:"id"`
	ConversationId string `json:"conversation_id"`
	BotId          string `json:"bot_id"`
	Role           string `json:"role"`
	Content        string `json:"content"`
	ContentType    string `json:"content_type"`
	Type           string `json:"type"`
}

type cozeUsage struct {
	TokenCount int `json:"token_count"`
}

type cozeStreamChatEvent struct {
	Id             string    `json:"id"`
	ConversationId string    `json:"conversation_id"`
	BotId          string    `json:"bot_id"`
	UserId         string    `json:"user_id"`
	CreatedAt      int64     `json:"created_at"`
	CompletedAt    int64     `json:"completed_at,omitempty"`
	Status         string    `json:"status"`
	Usage          cozeUsage `json:"usage,omitempty"`
}

type cozeStreamMessageEvent struct {
	Id             string `json:"id"`
	ConversationId string `json:"conversation_id"`
	BotId          string `json:"bot_id"`
	Role           string `json:"role"`
	Content        string `json:"content"`
	ContentType    string `json:"content_type"`
	Type           string `json:"type"`
}

type cozeDoneEvent struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
}
