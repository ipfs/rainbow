package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	sddaemon "github.com/coreos/go-systemd/v22/daemon"
	logging "github.com/ipfs/go-log/v2"
	"github.com/libp2p/go-libp2p/core/crypto"
	peer "github.com/libp2p/go-libp2p/core/peer"
	"github.com/urfave/cli/v2"
	"go.opentelemetry.io/contrib/propagators/autoprop"
	"go.opentelemetry.io/otel"
)

const EnvKuboRPC = "KUBO_RPC_URL"

var goLog = logging.Logger("rainbow")

func main() {
	app := cli.NewApp()

	app.Flags = []cli.Flag{

		&cli.StringFlag{
			Name:    "datadir",
			Value:   "",
			EnvVars: []string{"RAINBOW_DATADIR"},
			Usage:   "specify the directory that cache data will be stored",
		},
		&cli.StringFlag{
			Name:    "seed",
			Value:   "",
			EnvVars: []string{"RAINBOW_SEED"},
			Usage:   "Specify a seed to derive peerID from (needs --seed-index). Best to use CREDENTIALS_DIRECTORY/seed",
		},
		&cli.IntFlag{
			Name:    "seed-index",
			Value:   -1,
			EnvVars: []string{"RAINBOW_SEED_INDEX"},
			Usage:   "Specify an index to derivate the peerID from the key (needs --seed)",
		},
		&cli.StringFlag{
			Name:    "gateway-domain",
			Value:   "",
			EnvVars: []string{"RAINBOW_GATEWAY_DOMAIN"},
			Usage:   "Set to enable subdomain gateway on this domain",
		},
		&cli.IntFlag{
			Name:    "gateway-port",
			Value:   8090,
			EnvVars: []string{"RAINBOW_GATEWAY_PORT"},
			Usage:   "specify the listen address for the gateway endpoint",
		},
		&cli.IntFlag{
			Name:    "ctl-port",
			Value:   8091,
			EnvVars: []string{"RAINBOW_CTL_PORT"},
			Usage:   "specify the api listening address for the internal control api",
		},

		&cli.IntFlag{
			Name:    "connmgr-low",
			Value:   100,
			EnvVars: []string{"RAINBOW_CONNMGR_LOW"},
			Usage:   "libp2p connection manager 'low' water mark",
		},
		&cli.IntFlag{
			Name:    "connmgr-high",
			Value:   3000,
			EnvVars: []string{"RAINBOW_CONNMGR_HIGH"},
			Usage:   "libp2p connection manager 'high' water mark",
		},
		&cli.DurationFlag{
			Name:    "connmgr-grace",
			Value:   time.Minute,
			EnvVars: []string{"RAINBOW_CONNMGR_GRACE_PERIOD"},
			Usage:   "libp2p connection manager grace period",
		},
		&cli.IntFlag{
			Name:    "inmem-block-cache",
			Value:   1 << 30,
			EnvVars: []string{"RAINBOW_INMEM_BLOCK_CACHE"},
			Usage:   "Size of the in-memory block cache. 0 to disable (disables compression too)",
		},
		&cli.Uint64Flag{
			Name:    "max-memory",
			Value:   0,
			EnvVars: []string{"RAINBOW_MAX_MEMORY"},
			Usage:   "Libp2p resource manager max memory. Defaults to system's memory * 0.85",
		},
		&cli.Uint64Flag{
			Name:    "max-fd",
			Value:   0,
			EnvVars: []string{"RAINBOW_MAX_FD"},
			Usage:   "Libp2p resource manager file description limit. Defaults to the process' fd-limit/2",
		},
		&cli.StringFlag{
			Name:  "routing",
			Value: "",
			Usage: "RoutingV1 Endpoint (if none is supplied use the Amino DHT and cid.contact)",
		},
		&cli.BoolFlag{
			Name:  "dht-fallback-shared-host",
			Value: false,
			Usage: "If using an Amino DHT client should the libp2p host be shared with the data downloading host",
		},
		&cli.StringFlag{
			Name:    "denylists",
			Value:   "",
			EnvVars: []string{"RAINBOW_DENYLISTS"},
			Usage:   "Denylist subscriptions (comma-separated)",
		},
	}

	app.Commands = []*cli.Command{
		{
			Name:  "gen-seed",
			Usage: "Generate a seed for key derivation",
			Description: `
Running this command will generate a random seed and print it. The value can
be used with the RAINBOW_SEED env-var to use key-derivation from a single seed
to create libp2p identities for the gateway.
`,
			Flags: []cli.Flag{},
			Action: func(c *cli.Context) error {
				seed, err := newSeed()
				if err != nil {
					return err
				}
				fmt.Println(seed)
				return nil
			},
		},
	}

	app.Name = "rainbow"
	app.Usage = "a standalone ipfs gateway"
	app.Version = version
	app.Action = func(cctx *cli.Context) error {
		ddir := cctx.String("datadir")
		cdns := newCachedDNS(dnsCacheRefreshInterval)
		defer cdns.Close()

		var seed string
		var priv crypto.PrivKey
		var err error

		credDir := os.Getenv("CREDENTIALS_DIRECTORY")
		secretsDir := ddir

		if len(credDir) > 0 {
			secretsDir = credDir
		}

		// attempt to read seed from disk
		seedBytes, err := os.ReadFile(filepath.Join(secretsDir, "seed"))
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				// set seed from command line or env-var
				seed = cctx.String("seed")
			} else {
				return fmt.Errorf("error reading seed credentials: %w", err)
			}
		} else {
			seed = strings.TrimSpace(string(seedBytes))
		}

		index := cctx.Int("seed-index")
		if len(seed) > 0 && index >= 0 {
			fmt.Println("Deriving identity from seed")
			priv, err = deriveKey(seed, []byte(fmt.Sprintf("rainbow-%d", index)))
		} else {
			fmt.Println("Setting identity from libp2p.key")
			keyFile := filepath.Join(secretsDir, "libp2p.key")
			priv, err = loadOrInitPeerKey(keyFile)
		}
		if err != nil {
			return err
		}

		var denylists []string
		if list := cctx.String("denylists"); len(list) > 0 {
			denylists = strings.Split(list, ",")
		}

		cfg := Config{
			DataDir:         ddir,
			GatewayDomain:   cctx.String("gateway-domain"),
			ConnMgrLow:      cctx.Int("connmgr-low"),
			ConnMgrHi:       cctx.Int("connmgr-high"),
			ConnMgrGrace:    cctx.Duration("connmgr-grace"),
			MaxMemory:       cctx.Uint64("max-memory"),
			MaxFD:           cctx.Int("max-fd"),
			InMemBlockCache: cctx.Int64("inmem-block-cache"),
			Libp2pKey:       priv,
			RoutingV1:       cctx.String("routing"),
			KuboRPCURLs:     getEnvs(EnvKuboRPC, DefaultKuboRPC),
			DHTSharedHost:   cctx.Bool("dht-fallback-shared-host"),
			DNSCache:        cdns,
			DenylistSubs:    denylists,
		}

		fmt.Printf(`

Rainbow config:

%+v

`, cfg)

		gnd, err := Setup(cctx.Context, cfg)
		if err != nil {
			return err
		}

		gatewayPort := cctx.Int("gateway-port")
		apiPort := cctx.Int("ctl-port")

		handler, err := setupGatewayHandler(cfg, gnd)
		if err != nil {
			return err
		}

		gatewaySrv := &http.Server{
			Addr:    fmt.Sprintf("127.0.0.1:%d", gatewayPort),
			Handler: handler,
		}

		fmt.Printf("Starting %s %s\n", name, version)
		pid, err := peer.IDFromPublicKey(priv.GetPublic())
		if err != nil {
			return err
		}
		fmt.Printf("PeerID: %s\n\n", pid)
		registerVersionMetric(version)

		tp, shutdown, err := newTracerProvider(cctx.Context)
		if err != nil {
			return err
		}
		defer func() {
			_ = shutdown(cctx.Context)
		}()
		otel.SetTracerProvider(tp)
		otel.SetTextMapPropagator(autoprop.NewTextMapPropagator())

		apiMux := makeMetricsAndDebuggingHandler()
		apiMux.HandleFunc("/mgr/gc", GCHandler(gnd))

		apiSrv := &http.Server{
			Addr:    fmt.Sprintf("127.0.0.1:%d", apiPort),
			Handler: apiMux,
		}

		quit := make(chan os.Signal, 3)
		var wg sync.WaitGroup
		wg.Add(2)

		fmt.Printf("Legacy RPC at /api/v0 (%s): %s\n", EnvKuboRPC, strings.Join(gnd.kuboRPCs, " "))
		fmt.Printf("Path gateway: http://127.0.0.1:%d\n", gatewayPort)
		fmt.Printf("  Smoke test (JPG): http://127.0.0.1:%d/ipfs/bafybeigdyrzt5sfp7udm7hu76uh7y26nf3efuylqabf3oclgtqy55fbzdi\n", gatewayPort)
		fmt.Printf("Subdomain gateway: http://localhost:%d\n", gatewayPort)
		fmt.Printf("  Smoke test (Subdomain+DNSLink+UnixFS+HAMT): http://localhost:%d/ipns/en.wikipedia-on-ipfs.org/wiki/\n\n\n", gatewayPort)

		fmt.Printf("CTL port: http://127.0.0.1:%d\n", apiPort)
		fmt.Printf("Metrics: http://127.0.0.1:%d/debug/metrics/prometheus\n\n", apiPort)

		go func() {
			defer wg.Done()

			err := gatewaySrv.ListenAndServe()
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				fmt.Fprintf(os.Stderr, "Failed to start gateway: %s\n", err)
				quit <- os.Interrupt
			}
		}()

		go func() {
			defer wg.Done()

			err := apiSrv.ListenAndServe()
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				fmt.Fprintf(os.Stderr, "Failed to start metrics: %s\n", err)
				quit <- os.Interrupt
			}
		}()

		sddaemon.SdNotify(false, sddaemon.SdNotifyReady)
		signal.Notify(
			quit,
			syscall.SIGINT,
			syscall.SIGTERM,
			syscall.SIGHUP,
		)
		<-quit
		sddaemon.SdNotify(false, sddaemon.SdNotifyStopping)
		goLog.Info("Closing servers...")
		go gatewaySrv.Close()
		go apiSrv.Close()
		for _, sub := range gnd.denylistSubs {
			sub.Stop()
		}
		wg.Wait()
		return nil
	}

	if err := app.Run(os.Args); err != nil {
		goLog.Error(err)
		os.Exit(1)
	}
}

func writeAllGoroutineStacks(w io.Writer) error {
	buf := make([]byte, 64<<20)
	for i := 0; ; i++ {
		n := runtime.Stack(buf, true)
		if n < len(buf) {
			buf = buf[:n]
			break
		}
		if len(buf) >= 1<<30 {
			// Filled 1 GB - stop there.
			break
		}
		buf = make([]byte, 2*len(buf))
	}
	_, err := w.Write(buf)
	return err
}

func getEnvs(key, defaultValue string) []string {
	value := os.Getenv(key)
	if value == "" {
		if defaultValue == "" {
			return []string{}
		}
		value = defaultValue
	}
	value = strings.TrimSpace(value)
	return strings.Split(value, ",")
}
