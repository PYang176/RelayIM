-- +goose Up

ALTER TABLE user_blocks
    DROP INDEX idx_user_blocks_blocker_created;

-- +goose Down

ALTER TABLE user_blocks
    ADD KEY idx_user_blocks_blocker_created (blocker_id, created_at);
