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
| `00004_attachment_filename.sql` | 附件原始文件名 |
| `00005_unique_attachment_binding.sql` | 一个附件最多绑定一条消息 |
| `00006_user_blocks.sql` | 用户方向性拉黑关系与双向检查索引 |
| `00007_friendship_pagination_indexes.sql` | 好友与申请列表 Keyset 分页索引 |
| `00008_drop_unused_user_block_time_index.sql` | 拉黑列表改用联合主键分页并清理冗余索引 |
| `00009_group_invitation_pagination_indexes.sql` | 群邀请状态列表 Keyset 分页索引 |
| `00010_group_invitation_remark.sql` | 群邀请必填备注及数据库约束 |
| `00011_group_invitation_response.sql` | 审批回复备注和群邀请历史分页索引 |

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
