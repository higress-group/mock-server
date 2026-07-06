package chat

import (
	"net/http"

	"llm-mock-server/pkg/utils"

	"github.com/gin-gonic/gin"
)

func prompt2Response(prompt string) string {
	return prompt
}

func ptr[T any](v T) *T {
	return &v
}

// bindAndValidateChatRequest binds and validates the request body, writing a 400 response on failure.
func bindAndValidateChatRequest(ctx *gin.Context, chatRequest *chatCompletionRequest) bool {
	if err := ctx.ShouldBindJSON(chatRequest); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return false
	}
	if err := utils.Validate.Struct(chatRequest); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return false
	}
	return true
}

func lastStringPrompt(chatRequest *chatCompletionRequest) string {
	last := chatRequest.Messages[len(chatRequest.Messages)-1]
	if last.IsStringContent() {
		return last.StringContent()
	}
	return ""
}
