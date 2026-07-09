create table users (
	id INT(11) not null auto_increment,
	`name` VARCHAR(255) not null,
	email VARCHAR(255) default null,
	created_at TIMESTAMP default current_timestamp(),
	primary key (id),
	unique key email (email)
) ENGINE InnoDB,
  charset UTF8MB4,
  COLLATE UTF8MB4_0900_AI_CI;
