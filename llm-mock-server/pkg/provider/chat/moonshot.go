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
	moonshotDomain             = "api.moonshot.cn"
	moonshotChatCompletionPath = "/v1/chat/completions"
)

// moonshot stream chunks carry no top-level usage; the final chunk nests it in choices[0].usage.
type moonshotStreamResponse struct {
	Id      string                 `json:"id,omitempty"`
	Choices []chatCompletionChoice `json:"choices"`
	Created int64                  `json:"created,omitempty"`
	Model   string                 `json:"model,omitempty"`
	Object  string                 `json:"object,omitempty"`
}

type moonshotProvider struct{}

func (p *moonshotProvider) ShouldHandleRequest(ctx *gin.Context) bool {
	context, err := getRequestContext(ctx)
	if err != nil {
		log.Errorf("get request context failed: %v", err)
		return false
	}
	return context.Host == moonshotDomain && context.Path == moonshotChatCompletionPath
}

func (p *moonshotProvider) HandleChatCompletions(ctx *gin.Context) {
	// The real moonshot API requires "Authorization: Bearer <api key>"; the error body mirrors the actual API response.
	if !strings.HasPrefix(ctx.GetHeader("Authorization"), "Bearer ") {
		ctx.JSON(http.StatusUnauthorized, gin.H{
			"error": gin.H{
				"message": "Incorrect API key provided",
				"type":    "incorrect_api_key_error",
			},
		})
		return
	}

	var chatRequest chatCompletionRequest
	if !bindAndValidateChatRequest(ctx, &chatRequest) {
		return
	}
	response := prompt2Response(lastStringPrompt(&chatRequest))

	if chatRequest.Stream {
		p.handleStreamResponse(ctx, chatRequest, response)
	} else {
		ctx.JSON(http.StatusOK, createChatCompletionResponse(chatRequest.Model, response))
	}
}

func (p *moonshotProvider) handleStreamResponse(ctx *gin.Context, chatRequest chatCompletionRequest, response string) {
	utils.SetEventStreamHeaders(ctx)
	dataChan := make(chan string)
	stopChan := make(chan bool, 1)
	streamResponse := moonshotStreamResponse{
		Id:      completionMockId,
		Object:  objectChatCompletionChunk,
		Created: completionMockCreated,
		Model:   chatRequest.Model,
	}
	go func() {
		sendChunk := func(choice chatCompletionChoice) bool {
			streamResponse.Choices = []chatCompletionChoice{choice}
			jsonStr, _ := json.Marshal(streamResponse)
			select {
			case dataChan <- string(jsonStr):
				return true
			case <-ctx.Request.Context().Done():
				// client gone; stop producing to avoid leaking this goroutine
				return false
			}
		}

		// The first chunk carries the role and an empty content, matching the streaming example in the official docs.
		if !sendChunk(chatCompletionChoice{Delta: &chatMessage{Role: roleAssistant, Content: ""}}) {
			return
		}

		for _, s := range []rune(response) {
			if !sendChunk(chatCompletionChoice{Delta: &chatMessage{Content: string(s)}}) {
				return
			}
			// Simulate response delay; cancel promptly if the client disconnects
			select {
			case <-ctx.Request.Context().Done():
				return
			case <-time.After(50 * time.Millisecond):
			}
		}

		// Moonshot-specific: the stream ends with an empty-delta block whose usage is nested in choices[0].usage (the end data block shape of the real API).
		if !sendChunk(chatCompletionChoice{
			Delta:        &chatMessage{},
			FinishReason: ptr(stopReason),
			Usage:        &completionMockUsage,
		}) {
			return
		}
		stopChan <- true
	}()

	ctx.Stream(func(w io.Writer) bool {
		select {
		case data := <-dataChan:
			ctx.Render(-1, streamEvent{Data: "data: " + data})
			return true
		case <-stopChan:
			ctx.Render(-1, streamEvent{Data: "data: [DONE]"})
			return false
		}
	})
}
