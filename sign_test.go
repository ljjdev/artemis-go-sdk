package artemis

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"strings"
	"testing"
)

// TestBase64AndMD5 验证 MD5+Base64 与 Java MessageDigestUtil 等价。
func TestBase64AndMD5(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		// MD5("hello") = 5d41402abc4b2a76b9719d911017c592 → XUFAKrxLKna5cZ2REBfFkg==
		{"hello", "XUFAKrxLKna5cZ2REBfFkg=="},
		{"", ""},
		// MD5("a") = 0cc175b9c0f1b6a831c399e269772661 → DMF1ucDxtqgxw5niaXcmYQ==
		{"a", "DMF1ucDxtqgxw5niaXcmYQ=="},
	}
	for _, c := range cases {
		got := Base64AndMD5String(c.in)
		if got != c.want {
			t.Errorf("Base64AndMD5String(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestNewNonce 验证 UUID v4 格式。
func TestNewNonce(t *testing.T) {
	for i := 0; i < 5; i++ {
		s := newNonce()
		if len(s) != 36 {
			t.Fatalf("nonce length = %d, want 36", len(s))
		}
		if s[8] != '-' || s[13] != '-' || s[18] != '-' || s[23] != '-' {
			t.Fatalf("nonce %q missing dashes at expected positions", s)
		}
		if s[14] != '4' {
			t.Fatalf("nonce %q version nibble should be 4", s)
		}
	}
}

// TestIsHeaderToSign 覆盖关键分支。
//
// "Accept" 等标准 HTTP 头不会被 isHeaderToSign 选中，除非调用方显式把它加入
// signHeaderPrefixList（与 Java SignUtil.isHeaderToSign 一致）。
func TestIsHeaderToSign(t *testing.T) {
	prefixes := []string{"X-Custom", "x-other"}
	cases := []struct {
		name string
		want bool
	}{
		{"", false},
		{HeaderXCaPath, false},
		{HeaderXCaSignature, true},
		{HeaderXCaTimestamp, true},
		{HeaderXCaNonce, true},
		{HeaderAccept, false}, // 不在 signHeaderPrefixList 中，且非 x-ca- 前缀
		{"X-Custom", true},    // 大小写不敏感
		{"x-CUSTOM", true},
		{"X-Other", true},
		{"random", false},
	}
	for _, c := range cases {
		got := isHeaderToSign(c.name, prefixes)
		if got != c.want {
			t.Errorf("isHeaderToSign(%q) = %v, want %v", c.name, got, c.want)
		}
	}
}

// TestFormStringToMap 覆盖 "a=1&b=2" 与缺 key 场景。
func TestFormStringToMap(t *testing.T) {
	got := formStringToMap("a=1&b=2")
	if got["a"] != "1" || got["b"] != "2" || len(got) != 2 {
		t.Errorf("formStringToMap result = %v", got)
	}
	got = formStringToMap("a=hello%20world")
	if got["a"] != "hello world" {
		t.Errorf("decoded form value = %q, want %q", got["a"], "hello world")
	}
}

// TestSign_Stable 同输入必须得到同一签名（验证算法稳定）。
func TestSign_Stable(t *testing.T) {
	headers := map[string]string{
		HeaderAccept:       "*/*",
		HeaderContentType:  "application/text;charset=UTF-8",
		HeaderXCaKey:       "ak",
		HeaderXCaNonce:     "n",
		HeaderXCaTimestamp: "1",
	}
	s1 := Sign("secret", "GET", "/api/v1/test", headers, nil, nil, nil)
	s2 := Sign("secret", "GET", "/api/v1/test", headers, nil, nil, nil)
	if s1 != s2 {
		t.Errorf("Sign not stable: %q vs %q", s1, s2)
	}
	if s1 == "" {
		t.Errorf("Sign returned empty string")
	}
}

// TestSign_KnownVector 使用可手算的固定输入与等价 Java/Go 流程交叉验证。
//
// 输入 headers 包含 Accept、Content-Type、x-ca-key、x-ca-nonce、x-ca-timestamp、
// X-Custom-Header；signPrefixes 把 X-Custom-Header 纳入签名。
//
// buildStringToSign 的实际拼接顺序是 method → Accept → Content-Type →
// （按 key 升序）x-ca-* 与 X-Custom-Header → 资源段。
func TestSign_KnownVector(t *testing.T) {
	headers := map[string]string{
		HeaderAccept:       "*/*",
		HeaderContentType:  "application/text;charset=UTF-8",
		HeaderXCaKey:       "ak",
		HeaderXCaNonce:     "nonce-fixed",
		HeaderXCaTimestamp: "1700000000000",
		"X-Custom-Header":  "value1",
	}
	signPrefixes := []string{"X-Custom-Header"}

	wantSTS := "GET\n" +
		"*/*\n" +
		"application/text;charset=UTF-8\n" +
		"X-Custom-Header:value1\n" +
		"x-ca-key:ak\n" +
		"x-ca-nonce:nonce-fixed\n" +
		"x-ca-timestamp:1700000000000\n" +
		"/p?a=1&b=2"

	// 期望 stringToSign 与实际 stringToSign 一致
	gotSTS := buildStringToSign("GET", "/p", headers, map[string]any{"a": 1, "b": 2}, nil, signPrefixes)
	if gotSTS != wantSTS {
		t.Errorf("buildStringToSign mismatch:\n got=%s\nwant=%s", gotSTS, wantSTS)
	}

	// 对期望 stringToSign 算 HMAC 得到期望 sig
	mac := hmac.New(sha256.New, []byte("secret"))
	mac.Write([]byte(wantSTS))
	wantSig := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	got := Sign("secret", "GET", "/p", headers, map[string]any{"a": 1, "b": 2}, nil, signPrefixes)
	if got != wantSig {
		t.Errorf("Sign mismatch:\n got=%s\nwant=%s", got, wantSig)
	}

	// 校验 x-ca-signature-headers 写入：应包含 X-Custom-Header 和所有 x-ca-*（不含 x-ca-signature 自身）
	names := headers[HeaderXCaSignatureHeaders]
	for _, want := range []string{"X-Custom-Header", "x-ca-key", "x-ca-nonce", "x-ca-timestamp"} {
		if !strings.Contains(names, want) {
			t.Errorf("sign-headers missing %s: %q", want, names)
		}
	}
	if strings.Contains(names, HeaderXCaSignature) {
		t.Errorf("sign-headers should not contain x-ca-signature itself: %q", names)
	}
}

// TestSign_QueryOrdering 验证 query+body 合并后按 key 排序（每次调用使用全新 map）。
func TestSign_QueryOrdering(t *testing.T) {
	mkHeaders := func() map[string]string {
		return map[string]string{HeaderXCaKey: "k"}
	}
	sts1 := buildStringToSign("GET", "/p", mkHeaders(), map[string]any{"z": 1, "a": 2}, map[string]string{"m": "3"}, nil)
	sts2 := buildStringToSign("GET", "/p", mkHeaders(), map[string]any{"a": 2, "z": 1}, map[string]string{"m": "3"}, nil)
	if sts1 != sts2 {
		t.Errorf("ordering not stable:\n first=%s\nsecond=%s", sts1, sts2)
	}
	if !strings.HasSuffix(sts1, "/p?a=2&m=3&z=1") {
		t.Errorf("resource tail = %q, want /p?a=2&m=3&z=1", sts1)
	}
}

// TestSign_ContentTypeBoundary 验证 multipart 移除 boundary 子段。
func TestSign_ContentTypeBoundary(t *testing.T) {
	headers := map[string]string{
		HeaderContentType: "multipart/form-data;charset=UTF-8;boundary=----abc",
		HeaderXCaKey:      "k",
	}
	sts := buildStringToSign("POST", "/upload", headers, nil, nil, nil)
	if strings.Contains(sts, "boundary") {
		t.Errorf("boundary substring should be stripped, got: %q", sts)
	}
	if !strings.Contains(sts, "multipart/form-data;charset=UTF-8\n") {
		t.Errorf("content-type line should be multipart/form-data;charset=UTF-8, got: %q", sts)
	}
}

// TestSign_XCaPathOverride 验证 X-Ca-Path 覆盖签名用 path。
func TestSign_XCaPathOverride(t *testing.T) {
	headers := map[string]string{
		HeaderXCaKey:  "k",
		HeaderXCaPath: "/internal/real/path",
	}
	sts := buildStringToSign("GET", "/external/path", headers, nil, nil, nil)
	if !strings.HasSuffix(sts, "/internal/real/path") {
		t.Errorf("path not overridden by x-ca-path, got tail: %q", sts)
	}
}

// TestInjectSignatureHeaders 验证时间戳/Nonce/Key/Signature-Headers 全部就位。
func TestInjectSignatureHeaders(t *testing.T) {
	headers := map[string]string{}
	sig := InjectSignatureHeaders(headers, "ak", "secret", "GET", "/x", nil, nil, nil)
	if sig == "" {
		t.Fatalf("empty signature")
	}
	if headers[HeaderXCaKey] != "ak" {
		t.Errorf("x-ca-key not set: %q", headers[HeaderXCaKey])
	}
	if headers[HeaderXCaTimestamp] == "" {
		t.Errorf("x-ca-timestamp not set")
	}
	if headers[HeaderXCaNonce] == "" {
		t.Errorf("x-ca-nonce not set")
	}
	if headers[HeaderXCaSignatureHeaders] == "" {
		t.Errorf("x-ca-signature-headers not set")
	}
}
