# artemis-go-sdk

artemis-sdk的 Go 语言客户端 SDK。

本 SDK 完整复刻 Java 版 `artemis-http-client 1.1.15` 的核心调用类 `ArtemisHttpUtil`：包括 HMAC-SHA256 签名算法、x-ca-\* 系列系统头、HTTP 多 Method 调用、multipart 文件上传、代理场景下的 `X-Ca-Path` 头覆盖等。命名、字段、签名规则与 Java 版保持一致，Java 用户可以无缝迁移。

## 安装

```bash
go get -U github.com/ljjdev/artemis-go-sdk
```

要求 Go 1.25 或更高版本。

## 快速开始

构造 `Config` 与 `Path` 即可发起一次调用：

- `NewConfig(host, contextPath, appKey, appSecret)`：网关地址、API 网关上下文路径（如 `/artemis`）、合作方 AK、合作方 SK。
- `Path{Schema, Path}`：`Schema` 取 `HTTPSchemaHTTP` 或 `HTTPSchemaHTTPS`；`Path` 传 **API 地址**（不含上下文），SDK 会自动把 `Config.contextPath` 拼接到 `Path.Path` 之前作为最终请求路径与签名路径。

### 示例 1：分页获取子组织列表（POST JSON）

调用 `POST /api/resource/v1/org/parentOrgIndexCode/subOrgList`，入参为 JSON 字符串。

```go
package main

import (
	"fmt"

	artemis "github.com/ljjdev/artemis-go-sdk"
)

func main() {
	const contextPath = "/artemis"

	cfg := artemis.NewConfig("127.0.0.1:9016", contextPath, "your-app-key", "your-app-secret")
	cfg.InsecureTLS = true // 跳过证书

	path := artemis.Path{Schema: artemis.HTTPSchemaHTTP, Path: "/api/resource/v1/org/parentOrgIndexCode/subOrgList"}
	body, err := artemis.PostString(cfg, path,
		`{"parentOrgIndexCode":"b0901bbb-24d6-46fa-b7ed-2c499ec57692","pageNo":1,"pageSize":10}`,
		artemis.WithContentType(artemis.ContentTypeJSON),
	)
	if err != nil {
		fmt.Println("err:", err)
		return
	}
	fmt.Println(body)
}
```

最终请求 URL：

```
http://127.0.0.1:9016/artemis/api/resource/v1/org/parentOrgIndexCode/subOrgList
```

### 示例 2：事件订阅视图查询（GET）

调用 `GET /api/eventService/v1/eventSubscriptionView`，附带 JSON 内容类型。

```go
package main

import (
	"fmt"

	artemis "github.com/ljjdev/artemis-go-sdk"
)

func main() {
	const contextPath = "/artemis"

	cfg := artemis.NewConfig("127.0.0.1:9016", contextPath, "your-app-key", "your-app-secret")
	cfg.InsecureTLS = true

	path := artemis.Path{Schema: artemis.HTTPSchemaHTTP, Path: "/api/eventService/v1/eventSubscriptionView"}
	body, err := artemis.Get(cfg, path,
		artemis.WithContentType(artemis.ContentTypeJSON),
	)
	if err != nil {
		fmt.Println("err:", err)
		return
	}
	fmt.Println(body)
}
```

最终请求 URL：

```
http://127.0.0.1:9016/artemis/api/eventService/v1/eventSubscriptionView
```

## 常用选项

通过 functional options 调整单次调用：

| Option                          | 说明                                     |
| ------------------------------- | -------------------------------------- |
| `WithAccept(mediaType)`         | 覆盖 `Accept` 头，默认 `*/*`                 |
| `WithContentType(mediaType)`    | 覆盖 `Content-Type` 头；签名时会按 Java 行为做相应处理 |
| `WithHeader(map[string]string)` | 注入额外请求头                                |
| `WithQuery(map[string]any)`     | 注入 URL query 参数                        |

## 运行示例

仓库内 [example\_test.go](./example_test.go) 包含两个集成示例（需要真实网关可达）：

- `TestSubOrgList`：分页获取子组织列表
- `TestEventSubscriptionView`：事件订阅视图查询

```bash
go test -run TestSubOrgList -v ./...
go test -run TestEventSubscriptionView -v ./...
```

