ALTER TABLE task_projection
    DROP KEY idx_task_projection_receipt,
    DROP COLUMN received_at,
    DROP COLUMN receipt_status;
