create table sessions (
	id INT not null auto_increment,
	session_uuid CHAR(36) not null DEFAULT '',
	note VARCHAR(100) default null,
	primary key (id),
	unique key uuid_key (session_uuid)
) ENGINE InnoDB,
  charset UTF8MB4;
