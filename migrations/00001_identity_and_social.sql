-- +goose NO TRANSACTION
-- +goose Up

CREATE TABLE users (
    id BIGINT UNSIGNED NOT NULL,
    username VARCHAR(32) NOT NULL COMMENT '登录名，大小写不敏感',
    password_hash VARCHAR(255) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    status SMALLINT UNSIGNED NOT NULL DEFAULT 1 COMMENT '1正常 2禁用',
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
        ON UPDATE CURRENT_TIMESTAMP(3),
    deleted_at DATETIME(3) NULL,
    PRIMARY KEY (id),
    UNIQUE KEY uk_users_username (username),
    KEY idx_users_status (status, deleted_at),
    CONSTRAINT chk_users_status CHECK (status IN (1, 2)),
    CONSTRAINT chk_users_deleted_at CHECK (
        deleted_at IS NULL OR deleted_at >= created_at
    )
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci
  COMMENT='用户登录身份与账号状态';

CREATE TABLE user_profiles (
    user_id BIGINT UNSIGNED NOT NULL,
    nickname VARCHAR(64) NOT NULL,
    avatar_url VARCHAR(512) NULL,
    bio VARCHAR(500) NULL,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
        ON UPDATE CURRENT_TIMESTAMP(3),
    PRIMARY KEY (user_id),
    CONSTRAINT fk_user_profiles_user
        FOREIGN KEY (user_id) REFERENCES users (id)
        ON UPDATE RESTRICT ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci
  COMMENT='用户公开资料';

CREATE TABLE devices (
    id BIGINT UNSIGNED NOT NULL,
    user_id BIGINT UNSIGNED NOT NULL,
    platform SMALLINT UNSIGNED NOT NULL COMMENT '1Web 2Android 3iOS 4Desktop',
    device_name VARCHAR(100) NOT NULL,
    last_seen_at DATETIME(3) NULL,
    revoked_at DATETIME(3) NULL,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    KEY idx_devices_user_active (user_id, revoked_at, last_seen_at),
    CONSTRAINT fk_devices_user
        FOREIGN KEY (user_id) REFERENCES users (id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT chk_devices_platform CHECK (platform IN (1, 2, 3, 4)),
    CONSTRAINT chk_devices_revoked_at CHECK (
        revoked_at IS NULL OR revoked_at >= created_at
    )
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci
  COMMENT='用户设备与会话终端';

CREATE TABLE refresh_tokens (
    id BIGINT UNSIGNED NOT NULL,
    user_id BIGINT UNSIGNED NOT NULL,
    device_id BIGINT UNSIGNED NOT NULL,
    token_hash CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    expires_at DATETIME(3) NOT NULL,
    revoked_at DATETIME(3) NULL,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    UNIQUE KEY uk_refresh_tokens_hash (token_hash),
    KEY idx_refresh_tokens_user_device (user_id, device_id, revoked_at, expires_at),
    CONSTRAINT fk_refresh_tokens_user
        FOREIGN KEY (user_id) REFERENCES users (id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT fk_refresh_tokens_device
        FOREIGN KEY (device_id) REFERENCES devices (id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT chk_refresh_tokens_expiry CHECK (expires_at > created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci
  COMMENT='Refresh Token哈希与撤销状态';

CREATE TABLE friendships (
    id BIGINT UNSIGNED NOT NULL,
    user_low_id BIGINT UNSIGNED NOT NULL,
    user_high_id BIGINT UNSIGNED NOT NULL,
    status SMALLINT UNSIGNED NOT NULL DEFAULT 1 COMMENT '1正常 2已删除',
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
        ON UPDATE CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    UNIQUE KEY uk_friendships_pair (user_low_id, user_high_id),
    KEY idx_friendships_low_status (user_low_id, status, updated_at),
    KEY idx_friendships_high_status (user_high_id, status, updated_at),
    CONSTRAINT fk_friendships_low_user
        FOREIGN KEY (user_low_id) REFERENCES users (id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT fk_friendships_high_user
        FOREIGN KEY (user_high_id) REFERENCES users (id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT chk_friendships_order CHECK (user_low_id < user_high_id),
    CONSTRAINT chk_friendships_status CHECK (status IN (1, 2))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci
  COMMENT='规范化的一对好友关系';

CREATE TABLE friend_remarks (
    friendship_id BIGINT UNSIGNED NOT NULL,
    user_id BIGINT UNSIGNED NOT NULL,
    remark VARCHAR(64) NOT NULL,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
        ON UPDATE CURRENT_TIMESTAMP(3),
    PRIMARY KEY (friendship_id, user_id),
    KEY idx_friend_remarks_user (user_id, friendship_id),
    CONSTRAINT fk_friend_remarks_friendship
        FOREIGN KEY (friendship_id) REFERENCES friendships (id)
        ON UPDATE RESTRICT ON DELETE CASCADE,
    CONSTRAINT fk_friend_remarks_user
        FOREIGN KEY (user_id) REFERENCES users (id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT chk_friend_remarks_not_empty CHECK (CHAR_LENGTH(remark) > 0)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci
  COMMENT='每个用户针对好友设置的独立备注';

CREATE TABLE friend_requests (
    id BIGINT UNSIGNED NOT NULL,
    user_low_id BIGINT UNSIGNED NOT NULL,
    user_high_id BIGINT UNSIGNED NOT NULL,
    requester_id BIGINT UNSIGNED NOT NULL,
    status SMALLINT UNSIGNED NOT NULL DEFAULT 1
        COMMENT '1待处理 2已同意 3已拒绝 4已撤回',
    remark VARCHAR(200) NULL,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
        ON UPDATE CURRENT_TIMESTAMP(3),
    responded_at DATETIME(3) NULL,
    PRIMARY KEY (id),
    UNIQUE KEY uk_friend_requests_pair (user_low_id, user_high_id),
    KEY idx_friend_requests_requester (requester_id, status, created_at),
    CONSTRAINT fk_friend_requests_low_user
        FOREIGN KEY (user_low_id) REFERENCES users (id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT fk_friend_requests_high_user
        FOREIGN KEY (user_high_id) REFERENCES users (id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT fk_friend_requests_requester
        FOREIGN KEY (requester_id) REFERENCES users (id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT chk_friend_requests_order CHECK (user_low_id < user_high_id),
    CONSTRAINT chk_friend_requests_requester CHECK (
        requester_id IN (user_low_id, user_high_id)
    ),
    CONSTRAINT chk_friend_requests_status CHECK (status IN (1, 2, 3, 4)),
    CONSTRAINT chk_friend_requests_responded CHECK (
        (status = 1 AND responded_at IS NULL)
        OR (status IN (2, 3) AND responded_at IS NOT NULL)
        OR status = 4
    )
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci
  COMMENT='规范化好友申请及当前状态';

-- +goose Down

DROP TABLE IF EXISTS friend_requests;
DROP TABLE IF EXISTS friend_remarks;
DROP TABLE IF EXISTS friendships;
DROP TABLE IF EXISTS refresh_tokens;
DROP TABLE IF EXISTS devices;
DROP TABLE IF EXISTS user_profiles;
DROP TABLE IF EXISTS users;
