ALTER TABLE task_projection
    ADD COLUMN receipt_status VARCHAR(32) NOT NULL DEFAULT 'pending' AFTER status,
    ADD COLUMN received_at DATETIME(6) NULL AFTER receipt_status,
    ADD KEY idx_task_projection_receipt (employee_public_id, receipt_status, updated_at);
