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
	context, err := getRequestContext(ctx)
	if err != nil {
		log.Errorf("get request context failed: %v", err)
		return false
	}
	path := context.Path

	// Gemini uses the /v1beta/models/{model}:generateContent or :streamGenerateContent format
	return context.Host == geminiDomain &&
		strings.HasPrefix(path, geminiPath) &&
		(strings.HasSuffix(path, ":generateContent") ||
			strings.HasSuffix(path, ":streamGenerateContent"))
}

func (p *geminiProvider) HandleChatCompletions(ctx *gin.Context) {
	apiKey := ctx.GetHeader("x-goog-api-key")
	if apiKey == "" {
		apiKey = ctx.Query("key")
	}
	if apiKey == "" {
		p.sendErrorResponse(ctx, http.StatusForbidden, "Method doesn't allow unregistered callers (callers without established identity). Please use API Key or other form of API consumer identity to call this API.")
		return
	}

	model, action, ok := parseGeminiModelAndAction(ctx.Request.URL.Path)
	if !ok {
		p.sendErrorResponse(ctx, http.StatusBadRequest, "Invalid model and action")
		return
	}
	log.Infof("gemini request model: %s, action: %s", model, action)

	// Parse request body
	var geminiRequest geminiGenerateContentRequest
	if err := ctx.ShouldBindJSON(&geminiRequest); err != nil {
		p.sendErrorResponse(ctx, http.StatusBadRequest, fmt.Sprintf("Invalid request: %v", err.Error()))
		return
	}

	// Validate request body
	if err := p.validateRequest(&geminiRequest); err != nil {
		p.sendErrorResponse(ctx, http.StatusBadRequest, fmt.Sprintf("Validation error: %v", err.Error()))
		return
	}

	// Check whether this is a streaming request
	isStreaming := action == "streamGenerateContent"

	// Generate the reply content
	content := p.generateResponse(&geminiRequest)

	if isStreaming {
		p.handleStreamResponse(ctx, &geminiRequest, content)
	} else {
		p.handleNonStreamResponse(ctx, &geminiRequest, content)
	}
}

// parseGeminiModelAndAction extracts the model and action from a path of the form /v1beta/models/{model}:{action}
func parseGeminiModelAndAction(path string) (string, string, bool) {
	if !strings.HasPrefix(path, geminiPath) {
		return "", "", false
	}
	parts := strings.SplitN(strings.TrimPrefix(path, geminiPath), ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
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
	// Generate the mock reply content
	content := "This is a mock response from Gemini provider. "
	if len(req.Contents) > 0 {
		lastContent := req.Contents[len(req.Contents)-1]
		if len(lastContent.Parts) > 0 {
			runes := []rune(lastContent.Parts[0].Text)
			if len(runes) > 50 {
				content += "You said: " + string(runes[:50]) + "..."
			} else {
				content += "You said: " + string(runes)
			}
		}
	}
	return content
}

func (p *geminiProvider) handleStreamResponse(ctx *gin.Context, req *geminiGenerateContentRequest, response string) {
	// Set streaming response headers
	ctx.Header("Content-Type", "text/event-stream")
	ctx.Header("Cache-Control", "no-cache")
	ctx.Header("Connection", "keep-alive")
	ctx.Header("Access-Control-Allow-Origin", "*")

	// Split the reply into words and stream them
	words := strings.Fields(response)
	totalWords := len(words)

	for i, word := range words {
		select {
		case <-ctx.Request.Context().Done():
			return
		default:
		}

		chunk := geminiGenerateContentResponse{
			Candidates: []geminiCandidate{
				{
					Content: geminiContent{
						Parts: []geminiPart{
							{Text: word + " "},
						},
						Role: "model",
					},
					Index: 0,
				},
			},
			UsageMetadata: &geminiUsageMetadata{
				PromptTokenCount:     completionMockUsage.PromptTokens,
				CandidatesTokenCount: completionMockUsage.CompletionTokens,
				TotalTokenCount:      completionMockUsage.TotalTokens,
			},
		}

		// The final chunk carries the finish reason
		if i == totalWords-1 {
			chunk.Candidates[0].FinishReason = "STOP"
		}

		// Send the data chunk, in the same way as the OpenAI provider
		data, err := json.Marshal(chunk)
		if err != nil {
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to marshal response"})
			return
		}

		// Render SSE data via streamEvent and flush immediately so each event arrives on its own
		ctx.Render(-1, streamEvent{Data: "data: " + string(data)})
		ctx.Writer.Flush()

		select {
		case <-ctx.Request.Context().Done():
			return
		case <-time.After(50 * time.Millisecond):
		}
	}
}

func (p *geminiProvider) handleNonStreamResponse(ctx *gin.Context, req *geminiGenerateContentRequest, response string) {
	// Build the non-streaming response
	geminiResponse := geminiGenerateContentResponse{
		Candidates: []geminiCandidate{
			{
				Content: geminiContent{
					Parts: []geminiPart{
						{Text: response},
					},
					Role: "model",
				},
				FinishReason: "STOP",
				Index:        0,
			},
		},
		UsageMetadata: &geminiUsageMetadata{
			PromptTokenCount:     completionMockUsage.PromptTokens,
			CandidatesTokenCount: completionMockUsage.CompletionTokens,
			TotalTokenCount:      completionMockUsage.TotalTokens,
		},
	}

	ctx.JSON(http.StatusOK, geminiResponse)
}

func (p *geminiProvider) sendErrorResponse(ctx *gin.Context, statusCode int, message string) {
	ctx.JSON(statusCode, gin.H{
		"error": gin.H{
			"code":    statusCode,
			"message": message,
			"status":  geminiErrorStatus(statusCode),
		},
	})
}

// geminiErrorStatus returns the google.rpc.Code name that the real Gemini API pairs with the given HTTP status code
func geminiErrorStatus(statusCode int) string {
	switch statusCode {
	case http.StatusBadRequest:
		return "INVALID_ARGUMENT"
	case http.StatusUnauthorized:
		return "UNAUTHENTICATED"
	case http.StatusForbidden:
		return "PERMISSION_DENIED"
	case http.StatusNotFound:
		return "NOT_FOUND"
	case http.StatusTooManyRequests:
		return "RESOURCE_EXHAUSTED"
	default:
		return "INTERNAL"
	}
}

// Data structures of Gemini requests and responses
type geminiGenerateContentRequest struct {
	Contents         []geminiContent         `json:"contents"`
	SafetySettings   []geminiSafetySetting   `json:"safetySettings,omitempty"`
	GenerationConfig *geminiGenerationConfig `json:"generationConfig,omitempty"`
}

type geminiContent struct {
	Parts []geminiPart `json:"parts"`
	Role  string       `json:"role,omitempty"`
}

type geminiPart struct {
	Text string `json:"text"`
}

type geminiSafetySetting struct {
	Category  string `json:"category"`
	Threshold string `json:"threshold"`
}

type geminiGenerationConfig struct {
	Temperature     float64 `json:"temperature,omitempty"`
	TopP            float64 `json:"topP,omitempty"`
	TopK            int     `json:"topK,omitempty"`
	MaxOutputTokens int     `json:"maxOutputTokens,omitempty"`
}

type geminiGenerateContentResponse struct {
	Candidates    []geminiCandidate    `json:"candidates"`
	UsageMetadata *geminiUsageMetadata `json:"usageMetadata,omitempty"`
}

type geminiCandidate struct {
	Content      geminiContent `json:"content"`
	FinishReason string        `json:"finishReason,omitempty"`
	Index        int           `json:"index"`
}

type geminiUsageMetadata struct {
	PromptTokenCount     int `json:"promptTokenCount"`
	CandidatesTokenCount int `json:"candidatesTokenCount"`
	TotalTokenCount      int `json:"totalTokenCount"`
}
