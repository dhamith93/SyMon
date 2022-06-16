CREATE TABLE `server` (
  `id` int PRIMARY KEY AUTO_INCREMENT,
  `name` varchar(255),
  `timezone` varchar(30)
);


CREATE TABLE `symon_user` (
  `id` int PRIMARY KEY AUTO_INCREMENT,
  `username` varchar(100),
  `password` text,
  `email` varchar(255),
  `tp_no` varchar(35),
  `is_admin` tinyint
);

CREATE TABLE `user_server_access` (
  `id` int PRIMARY KEY AUTO_INCREMENT,
  `server_id` int,
  `user_id` int,
  `can_read` tinyint,
  `can_write` tinyint,
  `can_exec` tinyint
);

CREATE TABLE `config` (
  `id` int PRIMARY KEY AUTO_INCREMENT,
  `server_id` int,
  `key` varchar(255),
  `value` varchar(255)
);

CREATE TABLE `monitor_log` (
  `id` int PRIMARY KEY AUTO_INCREMENT,
  `server_id` int,
  `log_time` bigint(20),
  `log_type` varchar(255),
  `log_text` text
)
ENGINE=InnoDB
PAGE_COMPRESSED=1;

CREATE TABLE `monitor_alert` (
  `id` int PRIMARY KEY AUTO_INCREMENT,
  `server_id` int,
  `type` varchar(255),
  `enabled` tinyint
);

CREATE TABLE `monitor_alert_subscriber` (
  `id` int PRIMARY KEY AUTO_INCREMENT,
  `server_id` int,
  `type` int,
  `channel` int,
  `symon_user` int
);

CREATE TABLE `notification_type` (
  `id` int PRIMARY KEY AUTO_INCREMENT,
  `type` varchar(255)
);

CREATE TABLE `notification_channel` (
  `id` int PRIMARY KEY AUTO_INCREMENT,
  `type` varchar(255)
);

CREATE TABLE `notification` (
  `id` int PRIMARY KEY AUTO_INCREMENT,
  `to` text,
  `sent_at` bigint(20),
  `subject` varchar(255),
  `body` varchar(255),
  `type` int,
  `channel` int,
  `status` enum('PENDING', 'SENT', 'FAILED', 'ACKNOWLEDGED'),
  `acknowledged_by` int,
  `log_text` text
);

CREATE TABLE `alert` (
  `id` int PRIMARY KEY AUTO_INCREMENT,
  `server_id` int,
  `type` int,
  `expected` varchar(50),
  `actual` varchar(50),
  `time` bigint(20),
  `start_log_id` int,
  `end_log_id` int
);

CREATE TABLE `alert_notification` (
  `id` int PRIMARY KEY AUTO_INCREMENT,
  `alert_id` int,
  `notification_id` int
);

CREATE TABLE `notification_template` (
  `id` int PRIMARY KEY AUTO_INCREMENT,
  `name` varchar(255),
  `subject` varchar(255),
  `body` text,
  `type` int,
  `channel` int
);

ALTER TABLE `user_server_access` ADD FOREIGN KEY (`server_id`) REFERENCES `server` (`id`);

ALTER TABLE `user_server_access` ADD FOREIGN KEY (`user_id`) REFERENCES `symon_user` (`id`);

ALTER TABLE `config` ADD FOREIGN KEY (`server_id`) REFERENCES `server` (`id`);

ALTER TABLE `monitor_log` ADD FOREIGN KEY (`server_id`) REFERENCES `server` (`id`);

ALTER TABLE `monitor_alert` ADD FOREIGN KEY (`server_id`) REFERENCES `server` (`id`);

ALTER TABLE `monitor_alert_subscriber` ADD FOREIGN KEY (`server_id`) REFERENCES `server` (`id`);

ALTER TABLE `monitor_alert_subscriber` ADD FOREIGN KEY (`type`) REFERENCES `monitor_alert` (`id`);

ALTER TABLE `monitor_alert_subscriber` ADD FOREIGN KEY (`channel`) REFERENCES `notification_channel` (`id`);

ALTER TABLE `monitor_alert_subscriber` ADD FOREIGN KEY (`symon_user`) REFERENCES `symon_user` (`id`);

ALTER TABLE `notification` ADD FOREIGN KEY (`type`) REFERENCES `notification_type` (`id`);

ALTER TABLE `notification` ADD FOREIGN KEY (`channel`) REFERENCES `notification_channel` (`id`);

ALTER TABLE `notification` ADD FOREIGN KEY (`acknowledged_by`) REFERENCES `symon_user` (`id`);

ALTER TABLE `alert` ADD FOREIGN KEY (`server_id`) REFERENCES `server` (`id`);

ALTER TABLE `alert` ADD FOREIGN KEY (`type`) REFERENCES `monitor_alert` (`id`);

ALTER TABLE `alert` ADD FOREIGN KEY (`start_log_id`) REFERENCES `monitor_log` (`id`);

ALTER TABLE `alert` ADD FOREIGN KEY (`end_log_id`) REFERENCES `monitor_log` (`id`);

ALTER TABLE `alert_notification` ADD FOREIGN KEY (`alert_id`) REFERENCES `alert` (`id`);

ALTER TABLE `alert_notification` ADD FOREIGN KEY (`notification_id`) REFERENCES `notification` (`id`);

ALTER TABLE `notification_template` ADD FOREIGN KEY (`type`) REFERENCES `notification_type` (`id`);

ALTER TABLE `notification_template` ADD FOREIGN KEY (`channel`) REFERENCES `notification_channel` (`id`);

CREATE INDEX `log_time` ON `monitor_log`(`log_time`);

-- changes done for alerting functionalities
ALTER TABLE `monitor_log` ADD `log_name` VARCHAR(255);
CREATE INDEX `log_name` ON `monitor_log`(`log_name`);

-- changes done for `ping` functionality 
CREATE TABLE `server_ping_time` (
  `id` int PRIMARY KEY AUTO_INCREMENT,
  `server_id` int,
  `time` bigint(20)
);
ALTER TABLE `server_ping_time` ADD FOREIGN KEY (`server_id`) REFERENCES `server` (`id`);

-- change done to fix some lag on select queries
ALTER TABLE `monitor_log` ADD INDEX(`server_id`, `log_type`);