package chat

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"llm-mock-server/pkg/log"
	"llm-mock-server/pkg/provider"

	"github.com/gin-gonic/gin"
)

type requestHandler interface {
	provider.CommonRequestHandler

	HandleChatCompletions(context *gin.Context)
}

var (
	chatCompletionsHandlers = []requestHandler{
		&minimaxProvider{},
		&difyProvider{},
		&qwenProvider{},
		&openAiProvider{}, // As the last fallback
	}

	chatCompletionsRoutes = []string{
		// baidu
		"/v2/chat/completions",
		// doubao
		"/api/v3/chat/completions",
		// github
		"/chat/completions",
		// groq
		"/openai/v1/chat/completions",
		// minimax
		"/v1/text/chatcompletion_v2",
		"/v1/text/chatcompletion_pro",
		// openai
		"/v1/chat/completions",
		// qwen
		"/compatible-mode/v1/chat/completions",
		"/api/v1/services/aigc/text-generation/generation",
		// zhipu
		"/api/paas/v4/chat/completions",
		// dify
		"/v1/completion-messages",
		"/v1/chat-messages",
	}
)

func SetupRoutes(server *gin.Engine) {
	for _, route := range chatCompletionsRoutes {
		server.POST(route, handleChatCompletions)
	}
}

func handleChatCompletions(context *gin.Context) {
	if err := buildRequestContext(context); err != nil {
		return
	}
	for _, handler := range chatCompletionsHandlers {
		if handler.ShouldHandleRequest(context) {
			handler.HandleChatCompletions(context)
			return
		}
	}
	context.JSON(http.StatusNotFound, gin.H{"error": "Not found"})
}

type requestContext struct {
	Host  string
	Path  string
	Model string
}

func buildRequestContext(context *gin.Context) error {
	body, err := io.ReadAll(context.Request.Body)
	if err != nil {
		log.Errorf("Error reading request body:", err)
		context.JSON(http.StatusBadRequest, gin.H{"error": "Error reading request body"})
		return err
	}

	// Reset the request body so it can be read again by subsequent handlers
	context.Request.Body = io.NopCloser(strings.NewReader(string(body)))

	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		log.Errorf("Error unmarshalling JSON:", err)
		context.JSON(http.StatusBadRequest, gin.H{"error": "Error unmarshalling JSON"})
		return err
	}
	model, _ := data["model"].(string)

	context.Set("requestContext", requestContext{
		Host:  context.Request.Host,
		Path:  context.Request.URL.Path,
		Model: model})

	return nil
}

func getRequestContext(context *gin.Context) (requestContext, error) {
	requestCtx, exists := context.Get("requestContext")
	if !exists {
		return requestContext{}, fmt.Errorf("request context not found")
	}

	ctx, ok := requestCtx.(requestContext)
	if !ok {
		return requestContext{}, fmt.Errorf("invalid request context type")
	}

	return ctx, nil
}
