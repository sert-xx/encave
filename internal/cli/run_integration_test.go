package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sert-xx/encave/internal/agentmeta"
	"github.com/sert-xx/encave/internal/secrets"
	"github.com/zalando/go-keyring"
)

// captureStdout runs fn with os.Stdout redirected to a buffer and returns what
// was written.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	done := make(chan string)
	go func() {
		var sb strings.Builder
		buf := make([]byte, 4096)
		for {
			n, e := r.Read(buf)
			sb.Write(buf[:n])
			if e != nil {
				break
			}
		}
		done <- sb.String()
	}()
	fn()
	w.Close()
	os.Stdout = orig
	return <-done
}

// TestRunDryRunInjectsAuthAndRedacts is the end-to-end guard for the security
// promise: the credential is injected into the child env, but its value never
// appears in encave's own output.
func TestRunDryRunInjectsAuthAndRedacts(t *testing.T) {
	keyring.MockInit()

	root := t.TempDir()
	t.Setenv("ENCAVE_ROOT", root)

	agentDir := filepath.Join(root, "dai", "review-agent")
	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := `model = "internal-model"
[model_providers.proxy]
base_url = "https://proxy.example.com/v1"
env_key = "PROXY_TOKEN"
`
	if err := os.WriteFile(filepath.Join(agentDir, "config.toml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := agentmeta.Save(agentDir, agentmeta.Meta{Target: "codex"}); err != nil {
		t.Fatal(err)
	}

	const secret = "supersecret-token-value-DO-NOT-LEAK-9999"
	if err := secrets.Set(secrets.GlobalScope, secret); err != nil {
		t.Fatal(err)
	}

	var code int
	out := captureStdout(t, func() {
		code = cmdRun([]string{"dai/review-agent", "--dry-run", "--", "exec", "review"})
	})
	if code != 0 {
		t.Fatalf("cmdRun exit = %d", code)
	}

	if strings.Contains(out, secret) {
		t.Fatalf("SECURITY: secret value leaked into output:\n%s", out)
	}
	if !strings.Contains(out, "***redacted***") {
		t.Errorf("expected redacted auth marker in output:\n%s", out)
	}
	if !strings.Contains(out, "CODEX_HOME="+agentDir) {
		t.Errorf("expected CODEX_HOME set to agent dir:\n%s", out)
	}
	if !strings.Contains(out, "PROXY_TOKEN") {
		t.Errorf("expected PROXY_TOKEN to be the injected auth var:\n%s", out)
	}
	if !strings.Contains(out, "exec review") {
		t.Errorf("expected user args forwarded to target:\n%s", out)
	}
}

// TestRunMissingAgent reports a clear error when the agent isn't installed.
func TestRunMissingAgent(t *testing.T) {
	keyring.MockInit()
	t.Setenv("ENCAVE_ROOT", t.TempDir())
	if code := cmdRun([]string{"nobody/missing", "--dry-run"}); code == 0 {
		t.Fatal("expected non-zero exit for missing agent")
	}
}
