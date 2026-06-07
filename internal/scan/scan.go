// Package scan implements encave's fail-closed secret scanner. It is the last
// line of defense before an agent is published: if anything that looks like a
// credential is staged, publish aborts (design doc §4.2, §6).
//
// The scanner reports findings for three classes of problem:
//  1. Known credential filenames (auth.json, .env, private keys, ...).
//  2. Token-shaped strings matched by well-known provider prefixes.
//  3. High-entropy strings that resemble secrets even without a known prefix.
//
// It is intentionally biased toward false positives over false negatives: a
// noisy stop is recoverable, a leaked token is not.
package scan

import (
	"bufio"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Finding describes one suspected secret.
type Finding struct {
	File   string // path as supplied to the scanner (usually repo-relative)
	Line   int    // 1-based line number, or 0 for filename-level findings
	Reason string // human-readable explanation
	Sample string // short, redacted excerpt for context (never the full secret)
}

// knownSecretFilenames are basenames that should essentially never be committed.
var knownSecretFilenames = map[string]string{
	"auth.json":       "Codex credential file",
	".env":            "environment/secret file",
	".netrc":          "netrc credential file",
	"credentials":     "credentials file",
	"id_rsa":          "private SSH key",
	"id_ed25519":      "private SSH key",
	".npmrc":          "may contain npm auth token",
	".pypirc":         "may contain PyPI credentials",
	".dockercfg":      "Docker registry credentials",
	"config.json.bak": "possible credential backup",
}

// secretFileSuffixes are extensions that commonly hold private key material.
var secretFileSuffixes = map[string]string{
	".pem":      "PEM-encoded key/cert material",
	".key":      "private key material",
	".pfx":      "PKCS#12 key bundle",
	".p12":      "PKCS#12 key bundle",
	".keystore": "Java keystore",
}

// tokenPatterns matches well-known credential prefixes/shapes.
var tokenPatterns = []struct {
	re     *regexp.Regexp
	reason string
}{
	{regexp.MustCompile(`sk-[A-Za-z0-9_-]{16,}`), "OpenAI-style secret key (sk-)"},
	{regexp.MustCompile(`sk-ant-[A-Za-z0-9_-]{16,}`), "Anthropic API key (sk-ant-)"},
	{regexp.MustCompile(`github_pat_[A-Za-z0-9_]{20,}`), "GitHub fine-grained PAT"},
	{regexp.MustCompile(`gh[pousr]_[A-Za-z0-9]{20,}`), "GitHub token (ghp_/gho_/ghu_/ghs_/ghr_)"},
	{regexp.MustCompile(`xox[baprs]-[A-Za-z0-9-]{10,}`), "Slack token"},
	{regexp.MustCompile(`AKIA[0-9A-Z]{16}`), "AWS access key ID"},
	{regexp.MustCompile(`ASIA[0-9A-Z]{16}`), "AWS temporary access key ID"},
	{regexp.MustCompile(`AIza[0-9A-Za-z_-]{35}`), "Google API key"},
	{regexp.MustCompile(`ya29\.[0-9A-Za-z_-]+`), "Google OAuth token"},
	{regexp.MustCompile(`eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{6,}`), "JWT"},
	{regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`), "PEM private key block"},
	{regexp.MustCompile(`glpat-[A-Za-z0-9_-]{16,}`), "GitLab PAT"},
}

// assignmentRe captures the value side of `key = "value"` / `key: value` lines,
// where high-entropy values are most suspicious.
var assignmentRe = regexp.MustCompile(`(?i)(?:secret|token|key|password|passwd|pwd|api[-_]?key|access[-_]?key|bearer|credential)\w*\s*[:=]\s*["']?([A-Za-z0-9+/=_\-]{20,})["']?`)

// entropyValueRe captures any quoted long alphanumeric run for entropy scoring.
var entropyValueRe = regexp.MustCompile(`["']([A-Za-z0-9+/=_\-]{24,})["']`)

// entropyThreshold is the Shannon-entropy cutoff (bits/char) above which a long
// string is treated as a likely secret.
const entropyThreshold = 4.0

// maxScanBytes caps how much of a file we read, to keep large binaries cheap.
const maxScanBytes = 2 << 20 // 2 MiB

// FilenameFindings reports problems detected purely from a path/basename,
// without reading the file. Useful for files that cannot be opened.
func FilenameFindings(relPath string) []Finding {
	var out []Finding
	base := filepath.Base(relPath)
	if reason, ok := knownSecretFilenames[base]; ok {
		out = append(out, Finding{File: relPath, Reason: "known secret filename: " + reason})
	}
	for suf, reason := range secretFileSuffixes {
		if strings.HasSuffix(strings.ToLower(base), suf) {
			out = append(out, Finding{File: relPath, Reason: "secret file extension: " + reason})
		}
	}
	return out
}

// File scans a single file's content (and its name) for findings. relPath is
// used only for reporting; absPath is what is actually read.
func File(relPath, absPath string) ([]Finding, error) {
	findings := FilenameFindings(relPath)

	f, err := os.Open(absPath)
	if err != nil {
		return findings, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return findings, err
	}
	if info.IsDir() {
		return findings, nil
	}

	r := bufio.NewReader(f)
	lineNo := 0
	var read int64
	for {
		line, err := r.ReadString('\n')
		read += int64(len(line))
		if len(line) > 0 {
			lineNo++
			if !looksBinary(line) {
				findings = append(findings, scanLine(relPath, lineNo, line)...)
			}
		}
		if err != nil || read >= maxScanBytes {
			break
		}
	}
	return findings, nil
}

// scanLine applies all content heuristics to one line.
func scanLine(file string, lineNo int, line string) []Finding {
	var out []Finding

	for _, p := range tokenPatterns {
		if m := p.re.FindString(line); m != "" {
			out = append(out, Finding{File: file, Line: lineNo, Reason: p.reason, Sample: redact(m)})
		}
	}

	// Secret-ish assignments with a non-trivial value.
	if m := assignmentRe.FindStringSubmatch(line); m != nil {
		val := m[1]
		if !isObviousPlaceholder(val) {
			out = append(out, Finding{
				File:   file,
				Line:   lineNo,
				Reason: "secret-like assignment (key name suggests a credential)",
				Sample: redact(val),
			})
		}
	}

	// High-entropy quoted strings, even without a known key name.
	for _, m := range entropyValueRe.FindAllStringSubmatch(line, -1) {
		val := m[1]
		if isObviousPlaceholder(val) {
			continue
		}
		if shannonEntropy(val) >= entropyThreshold {
			out = append(out, Finding{
				File:   file,
				Line:   lineNo,
				Reason: "high-entropy string (possible secret)",
				Sample: redact(val),
			})
		}
	}

	return dedupe(out)
}

// shannonEntropy returns the per-character Shannon entropy of s in bits.
func shannonEntropy(s string) float64 {
	if s == "" {
		return 0
	}
	var counts [256]int
	for i := 0; i < len(s); i++ {
		counts[s[i]]++
	}
	n := float64(len(s))
	var h float64
	for _, c := range counts {
		if c == 0 {
			continue
		}
		p := float64(c) / n
		h -= p * math.Log2(p)
	}
	return h
}

// isObviousPlaceholder filters out common non-secret values to cut false
// positives (env-var references, dotted/pathy identifiers, repeated chars, and
// well-known dummy values).
func isObviousPlaceholder(v string) bool {
	lv := strings.ToLower(v)
	switch {
	case strings.HasPrefix(v, "${"), strings.HasPrefix(v, "$"):
		return true // shell/template reference, not a literal secret
	case strings.Contains(lv, "example"),
		strings.Contains(lv, "your-"),
		strings.Contains(lv, "changeme"),
		strings.Contains(lv, "placeholder"),
		strings.Contains(lv, "dummy"),
		strings.Contains(lv, "redacted"),
		strings.Contains(lv, "xxxx"):
		return true
	case strings.Contains(v, "/") || strings.Contains(v, "\\"):
		return true // looks like a path
	case looksLikeDottedIdentifier(v):
		return true // e.g. com.example.Foo, a.b.c
	case allSameRune(v):
		return true
	}
	return false
}

func looksLikeDottedIdentifier(v string) bool {
	if !strings.Contains(v, ".") {
		return false
	}
	for _, r := range v {
		if !(r == '.' || r == '_' || r == '-' ||
			(r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')) {
			return false
		}
	}
	return true
}

func allSameRune(v string) bool {
	if v == "" {
		return false
	}
	first := v[0]
	for i := 1; i < len(v); i++ {
		if v[i] != first {
			return false
		}
	}
	return true
}

// looksBinary heuristically detects binary content to skip noisy scanning.
func looksBinary(s string) bool {
	return strings.IndexByte(s, 0) >= 0
}

// redact returns a short, safe excerpt of a suspected secret: enough to locate
// it, never enough to use it.
func redact(s string) string {
	s = strings.TrimSpace(s)
	if len(s) <= 8 {
		return strings.Repeat("*", len(s))
	}
	return s[:4] + "…" + strings.Repeat("*", 4)
}

func dedupe(in []Finding) []Finding {
	seen := map[string]struct{}{}
	out := in[:0]
	for _, f := range in {
		k := f.File + "|" + f.Reason + "|" + f.Sample
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, f)
	}
	return out
}
