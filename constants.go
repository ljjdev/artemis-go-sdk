// Package artemis 实现了 Hikvision 萤石/海康 artemis 网关的 Go 客户端 SDK。
//
// 命名、字段与签名规则与 artemis-http-client 1.1.15 Java 版保持一致，
// 调用方可通过 Get / PostString / PostFileForm 等高层方法直接访问网关。
package artemis

import "time"

// 编码 / 算法常量。
const (
	// Encoding 默认字符编码，与 Java Constants.ENCODING 保持一致。
	Encoding = "UTF-8"
	// HMACAlgorithm 签名算法名。
	HMACAlgorithm = "HmacSHA256"
	// UserAgent HTTP User-Agent，与 Java 版保持一致。
	UserAgent = "demo/aliyun/java"
	// ClientName 写入 Hik-Request-ID 头的客户端标识，便于网关侧追踪。
	ClientName = "artemis&artemis-http-client&1.1.6"
)

// 分隔符常量，对应 Java Constants.SPE1..SPE6。
const (
	// SepComma 串联符 ","。
	SepComma = ","
	// SepColon 示意符 ":"。
	SepColon = ":"
	// SepAmp 连接符 "&"。
	SepAmp = "&"
	// SepEqual 赋值符 "="。
	SepEqual = "="
	// SepQuestion 问号符 "?"。
	SepQuestion = "?"
	// SepSemicolon 分隔符 ";"。
	SepSemicolon = ";"
	// LF 换行符。
	LF = "\n"
	// BoundaryKeyword multipart Content-Type 关键字。
	BoundaryKeyword = "boundary"
)

// 默认超时与签名头前缀。
const (
	// DefaultConnectTimeout 默认连接超时，对应 Java DEFAULT_TIMEOUT=5000ms。
	DefaultConnectTimeout = 5 * time.Second
	// DefaultSocketTimeout 默认读超时，对应 Java SOCKET_TIMEOUT=60000ms。
	DefaultSocketTimeout = 60 * time.Second
	// MaxTotalConnections 连接池最大连接数，对应 Java HttpUtil.MAX_TOTAL_CONNECTIONS。
	MaxTotalConnections = 300
	// DefaultMaxPerRoute 每个路由最大连接数，对应 Java HttpUtil.DEFAULT_MAX_PER_ROUTE。
	DefaultMaxPerRoute = 50
	// HeaderSignPrefix 参与签名的系统 Header 前缀。
	// Java 版使用全小写 "x-ca-"，这里保持一致。
	HeaderSignPrefix = "x-ca-"
)

// Content-Type 常用取值。
const (
	ContentTypeForm     = "application/x-www-form-urlencoded;charset=UTF-8"
	ContentTypeFileForm = "multipart/form-data;charset=UTF-8"
	ContentTypeStream   = "application/octet-stream;charset=UTF-8"
	ContentTypeJSON     = "application/json;charset=UTF-8"
	ContentTypeXML      = "application/xml;charset=UTF-8"
	ContentTypeText     = "application/text;charset=UTF-8"
)

// 标准 HTTP Header 名。Java 版混合大小写：标准 HTTP 头用规范形式，
// 自定义 x-ca-* 头用全小写以与 startsWith("x-ca-") 兼容。
const (
	HeaderAccept      = "Accept"
	HeaderContentMD5  = "Content-MD5"
	HeaderContentType = "Content-Type"
	HeaderUserAgent   = "User-Agent"
	HeaderDate        = "Date"
)

// HTTP 方法字面量。
const (
	HTTPMethodGET    = "GET"
	HTTPMethodPOST   = "POST"
	HTTPMethodPUT    = "PUT"
	HTTPMethodDELETE = "DELETE"
)

// HTTP Schema。
const (
	HTTPSchemaHTTP  = "http://"
	HTTPSchemaHTTPS = "https://"
)

// 系统签名前缀 Header（Java 版使用全小写以匹配 startsWith("x-ca-") 逻辑）。
const (
	HeaderXCaSignature        = "x-ca-signature"
	HeaderXCaSignatureHeaders = "x-ca-signature-headers"
	HeaderXCaTimestamp        = "x-ca-timestamp"
	HeaderXCaNonce            = "x-ca-nonce"
	HeaderXCaKey              = "x-ca-key"
	HeaderXCaPath             = "x-ca-path"
	// 响应侧头（网关回包），用户阅读时按大小写不敏感匹配。
	HeaderXCaRequestID    = "X-Ca-Request-Id"
	HeaderXCaErrorMessage = "X-Ca-Error-Message"
	HeaderHikRequestID    = "Hik-Request-ID"
)

// 错误信息常量。
const (
	// ErrHTTPSchema http/https 参数错误。
	ErrHTTPSchema = "http和https参数错误"
)
