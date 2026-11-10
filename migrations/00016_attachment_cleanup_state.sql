-- +goose Up

ALTER TABLE attachments
    DROP CHECK chk_attachments_status,
    ADD COLUMN cleanup_started_at DATETIME(3) NULL AFTER status,
    ADD CONSTRAINT chk_attachments_status
        CHECK (status IN (1, 2, 3, 4, 5));

-- +goose Down

ALTER TABLE attachments
    DROP CHECK chk_attachments_status,
    DROP COLUMN cleanup_started_at,
    ADD CONSTRAINT chk_attachments_status
        CHECK (status IN (1, 2, 3, 4));
