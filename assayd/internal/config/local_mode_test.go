package config

import (
	"strings"
	"testing"
)

func TestLocalModeMakesOnlyAdminTokenOptional(t *testing.T) {
	for _, flag := range []string{"true", "false", "", "invalid"} {
		t.Run(flag, func(t *testing.T) {
			environment := validEnvironment()
			delete(environment, "ASSAY_ADMIN_TOKEN")
			if flag != "" {
				environment["ASSAY_LOCAL_MODE"] = flag
			}
			_, err := parse(environment)
			if (err == nil) != (flag == "true") {
				t.Fatalf("local mode %q without admin token: %v", flag, err)
			}
		})
	}
	for _, key := range []string{"ASSAY_DATABASE_URL", "ASSAY_ENCRYPTION_KEY"} {
		environment := validEnvironment()
		environment["ASSAY_LOCAL_MODE"] = "true"
		delete(environment, key)
		if _, err := parse(environment); err == nil || !strings.Contains(err.Error(), key) {
			t.Fatalf("local mode must still require %s: %v", key, err)
		}
	}
}
