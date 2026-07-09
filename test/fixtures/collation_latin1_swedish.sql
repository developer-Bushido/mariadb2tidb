-- latin1/latin1_swedish_ci should be converted to utf8mb4/utf8mb4_0900_ai_ci

CREATE TABLE `legacy_table` (
  `id` int NOT NULL AUTO_INCREMENT,
  `name` varchar(100) COLLATE latin1_swedish_ci NOT NULL,
  `description` text COLLATE latin1_swedish_ci,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=latin1 COLLATE=latin1_swedish_ci;

CREATE TABLE `mixed_legacy` (
  `id` int NOT NULL AUTO_INCREMENT,
  `legacy_name` varchar(100) COLLATE latin1_swedish_ci NOT NULL,
  `modern_name` varchar(100) NOT NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=latin1;
