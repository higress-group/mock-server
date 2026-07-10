package chat

import (
	"net/http"

	"llm-mock-server/pkg/log"

	"github.com/gin-gonic/gin"
)

const (
	deeplHostFree       = "api-free.deepl.com"
	deeplHostPro        = "api.deepl.com"
	deeplTranslatePath  = "/v2/translate"
	deeplDetectedSource = "EN" // fixed detected source language the mock reports
)

// deeplRequest is the DeepL /v2/translate request shape. ai-proxy sends this (JSON) after
// converting the client's OpenAI-format request; the messages become the "text" array and
// target_lang comes from the provider's targetLang config.
type deeplRequest struct {
	Text       []string `json:"text"`
	TargetLang string   `json:"target_lang"`
	Context    string   `json:"context"`
}

type deeplProvider struct{}

func (p *deeplProvider) ShouldHandleRequest(ctx *gin.Context) bool {
	context, err := getRequestContext(ctx)
	if err != nil {
		log.Errorf("get request context failed: %v", err)
		return false
	}
	return (context.Host == deeplHostFree || context.Host == deeplHostPro) && context.Path == deeplTranslatePath
}

func (p *deeplProvider) HandleChatCompletions(ctx *gin.Context) {
	// The real DeepL API authenticates with "Authorization: DeepL-Auth-Key <key>"; ai-proxy injects it.
	if ctx.GetHeader("Authorization") == "" {
		ctx.JSON(http.StatusUnauthorized, gin.H{"message": "invalid api token"})
		return
	}

	var req deeplRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}

	// DeepL returns one translation per input text; echo each entry back.
	translations := make([]gin.H, 0, len(req.Text))
	for _, t := range req.Text {
		translations = append(translations, gin.H{"detected_source_language": deeplDetectedSource, "text": t})
	}

	// DeepL translation is non-streaming; ai-proxy converts this to an OpenAI chat.completion.
	ctx.JSON(http.StatusOK, gin.H{"translations": translations})
}
