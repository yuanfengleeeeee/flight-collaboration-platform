ALTER TABLE admin_identity
    DROP CHECK chk_admin_identity_role,
    ADD CONSTRAINT chk_admin_identity_role CHECK (role IN ('admin', 'manager', 'leader'));
