-- +goose NO TRANSACTION
-- +goose Up

CREATE TABLE attachments (
    id BIGINT UNSIGNED NOT NULL,
    owner_id BIGINT UNSIGNED NOT NULL,
    object_key VARCHAR(512) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    mime_type VARCHAR(127) CHARACTER SET ascii COLLATE ascii_general_ci NOT NULL,
    size_bytes BIGINT UNSIGNED NOT NULL,
    checksum_sha256 BINARY(32) NULL,
    status SMALLINT UNSIGNED NOT NULL DEFAULT 1
        COMMENT '1待确认 2可用 3已阻止 4已删除',
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    finalized_at DATETIME(3) NULL,
    PRIMARY KEY (id),
    UNIQUE KEY uk_attachments_object_key (object_key),
    KEY idx_attachments_owner_status (owner_id, status, created_at),
    CONSTRAINT fk_attachments_owner
        FOREIGN KEY (owner_id) REFERENCES users (id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT chk_attachments_status CHECK (status IN (1, 2, 3, 4)),
    CONSTRAINT chk_attachments_size CHECK (size_bytes > 0)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci
  COMMENT='对象存储附件元数据';

CREATE TABLE conversations (
    id BIGINT UNSIGNED NOT NULL,
    conversation_type SMALLINT UNSIGNED NOT NULL COMMENT '1私聊 2群聊',
    direct_low_user_id BIGINT UNSIGNED NULL,
    direct_high_user_id BIGINT UNSIGNED NULL,
    owner_id BIGINT UNSIGNED NULL,
    name VARCHAR(100) NULL,
    avatar_url VARCHAR(512) NULL,
    status SMALLINT UNSIGNED NOT NULL DEFAULT 1
        COMMENT '1正常 2已归档 3已解散',
    last_seq BIGINT UNSIGNED NOT NULL DEFAULT 0,
    last_message_id BIGINT UNSIGNED NULL,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
        ON UPDATE CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    UNIQUE KEY uk_conversations_direct_pair (
        direct_low_user_id,
        direct_high_user_id
    ),
    KEY idx_conversations_owner_status (owner_id, status, updated_at),
    CONSTRAINT fk_conversations_direct_low
        FOREIGN KEY (direct_low_user_id) REFERENCES users (id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT fk_conversations_direct_high
        FOREIGN KEY (direct_high_user_id) REFERENCES users (id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT fk_conversations_owner
        FOREIGN KEY (owner_id) REFERENCES users (id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT chk_conversations_type CHECK (conversation_type IN (1, 2)),
    CONSTRAINT chk_conversations_status CHECK (status IN (1, 2, 3)),
    CONSTRAINT chk_conversations_shape CHECK (
        (
            conversation_type = 1
            AND direct_low_user_id IS NOT NULL
            AND direct_high_user_id IS NOT NULL
            AND direct_low_user_id < direct_high_user_id
            AND owner_id IS NULL
        )
        OR
        (
            conversation_type = 2
            AND direct_low_user_id IS NULL
            AND direct_high_user_id IS NULL
            AND owner_id IS NOT NULL
        )
    )
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci
  COMMENT='私聊与群聊统一会话';

CREATE TABLE conversation_members (
    conversation_id BIGINT UNSIGNED NOT NULL,
    user_id BIGINT UNSIGNED NOT NULL,
    role SMALLINT UNSIGNED NOT NULL DEFAULT 1
        COMMENT '1成员 2管理员 3群主',
    last_read_seq BIGINT UNSIGNED NOT NULL DEFAULT 0,
    joined_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    left_at DATETIME(3) NULL,
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
        ON UPDATE CURRENT_TIMESTAMP(3),
    PRIMARY KEY (conversation_id, user_id),
    KEY idx_members_user_active (user_id, left_at, conversation_id),
    CONSTRAINT fk_members_conversation
        FOREIGN KEY (conversation_id) REFERENCES conversations (id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT fk_members_user
        FOREIGN KEY (user_id) REFERENCES users (id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT chk_members_role CHECK (role IN (1, 2, 3)),
    CONSTRAINT chk_members_left_at CHECK (
        left_at IS NULL OR left_at >= joined_at
    )
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci
  COMMENT='会话成员角色与已读游标';

CREATE TABLE group_invitations (
    id BIGINT UNSIGNED NOT NULL,
    conversation_id BIGINT UNSIGNED NOT NULL,
    inviter_id BIGINT UNSIGNED NOT NULL,
    invitee_id BIGINT UNSIGNED NOT NULL,
    status SMALLINT UNSIGNED NOT NULL DEFAULT 1
        COMMENT '1待处理 2已接受 3已拒绝 4已过期 5已取消',
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    expires_at DATETIME(3) NOT NULL,
    responded_at DATETIME(3) NULL,
    PRIMARY KEY (id),
    UNIQUE KEY uk_group_invitations_conversation_invitee (
        conversation_id,
        invitee_id
    ),
    KEY idx_group_invitations_invitee (
        invitee_id,
        status,
        expires_at
    ),
    CONSTRAINT fk_group_invitations_conversation
        FOREIGN KEY (conversation_id) REFERENCES conversations (id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT fk_group_invitations_inviter
        FOREIGN KEY (inviter_id) REFERENCES users (id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT fk_group_invitations_invitee
        FOREIGN KEY (invitee_id) REFERENCES users (id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT chk_group_invitations_status CHECK (status IN (1, 2, 3, 4, 5)),
    CONSTRAINT chk_group_invitations_people CHECK (inviter_id <> invitee_id),
    CONSTRAINT chk_group_invitations_expiry CHECK (expires_at > created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci
  COMMENT='群成员邀请及状态';

CREATE TABLE messages (
    id BIGINT UNSIGNED NOT NULL,
    conversation_id BIGINT UNSIGNED NOT NULL,
    seq BIGINT UNSIGNED NOT NULL,
    sender_id BIGINT UNSIGNED NOT NULL,
    client_msg_id VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    message_type SMALLINT UNSIGNED NOT NULL
        COMMENT '1文本 2图片 3文件 4音频',
    content TEXT NULL,
    attachment_id BIGINT UNSIGNED NULL,
    reply_to_message_id BIGINT UNSIGNED NULL,
    metadata JSON NULL,
    recalled_at DATETIME(3) NULL,
    recalled_by BIGINT UNSIGNED NULL,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    UNIQUE KEY uk_messages_conversation_seq (conversation_id, seq),
    UNIQUE KEY uk_messages_sender_client (sender_id, client_msg_id),
    KEY idx_messages_attachment (attachment_id),
    KEY idx_messages_reply_to (reply_to_message_id),
    KEY idx_messages_recalled_by (recalled_by),
    CONSTRAINT fk_messages_conversation
        FOREIGN KEY (conversation_id) REFERENCES conversations (id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT fk_messages_sender
        FOREIGN KEY (sender_id) REFERENCES users (id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT fk_messages_attachment
        FOREIGN KEY (attachment_id) REFERENCES attachments (id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT fk_messages_reply_to
        FOREIGN KEY (reply_to_message_id) REFERENCES messages (id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT fk_messages_recalled_by
        FOREIGN KEY (recalled_by) REFERENCES users (id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT chk_messages_seq_positive CHECK (seq > 0),
    CONSTRAINT chk_messages_type CHECK (message_type IN (1, 2, 3, 4)),
    CONSTRAINT chk_messages_content CHECK (
        message_type <> 1
        OR (content IS NOT NULL AND CHAR_LENGTH(TRIM(content)) > 0)
    ),
    CONSTRAINT chk_messages_attachment_shape CHECK (
        (message_type = 1 AND attachment_id IS NULL)
        OR (message_type IN (2, 3, 4) AND attachment_id IS NOT NULL)
    ),
    CONSTRAINT chk_messages_reply_not_self CHECK (
        reply_to_message_id IS NULL OR reply_to_message_id <> id
    ),
    CONSTRAINT chk_messages_recall_shape CHECK (
        (recalled_at IS NULL AND recalled_by IS NULL)
        OR (recalled_at IS NOT NULL AND recalled_by IS NOT NULL)
    )
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci
  COMMENT='会话消息正文与幂等键';

CREATE TABLE message_deliveries (
    message_id BIGINT UNSIGNED NOT NULL,
    device_id BIGINT UNSIGNED NOT NULL,
    delivered_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (message_id, device_id),
    KEY idx_message_deliveries_device (device_id, delivered_at),
    CONSTRAINT fk_message_deliveries_message
        FOREIGN KEY (message_id) REFERENCES messages (id)
        ON UPDATE RESTRICT ON DELETE CASCADE,
    CONSTRAINT fk_message_deliveries_device
        FOREIGN KEY (device_id) REFERENCES devices (id)
        ON UPDATE RESTRICT ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci
  COMMENT='消息到具体设备的送达回执';

-- +goose Down

DROP TABLE IF EXISTS message_deliveries;
DROP TABLE IF EXISTS messages;
DROP TABLE IF EXISTS group_invitations;
DROP TABLE IF EXISTS conversation_members;
DROP TABLE IF EXISTS conversations;
DROP TABLE IF EXISTS attachments;
