package chat

import (
	"github.com/gin-gonic/gin"
)

const (
	difyDomain             = "api.dify.ai"
	difyChatCompletionPath = "/v1/completion-messages"
)

type difyProvider struct {
}

func (p *difyProvider) ShouldHandleRequest(ctx *gin.Context) bool {
	context, _ := getRequestContext(ctx)
	if context.Host == difyDomain && context.Path == difyChatCompletionPath {
		return true
	}
	return false
}

// difyChatCompletionRequest represents the structure of a chat completion request.
type difyChatCompletionRequest struct {
	Model             string  `json:"model" validate:"required"`
	Stream            bool    `json:"stream,omitempty"`
	TokensToGenerate  int64   `json:"tokens_to_generate,omitempty"`
	Temperature       float64 `json:"temperature,omitempty"`
	TopP              float64 `json:"top_p,omitempty"`
	MaskSensitiveInfo bool    `json:"mask_sensitive_info"`
	// Messages          []minimaxMessage        `json:"messages" validate:"required,min=1"`
	// BotSettings       []minimaxBotSetting     `json:"bot_setting" validate:"required,min=1"`
	// ReplyConstraints  minimaxReplyConstraints `json:"reply_constraints" validate:"required"`
}
