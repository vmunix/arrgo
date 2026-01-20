package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSubstituteEnvVars_Simple(t *testing.T) {
	t.Setenv("TEST_VAR_SIMPLE", "hello")

	content, missing := substituteEnvVars("value = ${TEST_VAR_SIMPLE}")
	assert.Equal(t, "value = hello", content)
	assert.Empty(t, missing, "expected no missing vars")
}

func TestSubstituteEnvVars_Missing(t *testing.T) {
	// Use a unique var name that definitely doesn't exist
	// t.Setenv cannot truly unset, so we use a name we know is never set
	content, missing := substituteEnvVars("value = ${ARRGO_TEST_NONEXISTENT_VAR_12345}")
	assert.Equal(t, "value = ${ARRGO_TEST_NONEXISTENT_VAR_12345}", content)
	assert.Equal(t, []string{"ARRGO_TEST_NONEXISTENT_VAR_12345"}, missing)
}

func TestSubstituteEnvVars_Default(t *testing.T) {
	// Empty string should trigger default (same as unset for :- syntax)
	t.Setenv("UNSET_VAR_DEFAULT", "")

	content, missing := substituteEnvVars("value = ${UNSET_VAR_DEFAULT:-default_value}")
	assert.Equal(t, "value = default_value", content)
	assert.Empty(t, missing, "expected no missing vars with default")
}

func TestSubstituteEnvVars_DefaultOverriddenByEnv(t *testing.T) {
	t.Setenv("SET_VAR_OVERRIDE", "from_env")

	content, missing := substituteEnvVars("value = ${SET_VAR_OVERRIDE:-default}")
	assert.Equal(t, "value = from_env", content)
	assert.Empty(t, missing, "expected no missing vars")
}

func TestSubstituteEnvVars_RequiredError(t *testing.T) {
	// Empty string should trigger :? error (same as unset)
	t.Setenv("REQUIRED_VAR_TEST", "")

	content, missing := substituteEnvVars("value = ${REQUIRED_VAR_TEST:?API key is required}")
	assert.Equal(t, "value = ${REQUIRED_VAR_TEST:?API key is required}", content)
	assert.Equal(t, []string{"REQUIRED_VAR_TEST: API key is required"}, missing)
}

func TestSubstituteEnvVars_Multiple(t *testing.T) {
	t.Setenv("VAR1_MULTI", "one")
	// VAR2_MULTI_NONEXISTENT is never set - truly missing
	t.Setenv("VAR3_MULTI", "")

	content, missing := substituteEnvVars("${VAR1_MULTI} ${ARRGO_VAR2_NONEXISTENT} ${VAR3_MULTI:-three}")
	assert.Equal(t, "one ${ARRGO_VAR2_NONEXISTENT} three", content)
	assert.Equal(t, []string{"ARRGO_VAR2_NONEXISTENT"}, missing)
}
