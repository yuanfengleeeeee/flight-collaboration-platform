ALTER TABLE outbox_event
    DROP KEY idx_outbox_lease,
    DROP COLUMN lease_expires_at,
    DROP COLUMN lease_owner;
