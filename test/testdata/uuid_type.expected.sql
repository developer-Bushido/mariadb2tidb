CREATE TABLE `sessions` (
  `id` INT NOT NULL AUTO_INCREMENT,
  `session_uuid` CHAR(36) NOT NULL DEFAULT '',
  `note` VARCHAR(100) DEFAULT NULL,
  PRIMARY KEY(`id`),
  UNIQUE `uuid_key`(`session_uuid`)
) ENGINE = InnoDB DEFAULT CHARACTER SET = UTF8MB4;
