-- Reserved words as identifiers and options the old vitess formatter could
-- not parse must survive intact (vitess used to drop every column).

CREATE TABLE `events` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `until` datetime NOT NULL,
  `except` varchar(32) DEFAULT NULL,
  `key` varchar(64) NOT NULL,
  `order` int NOT NULL DEFAULT 0,
  PRIMARY KEY (`id`),
  KEY `idx_until` (`until`),
  UNIQUE KEY `key` (`key`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='calendar events';
