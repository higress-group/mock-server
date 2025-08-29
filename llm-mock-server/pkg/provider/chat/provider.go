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
	chatCompletionsHandlers = map[string]requestHandler{
		"minimax": &minimaxProvider{},
		"dify":    &difyProvider{},
		"qwen":    &qwenProvider{},
		"gemini":  &geminiProvider{},
		"openai":  &openAiProvider{}, // As the last fallback
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
		// gemini
		"/v1beta/models/:modelAndAction",
	}
)

// SetupRoutes 支持按provider类型配置不同的路由
func SetupRoutes(server *gin.Engine, providerType string) {
	// 根据provider类型配置对应的路由
	switch strings.ToLower(providerType) {
	case "minimax":
		server.POST("/v1/text/chatcompletion_v2", chatCompletionsHandlers["openai"].HandleChatCompletions)
		server.POST("/v1/text/chatcompletion_pro", chatCompletionsHandlers["minimax"].HandleChatCompletions)
	case "dify":
		server.POST("/v1/completion-messages", chatCompletionsHandlers["dify"].HandleChatCompletions)
		server.POST("/v1/chat-messages", chatCompletionsHandlers["dify"].HandleChatCompletions)
	case "qwen":
		server.POST("/compatible-mode/v1/chat/completions", chatCompletionsHandlers["openai"].HandleChatCompletions)
		server.POST("/api/v1/services/aigc/text-generation/generation", chatCompletionsHandlers["qwen"].HandleChatCompletions)
	case "gemini":
		server.POST("/v1beta/models/:modelAndAction", chatCompletionsHandlers["gemini"].HandleChatCompletions)
	case "doubao":
		server.POST("/api/v3/chat/completions", chatCompletionsHandlers["openai"].HandleChatCompletions)
	case "baidu":
		server.POST("/v2/chat/completions", chatCompletionsHandlers["openai"].HandleChatCompletions)
	case "zhipu":
		server.POST("/api/paas/v4/chat/completions", chatCompletionsHandlers["openai"].HandleChatCompletions)
	case "github":
		server.POST("/chat/completions", chatCompletionsHandlers["openai"].HandleChatCompletions)
	case "groq":
		server.POST("/openai/v1/chat/completions", chatCompletionsHandlers["openai"].HandleChatCompletions)
	case "openai", "ai360", "deepseek", "together", "baichuan", "yi", "stepfun":
		// 这些provider都使用OpenAI兼容的格式，调用openAiProvider
		server.POST("/v1/chat/completions", chatCompletionsHandlers["openai"].HandleChatCompletions)
	default:
		// 未知的provider类型，启用所有路由
		for _, route := range chatCompletionsRoutes {
			server.POST(route, handleChatCompletions)
		}
		log.Warnf("Unknown provider type: %s, enabled all routes", providerType)
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
