# 数据库迁移

数据库结构以 SQL migration 为事实来源，不使用 GORM `AutoMigrate`。

当前开发使用新的空库 `ginchat_v2`，不迁移旧库数据。

字段职责、业务不变量和核心事务说明见
[数据库开发指南](../docs/数据库开发指南.md)。

## 迁移文件

| 文件 | 内容 |
| --- | --- |
| `00001_identity_and_social.sql` | 用户、设备、好友关系与好友备注 |
| `00002_conversation_and_message.sql` | 会话、成员、附件、消息与送达记录 |
| `00003_reliable_messaging.sql` | Outbox 与消费幂等记录 |

## 执行迁移

在项目根目录使用 `goose`：

```powershell
goose -dir migrations mysql "<DSN>" up
```

检查版本：

```powershell
goose -dir migrations mysql "<DSN>" status
```

该命令中的 DSN 包含数据库密码，只应在本地终端使用，不能写入 Git。

## 回退

空库或开发库可以回退：

```powershell
goose -dir migrations mysql "<DSN>" down-to 0
```

有业务数据的数据库不得直接执行全部回退。生产环境应通过应用版本切换、数据备份或向前修复完成回滚。
