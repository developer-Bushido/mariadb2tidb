create table legacy_table (
	id INT not null auto_increment,
	`name` VARCHAR(100) collate utf8mb4_0900_ai_ci not null,
	description TEXT collate utf8mb4_0900_ai_ci,
	primary key (id)
) ENGINE InnoDB,
  charset UTF8MB4,
  COLLATE UTF8MB4_0900_AI_CI;

create table mixed_legacy (
	id INT not null auto_increment,
	legacy_name VARCHAR(100) collate utf8mb4_0900_ai_ci not null,
	modern_name VARCHAR(100) not null,
	primary key (id)
) ENGINE InnoDB,
  charset UTF8MB4,
  COLLATE UTF8MB4_0900_AI_CI;
