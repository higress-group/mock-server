package rerank

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// This package simulates the cohere v1 rerank endpoint (/v1/rerank),
// usable by any rerank-compatible upstream.
type rerankRequest struct {
	Model     string   `json:"model"`
	Query     string   `json:"query"`
	Documents []string `json:"documents"`
	TopN      int      `json:"top_n"`
}

type rerankResponse struct {
	Id            string           `json:"id"`
	Results       []rerankResult   `json:"results"`
	DocumentCount int              `json:"document_count"`
	Model         string           `json:"model"`
}

type rerankResult struct {
	Index         int     `json:"index"`
	RelevanceScore float64 `json:"relevance_score"`
}

// HandleRerank simulates the cohere v1 /v1/rerank endpoint. Scores are
// deterministic (descending by index) so conformance assertions can compare
// the full response body.
func HandleRerank(ctx *gin.Context) {
	var request rerankRequest
	if err := ctx.ShouldBindJSON(&request); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if len(request.Documents) == 0 {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "documents must not be empty"})
		return
	}

	count := len(request.Documents)
	if request.TopN > 0 && request.TopN < count {
		count = request.TopN
	}

	response := rerankResponse{
		Id:            "rerank-llm-mock",
		Results:       make([]rerankResult, 0, count),
		DocumentCount: len(request.Documents),
		Model:         request.Model,
	}
	for i := 0; i < count; i++ {
		response.Results = append(response.Results, rerankResult{
			Index:         i,
			RelevanceScore: 1.0 - float64(i)*0.1,
		})
	}

	ctx.JSON(http.StatusOK, response)
}
