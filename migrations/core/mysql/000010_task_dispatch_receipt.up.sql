-- Task generation and assignment are separate machine-controlled steps.
-- Existing awaiting_confirmation rows remain valid for compatibility with
-- historical data and the controlled leader override path.
ALTER TABLE task_instance
    DROP CHECK chk_task_instance_status,
    ADD CONSTRAINT chk_task_instance_status CHECK (status IN ('pending_dispatch', 'awaiting_confirmation', 'assigned', 'in_progress', 'completed', 'cancelled'));

ALTER TABLE task_status_history
    DROP CHECK chk_task_status_history_to,
    ADD CONSTRAINT chk_task_status_history_to CHECK (to_status IN ('pending_dispatch', 'awaiting_confirmation', 'assigned', 'in_progress', 'completed', 'cancelled'));

ALTER TABLE task_assignment
    ADD COLUMN receipt_status VARCHAR(32) NOT NULL DEFAULT 'pending' AFTER status_version,
    ADD COLUMN received_at DATETIME(6) NULL AFTER confirmed_at,
    ADD KEY idx_task_assignment_receipt (receipt_status, received_at),
    ADD CONSTRAINT chk_task_assignment_receipt CHECK (receipt_status IN ('pending', 'received'));
