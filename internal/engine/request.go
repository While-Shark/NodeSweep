package engine

import (
	"encoding/hex"
	"errors"
	"path/filepath"
	"strings"
)

func ValidID(id string) bool {
	b, err := hex.DecodeString(id)
	return err == nil && len(b) == 16 && len(id) == 32
}

func ValidateRequest(r Request) error {
	switch r.Kind {
	case "scan":
		if !filepath.IsAbs(r.Path) || len(r.Path) > 4096 || strings.ContainsRune(r.Path, 0) {
			return errors.New("absolute path required")
		}
	case "preview":
		return ValidateRule(r.Rule)
	case "execute":
		if !ValidID(r.PlanID) {
			return errors.New("invalid preview identifier")
		}
	case "detect":
	default:
		return errors.New("unsupported operation")
	}
	return nil
}
