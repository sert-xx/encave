// Package fsutil provides a recursive directory copy with exclusion patterns,
// used by `encave new` to scaffold a draft agent from a user's base home while
// dropping secrets, state, logs and other excluded entries.
package fsutil

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// CopyResult summarizes what a CopyTree call did.
type CopyResult struct {
	FilesCopied int
	Excluded    []string // relative paths that matched an exclusion pattern
}

// CopyTree recursively copies src into dst, skipping any entry whose basename or
// relative path matches one of the exclude patterns (filepath.Match semantics,
// tested against both the basename and the slash-joined relative path). When an
// excluded pattern matches a directory, the whole subtree is pruned.
//
// File modes are preserved; symlinks are recreated as symlinks (their targets
// are not followed). dst must not already exist.
func CopyTree(src, dst string, excludes []string) (CopyResult, error) {
	var res CopyResult

	srcInfo, err := os.Stat(src)
	if err != nil {
		return res, fmt.Errorf("source %q: %w", src, err)
	}
	if !srcInfo.IsDir() {
		return res, fmt.Errorf("source %q is not a directory", src)
	}
	if _, err := os.Stat(dst); err == nil {
		return res, fmt.Errorf("destination %q already exists", dst)
	}

	walk := func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, rerr := filepath.Rel(src, path)
		if rerr != nil {
			return rerr
		}
		if rel == "." {
			return os.MkdirAll(dst, srcInfo.Mode().Perm())
		}

		if matchesAny(rel, d.Name(), excludes) {
			res.Excluded = append(res.Excluded, rel)
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		target := filepath.Join(dst, rel)

		switch {
		case d.IsDir():
			info, ierr := d.Info()
			if ierr != nil {
				return ierr
			}
			return os.MkdirAll(target, info.Mode().Perm())
		case d.Type()&os.ModeSymlink != 0:
			link, lerr := os.Readlink(path)
			if lerr != nil {
				return lerr
			}
			return os.Symlink(link, target)
		case d.Type().IsRegular():
			if cerr := copyFile(path, target); cerr != nil {
				return cerr
			}
			res.FilesCopied++
			return nil
		default:
			// Skip sockets, devices, pipes, etc.
			return nil
		}
	}

	if err := filepath.WalkDir(src, walk); err != nil {
		return res, err
	}
	return res, nil
}

// matchesAny reports whether rel/base matches any exclusion pattern. A bare name
// pattern (no slash) is matched against every path component as well, so e.g.
// "logs" prunes any directory named logs at any depth. A pattern with a leading
// "/" is root-anchored: it matches only the relative path from src root (e.g.
// "/projects" prunes <root>/projects but not skills/x/projects), so common state
// names can be excluded without clobbering like-named content deeper in the tree.
func matchesAny(rel, base string, patterns []string) bool {
	relSlash := filepath.ToSlash(rel)
	for _, p := range patterns {
		if p == "" {
			continue
		}
		if strings.HasPrefix(p, "/") {
			// Root-anchored: match only against the full relative path.
			if ok, _ := filepath.Match(strings.TrimPrefix(p, "/"), relSlash); ok {
				return true
			}
			continue
		}
		if ok, _ := filepath.Match(p, base); ok {
			return true
		}
		if ok, _ := filepath.Match(p, relSlash); ok {
			return true
		}
		if !strings.Contains(p, "/") {
			for _, comp := range strings.Split(relSlash, "/") {
				if ok, _ := filepath.Match(p, comp); ok {
					return true
				}
			}
		}
	}
	return false
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	info, err := in.Stat()
	if err != nil {
		return err
	}

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
