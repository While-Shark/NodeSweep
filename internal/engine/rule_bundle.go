package engine

import "fmt"

const BundleFormat = "nodesweep.rules"
const BundleVersion = 1
const MaxBundleRules = 100

type RuleBundle struct {
	Format  string `json:"format"`
	Version int    `json:"version"`
	Rules   []Rule `json:"rules"`
}

func (b RuleBundle) Validate() error {
	if b.Format != BundleFormat || b.Version != BundleVersion {
		return fmt.Errorf("unsupported rule bundle format or version")
	}
	if len(b.Rules) < 1 || len(b.Rules) > MaxBundleRules {
		return fmt.Errorf("a bundle must contain 1–%d rules", MaxBundleRules)
	}
	for i, r := range b.Rules {
		if err := ValidateRule(r); err != nil {
			return fmt.Errorf("rule %d: %w", i+1, err)
		}
	}
	return nil
}
