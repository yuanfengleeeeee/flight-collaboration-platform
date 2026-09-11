DROP TABLE IF EXISTS `audit_log`;
DROP TABLE IF EXISTS `notification`;
DROP TABLE IF EXISTS `task_candidate`;
ALTER TABLE `task_assignment`
  DROP INDEX `uk_task_assignment_task_id`,
  DROP COLUMN `rejection_reason`,
  DROP COLUMN `rejected_at`,
  DROP COLUMN `completed_at`,
  DROP COLUMN `accepted_at`,
  DROP COLUMN `confirmed_at`,
  DROP COLUMN `confirmed_by`,
  DROP COLUMN `candidate_rank`;
ALTER TABLE `task_instance`
  DROP COLUMN `template_version`,
  DROP COLUMN `trigger_event_id`,
  DROP COLUMN `team_id`;
ALTER TABLE `task_template` DROP COLUMN `trigger_event_type`;
ALTER TABLE `event`
  DROP INDEX `uk_event_idempotency_key`,
  DROP COLUMN `idempotency_key`,
  DROP COLUMN `source_event_id`;
DROP TABLE IF EXISTS `team_member`;
DROP TABLE IF EXISTS `team`;
