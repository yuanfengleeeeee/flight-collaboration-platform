CREATE TABLE IF NOT EXISTS job_position (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    public_id CHAR(36) NOT NULL,
    code VARCHAR(64) NOT NULL,
    name VARCHAR(128) NOT NULL,
    description VARCHAR(255) NOT NULL DEFAULT '',
    enabled TINYINT(1) NOT NULL DEFAULT 1,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    PRIMARY KEY (id),
    UNIQUE KEY uk_job_position_public_id (public_id),
    UNIQUE KEY uk_job_position_code (code),
    KEY idx_job_position_enabled (enabled),
    CONSTRAINT chk_job_position_enabled CHECK (enabled IN (0, 1))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS capability (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    public_id CHAR(36) NOT NULL,
    code VARCHAR(64) NOT NULL,
    name VARCHAR(128) NOT NULL,
    description VARCHAR(255) NOT NULL DEFAULT '',
    enabled TINYINT(1) NOT NULL DEFAULT 1,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    PRIMARY KEY (id),
    UNIQUE KEY uk_capability_public_id (public_id),
    UNIQUE KEY uk_capability_code (code),
    KEY idx_capability_enabled (enabled),
    CONSTRAINT chk_capability_enabled CHECK (enabled IN (0, 1))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Existing fixtures used the old inline codes. Promote them into the
-- dictionaries before the service starts requiring dictionary membership.
INSERT INTO job_position (public_id, code, name, description, enabled)
SELECT UUID(), codes.code, codes.code, '', 1
FROM (
    SELECT DISTINCT position_code AS code FROM personnel
    UNION
    SELECT DISTINCT required_position_code AS code FROM task_template
) AS codes
WHERE codes.code IS NOT NULL AND codes.code <> ''
ON DUPLICATE KEY UPDATE updated_at = updated_at;

INSERT INTO capability (public_id, code, name, description, enabled)
SELECT UUID(), codes.code, codes.code, '', 1
FROM (
    SELECT DISTINCT capability_code AS code
    FROM personnel AS p
    JOIN JSON_TABLE(p.capabilities, '$[*]' COLUMNS (capability_code VARCHAR(64) PATH '$')) AS values_table
    UNION
    SELECT DISTINCT capability_code AS code
    FROM task_template AS t
    JOIN JSON_TABLE(t.required_capabilities, '$[*]' COLUMNS (capability_code VARCHAR(64) PATH '$')) AS template_values
) AS codes
WHERE codes.code IS NOT NULL AND codes.code <> ''
ON DUPLICATE KEY UPDATE updated_at = updated_at;

-- Keep the storage shape compatible with existing task snapshots while
-- making the first configured capability the sole staffing capability.
UPDATE personnel
SET capabilities = JSON_ARRAY(JSON_UNQUOTE(JSON_EXTRACT(capabilities, '$[0]')))
WHERE JSON_TYPE(capabilities) = 'ARRAY' AND JSON_LENGTH(capabilities) > 1;

UPDATE task_template
SET required_capabilities = JSON_ARRAY(JSON_UNQUOTE(JSON_EXTRACT(required_capabilities, '$[0]')))
WHERE JSON_TYPE(required_capabilities) = 'ARRAY' AND JSON_LENGTH(required_capabilities) > 1;
