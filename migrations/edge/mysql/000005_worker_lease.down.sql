ALTER TABLE mobile_command
    DROP KEY idx_mobile_command_lease,
    DROP COLUMN lease_expires_at,
    DROP COLUMN lease_owner;
