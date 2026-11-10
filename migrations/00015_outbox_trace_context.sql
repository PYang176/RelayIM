-- +goose Up

ALTER TABLE outbox_events
    ADD COLUMN trace_parent VARCHAR(128)
        CHARACTER SET ascii COLLATE ascii_bin NULL
        AFTER payload;

-- +goose Down

ALTER TABLE outbox_events
    DROP COLUMN trace_parent;
