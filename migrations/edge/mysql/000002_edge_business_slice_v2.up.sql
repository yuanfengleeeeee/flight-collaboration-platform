ALTER TABLE task_projection
    ADD COLUMN assignment_public_id CHAR(36) NOT NULL DEFAULT '' AFTER public_id;
