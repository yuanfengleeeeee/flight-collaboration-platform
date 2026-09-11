CREATE TABLE IF NOT EXISTS `team` (
  `id` BIGINT NOT NULL AUTO_INCREMENT,
  `code` VARCHAR(32) NOT NULL,
  `name` VARCHAR(64) NOT NULL,
  `enabled` TINYINT(1) NOT NULL DEFAULT 1,
  `created_at` DATETIME(3) NOT NULL,
  `updated_at` DATETIME(3) NOT NULL,
  `deleted_at` DATETIME(3) NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_team_code` (`code`),
  KEY `idx_team_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `team_member` (
  `id` BIGINT NOT NULL AUTO_INCREMENT,
  `team_id` BIGINT NOT NULL,
  `user_id` BIGINT NOT NULL,
  `is_primary` TINYINT(1) NOT NULL DEFAULT 0,
  `created_at` DATETIME(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_team_member` (`team_id`, `user_id`),
  KEY `idx_team_member_user_id` (`user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

ALTER TABLE `event`
  ADD COLUMN `source_event_id` VARCHAR(128) NOT NULL AFTER `source`,
  ADD COLUMN `idempotency_key` VARCHAR(191) NOT NULL AFTER `source_event_id`,
  ADD UNIQUE KEY `uk_event_idempotency_key` (`idempotency_key`);

ALTER TABLE `task_template`
  ADD COLUMN `trigger_event_type` VARCHAR(64) NOT NULL AFTER `required_capability`;

ALTER TABLE `task_instance`
  ADD COLUMN `team_id` BIGINT NOT NULL AFTER `template_id`,
  ADD COLUMN `trigger_event_id` BIGINT NOT NULL AFTER `team_id`,
  ADD COLUMN `template_version` INT NOT NULL DEFAULT 1 AFTER `trigger_event_id`;

ALTER TABLE `task_assignment`
  ADD COLUMN `candidate_rank` INT NOT NULL DEFAULT 0 AFTER `status`,
  ADD COLUMN `confirmed_by` BIGINT NULL AFTER `candidate_rank`,
  ADD COLUMN `confirmed_at` DATETIME(3) NULL AFTER `confirmed_by`,
  ADD COLUMN `accepted_at` DATETIME(3) NULL AFTER `confirmed_at`,
  ADD COLUMN `completed_at` DATETIME(3) NULL AFTER `accepted_at`,
  ADD COLUMN `rejected_at` DATETIME(3) NULL AFTER `completed_at`,
  ADD COLUMN `rejection_reason` VARCHAR(255) NOT NULL AFTER `rejected_at`,
  ADD UNIQUE KEY `uk_task_assignment_task_id` (`task_id`);

CREATE TABLE IF NOT EXISTS `task_candidate` (
  `id` BIGINT NOT NULL AUTO_INCREMENT,
  `task_id` BIGINT NOT NULL,
  `user_id` BIGINT NOT NULL,
  `rank` INT NOT NULL,
  `matched_by` VARCHAR(32) NOT NULL,
  `status` VARCHAR(16) NOT NULL,
  `created_at` DATETIME(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_task_candidate` (`task_id`, `user_id`),
  KEY `idx_task_candidate_user_id` (`user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `notification` (
  `id` BIGINT NOT NULL AUTO_INCREMENT,
  `user_id` BIGINT NOT NULL,
  `task_id` BIGINT NULL,
  `event_id` BIGINT NULL,
  `dedupe_key` VARCHAR(191) NOT NULL,
  `type` VARCHAR(32) NOT NULL,
  `status` VARCHAR(16) NOT NULL,
  `title` VARCHAR(255) NOT NULL,
  `content` TEXT NOT NULL,
  `sent_at` DATETIME(3) NULL,
  `read_at` DATETIME(3) NULL,
  `created_at` DATETIME(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_notification_dedupe_key` (`dedupe_key`),
  KEY `idx_notification_user_id` (`user_id`),
  KEY `idx_notification_task_id` (`task_id`),
  KEY `idx_notification_status` (`status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `audit_log` (
  `id` BIGINT NOT NULL AUTO_INCREMENT,
  `actor_user_id` BIGINT NULL,
  `action` VARCHAR(64) NOT NULL,
  `resource_type` VARCHAR(64) NOT NULL,
  `resource_id` BIGINT NOT NULL,
  `from_status` VARCHAR(32) NOT NULL,
  `to_status` VARCHAR(32) NOT NULL,
  `metadata` TEXT NOT NULL,
  `created_at` DATETIME(3) NOT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_audit_log_actor_user_id` (`actor_user_id`),
  KEY `idx_audit_log_action` (`action`),
  KEY `idx_audit_log_resource` (`resource_type`, `resource_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
