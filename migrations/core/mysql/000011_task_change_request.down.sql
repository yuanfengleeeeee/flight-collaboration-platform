ALTER TABLE task_instance
    DROP CHECK chk_task_instance_status,
    ADD CONSTRAINT chk_task_instance_status CHECK (status IN ('pending_dispatch', 'awaiting_confirmation', 'assigned', 'in_progress', 'completed', 'cancelled'));

ALTER TABLE task_status_history
    DROP CHECK chk_task_status_history_to,
    ADD CONSTRAINT chk_task_status_history_to CHECK (to_status IN ('pending_dispatch', 'awaiting_confirmation', 'assigned', 'in_progress', 'completed', 'cancelled'));

DROP TABLE IF EXISTS task_change_request;
