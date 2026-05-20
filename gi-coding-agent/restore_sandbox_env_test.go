package gicodingagent

import "testing"

func TestRestoreSandboxEnvPiCases(t *testing.T) {
	env := map[string]string{"EXISTING": "1"}
	called := false
	if RestoreSandboxEnv(false, env, func() (string, error) {
		called = true
		return "FOO=bar\x00", nil
	}) {
		t.Fatalf("non-bun process should not restore")
	}
	if called || env["EXISTING"] != "1" {
		t.Fatalf("non-bun env changed: %#v called=%v", env, called)
	}

	env = map[string]string{"RESTORE_SANDBOX_ENV_TEST": "1"}
	if RestoreSandboxEnv(true, env, func() (string, error) {
		t.Fatalf("non-empty bun env should not read environ")
		return "", nil
	}) {
		t.Fatalf("non-empty env should not restore")
	}
	if env["RESTORE_SANDBOX_ENV_TEST"] != "1" {
		t.Fatalf("non-empty env changed: %#v", env)
	}

	env = map[string]string{}
	if !RestoreSandboxEnv(true, env, func() (string, error) {
		return "FOO=bar\x00BAZ=qux\x00", nil
	}) {
		t.Fatalf("empty bun env should restore")
	}
	if env["FOO"] != "bar" || env["BAZ"] != "qux" {
		t.Fatalf("restored env = %#v", env)
	}
}
