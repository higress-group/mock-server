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
	claudeDomain       = "api.anthropic.com"
	claudeMessagesPath = "/v1/messages"
	// claudeMockId is an Anthropic-style message id. ai-proxy passes it through as the OpenAI response id.
	claudeMockId    = "msg_llm-mock"
	claudeMockModel = "claude-3-5-sonnet-20241022"
	// claudeMockRequestId mirrors the request id the real API returns in every error body and the
	// "request-id" response header.
	claudeMockRequestId = "req_llm-mock"
)

// claudeError writes an Anthropic-style error response: the request-id header plus a body carrying
// the top-level type/request_id and the nested error object, matching the real API's error shape.
func claudeError(ctx *gin.Context, status int, errType, message string) {
	ctx.Header("request-id", claudeMockRequestId)
	ctx.JSON(status, gin.H{
		"type":       "error",
		"request_id": claudeMockRequestId,
		"error":      gin.H{"type": errType, "message": message},
	})
}

// claudeMessagesRequest is the Anthropic /v1/messages request shape. ai-proxy sends this
// after converting the client's OpenAI-format request.
type claudeMessagesRequest struct {
	Model    string          `json:"model"`
	Messages []claudeMessage `json:"messages"`
	System   json.RawMessage `json:"system,omitempty"`
	Stream   bool            `json:"stream,omitempty"`
	Tools    []claudeTool    `json:"tools,omitempty"`
}

type claudeTool struct {
	Name string `json:"name"`
}

type claudeMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"` // string or [{type,text,...}]
}

type claudeProvider struct{}

func (p *claudeProvider) ShouldHandleRequest(ctx *gin.Context) bool {
	context, err := getRequestContext(ctx)
	if err != nil {
		log.Errorf("get request context failed: %v", err)
		return false
	}
	return context.Host == claudeDomain && context.Path == claudeMessagesPath
}

func (p *claudeProvider) HandleChatCompletions(ctx *gin.Context) {
	// The real API requires the anthropic-version header (ai-proxy always injects it).
	if ctx.GetHeader("anthropic-version") == "" {
		claudeError(ctx, http.StatusBadRequest, "invalid_request_error", "anthropic-version header is required")
		return
	}
	// The real Anthropic API authenticates with the "x-api-key" header; the error body mirrors it.
	if ctx.GetHeader("x-api-key") == "" {
		claudeError(ctx, http.StatusUnauthorized, "authentication_error", "invalid x-api-key")
		return
	}

	var req claudeMessagesRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	response := lastClaudeUserText(&req)

	// A sentinel prompt makes the mock return the upstream auth error, letting the e2e verify that
	// ai-proxy surfaces an upstream 401 to the client instead of masking it.
	if response == "__force_auth_error__" {
		claudeError(ctx, http.StatusUnauthorized, "authentication_error", "invalid x-api-key")
		return
	}

	// When the request carries tools, reply with a tool_use block so the tool-call conversion path
	// is exercised (the mock always calls the first tool with a fixed argument).
	if len(req.Tools) > 0 && req.Stream {
		p.handleToolUseStreamResponse(ctx, req.Tools[0].Name)
		return
	}

	if req.Stream {
		p.handleStreamResponse(ctx, response)
	} else {
		p.handleNonStreamResponse(ctx, response)
	}
}

// handleToolUseStreamResponse emits an Anthropic tool_use streaming sequence: message_start,
// content_block_start (tool_use), input_json_delta chunks, content_block_stop, message_delta
// (stop_reason tool_use), message_stop.
func (p *claudeProvider) handleToolUseStreamResponse(ctx *gin.Context, toolName string) {
	utils.SetEventStreamHeaders(ctx)
	send := func(payload gin.H) bool {
		data, _ := json.Marshal(payload)
		select {
		case <-ctx.Request.Context().Done():
			return false
		default:
		}
		// Real Anthropic streaming uses named SSE events (event: <type> + data: <json>) since the
		// 2023-06-01 version; ai-proxy reads only the data: lines but the mock stays wire-faithful.
		// Write raw (not via streamEvent, whose data replacer would mangle the embedded newline).
		eventType, _ := payload["type"].(string)
		ctx.Writer.Write([]byte("event: " + eventType + "\ndata: " + string(data) + "\n\n"))
		ctx.Writer.Flush()
		return true
	}

	if !send(gin.H{"type": "message_start", "message": gin.H{
		"id": claudeMockId, "type": "message", "role": roleAssistant, "model": claudeMockModel,
		"content": []gin.H{}, "stop_reason": nil, "stop_sequence": nil,
		"usage": gin.H{"input_tokens": completionMockUsage.PromptTokens, "output_tokens": 1},
	}}) {
		return
	}
	if !send(gin.H{"type": "content_block_start", "index": 0, "content_block": gin.H{
		"type": "tool_use", "id": "toolu_llm-mock", "name": toolName, "input": gin.H{},
	}}) {
		return
	}
	// The tool arguments arrive as partial_json fragments that ai-proxy concatenates.
	for _, frag := range []string{`{"location": `, `"Beijing"}`} {
		if !send(gin.H{"type": "content_block_delta", "index": 0, "delta": gin.H{"type": "input_json_delta", "partial_json": frag}}) {
			return
		}
	}
	send(gin.H{"type": "content_block_stop", "index": 0})
	send(gin.H{"type": "message_delta", "delta": gin.H{"stop_reason": "tool_use", "stop_sequence": nil}, "usage": gin.H{"output_tokens": completionMockUsage.CompletionTokens}})
	send(gin.H{"type": "message_stop"})
}

func (p *claudeProvider) handleNonStreamResponse(ctx *gin.Context, response string) {
	ctx.JSON(http.StatusOK, gin.H{
		"id":            claudeMockId,
		"type":          "message",
		"role":          roleAssistant,
		"model":         claudeMockModel,
		"content":       []gin.H{{"type": "text", "text": response}},
		"stop_reason":   "end_turn",
		"stop_sequence": nil,
		"usage": gin.H{
			"input_tokens":  completionMockUsage.PromptTokens,
			"output_tokens": completionMockUsage.CompletionTokens,
		},
	})
}

func (p *claudeProvider) handleStreamResponse(ctx *gin.Context, response string) {
	utils.SetEventStreamHeaders(ctx)

	send := func(payload gin.H) bool {
		data, _ := json.Marshal(payload)
		select {
		case <-ctx.Request.Context().Done():
			return false
		default:
		}
		// Real Anthropic streaming uses named SSE events (event: <type> + data: <json>) since the
		// 2023-06-01 version; ai-proxy reads only the data: lines but the mock stays wire-faithful.
		// Write raw (not via streamEvent, whose data replacer would mangle the embedded newline).
		eventType, _ := payload["type"].(string)
		ctx.Writer.Write([]byte("event: " + eventType + "\ndata: " + string(data) + "\n\n"))
		ctx.Writer.Flush()
		return true
	}

	// message_start carries the assistant role and the input usage plus a small initial
	// output_tokens (the real API reports ~1 here; the cumulative total arrives in message_delta).
	if !send(gin.H{"type": "message_start", "message": gin.H{
		"id": claudeMockId, "type": "message", "role": roleAssistant, "model": claudeMockModel,
		"content": []gin.H{}, "stop_reason": nil, "stop_sequence": nil,
		"usage": gin.H{"input_tokens": completionMockUsage.PromptTokens, "output_tokens": 1},
	}}) {
		return
	}
	if !send(gin.H{"type": "content_block_start", "index": 0, "content_block": gin.H{"type": "text", "text": ""}}) {
		return
	}

	// One text_delta per rune, mirroring the byte-by-byte streaming of the other provider mocks.
	for _, r := range response {
		if !send(gin.H{"type": "content_block_delta", "index": 0, "delta": gin.H{"type": "text_delta", "text": string(r)}}) {
			return
		}
		select {
		case <-ctx.Request.Context().Done():
			return
		case <-time.After(50 * time.Millisecond):
		}
	}

	send(gin.H{"type": "content_block_stop", "index": 0})
	// message_delta.usage.output_tokens is the cumulative total for the whole message, matching the real Anthropic API.
	send(gin.H{"type": "message_delta", "delta": gin.H{"stop_reason": "end_turn", "stop_sequence": nil}, "usage": gin.H{"output_tokens": completionMockUsage.CompletionTokens}})
	send(gin.H{"type": "message_stop"})
}

// lastClaudeUserText returns the text of the last message, handling both string and
// content-block-array content forms.
func lastClaudeUserText(req *claudeMessagesRequest) string {
	if len(req.Messages) == 0 {
		return ""
	}
	last := req.Messages[len(req.Messages)-1]
	var s string
	if err := json.Unmarshal(last.Content, &s); err == nil {
		return s
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(last.Content, &blocks); err == nil {
		text := ""
		for _, b := range blocks {
			if b.Type == "text" {
				text += b.Text
			}
		}
		return text
	}
	return ""
}
