package main

import (
	"context"
	"fmt"
	"io"
	gopath "path"
	"strings"

	"github.com/ipfs/go-blockservice"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-fetcher"
	files "github.com/ipfs/go-ipfs-files"
	format "github.com/ipfs/go-ipld-format"
	"github.com/ipfs/go-merkledag"
	namesys "github.com/ipfs/go-namesys"
	"github.com/ipfs/go-namesys/resolve"
	ipfspath "github.com/ipfs/go-path"
	ipfspathresolver "github.com/ipfs/go-path/resolver"
	unixfile "github.com/ipfs/go-unixfs/file"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
	options "github.com/ipfs/interface-go-ipfs-core/options"
	path "github.com/ipfs/interface-go-ipfs-core/path"
)

type Api struct {
	nd *Node

	bsses *blockservice.Session
}

func (api *Api) session(ctx context.Context) *Api {
	sess := blockservice.NewSession(ctx, api.nd.Blockservice)

	return &Api{
		nd:    api.nd,
		bsses: sess,
	}
}

func (api *Api) dag(ctx context.Context) format.DAGService {
	if api.bsses != nil {
		return merkledag.NewReadOnlyDagService(merkledag.WrapSession(api.bsses))
	}
	return merkledag.NewDAGService(api.nd.Blockservice)
}

type fetcherSessioner interface {
	FetcherWithSession(context.Context, *blockservice.Session) fetcher.Fetcher
}

func (api *Api) fetcher(ctx context.Context) fetcher.Fetcher {
	if api.bsses != nil {
		fs := api.nd.unixFSFetcherFactory.(fetcherSessioner)
		return fs.FetcherWithSession(ctx, api.bsses)
	}
	return api.nd.unixFSFetcherFactory.NewSession(ctx)
}

type fetcherFactoryShim struct {
	f fetcher.Fetcher
}

func (ffs *fetcherFactoryShim) NewSession(ctx context.Context) fetcher.Fetcher {
	return ffs.f
}

func (api *Api) ResolvePath(ctx context.Context, p path.Path) (path.Resolved, error) {
	if rp, ok := p.(path.Resolved); ok {
		return rp, nil
	}
	if err := p.IsValid(); err != nil {
		return nil, err
	}

	ipath := ipfspath.Path(p.String())
	ipath, err := resolve.ResolveIPNS(ctx, api.nd.Namesys, ipath)
	if err == resolve.ErrNoNamesys {
		return nil, coreiface.ErrOffline
	} else if err != nil {
		return nil, err
	}

	if ipath.Segments()[0] != "ipfs" && ipath.Segments()[0] != "ipld" {
		return nil, fmt.Errorf("unsupported path namespace: %s", p.Namespace())
	}

	/*
		var dataFetcher fetcher.Factory
		if ipath.Segments()[0] == "ipld" {
			dataFetcher = api.ipldFetcherFactory
		} else {
			dataFetcher = api.unixFSFetcherFactory
		}
	*/

	resolver := ipfspathresolver.NewBasicResolver(&fetcherFactoryShim{api.fetcher(ctx)})

	node, rest, err := resolver.ResolveToLastNode(ctx, ipath)
	if err != nil {
		return nil, err
	}

	root, err := cid.Parse(ipath.Segments()[1])
	if err != nil {
		return nil, err
	}

	return path.NewResolvedPath(ipath, node, root, gopath.Join(rest...)), nil
}

func (api *Api) ResolveNode(ctx context.Context, p path.Path) (format.Node, error) {
	rp, err := api.ResolvePath(ctx, p)
	if err != nil {
		return nil, err
	}

	node, err := api.dag(ctx).Get(ctx, rp.Cid())
	if err != nil {
		return nil, err
	}

	return node, nil
}

func (api *Api) Get(ctx context.Context, p path.Path) (files.Node, error) {
	// TODO: start session in a way that everything uses that same session

	nd, err := api.ResolveNode(ctx, p)
	if err != nil {
		return nil, err
	}

	return unixfile.NewUnixfsFile(ctx, api.dag(ctx), nd)
}

func (api *Api) NameResolve(ctx context.Context, name string, opts ...options.NameResolveOption) (path.Path, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	results, err := api.Search(ctx, name, opts...)
	if err != nil {
		return nil, err
	}

	err = coreiface.ErrResolveFailed
	var p path.Path

	for res := range results {
		p, err = res.Path, res.Err
		if err != nil {
			break
		}
	}

	return p, err
}

func (api *Api) Search(ctx context.Context, name string, opts ...options.NameResolveOption) (<-chan coreiface.IpnsResult, error) {
	options, err := options.NameResolveOptions(opts...)
	if err != nil {
		return nil, err
	}

	/*
		err = api.checkOnline(true)
		if err != nil {
			return nil, err
		}
	*/

	var resolver namesys.Resolver = api.nd.Namesys
	if !options.Cache {
		resolver, err = namesys.NewNameSystem(api.nd.FullRT,
			namesys.WithDatastore(api.nd.Datastore))
		if err != nil {
			return nil, err
		}
	}

	if !strings.HasPrefix(name, "/ipns/") {
		name = "/ipns/" + name
	}

	out := make(chan coreiface.IpnsResult)
	go func() {
		defer close(out)
		for res := range resolver.ResolveAsync(ctx, name, options.ResolveOpts...) {
			select {
			case out <- coreiface.IpnsResult{Path: path.New(res.Path.String()), Err: res.Err}:
			case <-ctx.Done():
				return
			}
		}
	}()

	return out, nil
}

func (api *Api) DagExport(ctx context.Context, c cid.Cid, w io.Writer) error {
	panic("NYI")
}
