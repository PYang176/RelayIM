-- 仅清理压测和动态 E2E 脚本使用固定前缀创建的临时账号及其依赖数据。
-- 执行前应确认没有正在运行的测试，避免与活跃测试互相干扰。

DROP TABLE IF EXISTS relayim_cleanup_messages;
DROP TABLE IF EXISTS relayim_cleanup_conversations;
DROP TABLE IF EXISTS relayim_cleanup_users;

CREATE TABLE relayim_cleanup_users (
    id BIGINT UNSIGNED PRIMARY KEY
);

INSERT INTO relayim_cleanup_users (id)
SELECT id
FROM users
WHERE username LIKE 'load_http_%'
   OR username LIKE 'load_gc_%'
   OR username LIKE 'loadwss_%'
   OR username LIKE 'loadwsr_%'
   OR username LIKE 'boundary_a_%'
   OR username LIKE 'boundary_b_%'
   OR username LIKE 'ux_a_%'
   OR username LIKE 'ux_b_%'
   OR username LIKE 'ux_c_%'
   OR username LIKE 'ux_d_%'
   OR username LIKE 'block_a_%'
   OR username LIKE 'block_b_%'
   OR username LIKE 'reason_a_%'
   OR username LIKE 'reason_b_%'
   OR username LIKE 'fault_a_%'
   OR username LIKE 'fault_b_%';

CREATE TABLE relayim_cleanup_conversations (
    id BIGINT UNSIGNED PRIMARY KEY
);

INSERT INTO relayim_cleanup_conversations (id)
SELECT DISTINCT c.id
FROM conversations AS c
WHERE c.owner_id IN (SELECT id FROM relayim_cleanup_users)
   OR c.direct_low_user_id IN (SELECT id FROM relayim_cleanup_users)
   OR c.direct_high_user_id IN (SELECT id FROM relayim_cleanup_users)
   OR EXISTS (
       SELECT 1
       FROM conversation_members AS cm
       WHERE cm.conversation_id = c.id
         AND cm.user_id IN (SELECT id FROM relayim_cleanup_users)
   );

CREATE TABLE relayim_cleanup_messages (
    id BIGINT UNSIGNED PRIMARY KEY
);

INSERT INTO relayim_cleanup_messages (id)
SELECT id
FROM messages
WHERE conversation_id IN (SELECT id FROM relayim_cleanup_conversations)
   OR sender_id IN (SELECT id FROM relayim_cleanup_users);

START TRANSACTION;

DELETE pe
FROM processed_events AS pe
JOIN outbox_events AS oe ON oe.event_id = pe.event_id
WHERE oe.aggregate_id IN (SELECT id FROM relayim_cleanup_users)
   OR oe.aggregate_id IN (SELECT id FROM relayim_cleanup_conversations)
   OR oe.aggregate_id IN (SELECT id FROM relayim_cleanup_messages);

DELETE FROM outbox_events
WHERE aggregate_id IN (SELECT id FROM relayim_cleanup_users)
   OR aggregate_id IN (SELECT id FROM relayim_cleanup_conversations)
   OR aggregate_id IN (SELECT id FROM relayim_cleanup_messages);

DELETE FROM message_deliveries
WHERE message_id IN (SELECT id FROM relayim_cleanup_messages);

DELETE FROM messages
WHERE id IN (SELECT id FROM relayim_cleanup_messages);

DELETE FROM group_invitations
WHERE conversation_id IN (SELECT id FROM relayim_cleanup_conversations)
   OR inviter_id IN (SELECT id FROM relayim_cleanup_users)
   OR invitee_id IN (SELECT id FROM relayim_cleanup_users);

DELETE FROM conversation_members
WHERE conversation_id IN (SELECT id FROM relayim_cleanup_conversations)
   OR user_id IN (SELECT id FROM relayim_cleanup_users);

DELETE FROM conversations
WHERE id IN (SELECT id FROM relayim_cleanup_conversations);

DELETE FROM attachments
WHERE owner_id IN (SELECT id FROM relayim_cleanup_users);

DELETE fr
FROM friend_remarks AS fr
JOIN friendships AS f ON f.id = fr.friendship_id
WHERE f.user_low_id IN (SELECT id FROM relayim_cleanup_users)
   OR f.user_high_id IN (SELECT id FROM relayim_cleanup_users);

DELETE FROM friend_requests
WHERE user_low_id IN (SELECT id FROM relayim_cleanup_users)
   OR user_high_id IN (SELECT id FROM relayim_cleanup_users)
   OR requester_id IN (SELECT id FROM relayim_cleanup_users);

DELETE FROM friendships
WHERE user_low_id IN (SELECT id FROM relayim_cleanup_users)
   OR user_high_id IN (SELECT id FROM relayim_cleanup_users);

DELETE FROM user_blocks
WHERE blocker_id IN (SELECT id FROM relayim_cleanup_users)
   OR blocked_id IN (SELECT id FROM relayim_cleanup_users);

DELETE FROM refresh_tokens
WHERE user_id IN (SELECT id FROM relayim_cleanup_users);

DELETE FROM devices
WHERE user_id IN (SELECT id FROM relayim_cleanup_users);

DELETE FROM user_profiles
WHERE user_id IN (SELECT id FROM relayim_cleanup_users);

DELETE FROM users
WHERE id IN (SELECT id FROM relayim_cleanup_users);

COMMIT;

DROP TABLE relayim_cleanup_messages;
DROP TABLE relayim_cleanup_conversations;
DROP TABLE relayim_cleanup_users;
