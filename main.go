package main

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	logging "github.com/ipfs/go-log/v2"
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
			Name:  "datadir",
			Value: "",
			Usage: "specify the directory that cache data will be stored",
		},
		&cli.IntFlag{
			Name:  "gateway-port",
			Value: 8090,
			Usage: "specify the listen address for the gateway endpoint",
		},
		&cli.IntFlag{
			Name:  "ctl-port",
			Value: 8091,
			Usage: "specify the api listening address for the internal control api",
		},

		&cli.IntFlag{
			Name:  "connmgr-low",
			Value: 100,
			Usage: "libp2p connection manager 'low' water mark",
		},
		&cli.IntFlag{
			Name:  "connmgr-hi",
			Value: 3000,
			Usage: "libp2p connection manager 'high' water mark",
		},
		&cli.DurationFlag{
			Name:  "connmgr-grace",
			Value: time.Minute,
			Usage: "libp2p connection manager grace period",
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
	}

	app.Name = "rainbow"
	app.Usage = "a standalone ipfs gateway"
	app.Version = version
	app.Action = func(cctx *cli.Context) error {
		ddir := cctx.String("datadir")
		cdns := newCachedDNS(dnsCacheRefreshInterval)
		defer cdns.Close()

		gnd, err := Setup(cctx.Context, &Config{
			ConnMgrLow:    cctx.Int("connmgr-low"),
			ConnMgrHi:     cctx.Int("connmgr-hi"),
			ConnMgrGrace:  cctx.Duration("connmgr-grace"),
			Blockstore:    filepath.Join(ddir, "blockstore"),
			Datastore:     filepath.Join(ddir, "datastore"),
			Libp2pKeyFile: filepath.Join(ddir, "libp2p.key"),
			RoutingV1:     cctx.String("routing"),
			KuboRPCURLs:   getEnvs(EnvKuboRPC, DefaultKuboRPC),
			DHTSharedHost: cctx.Bool("dht-fallback-shared-host"),
			DNSCache:      cdns,
		})
		if err != nil {
			return err
		}

		gatewayPort := cctx.Int("gateway-port")
		apiPort := cctx.Int("api-port")

		handler, err := setupGatewayHandler(gnd)
		if err != nil {
			return err
		}

		gatewaySrv := &http.Server{
			Addr:    fmt.Sprintf("127.0.0.1:%d", gatewayPort),
			Handler: handler,
		}

		fmt.Printf("Starting %s %s", name, version)
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

		quit := make(chan os.Signal, 1)
		var wg sync.WaitGroup
		wg.Add(2)

		go func() {
			defer wg.Done()

			// log important configuration flags
			log.Printf("Legacy RPC at /api/v0 (%s) provided by %s", EnvKuboRPC, strings.Join(gnd.kuboRPCs, " "))
			log.Printf("Path gateway listening on http://127.0.0.1:%d", gatewayPort)
			log.Printf("  Smoke test (JPG): http://127.0.0.1:%d/ipfs/bafybeigdyrzt5sfp7udm7hu76uh7y26nf3efuylqabf3oclgtqy55fbzdi", gatewayPort)
			log.Printf("Subdomain gateway configured on dweb.link and http://localhost:%d", gatewayPort)
			log.Printf("  Smoke test (Subdomain+DNSLink+UnixFS+HAMT): http://localhost:%d/ipns/en.wikipedia-on-ipfs.org/wiki/", gatewayPort)
			err := gatewaySrv.ListenAndServe()
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Printf("Failed to start gateway: %s", err)
				quit <- os.Interrupt
			}
		}()

		go func() {
			defer wg.Done()
			log.Printf("CTL port exposed at http://127.0.0.1:%d", apiPort)
			log.Printf("Metrics exposed at http://127.0.0.1:%d/debug/metrics/prometheus", apiPort)
			err := apiSrv.ListenAndServe()
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Printf("Failed to start metrics: %s", err)
				quit <- os.Interrupt
			}
		}()

		signal.Notify(quit, os.Interrupt)
		<-quit
		log.Printf("Closing servers...")
		go gatewaySrv.Close()
		go apiSrv.Close()
		wg.Wait()
		return nil
	}

	if err := app.Run(os.Args); err != nil {
		log.Print(err)
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
