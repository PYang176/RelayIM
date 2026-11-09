-- +goose Up

ALTER TABLE friendships
    DROP INDEX idx_friendships_low_status,
    DROP INDEX idx_friendships_high_status,
    ADD KEY idx_friendships_low_status_id (user_low_id, status, id),
    ADD KEY idx_friendships_high_status_id (user_high_id, status, id);

ALTER TABLE friend_requests
    DROP INDEX idx_friend_requests_requester,
    ADD KEY idx_friend_requests_low_status_id (user_low_id, status, id),
    ADD KEY idx_friend_requests_high_status_id (user_high_id, status, id),
    ADD KEY idx_friend_requests_requester_id (requester_id, id);

-- +goose Down

ALTER TABLE friend_requests
    DROP INDEX idx_friend_requests_requester_id,
    DROP INDEX idx_friend_requests_high_status_id,
    DROP INDEX idx_friend_requests_low_status_id,
    ADD KEY idx_friend_requests_requester (requester_id, status, created_at);

ALTER TABLE friendships
    DROP INDEX idx_friendships_high_status_id,
    DROP INDEX idx_friendships_low_status_id,
    ADD KEY idx_friendships_low_status (user_low_id, status, updated_at),
    ADD KEY idx_friendships_high_status (user_high_id, status, updated_at);
