package main

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"time"
)

const (
	// 最大请求体大小：10MB
	maxBodySize = 10 << 20
	// 服务器版本信息
	serverVersion = "ext-auth-mock-server/1.0"
)

// ErrorResponse 定义标准错误响应结构
type ErrorResponse struct {
	ErrorMessage string `json:"error"`
}

// SuccessResponse 定义标准成功响应结构
type SuccessResponse struct {
	Message string `json:"message"`
}

// validateAuth 验证请求头中是否包含 Authorization 字段
// 若缺失则返回 401 Unauthorized
func validateAuth(w http.ResponseWriter, r *http.Request) bool {
	if r.Header.Get("Authorization") == "" {
		sendErrorResponse(w, "Missing required request header", http.StatusUnauthorized, "MissingAuthorization")
		return false
	}
	return true
}

// always200Handler 处理始终返回 200 OK 的请求
// 记录请求信息并返回成功响应
func always200Handler(w http.ResponseWriter, r *http.Request) {
	if !validateRequest(w, r) {
		return
	}

	logRequest(r)
	setSuccessHeaders(w)
	json.NewEncoder(w).Encode(SuccessResponse{Message: "OK"})
}

// always500Handler 处理始终返回 500 Internal Server Error 的请求
// 记录请求信息并返回错误响应
func always500Handler(w http.ResponseWriter, r *http.Request) {
	if !validateRequest(w, r) {
		return
	}

	logRequest(r)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusInternalServerError)
	json.NewEncoder(w).Encode(ErrorResponse{ErrorMessage: "Internal Server Error"})
}

// requireRequestBody200Handler 处理需要校验请求体的场景
// 针对 ext-auth 插件中 authorization_request 配置为 true 的情况
// 当请求体不存在时返回 400，存在时返回成功响应
func requireRequestBody200Handler(w http.ResponseWriter, r *http.Request) {
	if !validateRequest(w, r) {
		return
	}

	// 检查请求体是否存在
	if r.ContentLength == 0 {
		sendErrorResponse(w, "Request body is required", http.StatusBadRequest, "MissingRequestBody")
		return
	}

	logRequest(r)
	setSuccessHeaders(w)
	json.NewEncoder(w).Encode(SuccessResponse{Message: "OK"})
}

// validateRequest 执行通用请求验证
// 包括认证校验和请求体读取
func validateRequest(w http.ResponseWriter, r *http.Request) bool {
	if !validateAuth(w, r) {
		return false
	}

	// 读取并恢复请求体，确保后续处理可以再次读取
	bodyBuf, err := copyBody(r)
	if err != nil {
		sendErrorResponse(w, "Error reading request body", http.StatusInternalServerError, "ServerError")
		return false
	}

	// 记录请求体（限制长度避免日志过大）
	if bodyBuf.Len() > 0 {
		bodyStr := bodyBuf.String()
		if len(bodyStr) > 1024 {
			bodyStr = bodyStr[:1024] + "..."
		}
		log.Printf("Request Body: %s", bodyStr)
	} else {
		log.Println("Request Body: Empty")
	}
	log.Println("==============================")

	return true
}

// copyBody 复制请求体内容以便日志记录和后续处理
// 返回请求体的副本并恢复原始请求体
func copyBody(r *http.Request) (*bytes.Buffer, error) {
	limitReader := io.LimitReader(r.Body, maxBodySize)
	bodyBytes, err := io.ReadAll(limitReader)
	if err != nil {
		return nil, err
	}
	r.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	return bytes.NewBuffer(bodyBytes), nil
}

// logRequest 记录请求的基本信息
func logRequest(r *http.Request) {
	log.Printf("Method: %s", r.Method)
	log.Printf("Path: %s", r.URL.Path)
	for key, values := range r.Header {
		log.Printf("Header[%q] = %q", key, values)
	}
}

// setSuccessHeaders 设置成功响应的通用头部
func setSuccessHeaders(w http.ResponseWriter) {
	w.Header().Set("X-User-ID", "123456")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
}

// sendErrorResponse 统一发送错误响应
// 设置标准错误格式和自定义错误头部
func sendErrorResponse(w http.ResponseWriter, errorMsg string, statusCode int, errorReason string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Error-Reason", errorReason)
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(ErrorResponse{ErrorMessage: errorMsg})
}

// withHeaders 中间件，为所有响应添加统一的头部信息
func withHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Mock-Server-Timestamp", time.Now().UTC().Format(time.RFC3339))
		w.Header().Set("X-Server-Name", serverVersion)
		next.ServeHTTP(w, r)
	})
}

func main() {
	mux := http.NewServeMux()

	// 基础测试接口
	mux.HandleFunc("/always-200", always200Handler)
	mux.HandleFunc("/prefix/always-200/", always200Handler)
	mux.HandleFunc("/always-500", always500Handler)
	mux.HandleFunc("/prefix/always-500/", always500Handler)

	// 针对 ext-auth 插件中 authorization_request 配置中 with_request_body 为 true 的场景
	// 校验请求体是否存在，不存在时返回 400
	mux.HandleFunc("/require-request-body-200", requireRequestBody200Handler)
	mux.HandleFunc("/prefix/require-request-body-200/", requireRequestBody200Handler)

	log.Printf("Starting server on :8090")
	log.Printf("Server version: %s", serverVersion)
	log.Fatal(http.ListenAndServe(":8090", withHeaders(mux)))
}
