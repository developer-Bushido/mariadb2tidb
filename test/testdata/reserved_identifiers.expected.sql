CREATE TABLE `events` (
  `id` BIGINT NOT NULL AUTO_INCREMENT,
  `until` DATETIME NOT NULL,
  `except` VARCHAR(32) DEFAULT NULL,
  `key` VARCHAR(64) NOT NULL,
  `order` INT NOT NULL DEFAULT 0,
  PRIMARY KEY(`id`),
  INDEX `idx_until`(`until`),
  UNIQUE `key`(`key`)
) ENGINE = InnoDB DEFAULT CHARACTER SET = UTF8MB4 COMMENT = 'calendar events';
