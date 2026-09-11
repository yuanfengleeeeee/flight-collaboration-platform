-- Employee and supervisor changes are requests. A manager approval is the
-- only path that can apply pause, reschedule, reassignment or cancellation.
CREATE TABLE IF NOT EXISTS task_change_request (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    public_id CHAR(36) NOT NULL,
    task_id BIGINT UNSIGNED NOT NULL,
    exception_id BIGINT UNSIGNED NULL,
    action VARCHAR(32) NOT NULL,
    reason VARCHAR(1024) NOT NULL,
    target_candidate_public_id CHAR(36) NULL,
    target_planned_at DATETIME(6) NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'pending',
    requested_by_public_id CHAR(36) NOT NULL,
    requested_at DATETIME(6) NOT NULL,
    reviewed_by_public_id CHAR(36) NULL,
    reviewed_at DATETIME(6) NULL,
    review_note VARCHAR(1024) NULL,
    applied_at DATETIME(6) NULL,
    failure_reason VARCHAR(1024) NULL,
    request_id VARCHAR(128) NOT NULL,
    trace_id VARCHAR(128) NOT NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    PRIMARY KEY (id),
    UNIQUE KEY uk_task_change_request_public_id (public_id),
    UNIQUE KEY uk_task_change_request_request_id (request_id),
    KEY idx_task_change_request_task_status (task_id, status, requested_at),
    KEY idx_task_change_request_exception (exception_id),
    CONSTRAINT fk_task_change_request_task FOREIGN KEY (task_id) REFERENCES task_instance (id),
    CONSTRAINT fk_task_change_request_exception FOREIGN KEY (exception_id) REFERENCES task_exception (id),
    CONSTRAINT chk_task_change_request_action CHECK (action IN ('pause', 'reassign', 'reschedule', 'cancel', 'resume')),
    CONSTRAINT chk_task_change_request_status CHECK (status IN ('pending', 'approved', 'rejected', 'applied', 'failed'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

ALTER TABLE task_instance
    DROP CHECK chk_task_instance_status,
    ADD CONSTRAINT chk_task_instance_status CHECK (status IN ('pending_dispatch', 'awaiting_confirmation', 'assigned', 'in_progress', 'paused', 'completed', 'cancelled'));

ALTER TABLE task_status_history
    DROP CHECK chk_task_status_history_to,
    ADD CONSTRAINT chk_task_status_history_to CHECK (to_status IN ('pending_dispatch', 'awaiting_confirmation', 'assigned', 'in_progress', 'paused', 'completed', 'cancelled'));
