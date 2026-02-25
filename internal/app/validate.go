package app

import (
	"fmt"
	"regexp"
)

var profileNamePattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

func validateProfileName(name string) error {
	if name == "" {
		return fmt.Errorf("profile name is required")
	}
	if !profileNamePattern.MatchString(name) {
		return fmt.Errorf("invalid profile name %q (allowed: letters, numbers, ., _, -)", name)
	}
	return nil
}

func redactSecret(value string) string {
	if len(value) <= 8 {
		return "****"
	}
	return value[:4] + "..." + value[len(value)-4:]
}
