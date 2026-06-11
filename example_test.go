package artemis_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	artemis "github.com/ljjdev/artemis-go-sdk"
)

// 示例 / 集成测试共用的占位网关与凭证。
//
// 目的：发布时不留真实环境信息。所有值都指向本地回环 + 占位 ak/sk，
// 真实集成测试可临时改此处的常量；务必勿在 commit 中保留真实 host/ak/sk。
const (
	contextPath = "/artemis"
	hostTest    = "127.0.0.1:80"
	akTest      = "test"
	skTest      = "test"
)

// demoGet 演示一次最简单的 GET 调用。
func demoGet() {
	cfg := artemis.NewConfig(hostTest, contextPath, akTest, skTest)
	cfg.InsecureTLS = false // 生产环境务必关闭证书跳过

	path := artemis.Path{Schema: artemis.HTTPSchemaHTTPS, Path: "/api/v1/camera/list"}
	body, err := artemis.Get(cfg, path,
		artemis.WithQuery(map[string]any{"page": 1, "size": 20}),
		artemis.WithAccept(artemis.ContentTypeJSON),
	)
	if err != nil {
		_ = err
		return
	}
	_ = body
}

// demoPostString 演示 JSON 字符串 POST。
func demoPostString() {
	cfg := artemis.NewConfig(hostTest, contextPath, akTest, skTest)
	path := artemis.Path{Schema: artemis.HTTPSchemaHTTPS, Path: "/api/v1/event/subscribe"}

	body, err := artemis.PostString(cfg, path,
		`{"eventType":"motion","deviceSerial":"DS-1234"}`,
		artemis.WithContentType(artemis.ContentTypeJSON),
	)
	if err != nil {
		return
	}
	_ = body
}

// demoPostFileForm 演示 multipart 文件上传。
func demoPostFileForm() {
	cfg := artemis.NewConfig(hostTest, contextPath, akTest, skTest)
	path := artemis.Path{Schema: artemis.HTTPSchemaHTTPS, Path: "/api/v1/face/upload"}

	body, err := artemis.PostFileForm(cfg, path, map[string]any{
		"faceImage": strings.NewReader("fake-binary-data"),
		"personId":  "person-001",
	})
	if err != nil {
		return
	}
	_ = body
}

// demoDownload 演示二进制下载（图片/录像）。
func demoDownload() {
	cfg := artemis.NewConfig(hostTest, contextPath, akTest, skTest)
	path := artemis.Path{Schema: artemis.HTTPSchemaHTTPS, Path: "/api/v1/camera/preview"}

	resp, err := artemis.GetResponse(cfg, path)
	if err != nil {
		return
	}
	defer resp.Close()
	data, err := resp.ReadAll()
	if err != nil {
		return
	}
	_ = data
}

// demoContextPath 演示用 Config.ContextPath 集中管理网关上下文。
//
// 等价于 Java 端 ARTEMIS_PATH + "/api/..." 的手工拼接，但只需在 Config 上
// 配置一次，所有 API 调用共享同一前缀。
func demoContextPath() {
	cfg := artemis.NewConfig(hostTest, contextPath, akTest, skTest)
	cfg.ContextPath = contextPath // 全局一次配置

	// 调用时 Path.Path 只传 API 地址，不再重复写 /artemis 前缀
	path := artemis.Path{Schema: artemis.HTTPSchemaHTTPS, Path: "/api/eventService/v1/eventSubscriptionView"}
	body, err := artemis.PostString(cfg, path,
		`{"eventType":"motion","deviceSerial":"DS-1234"}`,
		artemis.WithContentType(artemis.ContentTypeJSON),
	)
	if err != nil {
		return
	}
	_ = body
}

// TestExampleEnd2End 把整套高层 API 跑一遍，确保示例可执行。
func TestExampleEnd2End(t *testing.T) {
	// 调用 demo 以保证编译通过；实际不依赖其返回值。
	demoGet()
	demoPostString()
	demoPostFileForm()
	demoDownload()
	demoContextPath()

	// 一个会回显请求 method/path/body 的 server。
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Ca-Request-Id", "test-req")
		body, _ := io.ReadAll(r.Body)
		out := map[string]any{
			"method": r.Method,
			"path":   r.URL.Path,
			"query":  r.URL.Query(),
			"body":   string(body),
		}
		_ = json.NewEncoder(w).Encode(out)
	}))
	t.Cleanup(srv.Close)

	// 构造一个 *Config 调用示例 API。host 用回环 server 的真实 host。
	u, _ := url.Parse(srv.URL)
	cfg := artemis.NewConfig(u.Host, contextPath, akTest, skTest)
	cfg.InsecureTLS = true
	p := artemis.Path{Schema: u.Scheme + "://", Path: "/api/v1/example"}

	// GET
	body, err := artemis.Get(cfg, p, artemis.WithQuery(map[string]any{"page": 1}))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !strings.Contains(body, `"method":"GET"`) {
		t.Errorf("Get body = %q", body)
	}

	// POST form
	body, err = artemis.PostForm(cfg, p, map[string]string{"a": "1", "b": "2"})
	if err != nil {
		t.Fatalf("PostForm: %v", err)
	}
	if !strings.Contains(body, `"method":"POST"`) || !strings.Contains(body, `a=1`) {
		t.Errorf("PostForm body = %q", body)
	}

	// POST string
	body, err = artemis.PostString(cfg, p, "hello")
	if err != nil {
		t.Fatalf("PostString: %v", err)
	}
	if !strings.Contains(body, `"body":"hello"`) {
		t.Errorf("PostString body = %q", body)
	}

	// POST bytes
	body, err = artemis.PostBytes(cfg, p, []byte{0x01, 0x02, 0x03})
	if err != nil {
		t.Fatalf("PostBytes: %v", err)
	}
	if !strings.Contains(body, `body":"\u0001\u0002\u0003"`) {
		t.Errorf("PostBytes body = %q", body)
	}

	// PUT string
	body, err = artemis.PutString(cfg, p, "data")
	if err != nil {
		t.Fatalf("PutString: %v", err)
	}
	if !strings.Contains(body, `"method":"PUT"`) {
		t.Errorf("PutString body = %q", body)
	}

	// PUT bytes
	body, err = artemis.PutBytes(cfg, p, []byte{0xaa})
	if err != nil {
		t.Fatalf("PutBytes: %v", err)
	}
	if !strings.Contains(body, `"method":"PUT"`) {
		t.Errorf("PutBytes body = %q", body)
	}

	// DELETE
	body, err = artemis.Delete(cfg, p)
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if !strings.Contains(body, `"method":"DELETE"`) {
		t.Errorf("Delete body = %q", body)
	}
}

func TestSubOrgList(t *testing.T) {
	cfg := artemis.NewConfig(hostTest, contextPath, akTest, skTest)
	cfg.InsecureTLS = true
	path := artemis.Path{Schema: artemis.HTTPSchemaHTTP, Path: "/api/resource/v1/org/parentOrgIndexCode/subOrgList"}
	body, err := artemis.PostString(cfg, path,
		`{"parentOrgIndexCode":"b0901bbb-24d6-46fa-b7ed-2c499ec57692","pageNo":1,"pageSize":10}`,
		artemis.WithContentType(artemis.ContentTypeJSON),
	)
	if err != nil {
		t.Fatalf("PostString: %v", err)
	}
	fmt.Println(body)
}

func TestEventSubscriptionView(t *testing.T) {
	cfg := artemis.NewConfig(hostTest, contextPath, akTest, skTest)
	cfg.InsecureTLS = true
	path := artemis.Path{Schema: artemis.HTTPSchemaHTTP, Path: "/api/eventService/v1/eventSubscriptionView"}
	body, err := artemis.Get(cfg, path,
		artemis.WithContentType(artemis.ContentTypeJSON),
	)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	fmt.Println(body)
}

// 测试获取事件图片
func TestGetEventPics(t *testing.T) {
	cfg := artemis.NewConfig(hostTest, contextPath, akTest, skTest)
	cfg.InsecureTLS = true
	path := artemis.Path{Schema: artemis.HTTPSchemaHTTP, Path: "/api/acs/v1/event/pictures"}
	resp, err := artemis.PostStringImg(cfg, path,
		`{"svrIndexCode":"dde962d0-17b3-4eb5-bb54-48f46429d7c7","picUri":"/pic?8d86=6853i31-=o391bp24f8d636-5ebdb03b4*011s=**316==*p718=7t7578039070=0l2*1*6848=9o61f-13*lef-od36ba2493f1"}`,
		artemis.WithContentType(artemis.ContentTypeJSON), artemis.WithHeader(map[string]string{"tagId": "tianchuang"}),
	)
	if err != nil {
		t.Fatalf("PostStringResponse: %v", err)
	}
	t.Logf("StatusCode: %d, Location: %s", resp.StatusCode, resp.Headers["Location"])
}
