package chat

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"llm-mock-server/pkg/log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	// ai-proxy rewrites Bedrock requests to bedrock-runtime.{region}.amazonaws.com
	// (or bedrock-mantle.{region}.api.aws). Match on the stable fragments.
	bedrockHostFragment      = "bedrock"
	bedrockDomainFragment    = "amazonaws.com"
	bedrockConversePath      = "/converse"
	bedrockConverseStream    = "/converse-stream"
	bedrockStopReasonEndTurn = "end_turn"
)

type bedrockProvider struct{}

func (p *bedrockProvider) ShouldHandleRequest(ctx *gin.Context) bool {
	requestCtx, err := getRequestContext(ctx)
	if err != nil {
		log.Errorf("get request context failed: %v", err)
		return false
	}

	// ai-proxy transforms OpenAI chat completion requests into Bedrock Converse
	// /model/{modelId}/converse(-stream) before reaching the mock.
	host := requestCtx.Host
	path := requestCtx.Path
	if !strings.Contains(host, bedrockHostFragment) || !strings.Contains(host, bedrockDomainFragment) {
		return false
	}
	return strings.HasSuffix(path, bedrockConversePath) || strings.HasSuffix(path, bedrockConverseStream)
}

func (p *bedrockProvider) HandleChatCompletions(ctx *gin.Context) {
	// Bedrock auth (SigV4 with AK/SK, or apiTokens as x-api-key / Bearer) is not
	// enforced: ai-proxy's apiTokens mode sends a Bearer/x-api-key the mock has no
	// need to validate, and the mock simulates the protocol rather than credentials.
	isStreaming := strings.HasSuffix(ctx.Request.URL.Path, bedrockConverseStream)

	var bedrockRequest bedrockConverseRequest
	if err := ctx.ShouldBindJSON(&bedrockRequest); err != nil {
		p.sendErrorResponse(ctx, http.StatusBadRequest, fmt.Sprintf("Invalid request: %v", err.Error()))
		return
	}

	if err := p.validateRequest(&bedrockRequest); err != nil {
		p.sendErrorResponse(ctx, http.StatusBadRequest, fmt.Sprintf("Validation error: %v", err.Error()))
		return
	}

	content := p.generateResponse(&bedrockRequest)

	if isStreaming {
		p.handleStreamResponse(ctx, content)
	} else {
		p.handleNonStreamResponse(ctx, content)
	}
}

func (p *bedrockProvider) validateRequest(req *bedrockConverseRequest) error {
	if len(req.Messages) == 0 {
		return fmt.Errorf("messages are required")
	}
	for i, msg := range req.Messages {
		if len(msg.Content) == 0 {
			return fmt.Errorf("message %d: content is required", i)
		}
		for j, block := range msg.Content {
			if block.Text == "" {
				return fmt.Errorf("message %d, content %d: text is required", i, j)
			}
		}
	}
	return nil
}

func (p *bedrockProvider) generateResponse(req *bedrockConverseRequest) string {
	// Mirror the gemini/vertex mocks so the response is identifiable as the
	// Bedrock simulation.
	content := "This is a mock response from Bedrock provider. "
	if len(req.Messages) > 0 {
		lastMsg := req.Messages[len(req.Messages)-1]
		if len(lastMsg.Content) > 0 {
			runes := []rune(lastMsg.Content[len(lastMsg.Content)-1].Text)
			if len(runes) > 50 {
				content += "You said: " + string(runes[:50]) + "..."
			} else {
				content += "You said: " + string(runes)
			}
		}
	}
	return content
}

func (p *bedrockProvider) handleNonStreamResponse(ctx *gin.Context, response string) {
	// Schema matches Bedrock Converse (output.message.content[].text / stopReason /
	// usage), which ai-proxy parses into an OpenAI response.
	bedrockResponse := bedrockConverseResponse{
		Metrics:    bedrockConverseMetrics{LatencyMs: 100},
		Output:     bedrockConverseOutput{Message: bedrockConverseMessage{Role: "assistant", Content: []bedrockContentBlock{{Text: response}}}},
		StopReason: bedrockStopReasonEndTurn,
		Usage: bedrockTokenUsage{
			InputTokens:  completionMockUsage.PromptTokens,
			OutputTokens: completionMockUsage.CompletionTokens,
			TotalTokens:  completionMockUsage.TotalTokens,
		},
	}
	ctx.JSON(http.StatusOK, bedrockResponse)
}

func (p *bedrockProvider) handleStreamResponse(ctx *gin.Context, response string) {
	// Bedrock converse-stream uses the Amazon Event Stream binary framing, not
	// SSE/JSON. Encode each ConverseStreamEvent as an event-stream message.
	ctx.Header("Content-Type", "application/vnd.amazon.eventstream")
	ctx.Header("Cache-Control", "no-cache")
	ctx.Header("Connection", "keep-alive")
	ctx.Header("Access-Control-Allow-Origin", "*")

	words := strings.Fields(response)
	flusher, ok := ctx.Writer.(http.Flusher)

	for i, word := range words {
		deltaPayload, _ := json.Marshal(map[string]interface{}{
			"contentBlockIndex": 0,
			"delta":             map[string]string{"text": word + " "},
		})
		ctx.Writer.Write(encodeBedrockEventStreamMessage("contentBlockDelta", deltaPayload))
		if ok {
			flusher.Flush()
		}
		select {
		case <-ctx.Request.Context().Done():
			return
		default:
		}

		// The final word is followed by a messageStop carrying the stop reason.
		if i == len(words)-1 {
			stopPayload, _ := json.Marshal(map[string]string{"stopReason": bedrockStopReasonEndTurn})
			ctx.Writer.Write(encodeBedrockEventStreamMessage("messageStop", stopPayload))
			if ok {
				flusher.Flush()
			}
		}
		select {
		case <-ctx.Request.Context().Done():
			return
		case <-time.After(50 * time.Millisecond):
		}
	}
}

func (p *bedrockProvider) sendErrorResponse(ctx *gin.Context, statusCode int, message string) {
	ctx.JSON(statusCode, gin.H{
		"message": message,
	})
}

// encodeBedrockEventStreamMessage builds a single AWS Event Stream message frame:
// [TotalLength:4][HeadersLength:4][PreludeCRC:4][Headers][Payload][MessageCRC:4].
// All integers are big-endian; CRCs are CRC32-IEEE. This mirrors the framing
// ai-proxy's bedrock provider decodes (see extractAmazonEventStreamEvents).
func encodeBedrockEventStreamMessage(eventType string, payload []byte) []byte {
	headers := encodeBedrockEventStreamHeaders(eventType)
	headersLen := len(headers)
	totalLen := uint32(16 + headersLen + len(payload))

	prelude := make([]byte, 8)
	binary.BigEndian.PutUint32(prelude[0:4], totalLen)
	binary.BigEndian.PutUint32(prelude[4:8], uint32(headersLen))
	preludeCRC := crc32.ChecksumIEEE(prelude)
	preludeCrc := make([]byte, 4)
	binary.BigEndian.PutUint32(preludeCrc, preludeCRC)

	msg := make([]byte, 0, int(totalLen))
	msg = append(msg, prelude...)
	msg = append(msg, preludeCrc...)
	msg = append(msg, headers...)
	msg = append(msg, payload...)
	msgCRC := crc32.ChecksumIEEE(msg)
	msgCrc := make([]byte, 4)
	binary.BigEndian.PutUint32(msgCrc, msgCRC)
	msg = append(msg, msgCrc...)
	return msg
}

// encodeBedrockEventStreamHeaders encodes the three headers ai-proxy expects on
// an event frame: :message-type=event, :event-type=<eventType>,
// :content-type=application/json. String values use type byte 7.
func encodeBedrockEventStreamHeaders(eventType string) []byte {
	var buf bytes.Buffer
	writeHeader := func(name, value string) {
		buf.WriteByte(byte(len(name)))
		buf.WriteString(name)
		buf.WriteByte(7) // type: string
		lenBuf := make([]byte, 2)
		binary.BigEndian.PutUint16(lenBuf, uint16(len(value)))
		buf.Write(lenBuf)
		buf.WriteString(value)
	}
	writeHeader(":message-type", "event")
	writeHeader(":event-type", eventType)
	writeHeader(":content-type", "application/json")
	return buf.Bytes()
}

// Bedrock Converse request / response data structures. The request schema matches
// what ai-proxy produces after transforming an OpenAI request; the response
// schema matches what ai-proxy parses back into an OpenAI response.
type bedrockConverseRequest struct {
	Messages []bedrockRequestMessage `json:"messages"`
}

type bedrockRequestMessage struct {
	Role    string                  `json:"role"`
	Content []bedrockRequestContent `json:"content"`
}

type bedrockRequestContent struct {
	Text string `json:"text"`
}

type bedrockConverseResponse struct {
	Metrics    bedrockConverseMetrics `json:"metrics"`
	Output     bedrockConverseOutput  `json:"output"`
	StopReason string                 `json:"stopReason"`
	Usage      bedrockTokenUsage      `json:"usage"`
}

type bedrockConverseMetrics struct {
	LatencyMs int `json:"latencyMs"`
}

type bedrockConverseOutput struct {
	Message bedrockConverseMessage `json:"message"`
}

type bedrockConverseMessage struct {
	Content []bedrockContentBlock `json:"content"`
	Role    string                `json:"role"`
}

type bedrockContentBlock struct {
	Text string `json:"text,omitempty"`
}

type bedrockTokenUsage struct {
	InputTokens  int `json:"inputTokens,omitempty"`
	OutputTokens int `json:"outputTokens,omitempty"`
	TotalTokens  int `json:"totalTokens"`
}
