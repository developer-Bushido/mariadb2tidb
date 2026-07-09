-- TEXT/BLOB/JSON literal defaults must be stripped; unsupported function
-- defaults removed; CURRENT_TIMESTAMP defaults kept; JSON_VALID checks dropped.

CREATE TABLE `articles` (
  `id` int NOT NULL AUTO_INCREMENT,
  `body` text NOT NULL DEFAULT '',
  `payload` longtext DEFAULT NULL CHECK (json_valid(`payload`)),
  `token` varchar(64) NOT NULL DEFAULT uuid(),
  `created_at` timestamp NOT NULL DEFAULT current_timestamp(),
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
