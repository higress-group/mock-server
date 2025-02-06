package embeddings

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"llm-mock-server/pkg/provider"
)

type requestHandler interface {
	provider.CommonRequestHandler

	HandleEmbeddings(context *gin.Context)
}

var chatCompletionsHandlers []requestHandler

func HandleEmbeddings(context *gin.Context) {
	for _, handler := range chatCompletionsHandlers {
		if handler.ShouldHandleRequest(context) {
			handler.HandleEmbeddings(context)
			return
		}
	}
	context.JSON(http.StatusNotFound, gin.H{"error": "Not found"})
}
