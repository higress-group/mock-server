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

var (
	orderedEmbeddingsHandlers = []struct {
		name    string
		handler requestHandler
	}{
		{"openai", &openAiProvider{}},
	}
)

// HandleEmbeddings dispatches an embeddings request to the first registered
// handler that accepts it.
func HandleEmbeddings(context *gin.Context) {
	for _, entry := range orderedEmbeddingsHandlers {
		if entry.handler.ShouldHandleRequest(context) {
			entry.handler.HandleEmbeddings(context)
			return
		}
	}
	context.JSON(http.StatusNotFound, gin.H{"error": "Not found"})
}
