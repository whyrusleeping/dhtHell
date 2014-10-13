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

	"flag"

	"code.google.com/p/go.net/context"

	"bufio"
	b64 "encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"runtime"

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

// Creates an ipfs node that listens on the given multiaddr and boostraps to
// the peer in 'bootstrap'
func setupDHT(cfg *config.Config) *core.IpfsNode {
	fmt.Printf("Creating node with id: '%s'\n", cfg.Identity.PeerID)
	node, err := core.NewIpfsNode(cfg, true)
	if err != nil {
		panic(err)
	}

	return node
}

// Parses a range of the form: "[x-y]"
// Also will parse: "x" and "[x]" as single value ranges
func ParseRange(s string) ([]int, error) {
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
	fmt.Printf("%s will connect to %s on startup.\n", cfg.Identity.PeerID, root.Identity.PeerID)
	bsp := new(config.BootstrapPeer)
	bsp.Address = root.Addresses.Swarm
	bsp.PeerID = root.Identity.PeerID
	cfg.Bootstrap = append(cfg.Bootstrap, bsp)
}

type testConfig struct {
	NumNodes int
}

func ParseCommandFile(finame string, cfg *testConfig) (*bufio.Scanner, error) {
	fi, err := os.Open(finame)
	if err != nil {
		return nil, err
	}
	scan := bufio.NewScanner(fi)
	if !scan.Scan() {
		return nil, errors.New("Invalid file syntax! first line must be num nodes")
	}

	num, err := strconv.Atoi(scan.Text())
	if err != nil {
		return nil, err
	}

	cfg.NumNodes = num
	SetupNConfigs(cfg)

	boostrappingSet := false

	for scan.Scan() {
		if scan.Text() == "--" {
			goto out
		}
		if strings.Contains(scan.Text(), "->") {

			parts := strings.Split(scan.Text(), "->")
			lrange, err := ParseRange(parts[0])
			if err != nil {
				fmt.Printf("Error parsing range: %s\n", err)
				continue
			}
			rrange, err := ParseRange(parts[1])
			if err != nil {
				fmt.Printf("Error parsing range: %s\n", err)
				continue
			}

			for _, n := range lrange {
				for _, r := range rrange {
					BootstrapTo(configs[n], configs[r])
				}
			}
			boostrappingSet = true
		} else {
			fmt.Printf("Invalid Syntax for setup: '%s'\n", scan.Text())
			continue
		}
	}

	// If we read through the whole file as config, set to read commands from stdin
	scan = nil
out:

	// If no bootstrapping options selected, everyone boostraps with node 0
	if !boostrappingSet {
		for i := 1; i < len(configs); i++ {
			BootstrapTo(configs[i], configs[0])
		}
	}
	return scan, nil
}

func SetupNConfigs(c *testConfig) {
	for i := 0; i < c.NumNodes; i++ {
		ncfg := BuildConfig(fmt.Sprintf("/ip4/127.0.0.1/tcp/%d", 5000+i))
		configs = append(configs, ncfg)
	}
}

// global array of nodes, because im lazy and hate passing things to functions
var nodes []*core.IpfsNode
var configs []*config.Config

func main() {
	nnodes := flag.Int("n", 15, "number of nodes to spawn")
	cmdfile := flag.String("f", "", "a file of commands to run")
	flag.Parse()

	u.Debug = true
	runtime.GOMAXPROCS(10)

	var scan *bufio.Scanner
	var err error
	testconf := new(testConfig)
	if *cmdfile != "" {
		scan, err = ParseCommandFile(*cmdfile, testconf)
		if err != nil {
			fmt.Println(err)
			return
		}
	} else {
		// Setup default configs
		testconf.NumNodes = *nnodes
		SetupNConfigs(testconf)
		for i := 1; i < len(configs); i++ {
			BootstrapTo(configs[i], configs[0])
		}
	}

	for _, ncfg := range configs {
		nodes = append(nodes, setupDHT(ncfg))
	}
	fmt.Println("Finished DHT creation.")

	if scan == nil {
		scan = bufio.NewScanner(os.Stdin)
		fmt.Println("Enter a command:")
	}
	for scan.Scan() {
		if scan.Text() == "==" {
			scan = bufio.NewScanner(os.Stdin)
			continue
		}
		con := RunCommand(scan.Text())
		if !con {
			return
		}
	}
}

func RunCommand(cmdstr string) bool {
	if cmdstr == "quit" {
		return false
	}
	cmdparts := strings.Split(cmdstr, " ")
	idex, err := strconv.Atoi(cmdparts[0])
	if err != nil {
		fmt.Println(err)
		return true
	}
	if idex >= len(nodes) || idex < 0 {
		fmt.Println("Index out of range!")
		return true
	}
	if len(cmdparts) < 2 {
		fmt.Println("must specify command!")
		return true
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
	case "findpeer":
		FindPeer(idex, cmdparts)
	}
	return true
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
	var jsonout bool
	if len(cmdparts) == 3 {
		if cmdparts[2] == "json" {
			jsonout = true
		}
	}
	if jsonout {
		enc := json.NewEncoder(os.Stdout)
		err := enc.Encode(diag)
		if err != nil {
			fmt.Println(err)
		}
	} else {
		cmds.PrintDiagnostics(diag, os.Stdout)
	}

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
		fmt.Println("findprov: '# findprov key [count]'")
		return
	}
	count := 1
	var err error
	if len(cmdparts) >= 4 {
		count, err = strconv.Atoi(cmdparts[3])
		if err != nil {
			fmt.Println(err)
			return
		}
	}
	ctx, _ := context.WithDeadline(context.TODO(), time.Now().Add(time.Second*5))
	pchan := nodes[idex].Routing.FindProvidersAsync(ctx, u.Key(cmdparts[2]), count)
	fmt.Printf("Providers of '%s'\n", cmdparts[2])
	for p := range pchan {
		fmt.Printf("\t%s\n", p)
	}
}

func FindPeer(idex int, cmdparts []string) {
	if len(cmdparts) < 3 {
		fmt.Println("findpeer: '# findpeer peerid'")
		return
	}

	ctx, _ := context.WithDeadline(context.TODO(), time.Now().Add(time.Second*5))
	p, err := nodes[idex].Routing.FindPeer(ctx, peer.ID(cmdparts[2]))
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Printf("Got peer: %s\n", p)
}
