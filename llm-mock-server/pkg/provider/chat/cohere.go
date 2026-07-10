package chat

import (
	"encoding/json"
	"net/http"
	"time"

	"llm-mock-server/pkg/log"

	"github.com/gin-gonic/gin"
)

const (
	cohereDomain   = "api.cohere.com"
	cohereChatPath = "/v1/chat"
)

// cohereRequest is the Cohere v1 /v1/chat request shape. ai-proxy sends this after converting
// the client's OpenAI-format request (it maps the first message to the top-level "message").
type cohereRequest struct {
	Message string `json:"message"`
	Stream  bool   `json:"stream"`
}

type cohereProvider struct{}

func (p *cohereProvider) ShouldHandleRequest(ctx *gin.Context) bool {
	context, err := getRequestContext(ctx)
	if err != nil {
		log.Errorf("get request context failed: %v", err)
		return false
	}
	return context.Host == cohereDomain && context.Path == cohereChatPath
}

func (p *cohereProvider) HandleChatCompletions(ctx *gin.Context) {
	// The real Cohere API requires "Authorization: Bearer <api key>"; ai-proxy always injects it.
	if ctx.GetHeader("Authorization") == "" {
		ctx.JSON(http.StatusUnauthorized, gin.H{"message": "invalid api token"})
		return
	}

	var req cohereRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}

	// This native Cohere response is what ai-proxy converts to OpenAI shape.
	if req.Stream {
		p.handleStreamResponse(ctx, req.Message)
	} else {
		p.handleNonStreamResponse(ctx, req.Message)
	}
}

// cohereMeta builds the Cohere v1 "meta" object, which carries api_version plus tokens and billed_units.
func cohereMeta() gin.H {
	counts := gin.H{
		"input_tokens":  completionMockUsage.PromptTokens,
		"output_tokens": completionMockUsage.CompletionTokens,
	}
	return gin.H{
		"api_version":  gin.H{"version": "1"},
		"tokens":       counts,
		"billed_units": counts,
	}
}

func (p *cohereProvider) handleNonStreamResponse(ctx *gin.Context, response string) {
	ctx.JSON(http.StatusOK, gin.H{
		"response_id":   completionMockId,
		"text":          response,
		"generation_id": completionMockId,
		"chat_history": []gin.H{
			{"role": "USER", "message": response},
			{"role": "CHATBOT", "message": response},
		},
		"finish_reason": "COMPLETE",
		"meta":          cohereMeta(),
	})
}

func (p *cohereProvider) handleStreamResponse(ctx *gin.Context, response string) {
	// ai-proxy requests streaming with Accept: text/event-stream, so Cohere replies with SSE frames
	// (event: <event_type> + data: <json>).
	ctx.Header("Content-Type", "text/event-stream")
	ctx.Status(http.StatusOK)

	send := func(payload gin.H) bool {
		data, _ := json.Marshal(payload)
		select {
		case <-ctx.Request.Context().Done():
			return false
		default:
		}
		eventType, _ := payload["event_type"].(string)
		ctx.Writer.Write([]byte("event: " + eventType + "\ndata: " + string(data) + "\n\n"))
		ctx.Writer.Flush()
		return true
	}

	if !send(gin.H{"event_type": "stream-start", "generation_id": completionMockId, "is_finished": false}) {
		return
	}
	for _, r := range response {
		if !send(gin.H{"event_type": "text-generation", "text": string(r), "is_finished": false}) {
			return
		}
		select {
		case <-ctx.Request.Context().Done():
			return
		case <-time.After(50 * time.Millisecond):
		}
	}
	send(gin.H{
		"event_type":    "stream-end",
		"is_finished":   true,
		"finish_reason": "COMPLETE",
		"response": gin.H{
			"response_id":   completionMockId,
			"text":          response,
			"generation_id": completionMockId,
			"finish_reason": "COMPLETE",
			"meta":          cohereMeta(),
		},
	})
}
