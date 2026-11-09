-- +goose Up

ALTER TABLE group_invitations
    ADD COLUMN invite_remark VARCHAR(200) NULL AFTER inviter_id;

UPDATE group_invitations
SET invite_remark = '邀请加入群聊'
WHERE invite_remark IS NULL;

ALTER TABLE group_invitations
    MODIFY COLUMN invite_remark VARCHAR(200) NOT NULL,
    ADD CONSTRAINT chk_group_invitations_remark CHECK (
        CHAR_LENGTH(TRIM(invite_remark)) > 0
    );

-- +goose Down

ALTER TABLE group_invitations
    DROP CHECK chk_group_invitations_remark,
    DROP COLUMN invite_remark;
