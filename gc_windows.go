//go:build windows

package main

import (
	"context"
	"errors"
)

func (nd *Node) periodicGC(ctx context.Context, threshold float64) error {
	return errors.New("feature not implemented on windows")
}
