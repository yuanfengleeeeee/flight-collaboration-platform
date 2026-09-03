ALTER TABLE `task_instance`
  ADD UNIQUE KEY `uk_task_instance_trigger_event_id` (`trigger_event_id`);
