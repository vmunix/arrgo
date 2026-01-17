// internal/config/envsubst_test.go
package config

import (
	"os"
	"testing"
)

func TestSubstituteEnvVars_Simple(t *testing.T) {
	os.Setenv("TEST_VAR", "hello")
	defer os.Unsetenv("TEST_VAR")

	content, missing := substituteEnvVars("value = ${TEST_VAR}")
	if content != "value = hello" {
		t.Errorf("expected 'value = hello', got %q", content)
	}
	if len(missing) != 0 {
		t.Errorf("expected no missing vars, got %v", missing)
	}
}

func TestSubstituteEnvVars_Missing(t *testing.T) {
	os.Unsetenv("MISSING_VAR")

	content, missing := substituteEnvVars("value = ${MISSING_VAR}")
	if content != "value = ${MISSING_VAR}" {
		t.Errorf("expected unchanged, got %q", content)
	}
	if len(missing) != 1 || missing[0] != "MISSING_VAR" {
		t.Errorf("expected [MISSING_VAR], got %v", missing)
	}
}

func TestSubstituteEnvVars_Default(t *testing.T) {
	os.Unsetenv("UNSET_VAR")

	content, missing := substituteEnvVars("value = ${UNSET_VAR:-default_value}")
	if content != "value = default_value" {
		t.Errorf("expected 'value = default_value', got %q", content)
	}
	if len(missing) != 0 {
		t.Errorf("expected no missing vars with default, got %v", missing)
	}
}

func TestSubstituteEnvVars_DefaultOverriddenByEnv(t *testing.T) {
	os.Setenv("SET_VAR", "from_env")
	defer os.Unsetenv("SET_VAR")

	content, missing := substituteEnvVars("value = ${SET_VAR:-default}")
	if content != "value = from_env" {
		t.Errorf("expected 'value = from_env', got %q", content)
	}
	if len(missing) != 0 {
		t.Errorf("expected no missing vars, got %v", missing)
	}
}

func TestSubstituteEnvVars_RequiredError(t *testing.T) {
	os.Unsetenv("REQUIRED_VAR")

	content, missing := substituteEnvVars("value = ${REQUIRED_VAR:?API key is required}")
	if content != "value = ${REQUIRED_VAR:?API key is required}" {
		t.Errorf("expected unchanged, got %q", content)
	}
	if len(missing) != 1 || missing[0] != "REQUIRED_VAR: API key is required" {
		t.Errorf("expected error message, got %v", missing)
	}
}

func TestSubstituteEnvVars_Multiple(t *testing.T) {
	os.Setenv("VAR1", "one")
	os.Unsetenv("VAR2")
	os.Unsetenv("VAR3")
	defer os.Unsetenv("VAR1")

	content, missing := substituteEnvVars("${VAR1} ${VAR2} ${VAR3:-three}")
	if content != "one ${VAR2} three" {
		t.Errorf("expected 'one ${VAR2} three', got %q", content)
	}
	if len(missing) != 1 || missing[0] != "VAR2" {
		t.Errorf("expected [VAR2], got %v", missing)
	}
}
