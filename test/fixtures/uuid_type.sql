-- MariaDB UUID columns become CHAR(36); the uuid() function default is
-- stripped (unsupported), NOT NULL uuid columns get a safe empty default.

CREATE TABLE `sessions` (
  `id` int NOT NULL AUTO_INCREMENT,
  `session_uuid` uuid NOT NULL,
  `note` varchar(100) DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uuid` (`session_uuid`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
