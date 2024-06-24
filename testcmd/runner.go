package testcmd

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const (
	// When this environ var is set to a value and running tests with -v flag,
	// then Runner output is logged.
	EnvTestRunnerOutput = "TEST_RUNNER_OUTPUT"
)

// Runner is a helper for running the indexer and other commands. Runner is not
// specifically tied to the indexer, but is designed to be used to manage
// multiple processes in a test; and is therefore useful for testing the
// indexer, the dhstore, and providers, all in a temporary directory and with a
// test environment.
type Runner struct {
	t       *testing.T
	verbose bool

	Dir string
	Env []string
}

// NewRunner creates a new Runner for the given test, context, and temporary
// directory. It also takes a list of StdoutWatchers, which will be used to
// watch for specific output from the commands.
func NewRunner(t *testing.T, dir string) *Runner {
	rnr := Runner{
		t:       t,
		verbose: os.Getenv(EnvTestRunnerOutput) != "",

		Dir: dir,
	}

	// Use a clean environment, with the host's PATH, and a temporary HOME. We
	// also tell "go install" to place binaries there.
	hostEnv := os.Environ()
	var filteredEnv []string
	for _, env := range hostEnv {
		if strings.Contains(env, "CC") || strings.Contains(env, "LDFLAGS") || strings.Contains(env, "CFLAGS") {
			// Bring in the C compiler flags from the host. For example on a
			// Nix machine, this compilation within the test will fail since
			// the compiler will not find correct libraries.
			filteredEnv = append(filteredEnv, env)
		} else if strings.HasPrefix(env, "PATH") {
			// Bring in the host's PATH.
			filteredEnv = append(filteredEnv, env)
		}
	}
	rnr.Env = append(filteredEnv, []string{
		"HOME=" + rnr.Dir,
		"GOBIN=" + rnr.Dir,
	}...)
	if runtime.GOOS == "windows" {
		const gopath = "C:\\Projects\\Go"
		err := os.MkdirAll(gopath, 0666)
		require.NoError(t, err)
		rnr.Env = append(rnr.Env, fmt.Sprintf("GOPATH=%s", gopath))
	}
	if rnr.verbose {
		t.Logf("Env: %s", strings.Join(rnr.Env, " "))
	}

	// Reuse the host's build and module download cache. This should allow "go
	// install" to reuse work.
	for _, name := range []string{"GOCACHE", "GOMODCACHE"} {
		out, err := exec.Command("go", "env", name).CombinedOutput()
		require.NoError(t, err)
		out = bytes.TrimSpace(out)
		rnr.Env = append(rnr.Env, fmt.Sprintf("%s=%s", name, out))
	}

	return &rnr
}

// Run runs a command and returns its output. This is useful for executing
// synchronous commands within the temporary environment.
func (rnr *Runner) Run(ctx context.Context, name string, args ...string) []byte {
	rnr.t.Helper()

	if rnr.verbose {
		rnr.t.Logf("run: %s %s", name, strings.Join(args, " "))
	}

	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = rnr.Env
	out, err := cmd.CombinedOutput()
	require.NoError(rnr.t, err, string(out))
	return out
}

// CmdArgs contains a command name and any arguments.
type CmdArgs struct {
	name string
	args []string
}

// Args creates a CmdArgs instance with the given command name and args. This
// is used to supply the command names and args to Start.
func Args(name string, args ...string) CmdArgs {
	return CmdArgs{
		name: name,
		args: args,
	}
}

func (a CmdArgs) String() string {
	return a.name + " " + strings.Join(a.args, " ")
}

// Start starts and returns the command. This is useful for executing
// asynchronous commands within the temporary environment. If any watchers are
// supplied, the command's stdout is scanned to look for any matches and signal
// the corresponding watchers.
func (rnr *Runner) Start(ctx context.Context, args CmdArgs, watchers ...StdoutWatcher) *exec.Cmd {
	rnr.t.Helper()

	name := filepath.Base(args.name)
	if rnr.verbose {
		rnr.t.Logf("run: %s", args.String())
	}

	cmd := exec.CommandContext(ctx, args.name, args.args...)
	cmd.Env = rnr.Env

	stdout, err := cmd.StdoutPipe()
	require.NoError(rnr.t, err)
	cmd.Stderr = cmd.Stdout

	scanner := bufio.NewScanner(stdout)

	if rnr.verbose {
		for _, watcher := range watchers {
			rnr.t.Logf("watching: %s for [%s]", name, watcher.match)
		}
	}
	if rnr.verbose || len(watchers) != 0 {
		go func() {
			for scanner.Scan() {
				line := strings.ToLower(scanner.Text())

				if rnr.verbose {
					rnr.t.Logf("%s: %s", name, line)
				}

				for _, watcher := range watchers {
					if strings.Contains(line, strings.ToLower(watcher.match)) {
						watcher.signal <- struct{}{}
					}
				}
			}
		}()
	}

	err = cmd.Start()
	require.NoError(rnr.t, err)
	return cmd
}

// Stop stops a command. It sends SIGINT, and if that does not work, SIGKILL.
func (rnr *Runner) Stop(cmd *exec.Cmd, timeout time.Duration) {
	sig := os.Interrupt
	if runtime.GOOS == "windows" {
		// Windows can't send SIGINT.
		sig = os.Kill
	}
	err := cmd.Process.Signal(sig)
	require.NoError(rnr.t, err)

	waitErr := make(chan error, 1)
	go func() { waitErr <- cmd.Wait() }()

	timer := time.NewTimer(timeout)
	select {
	case <-timer.C:
		rnr.t.Logf("killing command after %s: %s", timeout, cmd)
		err = cmd.Process.Kill()
		require.NoError(rnr.t, err)
	case err = <-waitErr:
		if runtime.GOOS != "windows" {
			require.NoError(rnr.t, err)
		}
		timer.Stop()
	}
}
