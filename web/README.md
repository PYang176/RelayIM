# GinChat Web

## 技术栈

- Vue 3.5
- TypeScript
- Vite
- Vue Router
- Pinia

## 目录规范

```text
src/
├── api/          接口函数和 HTTP 封装
├── components/   可复用业务组件
├── router/       路由和登录守卫
├── stores/       跨页面 Pinia 状态
├── styles/       全局样式
├── types/        前后端协议类型
└── views/        页面级组件
```

- 页面组件不直接调用 `fetch`。
- Snowflake ID 全部使用字符串。
- WebSocket 建立前申请一次性 Ticket。
- 收到消息后发送 ACK；阅读后发送 Read。
- 断线重连后使用 `after_seq` 补拉。
- 登录后保存 Access Token、Refresh Token 和设备 ID；启动时 Access Token 失效会尝试刷新。
- 用户可以编辑昵称、头像地址和个人简介，保存后立即更新当前登录状态。
- 退出登录会调用服务端撤销 Access Token 和 Refresh Token，而不只是清理 localStorage。
- 好友抽屉支持搜索、申请、同意、拒绝、备注和打开私聊。
- 创建群聊时可以从好友列表多选成员，群主和受邀成员在同一事务中加入。
- 群会话显示有效成员和群主标识，成员才能读取成员列表。
- 群主可继续邀请好友、移除普通成员；普通成员可以主动退出群聊。
- 群主可以修改群名称与群头像，并完成管理员、群主转让和解散群操作。
- 页面不展示用户或会话 Snowflake ID，私聊标题使用对方昵称。
- 私聊和群聊共用文字、图片、文件、实时消息、历史补拉和已读链路。
- 消息输入区支持“图片/文件”，单文件最大 25 MB。
- 图片消息显示预览，普通文件显示下载卡片。
- 附件通过短时预签名地址加载，刷新历史后仍可恢复。

## 本地启动

先启动 Go 后端、MySQL、Redis 和 RabbitMQ：

```powershell
cd "E:\Develop Projects\golang projects\ginChatProject"
.\scripts\run-server.ps1
```

再启动前端：

```powershell
cd "E:\Develop Projects\golang projects\ginChatProject\web"
npm install
npm run dev
```

浏览器访问：

```text
http://127.0.0.1:5173
```

Vite 将 `/api` 和 `/ws` 代理到：

```text
http://127.0.0.1:8081
ws://127.0.0.1:8081
```

## 检查

```powershell
npm run typecheck
npm run build
```
