package chat

import (
	"encoding/json"
	"net/http"
	"time"

	"llm-mock-server/pkg/log"
	"llm-mock-server/pkg/utils"

	"github.com/gin-gonic/gin"
)

const (
	hunyuanDomain = "hunyuan.tencentcloudapi.com"
	hunyuanPath   = "/"
	// hunyuanNote is the disclaimer the real Hunyuan API always returns.
	hunyuanNote = "以上内容为AI生成，不代表开发者立场，请勿删除或修改本标记"
)

// hunyuanRequest is the native Tencent Hunyuan ChatCompletions request shape (capitalized keys).
// ai-proxy sends this after converting the client's OpenAI-format request. Only the text-chat
// subset is modeled; Tools / ToolChoice are not (ai-proxy's hunyuan request does not forward them).
type hunyuanRequest struct {
	Model    string `json:"Model"`
	Messages []struct {
		Role    string `json:"Role"`
		Content string `json:"Content"`
	} `json:"Messages"`
	Stream bool `json:"Stream"`
}

type hunyuanProvider struct{}

func (p *hunyuanProvider) ShouldHandleRequest(ctx *gin.Context) bool {
	context, err := getRequestContext(ctx)
	if err != nil {
		log.Errorf("get request context failed: %v", err)
		return false
	}
	return context.Host == hunyuanDomain && context.Path == hunyuanPath
}

func (p *hunyuanProvider) HandleChatCompletions(ctx *gin.Context) {
	// The real API rejects unsigned requests; ai-proxy always sets a TC3 Authorization header.
	if ctx.GetHeader("Authorization") == "" {
		ctx.JSON(http.StatusUnauthorized, gin.H{
			"Response": gin.H{"Error": gin.H{"Code": "AuthFailure", "Message": "missing Authorization"}},
		})
		return
	}
	// The native TC3 API is selected by the X-TC-Action / X-TC-Version headers; ai-proxy always
	// injects them (X-TC-Action: ChatCompletions, X-TC-Version: 2023-09-01).
	if ctx.GetHeader("X-TC-Action") != "ChatCompletions" || ctx.GetHeader("X-TC-Version") != "2023-09-01" {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"Response": gin.H{"Error": gin.H{"Code": "InvalidAction", "Message": "invalid X-TC-Action / X-TC-Version"}},
		})
		return
	}

	var req hunyuanRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"Response": gin.H{"Error": gin.H{"Message": err.Error()}}})
		return
	}
	response := lastHunyuanUserText(&req)

	// The real API returns the request id in the X-TC-RequestId response header.
	ctx.Header("X-TC-RequestId", completionMockId)
	if req.Stream {
		p.handleStreamResponse(ctx, response)
	} else {
		p.handleNonStreamResponse(ctx, response)
	}
}

func (p *hunyuanProvider) handleNonStreamResponse(ctx *gin.Context, response string) {
	ctx.JSON(http.StatusOK, gin.H{
		"Response": gin.H{
			"RequestId": completionMockId,
			"Note":      hunyuanNote,
			"Id":        completionMockId, // becomes the OpenAI response "id"
			"Created":   completionMockCreated,
			"Choices": []gin.H{{
				"Index":        0,
				"FinishReason": stopReason,
				"Message":      gin.H{"Role": roleAssistant, "Content": response},
			}},
			"Usage": gin.H{
				"PromptTokens":     completionMockUsage.PromptTokens,
				"CompletionTokens": completionMockUsage.CompletionTokens,
				"TotalTokens":      completionMockUsage.TotalTokens,
			},
		},
	})
}

func (p *hunyuanProvider) handleStreamResponse(ctx *gin.Context, response string) {
	utils.SetEventStreamHeaders(ctx)

	// Every frame MUST carry a non-empty Choices array: ai-proxy indexes Choices[0]
	// without a bounds check when converting Hunyuan chunks.
	send := func(content, finish string) bool {
		data, _ := json.Marshal(gin.H{
			"Note":    hunyuanNote,
			"Id":      completionMockId,
			"Created": time.Now().Unix(),
			"Choices": []gin.H{{
				"Index":        0,
				"Delta":        gin.H{"Role": roleAssistant, "Content": content},
				"FinishReason": finish,
			}},
			"Usage": gin.H{
				"PromptTokens":     completionMockUsage.PromptTokens,
				"CompletionTokens": completionMockUsage.CompletionTokens,
				"TotalTokens":      completionMockUsage.TotalTokens,
			},
		})
		select {
		case <-ctx.Request.Context().Done():
			return false
		default:
		}
		ctx.Render(-1, streamEvent{Data: "data: " + string(data)})
		ctx.Writer.Flush()
		return true
	}

	// One content delta per rune, then a terminal frame with finish_reason "stop" (native
	// Hunyuan emits no [DONE]).
	for _, r := range response {
		if !send(string(r), "") {
			return
		}
		select {
		case <-ctx.Request.Context().Done():
			return
		case <-time.After(50 * time.Millisecond):
		}
	}
	send("", stopReason)
}

func lastHunyuanUserText(req *hunyuanRequest) string {
	if len(req.Messages) == 0 {
		return ""
	}
	for i := len(req.Messages) - 1; i >= 0; i-- {
		if req.Messages[i].Role == "user" {
			return req.Messages[i].Content
		}
	}
	return req.Messages[len(req.Messages)-1].Content
}
