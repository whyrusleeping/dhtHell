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
	u "github.com/jbenet/go-ipfs/util"

	"flag"

	"code.google.com/p/go.net/context"

	"bufio"
	b64 "encoding/base64"
	"fmt"
	"runtime"

	b58 "github.com/jbenet/go-base58"
)

// GenIdentity creates a random keypair and returns the associated
// peerID and private key encoded to match config values
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

// Creates an ipfs node that listens on the given multiaddr and boostraps to
// the peer in 'bootstrap'
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

// global array of nodes, because im lazy and hate passing things to functions
var nodes []*core.IpfsNode

func main() {
	nnodes := flag.Int("n", 15, "number of nodes to spawn")
	flag.Parse()

	u.Debug = true
	runtime.GOMAXPROCS(10)

	root := setupDHT("/ip4/127.0.0.1/tcp/4999", nil)
	nodes = []*core.IpfsNode{root}

	for i := 0; i < *nnodes; i++ {
		nodes = append(nodes, setupDHT(fmt.Sprintf("/ip4/127.0.0.1/tcp/%d", 5000+i), root))
	}
	fmt.Println("Finished DHT creation.")

	scan := bufio.NewScanner(os.Stdin)
	fmt.Println("Enter a command:")
	for scan.Scan() {
		if scan.Text() == "quit" {
			return
		}
		cmdparts := strings.Split(scan.Text(), " ")
		idex, err := strconv.Atoi(cmdparts[0])
		if err != nil {
			fmt.Println(err)
			continue
		}
		if idex > *nnodes || idex < 0 {
			fmt.Println("Index out of range!")
			continue
		}
		if len(cmdparts) < 2 {
			fmt.Println("must specify command!")
			continue
		}
		cmd := strings.ToLower(cmdparts[1])
		switch cmd {
		case "put":
			Put(idex, cmdparts)
		case "get":
			Get(idex, cmdparts)
		case "findprov":
			FindProv(idex, cmdparts)
		case "store":
			Store(idex, cmdparts)
		case "provide":
			Provide(idex, cmdparts)
		case "diag":
			Diag(idex, cmdparts)
		}
	}
}

func Put(idex int, cmdparts []string) {
	if len(cmdparts) < 4 {
		fmt.Println("put: '# put key val'")
		return
	}
	fmt.Printf("putting value: '%s' for key '%s'\n", cmdparts[3], cmdparts[2])
	ctx, _ := context.WithDeadline(context.TODO(), time.Now().Add(time.Second*5))
	err := nodes[idex].Routing.PutValue(ctx, u.Key(cmdparts[2]), []byte(cmdparts[3]))
	if err != nil {
		fmt.Println(err)
	}
}

func Get(idex int, cmdparts []string) {
	if len(cmdparts) < 3 {
		fmt.Println("get: '# get key'")
		return
	}
	ctx, _ := context.WithDeadline(context.TODO(), time.Now().Add(time.Second*5))
	val, err := nodes[idex].Routing.GetValue(ctx, u.Key(cmdparts[2]))
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Printf("Got value: '%s'\n", string(val))
}

func Diag(idex int, cmdparts []string) {
	diag, err := nodes[idex].Diagnostics.GetDiagnostic(time.Second * 5)
	if err != nil {
		fmt.Println(err)
	}
	cmds.PrintDiagnostics(diag, os.Stdout)

}

func Store(idex int, cmdparts []string) {
	if len(cmdparts) < 4 {
		fmt.Println("store: '# store key val'")
		return
	}
	err := nodes[idex].Datastore.Put(u.Key(cmdparts[2]).DsKey(), []byte(cmdparts[3]))
	if err != nil {
		fmt.Println(err)
		return
	}

}

func Provide(idex int, cmdparts []string) {
	if len(cmdparts) < 3 {
		fmt.Println("provide: '# provide key'")
		return
	}
	ctx, _ := context.WithDeadline(context.TODO(), time.Now().Add(time.Second*5))
	err := nodes[idex].Routing.Provide(ctx, u.Key(cmdparts[2]))
	if err != nil {
		fmt.Println(err)
		return
	}
}

func FindProv(idex int, cmdparts []string) {
	if len(cmdparts) < 4 {
		fmt.Println("findprov: '# findprov key count'")
		return
	}
	count, err := strconv.Atoi(cmdparts[3])
	if err != nil {
		fmt.Println(err)
		return
	}
	ctx, _ := context.WithDeadline(context.TODO(), time.Now().Add(time.Second*5))
	pchan := nodes[idex].Routing.FindProvidersAsync(ctx, u.Key(cmdparts[2]), count)
	fmt.Printf("Providers of '%s'\n", cmdparts[2])
	for p := range pchan {
		fmt.Printf("\t%s\n", p)
	}
}
