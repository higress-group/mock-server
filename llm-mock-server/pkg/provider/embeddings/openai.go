package embeddings

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"llm-mock-server/pkg/log"
)

// openAiProvider simulates OpenAI-compatible embeddings endpoints, which are
// also used by baidu (v2 API, /v2/embeddings) and by OpenAI-compatible
// providers such as hunyuan (/v1/embeddings).
type openAiProvider struct{}

type embeddingsRequest struct {
	Model string      `json:"model"`
	Input interface{} `json:"input"`
}

type embeddingsResponse struct {
	Object string         `json:"object"`
	Data   []embeddingObj `json:"data"`
	Model  string         `json:"model"`
	Usage  embeddingsUsage `json:"usage"`
}

type embeddingObj struct {
	Object    string    `json:"object"`
	Embedding []float64 `json:"embedding"`
	Index     int       `json:"index"`
}

type embeddingsUsage struct {
	PromptTokens int `json:"prompt_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

// mockEmbedding is a fixed vector so conformance assertions can compare the
// full response body deterministically.
var mockEmbedding = []float64{0.1, 0.2, 0.3, 0.4}

func (p *openAiProvider) ShouldHandleRequest(ctx *gin.Context) bool {
	return true
}

func (p *openAiProvider) HandleEmbeddings(ctx *gin.Context) {
	var request embeddingsRequest
	if err := ctx.ShouldBindJSON(&request); err != nil {
		log.Errorf("failed to bind embeddings request: %v", err)
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	inputs := normalizeInputs(request.Input)
	if len(inputs) == 0 {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "input must not be empty"})
		return
	}

	response := embeddingsResponse{
		Object: "list",
		Data:   make([]embeddingObj, 0, len(inputs)),
		Model:  request.Model,
		Usage: embeddingsUsage{
			PromptTokens: promptTokens(inputs),
			TotalTokens:  promptTokens(inputs),
		},
	}
	for i := range inputs {
		response.Data = append(response.Data, embeddingObj{
			Object:    "embedding",
			Embedding: mockEmbedding,
			Index:     i,
		})
	}

	ctx.JSON(http.StatusOK, response)
}

// normalizeInputs flattens the OpenAI embeddings input, which may be a
// string, an array of strings, or an array of token arrays, into strings.
func normalizeInputs(input interface{}) []string {
	switch v := input.(type) {
	case string:
		return []string{v}
	case []interface{}:
		inputs := make([]string, 0, len(v))
		for _, item := range v {
			switch s := item.(type) {
			case string:
				inputs = append(inputs, s)
			default:
				// token arrays are treated as a single input
				inputs = append(inputs, "")
			}
		}
		return inputs
	default:
		return nil
	}
}

func promptTokens(inputs []string) int {
	tokens := 0
	for _, s := range inputs {
		tokens += len([]rune(s))
	}
	if tokens == 0 {
		tokens = 1
	}
	return tokens
}
