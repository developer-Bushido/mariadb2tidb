package rules_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/developer-Bushido/mariadb2tidb/internal/compat"
	"github.com/developer-Bushido/mariadb2tidb/internal/config"
	"github.com/developer-Bushido/mariadb2tidb/internal/parser"
	"github.com/developer-Bushido/mariadb2tidb/internal/rules"
	"github.com/developer-Bushido/mariadb2tidb/internal/transformer"
	"github.com/developer-Bushido/mariadb2tidb/internal/utils"
)

// run pushes sql through the full pipeline and returns the output (on one
// line) and the warnings.
func run(t *testing.T, cfg *config.Config, sql string) (string, []string) {
	t.Helper()
	require.NoError(t, utils.InitLogger(true))
	rep := compat.NewReporter()
	stmts, err := parser.NewLoaderWithConfig(cfg).WithReporter(rep).LoadFromString(sql)
	require.NoError(t, err)
	shape := transformer.ShapeOf(stmts)
	out, err := transformer.NewEngineWithReporter(cfg, rep).Transform(stmts)
	require.NoError(t, err)
	got, err := parser.NewWriterWithConfig(cfg).WriteStatements(out)
	require.NoError(t, err)
	require.NoError(t, transformer.VerifyOutput(shape, got))
	return strings.Join(strings.Fields(got), " "), rep.Warnings()
}

func TestEngineRule(t *testing.T) {
	got, warns := run(t, config.DefaultConfig(), "CREATE TABLE a (id INT) ENGINE=Aria; CREATE TABLE b (id INT) ENGINE=MyISAM; ALTER TABLE c ENGINE=ColumnStore; CREATE TABLE h (id INT) ENGINE=HEAP;")
	assert.Contains(t, got, "`a` ( `id` INT ) ENGINE = InnoDB")
	assert.Contains(t, got, "ENGINE = MyISAM", "engines TiDB accepts are kept")
	assert.Contains(t, got, "ALTER TABLE `c` ENGINE = InnoDB")
	assert.Len(t, warns, 2)
}

func TestJSONValueRule(t *testing.T) {
	got, _ := run(t, config.DefaultConfig(), "CREATE TABLE t (j LONGTEXT, v VARCHAR(10) AS (json_value(j,'$.k')) STORED); CREATE VIEW w AS SELECT JSON_VALUE(j, '$.a') AS a FROM t;")
	assert.Contains(t, got, "CASE WHEN JSON_VALID(`j`) AND JSON_TYPE(JSON_EXTRACT(`j`, _UTF8MB4'$.k')) NOT IN ('OBJECT','ARRAY','NULL') THEN JSON_UNQUOTE(JSON_EXTRACT(`j`, _UTF8MB4'$.k')) END")
	assert.Contains(t, got, "JSON_UNQUOTE(JSON_EXTRACT(`j`, _UTF8MB4'$.a'))")
	assert.NotContains(t, strings.ToLower(got), "json_value")

	got, _ = run(t, config.DefaultConfig(), "CREATE VIEW n AS SELECT JSON_VALUE(JSON_VALUE(d, '$.a'), '$.b') AS x FROM t;")
	assert.NotContains(t, strings.ToLower(got), "json_value", "nested calls are rewritten too")
}

func TestZeroDateNonCanonicalForms(t *testing.T) {
	sql := "CREATE TABLE t (a TIMESTAMP DEFAULT 0, b DATE DEFAULT '00000000000000', c DATE DEFAULT '0000/00/00', d DATETIME DEFAULT '0', e DATE DEFAULT 20200010, f DATE DEFAULT 20200101, g DATE DEFAULT '2020/1/1');"
	_, warns := run(t, config.DefaultConfig(), sql)
	assert.Len(t, warns, 5, "a-e are zero dates, f and g are not")
}

func TestZeroDateRule(t *testing.T) {
	sql := "CREATE TABLE t (a DATETIME NOT NULL DEFAULT '0000-00-00 00:00:00', b DATE DEFAULT '2020-00-10', c DATE DEFAULT '0000-01-01', d DATETIME DEFAULT NULL, e VARCHAR(20) DEFAULT '0000-00-00');"
	got, warns := run(t, config.DefaultConfig(), sql)
	assert.Contains(t, got, "DEFAULT _UTF8MB4'0000-00-00 00:00:00'", "keep is the default")
	assert.Len(t, warns, 2, "a and b; year 0 alone, NULL and non-date columns are fine")

	cfg := config.DefaultConfig()
	cfg.ZeroDateDefaults = "drop"
	got, _ = run(t, cfg, sql)
	assert.Contains(t, got, "`a` DATETIME NOT NULL,")
	assert.Contains(t, got, "`b` DATE,")
	assert.Contains(t, got, "`c` DATE DEFAULT _UTF8MB4'0000-01-01'")
	assert.Contains(t, got, "`e` VARCHAR(20) DEFAULT _UTF8MB4'0000-00-00'")
}

func TestAutoRandomRule(t *testing.T) {
	sql := "CREATE TABLE a (id BIGINT NOT NULL AUTO_INCREMENT, PRIMARY KEY (id));" +
		"CREATE TABLE b (id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY);" +
		"CREATE TABLE c (id INT NOT NULL AUTO_INCREMENT PRIMARY KEY);" +
		"CREATE TABLE d (id BIGINT AUTO_INCREMENT, PRIMARY KEY (id) NONCLUSTERED);" +
		"CREATE TABLE e (id BIGINT AUTO_INCREMENT PRIMARY KEY);"

	got, _ := run(t, config.DefaultConfig(), sql)
	assert.NotContains(t, got, "AUTO_RANDOM", "off unless tables are listed")

	cfg := config.DefaultConfig()
	cfg.AutoRandom.Tables = []string{"*"}
	got, warns := run(t, cfg, sql)
	assert.Contains(t, got, "`a` ( `id` BIGINT NOT NULL AUTO_RANDOM(5, 54), PRIMARY KEY(`id`) CLUSTERED )")
	assert.Contains(t, got, "`e` ( `id` BIGINT AUTO_RANDOM(5, 54) PRIMARY KEY CLUSTERED )")
	assert.Contains(t, got, "`b` ( `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY )", "UNSIGNED is refused")
	assert.Contains(t, got, "`c` ( `id` INT NOT NULL AUTO_INCREMENT PRIMARY KEY )")
	assert.Contains(t, got, "PRIMARY KEY(`id`) NONCLUSTERED")
	joined := strings.Join(warns, "\n")
	assert.Contains(t, joined, "PHP_INT_MAX")
	assert.Contains(t, joined, "not BIGINT")
	assert.Contains(t, joined, "NONCLUSTERED")

	cfg.AutoRandom = config.AutoRandomConfig{Tables: []string{"A"}, ShardBits: 4, RangeBits: 64}
	got, warns = run(t, cfg, sql)
	assert.Contains(t, got, "AUTO_RANDOM(4, 64)")
	assert.Equal(t, 1, strings.Count(got, "AUTO_RANDOM"), "only the listed table")
	assert.Contains(t, strings.Join(warns, "\n"), "2^53")
}

func TestShardRowIDRule(t *testing.T) {
	sql := "CREATE TABLE nopk (v INT); CREATE TABLE intpk (id INT PRIMARY KEY); CREATE TABLE comp (a INT, b INT, PRIMARY KEY (a, b));" +
		"CREATE TABLE nc (id INT, PRIMARY KEY (id) NONCLUSTERED); CREATE TABLE has (v INT) SHARD_ROW_ID_BITS=2;"
	cfg := config.DefaultConfig()
	cfg.ShardRowIDBits, cfg.PreSplitRegions = 4, 2
	got, _ := run(t, cfg, sql)
	assert.Contains(t, got, "`nopk` ( `v` INT ) SHARD_ROW_ID_BITS = 4 PRE_SPLIT_REGIONS = 2")
	assert.Contains(t, got, "NONCLUSTERED ) SHARD_ROW_ID_BITS = 4")
	assert.Contains(t, got, "`has` ( `v` INT ) SHARD_ROW_ID_BITS = 2;")
	assert.Equal(t, 3, strings.Count(got, "SHARD_ROW_ID_BITS"), "clustered keys are left alone")

	got, _ = run(t, cfg, "CREATE TEMPORARY TABLE tmp (v INT);")
	assert.NotContains(t, got, "SHARD_ROW_ID_BITS", "TiDB rejects it on temporary tables")
}

func TestGeneratedValuesRule(t *testing.T) {
	sql := "CREATE TABLE g (a INT, b INT AS (a*2) VIRTUAL, c INT AS (a+1) STORED, d INT);" +
		"INSERT INTO g VALUES (1,2,2,9),(3,6,4,9);" +
		"INSERT INTO g (`d`, `c`, `a`) VALUES (9, 4, 3);" +
		"REPLACE INTO g VALUES (5,10,6,9);" +
		"INSERT INTO other VALUES (1,2,3);" +
		"INSERT INTO g (a, d) VALUES (7, 7);"
	got, warns := run(t, config.DefaultConfig(), sql)
	assert.Contains(t, got, "INSERT INTO `g` VALUES (1,DEFAULT,DEFAULT,9),(3,DEFAULT,DEFAULT,9);")
	assert.Contains(t, got, "INSERT INTO `g` (`d`,`c`,`a`) VALUES (9,DEFAULT,3);")
	assert.Contains(t, got, "REPLACE INTO `g` VALUES (5,DEFAULT,DEFAULT,9);")
	assert.Contains(t, got, "INSERT INTO `other` VALUES (1,2,3);", "unknown tables are left alone")
	assert.Contains(t, got, "INSERT INTO `g` (`a`,`d`) VALUES (7,7);")
	assert.Len(t, warns, 2) // "b, c" and "c"
}

func TestGeneratedValuesFollowsDatabases(t *testing.T) {
	sql := "CREATE TABLE d1.t (a INT, b INT, c INT);" +
		"USE d2; CREATE TABLE t (a INT, b INT AS (a+1) STORED, c INT);" +
		"INSERT INTO d1.t VALUES (1,50,7);" + // plain columns in d1: kept
		"INSERT INTO t VALUES (1,50,7);" + // d2.t via USE
		"INSERT INTO d2.t (a, b) VALUES (1, 2) ON DUPLICATE KEY UPDATE b = VALUES(b), c = 1;" +
		"INSERT INTO d2.t (a, b) SELECT a, b FROM d1.t;"
	got, warns := run(t, config.DefaultConfig(), sql)
	assert.Contains(t, got, "INSERT INTO `d1`.`t` VALUES (1,50,7);")
	assert.Contains(t, got, "INSERT INTO `t` VALUES (1,DEFAULT,7);")
	assert.Contains(t, got, "VALUES (1,DEFAULT) ON DUPLICATE KEY UPDATE `c`=1;")
	joined := strings.Join(warns, "\n")
	assert.Contains(t, joined, "INSERT ... SELECT writes generated columns b")
	assert.Contains(t, joined, "ON DUPLICATE KEY UPDATE of generated columns dropped")
}

func TestRulesSurviveColumnsWithoutType(t *testing.T) {
	// ALTER COLUMN ... SET DEFAULT has a ColumnDef without a type; this
	// used to panic in TextBlobDefaults.
	got, _ := run(t, config.DefaultConfig(), "ALTER TABLE t ALTER COLUMN d SET DEFAULT 1, ALTER COLUMN e DROP DEFAULT;")
	assert.Contains(t, got, "ALTER COLUMN `d` SET DEFAULT 1")
}

func TestEngineRuleMergeTables(t *testing.T) {
	got, warns := run(t, config.DefaultConfig(), "CREATE TABLE m (a INT) ENGINE=MRG_MyISAM DEFAULT CHARSET=utf8mb4 INSERT_METHOD=LAST UNION=(`a`,`b`);")
	assert.Contains(t, got, "ENGINE = InnoDB DEFAULT CHARACTER SET = UTF8MB4;")
	assert.Contains(t, strings.Join(warns, "\n"), "MERGE table became an ordinary empty table")

	got, _ = run(t, config.DefaultConfig(), "ALTER TABLE m UNION=(`a`), INSERT_METHOD=FIRST, ENGINE=MRG_MyISAM;")
	assert.NotContains(t, got, "UNION")
	assert.NotContains(t, got, "INSERT_METHOD")
}

func TestCheckRuleNames(t *testing.T) {
	cfg := config.DefaultConfig()
	require.NoError(t, rules.CheckRuleNames(cfg))
	cfg.EnabledRules = []string{"ZeroDat"}
	assert.ErrorContains(t, rules.CheckRuleNames(cfg), `unknown rule "ZeroDat"`)
	_, err := transformer.TransformString(cfg, "CREATE TABLE t (a INT);", nil)
	assert.Error(t, err, "the pipeline refuses it too")
}

func TestCollationRuleKeepsExplicitCollate(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.CharsetMappings["utf8mb4"] = config.CharsetMapping{TargetCharset: "utf8mb4", TargetCollation: "utf8mb4_0900_ai_ci"}
	got, _ := run(t, cfg, "CREATE DATABASE d CHARACTER SET utf8mb4 COLLATE utf8mb4_bin; CREATE TABLE a (x VARCHAR(5)) DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin; CREATE TABLE b (x VARCHAR(5)) DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_nopad_bin; CREATE TABLE c (x INT) DEFAULT CHARSET=utf8mb4;")
	lower := strings.ToLower(got)
	assert.Contains(t, lower, "collate = utf8mb4_bin;", "database keeps its explicit collation")
	assert.Contains(t, lower, "`a` ( `x` varchar(5) ) default character set = utf8mb4 default collate = utf8mb4_bin")
	assert.Contains(t, lower, "default collate = utf8mb4_0900_bin", "nopad_bin as resolved by the loader")
	assert.Contains(t, lower, "`c` ( `x` int ) default character set = utf8mb4 default collate = utf8mb4_0900_ai_ci", "the mapping's collation only where none is given")
}
