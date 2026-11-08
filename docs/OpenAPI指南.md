# OpenAPI 接口契约

Swagger UI 使用 OpenAPI 规范展示和调试 HTTP API。GinChat 采用“契约优先”，不依赖 Handler 上的 Swagger 注解生成文档。

## 1. 文件

| 文件 | 用途 |
| --- | --- |
| `docs/swagger.yaml` | 人工维护的唯一事实来源 |
| `docs/swagger.json` | 由 YAML 机械生成，方便 Swagger UI、Postman、Apifox 直接导入 |
| `scripts/openapi/main.go` | YAML 转 JSON 工具 |

修改接口后：

```powershell
go run ./scripts/openapi
go test ./internal/router -run OpenAPI
```

第二项测试会检查：

- YAML 和 JSON 内容完全一致。
- OpenAPI 中每个操作都有唯一的 `operationId`。
- 所有本地 `$ref` 都能解析。
- OpenAPI 的路径、方法和 Gin 实际注册路由完全一致。

因此新增或删除接口却忘记更新文档时，CI 会直接失败。

## 2. 查看和联调

可以任选一种方式：

1. 打开 Swagger Editor，导入 `docs/swagger.yaml`。
2. 在 Apifox 或 Postman 中导入 `docs/swagger.json`。
3. 在 IDE 的 OpenAPI/Swagger 插件中打开 YAML。

`servers` 默认指向 `http://127.0.0.1:8081`。Bearer Token 通过 `bearerAuth` 安全方案填写。

WebSocket 升级后的双向事件不属于 OpenAPI 能完整表达的 HTTP 请求，仍以 [WebSocket 协议](./WebSocket协议.md) 为准。

## 3. 为什么不用 Swag 注解生成

注解驱动 Swagger 在大型团队和代码优先项目中很常见，但本项目不采用，原因是：

- 请求和响应字段已经有独立的 DTO，再写一遍注解容易漂移。
- Handler 应保持业务可读性，不应混合几十行文档元数据。
- 路由覆盖错误往往不会因为注释存在而被发现。
- 当前项目没有运行时 Swagger UI 需求，前端和面试演示更需要稳定、可校验的契约文件。

Swagger 注解本身并不“落后”，它适合注解已经成熟、需要自动生成大量文档的团队。这里选择契约优先，是为了减少重复和过期风险。

## 4. 代码注释规范

- 所有导出的 Go 类型、函数和方法写简洁的 Go Doc，说明职责和边界。
- 复杂事务、并发、失败恢复和不变量使用阶段中文注释。
- DTO 业务字段说明用途；协议字段使用英文标识符。
- 不再维护旧版 `@Summary`、`@Router` 等 Swag 注解。
- 普通网页接口说明继续保留在 `docs/*.md`，复杂语义不应只写进 OpenAPI 描述。
