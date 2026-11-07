# Web 前端协作规则

1. 技术栈固定为 Vue 3、TypeScript、Vite、Vue Router、Pinia。
2. 页面组件放在 `src/views`，可复用组件放在 `src/components`，HTTP 请求放在 `src/api`。
3. 组件使用 `<script setup lang="ts">` 和 Composition API，不使用 Options API。
4. 组件不直接拼接后端 URL；所有请求通过 `src/api` 的类型化函数完成。
5. 跨页面状态放 Pinia；单个组件内部状态保留在组件中，不提前全局化。
6. Snowflake ID 在前后端协议中一律使用字符串，禁止转成 JavaScript `number`。
7. WebSocket 必须处理一次 Ticket、ACK、已读、断线重连和 HTTP `after_seq` 补拉。
8. 用户可见文本、注释和错误提示使用中文；代码标识符和协议字段保持英文。
9. 不引入大型 UI 框架或状态框架替代现有方案，优先保持依赖数量和结构简洁。
10. 提交前至少执行 `npm run typecheck` 和 `npm run build`。
11. TypeScript 与 Vue 统一使用 4 空格缩进、单引号、无分号。
12. 导入和大括号对象内部不留多余空格，例如 `{request}`、`{q: query}`。
13. Vue 模板使用 4 空格缩进；长属性续行相对开始标签缩进 4 个空格；
    无子节点的组件和 HTML 标签使用 `/>` 自闭合。
14. 文件编码使用 UTF-8、LF 换行，文件末尾保留一个换行。
15. 页面不展示用户、会话、消息等内部 Snowflake ID；需要标识对象时使用昵称、
    群名称或业务标题。
