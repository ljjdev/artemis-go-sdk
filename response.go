package artemis

import (
	"fmt"
	"io"
	"net/http"
)

// Response 表示一次 artemis 请求的响应。
//
// 当作为文本响应返回时，Body 字段包含完整内容；
// 当作为下载响应（image / file download）返回时，Body 字段为 nil，
// 调用方应使用 RawBody 自行读取并 Close。
type Response struct {
	// StatusCode HTTP 状态码。
	StatusCode int
	// Headers 响应 Header 列表。
	Headers map[string]string
	// ContentType 响应的 Content-Type，等价于 Headers["Content-Type"]。
	ContentType string
	// RequestID 网关侧的请求追踪 ID，等价于 X-Ca-Request-Id。
	RequestID string
	// ErrorMessage 网关侧返回的错误信息，等价于 X-Ca-Error-Message。
	ErrorMessage string
	// Body 响应体字符串（仅在 KeepBody=false 时填充）。
	Body string
	// RawBody 原始响应流（仅在 KeepBody=true 时非 nil），由调用方负责 Close。
	RawBody io.ReadCloser
	// raw 保留底层 http.Response 以便在需要时访问 Transport 相关信息。
	raw *http.Response
}

// ReadAll 在下载场景下读取完整响应体到内存。
func (r *Response) ReadAll() ([]byte, error) {
	if r == nil || r.RawBody == nil {
		return nil, fmt.Errorf("artemis: response body is nil")
	}
	defer r.RawBody.Close()
	return io.ReadAll(r.RawBody)
}

// SaveTo 在下载场景下将响应体写入 w 并返回写入字节数。
func (r *Response) SaveTo(w io.Writer) (int64, error) {
	if r == nil || r.RawBody == nil {
		return 0, fmt.Errorf("artemis: response body is nil")
	}
	defer r.RawBody.Close()
	return io.Copy(w, r.RawBody)
}

// Close 释放底层响应资源。仅在 KeepBody=true 时有实际效果。
func (r *Response) Close() error {
	if r == nil || r.RawBody == nil {
		return nil
	}
	return r.RawBody.Close()
}
