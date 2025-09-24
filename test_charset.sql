CREATE TABLE modelGoals (
	modelId INT(11) unsigned not null,
	requestId VARCHAR(23) CHARACTER SET utf8mb3 COLLATE utf8mb3_unicode_ci DEFAULT NULL,
	historyId VARCHAR(23) CHARACTER SET utf8mb3 COLLATE utf8mb3_unicode_ci NOT NULL,
	PRIMARY KEY (historyId)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_520_ci;
