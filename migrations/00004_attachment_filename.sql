-- +goose Up

ALTER TABLE attachments
    ADD COLUMN original_name VARCHAR(255) NOT NULL DEFAULT ''
        COMMENT '用户上传时的原始文件名'
        AFTER object_key;

-- +goose Down

ALTER TABLE attachments
    DROP COLUMN original_name;
