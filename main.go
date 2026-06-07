// Command encave distributes and runs tuned coding-agent configurations as
// isolated, reproducible, self-contained agent homes shared over GitHub.
//
// See the design document for the full rationale. In brief:
//   - An "agent" is a complete agent home directory (config + orchestration +
//     skills), distributed via git clone + tag for byte-for-byte reproducibility.
//   - Each installed agent lives in its own directory under the encave root and
//     is launched in isolation, never touching the user's personal home.
//   - Non-secret config is committed; real credentials live in the OS keyring
//     and are injected into the launched child process's environment only.
package main

import (
	"os"

	"github.com/sert-xx/encave/internal/cli"
)

func main() {
	os.Exit(cli.Main(os.Args[1:]))
}
