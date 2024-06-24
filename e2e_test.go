package main_test

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	testcmd "github.com/ipni/test/cmd"
	"github.com/stretchr/testify/require"
)

const (
	startTimeout = 5 * time.Second
	testTimeout  = 15 * time.Second
)

func TestEndToEndTrustlessGatewayDomains(t *testing.T) {
	switch runtime.GOOS {
	case "windows":
		t.Skip("skipping test on", runtime.GOOS)
	}

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	runner := testcmd.NewRunner(t, t.TempDir())

	// install rainbow
	runner.Run(ctx, "go", "install", ".")
	rainbow := filepath.Join(runner.Dir, "rainbow")

	args := testcmd.Args(rainbow, "--trustless-gateway-domains", "example.org")
	ready := testcmd.NewStdoutWatcher("IPFS Gateway listening")
	domain := testcmd.NewStdoutWatcher("RAINBOW_TRUSTLESS_GATEWAY_DOMAINS = example.org")
	cmdRainbow := runner.Start(ctx, args, ready, domain)

	startCtx, startCancel := context.WithTimeout(context.Background(), startTimeout)
	defer startCancel()

	err := ready.Wait(startCtx)
	require.NoError(t, err)
	t.Log("Rainbow is running")

	err = domain.Wait(startCtx)
	require.NoError(t, err)
	t.Log("Correct value set by cli flag --trustless-gateway-domains")

	runner.Stop(cmdRainbow, 5*time.Second)
	t.Log("Rainbow stopped")

	runner.Env = append(runner.Env, fmt.Sprintf("%s=%s", "RAINBOW_TRUSTLESS_GATEWAY_DOMAINS", "example.com"))
	domain = testcmd.NewStdoutWatcher("RAINBOW_TRUSTLESS_GATEWAY_DOMAINS = example.com")
	cmdRainbow = runner.Start(ctx, testcmd.Args(rainbow), ready, domain)

	startCancel()
	startCtx, startCancel = context.WithTimeout(context.Background(), startTimeout)
	defer startCancel()

	err = ready.Wait(startCtx)
	require.NoError(t, err)
	t.Log("Rainbow is running")

	err = domain.Wait(startCtx)
	require.NoError(t, err)
	t.Log("Correct value set by environ var RAINBOW_TRUSTLESS_GATEWAY_DOMAINS")

	runner.Stop(cmdRainbow, 5*time.Second)
	t.Log("Rainbow stopped")
}
