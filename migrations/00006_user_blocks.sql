-- +goose Up

CREATE TABLE user_blocks (
    blocker_id BIGINT UNSIGNED NOT NULL,
    blocked_id BIGINT UNSIGNED NOT NULL,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (blocker_id, blocked_id),
    KEY idx_user_blocks_blocked (blocked_id, blocker_id),
    KEY idx_user_blocks_blocker_created (blocker_id, created_at),
    CONSTRAINT fk_user_blocks_blocker
        FOREIGN KEY (blocker_id) REFERENCES users (id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT fk_user_blocks_blocked
        FOREIGN KEY (blocked_id) REFERENCES users (id)
        ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT chk_user_blocks_distinct_users CHECK (
        blocker_id <> blocked_id
    )
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci
  COMMENT='用户主动拉黑关系';

-- +goose Down

DROP TABLE IF EXISTS user_blocks;
