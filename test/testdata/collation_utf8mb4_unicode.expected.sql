create table users (
	id INT not null auto_increment,
	username VARCHAR(50) collate utf8mb4_0900_ai_ci not null,
	email VARCHAR(255) collate utf8mb4_0900_ai_ci not null,
	display_name VARCHAR(100) collate utf8mb4_0900_ai_ci default null,
	primary key (id),
	unique key unique_username (username),
	unique key unique_email (email)
) ENGINE InnoDB,
  charset UTF8MB4,
  COLLATE UTF8MB4_0900_AI_CI;

create table posts (
	id BIGINT not null auto_increment,
	title VARCHAR(200) collate utf8mb4_0900_ai_ci not null,
	author_id INT not null,
	primary key (id),
	key idx_author (author_id)
) ENGINE InnoDB,
  charset UTF8MB4,
  COLLATE UTF8MB4_0900_AI_CI;
