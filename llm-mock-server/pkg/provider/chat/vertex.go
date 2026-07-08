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
	// vertexDomain is the host that ai-proxy rewrites Vertex AI requests to.
	// Express Mode targets the global endpoint without a region prefix.
	vertexDomain = "aiplatform.googleapis.com"
	// vertexPathFragment is the path fragment common to both the Express Mode
	// path (/v1/publishers/google/models/{model}:{action}) and the standard
	// path (/v1/projects/{project}/locations/{location}/publishers/google/models/{model}:{action}).
	vertexPathFragment = "/publishers/google/models/"

	vertexActionGenerateContent = "generateContent"
	vertexActionStreamGenerate  = "streamGenerateContent"
)

type vertexProvider struct{}

func (p *vertexProvider) ShouldHandleRequest(ctx *gin.Context) bool {
	requestCtx, err := getRequestContext(ctx)
	if err != nil {
		log.Errorf("get request context failed: %v", err)
		return false
	}

	// ai-proxy transforms OpenAI chat completion requests into Vertex's native
	// generateContent / streamGenerateContent actions before they reach the mock.
	if requestCtx.Host != vertexDomain {
		return false
	}
	path := requestCtx.Path
	if !strings.Contains(path, vertexPathFragment) {
		return false
	}
	return strings.HasSuffix(path, ":"+vertexActionGenerateContent) ||
		strings.HasSuffix(path, ":"+vertexActionStreamGenerate)
}

func (p *vertexProvider) HandleChatCompletions(ctx *gin.Context) {
	// Auth is intentionally not enforced. The mock's purpose is to simulate the
	// Vertex generateContent protocol, not to validate credentials: ai-proxy's
	// Express Mode chat path does not attach the API key on the transformed path,
	// and standard mode carries an OAuth bearer that the mock has no need to check.
	model, action, ok := parseVertexModelAndAction(ctx.Request.URL.Path)
	if !ok {
		p.sendErrorResponse(ctx, http.StatusBadRequest, "Invalid model and action")
		return
	}
	log.Infof("vertex request model: %s, action: %s", model, action)

	var vertexRequest vertexGenerateContentRequest
	if err := ctx.ShouldBindJSON(&vertexRequest); err != nil {
		p.sendErrorResponse(ctx, http.StatusBadRequest, fmt.Sprintf("Invalid request: %v", err.Error()))
		return
	}

	if err := p.validateRequest(&vertexRequest); err != nil {
		p.sendErrorResponse(ctx, http.StatusBadRequest, fmt.Sprintf("Validation error: %v", err.Error()))
		return
	}

	isStreaming := action == vertexActionStreamGenerate
	content := p.generateResponse(&vertexRequest)

	if isStreaming {
		p.handleStreamResponse(ctx, content)
	} else {
		p.handleNonStreamResponse(ctx, content)
	}
}

// parseVertexModelAndAction extracts the model and action from a path of the form
// .../publishers/google/models/{model}:{action}
func parseVertexModelAndAction(path string) (string, string, bool) {
	idx := strings.LastIndex(path, vertexPathFragment)
	if idx < 0 {
		return "", "", false
	}
	rest := path[idx+len(vertexPathFragment):]
	parts := strings.SplitN(rest, ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	// ctx.Request.URL.Path excludes the query string, so the action here is the
	// bare generateContent / streamGenerateContent even for streaming requests.
	return parts[0], parts[1], true
}

func (p *vertexProvider) validateRequest(req *vertexGenerateContentRequest) error {
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

func (p *vertexProvider) generateResponse(req *vertexGenerateContentRequest) string {
	// Generate the mock reply content, mirroring the Gemini provider so the
	// response is identifiable as having been served by the Vertex simulation.
	content := "This is a mock response from Vertex provider. "
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

func (p *vertexProvider) handleStreamResponse(ctx *gin.Context, response string) {
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

		chunk := vertexGenerateContentResponse{
			Candidates: []vertexCandidate{
				{
					Content: vertexContent{
						Parts: []vertexPart{
							{Text: word + " "},
						},
						Role: "model",
					},
					Index: 0,
				},
			},
			UsageMetadata: &vertexUsageMetadata{
				PromptTokenCount:     completionMockUsage.PromptTokens,
				CandidatesTokenCount: completionMockUsage.CompletionTokens,
				TotalTokenCount:      completionMockUsage.TotalTokens,
			},
		}

		// The final chunk carries the finish reason
		if i == totalWords-1 {
			chunk.Candidates[0].FinishReason = "STOP"
		}

		// Send the data chunk and flush immediately so each event arrives on its own
		data, err := json.Marshal(chunk)
		if err != nil {
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to marshal response"})
			return
		}

		ctx.Render(-1, streamEvent{Data: "data: " + string(data)})
		ctx.Writer.Flush()

		select {
		case <-ctx.Request.Context().Done():
			return
		case <-time.After(50 * time.Millisecond):
		}
	}
}

func (p *vertexProvider) handleNonStreamResponse(ctx *gin.Context, response string) {
	// Build the non-streaming response. The schema matches what ai-proxy's vertex
	// provider parses (responseId / candidates[].content.parts[].text /
	// finishReason / usageMetadata), so it round-trips into an OpenAI response.
	vertexResponse := vertexGenerateContentResponse{
		ResponseId: completionMockId,
		Candidates: []vertexCandidate{
			{
				Content: vertexContent{
					Parts: []vertexPart{
						{Text: response},
					},
					Role: "model",
				},
				FinishReason: "STOP",
				Index:        0,
			},
		},
		UsageMetadata: &vertexUsageMetadata{
			PromptTokenCount:     completionMockUsage.PromptTokens,
			CandidatesTokenCount: completionMockUsage.CompletionTokens,
			TotalTokenCount:      completionMockUsage.TotalTokens,
		},
	}

	ctx.JSON(http.StatusOK, vertexResponse)
}

func (p *vertexProvider) sendErrorResponse(ctx *gin.Context, statusCode int, message string) {
	ctx.JSON(statusCode, gin.H{
		"error": gin.H{
			"code":    statusCode,
			"message": message,
			"status":  vertexErrorStatus(statusCode),
		},
	})
}

// vertexErrorStatus returns the google.rpc.Code name that the real Vertex API
// pairs with the given HTTP status code.
func vertexErrorStatus(statusCode int) string {
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

// Data structures of Vertex generateContent requests and responses.
// The request schema matches Google's generateContent API (contents/parts),
// which is the body ai-proxy produces after transforming an OpenAI request.
type vertexGenerateContentRequest struct {
	Contents         []vertexContent         `json:"contents"`
	SafetySettings   []vertexSafetySetting   `json:"safetySettings,omitempty"`
	GenerationConfig *vertexGenerationConfig `json:"generationConfig,omitempty"`
}

type vertexContent struct {
	Parts []vertexPart `json:"parts"`
	Role  string       `json:"role,omitempty"`
}

type vertexPart struct {
	Text string `json:"text"`
}

type vertexSafetySetting struct {
	Category  string `json:"category"`
	Threshold string `json:"threshold"`
}

type vertexGenerationConfig struct {
	Temperature     float64 `json:"temperature,omitempty"`
	TopP            float64 `json:"topP,omitempty"`
	TopK            int     `json:"topK,omitempty"`
	MaxOutputTokens int     `json:"maxOutputTokens,omitempty"`
}

type vertexGenerateContentResponse struct {
	ResponseId    string               `json:"responseId,omitempty"`
	Candidates    []vertexCandidate    `json:"candidates"`
	UsageMetadata *vertexUsageMetadata `json:"usageMetadata,omitempty"`
}

type vertexCandidate struct {
	Content      vertexContent `json:"content"`
	FinishReason string        `json:"finishReason,omitempty"`
	Index        int           `json:"index"`
}

type vertexUsageMetadata struct {
	PromptTokenCount     int `json:"promptTokenCount"`
	CandidatesTokenCount int `json:"candidatesTokenCount"`
	TotalTokenCount      int `json:"totalTokenCount"`
}
