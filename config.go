package artemis

import (
	"crypto/tls"
	"net"
	"net/http"
	"sync"
	"time"
)

// Config 保存 artemis 网关的连接信息和客户端行为配置。
//
// Config 是惰性初始化的：第一次调用 HTTPClient 时才会构造共享的 *http.Client。
// 多次调用会复用同一个 client 及其连接池，行为与 Java 版 HttpUtil 单例一致。
type Config struct {
	// Host 网关主机地址，示例：artemis.example.com:443。
	Host string
	// AppKey 合作方 AK。
	AppKey string
	// AppSecret 合作方 SK，用于签名。
	AppSecret string
	// ConnectTimeout 单次 TCP 连接建立超时，零值使用 DefaultConnectTimeout。
	ConnectTimeout time.Duration
	// SocketTimeout 整体读超时（含连接+读写），零值使用 DefaultSocketTimeout。
	SocketTimeout time.Duration
	// InsecureTLS 是否跳过证书校验。默认 true 以兼容 Java 版行为；
	// 生产环境建议显式置为 false。
	InsecureTLS bool
	// ContextPath API 网关上下文路径（可选）。
	//
	// 留空时，Path.Path 直接作为请求路径与签名路径，与 Java demo
	// 手工拼接的效果一致。填写后（例如 "/artemis"），SDK 会自动把它拼接到
	// Path.Path 之前；Path.Path 应当只传 API 地址，不带上下文。
	//
	// 拼接规则：去除 contextPath 末尾的 "/" 与 path 开头的 "/"，中间用
	// 单个 "/" 连接。
	ContextPath string

	once   sync.Once
	client *http.Client
}

// NewConfig 返回一个最小可用配置，TLS 跳过校验、超时取默认值。
func NewConfig(host, contextPath, appKey, appSecret string) *Config {
	return &Config{
		Host:           host,
		AppKey:         appKey,
		AppSecret:      appSecret,
		ConnectTimeout: DefaultConnectTimeout,
		SocketTimeout:  DefaultSocketTimeout,
		InsecureTLS:    true,
		ContextPath:    contextPath,
	}
}

// HTTPClient 返回复用的 *http.Client，第一次调用时构造。
func (c *Config) HTTPClient() *http.Client {
	c.once.Do(func() {
		connect := c.ConnectTimeout
		if connect <= 0 {
			connect = DefaultConnectTimeout
		}
		socket := c.SocketTimeout
		if socket <= 0 {
			socket = DefaultSocketTimeout
		}
		transport := &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   connect,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          MaxTotalConnections,
			MaxIdleConnsPerHost:   DefaultMaxPerRoute,
			IdleConnTimeout:       30 * time.Second,
			TLSHandshakeTimeout:   connect,
			ExpectContinueTimeout: 1 * time.Second,
			TLSClientConfig:       &tls.Config{InsecureSkipVerify: c.InsecureTLS}, //nolint:gosec // 与 Java 版默认行为一致，由调用方显式控制。
		}
		c.client = &http.Client{
			Transport: transport,
			Timeout:   socket,
		}
	})
	return c.client
}
