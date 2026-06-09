// Package modproxy queries the Go module proxy for the latest published version
// of a module. encave uses it for a single purpose: discovering whether a newer
// release of encave itself exists, so it can offer to `go install` that exact
// version. It is intentionally best-effort — a short timeout, and any error is a
// signal to simply skip the update check.
package modproxy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

// defaultProxy is the public Go module proxy, used when GOPROXY is unset.
const defaultProxy = "https://proxy.golang.org"

// Latest returns the version string the module proxy reports as latest for the
// given module path (e.g. "v0.8.0"). It honors GOPROXY, mirroring the proxy the
// user's own `go install` would consult.
func Latest(ctx context.Context, module string) (string, error) {
	base, ok := proxyBase(os.Getenv("GOPROXY"))
	if !ok {
		return "", fmt.Errorf("no usable GOPROXY (off/direct only)")
	}
	url := strings.TrimRight(base, "/") + "/" + escapePath(module) + "/@latest"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("proxy returned %s", resp.Status)
	}
	var info struct {
		Version string `json:"Version"`
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if err != nil {
		return "", err
	}
	if err := json.Unmarshal(body, &info); err != nil {
		return "", err
	}
	if info.Version == "" {
		return "", fmt.Errorf("proxy returned no version")
	}
	return info.Version, nil
}

// proxyBase resolves the first usable HTTP proxy URL from a GOPROXY value
// (comma- or pipe-separated). It returns the default public proxy when GOPROXY is
// empty, and ok=false when GOPROXY contains only "off"/"direct" (so no proxy can
// answer a query).
func proxyBase(goproxy string) (string, bool) {
	goproxy = strings.TrimSpace(goproxy)
	if goproxy == "" {
		return defaultProxy, true
	}
	for _, part := range strings.FieldsFunc(goproxy, func(r rune) bool { return r == ',' || r == '|' }) {
		part = strings.TrimSpace(part)
		if part == "" || part == "off" || part == "direct" {
			continue
		}
		if strings.HasPrefix(part, "http://") || strings.HasPrefix(part, "https://") {
			return part, true
		}
	}
	return "", false
}

// escapePath applies the module-proxy case encoding: every uppercase letter is
// replaced with "!" followed by its lowercase form, so case-insensitive file
// systems behind the proxy stay unambiguous. Lowercase module paths (encave's
// own) pass through unchanged.
func escapePath(p string) string {
	var b strings.Builder
	for _, r := range p {
		if r >= 'A' && r <= 'Z' {
			b.WriteByte('!')
			b.WriteRune(r + ('a' - 'A'))
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}
