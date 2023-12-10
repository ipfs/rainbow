package main

import (
	"context"

	badger4 "github.com/ipfs/go-ds-badger4"
)

// GC is a really stupid simple algorithm where we just delete things until
// weve deleted enough things
func (nd *Node) GC(ctx context.Context, todelete int64) error {
	keys, err := nd.blockstore.AllKeysChan(ctx)
	if err != nil {
		return err
	}

deleteBlocks:
	for todelete > 0 {
		select {
		case k, ok := <-keys:
			if !ok {
				break deleteBlocks
			}

			size, err := nd.blockstore.GetSize(ctx, k)
			if err != nil {
				goLog.Warnf("failed to get size for block we are about to delete: %s", err)
			}

			if err := nd.blockstore.DeleteBlock(ctx, k); err != nil {
				return err
			}

			todelete -= int64(size)
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	if ds, ok := nd.datastore.(*badger4.Datastore); ok {
		err = ds.CollectGarbage(ctx)
		if err != nil {
			return err
		}
	}

	return nil
}
