-- +goose Up

ALTER TABLE group_invitations
    DROP INDEX idx_group_invitations_invitee,
    ADD KEY idx_group_invitations_invitee_status_id (
        invitee_id,
        status,
        id
    ),
    ADD KEY idx_group_invitations_conversation_status_id (
        conversation_id,
        status,
        id
    );

-- +goose Down

ALTER TABLE group_invitations
    DROP INDEX idx_group_invitations_conversation_status_id,
    DROP INDEX idx_group_invitations_invitee_status_id,
    ADD KEY idx_group_invitations_invitee (
        invitee_id,
        status,
        expires_at
    );
