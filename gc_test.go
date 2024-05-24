package main

import (
	"context"
	"testing"

	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPeriodicGC(t *testing.T) {
	t.Parallel()

	gnd := mustTestNode(t, Config{
		Bitswap: true,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cids := []cid.Cid{
		mustAddFile(t, gnd, []byte("a")),
		mustAddFile(t, gnd, []byte("b")),
		mustAddFile(t, gnd, []byte("c")),
		mustAddFile(t, gnd, []byte("d")),
		mustAddFile(t, gnd, []byte("e")),
		mustAddFile(t, gnd, []byte("f")),
	}

	for i, cid := range cids {
		has, err := gnd.blockstore.Has(ctx, cid)
		assert.NoError(t, err, i)
		assert.True(t, has, i)
	}

	// NOTE: ideally, we'd be able to spawn an isolated Rainbow instance with the
	// periodic GC settings configured (interval, threshold). The way it is now,
	// we can only test if the periodicGC function and the GC function work, but
	// not if the timer is being correctly set-up.
	//
	// Tracked in https://github.com/ipfs/rainbow/issues/89
	err := gnd.periodicGC(ctx, 1)
	require.NoError(t, err)

	for i, cid := range cids {
		has, err := gnd.blockstore.Has(ctx, cid)
		assert.NoError(t, err, i)
		assert.False(t, has, i)
	}
}
