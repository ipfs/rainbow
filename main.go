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
	app.Name = "rainbow"
	app.Usage = "The IPFS HTTP gateway daemon"
	app.Version = version
	app.Description = `
Rainbow runs an IPFS HTTP gateway.

An IPFS HTTP gateway is able to fetch content from the IPFS network and serve
it via HTTP, so that it becomes seamless to browse the web, when the web is
stored and provided by peers in the IPFS network.

HTTP gateways are also able to facilitate download of any IPFS content (not
only websites, but any supported content-addressed Merkle-DAG), in formats
that are suitable for verification client-side (i.e. CAR files).

Rainbow is optimized to perform the tasks of a gateway and only that, making
opinionated choices on the configration and setup of internal
components. Rainbow aims to serve production environments, where gateways are
deployed as a public service meant to be accessible by anyone. Rainbow acts as
a client to the IPFS network and does not serve or provide content to
it. Rainbow cannot be used to store or pin IPFS content, other than that
temporailly served over HTTP. Rainbow is just a gateway.

Persistent configuration and data is stored in $RAINBOW_DATADIR (by default,
the folder in which rainbow is run).

EXAMPLES

Launch a gateway with randomly generated libp2p.key (will be written to
$RAINBOW_DATADIR/libp2p.key and used in subsequent runs):

  $ rainbow

Generate an identity seed and launch a gateway:

  $ rainbow gen-seed > $RAINBOW_DATADIR/seed
  $ rainbow --seed-index 0

(other rainbow gateways can use the same seed with different indexes to
 derivate their identities)
`

	app.Flags = []cli.Flag{

		&cli.StringFlag{
			Name:    "datadir",
			Value:   "",
			EnvVars: []string{"RAINBOW_DATADIR"},
			Usage:   "Directory for persistent data (keys, blocks, denylists)",
		},
		&cli.StringFlag{
			Name:    "seed",
			Value:   "",
			EnvVars: []string{"RAINBOW_SEED"},
			Usage:   "Seed to derive peerID from. Generate with gen-seed. Needs --seed-index. Best to use $CREDENTIALS_DIRECTORY/seed or $RAINBOW_DATADIR/seed.",
		},
		&cli.IntFlag{
			Name:    "seed-index",
			Value:   -1,
			EnvVars: []string{"RAINBOW_SEED_INDEX"},
			Usage:   "Index to derivate the peerID (needs --seed)",
		},
		&cli.StringFlag{
			Name:    "gateway-domains",
			Value:   "",
			EnvVars: []string{"RAINBOW_GATEWAY_DOMAINS"},
			Usage:   "Legacy path-gateway domains. Comma-separated list.",
		},
		&cli.StringFlag{
			Name:    "subdomain-gateway-domains",
			Value:   "",
			EnvVars: []string{"RAINBOW_SUBDOMAIN_GATEWAY_DOMAINS"},
			Usage:   "Subdomain gateway domains. Comma-separated list.",
		},
		&cli.StringFlag{
			Name:    "gateway-listen-address",
			Value:   "127.0.0.1:8090",
			EnvVars: []string{"RAINBOW_GATEWAY_LISTEN_ADDRESS"},
			Usage:   "Listen address for the gateway endpoint",
		},
		&cli.StringFlag{
			Name:    "ctl-listen-address",
			Value:   "127.0.0.1:8091",
			EnvVars: []string{"RAINBOW_CTL_LISTEN_ADDRESS"},
			Usage:   "Listen address for the management api and metrics",
		},

		&cli.IntFlag{
			Name:    "connmgr-low",
			Value:   100,
			EnvVars: []string{"RAINBOW_CONNMGR_LOW"},
			Usage:   "Minimum number of connections to keep",
		},
		&cli.IntFlag{
			Name:    "connmgr-high",
			Value:   3000,
			EnvVars: []string{"RAINBOW_CONNMGR_HIGH"},
			Usage:   "Maximum number of connections to keep",
		},
		&cli.DurationFlag{
			Name:    "connmgr-grace",
			Value:   time.Minute,
			EnvVars: []string{"RAINBOW_CONNMGR_GRACE_PERIOD"},
			Usage:   "Minimum connection TTL",
		},
		&cli.IntFlag{
			Name:    "inmem-block-cache",
			Value:   1 << 30,
			EnvVars: []string{"RAINBOW_INMEM_BLOCK_CACHE"},
			Usage:   "Size of the in-memory block cache. 0 to disable (disables compression on disk too)",
		},
		&cli.Uint64Flag{
			Name:    "max-memory",
			Value:   0,
			EnvVars: []string{"RAINBOW_MAX_MEMORY"},
			Usage:   "Max memory to use. Defaults to 85% of the system's available RAM",
		},
		&cli.Uint64Flag{
			Name:    "max-fd",
			Value:   0,
			EnvVars: []string{"RAINBOW_MAX_FD"},
			Usage:   "Maximum number of file descriptors. Defaults to 50% of the process' limit",
		},
		&cli.StringFlag{
			Name:  "routing",
			Value: "",
			Usage: "RoutingV1 Endpoint (otherwise Amino DHT and cid.contact is used)",
		},
		&cli.BoolFlag{
			Name:    "dht-share-host",
			Value:   false,
			EnvVars: []string{"RAINBOW_DHT_SHARED_HOST"},
			Usage:   "If false, DHT operations are run using an ephemeral peer, separate from the main one",
		},
		&cli.StringFlag{
			Name:    "denylists",
			Value:   "",
			EnvVars: []string{"RAINBOW_DENYLISTS"},
			Usage:   "Denylist HTTP subscriptions (comma-separated). Must be append-only denylists",
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

The seed can be provided to rainbow by:

  - Storing it in $RAINBOW_DATADIR/seed
  - Storing it in $CREDENTIALS_DIRECTORY/seed
  - Passing the --seed flag

In all cases the --seed-index flag will be necessary. Multiple gateways can
share the same seed as long as the indexes are different.
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

		cfg := Config{
			DataDir:                 ddir,
			GatewayDomains:          getCommaSeparatedList(cctx.String("gateway-domains")),
			SubdomainGatewayDomains: getCommaSeparatedList(cctx.String("subdomain-gateway-domains")),
			ConnMgrLow:              cctx.Int("connmgr-low"),
			ConnMgrHi:               cctx.Int("connmgr-high"),
			ConnMgrGrace:            cctx.Duration("connmgr-grace"),
			MaxMemory:               cctx.Uint64("max-memory"),
			MaxFD:                   cctx.Int("max-fd"),
			InMemBlockCache:         cctx.Int64("inmem-block-cache"),
			RoutingV1:               cctx.String("routing"),
			KuboRPCURLs:             getEnvs(EnvKuboRPC, DefaultKuboRPC),
			DHTSharedHost:           cctx.Bool("dht-shared-host"),
			DenylistSubs:            getCommaSeparatedList(cctx.String("denylists")),
		}

		goLog.Debugf("Rainbow config: %+v", cfg)

		gnd, err := Setup(cctx.Context, cfg, priv, cdns)
		if err != nil {
			return err
		}

		gatewayListen := cctx.String("gateway-listen-address")
		ctlListen := cctx.String("ctl-listen-address")

		handler, err := setupGatewayHandler(cfg, gnd)
		if err != nil {
			return err
		}

		gatewaySrv := &http.Server{
			Addr:    gatewayListen,
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
			Addr:    ctlListen,
			Handler: apiMux,
		}

		quit := make(chan os.Signal, 3)
		var wg sync.WaitGroup
		wg.Add(2)

		fmt.Printf("Gateway listening at %s\n", gatewayListen)
		fmt.Printf("Legacy RPC at /api/v0 (%s): %s\n", EnvKuboRPC, strings.Join(gnd.kuboRPCs, " "))
		fmt.Printf("CTL endpoint listening at http://%s\n", ctlListen)
		fmt.Printf("Metrics: http://%s/debug/metrics/prometheus\n\n", ctlListen)

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

func getCommaSeparatedList(val string) []string {
	if val == "" {
		return nil
	}
	items := strings.Split(val, ",")
	for i, item := range items {
		items[i] = strings.TrimSpace(item)
	}
	return items
}
