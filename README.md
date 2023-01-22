# ⚠️ This is an old research EXPERIMENT. For production use, see [Kubo's](https://github.com/ipfs/go-ipfs)' [gateway recipes](https://github.com/ipfs/go-ipfs/blob/master/docs/config.md#gateway-recipes) and  ongoing [work to extract gateway code to standalone library](https://github.com/ipfs/kubo/issues/8524). <!-- omit in toc -->

# rainbow 

> Because ipfs should just work like unicorns and rainbows

## Building
```
go build
```

## Running
```
rainbow
```

## Configuration
```
NAME:
   rainbow - a standalone ipfs gateway

USAGE:
   rainbow [global options] command [command options] [arguments...]

VERSION:
   0.0.0

COMMANDS:
   help, h  Shows a list of commands or help for one command

GLOBAL OPTIONS:
   --datadir value        specify the directory that cache data will be stored
   --listen value         specify the listen address for the gateway endpoint (default: ":8080")
   --api-listen value     specify the api listening address for the internal control api (default: "127.0.0.1:8081")
   --connmgr-low value    libp2p connection manager 'low' water mark (default: 2000)
   --connmgr-hi value     libp2p connection manager 'high' water mark (default: 3000)
   --connmgr-grace value  libp2p connection manager grace period (default: 1m0s)
   --help, -h             show help (default: false)
   --version, -v          print the version (default: false)
```

## License

Dual-licensed under [MIT](https://github.com/filecoin-project/lotus/blob/master/LICENSE-MIT) + [Apache 2.0](https://github.com/filecoin-project/lotus/blob/master/LICENSE-APACHE)
