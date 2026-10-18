package main

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/ipfs/boxo/blockstore"
	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore/query"
	flatfs "github.com/ipfs/go-ds-flatfs"
	"github.com/stretchr/testify/require"
)

// Observe Close instead of draining the key channel: draining it would let the
// producer finish naturally and hide the leak this test is meant to catch.
type gcQueryResults struct {
	query.Results
	closed chan struct{}
}

func (r *gcQueryResults) Close() error {
	err := r.Results.Close()
	close(r.closed)
	return err
}

type gcFlatfsDatastore struct {
	*flatfs.Datastore
	results *gcQueryResults
}

func (ds *gcFlatfsDatastore) Query(ctx context.Context, q query.Query) (query.Results, error) {
	results, err := ds.Datastore.Query(ctx, q)
	if err != nil {
		return nil, err
	}
	ds.results = &gcQueryResults{Results: results, closed: make(chan struct{})}
	return ds.results, nil
}

func TestGCPartialFlatfsClosesQuery(t *testing.T) {
	ds, err := flatfs.CreateOrOpen(filepath.Join(t.TempDir(), "blocks"), flatfs.NextToLast(3), false)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, ds.Close()) })
	tracked := &gcFlatfsDatastore{Datastore: ds}
	bs := blockstore.NewBlockstore(tracked, blockstore.NoPrefix(), blockstore.WriteThrough(true))
	nd := &Node{blockstore: bs, datastore: tracked}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	// Exceed both the Boxo key channel and flatfs query result buffers, so
	// enumeration cannot finish while GC only consumes its first key.
	cids := make([]cid.Cid, 4*query.KeysOnlyBufSize+1)
	for i := range cids {
		block := blocks.NewBlock([]byte(fmt.Sprintf("block-%d", i)))
		cids[i] = block.Cid()
		require.NoError(t, bs.Put(ctx, block))
	}

	for pass := 1; pass <= 3; pass++ {
		require.NoError(t, nd.GC(ctx, 1))
		select {
		case <-tracked.results.closed:
		case <-time.After(5 * time.Second):
			t.Fatal("GC returned without closing its blockstore query")
		}
		select {
		case <-tracked.results.Done():
		default:
			t.Fatal("flatfs query producer is still running after Close")
		}
		require.NoError(t, ctx.Err(), "GC must not cancel its caller's context")

		remaining := 0
		for _, key := range cids {
			has, err := bs.Has(ctx, key)
			require.NoError(t, err)
			if has {
				remaining++
			}
		}
		require.Equal(t, len(cids)-pass, remaining)
	}
}

type gcTestBlockstore struct {
	blockstore.Blockstore
	ctx           context.Context
	done          chan struct{}
	started       chan struct{}
	deleteErr     error
	queryErr      error
	deletes       int
	waitForCancel bool
}

func (bs *gcTestBlockstore) AllKeysChan(ctx context.Context) (<-chan cid.Cid, error) {
	bs.ctx = ctx
	if bs.started != nil {
		close(bs.started)
	}
	if bs.queryErr != nil {
		return nil, bs.queryErr
	}
	keys := make(chan cid.Cid)
	go func() {
		defer close(bs.done)
		// Keep keys open in the cancellation-only case so GC must observe
		// ctx.Done rather than racing cancellation against channel exhaustion.
		if bs.waitForCancel {
			<-ctx.Done()
			return
		}
		defer close(keys)
		key := blocks.NewBlock([]byte("test block")).Cid()
		for {
			select {
			case keys <- key:
			case <-ctx.Done():
				return
			}
		}
	}()
	return keys, nil
}

func (bs *gcTestBlockstore) GetSize(context.Context, cid.Cid) (int, error) {
	return 1, nil
}

func (bs *gcTestBlockstore) DeleteBlock(context.Context, cid.Cid) error {
	bs.deletes++
	return bs.deleteErr
}

func TestGCCancelsKeyEnumeration(t *testing.T) {
	deleteErr := errors.New("delete failed")
	queryErr := errors.New("query failed")
	for _, tc := range []struct {
		name      string
		target    int64
		deleteErr error
		queryErr  error
		deletes   int
		wantErr   error
	}{
		{name: "partial collection", target: 1, deletes: 1},
		{name: "delete failure", target: 1, deleteErr: deleteErr, deletes: 1, wantErr: deleteErr},
		{name: "zero target"},
		{name: "negative target", target: -1},
		{name: "query failure", target: 1, queryErr: queryErr, wantErr: queryErr},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			t.Cleanup(cancel)
			bs := &gcTestBlockstore{done: make(chan struct{}), deleteErr: tc.deleteErr, queryErr: tc.queryErr}
			nd := &Node{blockstore: bs}
			require.ErrorIs(t, nd.GC(ctx, tc.target), tc.wantErr)
			require.Equal(t, tc.deletes, bs.deletes)
			require.NoError(t, ctx.Err())
			select {
			case <-bs.ctx.Done():
			case <-time.After(5 * time.Second):
				t.Fatal("GC did not cancel its enumeration context")
			}
			if tc.queryErr == nil {
				select {
				case <-bs.done:
				case <-time.After(5 * time.Second):
					t.Fatal("key producer did not exit")
				}
			}
		})
	}
}

func TestGCParentCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	bs := &gcTestBlockstore{done: make(chan struct{}), started: make(chan struct{}), waitForCancel: true}
	nd := &Node{blockstore: bs}
	result := make(chan error, 1)
	go func() { result <- nd.GC(ctx, 1) }()
	select {
	case <-bs.started:
	case <-time.After(5 * time.Second):
		t.Fatal("GC did not start key enumeration")
	}
	cancel()
	select {
	case err := <-result:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(5 * time.Second):
		t.Fatal("GC did not return after parent cancellation")
	}
	select {
	case <-bs.done:
	case <-time.After(5 * time.Second):
		t.Fatal("key producer did not exit after parent cancellation")
	}
	require.Zero(t, bs.deletes)
}
