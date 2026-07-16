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
	sparkDomain             = "spark-api-open.xf-yun.com"
	sparkChatCompletionPath = "/v1/chat/completions"
)

// sparkResponse mirrors the native Spark (Xunfei) chat completion response shape
// that the ai-proxy spark provider parses (see provider/spark.go: sparkResponse
// and sparkStreamResponse). Spark wraps the OpenAI-style choices/usage with a
// top-level code/message/sid envelope; the streaming form additionally carries
// id and created on every chunk.
type sparkResponse struct {
	Code    int                    `json:"code"`
	Message string                 `json:"message,omitempty"`
	Sid     string                 `json:"sid"`
	Id      string                 `json:"id,omitempty"`
	Created int64                  `json:"created,omitempty"`
	Choices []chatCompletionChoice `json:"choices"`
	Usage   *usage                 `json:"usage,omitempty"`
}

type sparkProvider struct{}

func (p *sparkProvider) ShouldHandleRequest(ctx *gin.Context) bool {
	requestCtx, err := getRequestContext(ctx)
	if err != nil {
		log.Errorf("get request context failed: %v", err)
		return false
	}
	return requestCtx.Host == sparkDomain && requestCtx.Path == sparkChatCompletionPath
}

func (p *sparkProvider) HandleChatCompletions(ctx *gin.Context) {
	// The real Spark API requires "Authorization: Bearer <api key>"; the error
	// body mirrors the actual API response.
	if !strings.HasPrefix(ctx.GetHeader("Authorization"), "Bearer ") {
		ctx.JSON(http.StatusUnauthorized, sparkResponse{
			Code:    401,
			Message: "token is empty",
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
		p.handleNonStreamResponse(ctx, chatRequest, response)
	}
}

func (p *sparkProvider) handleNonStreamResponse(ctx *gin.Context, chatRequest chatCompletionRequest, response string) {
	resp := sparkResponse{
		Code: 0,
		Sid:  completionMockId,
		Choices: []chatCompletionChoice{
			{
				Index:        0,
				Message:      &chatMessage{Role: roleAssistant, Content: response},
				FinishReason: ptr(stopReason),
			},
		},
		Usage: &completionMockUsage,
	}
	ctx.JSON(http.StatusOK, resp)
}

func (p *sparkProvider) handleStreamResponse(ctx *gin.Context, chatRequest chatCompletionRequest, response string) {
	utils.SetEventStreamHeaders(ctx)
	dataChan := make(chan string)
	stopChan := make(chan bool, 1)
	// Usage is carried on every chunk because the spark provider parses
	// sparkStreamResponse.Usage as a value type and forwards &response.Usage on
	// each converted OpenAI chunk — emitting it once would leave the earlier
	// chunks with a zero usage block.
	streamResponse := sparkResponse{
		Code:    0,
		Sid:     completionMockId,
		Id:      completionMockId,
		Created: completionMockCreated,
		Usage:   &completionMockUsage,
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

		// The final chunk carries the finish reason (empty delta), matching the
		// end-block shape of the real Spark streaming API.
		if !sendChunk(chatCompletionChoice{
			Delta:        &chatMessage{},
			FinishReason: ptr(stopReason),
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
