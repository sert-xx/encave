package scan

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTemp(t *testing.T, name, content string) (rel, abs string) {
	t.Helper()
	dir := t.TempDir()
	abs = filepath.Join(dir, name)
	if err := os.WriteFile(abs, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return name, abs
}

func TestDetectsKnownTokens(t *testing.T) {
	cases := map[string]string{
		"openai":     `api_key = "sk-abcdefghijklmnopqrstuvwxyz0123"`,
		"ghp":        `token: ghp_0123456789abcdefABCDEF0123456789wxyz`,
		"github_pat": `GITHUB_PAT=github_pat_11ABCDEFG0aBcDeFgHiJkLmNoPqRsTuVwXyZ`,
		"aws":        `aws_access_key_id = AKIAIOSFODNN7EXAMPLE`,
		"pem":        "-----BEGIN RSA PRIVATE KEY-----",
	}
	for name, line := range cases {
		t.Run(name, func(t *testing.T) {
			rel, abs := writeTemp(t, "f.txt", line+"\n")
			f, err := File(rel, abs)
			if err != nil {
				t.Fatal(err)
			}
			if len(f) == 0 {
				t.Fatalf("expected a finding for %q, got none", line)
			}
		})
	}
}

func TestDetectsKnownSecretFilename(t *testing.T) {
	f := FilenameFindings("some/dir/auth.json")
	if len(f) == 0 {
		t.Fatal("expected auth.json to be flagged by filename")
	}
}

func TestDetectsHighEntropyValue(t *testing.T) {
	// A 40-char random-looking base64 string assigned to a generic key.
	rel, abs := writeTemp(t, "f.txt", `value = "Zx8Kq2Lp9Wm4Rt7Yn1Bv6Cs3Df0Hg5Jk8Ll2Pq"`+"\n")
	f, _ := File(rel, abs)
	if len(f) == 0 {
		t.Fatal("expected high-entropy value to be flagged")
	}
}

func TestIgnoresPlaceholdersAndPaths(t *testing.T) {
	lines := []string{
		`token = "${MY_TOKEN}"`,
		`api_key = "your-api-key-here"`,
		`base_url = "https://proxy.internal.example.com/v1/bedrock"`,
		`name = "com.example.some.long.package.identifier"`,
		`env_key = "PROXY_TOKEN"`,
	}
	for _, l := range lines {
		rel, abs := writeTemp(t, "config.toml", l+"\n")
		f, _ := File(rel, abs)
		if len(f) != 0 {
			t.Errorf("did not expect a finding for %q, got %+v", l, f)
		}
	}
}

func TestEntropy(t *testing.T) {
	if shannonEntropy("aaaaaaaa") > 0.01 {
		t.Error("expected ~0 entropy for repeated chars")
	}
	if shannonEntropy("Zx8Kq2Lp9Wm4Rt7Yn1Bv6Cs3Df0Hg5Jk8Ll2Pq") < entropyThreshold {
		t.Error("expected high entropy for random-looking string")
	}
}

func TestRedactNeverRevealsFull(t *testing.T) {
	secret := "sk-abcdefghijklmnopqrstuvwxyz"
	r := redact(secret)
	if r == secret {
		t.Fatal("redact returned the full secret")
	}
	if len(r) >= len(secret) {
		t.Fatal("redacted form should be shorter than the secret")
	}
}
