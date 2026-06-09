package artemis

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// dispatchRequest 是 artemis.go 与 HTTP 实现的统一入口。
//
// 流程：
//  1. 注入公共 Header（Hik-Request-ID、User-Agent）
//  2. 按 Method 分发到具体方法；具体方法在构造完 body / 设定 Content-Type 后自行签名
//  3. 复用 cfg.HTTPClient() 发送请求
//
// 返回 *Response 包含响应体字符串（下载场景为原始 ReadCloser）。
func dispatchRequest(cfg *Config, req *Request) (*Response, error) {
	if cfg == nil {
		return nil, errors.New("artemis: nil config")
	}
	if req == nil {
		return nil, errors.New("artemis: nil request")
	}
	if req.Headers == nil {
		req.Headers = make(map[string]string)
	}
	req.Headers[HeaderHikRequestID] = ClientName
	if _, ok := req.Headers[HeaderUserAgent]; !ok {
		req.Headers[HeaderUserAgent] = UserAgent
	}
	if req.Path == "" {
		return nil, fmt.Errorf("artemis: missing path")
	}

	switch req.Method {
	case MethodGet:
		return httpGet(cfg, req)
	case MethodGetResponse:
		return httpGetResponse(cfg, req)
	case MethodPostForm:
		return httpPostForm(cfg, req)
	case MethodPostFormResponse:
		return httpPostFormResponse(cfg, req)
	case MethodPostString:
		return httpPostString(cfg, req)
	case MethodPostStringResponse:
		return httpPostStringResponse(cfg, req)
	case MethodPostBytes:
		return httpPostBytes(cfg, req)
	case MethodPostFile:
		return httpPostFile(cfg, req)
	case MethodPutString:
		return httpPutString(cfg, req)
	case MethodPutBytes:
		return httpPutBytes(cfg, req)
	case MethodDelete:
		return httpDelete(cfg, req)
	case MethodPostDownload:
		return httpPostDownload(cfg, req)
	default:
		return nil, fmt.Errorf("artemis: unsupported method: %s", req.Method)
	}
}

// httpGet 文本 GET。返回 body 字符串。
func httpGet(cfg *Config, req *Request) (*Response, error) {
	signRequest(req, nil)
	return doTextRequest(cfg, req, HTTPMethodGET, nil)
}

// httpGetResponse 二进制 GET（图片/文件下载）。保留 resp.Body。
func httpGetResponse(cfg *Config, req *Request) (*Response, error) {
	signRequest(req, nil)
	return doRawRequest(cfg, req, HTTPMethodGET, nil)
}

// httpPostForm 文本 POST（application/x-www-form-urlencoded）。
func httpPostForm(cfg *Config, req *Request) (*Response, error) {
	if _, ok := req.Headers[HeaderContentType]; !ok {
		req.Headers[HeaderContentType] = ContentTypeForm
	}
	form := bodysToURLValues(req.Bodys)
	body := strings.NewReader(form.Encode())
	signRequest(req, bodysToStringMap(req.Bodys))
	fullURL, err := buildFullURL(req)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(context.Background(), HTTPMethodPOST, fullURL, body)
	if err != nil {
		return nil, err
	}
	applyHeaders(httpReq, req.Headers)
	httpReq.Header.Set(HeaderContentType, ContentTypeForm)
	resp, err := cfg.HTTPClient().Do(httpReq)
	if err != nil {
		return nil, err
	}
	return convertResponse(resp, false)
}

// httpPostFormResponse 表单 POST 的二进制下载变体。
func httpPostFormResponse(cfg *Config, req *Request) (*Response, error) {
	if _, ok := req.Headers[HeaderContentType]; !ok {
		req.Headers[HeaderContentType] = ContentTypeForm
	}
	form := bodysToURLValues(req.Bodys)
	body := strings.NewReader(form.Encode())
	signRequest(req, bodysToStringMap(req.Bodys))
	fullURL, err := buildFullURL(req)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(context.Background(), HTTPMethodPOST, fullURL, body)
	if err != nil {
		return nil, err
	}
	applyHeaders(httpReq, req.Headers)
	httpReq.Header.Set(HeaderContentType, ContentTypeForm)
	resp, err := cfg.HTTPClient().Do(httpReq)
	if err != nil {
		return nil, err
	}
	return convertResponse(resp, true)
}

// httpPostString 文本 POST（application/text 或 application/x-www-form-urlencoded）。
//
// Java 版特殊处理：当 Content-Type 为 form 时，把 body 拆为 map 用于签名；
// 否则用 null。Go 端复刻此行为。
func httpPostString(cfg *Config, req *Request) (*Response, error) {
	if _, ok := req.Headers[HeaderContentType]; !ok {
		req.Headers[HeaderContentType] = ContentTypeText
	}
	ct := req.Headers[HeaderContentType]
	var signBodys map[string]string
	if ct == ContentTypeForm {
		signBodys = formStringToMap(req.StringBody)
	}
	signRequest(req, signBodys)
	fullURL, err := buildFullURL(req)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(context.Background(), HTTPMethodPOST, fullURL, strings.NewReader(req.StringBody))
	if err != nil {
		return nil, err
	}
	applyHeaders(httpReq, req.Headers)
	httpReq.Header.Set(HeaderContentType, ct)
	resp, err := cfg.HTTPClient().Do(httpReq)
	if err != nil {
		return nil, err
	}
	return convertResponse(resp, false)
}

// httpPostStringResponse 文本 POST 的二进制下载变体。
func httpPostStringResponse(cfg *Config, req *Request) (*Response, error) {
	if _, ok := req.Headers[HeaderContentType]; !ok {
		req.Headers[HeaderContentType] = ContentTypeText
	}
	ct := req.Headers[HeaderContentType]
	var signBodys map[string]string
	if ct == ContentTypeForm {
		signBodys = formStringToMap(req.StringBody)
	}
	signRequest(req, signBodys)
	fullURL, err := buildFullURL(req)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(context.Background(), HTTPMethodPOST, fullURL, strings.NewReader(req.StringBody))
	if err != nil {
		return nil, err
	}
	applyHeaders(httpReq, req.Headers)
	httpReq.Header.Set(HeaderContentType, ct)
	resp, err := cfg.HTTPClient().Do(httpReq)
	if err != nil {
		return nil, err
	}
	return convertResponse(resp, true)
}

// httpPostBytes 字节数组 POST。若 body 非空，写入 Content-MD5。
func httpPostBytes(cfg *Config, req *Request) (*Response, error) {
	if _, ok := req.Headers[HeaderContentType]; !ok {
		req.Headers[HeaderContentType] = ContentTypeText
	}
	if len(req.BytesBody) > 0 {
		req.Headers[HeaderContentMD5] = Base64AndMD5(req.BytesBody)
	}
	signRequest(req, nil)
	fullURL, err := buildFullURL(req)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(context.Background(), HTTPMethodPOST, fullURL, bytes.NewReader(req.BytesBody))
	if err != nil {
		return nil, err
	}
	applyHeaders(httpReq, req.Headers)
	resp, err := cfg.HTTPClient().Do(httpReq)
	if err != nil {
		return nil, err
	}
	return convertResponse(resp, false)
}

// httpPostFile multipart/form-data 文件上传。body 不参与签名。
func httpPostFile(cfg *Config, req *Request) (*Response, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for k, v := range req.Bodys {
		if err := addMultipartField(w, k, v); err != nil {
			return nil, err
		}
	}
	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("artemis: close multipart writer: %w", err)
	}
	// Content-Type 需要在签名之前写入 headers，签名时会去除 boundary 子段。
	req.Headers[HeaderContentType] = ContentTypeFileForm + SepSemicolon + "boundary=" + w.Boundary()
	signRequest(req, nil) // multipart body 不参与签名
	fullURL, err := buildFullURL(req)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(context.Background(), HTTPMethodPOST, fullURL, &buf)
	if err != nil {
		return nil, err
	}
	applyHeaders(httpReq, req.Headers)
	resp, err := cfg.HTTPClient().Do(httpReq)
	if err != nil {
		return nil, err
	}
	return convertResponse(resp, false)
}

// httpPutString 文本 PUT。
func httpPutString(cfg *Config, req *Request) (*Response, error) {
	if _, ok := req.Headers[HeaderContentType]; !ok {
		req.Headers[HeaderContentType] = ContentTypeText
	}
	if req.StringBody != "" {
		req.Headers[HeaderContentMD5] = Base64AndMD5String(req.StringBody)
	}
	signRequest(req, nil)
	fullURL, err := buildFullURL(req)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(context.Background(), HTTPMethodPUT, fullURL, strings.NewReader(req.StringBody))
	if err != nil {
		return nil, err
	}
	applyHeaders(httpReq, req.Headers)
	resp, err := cfg.HTTPClient().Do(httpReq)
	if err != nil {
		return nil, err
	}
	return convertResponse(resp, false)
}

// httpPutBytes 字节 PUT。
func httpPutBytes(cfg *Config, req *Request) (*Response, error) {
	if _, ok := req.Headers[HeaderContentType]; !ok {
		req.Headers[HeaderContentType] = ContentTypeText
	}
	if len(req.BytesBody) > 0 {
		req.Headers[HeaderContentMD5] = Base64AndMD5(req.BytesBody)
	}
	signRequest(req, nil)
	fullURL, err := buildFullURL(req)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(context.Background(), HTTPMethodPUT, fullURL, bytes.NewReader(req.BytesBody))
	if err != nil {
		return nil, err
	}
	applyHeaders(httpReq, req.Headers)
	resp, err := cfg.HTTPClient().Do(httpReq)
	if err != nil {
		return nil, err
	}
	return convertResponse(resp, false)
}

// httpDelete DELETE。
func httpDelete(cfg *Config, req *Request) (*Response, error) {
	signRequest(req, nil)
	return doTextRequest(cfg, req, HTTPMethodDELETE, nil)
}

// httpPostDownload 与 httpPostString 类似，但保留 resp.Body 不读取。
func httpPostDownload(cfg *Config, req *Request) (*Response, error) {
	if _, ok := req.Headers[HeaderContentType]; !ok {
		req.Headers[HeaderContentType] = ContentTypeText
	}
	ct := req.Headers[HeaderContentType]
	var signBodys map[string]string
	if ct == ContentTypeForm {
		signBodys = formStringToMap(req.StringBody)
	}
	signRequest(req, signBodys)
	fullURL, err := buildFullURL(req)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(context.Background(), HTTPMethodPOST, fullURL, strings.NewReader(req.StringBody))
	if err != nil {
		return nil, err
	}
	applyHeaders(httpReq, req.Headers)
	httpReq.Header.Set(HeaderContentType, ct)
	resp, err := cfg.HTTPClient().Do(httpReq)
	if err != nil {
		return nil, err
	}
	return convertResponse(resp, true)
}

// doTextRequest 是无 body 或纯字符串 body 的请求的统一执行器。
func doTextRequest(cfg *Config, req *Request, method string, body io.Reader) (*Response, error) {
	fullURL, err := buildFullURL(req)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(context.Background(), method, fullURL, body)
	if err != nil {
		return nil, err
	}
	applyHeaders(httpReq, req.Headers)
	resp, err := cfg.HTTPClient().Do(httpReq)
	if err != nil {
		return nil, err
	}
	return convertResponse(resp, false)
}

// doRawRequest 是下载场景（保留 resp.Body）的统一执行器。
func doRawRequest(cfg *Config, req *Request, method string, body io.Reader) (*Response, error) {
	fullURL, err := buildFullURL(req)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(context.Background(), method, fullURL, body)
	if err != nil {
		return nil, err
	}
	applyHeaders(httpReq, req.Headers)
	resp, err := cfg.HTTPClient().Do(httpReq)
	if err != nil {
		return nil, err
	}
	return convertResponse(resp, true)
}

// signRequest 在 req.Headers 上完成 x-ca-* 注入并写入签名。
//
// signBodys 是当前 Method 应当参与签名的表单键值对（map[string]string 形式）；
// 传入 nil 表示该 Method 不需要把 body 纳入签名。
func signRequest(req *Request, signBodys map[string]string) {
	method := normalizeMethod(req.Method)
	sig := InjectSignatureHeaders(req.Headers, req.AppKey, req.AppSecret, method, req.Path, req.Querys, signBodys, req.SignHeaderPrefixList)
	req.Headers[HeaderXCaSignature] = sig
}

// normalizeMethod 去除 Method 名字中 "_" 之后的部分（RESPONSE / DOWNLOAD 等变体），
// 签名只关心 HTTP 方法本体（GET/POST/PUT/DELETE）。
func normalizeMethod(m Method) string {
	s := strings.ToUpper(string(m))
	if i := strings.IndexByte(s, '_'); i >= 0 {
		s = s[:i]
	}
	return s
}

// buildFullURL 用 req.Host + req.Path + req.Querys 拼出最终 URL。
func buildFullURL(req *Request) (string, error) {
	return initURL(req.Host, req.Path, req.Querys)
}

// applyHeaders 把 req.Headers 复制到 httpReq.Header。
func applyHeaders(httpReq *http.Request, headers map[string]string) {
	for k, v := range headers {
		httpReq.Header.Set(k, v)
	}
}

// convertResponse 把 *http.Response 转成 *Response。
//
// keepBody=true 时不读取 body，resp.Body 由调用方负责 Close。
func convertResponse(resp *http.Response, keepBody bool) (*Response, error) {
	if resp == nil {
		return nil, errors.New("artemis: nil http response")
	}
	r := &Response{
		StatusCode: resp.StatusCode,
		Headers:    make(map[string]string, len(resp.Header)),
		raw:        resp,
	}
	for k, v := range resp.Header {
		if len(v) > 0 {
			r.Headers[k] = v[0]
		}
	}
	r.ContentType = r.Headers[HeaderContentType]
	r.RequestID = r.Headers[HeaderXCaRequestID]
	r.ErrorMessage = r.Headers[HeaderXCaErrorMessage]
	if keepBody {
		r.RawBody = resp.Body
		return r, nil
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("artemis: read response body: %w", err)
	}
	r.Body = string(data)
	return r, nil
}

// initURL 复刻 Java HttpUtil.initUrl 的行为：value 经 URLEncoder.encode(value, UTF-8)。
func initURL(host, path string, querys map[string]any) (string, error) {
	var sb strings.Builder
	sb.WriteString(host)
	if path != "" {
		sb.WriteString(path)
	}
	if len(querys) == 0 {
		return sb.String(), nil
	}
	keys := make([]string, 0, len(querys))
	for k := range querys {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var qb strings.Builder
	for _, k := range keys {
		v := querys[k]
		if qb.Len() > 0 {
			qb.WriteString(SepAmp)
		}
		if k == "" && v != nil {
			qb.WriteString(toString(v))
			continue
		}
		if k != "" {
			qb.WriteString(k)
			if v != nil {
				qb.WriteString(SepEqual)
				qb.WriteString(url.QueryEscape(toString(v)))
			}
		}
	}
	if qb.Len() > 0 {
		sb.WriteString(SepQuestion)
		sb.WriteString(qb.String())
	}
	return sb.String(), nil
}

// bodysToURLValues 把 Request.Bodys (map[string]any) 转为 url.Values 供表单编码。
func bodysToURLValues(bodys map[string]any) url.Values {
	out := make(url.Values, len(bodys))
	for k, v := range bodys {
		out.Set(k, toString(v))
	}
	return out
}

// bodysToStringMap 把 Request.Bodys (map[string]any) 转为 map[string]string 供签名。
func bodysToStringMap(bodys map[string]any) map[string]string {
	out := make(map[string]string, len(bodys))
	for k, v := range bodys {
		out[k] = toString(v)
	}
	return out
}

// addMultipartField 按 Java HttpUtil.buildFormHttpEntity 的类型分支写入 multipart part。
func addMultipartField(w *multipart.Writer, name string, value any) error {
	switch v := value.(type) {
	case nil:
		return w.WriteField(name, "")
	case string:
		return w.WriteField(name, v)
	case *os.File:
		f, err := os.Open(v.Name())
		if err != nil {
			return fmt.Errorf("artemis: open file %q: %w", v.Name(), err)
		}
		defer f.Close()
		fw, err := w.CreateFormFile(name, filepath.Base(v.Name()))
		if err != nil {
			return err
		}
		_, err = io.Copy(fw, f)
		return err
	case io.Reader:
		fw, err := w.CreateFormFile(name, "blob")
		if err != nil {
			return err
		}
		_, err = io.Copy(fw, v)
		return err
	}
	return w.WriteField(name, toString(value))
}
