package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	_ "net/http/pprof"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/urfave/cli/v2"
)

func main() {
	app := cli.NewApp()

	app.Flags = []cli.Flag{

		&cli.StringFlag{
			Name:  "datadir",
			Value: "",
			Usage: "specify the directory that cache data will be stored",
		},
		&cli.StringFlag{
			Name:  "listen",
			Value: ":8080",
			Usage: "specify the listen address for the gateway endpoint",
		},
		&cli.StringFlag{
			Name:  "api-listen",
			Value: "127.0.0.1:8081",
			Usage: "specify the api listening address for the internal control api",
		},

		&cli.IntFlag{
			Name:  "connmgr-low",
			Value: 2000,
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
	}

	app.Name = "rainbow"
	app.Usage = "a standalone ipfs gateway"
	app.Action = func(cctx *cli.Context) error {
		ddir := cctx.String("datadir")
		gnd, err := Setup(cctx.Context, &Config{
			ConnMgrLow:    cctx.Int("connmgr-low"),
			ConnMgrHi:     cctx.Int("connmgr-hi"),
			ConnMgrGrace:  cctx.Duration("connmgr-grace"),
			Blockstore:    filepath.Join(ddir, "blockstore"),
			Datastore:     filepath.Join(ddir, "datastore"),
			Libp2pKeyFile: filepath.Join(ddir, "libp2p.key"),
		})
		if err != nil {
			return err
		}

		go func() {
			http.HandleFunc("/debug/stack", func(w http.ResponseWriter, r *http.Request) {
				if err := writeAllGoroutineStacks(w); err != nil {
					log.Error(err)
				}
			})

			http.HandleFunc("/mgr/gc", func(w http.ResponseWriter, r *http.Request) {
				defer r.Body.Close()

				var body struct {
					BytesToFree int64
				}

				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					http.Error(w, err.Error(), 500)
					return
				}

				if err := gnd.GC(r.Context(), body.BytesToFree); err != nil {
					http.Error(w, err.Error(), 500)
					return
				}

				return
			})

			if err := http.ListenAndServe(cctx.String("api-listen"), nil); err != nil {
				panic(err)
			}
		}()

		paths := []string{""}
		cfgheaders := map[string][]string{}
		pathPrefixes := []string{}

		mux, err := setupHandlerMux(paths, cfgheaders, pathPrefixes, gnd)
		if err != nil {
			return err
		}

		s := &http.Server{
			Addr:    cctx.String("listen"),
			Handler: mux,
		}

		ctx, cancel := context.WithCancel(cctx.Context)

		go func() {
			defer cancel()
			if err := s.ListenAndServe(); err != nil {
				log.Error(err)
			}
		}()

		<-ctx.Done()
		return s.Shutdown(context.TODO())
	}

	if err := app.Run(os.Args); err != nil {
		log.Error(err)
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
