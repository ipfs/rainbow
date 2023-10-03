# rainbow 

> The rainbow bridge between your CIDs and your html, javascript, wasm, jpegs, mp4s, tars, CARs, blocks, and more!

## About

Rainbow is an experimental specifically dedicated implementation of the [IPFS HTTP Gateway API](https://specs.ipfs.tech/http-gateways), 
based on [boxo](https://github.com/ipfs/boxo) which is the tooling the powers [kubo](https://github.com/ipfs/kubo).

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
   v0.0.1-dev

COMMANDS:
   help, h  Shows a list of commands or help for one command

GLOBAL OPTIONS:
   --datadir value        specify the directory that cache data will be stored
   --listen value         specify the listen address for the gateway endpoint (default: ":8080")
   --api-listen value     specify the api listening address for the internal control api (default: "127.0.0.1:8081")
   --connmgr-low value    libp2p connection manager 'low' water mark (default: 100)
   --connmgr-hi value     libp2p connection manager 'high' water mark (default: 3000)
   --connmgr-grace value  libp2p connection manager grace period (default: 1m0s)
   --routing value        RoutingV1 Endpoint (default: "http://127.0.0.1:8090")
   --help, -h             show help (default: false)
   --version, -v          print the version (default: false)
```

## License

Dual-licensed under [MIT](https://github.com/filecoin-project/lotus/blob/master/LICENSE-MIT) + [Apache 2.0](https://github.com/filecoin-project/lotus/blob/master/LICENSE-APACHE)
