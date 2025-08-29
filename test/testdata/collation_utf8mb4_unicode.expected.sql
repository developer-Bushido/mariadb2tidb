-- Test fixture for utf8mb4_unicode_* collation transformations
-- These should be transformed to utf8mb4_0900_ai_ci

CREATE TABLE users (
  id int NOT NULL AUTO_INCREMENT,
  username varchar(50) COLLATE utf8mb4_0900_ai_ci NOT NULL,
  email varchar(255) COLLATE utf8mb4_0900_ai_ci NOT NULL,
  display_name varchar(100) COLLATE utf8mb4_0900_ai_ci DEFAULT NULL,
  bio text COLLATE utf8mb4_0900_ai_ci,
  PRIMARY KEY (id),
  UNIQUE KEY unique_username (username),
  UNIQUE KEY unique_email (email)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE posts (
  id bigint NOT NULL AUTO_INCREMENT,
  title varchar(200) COLLATE utf8mb4_0900_ai_ci NOT NULL,
  content longtext COLLATE utf8mb4_0900_ai_ci,
  author_id int NOT NULL,
  PRIMARY KEY (id),
  KEY idx_author (author_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
