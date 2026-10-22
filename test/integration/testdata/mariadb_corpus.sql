-- MariaDB features the migration must handle; created on a real MariaDB,
-- dumped with mariadb-dump, converted, applied to TiDB and compared.
CREATE TABLE plain (
  id INT UNSIGNED NOT NULL AUTO_INCREMENT,
  `key` VARCHAR(64) NOT NULL,
  `order` INT NOT NULL DEFAULT 0,
  `until` DATETIME NULL,
  note VARCHAR(100) COMMENT 'stores a uuid value, latin1_swedish_ci text',
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  UNIQUE KEY `key` (`key`),
  KEY idx_until (`until`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='a table, with (parens)';
CREATE TABLE legacy (
  id INT NOT NULL AUTO_INCREMENT PRIMARY KEY,
  name VARCHAR(100) NOT NULL,
  zero_dt DATETIME NOT NULL DEFAULT '0000-00-00 00:00:00',
  zero_d DATE NOT NULL DEFAULT '0000-00-00'
) ENGINE=MyISAM DEFAULT CHARSET=latin1;
CREATE TABLE modern_types (
  id BIGINT NOT NULL PRIMARY KEY,
  u UUID NOT NULL,
  ip4 INET4,
  ip6 INET6 DEFAULT '::1',
  doc JSON,
  bits BIT(8) DEFAULT b'0',
  e ENUM('a','b') DEFAULT 'a',
  s SET('x','y'),
  d DECIMAL(65,30) DEFAULT 0,
  f DOUBLE DEFAULT 1.5,
  g POINT NULL,
  UNIQUE KEY uuid (u)
) ENGINE=InnoDB;
CREATE TABLE compressed (id INT PRIMARY KEY, b BLOB COMPRESSED) ENGINE=InnoDB PAGE_COMPRESSED=1 PAGE_COMPRESSION_LEVEL=6;
CREATE TABLE aria_t (id INT PRIMARY KEY) ENGINE=Aria TRANSACTIONAL=1 PAGE_CHECKSUM=1;
CREATE TABLE stats_t (id INT PRIMARY KEY) STATS_PERSISTENT=0 STATS_AUTO_RECALC=1 STATS_SAMPLE_PAGES=10 ROW_FORMAT=DYNAMIC;
CREATE TABLE gen (
  a INT, b INT AS (a*2) VIRTUAL, c INT AS (a+1) PERSISTENT,
  j LONGTEXT, jv VARCHAR(10) AS (json_value(j,'$.k')) STORED,
  CONSTRAINT chk_a CHECK (a > 0)
);
CREATE TABLE fk_parent (id INT PRIMARY KEY) ENGINE=InnoDB;
CREATE TABLE fk_child (id INT PRIMARY KEY, pid INT, KEY (pid), CONSTRAINT fk1 FOREIGN KEY (pid) REFERENCES fk_parent(id) ON DELETE CASCADE) ENGINE=InnoDB;
CREATE TABLE texts (id INT PRIMARY KEY, body TEXT DEFAULT 'x', blb BLOB, KEY kb (body(100))) ENGINE=InnoDB;
CREATE TABLE inv (id INT PRIMARY KEY, secret INT INVISIBLE) ENGINE=InnoDB;
CREATE TABLE versioned (id INT PRIMARY KEY, v INT) WITH SYSTEM VERSIONING;
CREATE TABLE part_t (id INT, d DATE, PRIMARY KEY (id, d)) PARTITION BY RANGE (YEAR(d)) (PARTITION p0 VALUES LESS THAN (2020), PARTITION pmax VALUES LESS THAN MAXVALUE);
CREATE TABLE mb3 (id INT PRIMARY KEY, s VARCHAR(10)) DEFAULT CHARSET=utf8mb3;
CREATE TABLE uca (id INT PRIMARY KEY, cs VARCHAR(10) COLLATE utf8mb4_uca1400_as_cs, ci VARCHAR(10) COLLATE utf8mb4_uca1400_ai_ci);
CREATE TABLE logs (msg VARCHAR(100), at DATETIME);
CREATE SEQUENCE seq_up START WITH 100 INCREMENT BY 5;
CREATE SEQUENCE seq_down START WITH 100 INCREMENT BY -5 MAXVALUE 100;
CREATE VIEW v_plain AS SELECT id, `key`, json_value('{"k":"v"}', '$.k') AS jv FROM plain WHERE `order` > 0;
DELIMITER ;;
CREATE TRIGGER trg_plain BEFORE INSERT ON plain FOR EACH ROW BEGIN SET NEW.`order` = NEW.`order` + 1; END;;
CREATE PROCEDURE p1() BEGIN SELECT 1; END;;
CREATE FUNCTION f1(x INT) RETURNS INT DETERMINISTIC RETURN x + 1;;
DELIMITER ;
CREATE EVENT ev1 ON SCHEDULE EVERY 1 DAY DO DELETE FROM plain WHERE id < 0;
INSERT INTO plain (id, `key`, note) VALUES (1, 'k1', 'generate a uuid here'), (2, 'k2', 'x char(36) not null y'), (3, 'k3', 'C:\\new\\path'), (4, 'k4', 'it''s'), (5, 'k5', 'utf8: café ✓'), (6, 'k6', 'nul:\0:');
INSERT INTO legacy (id, name) VALUES (1, 'Åsa');
INSERT INTO modern_types (id, u, ip4, ip6, doc, bits, s, g) VALUES (1, '123e4567-e89b-12d3-a456-426614174000', '10.0.0.1', '2001:db8::1', '{"a": [1, "two"]}', b'101', 'x,y', ST_GeomFromText('POINT(1 2)'));
INSERT INTO gen (a, j) VALUES (1, '{"k":"v"}');
INSERT INTO uca VALUES (1, 'Abc', 'éte');
INSERT INTO logs VALUES ('m1', '2026-10-22 10:00:00'), ('m2', NULL);
INSERT INTO inv (id, secret) VALUES (1, 42);
-- Cases from an adversarial review that used to lose data silently.
-- A routine created under NO_BACKSLASH_ESCAPES: in that mode 'C:\' is a
-- complete string, and the tables after it must survive.
SET sql_mode = CONCAT(@@sql_mode, ',NO_BACKSLASH_ESCAPES');
DELIMITER ;;
CREATE PROCEDURE p_nbe() SELECT 'C:\' AS path;;
DELIMITER ;
SET sql_mode = DEFAULT;
CREATE TABLE after_nbe (id INT PRIMARY KEY, p VARCHAR(20));
INSERT INTO after_nbe VALUES (1, 'D:\\x');
-- Case-sensitive collation: 'a' and 'A' are distinct under a unique key.
CREATE TABLE cs_t (id INT PRIMARY KEY, a VARCHAR(5) COLLATE latin1_general_cs, UNIQUE KEY ua (a)) DEFAULT CHARSET=latin1;
INSERT INTO cs_t VALUES (1, 'a'), (2, 'A');
-- JSON_VALUE over invalid JSON, JSON null and an array is NULL in MariaDB.
CREATE TABLE jdocs (id INT PRIMARY KEY, d LONGTEXT, v VARCHAR(20) AS (json_value(d, '$.k')) STORED);
INSERT INTO jdocs (id, d) VALUES (1, '{"k":"v"}'), (2, '{"k":null}'), (3, 'not json'), (4, '{"k":[1]}');
-- A "*/" inside a string of an executable comment (the dump wraps triggers).
DELIMITER ;;
CREATE TRIGGER trg_cmt BEFORE INSERT ON jdocs FOR EACH ROW SET NEW.d = REPLACE(NEW.d, '*/', '');;
DELIMITER ;
CREATE TABLE after_trg (id INT PRIMARY KEY);
INSERT INTO after_trg VALUES (1);
CREATE SEQUENCE seq_exhausted START WITH 3 MINVALUE 1 MAXVALUE 3 INCREMENT BY -1 NOCYCLE;
