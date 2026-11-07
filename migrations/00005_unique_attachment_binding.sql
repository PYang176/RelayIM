-- +goose Up

ALTER TABLE messages
    ADD UNIQUE KEY uk_messages_attachment (attachment_id);

ALTER TABLE messages
    DROP INDEX idx_messages_attachment;

-- +goose Down

ALTER TABLE messages
    ADD KEY idx_messages_attachment (attachment_id);

ALTER TABLE messages
    DROP INDEX uk_messages_attachment;
