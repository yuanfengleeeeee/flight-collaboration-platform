ALTER TABLE mobile_session
    ADD COLUMN client VARCHAR(32) NOT NULL DEFAULT 'employee-miniapp' AFTER actor_public_id,
    ADD COLUMN refresh_token_hash CHAR(64) NULL AFTER client,
    ADD COLUMN absolute_expires_at DATETIME(6) NULL AFTER expires_at,
    ADD COLUMN last_seen_at DATETIME(6) NULL AFTER absolute_expires_at,
    ADD COLUMN revoked_at DATETIME(6) NULL AFTER last_seen_at,
    ADD COLUMN replaced_by_session_public_id CHAR(36) NULL AFTER revoked_at;

UPDATE mobile_session
SET absolute_expires_at = expires_at,
    last_seen_at = created_at
WHERE absolute_expires_at IS NULL OR last_seen_at IS NULL;

ALTER TABLE mobile_session
    ADD UNIQUE KEY uk_mobile_session_refresh_token_hash (refresh_token_hash),
    ADD KEY idx_mobile_session_actor_active (actor_public_id, revoked_at, expires_at);
