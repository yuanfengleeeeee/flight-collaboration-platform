CREATE TABLE IF NOT EXISTS realtime_connection_ticket (
    ticket_hash BINARY(32) NOT NULL PRIMARY KEY,
    employee_public_id CHAR(36) NOT NULL,
    expires_at DATETIME(6) NOT NULL,
    consumed_at DATETIME(6) NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    KEY idx_realtime_ticket_employee (employee_public_id),
    KEY idx_realtime_ticket_expiry (expires_at, consumed_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
