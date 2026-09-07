DROP TABLE IF EXISTS flight_source_health;

ALTER TABLE flight
    DROP CHECK chk_flight_source_state,
    DROP KEY idx_flight_source_state,
    DROP COLUMN source_last_error,
    DROP COLUMN source_last_attempt_at,
    DROP COLUMN source_state;
