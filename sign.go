package artemis

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Sign 计算签名（HMAC-SHA256 + Base64），与 Java SignUtil.sign 等价。
//
// 该函数会向 headers 中写入 "x-ca-signature-headers" 字段，调用方应在拿到返回值后
// 自行把 "x-ca-signature" 注入到 headers（避免签名自引用）。
func Sign(secret, method, path string,
	headers map[string]string,
	querys map[string]any,
	bodys map[string]string,
	signHeaderPrefixList []string) string {

	// 清理上一次可能残留的 x-ca-signature-headers，避免在多次复用同一 map 时自引用。
	delete(headers, HeaderXCaSignatureHeaders)

	stringToSign := buildStringToSign(method, path, headers, querys, bodys, signHeaderPrefixList)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(stringToSign))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

// InjectSignatureHeaders 在 headers 中注入 artemis 必需的签名 Header（不含 x-ca-signature 自身），
// 并返回应填入 x-ca-signature 的值。
func InjectSignatureHeaders(headers map[string]string, appKey, secret, method, path string,
	querys map[string]any, bodys map[string]string, signHeaderPrefixList []string) string {

	headers[HeaderXCaTimestamp] = strconv.FormatInt(time.Now().UnixMilli(), 10)
	headers[HeaderXCaNonce] = newNonce()
	headers[HeaderXCaKey] = appKey
	return Sign(secret, method, path, headers, querys, bodys, signHeaderPrefixList)
}

// buildStringToSign 复刻 Java SignUtil.buildStringToSign 的字节序列。
//
// 顺序：
//  1. METHOD\n
//  2. Accept\n（若存在）
//  3. Content-MD5\n（若存在）
//  4. Content-Type\n（若存在；multipart 需去除 boundary 子段）
//  5. Date\n（若存在）
//  6. 参与签名的 Header（key 升序，name:value\n）
//  7. path?key=value&key=value...（query + body 合并后按 key 升序）
func buildStringToSign(method, path string,
	headers map[string]string,
	querys map[string]any,
	bodys map[string]string,
	signHeaderPrefixList []string) string {

	var sb strings.Builder
	sb.WriteString(strings.ToUpper(method))
	sb.WriteString(LF)

	if headers != nil {
		if v, ok := headers[HeaderAccept]; ok {
			sb.WriteString(v)
			sb.WriteString(LF)
		}
		if v, ok := headers[HeaderContentMD5]; ok {
			sb.WriteString(v)
			sb.WriteString(LF)
		}
		if v, ok := headers[HeaderContentType]; ok {
			contentType := v
			if strings.Contains(contentType, BoundaryKeyword) {
				// multipart/form-data 需去除 boundary 子段。
				parts := strings.Split(contentType, SepSemicolon)
				var ctBuf strings.Builder
				for _, p := range parts {
					if strings.Contains(p, BoundaryKeyword) {
						continue
					}
					ctBuf.WriteString(p)
					ctBuf.WriteString(SepSemicolon)
				}
				sb.WriteString(strings.TrimSuffix(ctBuf.String(), SepSemicolon))
			} else {
				sb.WriteString(contentType)
			}
			sb.WriteString(LF)
		}
		if v, ok := headers[HeaderDate]; ok {
			sb.WriteString(v)
			sb.WriteString(LF)
		}
		// x-ca-path 覆盖签名 path（代理场景）。
		if v, ok := headers[HeaderXCaPath]; ok && v != "" {
			path = v
		}
	}

	sb.WriteString(buildSignedHeaders(headers, signHeaderPrefixList))
	sb.WriteString(buildResource(path, querys, bodys))
	return sb.String()
}

// buildResource 拼接 path?key=value&...，key 按字典序升序。
func buildResource(path string, querys map[string]any, bodys map[string]string) string {
	var sb strings.Builder
	if path != "" {
		sb.WriteString(path)
	}

	type kv struct {
		key string
		val string
	}
	var merged []kv
	if querys != nil {
		for k, v := range querys {
			if k == "" {
				continue
			}
			merged = append(merged, kv{k, toString(v)})
		}
	}
	if bodys != nil {
		for k, v := range bodys {
			if k == "" {
				continue
			}
			merged = append(merged, kv{k, v})
		}
	}
	sort.Slice(merged, func(i, j int) bool { return merged[i].key < merged[j].key })

	if len(merged) > 0 {
		sb.WriteString(SepQuestion)
		for i, item := range merged {
			if i > 0 {
				sb.WriteString(SepAmp)
			}
			sb.WriteString(item.key)
			sb.WriteString(SepEqual)
			sb.WriteString(item.val)
		}
	}
	return sb.String()
}

// buildSignedHeaders 拼接待签名的 Header 段，并把签名 Header 名列表回写到 headers。
func buildSignedHeaders(headers map[string]string, signHeaderPrefixList []string) string {
	// 1) 过滤 signHeaderPrefixList 中固定不应参与签名的项，剩余项排序。
	filtered := make([]string, 0, len(signHeaderPrefixList))
	for _, p := range signHeaderPrefixList {
		if p == HeaderXCaSignature ||
			p == HeaderAccept ||
			p == HeaderContentMD5 ||
			p == HeaderContentType ||
			p == HeaderDate {
			continue
		}
		filtered = append(filtered, p)
	}
	sort.Strings(filtered)

	if headers == nil {
		return ""
	}

	// 2) 按 key 升序遍历 headers，把参与签名的项拼成 name:value\n。
	keys := make([]string, 0, len(headers))
	for k := range headers {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var sb strings.Builder
	var signNames strings.Builder
	for _, k := range keys {
		if !isHeaderToSign(k, filtered) {
			continue
		}
		sb.WriteString(k)
		sb.WriteString(SepColon)
		if v := headers[k]; v != "" {
			sb.WriteString(v)
		}
		sb.WriteString(LF)
		if signNames.Len() > 0 {
			signNames.WriteString(SepComma)
		}
		signNames.WriteString(k)
	}
	headers[HeaderXCaSignatureHeaders] = signNames.String()
	return sb.String()
}

// isHeaderToSign 判断 headerName 是否参与签名，对应 Java SignUtil.isHeaderToSign。
//
//   - 空返回 false
//   - x-ca-path 永远不参与签名（用于覆盖 path，本身不计入签名段）
//   - 以 x-ca- 开头返回 true
//   - 与 signHeaderPrefixList 中任意项（大小写不敏感）相等返回 true
func isHeaderToSign(headerName string, signHeaderPrefixList []string) bool {
	if headerName == "" {
		return false
	}
	if headerName == HeaderXCaPath {
		return false
	}
	if strings.HasPrefix(headerName, HeaderSignPrefix) {
		return true
	}
	for _, p := range signHeaderPrefixList {
		if strings.EqualFold(headerName, p) {
			return true
		}
	}
	return false
}

// toString 把 any 转为签名用的字符串（与 Java String.valueOf 等价）。
func toString(v any) string {
	if v == nil {
		return ""
	}
	switch s := v.(type) {
	case string:
		return s
	case []byte:
		return string(s)
	case fmt.Stringer:
		return s.String()
	}
	return fmt.Sprintf("%v", v)
}

// newNonce 生成一个 UUID v4 字符串，行为与 Java UUID.randomUUID().toString() 等价。
func newNonce() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// 极低概率：rand 不可用则退化为时间戳。
		return strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	b[6] = (b[6] & 0x0F) | 0x40 // version 4
	b[8] = (b[8] & 0x3F) | 0x80 // variant 10
	dst := make([]byte, 36)
	hex.Encode(dst[0:8], b[0:4])
	dst[8] = '-'
	hex.Encode(dst[9:13], b[4:6])
	dst[13] = '-'
	hex.Encode(dst[14:18], b[6:8])
	dst[18] = '-'
	hex.Encode(dst[19:23], b[8:10])
	dst[23] = '-'
	hex.Encode(dst[24:36], b[10:16])
	return string(dst)
}

// formStringToMap 把 "a=1&b=2" 形式的字符串解析为 map，用于 POST_STRING
// Content-Type=application/x-www-form-urlencoded 时的签名回退（对齐 Java HttpUtil.strToMap）。
func formStringToMap(s string) map[string]string {
	out := make(map[string]string)
	if s == "" {
		return out
	}
	for _, kv := range strings.Split(s, SepAmp) {
		idx := strings.IndexByte(kv, '=')
		if idx < 0 {
			if kv != "" {
				out[url.QueryEscape(kv)] = ""
			}
			continue
		}
		k, _ := url.QueryUnescape(kv[:idx])
		v, _ := url.QueryUnescape(kv[idx+1:])
		out[k] = v
	}
	return out
}
