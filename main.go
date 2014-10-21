package main

import (
	"os"
	"strconv"
	"strings"
	"time"

	config "github.com/jbenet/go-ipfs/config"
	core "github.com/jbenet/go-ipfs/core"
	crypto "github.com/jbenet/go-ipfs/crypto"
	"github.com/jbenet/go-ipfs/diagnostics"
	u "github.com/jbenet/go-ipfs/util"

	"flag"
	"net/http"

	"bufio"
	b64 "encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"runtime"

	b58 "github.com/jbenet/go-base58"
)

var _ = json.Decoder{}

var ErrArgCount = errors.New("not enough arguments")

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
	fmt.Printf("%s will connect to %s on startup.\n", cfg.Identity.PeerID, root.Identity.PeerID)
	bsp := new(config.BootstrapPeer)
	bsp.Address = root.Addresses.Swarm
	bsp.PeerID = root.Identity.PeerID
	cfg.Bootstrap = append(cfg.Bootstrap, bsp)
}

type testConfig struct {
	NumNodes int
}

func ExecConfigLine(s string) bool {
	if len(s) > 0 && s[0] == '#' {
		return false
	}
	if s == "--" {
		return true
	}
	if strings.Contains(s, "->") {

		parts := strings.Split(s, "->")
		lrange, err := ParseRange(parts[0])
		if err != nil {
			fmt.Printf("Error parsing range: %s\n", err)
			return false
		}
		rrange, err := ParseRange(parts[1])
		if err != nil {
			fmt.Printf("Error parsing range: %s\n", err)
			return false
		}

		for _, n := range lrange {
			for _, r := range rrange {
				BootstrapTo(configs[n], configs[r])
			}
		}
		bootstrappingSet = true
	} else {
		fmt.Printf("Invalid Syntax for setup: '%s'\n", s)
		return false
	}
	return false
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
			bootstrappingSet = true
		} else {
			fmt.Printf("Invalid Syntax for setup: '%s'\n", scan.Text())
			continue
		}
	}

	// If we read through the whole file as config, set to read commands from stdin
	scan = nil
out:

	// If no bootstrapping options selected, everyone bootstraps with node 0
	if !bootstrappingSet {
		for i := 1; i < len(configs); i++ {
			BootstrapTo(configs[i], configs[0])
		}
	}
	return scan, nil
}

func SetupNConfigs(c *testConfig) {
	for i := 0; i < c.NumNodes; i++ {
		ncfg := BuildConfig(fmt.Sprintf("/ip4/127.0.0.1/tcp/%d", 5000+i))
		if setuprpc {
			ncfg.Addresses.API = fmt.Sprintf("/ip4/127.0.0.1/tcp/%d", 9000+i)
		}
		configs = append(configs, ncfg)
	}
}

// Runs the visualization server to view d3 graph of the network
func RunServer(s string) {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "index.html")
	})
	http.HandleFunc("/data", func(w http.ResponseWriter, r *http.Request) {
		diag, err := nodes[0].Diagnostics.GetDiagnostic(time.Second * 10)
		if err != nil {
			fmt.Println(err)
		}
		w.Write(diagnostics.GetGraphJson(diag))
	})
	err := http.ListenAndServe(s, nil)
	if err != nil {
		fmt.Println(err)
	}
}

func ConfigPrompt(scan *bufio.Scanner) error {
	fmt.Println("Please enter number of nodes:")
	if !scan.Scan() {
		return errors.New("not enough input!")
	}
	nnum := scan.Text()
	n, err := strconv.Atoi(nnum)
	if err != nil {
		return err
	}

	c := new(testConfig)
	c.NumNodes = n
	SetupNConfigs(c)

	fmt.Println("Enter bootstrapping config: ('--' to stop)")
	for scan.Scan() {
		if ExecConfigLine(scan.Text()) {
			break
		}
	}

	return nil
}

// global array of nodes, because im lazy and hate passing things to functions
var nodes []*core.IpfsNode
var configs []*config.Config
var setuprpc bool
var bootstrappingSet bool

func main() {
	//nnodes := flag.Int("n", 15, "number of nodes to spawn")
	cmdfile := flag.String("f", "", "a file of commands to run")
	serv := flag.String("s", "", "address to run d3 viz server on")
	rpc := flag.Bool("r", false, "whether or not to turn on rpc")
	def := flag.Bool("default", false, "whether or not to load default config")
	flag.Parse()

	setuprpc = *rpc

	u.Debug = true
	runtime.GOMAXPROCS(10)

	if *serv != "" {
		go RunServer(*serv)
	}

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
		scan = bufio.NewScanner(os.Stdin)
		if *def {
			testconf.NumNodes = 15
			SetupNConfigs(testconf)
			for _, cfg := range configs[1:] {
				BootstrapTo(cfg, configs[0])
			}
		} else {
			ConfigPrompt(scan)
			if scan.Err() != nil {
				fmt.Printf("Scan error: %s\n", scan.Err())
			}
		}
	}

	for _, ncfg := range configs {
		nodes = append(nodes, setupDHT(ncfg))
	}
	fmt.Println("Finished DHT creation.")

	fmt.Println("Enter a command:")
	for scan.Scan() {
		if len(scan.Text()) > 0 && scan.Text()[0] == '#' {
			continue
		}
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
