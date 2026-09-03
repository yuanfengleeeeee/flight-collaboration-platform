ALTER TABLE outbox_event
    ADD COLUMN lease_owner VARCHAR(128) NULL AFTER last_error,
    ADD COLUMN lease_expires_at DATETIME(6) NULL AFTER lease_owner,
    ADD KEY idx_outbox_lease (status, next_attempt_at, lease_expires_at);
