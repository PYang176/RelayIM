-- +goose Up

ALTER TABLE group_invitations
    ADD COLUMN response_remark VARCHAR(200) NULL AFTER invite_remark,
    ADD KEY idx_group_invitations_conversation_id (conversation_id, id),
    ADD CONSTRAINT chk_group_invitations_response_remark CHECK (
        response_remark IS NULL
        OR CHAR_LENGTH(TRIM(response_remark)) > 0
    );

-- +goose Down

ALTER TABLE group_invitations
    DROP CHECK chk_group_invitations_response_remark,
    DROP INDEX idx_group_invitations_conversation_id,
    DROP COLUMN response_remark;
