-- Test fixture for mixed charset and collation scenarios
-- Combines utf8mb4_unicode_*, latin1_swedish_ci, and other collations

CREATE TABLE mixed_collations (
  id int NOT NULL AUTO_INCREMENT,
  utf8_field varchar(100) COLLATE utf8mb4_0900_ai_ci NOT NULL,
  latin1_field varchar(100) COLLATE utf8mb4_0900_ai_ci,
  another_utf8 text COLLATE utf8mb4_0900_ai_ci,
  normal_field varchar(50) NOT NULL,
  PRIMARY KEY (id),
  KEY idx_utf8 (utf8_field),
  KEY idx_latin1 (latin1_field)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE charset_only (
  id int NOT NULL AUTO_INCREMENT,
  content text NOT NULL,
  PRIMARY KEY (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE already_modern (
  id int NOT NULL AUTO_INCREMENT,
  name varchar(100) COLLATE utf8mb4_0900_ai_ci NOT NULL,
  description text,
  PRIMARY KEY (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
