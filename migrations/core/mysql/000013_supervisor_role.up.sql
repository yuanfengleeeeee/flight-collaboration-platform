-- A supervisor is a read-only management identity. Area/team scope controls
-- the task events visible on the management realtime stream.
ALTER TABLE admin_identity
    DROP CHECK chk_admin_identity_role,
    ADD CONSTRAINT chk_admin_identity_role CHECK (role IN ('admin', 'manager', 'leader', 'supervisor'));
