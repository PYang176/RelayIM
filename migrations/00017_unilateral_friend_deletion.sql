-- +goose Up

ALTER TABLE friendships
    ADD COLUMN low_user_deleted_at DATETIME(3) NULL AFTER status,
    ADD COLUMN high_user_deleted_at DATETIME(3) NULL AFTER low_user_deleted_at,
    ADD CONSTRAINT chk_friendships_low_deleted
        CHECK (low_user_deleted_at IS NULL OR low_user_deleted_at >= created_at),
    ADD CONSTRAINT chk_friendships_high_deleted
        CHECK (high_user_deleted_at IS NULL OR high_user_deleted_at >= created_at);

ALTER TABLE friendships
    DROP INDEX idx_friendships_low_status_id,
    DROP INDEX idx_friendships_high_status_id,
    ADD KEY idx_friendships_low_active_id
        (user_low_id, status, low_user_deleted_at, id),
    ADD KEY idx_friendships_high_active_id
        (user_high_id, status, high_user_deleted_at, id);

-- +goose Down

ALTER TABLE friendships
    DROP INDEX idx_friendships_low_active_id,
    DROP INDEX idx_friendships_high_active_id,
    ADD KEY idx_friendships_low_status_id (user_low_id, status, id),
    ADD KEY idx_friendships_high_status_id (user_high_id, status, id);

ALTER TABLE friendships
    DROP CONSTRAINT chk_friendships_high_deleted,
    DROP CONSTRAINT chk_friendships_low_deleted,
    DROP COLUMN high_user_deleted_at,
    DROP COLUMN low_user_deleted_at;
