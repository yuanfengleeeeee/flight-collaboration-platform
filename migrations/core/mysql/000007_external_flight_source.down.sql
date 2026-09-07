ALTER TABLE flight
    DROP KEY uk_flight_source_external_id,
    DROP KEY idx_flight_source_operating_date,
    DROP COLUMN source_last_synced_at,
    DROP COLUMN external_flight_id,
    DROP COLUMN source_provider;
