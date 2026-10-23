package transformer

import (
	"github.com/developer-Bushido/mariadb2tidb/internal/compat"
	"github.com/developer-Bushido/mariadb2tidb/internal/config"
	"github.com/developer-Bushido/mariadb2tidb/internal/parser"
	"github.com/developer-Bushido/mariadb2tidb/internal/rules"
)

// TransformString runs the whole pipeline on a MariaDB script: preprocess
// and parse, apply the rules, write TiDB SQL, and check the output still
// has the input's tables, columns, keys and statements. Migration warnings
// go to report (may be nil). A nil cfg means the default configuration.
func TransformString(cfg *config.Config, sql string, report *compat.Reporter) (string, error) {
	if cfg == nil {
		cfg = config.DefaultConfig()
	}
	if err := rules.CheckRuleNames(cfg); err != nil {
		return "", err
	}
	stmts, err := parser.NewLoaderWithConfig(cfg).WithReporter(report).LoadFromString(sql)
	if err != nil {
		return "", err
	}
	shape := ShapeOf(stmts)
	transformed, err := NewEngineWithReporter(cfg, report).Transform(stmts)
	if err != nil {
		return "", err
	}
	out, err := parser.NewWriterWithConfig(cfg).WriteStatements(transformed)
	if err != nil {
		return "", err
	}
	if err := VerifyOutput(shape, out); err != nil {
		return "", err
	}
	return out, nil
}
