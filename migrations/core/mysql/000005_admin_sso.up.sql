CREATE TABLE IF NOT EXISTS admin_identity (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    public_id CHAR(36) NOT NULL,
    provider VARCHAR(32) NOT NULL,
    external_subject VARCHAR(255) NOT NULL,
    display_name VARCHAR(128) NOT NULL,
    role VARCHAR(32) NOT NULL,
    enabled TINYINT(1) NOT NULL DEFAULT 1,
    global_scope TINYINT(1) NOT NULL DEFAULT 0,
    area_ids JSON NOT NULL,
    team_ids JSON NOT NULL,
    user_id BIGINT UNSIGNED NOT NULL DEFAULT 0,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    PRIMARY KEY (id),
    UNIQUE KEY uk_admin_identity_public_id (public_id),
    UNIQUE KEY uk_admin_identity_external (provider, external_subject),
    KEY idx_admin_identity_enabled (enabled, role),
    CONSTRAINT chk_admin_identity_role CHECK (role IN ('admin', 'manager', 'leader')),
    CONSTRAINT chk_admin_identity_enabled CHECK (enabled IN (0, 1)),
    CONSTRAINT chk_admin_identity_global_scope CHECK (global_scope IN (0, 1))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS admin_sso_state (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    public_id CHAR(36) NOT NULL,
    state_hash CHAR(64) NOT NULL,
    provider VARCHAR(32) NOT NULL,
    redirect_uri VARCHAR(2048) NOT NULL,
    expires_at DATETIME(6) NOT NULL,
    consumed_at DATETIME(6) NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    PRIMARY KEY (id),
    UNIQUE KEY uk_admin_sso_state_public_id (public_id),
    UNIQUE KEY uk_admin_sso_state_hash (state_hash),
    KEY idx_admin_sso_state_expiry (expires_at, consumed_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS admin_session (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    public_id CHAR(36) NOT NULL,
    admin_identity_id BIGINT UNSIGNED NOT NULL,
    created_at DATETIME(6) NOT NULL,
    last_seen_at DATETIME(6) NOT NULL,
    expires_at DATETIME(6) NOT NULL,
    absolute_expires_at DATETIME(6) NOT NULL,
    revoked_at DATETIME(6) NULL,
    PRIMARY KEY (id),
    UNIQUE KEY uk_admin_session_public_id (public_id),
    KEY idx_admin_session_active (admin_identity_id, revoked_at, expires_at),
    CONSTRAINT fk_admin_session_identity FOREIGN KEY (admin_identity_id) REFERENCES admin_identity (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
