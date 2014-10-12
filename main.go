package main

import (
	"os"
	"strconv"
	"strings"
	"time"

	config "github.com/jbenet/go-ipfs/config"
	core "github.com/jbenet/go-ipfs/core"
	cmds "github.com/jbenet/go-ipfs/core/commands"
	crypto "github.com/jbenet/go-ipfs/crypto"
	peer "github.com/jbenet/go-ipfs/peer"
	u "github.com/jbenet/go-ipfs/util"

	"code.google.com/p/go.net/context"

	"bufio"
	"crypto/rand"
	b64 "encoding/base64"
	"encoding/hex"
	"fmt"
	b58 "github.com/jbenet/go-base58"
	"runtime"
)

func _randPeerID() peer.ID {
	buf := make([]byte, 16)
	rand.Read(buf)
	return peer.ID(buf)
}

func _randString() string {
	b := make([]byte, 6)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func GenIdentity() (string, string, error) {
	k, pub, err := crypto.GenerateKeyPair(crypto.RSA, 1024)
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

func setupDHT(addr string, boostrap *core.IpfsNode) *core.IpfsNode {
	cfg := new(config.Config)
	cfg.Addresses.Swarm = addr
	cfg.Datastore.Type = "memory"

	if boostrap != nil {
		bsp := new(config.BootstrapPeer)
		bsp.Address = boostrap.Identity.Addresses[0].String()
		bsp.PeerID = boostrap.Identity.ID.String()
		cfg.Bootstrap = []*config.BootstrapPeer{bsp}
	}

	id, priv, err := GenIdentity()
	if err != nil {
		panic(err)
	}

	cfg.Identity.PeerID = id
	cfg.Identity.PrivKey = priv

	fmt.Printf("Creating node with id: '%s'\n", id)

	node, err := core.NewIpfsNode(cfg, true)
	if err != nil {
		panic(err)
	}

	return node
}

func main() {
	u.Debug = true
	runtime.GOMAXPROCS(10)

	var nodes []*core.IpfsNode
	root := setupDHT("/ip4/127.0.0.1/tcp/4999", nil)
	nodes = []*core.IpfsNode{root}

	for i := 0; i < 15; i++ {
		nodes = append(nodes, setupDHT(fmt.Sprintf("/ip4/127.0.0.1/tcp/%d", 5000+i), root))
	}
	fmt.Println("Finished DHT creation.")

	scan := bufio.NewScanner(os.Stdin)
	fmt.Println("Enter a command:")
	for scan.Scan() {
		cmdparts := strings.Split(scan.Text(), " ")
		idex, err := strconv.Atoi(cmdparts[0])
		if err != nil {
			fmt.Println(err)
			continue
		}
		if len(cmdparts) < 2 {
			fmt.Println("must specify command!")
			continue
		}
		cmd := strings.ToLower(cmdparts[1])
		ctx, _ := context.WithDeadline(context.TODO(), time.Now().Add(time.Second*5))
		switch cmd {
		case "put":
			if len(cmdparts) < 4 {
				fmt.Println("put: '# put key val'")
				continue
			}
			fmt.Printf("putting value: '%s' for key '%s'\n", cmdparts[3], cmdparts[2])
			err := nodes[idex].Routing.PutValue(ctx, u.Key(cmdparts[2]), []byte(cmdparts[3]))
			if err != nil {
				fmt.Println(err)
			}
		case "get":
			if len(cmdparts) < 3 {
				fmt.Println("get: '# get key'")
				continue
			}
			val, err := nodes[idex].Routing.GetValue(ctx, u.Key(cmdparts[2]))
			if err != nil {
				fmt.Println(err)
				continue
			}
			fmt.Printf("Got value: '%s'\n", string(val))
		case "diag":
			diag, err := nodes[idex].Diagnostics.GetDiagnostic(time.Second * 5)
			if err != nil {
				fmt.Println(err)
			}
			cmds.PrintDiagnostics(diag, os.Stdout)
		case "findprov":
			if len(cmdparts) < 4 {
				fmt.Println("findprov: '# findprov key count'")
				continue
			}
			count, err := strconv.Atoi(cmdparts[3])
			if err != nil {
				fmt.Println(err)
				continue
			}
			pchan := nodes[idex].Routing.FindProvidersAsync(ctx, u.Key(cmdparts[2]), count)
			fmt.Printf("Providers of '%s'\n", cmdparts[2])
			for p := range pchan {
				fmt.Printf("\t%s\n", p)
			}
		}
	}

}
