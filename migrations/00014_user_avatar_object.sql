-- +goose Up

ALTER TABLE user_profiles
    ADD COLUMN avatar_object_key VARCHAR(512)
        CHARACTER SET ascii COLLATE ascii_bin NULL
        AFTER avatar_url;

-- +goose Down

ALTER TABLE user_profiles
    DROP COLUMN avatar_object_key;
