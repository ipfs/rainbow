//go:build !windows

package main

import (
	"context"
	"syscall"
)

func (nd *Node) periodicGC(ctx context.Context, threshold float64) error {
	var stat syscall.Statfs_t

	err := syscall.Statfs(nd.dataDir, &stat)
	if err != nil {
		return err
	}

	totalBytes := uint64(stat.Blocks) * uint64(stat.Bsize)
	availableBytes := stat.Bfree * uint64(stat.Bsize)

	// Calculate % of the total space
	minFreeBytes := uint64((float64(totalBytes) * threshold))

	goLog.Infow("fileystem data collected", "total_bytes", totalBytes, "available_bytes", availableBytes, "min_free_bytes", minFreeBytes)

	// If there's enough free space, do nothing.
	if minFreeBytes > availableBytes {
		return nil
	}

	bytesToFree := (minFreeBytes - availableBytes)
	if bytesToFree <= 0 {
		return nil
	}

	return nd.GC(ctx, int64(bytesToFree))
}
