create table modelGoals (
	modelId INT(11) unsigned not null,
	requestId VARCHAR(23) character set UTF8MB4 collate utf8mb4_0900_ai_ci default null,
	historyId VARCHAR(23) character set UTF8MB4 collate utf8mb4_0900_ai_ci not null,
	primary key (historyId(23))
) ENGINE InnoDB,
  charset UTF8MB4,
  COLLATE UTF8MB4_0900_AI_CI;