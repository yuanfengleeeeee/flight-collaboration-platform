CREATE TABLE IF NOT EXISTS `flight` (
  `id` BIGINT NOT NULL AUTO_INCREMENT,
  `flight_no` VARCHAR(32) NOT NULL,
  `aircraft_type` VARCHAR(32) NOT NULL,
  `origin` VARCHAR(8) NOT NULL,
  `destination` VARCHAR(8) NOT NULL,
  `planned_departure` DATETIME(3) NOT NULL,
  `planned_arrival` DATETIME(3) NOT NULL,
  `actual_departure` DATETIME(3) NULL,
  `actual_arrival` DATETIME(3) NULL,
  `gate` VARCHAR(16) NOT NULL,
  `stand` VARCHAR(16) NOT NULL,
  `status` VARCHAR(32) NOT NULL,
  `source` VARCHAR(16) NOT NULL DEFAULT 'manual',
  `created_at` DATETIME(3) NOT NULL,
  `updated_at` DATETIME(3) NOT NULL,
  `deleted_at` DATETIME(3) NULL,
  PRIMARY KEY (`id`),
  KEY `idx_flight_flight_no` (`flight_no`),
  KEY `idx_flight_status` (`status`),
  KEY `idx_flight_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `user` (
  `id` BIGINT NOT NULL AUTO_INCREMENT,
  `name` VARCHAR(64) NOT NULL,
  `employee_no` VARCHAR(32) NOT NULL,
  `phone` VARCHAR(20) NOT NULL,
  `username` VARCHAR(64) NOT NULL,
  `password` VARCHAR(128) NOT NULL,
  `role` VARCHAR(32) NOT NULL,
  `status` VARCHAR(32) NOT NULL DEFAULT 'active',
  `created_at` DATETIME(3) NOT NULL,
  `updated_at` DATETIME(3) NOT NULL,
  `deleted_at` DATETIME(3) NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_user_employee_no` (`employee_no`),
  UNIQUE KEY `uk_user_username` (`username`),
  KEY `idx_user_role` (`role`),
  KEY `idx_user_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `position` (
  `id` BIGINT NOT NULL AUTO_INCREMENT,
  `category` VARCHAR(64) NOT NULL,
  `name` VARCHAR(64) NOT NULL,
  `capabilities` TEXT NOT NULL,
  `enabled` TINYINT(1) NOT NULL DEFAULT 1,
  `created_at` DATETIME(3) NOT NULL,
  `updated_at` DATETIME(3) NOT NULL,
  `deleted_at` DATETIME(3) NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_position_name` (`name`),
  KEY `idx_position_category` (`category`),
  KEY `idx_position_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `user_position` (
  `id` BIGINT NOT NULL AUTO_INCREMENT,
  `user_id` BIGINT NOT NULL,
  `position_id` BIGINT NOT NULL,
  `is_primary` TINYINT(1) NOT NULL DEFAULT 0,
  `created_at` DATETIME(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_user_position` (`user_id`, `position_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `personnel_status` (
  `id` BIGINT NOT NULL AUTO_INCREMENT,
  `user_id` BIGINT NOT NULL,
  `status` VARCHAR(16) NOT NULL,
  `current_task_id` BIGINT NULL,
  `last_event_time` DATETIME(3) NOT NULL,
  `source` VARCHAR(16) NOT NULL DEFAULT 'task',
  `created_at` DATETIME(3) NOT NULL,
  `updated_at` DATETIME(3) NOT NULL,
  `deleted_at` DATETIME(3) NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_personnel_status_user_id` (`user_id`),
  KEY `idx_personnel_status_status` (`status`),
  KEY `idx_personnel_status_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `task_template` (
  `id` BIGINT NOT NULL AUTO_INCREMENT,
  `name` VARCHAR(64) NOT NULL,
  `phase` VARCHAR(32) NOT NULL,
  `required_position` VARCHAR(64) NOT NULL,
  `required_capability` VARCHAR(64) NOT NULL,
  `required_count` INT NOT NULL DEFAULT 1,
  `timeout_seconds` INT NOT NULL,
  `warning_advance_seconds` INT NOT NULL,
  `version` INT NOT NULL DEFAULT 1,
  `enabled` TINYINT(1) NOT NULL DEFAULT 1,
  `created_at` DATETIME(3) NOT NULL,
  `updated_at` DATETIME(3) NOT NULL,
  `deleted_at` DATETIME(3) NULL,
  PRIMARY KEY (`id`),
  KEY `idx_task_template_phase` (`phase`),
  KEY `idx_task_template_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `task_instance` (
  `id` BIGINT NOT NULL AUTO_INCREMENT,
  `flight_id` BIGINT NOT NULL,
  `template_id` BIGINT NOT NULL,
  `planned_start` DATETIME(3) NOT NULL,
  `planned_end` DATETIME(3) NOT NULL,
  `actual_start` DATETIME(3) NULL,
  `actual_end` DATETIME(3) NULL,
  `status` VARCHAR(32) NOT NULL,
  `assigned_count` INT NOT NULL DEFAULT 0,
  `created_at` DATETIME(3) NOT NULL,
  `updated_at` DATETIME(3) NOT NULL,
  `deleted_at` DATETIME(3) NULL,
  PRIMARY KEY (`id`),
  KEY `idx_task_instance_flight_id` (`flight_id`),
  KEY `idx_task_instance_template_id` (`template_id`),
  KEY `idx_task_instance_status` (`status`),
  KEY `idx_task_instance_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `task_assignment` (
  `id` BIGINT NOT NULL AUTO_INCREMENT,
  `task_id` BIGINT NOT NULL,
  `user_id` BIGINT NOT NULL,
  `status` VARCHAR(32) NOT NULL,
  `created_at` DATETIME(3) NOT NULL,
  `updated_at` DATETIME(3) NOT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_task_assignment_task_id` (`task_id`),
  KEY `idx_task_assignment_user_id` (`user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `event` (
  `id` BIGINT NOT NULL AUTO_INCREMENT,
  `type` VARCHAR(64) NOT NULL,
  `level` VARCHAR(16) NOT NULL,
  `source` VARCHAR(16) NOT NULL,
  `trigger_time` DATETIME(3) NOT NULL,
  `flight_id` BIGINT NULL,
  `task_id` BIGINT NULL,
  `affected_positions` TEXT NOT NULL,
  `affected_users` TEXT NOT NULL,
  `status` VARCHAR(16) NOT NULL,
  `title` VARCHAR(255) NOT NULL,
  `description` TEXT NOT NULL,
  `handle_logs` TEXT NOT NULL,
  `closed_at` DATETIME(3) NULL,
  `created_at` DATETIME(3) NOT NULL,
  `updated_at` DATETIME(3) NOT NULL,
  `deleted_at` DATETIME(3) NULL,
  PRIMARY KEY (`id`),
  KEY `idx_event_type` (`type`),
  KEY `idx_event_level` (`level`),
  KEY `idx_event_trigger_time` (`trigger_time`),
  KEY `idx_event_flight_id` (`flight_id`),
  KEY `idx_event_task_id` (`task_id`),
  KEY `idx_event_status` (`status`),
  KEY `idx_event_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `rule` (
  `id` BIGINT NOT NULL AUTO_INCREMENT,
  `type` VARCHAR(32) NOT NULL,
  `name` VARCHAR(128) NOT NULL,
  `condition` TEXT NOT NULL,
  `action` TEXT NOT NULL,
  `thresholds` TEXT NOT NULL,
  `enabled` TINYINT(1) NOT NULL DEFAULT 1,
  `version` INT NOT NULL DEFAULT 1,
  `created_at` DATETIME(3) NOT NULL,
  `updated_at` DATETIME(3) NOT NULL,
  `deleted_at` DATETIME(3) NULL,
  PRIMARY KEY (`id`),
  KEY `idx_rule_type` (`type`),
  KEY `idx_rule_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `flight_operation_statistics` (
  `id` BIGINT NOT NULL AUTO_INCREMENT,
  `flight_id` BIGINT NOT NULL,
  `date` VARCHAR(10) NOT NULL,
  `task_count` INT NOT NULL,
  `event_count` INT NOT NULL,
  `completion_rate` DOUBLE NOT NULL,
  `normal_completed_count` INT NOT NULL,
  `abnormal_count` INT NOT NULL,
  `avg_task_duration` DOUBLE NOT NULL,
  `created_at` DATETIME(3) NOT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_flight_operation_statistics_flight_id` (`flight_id`),
  KEY `idx_flight_operation_statistics_date` (`date`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `event_statistics` (
  `id` BIGINT NOT NULL AUTO_INCREMENT,
  `event_type` VARCHAR(64) NOT NULL,
  `stat_date` VARCHAR(10) NOT NULL,
  `count` INT NOT NULL,
  `avg_process_time` DOUBLE NOT NULL,
  `close_rate` DOUBLE NOT NULL,
  `affected_flight_count` INT NOT NULL,
  `created_at` DATETIME(3) NOT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_event_statistics_event_type` (`event_type`),
  KEY `idx_event_statistics_stat_date` (`stat_date`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `resource_statistics` (
  `id` BIGINT NOT NULL AUTO_INCREMENT,
  `role_type` VARCHAR(64) NOT NULL,
  `stat_date` VARCHAR(10) NOT NULL,
  `task_count` INT NOT NULL,
  `active_count` INT NOT NULL,
  `risk_count` INT NOT NULL,
  `idle_count` INT NOT NULL,
  `busy_count` INT NOT NULL,
  `completed_count` INT NOT NULL,
  `created_at` DATETIME(3) NOT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_resource_statistics_role_type` (`role_type`),
  KEY `idx_resource_statistics_stat_date` (`stat_date`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `prediction_interface` (
  `id` BIGINT NOT NULL AUTO_INCREMENT,
  `request_id` VARCHAR(64) NOT NULL,
  `request_payload` TEXT NOT NULL,
  `prediction_status` VARCHAR(32) NOT NULL DEFAULT 'not_enabled',
  `created_at` DATETIME(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_prediction_interface_request_id` (`request_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
