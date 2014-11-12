package main

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/jbenet/go-ipfs/config"
	"github.com/jbenet/go-ipfs/core"
	"github.com/jbenet/go-ipfs/crypto"
	u "github.com/jbenet/go-ipfs/util"

	b64 "encoding/base64"
	b58 "github.com/jbenet/go-base58"
)

// GenIdentity creates a random keypair and returns the associated
// peerID and private key encoded to match config values
func GenIdentity() (string, string, error) {
	k, pub, err := crypto.GenerateKeyPair(crypto.RSA, 512)
	if err != nil {
		return "", "", err
	}

	b, err := k.Bytes()
	if err != nil {
		return "", "", err
	}

	privkey := b64.StdEncoding.EncodeToString(b)

	pubkeyb, err := pub.Bytes()
	if err != nil {
		return "", "", err
	}

	id := b58.Encode(u.Hash(pubkeyb))
	return id, privkey, nil
}

// Creates an ipfs node that listens on the given multiaddr and bootstraps to
// the peer in 'bootstrap'
func nodeFromConfig(cfg *config.Config) *core.IpfsNode {
	if !logquiet {
		fmt.Printf("Creating node with id: '%s'\n", cfg.Identity.PeerID)
	}
	node, err := core.NewIpfsNode(cfg, true)
	if err != nil {
		panic(err)
	}

	return node
}

// Parses a range of the form: "[x-y]"
// Also will parse: "x" and "[x]" as single value ranges
func ParseRange(s string) ([]int, error) {
	if len(s) == 0 {
		return nil, errors.New("no input given")
	}
	if s[0] == '[' && s[len(s)-1] == ']' {
		parts := strings.Split(s[1:len(s)-1], "-")
		if len(parts) == 0 {
			return nil, errors.New("No value in range!")
		}
		if len(parts) == 1 {
			n, err := strconv.Atoi(parts[0])
			if err != nil {
				return nil, err
			}
			return []int{n}, nil
		}
		low, err := strconv.Atoi(parts[0])
		if err != nil {
			return nil, err
		}

		high, err := strconv.Atoi(parts[1])
		if err != nil {
			return nil, err
		}

		var out []int
		for i := low; i <= high; i++ {
			out = append(out, i)
		}
		return out, nil

	} else {
		n, err := strconv.Atoi(s)
		if err != nil {
			return nil, err
		}
		return []int{n}, nil
	}
}

func BuildConfig(addr string) *config.Config {
	cfg := new(config.Config)
	cfg.Addresses.Swarm = addr
	cfg.Datastore.Type = "memory"

	id, priv, err := GenIdentity()
	if err != nil {
		panic(err)
	}

	cfg.Identity.PeerID = id
	cfg.Identity.PrivKey = priv

	return cfg
}

func BootstrapTo(cfg *config.Config, root *config.Config) {
	if !logquiet {
		fmt.Printf("%s will connect to %s on startup.\n", cfg.Identity.PeerID, root.Identity.PeerID)
	}
	bsp := new(config.BootstrapPeer)
	bsp.Address = root.Addresses.Swarm
	bsp.PeerID = root.Identity.PeerID
	cfg.Bootstrap = append(cfg.Bootstrap, bsp)
}
