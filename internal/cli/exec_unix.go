//go:build unix

package cli

import "syscall"

// execProcess replaces the current process image with argv[0]. On Unix this is
// execve(2): the target CLI inherits encave's stdio and terminal directly, and
// the injected credential lives only for the lifetime of that process (design
// doc §3.4, §6). This call does not return on success.
func execProcess(path string, argv []string, env []string) error {
	return syscall.Exec(path, argv, env)
}
