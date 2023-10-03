package main

import "context"

// really stupid simple algorithm where we just delete things until weve deleted enough things
func (nd *Node) GC(ctx context.Context, todelete int64) error {
	keys, err := nd.blockstore.AllKeysChan(ctx)
	if err != nil {
		return err
	}

	for todelete > 0 {
		select {
		case k, ok := <-keys:
			if !ok {
				return nil
			}

			size, err := nd.blockstore.GetSize(ctx, k)
			if err != nil {
				log.Warnf("failed to get size for block we are about to delete: %s", err)
			}

			if err := nd.blockstore.DeleteBlock(ctx, k); err != nil {
				return err
			}

			todelete -= int64(size)
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	return nil
}
