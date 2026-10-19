# dispatch 统一更新入口（schemaVersion 1）

客户端只配置一个 `/dispatch.json` 地址。dispatch 保留原 `servers` 数组，通过可选的 `update` 对象告知资源清单地址及各平台的应用更新信息。此 HTTP 元数据扩展不改变游戏登录、握手或其他游戏报文协议，也不把应用版本当作报文协议版本。

## 配置

默认不配置更新信息时，响应仍只有原来的 `servers`，字节结构保持一致。旧客户端可继续只解析 `servers`。新客户端如何处理缺少资源配置由客户端决定，服务端不会凭空生成可用资源或声称客户端可继续运行。

更新配置在启动时读取一次，以下方式二选一：

- 设置环境变量 `DISPATCH_UPDATE_CONFIG`，或用 `-update-config /absolute/path/update.json` 指定仓库外的 JSON 文件。命令行覆盖文件路径环境变量。
- 设置 `DISPATCH_UPDATE_JSON`，内容为同一个 JSON 对象。

文件内容是 `update` 对象本身，不包含顶层 `update` 或 `servers` 包装。配置文件必须是普通文件，最大 64 KiB。示例模板见 [`server/config/dispatch-update.example.json`](../server/config/dispatch-update.example.json)。模板地址和校验值不是可用下载内容，应在私有配置中换为真实值。不要把安装包、素材、私密配置或凭据提交到源码仓库。

若同时配置文件和内联 JSON、文件读取失败、JSON 含重复/未知字段或配置不符合下述契约，dispatch 在连接数据库和开始监听前退出；不会静默退回无更新模式。要关闭更新信息，清除两种来源后再启动。`-version` 仍可独立工作，不读取配置或连接数据库。此功能不自动重启运行中的服务。

dispatch 只校验配置格式并返回元数据，不下载任何清单或安装包，不验证远端文件是否存在，不安装、不运行清单中的内容。所有内容可用性和数据校验由对应消费者执行。

## 响应示例

```json
{
  "servers": [
    {"name":"怀旧一区","host":"192.168.1.2","port":19000,"zone":"龙城","pvp":false,"busy":0}
  ],
  "update": {
    "schemaVersion": 1,
    "resourceManifestUrl": "https://assets.example.test/resources/manifest.json",
    "resourceManifestSha256": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
    "resourceManifestSizeBytes": 456,
    "clients": {
      "android": {
        "version": "1.2.3",
        "minVersion": "1.0.0",
        "artifactUrl": "https://download.example.test/client.apk",
        "sha256": "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
        "sizeBytes": 12345
      }
    }
  }
}
```

示例摘要和大小仅用于说明 JSON 结构，不能用于真实文件校验。

## 字段约束

- `update` 可整体省略；存在时，`schemaVersion` 必须为整数 `1`，`resourceManifestUrl` 必填。
- `resourceManifestSha256` 与 `resourceManifestSizeBytes` 可同时省略；提供任意一个就必须两个一起提供。摘要是 64 位十六进制 SHA-256，大小范围为 1–8,388,608 字节（8 MiB）。它们描述下载到的资源清单原始字节；消费者应先核对大小和摘要再解析，校验失败不得退回无校验模式。
- 可选 `clients` 是按平台分组的对象，支持 `android`、`windows`、`macos`、`linux`。缺少某平台表示没有该平台的应用更新声明。每个存在的平台都必须具备 `version`、`artifactUrl`、`sha256`、`sizeBytes`；`minVersion` 可省略。安装包摘要必须为 64 位十六进制，大小必须为正的有符号 64 位整数。只发布版本号或 URL 而没有摘要、大小会被拒绝。
- `version` 和 `minVersion` 是应用版本，格式严格为 `MAJOR.MINOR.PATCH`，各段是 0–4,294,967,295 的十进制整数，除单独的 `0` 外不允许前导零。不接受 `v` 前缀、候选后缀或构建元数据。`minVersion` 不得大于 `version`；省略时不声明最低版本门槛。
- `hotupdateManifestUrl` 可省略，是未来热更新清单的地址预留。提供该字段不表示客户端已经支持热更新，也不授权下载、加载或执行代码。当前应用更新信息仅描述完整安装包。
- SHA-256 输入接受大小写，响应统一使用小写。除上述字段外不接受额外配置字段。

## URL 与信任边界

所有 URL 都必须是绝对地址，最长 4096 字节；禁止 userinfo、查询参数、fragment 和反斜线。默认要求 HTTPS。仅允许对字面量 RFC1918 IPv4（`10.0.0.0/8`、`172.16.0.0/12`、`192.168.0.0/16`）使用 HTTP，且客户端仍须显式启用开发模式。禁止 localhost、loopback 和未指定地址。不要把凭据或临时签名放进这些 URL。

这些校验不构成域名信任、证书验证、重定向批准或安装授权。客户端应只接受用户信任的 dispatch 地址，并执行自己的来源、重定向、大小、摘要和平台检查。来自同一 dispatch 的摘要提供完整性绑定，不是独立数字签名。无摘要的资源清单没有 dispatch 提供的完整性绑定；明文开发网络也没有 TLS 传输保证。

`/dispatch.json` 继续只向非本机请求开放 GET/HEAD，响应禁止缓存。公网/LAN 可访问路由白名单仍仅为 `/dispatch.json`、`/download/` 和 `/download/client.apk`；没有增加资源目录、配置文件、通用文件代理、管理控制台或目录 API。URL 元数据可以指向单独授权的资源服务，这不会使该路径在 dispatch 本身可访问。已有 8088 端口仍不应直接转发到公网。

## 测试

```sh
cd server
go test ./cmd/dispatch
```

测试覆盖无配置的精确旧响应、旧服务器列表解码器、有配置的完整响应、配置来源冲突/上限/重复字段、版本和 URL 拒绝规则、GET/HEAD 以及公开路由边界。测试 fixture 的域名和值仅用于解析契约，不触发网络下载。
