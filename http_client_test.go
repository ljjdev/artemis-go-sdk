package artemis

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// newTestServer 返回一个 httptest.Server：把请求 Header / query / body 全部 echo 回 body。
type echoServer struct {
	t        *testing.T
	server   *httptest.Server
	lastBody []byte
	lastHdr  http.Header
}

const contextPath = "/artemis"

func newEchoServer(t *testing.T) *echoServer {
	t.Helper()
	es := &echoServer{t: t}
	es.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		es.lastBody = body
		es.lastHdr = r.Header.Clone()
		w.Header().Set("X-Ca-Request-Id", "req-12345")
		w.Header().Set("Content-Type", "application/json")
		out := map[string]any{
			"method":  r.Method,
			"path":    r.URL.Path,
			"query":   r.URL.Query(),
			"headers": r.Header,
			"body":    string(body),
		}
		_ = json.NewEncoder(w).Encode(out)
	}))
	t.Cleanup(es.server.Close)
	return es
}

func (es *echoServer) URL() string { return es.server.URL }

func (es *echoServer) host(t *testing.T) (host string, path Path) {
	t.Helper()
	u, _ := url.Parse(es.URL())
	return u.Host, Path{Schema: u.Scheme + "://", Path: "/api/v1/test"}
}

// newTestConfig 根据测试 server URL 构造 *Config。
func newTestConfig(t *testing.T, es *echoServer) *Config {
	t.Helper()
	host, _ := es.host(t)
	return NewConfig(host, contextPath, "test-ak", "test-sk")
}

func TestGet_QueryAndSignature(t *testing.T) {
	es := newEchoServer(t)
	cfg := newTestConfig(t, es)
	p := Path{Schema: "http://", Path: "/api/v1/test"}

	body, err := Get(cfg, p, WithQuery(map[string]any{"foo": "bar", "n": 1}))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !strings.Contains(body, `"path":"/api/v1/test"`) {
		t.Errorf("server did not receive path: %s", body)
	}
	if !strings.Contains(body, `"query":{"foo":["bar"]`) || !strings.Contains(body, `"n":["1"]`) {
		t.Errorf("query not echoed: %s", body)
	}
	if es.lastHdr.Get("X-Ca-Signature") == "" {
		t.Errorf("X-Ca-Signature not set")
	}
	if es.lastHdr.Get("X-Ca-Key") != "test-ak" {
		t.Errorf("X-Ca-Key = %q, want test-ak", es.lastHdr.Get("X-Ca-Key"))
	}
	if es.lastHdr.Get("X-Ca-Timestamp") == "" {
		t.Errorf("X-Ca-Timestamp empty")
	}
	if es.lastHdr.Get("Hik-Request-Id") != ClientName {
		t.Errorf("Hik-Request-Id = %q", es.lastHdr.Get("Hik-Request-Id"))
	}
}

func TestGetResponse_BinaryKeptOpen(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0xff, 0xaa})
	}))
	t.Cleanup(server.Close)
	u, _ := url.Parse(server.URL)
	cfg := NewConfig(u.Host, contextPath, "ak", "sk")
	resp, err := GetResponse(cfg, Path{Schema: u.Scheme + "://", Path: "/img"})
	if err != nil {
		t.Fatalf("GetResponse: %v", err)
	}
	defer resp.Close()
	if resp.ContentType != "image/png" {
		t.Errorf("Content-Type = %q", resp.ContentType)
	}
	data, err := resp.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if !bytes.Equal(data[:8], []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}) {
		t.Errorf("binary body not preserved: %x", data)
	}
}

func TestPostForm_Encoding(t *testing.T) {
	es := newEchoServer(t)
	cfg := newTestConfig(t, es)
	p := Path{Schema: "http://", Path: "/login"}
	body, err := PostForm(cfg, p, map[string]string{
		"user": "alice",
		"lang": "zh-CN",
	})
	if err != nil {
		t.Fatalf("PostForm: %v", err)
	}
	if !strings.Contains(body, `"path":"/login"`) {
		t.Errorf("path not echoed: %s", body)
	}
	if !strings.Contains(string(es.lastBody), "user=alice") {
		t.Errorf("form body missing user=alice: %q", string(es.lastBody))
	}
	ct := es.lastHdr.Get("Content-Type")
	if ct != ContentTypeForm {
		t.Errorf("Content-Type = %q, want %q", ct, ContentTypeForm)
	}
}

func TestPostString_DefaultContentType(t *testing.T) {
	es := newEchoServer(t)
	cfg := newTestConfig(t, es)
	body, err := PostString(cfg, Path{Schema: "http://", Path: "/echo"}, `{"a":1}`)
	if err != nil {
		t.Fatalf("PostString: %v", err)
	}
	if !strings.Contains(body, `"body":"{\"a\":1}"`) {
		t.Errorf("body not echoed: %s", body)
	}
	ct := es.lastHdr.Get("Content-Type")
	if ct != ContentTypeText {
		t.Errorf("Content-Type = %q, want %q", ct, ContentTypeText)
	}
}

func TestPostString_FormContentType(t *testing.T) {
	es := newEchoServer(t)
	cfg := newTestConfig(t, es)
	_, err := PostString(cfg, Path{Schema: "http://", Path: "/form"},
		"a=1&b=2",
		WithContentType(ContentTypeForm),
	)
	if err != nil {
		t.Fatalf("PostString: %v", err)
	}
	ct := es.lastHdr.Get("Content-Type")
	if ct != ContentTypeForm {
		t.Errorf("Content-Type = %q, want %q", ct, ContentTypeForm)
	}
}

func TestPostBytes_ContentMD5(t *testing.T) {
	es := newEchoServer(t)
	cfg := newTestConfig(t, es)
	payload := []byte("hello world")
	body, err := PostBytes(cfg, Path{Schema: "http://", Path: "/data"}, payload)
	if err != nil {
		t.Fatalf("PostBytes: %v", err)
	}
	if !strings.Contains(body, `"body":"hello world"`) {
		t.Errorf("body not echoed: %s", body)
	}
	md5 := es.lastHdr.Get("Content-MD5")
	want := Base64AndMD5(payload)
	if md5 != want {
		t.Errorf("Content-MD5 = %q, want %q", md5, want)
	}
}

func TestPostFile_Multipart(t *testing.T) {
	var receivedContentType string
	var received *multipart.Form
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedContentType = r.Header.Get("Content-Type")
		const maxMemory = 8 << 20
		_ = r.ParseMultipartForm(maxMemory)
		received = r.MultipartForm
		w.WriteHeader(200)
	}))
	t.Cleanup(srv.Close)
	u, _ := url.Parse(srv.URL)
	cfg := NewConfig(u.Host, contextPath, "ak", "sk")

	tmp, err := os.CreateTemp(t.TempDir(), "artemis-test-*.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tmp.WriteString("file content"); err != nil {
		t.Fatal(err)
	}
	tmp.Close()

	body, err := PostFileForm(cfg, Path{Schema: u.Scheme + "://", Path: "/upload"}, map[string]any{
		"file": tmp,
		"desc": "hello",
	})
	if err != nil {
		t.Fatalf("PostFileForm: %v", err)
	}
	_ = body
	if !strings.HasPrefix(receivedContentType, "multipart/form-data") {
		t.Errorf("Content-Type = %q", receivedContentType)
	}
	if !strings.Contains(receivedContentType, "boundary=") {
		t.Errorf("Content-Type missing boundary: %q", receivedContentType)
	}
	if received == nil {
		t.Fatalf("server did not parse multipart")
	}
	if v := received.Value["desc"]; len(v) == 0 || v[0] != "hello" {
		t.Errorf("desc = %v", v)
	}
	if v := received.File["file"]; len(v) == 0 {
		t.Errorf("file not received")
	} else {
		f, err := v[0].Open()
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		data, _ := io.ReadAll(f)
		if string(data) != "file content" {
			t.Errorf("file content = %q", data)
		}
	}
}

func TestPutString(t *testing.T) {
	es := newEchoServer(t)
	cfg := newTestConfig(t, es)
	_, err := PutString(cfg, Path{Schema: "http://", Path: "/p"}, "data")
	if err != nil {
		t.Fatalf("PutString: %v", err)
	}
	if es.lastHdr.Get("Content-MD5") != Base64AndMD5String("data") {
		t.Errorf("Content-MD5 = %q", es.lastHdr.Get("Content-MD5"))
	}
}

func TestPutBytes(t *testing.T) {
	es := newEchoServer(t)
	cfg := newTestConfig(t, es)
	payload := []byte{0x01, 0x02, 0x03}
	_, err := PutBytes(cfg, Path{Schema: "http://", Path: "/p"}, payload)
	if err != nil {
		t.Fatalf("PutBytes: %v", err)
	}
	if es.lastHdr.Get("Content-MD5") != Base64AndMD5(payload) {
		t.Errorf("Content-MD5 = %q", es.lastHdr.Get("Content-MD5"))
	}
}

func TestDelete(t *testing.T) {
	es := newEchoServer(t)
	cfg := newTestConfig(t, es)
	_, err := Delete(cfg, Path{Schema: "http://", Path: "/r"})
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
}

func TestPostDownloadFile_BodyPreserved(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Ca-Request-Id", "req-abc")
		_, _ = w.Write([]byte("binary data"))
	}))
	t.Cleanup(srv.Close)
	u, _ := url.Parse(srv.URL)
	cfg := NewConfig(u.Host, contextPath, "ak", "sk")
	resp, err := PostDownloadFile(cfg, Path{Schema: u.Scheme + "://", Path: "/d"}, "req-body")
	if err != nil {
		t.Fatalf("PostDownloadFile: %v", err)
	}
	defer resp.Close()
	data, _ := resp.ReadAll()
	if string(data) != "binary data" {
		t.Errorf("body = %q", data)
	}
	if resp.RequestID != "req-abc" {
		t.Errorf("RequestID = %q", resp.RequestID)
	}
}

func TestDispatchRequest_BadMethod(t *testing.T) {
	cfg := NewConfig("example.com:443", contextPath, "ak", "sk")
	req := &Request{Method: "BOGUS", Host: "http://x", Path: "/p"}
	if _, err := dispatchRequest(cfg, req); err == nil {
		t.Errorf("expected error for unsupported method")
	}
}

func TestInitURL_EdgeCases(t *testing.T) {
	cases := []struct {
		name   string
		host   string
		path   string
		querys map[string]any
		want   string
	}{
		{"empty query", "http://h", "/p", nil, "http://h/p"},
		{"empty path", "http://h", "", nil, "http://h"},
		{"string value", "http://h", "/p", map[string]any{"a": "1"}, "http://h/p?a=1"},
		{"int value", "http://h", "/p", map[string]any{"n": 1}, "http://h/p?n=1"},
		{"space encoded", "http://h", "/p", map[string]any{"a": "a b"}, "http://h/p?a=a+b"},
		{"blank key with value", "http://h", "/p", map[string]any{"": "v"}, "http://h/p?v"},
		{"blank key with nil", "http://h", "/p", map[string]any{"": nil}, "http://h/p"},
		{"sorted order", "http://h", "/p", map[string]any{"b": 2, "a": 1}, "http://h/p?a=1&b=2"},
	}
	for _, c := range cases {
		got, err := initURL(c.host, c.path, c.querys)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if got != c.want {
			t.Errorf("%s: initURL = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestApplyHeaders_HandlesAllKeys(t *testing.T) {
	headers := map[string]string{
		"X-Custom":   "v1",
		"x-ca-foo":   "v2",
		HeaderAccept: "*/*",
	}
	httpReq, _ := http.NewRequest("GET", "http://h", nil)
	applyHeaders(httpReq, headers)
	for k, v := range headers {
		if got := httpReq.Header.Get(k); got != v {
			t.Errorf("header %s = %q, want %q", k, got, v)
		}
	}
}

func TestBodysToStringMap(t *testing.T) {
	m := bodysToStringMap(map[string]any{
		"a":   "1",
		"n":   2,
		"nil": nil,
	})
	if m["a"] != "1" || m["n"] != "2" || m["nil"] != "" {
		t.Errorf("bodysToStringMap = %v", m)
	}
}

func TestAddMultipartField_Cases(t *testing.T) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	if err := addMultipartField(w, "k1", "v1"); err != nil {
		t.Fatal(err)
	}
	if err := addMultipartField(w, "k2", strings.NewReader("stream-data")); err != nil {
		t.Fatal(err)
	}
	if err := addMultipartField(w, "k3", 123); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	ct := w.FormDataContentType()
	r := multipart.NewReader(&buf, strings.SplitN(ct, "boundary=", 2)[1])
	for {
		part, err := r.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(part)
		switch part.FormName() {
		case "k1":
			if string(body) != "v1" {
				t.Errorf("k1 = %q", body)
			}
		case "k2":
			if string(body) != "stream-data" {
				t.Errorf("k2 = %q", body)
			}
		case "k3":
			if string(body) != "123" {
				t.Errorf("k3 = %q", body)
			}
		}
	}
}

// TestSignRequest_Recompute 验证 SDK 在发出请求时计算的签名能被独立复算。
//
// 服务端拿到的 X-Ca-Signature 是 SDK 在发送前算出来的；我们用同样的 headers 输入
// 重新跑一次 buildStringToSign + HMAC，必须得到同样的签名。注意：x-ca-signature 和
// x-ca-signature-headers 这两个 Header 不参与签名（前者不在签名头列表里，后者是签名
// 后的产物），所以重算输入里不能包含它们。
func TestSignRequest_Recompute(t *testing.T) {
	es := newEchoServer(t)
	cfg := newTestConfig(t, es)
	_, _ = Get(cfg, Path{Schema: "http://", Path: "/recompute"})

	got := es.lastHdr.Get("X-Ca-Signature")
	if got == "" {
		t.Fatalf("X-Ca-Signature empty")
	}
	hm := map[string]string{
		HeaderAccept:       es.lastHdr.Get("Accept"),
		HeaderContentType:  es.lastHdr.Get("Content-Type"),
		HeaderXCaKey:       es.lastHdr.Get("X-Ca-Key"),
		HeaderXCaNonce:     es.lastHdr.Get("X-Ca-Nonce"),
		HeaderXCaTimestamp: es.lastHdr.Get("X-Ca-Timestamp"),
	}
	mac := hmac.New(sha256.New, []byte("test-sk"))
	mac.Write([]byte(buildStringToSign("GET", "/recompute", hm, nil, nil, nil)))
	want := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	if got != want {
		t.Errorf("server-side signature mismatch:\n got=%s\nwant=%s", got, want)
	}
}

func TestPostFileForm_Reader(t *testing.T) {
	var received *multipart.Form
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseMultipartForm(8 << 20)
		received = r.MultipartForm
		w.WriteHeader(200)
	}))
	t.Cleanup(srv.Close)
	u, _ := url.Parse(srv.URL)
	cfg := NewConfig(u.Host, contextPath, "ak", "sk")

	_, err := PostFileForm(cfg, Path{Schema: u.Scheme + "://", Path: "/u"}, map[string]any{
		"file": strings.NewReader("reader-content"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if received == nil {
		t.Fatal("nil multipart form")
	}
	files := received.File["file"]
	if len(files) == 0 {
		t.Fatal("no file part")
	}
	f, _ := files[0].Open()
	defer f.Close()
	data, _ := io.ReadAll(f)
	if string(data) != "reader-content" {
		t.Errorf("file content = %q", data)
	}
}

func TestConfig_HTTPCache(t *testing.T) {
	cfg := NewConfig("h:1", contextPath, "a", "b")
	c1 := cfg.HTTPClient()
	c2 := cfg.HTTPClient()
	if c1 != c2 {
		t.Errorf("HTTPClient not cached")
	}
}

func TestUploadFile_RealFile(t *testing.T) {
	tmpDir := t.TempDir()
	src := filepath.Join(tmpDir, "data.txt")
	if err := os.WriteFile(src, []byte("real file content"), 0o600); err != nil {
		t.Fatal(err)
	}
	var got []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseMultipartForm(8 << 20)
		if files := r.MultipartForm.File["file"]; len(files) > 0 {
			f, _ := files[0].Open()
			got, _ = io.ReadAll(f)
			f.Close()
		}
		w.WriteHeader(200)
	}))
	t.Cleanup(srv.Close)
	u, _ := url.Parse(srv.URL)
	cfg := NewConfig(u.Host, contextPath, "ak", "sk")

	f, err := os.Open(src)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	_, err = PostFileForm(cfg, Path{Schema: u.Scheme + "://", Path: "/u"}, map[string]any{"file": f})
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "real file content" {
		t.Errorf("file content = %q", got)
	}
}

func TestConfig_TimeoutApplied(t *testing.T) {
	cfg := NewConfig("h:1", contextPath, "a", "b")
	cfg.SocketTimeout = 3 * time.Second
	c := cfg.HTTPClient()
	if c.Timeout != 3*time.Second {
		t.Errorf("client.Timeout = %v, want 3s", c.Timeout)
	}
}

// ---------------- Config.ContextPath 特性 ----------------

// TestJoinPath 验证 joinPath 的全部边界。
func TestJoinPath(t *testing.T) {
	cases := []struct {
		name string
		ctx  string
		path string
		want string
	}{
		{"空 ctx 透传 path", "", "/api/v1/x", "/api/v1/x"},
		{"空 ctx + 无前导斜杠 path", "", "api/v1/x", "api/v1/x"},
		{"典型 /artemis + /api", "/artemis", "/api/v1/x", "/artemis/api/v1/x"},
		{"ctx 末尾带 /", "/artemis/", "/api/v1/x", "/artemis/api/v1/x"},
		{"path 前无 /", "/artemis", "api/v1/x", "/artemis/api/v1/x"},
		{"ctx 末尾带 / 且 path 前无 /", "/artemis/", "api/v1/x", "/artemis/api/v1/x"},
		{"ctx 退化为 / 等于无 ctx", "/", "/api/v1/x", "/api/v1/x"},
		{"ctx 退化为 // 也等于无 ctx", "//", "/api/v1/x", "/api/v1/x"},
		{"ctx 多级", "/a/b", "/x/y", "/a/b/x/y"},
		{"ctx 非 / 开头", "artemis", "/api", "artemis/api"},
		{"path 为空时返回 ctx", "/artemis", "", "/artemis"},
		{"path 为空 + ctx 末尾带 /", "/artemis/", "", "/artemis"},
		{"双侧为空时返回空 path", "", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := joinPath(tc.ctx, tc.path)
			if got != tc.want {
				t.Errorf("joinPath(%q,%q) = %q, want %q", tc.ctx, tc.path, got, tc.want)
			}
		})
	}
}

// TestContextPath_PrependedToURL 验证 ContextPath 会被拼接到 URL 路径上。
func TestContextPath_PrependedToURL(t *testing.T) {
	es := newEchoServer(t)
	cfg := newTestConfig(t, es)
	cfg.ContextPath = "/artemis"
	p := Path{Schema: "http://", Path: "/api/eventService/v1/eventSubscriptionView"}

	respBody, err := Get(cfg, p)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if es.lastHdr == nil {
		t.Fatal("no headers captured")
	}
	// 通过解析 server echo 出的 path 字段验证（响应 body 是 echo JSON）
	if !strings.Contains(respBody, `"/artemis/api/eventService/v1/eventSubscriptionView"`) {
		t.Errorf("server did not see contextPath-prepended path; resp=%s", respBody)
	}
}

// TestContextPath_EmptyIsNoOp 验证 ContextPath 为空时行为与原先一致。
func TestContextPath_EmptyIsNoOp(t *testing.T) {
	es := newEchoServer(t)
	cfg := newTestConfig(t, es)
	// 显式不设置 ContextPath
	p := Path{Schema: "http://", Path: "/api/v1/x"}

	respBody, err := Get(cfg, p)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !strings.Contains(respBody, `"/api/v1/x"`) {
		t.Errorf("empty ContextPath should pass through; resp=%s", respBody)
	}
}

// TestContextPath_TrailingSlashAndLeadingSlash 验证双斜杠被归一化。
func TestContextPath_TrailingSlashAndLeadingSlash(t *testing.T) {
	es := newEchoServer(t)
	cfg := newTestConfig(t, es)
	cfg.ContextPath = "/artemis/" // 末尾带 /
	p := Path{Schema: "http://", Path: "/api/v1/x"}

	respBody, err := Get(cfg, p)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !strings.Contains(respBody, `"/artemis/api/v1/x"`) {
		t.Errorf("double slash not normalized; resp=%s", respBody)
	}
	// 且不应该出现 "//api"
	if strings.Contains(respBody, `"//api/v1/x"`) {
		t.Errorf("unexpected double slash in path; resp=%s", respBody)
	}
}

// TestContextPath_AffectsSignature 验证 req.Path 在签名时是拼接后的全路径。
//
// 复用 TestSignRequest_Recompute 的算法：r.Header 的键是规范大小写（"X-Ca-Key"
// 等），buildStringToSign 内部负责小写化。我们用拼接后的全路径重算应当得到与
// SDK 发出的 X-Ca-Signature 一致的结果。
func TestContextPath_AffectsSignature(t *testing.T) {
	es := newEchoServer(t)
	cfg := newTestConfig(t, es)
	cfg.AppKey = "ctx-ak"
	cfg.AppSecret = "ctx-sk"
	cfg.ContextPath = "/artemis"
	p := Path{Schema: "http://", Path: "/api/v1/x"}

	if _, err := Get(cfg, p); err != nil {
		t.Fatalf("Get: %v", err)
	}

	got := es.lastHdr.Get(HeaderXCaSignature)
	if got == "" {
		t.Fatal("X-Ca-Signature missing")
	}

	// 用 r.Header 的规范大小写键名（与 TestSignRequest_Recompute 一致）显式构造
	hm := map[string]string{
		HeaderAccept:       es.lastHdr.Get("Accept"),
		HeaderContentType:  es.lastHdr.Get("Content-Type"),
		HeaderXCaKey:       es.lastHdr.Get("X-Ca-Key"),
		HeaderXCaNonce:     es.lastHdr.Get("X-Ca-Nonce"),
		HeaderXCaTimestamp: es.lastHdr.Get("X-Ca-Timestamp"),
	}
	joined := joinPath("/artemis", "/api/v1/x")
	mac := hmac.New(sha256.New, []byte("ctx-sk"))
	mac.Write([]byte(buildStringToSign("GET", joined, hm, nil, nil, nil)))
	want := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	if got != want {
		t.Errorf("signature mismatch with ContextPath-joined path:\n got=%s\nwant=%s joined=%s", got, want, joined)
	}
}
