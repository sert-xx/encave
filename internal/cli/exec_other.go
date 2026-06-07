//go:build !unix

package cli

import (
	"os"
	"os/exec"
)

// execProcess runs the target CLI as a child process on platforms without
// execve semantics (e.g. Windows). encave waits for it and mirrors its exit
// code; the credential still lives only for the child's lifetime.
func execProcess(path string, argv []string, env []string) error {
	cmd := exec.Command(path, argv[1:]...)
	cmd.Env = env
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		return err
	}
	os.Exit(0)
	return nil
}
