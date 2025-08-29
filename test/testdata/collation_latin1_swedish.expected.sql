-- Test fixture for latin1_swedish_ci collation transformations
-- These should be transformed to utf8mb4_0900_ai_ci

CREATE TABLE legacy_table (
  id int NOT NULL AUTO_INCREMENT,
  name varchar(100) COLLATE utf8mb4_0900_ai_ci NOT NULL,
  description text COLLATE utf8mb4_0900_ai_ci,
  PRIMARY KEY (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE another_legacy (
  id int NOT NULL AUTO_INCREMENT,
  old_field varchar(50) COLLATE utf8mb4_0900_ai_ci,
  PRIMARY KEY (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE mixed_legacy (
  id int NOT NULL AUTO_INCREMENT,
  legacy_name varchar(100) COLLATE utf8mb4_0900_ai_ci NOT NULL,
  modern_name varchar(100) NOT NULL,
  PRIMARY KEY (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
