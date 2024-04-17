package main

import (
	"crypto/ed25519"
	crand "crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"

	libp2p "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/mr-tron/base58"
	"golang.org/x/crypto/hkdf"
)

const seedBytes = 32

// newSeed returns a b58 encoded random seed.
func newSeed() (string, error) {
	bs := make([]byte, seedBytes)
	_, err := io.ReadFull(crand.Reader, bs)
	if err != nil {
		return "", err
	}
	return base58.Encode(bs), nil
}

// derive derives libp2p keys from a b58-encoded seed.
func deriveKey(b58secret string, info []byte) (libp2p.PrivKey, error) {
	secret, err := base58.Decode(b58secret)
	if err != nil {
		return nil, err
	}
	if len(secret) < seedBytes {
		return nil, errors.New("derivation seed is too short")
	}

	hash := sha256.New
	hkdf := hkdf.New(hash, secret, nil, info)
	keySeed := make([]byte, ed25519.SeedSize)
	if _, err := io.ReadFull(hkdf, keySeed); err != nil {
		return nil, err
	}
	key := ed25519.NewKeyFromSeed(keySeed)
	return libp2p.UnmarshalEd25519PrivateKey(key)
}

func deriveKeyInfo(index int) []byte {
	return []byte(fmt.Sprintf("rainbow-%d", index))
}
