ALTER TABLE task_assignment
    DROP CHECK chk_task_assignment_receipt,
    DROP KEY idx_task_assignment_receipt,
    DROP COLUMN received_at,
    DROP COLUMN receipt_status;

ALTER TABLE task_status_history
    DROP CHECK chk_task_status_history_to,
    ADD CONSTRAINT chk_task_status_history_to CHECK (to_status IN ('awaiting_confirmation', 'assigned', 'in_progress', 'completed', 'cancelled'));

ALTER TABLE task_instance
    DROP CHECK chk_task_instance_status,
    ADD CONSTRAINT chk_task_instance_status CHECK (status IN ('awaiting_confirmation', 'assigned', 'in_progress', 'completed', 'cancelled'));
