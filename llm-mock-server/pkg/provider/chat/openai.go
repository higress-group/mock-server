package chat

import (
	"encoding/json"
	"io"
	"net/http"
	"time"

	"llm-mock-server/pkg/utils"

	"github.com/gin-gonic/gin"
)

type openAiProvider struct{}

func (p *openAiProvider) ShouldHandleRequest(ctx *gin.Context) bool {
	return true
}

func (p *openAiProvider) HandleChatCompletions(ctx *gin.Context) {
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

func (p *openAiProvider) handleStreamResponse(ctx *gin.Context, chatRequest chatCompletionRequest, response string) {
	utils.SetEventStreamHeaders(ctx)
	dataChan := make(chan string)
	stopChan := make(chan bool, 1)
	streamResponse := chatCompletionResponse{
		Id:      completionMockId,
		Object:  objectChatCompletionChunk,
		Created: completionMockCreated,
		Model:   chatRequest.Model,
	}
	go func() {
		responseRunes := []rune(response)
		for i, s := range responseRunes {
			choice := chatCompletionChoice{Delta: &chatMessage{Content: string(s)}}
			if i == len(responseRunes)-1 {
				choice.FinishReason = ptr(stopReason)
			}
			streamResponse.Choices = []chatCompletionChoice{choice}
			jsonStr, _ := json.Marshal(streamResponse)
			select {
			case dataChan <- string(jsonStr):
			case <-ctx.Request.Context().Done():
				// client gone; stop producing to avoid leaking this goroutine
				return
			}

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
			ctx.Render(-1, streamEvent{Data: "data: [DONE]"})
			return false
		}
	})
}

func (p *openAiProvider) handleNonStreamResponse(ctx *gin.Context, chatRequest chatCompletionRequest, response string) {
	completion := createChatCompletionResponse(chatRequest.Model, response)
	ctx.JSON(http.StatusOK, completion)
}

func createChatCompletionResponse(model, response string) chatCompletionResponse {
	return chatCompletionResponse{
		Id:      completionMockId,
		Object:  objectChatCompletion,
		Created: completionMockCreated,
		Model:   model,
		Choices: []chatCompletionChoice{
			{
				Index: 0,
				Message: &chatMessage{
					Role:    roleAssistant,
					Content: response,
				},
				FinishReason: ptr(stopReason),
			},
		},
		Usage: &completionMockUsage,
	}
}
