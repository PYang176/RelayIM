-- +goose Up

ALTER TABLE conversations
    ADD COLUMN avatar_object_key VARCHAR(512)
        CHARACTER SET ascii COLLATE ascii_bin NULL
        AFTER avatar_url;

-- +goose Down

ALTER TABLE conversations
    DROP COLUMN avatar_object_key;
