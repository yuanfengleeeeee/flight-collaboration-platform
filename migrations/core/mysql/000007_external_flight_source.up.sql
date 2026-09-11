ALTER TABLE flight
    ADD COLUMN source_provider VARCHAR(64) NOT NULL DEFAULT 'development' AFTER flight_display_no,
    ADD COLUMN external_flight_id VARCHAR(128) NULL AFTER source_provider,
    ADD COLUMN source_last_synced_at DATETIME(6) NULL AFTER external_flight_id,
    ADD UNIQUE KEY uk_flight_source_external_id (source_provider, external_flight_id),
    ADD KEY idx_flight_source_operating_date (source_provider, operating_date, scheduled_at);
