package store

import (
	"encoding/json"
	"github.com/While-Shark/NodeSweep/internal/engine"
)

type ImportResult struct {
	Added   int `json:"added"`
	Skipped int `json:"skipped"`
}

// Imports append rules, never overwrite rules by IDs supplied in a file. Validate
// the whole bundle first, then commit all additions in one transaction. Repeated
// imports skip identical rules, avoiding accidental duplicate cleanup previews.
func (s *Store) ImportRules(bundle engine.RuleBundle) (ImportResult, error) {
	result := ImportResult{}
	if err := bundle.Validate(); err != nil {
		return result, err
	}
	tx, err := s.DB.Begin()
	if err != nil {
		return result, err
	}
	defer tx.Rollback()
	rows, err := tx.Query("SELECT body FROM rules")
	if err != nil {
		return result, err
	}
	existing := map[string]bool{}
	for rows.Next() {
		var body string
		var rule engine.Rule
		if err = rows.Scan(&body); err != nil {
			rows.Close()
			return result, err
		}
		if err = json.Unmarshal([]byte(body), &rule); err != nil {
			rows.Close()
			return result, err
		}
		rule.ID = ""
		key, _ := json.Marshal(rule)
		existing[string(key)] = true
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return result, err
	}
	for _, rule := range bundle.Rules {
		rule.ID = ""
		key, _ := json.Marshal(rule)
		if existing[string(key)] {
			result.Skipped++
			continue
		}
		rule.ID = engine.ID()
		body, err := json.Marshal(rule)
		if err != nil {
			return ImportResult{}, err
		}
		if _, err = tx.Exec("INSERT INTO rules(id,body) VALUES(?,?)", rule.ID, string(body)); err != nil {
			return ImportResult{}, err
		}
		existing[string(key)] = true
		result.Added++
	}
	if err = tx.Commit(); err != nil {
		return ImportResult{}, err
	}
	return result, nil
}
