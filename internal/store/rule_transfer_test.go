package store

import (
	"github.com/While-Shark/NodeSweep/internal/engine"
	"testing"
)

func transferRule(name string) engine.Rule {
	return engine.Rule{ID: "untrusted-id", Name: name, Scheme: "保守清理", Root: "/var/log/myapp", Patterns: []string{"*.log.*"}, Excludes: []string{"audit"}, KeepDays: 14}
}
func bundle(rules ...engine.Rule) engine.RuleBundle {
	return engine.RuleBundle{Format: engine.BundleFormat, Version: engine.BundleVersion, Rules: rules}
}
func TestRuleImportRegeneratesIDsAndDeduplicates(t *testing.T) {
	s := testStore(t)
	r := transferRule("归档")
	result, err := s.ImportRules(bundle(r, r))
	if err != nil {
		t.Fatal(err)
	}
	if result.Added != 1 || result.Skipped != 1 {
		t.Fatal(result)
	}
	rules, err := s.Rules()
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 1 || rules[0].ID == r.ID {
		t.Fatal("import retained an untrusted ID")
	}
	result, err = s.ImportRules(bundle(r))
	if err != nil {
		t.Fatal(err)
	}
	if result.Added != 0 || result.Skipped != 1 {
		t.Fatal(result)
	}
	r.KeepDays = 30
	result, err = s.ImportRules(bundle(r))
	if err != nil || result.Added != 1 {
		t.Fatal(result, err)
	}
	rules, _ = s.Rules()
	if len(rules) != 2 {
		t.Fatal("different rule was not preserved")
	}
}
func TestInvalidBundleWritesNothing(t *testing.T) {
	s := testStore(t)
	invalid := transferRule("bad")
	invalid.Root = "relative/path"
	if _, err := s.ImportRules(bundle(transferRule("valid"), invalid)); err == nil {
		t.Fatal("invalid path accepted")
	}
	rules, _ := s.Rules()
	if len(rules) != 0 {
		t.Fatal("partially imported invalid bundle")
	}
	for _, b := range []engine.RuleBundle{{Format: "other", Version: 1, Rules: []engine.Rule{transferRule("a")}}, {Format: engine.BundleFormat, Version: 2, Rules: []engine.Rule{transferRule("a")}}, bundle()} {
		if _, err := s.ImportRules(b); err == nil {
			t.Fatal("invalid format/count accepted")
		}
	}
}
func TestImportTransactionRollsBackOnWriteFailure(t *testing.T) {
	s := testStore(t)
	_, err := s.DB.Exec(`CREATE TRIGGER reject_bad_rule BEFORE INSERT ON rules WHEN json_extract(NEW.body,'$.name')='reject' BEGIN SELECT RAISE(ABORT,'test rejection'); END;`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.ImportRules(bundle(transferRule("first"), transferRule("reject"))); err == nil {
		t.Fatal("expected write failure")
	}
	rules, _ := s.Rules()
	if len(rules) != 0 {
		t.Fatal("first rule was committed before second failed")
	}
}
