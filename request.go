package artemis

import "time"

// Method 描述请求方法分类，对应 Java 端 enums.Method。
//
// 命名沿用 Java 枚举风格以保持兼容性；带有 RESPONSE / DOWNLOAD 后缀的项
// 表示返回二进制响应的内部变体（与 Java HttpUtil.httpImg* 路径对应）。
type Method string

const (
	MethodGet                Method = "GET"
	MethodGetResponse        Method = "GET_RESPONSE"
	MethodPostForm           Method = "POST_FORM"
	MethodPostFormResponse   Method = "POST_FORM_RESPONSE"
	MethodPostString         Method = "POST_STRING"
	MethodPostStringResponse Method = "POST_STRING_RESPONSE"
	MethodPostBytes          Method = "POST_BYTES"
	MethodPostFile           Method = "POST_FILE"
	MethodPutString          Method = "PUT_STRING"
	MethodPutBytes           Method = "PUT_BYTES"
	MethodDelete             Method = "DELETE"
	MethodPostDownload       Method = "POST_DOWNLOAD"
)

// Path 表示一个 artemis 接口路径。Java 端用 Map[String,String]{schema: path} 表达，
// 这里用结构体取代，避免依赖 map 迭代顺序。
type Path struct {
	// Schema "http://" 或 "https://"。
	Schema string
	// Path 网关接口路径，例如 "/api/artemis/v1/xxx"。
	Path string
}

// Request 描述一个完整的 artemis 请求。Request 由内部 HTTP 层使用，
// 公共 API 通过 functional options 在内部构造并销毁 Request。
type Request struct {
	Method    Method
	Host      string
	Path      string
	AppKey    string
	AppSecret string
	Headers   map[string]string
	Querys    map[string]any
	// Bodys 同时承载表单（POST_FORM）与文件（POST_FILE）两种场景：
	//   - 表单：值通常是 string
	//   - 文件：值可能是 string / *os.File / io.Reader / fmt.Stringer
	// 签名逻辑会按 Method 决定是否纳入；具体见 http_client.go 中各 method 实现。
	Bodys                map[string]any
	StringBody           string
	BytesBody            []byte
	SignHeaderPrefixList []string
	// Timeout 单次请求总超时，零值使用 Config.SocketTimeout。
	Timeout time.Duration
	// KeepBody true 表示下载场景：保留 Body io.ReadCloser，不读取为字符串。
	KeepBody bool
}
