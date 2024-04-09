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
opinionated choices on the configuration and setup of internal
components. Rainbow aims to serve production environments, where gateways are
deployed as a public service meant to be accessible by anyone. Rainbow acts as
a client to the IPFS network and does not serve or provide content to
it. Rainbow cannot be used to store or pin IPFS content, other than that
temporarily served over HTTP. Rainbow is just a gateway.

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
			Usage:   "Seed to derive peerID from. Generate with gen-seed. Needs --seed-index. Best to use $CREDENTIALS_DIRECTORY/seed or $RAINBOW_DATADIR/seed",
		},
		&cli.IntFlag{
			Name:    "seed-index",
			Value:   -1,
			EnvVars: []string{"RAINBOW_SEED_INDEX"},
			Usage:   "Index to derivate the peerID (needs --seed)",
		},
		&cli.StringSliceFlag{
			Name:    "gateway-domains",
			Value:   cli.NewStringSlice(),
			EnvVars: []string{"RAINBOW_GATEWAY_DOMAINS"},
			Usage:   "Domains with flat path gateway, no Origin isolation (comma-separated)",
		},
		&cli.StringSliceFlag{
			Name:    "subdomain-gateway-domains",
			Value:   cli.NewStringSlice(),
			EnvVars: []string{"RAINBOW_SUBDOMAIN_GATEWAY_DOMAINS"},
			Usage:   "Domains with subdomain-based Origin isolation (comma-separated)",
		},
		&cli.StringSliceFlag{
			Name:    "trustless-gateway-domains",
			Value:   cli.NewStringSlice(),
			EnvVars: []string{"RAINBOW_TRUSTLESS_GATEWAY_DOMAINS"},
			Usage:   "Domains limited to trustless, verifiable response types (comma-separated)",
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
		&cli.DurationFlag{
			Name:    "gc-interval",
			Value:   time.Minute * 60,
			EnvVars: []string{"RAINBOW_GC_INTERVAL"},
			Usage:   "The interval between automatic GC runs. Set 0 to disable",
		},
		&cli.Float64Flag{
			Name:    "gc-threshold",
			Value:   0.3,
			EnvVars: []string{"RAINBOW_GC_THRESHOLD"},
			Usage:   "Percentage of how much of the disk free space must be available",
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
			Usage:   "Size of the in-memory block cache (currently only used for badger). 0 to disable (disables compression on disk too)",
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
		&cli.StringSliceFlag{
			Name:    "http-routers",
			Value:   cli.NewStringSlice(cidContactEndpoint),
			EnvVars: []string{"RAINBOW_HTTP_ROUTERS"},
			Usage:   "HTTP servers with /routing/v1 endpoints to use for delegated routing (comma-separated)",
		},
		&cli.StringFlag{
			Name:    "dht-routing",
			Value:   "accelerated",
			EnvVars: []string{"RAINBOW_DHT_ROUTING"},
			Usage:   "Use the Amino DHT for routing. Options are 'accelerated', 'standard' and 'off'",
			Action: func(ctx *cli.Context, s string) error {
				switch DHTRouting(s) {
				case DHTAccelerated, DHTStandard, DHTOff:
					return nil
				default:
					return errors.New("invalid value for --dht-routing: use 'accelerated', 'standard' or 'off'")
				}
			},
		},
		&cli.BoolFlag{
			Name:    "dht-shared-host",
			Value:   false,
			EnvVars: []string{"RAINBOW_DHT_SHARED_HOST"},
			Usage:   "If false, DHT operations are run using an ephemeral peer, separate from the main one",
		},
		&cli.StringSliceFlag{
			Name:    "denylists",
			Value:   cli.NewStringSlice(),
			EnvVars: []string{"RAINBOW_DENYLISTS"},
			Usage:   "Denylist HTTP subscriptions (comma-separated). Must be append-only denylists",
		},
		&cli.StringSliceFlag{
			Name:    "peering",
			Value:   cli.NewStringSlice(),
			EnvVars: []string{"RAINBOW_PEERING"},
			Usage:   "Multiaddresses of peers to stay connected to (comma-separated)",
		},
		&cli.StringFlag{
			Name:    "blockstore",
			Value:   "flatfs",
			EnvVars: []string{"RAINBOW_BLOCKSTORE"},
			Usage:   "Type of blockstore to use, such as flatfs or badger. See https://github.com/ipfs/rainbow/blob/main/docs/blockstores.md for more details",
		},
		&cli.DurationFlag{
			Name:    "ipns-max-cache-ttl",
			Value:   0,
			EnvVars: []string{"RAINBOW_IPNS_MAX_CACHE_TTL"},
			Usage:   "Optional cap on caching duration for IPNS/DNSLink lookups. Set to 0 to respect original TTLs",
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

		var peeringAddrs []peer.AddrInfo
		for _, maStr := range cctx.StringSlice("peering") {
			ai, err := peer.AddrInfoFromString(maStr)
			if err != nil {
				return err
			}
			peeringAddrs = append(peeringAddrs, *ai)
		}

		cfg := Config{
			DataDir:                 ddir,
			BlockstoreType:          cctx.String("blockstore"),
			GatewayDomains:          cctx.StringSlice("gateway-domains"),
			SubdomainGatewayDomains: cctx.StringSlice("subdomain-gateway-domains"),
			TrustlessGatewayDomains: cctx.StringSlice("trustless-gateway-domains"),
			ConnMgrLow:              cctx.Int("connmgr-low"),
			ConnMgrHi:               cctx.Int("connmgr-high"),
			ConnMgrGrace:            cctx.Duration("connmgr-grace"),
			MaxMemory:               cctx.Uint64("max-memory"),
			MaxFD:                   cctx.Int("max-fd"),
			InMemBlockCache:         cctx.Int64("inmem-block-cache"),
			RoutingV1Endpoints:      cctx.StringSlice("http-routers"),
			DHTRouting:              DHTRouting(cctx.String("dht-routing")),
			DHTSharedHost:           cctx.Bool("dht-shared-host"),
			IpnsMaxCacheTTL:         cctx.Duration("ipns-max-cache-ttl"),
			DenylistSubs:            cctx.StringSlice("denylists"),
			Peering:                 peeringAddrs,
			GCInterval:              cctx.Duration("gc-interval"),
			GCThreshold:             cctx.Float64("gc-threshold"),
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
		registerIpfsNodeCollector(gnd)

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

		fmt.Printf("IPFS Gateway listening at %s\n\n", gatewayListen)

		printIfListConfigured("  RAINBOW_GATEWAY_DOMAINS           = ", cfg.GatewayDomains)
		printIfListConfigured("  RAINBOW_SUBDOMAIN_GATEWAY_DOMAINS = ", cfg.SubdomainGatewayDomains)
		printIfListConfigured("  RAINBOW_TRUSTLESS_GATEWAY_DOMAINS = ", cfg.TrustlessGatewayDomains)

		fmt.Printf("\n")
		fmt.Printf("CTL endpoint listening at http://%s\n", ctlListen)
		fmt.Printf("  Metrics: http://%s/debug/metrics/prometheus\n\n", ctlListen)

		go func() {
			defer wg.Done()

			err := gatewaySrv.ListenAndServe()
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				fmt.Fprintf(os.Stderr, "Failed to start gateway: %s\n", err)
				quit <- os.Interrupt
			}
		}()

		_ = gnd.periodicGC(cctx.Context, cfg.GCThreshold)

		go func() {
			defer wg.Done()

			err := apiSrv.ListenAndServe()
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				fmt.Fprintf(os.Stderr, "Failed to start metrics: %s\n", err)
				quit <- os.Interrupt
			}
		}()

		var gcTicker *time.Ticker
		var gcTickerDone chan bool

		if cfg.GCInterval > 0 {
			gcTicker = time.NewTicker(cfg.GCInterval)
			gcTickerDone = make(chan bool)
			wg.Add(1)

			go func() {
				defer wg.Done()

				for {
					select {
					case <-gcTickerDone:
						return
					case <-gcTicker.C:
						err = gnd.periodicGC(cctx.Context, cfg.GCThreshold)
						if err != nil {
							goLog.Errorf("error when running periodic gc: %w", err)
						}
					}
				}
			}()
		}

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

		if gcTicker != nil {
			gcTicker.Stop()
			gcTickerDone <- true
		}

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

func printIfListConfigured(message string, list []string) {
	if len(list) > 0 {
		fmt.Printf(message+"%v\n", strings.Join(list, ", "))
	}
}
