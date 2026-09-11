ALTER TABLE flight
    ADD COLUMN source_state VARCHAR(16) NOT NULL DEFAULT 'stale' AFTER source_last_synced_at,
    ADD COLUMN source_last_attempt_at DATETIME(6) NULL AFTER source_state,
    ADD COLUMN source_last_error VARCHAR(1024) NULL AFTER source_last_attempt_at,
    ADD KEY idx_flight_source_state (source_provider, source_state, source_last_synced_at),
    ADD CONSTRAINT chk_flight_source_state CHECK (source_state IN ('fresh', 'stale', 'fallback', 'failed'));

CREATE TABLE IF NOT EXISTS flight_source_health (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    public_id CHAR(36) NOT NULL,
    provider VARCHAR(64) NOT NULL,
    state VARCHAR(16) NOT NULL DEFAULT 'stale',
    last_attempt_at DATETIME(6) NULL,
    last_success_at DATETIME(6) NULL,
    last_failure_at DATETIME(6) NULL,
    last_error VARCHAR(1024) NULL,
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    PRIMARY KEY (id),
    UNIQUE KEY uk_flight_source_health_public_id (public_id),
    UNIQUE KEY uk_flight_source_health_provider (provider),
    CONSTRAINT chk_flight_source_health_state CHECK (state IN ('fresh', 'stale', 'fallback', 'failed'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
