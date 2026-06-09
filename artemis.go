package artemis

import "strings"

// 公共 API 入口。
//
// 12 个方法对应 Java 端 ArtemisHttpUtil 的 do* 系列：
//
//	GET 文本/下载         Get / GetResponse
//	POST 表单            PostForm / PostFormResponse
//	POST 字符串          PostString / PostStringResponse
//	POST 字节            PostBytes
//	POST 文件(multipart) PostFileForm
//	PUT  字符串/字节     PutString / PutBytes
//	DELETE               Delete
//	POST 下载二进制       PostDownloadFile
//
// 参数通过 functional options 注入，常见选项见 Option 系列构造器。
// 所有方法在内部构造 *Request 并由 dispatchRequest 调度到具体 HTTP 实现。

// callOptions 是 functional options 累积的中间状态。
type callOptions struct {
	accept      string
	contentType string
	header      map[string]string
	query       map[string]any
}

// Option 修改一次调用的可选参数。
type Option func(*callOptions)

// WithAccept 设置 Accept 头；为空时由各方法取默认值（*/*）。
func WithAccept(accept string) Option {
	return func(o *callOptions) { o.accept = accept }
}

// WithContentType 设置 Content-Type 头；为空时由各方法取默认值。
func WithContentType(ct string) Option {
	return func(o *callOptions) { o.contentType = ct }
}

// WithHeader 合并额外的 HTTP 头，相同 key 覆盖已有值。
func WithHeader(h map[string]string) Option {
	return func(o *callOptions) { o.header = h }
}

// WithQuery 合并 URL query 参数。
func WithQuery(q map[string]any) Option {
	return func(o *callOptions) { o.query = q }
}

// applyOptions 把 opts 累积到 callOptions，并设置各方法的默认值。
func applyOptions(method Method, p Path, opts []Option) callOptions {
	o := callOptions{
		accept:      "*/*",
		contentType: defaultContentType(method),
	}
	for _, opt := range opts {
		opt(&o)
	}
	return o
}

// defaultContentType 给出各 Method 的默认 Content-Type，与 Java 版对齐。
func defaultContentType(method Method) string {
	switch method {
	case MethodPostForm, MethodPostFormResponse:
		return ContentTypeForm
	case MethodPostFile:
		return ContentTypeFileForm
	case MethodPutString, MethodPutBytes, MethodPostBytes,
		MethodPostString, MethodPostStringResponse, MethodPostDownload:
		return ContentTypeText
	}
	return ""
}

// buildRequest 装配一个 *Request，自动从 cfg / p 提取 host/appKey/appSecret。
func buildRequest(cfg *Config, p Path, method Method, o callOptions) *Request {
	headers := map[string]string{
		HeaderAccept:      o.accept,
		HeaderContentType: o.contentType,
	}
	for k, v := range o.header {
		headers[k] = v
	}
	return &Request{
		Method:               method,
		Host:                 p.Schema + cfg.Host,
		Path:                 joinPath(cfg.ContextPath, p.Path),
		AppKey:               cfg.AppKey,
		AppSecret:            cfg.AppSecret,
		Headers:              headers,
		Querys:               o.query,
		SignHeaderPrefixList: nil,
	}
}

// Get 发起 GET 请求并返回响应体字符串。
//
// 等价于 Java ArtemisHttpUtil.doGetArtemis(... headers != null)。
func Get(cfg *Config, p Path, opts ...Option) (string, error) {
	o := applyOptions(MethodGet, p, opts)
	req := buildRequest(cfg, p, MethodGet, o)
	resp, err := dispatchRequest(cfg, req)
	if err != nil {
		return "", err
	}
	return resp.Body, nil
}

// GetResponse 发起 GET 请求并返回完整 *Response（图片/文件下载场景）。
//
// 等价于 Java ArtemisHttpUtil.doGetResponse(... headers != null)。
func GetResponse(cfg *Config, p Path, opts ...Option) (*Response, error) {
	o := applyOptions(MethodGetResponse, p, opts)
	req := buildRequest(cfg, p, MethodGetResponse, o)
	return dispatchRequest(cfg, req)
}

// PostForm 发起 application/x-www-form-urlencoded POST 并返回响应体字符串。
//
// 等价于 Java ArtemisHttpUtil.doPostFormArtemis。
func PostForm(cfg *Config, p Path, body map[string]string, opts ...Option) (string, error) {
	o := applyOptions(MethodPostForm, p, opts)
	req := buildRequest(cfg, p, MethodPostForm, o)
	req.Bodys = anyMapFromStringMap(body)
	resp, err := dispatchRequest(cfg, req)
	if err != nil {
		return "", err
	}
	return resp.Body, nil
}

// PostFormResponse 表单 POST 的二进制下载变体。
func PostFormResponse(cfg *Config, p Path, body map[string]string, opts ...Option) (*Response, error) {
	o := applyOptions(MethodPostFormResponse, p, opts)
	req := buildRequest(cfg, p, MethodPostFormResponse, o)
	req.Bodys = anyMapFromStringMap(body)
	return dispatchRequest(cfg, req)
}

// PostString 发起 application/text (默认) POST 并返回响应体字符串。
//
// 当 contentType 为 application/x-www-form-urlencoded 时，body 会被拆为表单键值对用于签名。
func PostString(cfg *Config, p Path, body string, opts ...Option) (string, error) {
	o := applyOptions(MethodPostString, p, opts)
	req := buildRequest(cfg, p, MethodPostString, o)
	req.StringBody = body
	resp, err := dispatchRequest(cfg, req)
	if err != nil {
		return "", err
	}
	return resp.Body, nil
}

// PostStringResponse 文本 POST 的二进制下载变体。
func PostStringResponse(cfg *Config, p Path, body string, opts ...Option) (*Response, error) {
	o := applyOptions(MethodPostStringResponse, p, opts)
	req := buildRequest(cfg, p, MethodPostStringResponse, o)
	req.StringBody = body
	return dispatchRequest(cfg, req)
}

// PostBytes 发起 application/octet-stream POST 并返回响应体字符串。
//
// 等价于 Java ArtemisHttpUtil.doPostBytesArtemis。
func PostBytes(cfg *Config, p Path, body []byte, opts ...Option) (string, error) {
	o := applyOptions(MethodPostBytes, p, opts)
	req := buildRequest(cfg, p, MethodPostBytes, o)
	req.BytesBody = body
	resp, err := dispatchRequest(cfg, req)
	if err != nil {
		return "", err
	}
	return resp.Body, nil
}

// PostFileForm 发起 multipart/form-data POST 并返回响应体字符串。
//
// body 中 value 类型支持：
//   - string / fmt.Stringer：作为文本字段
//   - *os.File：作为文件字段
//   - io.Reader：作为文件字段（无文件名）
//
// multipart 内容不参与签名（与 Java HttpUtil.httpFilePost 一致）。
func PostFileForm(cfg *Config, p Path, body map[string]any, opts ...Option) (string, error) {
	o := applyOptions(MethodPostFile, p, opts)
	req := buildRequest(cfg, p, MethodPostFile, o)
	req.Bodys = body
	resp, err := dispatchRequest(cfg, req)
	if err != nil {
		return "", err
	}
	return resp.Body, nil
}

// PutString 发起 application/text (默认) PUT 并返回响应体字符串。
func PutString(cfg *Config, p Path, body string, opts ...Option) (string, error) {
	o := applyOptions(MethodPutString, p, opts)
	req := buildRequest(cfg, p, MethodPutString, o)
	req.StringBody = body
	resp, err := dispatchRequest(cfg, req)
	if err != nil {
		return "", err
	}
	return resp.Body, nil
}

// PutBytes 发起 application/octet-stream PUT 并返回响应体字符串。
func PutBytes(cfg *Config, p Path, body []byte, opts ...Option) (string, error) {
	o := applyOptions(MethodPutBytes, p, opts)
	req := buildRequest(cfg, p, MethodPutBytes, o)
	req.BytesBody = body
	resp, err := dispatchRequest(cfg, req)
	if err != nil {
		return "", err
	}
	return resp.Body, nil
}

// Delete 发起 DELETE 请求并返回响应体字符串。
func Delete(cfg *Config, p Path, opts ...Option) (string, error) {
	o := applyOptions(MethodDelete, p, opts)
	req := buildRequest(cfg, p, MethodDelete, o)
	resp, err := dispatchRequest(cfg, req)
	if err != nil {
		return "", err
	}
	return resp.Body, nil
}

// PostDownloadFile 发起 POST 并保留完整 *Response（用于二进制下载）。
//
// 等价于 Java ArtemisHttpUtil.doPostDownloadFileArtemis。
func PostDownloadFile(cfg *Config, p Path, body string, opts ...Option) (*Response, error) {
	o := applyOptions(MethodPostDownload, p, opts)
	req := buildRequest(cfg, p, MethodPostDownload, o)
	req.StringBody = body
	return dispatchRequest(cfg, req)
}

// anyMapFromStringMap 把 map[string]string 转成 map[string]any 供 Request.Bodys 使用。
func anyMapFromStringMap(in map[string]string) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// joinPath 把 contextPath 和 path 拼成一条 URL 路径，规则：
//   - contextPath 为空时直接返回 path
//   - 否则去除 contextPath 末尾的 "/" 与 path 开头的 "/"，中间用单个 "/" 连接
//   - contextPath 退化为 "/" 时（即去除末尾斜杠后为空）退化为 path
func joinPath(contextPath, path string) string {
	if contextPath == "" {
		return path
	}
	ctx := strings.TrimRight(contextPath, "/")
	if ctx == "" {
		return path
	}
	api := strings.TrimLeft(path, "/")
	if api == "" {
		return ctx
	}
	return ctx + "/" + api
}
