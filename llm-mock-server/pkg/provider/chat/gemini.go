package chat

import (
	"encoding/json"
	"fmt"
	"llm-mock-server/pkg/log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	geminiDomain = "generativelanguage.googleapis.com"
	geminiPath   = "/v1beta/models/"
)

type geminiProvider struct{}

func (p *geminiProvider) ShouldHandleRequest(ctx *gin.Context) bool {
	context, _ := getRequestContext(ctx)
	path := context.Path

	// Gemini使用 /v1beta/models/{model}:generateContent 或 :streamGenerateContent 格式
	return context.Host == geminiDomain &&
		strings.HasPrefix(path, geminiPath) &&
		(strings.HasSuffix(path, ":generateContent") ||
			strings.HasSuffix(path, ":streamGenerateContent"))
}

func (p *geminiProvider) HandleChatCompletions(ctx *gin.Context) {
	// 验证Authorization header
	authHeader := ctx.GetHeader("x-goog-api-key")
	if authHeader == "" {
		p.sendErrorResponse(ctx, http.StatusUnauthorized, "Unauthorized: Please provide an API key")
		return
	}

	modelAndAction := strings.SplitN(ctx.Param("modelAndAction"), ":", 2)
	if len(modelAndAction) < 2 {
		p.sendErrorResponse(ctx, http.StatusBadRequest, "Invalid model and action")
		return
	}

	model := modelAndAction[0]
	action := modelAndAction[1]
	log.Infof("gemini request model: %s, action: %s", model, action)

	// 解析请求体
	var geminiRequest geminiGenerateContentRequest
	if err := ctx.ShouldBindJSON(&geminiRequest); err != nil {
		p.sendErrorResponse(ctx, http.StatusBadRequest, fmt.Sprintf("Invalid request: %v", err.Error()))
		return
	}

	// 验证请求体
	if err := p.validateRequest(&geminiRequest); err != nil {
		p.sendErrorResponse(ctx, http.StatusBadRequest, fmt.Sprintf("Validation error: %v", err.Error()))
		return
	}

	// 检查是否为流式请求
	path := ctx.Request.URL.Path
	isStreaming := strings.HasSuffix(path, ":streamGenerateContent")

	// 生成回复内容
	content := p.generateResponse(&geminiRequest)

	if isStreaming {
		p.handleStreamResponse(ctx, &geminiRequest, content)
	} else {
		p.handleNonStreamResponse(ctx, &geminiRequest, content)
	}
}

func (p *geminiProvider) validateRequest(req *geminiGenerateContentRequest) error {
	if len(req.Contents) == 0 {
		return fmt.Errorf("contents are required")
	}

	for i, content := range req.Contents {
		if len(content.Parts) == 0 {
			return fmt.Errorf("content %d: parts are required", i)
		}
		for j, part := range content.Parts {
			if part.Text == "" {
				return fmt.Errorf("content %d, part %d: text is required", i, j)
			}
		}
	}

	return nil
}

func (p *geminiProvider) generateResponse(req *geminiGenerateContentRequest) string {
	// 生成mock回复内容
	content := "This is a mock response from Gemini provider. "
	if len(req.Contents) > 0 && len(req.Contents[0].Parts) > 0 {
		text := req.Contents[0].Parts[0].Text
		if len(text) > 50 {
			content += "You said: " + text[:50] + "..."
		} else {
			content += "You said: " + text
		}
	}
	return content
}

func (p *geminiProvider) handleStreamResponse(ctx *gin.Context, req *geminiGenerateContentRequest, response string) {
	// 设置流式响应头
	ctx.Header("Content-Type", "text/event-stream")
	ctx.Header("Cache-Control", "no-cache")
	ctx.Header("Connection", "keep-alive")
	ctx.Header("Access-Control-Allow-Origin", "*")

	// 将回复内容分割成单词进行流式传输
	words := strings.Fields(response)
	totalWords := len(words)

	for i, word := range words {
		// 创建流式响应块
		chunk := geminiGenerateContentResponse{
			Candidates: []geminiCandidate{
				{
					Content: geminiContent{
						Parts: []geminiPart{
							{Text: word + " "},
						},
					},
					FinishReason: "STOP",
				},
			},
		}

		// 最后一个块设置结束原因
		if i == totalWords-1 {
			chunk.Candidates[0].FinishReason = "STOP"
		} else {
			chunk.Candidates[0].FinishReason = ""
		}

		// 发送数据块 - 使用与OpenAI provider相同的方式
		data, err := json.Marshal(chunk)
		if err != nil {
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to marshal response"})
			return
		}

		// 使用streamEvent来渲染SSE数据
		ctx.Render(-1, streamEvent{Data: "data: " + string(data)})

		// 模拟流式传输延迟
		time.Sleep(50 * time.Millisecond)
	}

	// 发送结束标记 - 使用与OpenAI provider相同的方式
	ctx.Render(-1, streamEvent{Data: "data: [DONE]"})
}

func (p *geminiProvider) handleNonStreamResponse(ctx *gin.Context, req *geminiGenerateContentRequest, response string) {
	// 创建非流式响应
	geminiResponse := geminiGenerateContentResponse{
		Candidates: []geminiCandidate{
			{
				Content: geminiContent{
					Parts: []geminiPart{
						{Text: response},
					},
				},
				FinishReason: "STOP",
			},
		},
		PromptFeedback: geminiPromptFeedback{
			BlockReason: "BLOCK_REASON_UNSPECIFIED",
		},
	}

	ctx.JSON(http.StatusOK, geminiResponse)
}

func (p *geminiProvider) sendErrorResponse(ctx *gin.Context, statusCode int, message string) {
	ctx.JSON(statusCode, gin.H{
		"error": gin.H{
			"code":    statusCode,
			"message": message,
		},
	})
}

// Gemini请求和响应的数据结构
type geminiGenerateContentRequest struct {
	Contents         []geminiContent         `json:"contents" validate:"required,min=1"`
	SafetySettings   []geminiSafetySetting   `json:"safety_settings,omitempty"`
	GenerationConfig *geminiGenerationConfig `json:"generation_config,omitempty"`
}

type geminiContent struct {
	Parts []geminiPart `json:"parts" validate:"required,min=1"`
	Role  string       `json:"role,omitempty"`
}

type geminiPart struct {
	Text string `json:"text" validate:"required"`
}

type geminiSafetySetting struct {
	Category  string `json:"category"`
	Threshold string `json:"threshold"`
}

type geminiGenerationConfig struct {
	Temperature     float64 `json:"temperature,omitempty"`
	TopP            float64 `json:"top_p,omitempty"`
	TopK            int     `json:"top_k,omitempty"`
	MaxOutputTokens int     `json:"max_output_tokens,omitempty"`
}

type geminiGenerateContentResponse struct {
	Candidates     []geminiCandidate    `json:"candidates"`
	PromptFeedback geminiPromptFeedback `json:"prompt_feedback,omitempty"`
}

type geminiCandidate struct {
	Content      geminiContent `json:"content"`
	FinishReason string        `json:"finish_reason"`
}

type geminiPromptFeedback struct {
	BlockReason string `json:"block_reason"`
}
