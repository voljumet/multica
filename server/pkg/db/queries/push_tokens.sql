-- name: UpsertPushToken :exec
INSERT INTO device_push_tokens (user_id, token, platform, updated_at)
VALUES ($1, $2, $3, now())
ON CONFLICT (token)
DO UPDATE SET user_id = EXCLUDED.user_id, platform = EXCLUDED.platform, updated_at = now();

-- name: ListPushTokensByUser :many
SELECT * FROM device_push_tokens
WHERE user_id = $1;

-- name: DeletePushToken :exec
DELETE FROM device_push_tokens WHERE token = $1;
