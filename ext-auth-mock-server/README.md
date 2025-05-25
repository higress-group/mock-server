# Ext Auth Mock Server

> 模拟外部认证服务，专为 Higress [外部认证插件](https://higress.cn/docs/latest/plugins/authentication/ext-auth) e2e 测试设计。

## 接口说明

> `/always-200`、`/always-500`和`/require-request-body-200`接口适用于 ext-auth 插件中 endpoint_mode 为 **forward_auth** 的场景；
> `/prefix/always-200/`、`/prefix/always-500/`和`/prefix/require-request-body-200/`接口适用于 endpoint_mode 为 **envoy** 的场景。

### 1、`/always-200` 和 `/prefix/always-200/`

**功能**：无论请求如何，始终返回 HTTP 200 OK 响应。

**请求要求**：

- 必须包含 `Authorization` 请求头
- 不校验请求体是否存在

**响应**：
```json
{
  "message": "OK"
}
```

**响应头**：
- `X-User-ID`: "123456"
- `Content-Type`: "application/json"

### 2、 `/always-500` 和 `/prefix/always-500/`

**功能**：无论请求如何，始终返回 HTTP 500 Internal Server Error 响应。

**请求要求**：
- 必须包含 `Authorization` 请求头
- 不校验请求体是否存在

**响应**：
```json
{
  "error": "Internal Server Error"
}
```

**响应头**：
- `Content-Type`: "application/json"

### 3、 `/require-request-body-200` 和 `/prefix/require-request-body-200/`

**功能**：校验请求体是否存在，存在则返回 HTTP 200 OK，不存在则返回 HTTP 400 Bad Request。

**适用场景**：

专为测试 ext-auth 插件中 `authorization_request` 配置中 `with_request_body` 为 true 的场景设计，该配置要求请求必须包含请求体。

**请求要求**：

- 必须包含 `Authorization` 请求头
- 必须包含非空请求体（`Content-Length > 0`）

**成功响应**：
```json
{
  "message": "OK"
}
```

**成功响应头**：
- `X-User-ID`: "123456"
- `Content-Type`: "application/json"

**失败响应（请求体缺失）**：
```json
{
  "error": "Request body is required"
}
```

**失败响应头**：
- `X-Error-Reason`: "MissingRequestBody"
- `Content-Type`: "application/json"
- HTTP 状态码: 400 Bad Request


## 通用响应头

所有接口响应都会包含以下通用头部：
- `X-Mock-Server-Timestamp`: 服务器处理时间（UTC RFC3339 格式）
- `X-Server-Name`: "ext-auth-mock-server/1.0"


## 错误处理

当发生错误时，服务器会返回统一的错误响应格式：
```json
{
  "error": "错误描述信息"
}
```

常见错误类型：
| 错误描述 | HTTP 状态码 | X-Error-Reason |
|--------------------------|-------------|----------------|
| Missing required request header | 401 | MissingAuthorization |
| Request body is required | 400 | MissingRequestBody |
| Error reading request body | 500 | ServerError |

