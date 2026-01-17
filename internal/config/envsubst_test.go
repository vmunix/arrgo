package config

import (
	"testing"
)

func TestSubstituteEnvVars_Simple(t *testing.T) {
	t.Setenv("TEST_VAR_SIMPLE", "hello")

	content, missing := substituteEnvVars("value = ${TEST_VAR_SIMPLE}")
	if content != "value = hello" {
		t.Errorf("expected 'value = hello', got %q", content)
	}
	if len(missing) != 0 {
		t.Errorf("expected no missing vars, got %v", missing)
	}
}

func TestSubstituteEnvVars_Missing(t *testing.T) {
	// Use a unique var name that definitely doesn't exist
	// t.Setenv cannot truly unset, so we use a name we know is never set
	content, missing := substituteEnvVars("value = ${ARRGO_TEST_NONEXISTENT_VAR_12345}")
	if content != "value = ${ARRGO_TEST_NONEXISTENT_VAR_12345}" {
		t.Errorf("expected unchanged, got %q", content)
	}
	if len(missing) != 1 || missing[0] != "ARRGO_TEST_NONEXISTENT_VAR_12345" {
		t.Errorf("expected [ARRGO_TEST_NONEXISTENT_VAR_12345], got %v", missing)
	}
}

func TestSubstituteEnvVars_Default(t *testing.T) {
	// Empty string should trigger default (same as unset for :- syntax)
	t.Setenv("UNSET_VAR_DEFAULT", "")

	content, missing := substituteEnvVars("value = ${UNSET_VAR_DEFAULT:-default_value}")
	if content != "value = default_value" {
		t.Errorf("expected 'value = default_value', got %q", content)
	}
	if len(missing) != 0 {
		t.Errorf("expected no missing vars with default, got %v", missing)
	}
}

func TestSubstituteEnvVars_DefaultOverriddenByEnv(t *testing.T) {
	t.Setenv("SET_VAR_OVERRIDE", "from_env")

	content, missing := substituteEnvVars("value = ${SET_VAR_OVERRIDE:-default}")
	if content != "value = from_env" {
		t.Errorf("expected 'value = from_env', got %q", content)
	}
	if len(missing) != 0 {
		t.Errorf("expected no missing vars, got %v", missing)
	}
}

func TestSubstituteEnvVars_RequiredError(t *testing.T) {
	// Empty string should trigger :? error (same as unset)
	t.Setenv("REQUIRED_VAR_TEST", "")

	content, missing := substituteEnvVars("value = ${REQUIRED_VAR_TEST:?API key is required}")
	if content != "value = ${REQUIRED_VAR_TEST:?API key is required}" {
		t.Errorf("expected unchanged, got %q", content)
	}
	if len(missing) != 1 || missing[0] != "REQUIRED_VAR_TEST: API key is required" {
		t.Errorf("expected error message, got %v", missing)
	}
}

func TestSubstituteEnvVars_Multiple(t *testing.T) {
	t.Setenv("VAR1_MULTI", "one")
	// VAR2_MULTI_NONEXISTENT is never set - truly missing
	t.Setenv("VAR3_MULTI", "")

	content, missing := substituteEnvVars("${VAR1_MULTI} ${ARRGO_VAR2_NONEXISTENT} ${VAR3_MULTI:-three}")
	if content != "one ${ARRGO_VAR2_NONEXISTENT} three" {
		t.Errorf("expected 'one ${ARRGO_VAR2_NONEXISTENT} three', got %q", content)
	}
	if len(missing) != 1 || missing[0] != "ARRGO_VAR2_NONEXISTENT" {
		t.Errorf("expected [ARRGO_VAR2_NONEXISTENT], got %v", missing)
	}
}
