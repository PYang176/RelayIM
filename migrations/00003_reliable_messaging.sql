-- +goose NO TRANSACTION
-- +goose Up

CREATE TABLE outbox_events (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    event_id BIGINT UNSIGNED NOT NULL,
    aggregate_type VARCHAR(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    aggregate_id BIGINT UNSIGNED NOT NULL,
    event_type VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    payload JSON NOT NULL,
    status TINYINT UNSIGNED NOT NULL DEFAULT 0
        COMMENT '0待发布 1处理中 2已发布 3死信',
    attempt_count INT UNSIGNED NOT NULL DEFAULT 0,
    available_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    locked_by VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NULL,
    locked_at DATETIME(3) NULL,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    published_at DATETIME(3) NULL,
    last_error TEXT NULL,
    PRIMARY KEY (id),
    UNIQUE KEY uk_outbox_event_id (event_id),
    KEY idx_outbox_claim (status, available_at, id),
    KEY idx_outbox_lease (status, locked_at),
    KEY idx_outbox_published_at (published_at),
    CONSTRAINT chk_outbox_status CHECK (status IN (0, 1, 2, 3))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci
  COMMENT='业务事务产生的待发布RabbitMQ事件';

CREATE TABLE processed_events (
    event_id BIGINT UNSIGNED NOT NULL,
    consumer_name VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    processed_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (event_id, consumer_name),
    KEY idx_processed_events_time (processed_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci
  COMMENT='消费端已成功处理的事件记录';

-- +goose Down

DROP TABLE IF EXISTS processed_events;
DROP TABLE IF EXISTS outbox_events;
