package artemis

import (
	"crypto/md5"
	"encoding/base64"
)

// Base64AndMD5 对输入字节先做 MD5 摘要，再做 Base64 编码，行为与 Java
// MessageDigestUtil.base64AndMD5(byte[]) 完全一致。
//
// 空输入返回空串（与 Java 实现不同——Java 会抛 IllegalArgumentException）；
// 这是有意为之，以适配 artemis 允许空 body 的场景。
func Base64AndMD5(data []byte) string {
	if len(data) == 0 {
		return ""
	}
	sum := md5.Sum(data)
	return base64.StdEncoding.EncodeToString(sum[:])
}

// Base64AndMD5String 是 Base64AndMD5 的字符串便捷版本。
func Base64AndMD5String(s string) string {
	if s == "" {
		return ""
	}
	return Base64AndMD5([]byte(s))
}
