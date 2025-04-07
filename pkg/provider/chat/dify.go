package chat

import "github.com/gin-gonic/gin"

const (
	difyDomain             = "api.dify.cn"
	difyChatCompletionPath = "/v1/chat/chat-messages"
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
