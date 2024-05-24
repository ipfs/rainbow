package main

import (
	"context"

	badger4 "github.com/ipfs/go-ds-badger4"
	"github.com/shirou/gopsutil/v3/disk"
)

// GC is a really stupid simple algorithm where we just delete things until
// we've deleted enough things. It is no-op if the current setup does not have
// a (local) blockstore.
func (nd *Node) GC(ctx context.Context, todelete int64) error {
	if nd.blockstore == nil {
		return nil
	}

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

func (nd *Node) periodicGC(ctx context.Context, threshold float64) error {
	stat, err := disk.Usage(nd.dataDir)
	if err != nil {
		return err
	}

	// Calculate % of the total space
	minFreeBytes := uint64((float64(stat.Total) * threshold))

	goLog.Infow("disk data collected", "total_bytes", stat.Total, "available_bytes", stat.Free, "min_free_bytes", minFreeBytes)

	// If there's enough free space, do nothing.
	if minFreeBytes < stat.Free {
		return nil
	}

	bytesToFree := (minFreeBytes - stat.Free)
	if bytesToFree <= 0 {
		return nil
	}

	return nd.GC(ctx, int64(bytesToFree))
}
