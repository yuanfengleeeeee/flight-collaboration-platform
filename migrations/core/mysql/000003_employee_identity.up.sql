CREATE TABLE IF NOT EXISTS employee_credential (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    public_id CHAR(36) NOT NULL,
    personnel_id BIGINT UNSIGNED NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    failed_attempts INT UNSIGNED NOT NULL DEFAULT 0,
    locked_until DATETIME(6) NULL,
    password_changed_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    PRIMARY KEY (id),
    UNIQUE KEY uk_employee_credential_public_id (public_id),
    UNIQUE KEY uk_employee_credential_personnel (personnel_id),
    CONSTRAINT fk_employee_credential_personnel FOREIGN KEY (personnel_id) REFERENCES personnel (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS external_identity_binding (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    public_id CHAR(36) NOT NULL,
    personnel_id BIGINT UNSIGNED NOT NULL,
    provider VARCHAR(32) NOT NULL,
    provider_app VARCHAR(128) NOT NULL,
    external_subject VARCHAR(255) NOT NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    PRIMARY KEY (id),
    UNIQUE KEY uk_external_identity_binding_public_id (public_id),
    UNIQUE KEY uk_external_identity_binding_subject (provider, provider_app, external_subject),
    UNIQUE KEY uk_external_identity_binding_personnel_provider (personnel_id, provider, provider_app),
    KEY idx_external_identity_binding_personnel (personnel_id),
    CONSTRAINT fk_external_identity_binding_personnel FOREIGN KEY (personnel_id) REFERENCES personnel (id),
    CONSTRAINT chk_external_identity_binding_provider CHECK (provider IN ('personal_wechat', 'wecom'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS identity_binding_ticket (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    public_id CHAR(36) NOT NULL,
    ticket_hash CHAR(64) NOT NULL,
    personnel_id BIGINT UNSIGNED NOT NULL,
    client VARCHAR(32) NOT NULL,
    expires_at DATETIME(6) NOT NULL,
    used_at DATETIME(6) NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (id),
    UNIQUE KEY uk_identity_binding_ticket_public_id (public_id),
    UNIQUE KEY uk_identity_binding_ticket_hash (ticket_hash),
    KEY idx_identity_binding_ticket_personnel (personnel_id, expires_at),
    CONSTRAINT fk_identity_binding_ticket_personnel FOREIGN KEY (personnel_id) REFERENCES personnel (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
