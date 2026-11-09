-- +goose Up

ALTER TABLE conversation_members
    ADD COLUMN pinned_at DATETIME(3) NULL AFTER last_read_seq;

-- +goose Down

ALTER TABLE conversation_members
    DROP COLUMN pinned_at;
