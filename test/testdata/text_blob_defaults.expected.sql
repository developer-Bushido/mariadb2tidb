create table articles (
	id INT not null auto_increment,
	body TEXT not null,
	payload LONGTEXT,
	token VARCHAR(64) not null,
	created_at TIMESTAMP not null default current_timestamp(),
	primary key (id)
) ENGINE InnoDB,
  charset UTF8MB4;
