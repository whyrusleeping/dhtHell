package main

import (
	"os"
	"runtime/pprof"
	"strconv"
	"strings"
	"sync"
	"time"

	"code.google.com/p/go.net/context"

	config "github.com/jbenet/go-ipfs/repo/config"
	u "github.com/jbenet/go-ipfs/util"

	"flag"

	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"runtime"
)

var _ = json.Decoder{}

var ErrArgCount = errors.New("not enough arguments")

// Test config represents a test configuration
// right now, its just the number of nodes...
// TODO: decide if its still worth keeping a struct around
type testConfig struct {
	NumNodes int
}

type nodeBWInfo struct {
	BwIn, BwOut      uint64
	MesSend, MesRecv uint64
}

type transferInfo struct {
	Size  int
	Time  int64
	Speed float64
}

type Statistics struct {
	BwStats   []nodeBWInfo
	Transfers []transferInfo
}

var gslock sync.Mutex
var globalStats Statistics

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
	} else if strings.HasPrefix(s, "off") {
		parts := strings.Split(s, " ")
		if len(parts) < 2 {
			fmt.Printf("Syntax error, no range given!\n")
			return false
		}
		rng, err := ParseRange(parts[1])
		if err != nil {
			fmt.Printf("Syntax error: %s\n", err)
			return false
		}

		for _, v := range rng {
			disabledAtStart[v] = true
		}
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
		if ExecConfigLine(scan.Text()) {
			goto out
		}
	}

	// If we read through the whole file as config, set to read commands from stdin
	// XXX: hacky
	scan = nil
out:

	// If no bootstrapping options selected, everyone bootstraps with node 0
	if !bootstrappingSet {
		fmt.Println("Setting default bootstrapping config.")
		for i := 1; i < len(configs); i++ {
			BootstrapTo(configs[i], configs[0])
		}
	}
	return scan, nil
}

func SetupNConfigs(c *testConfig) {
	disabledAtStart = make([]bool, c.NumNodes)
	for i := 0; i < c.NumNodes; i++ {
		ncfg := BuildConfig(fmt.Sprintf("/ip4/127.0.0.1/tcp/%d", 10000+i))
		if setuprpc {
			ncfg.Addresses.API = fmt.Sprintf("/ip4/127.0.0.1/tcp/%d", 9000+i)
		}
		configs = append(configs, ncfg)
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

func SetupNodes(master context.Context) {
	controllers = make([]NodeController, len(configs))
	for i, ncfg := range configs {
		if !disabledAtStart[i] {
			nd := nodeFromConfig(master, ncfg)
			controllers[i] = &localNode{nd}
		}
	}
	fmt.Println("Finished DHT creation.")
}

// global array of nodes, because im lazy and hate passing things to functions
var controllers []NodeController
var configs []*config.Config
var disabledAtStart []bool
var setuprpc bool
var bootstrappingSet bool
var logquiet bool
var masterCtx context.Context

func main() {
	cmdfile := flag.String("f", "", "a file of commands to run")
	serv := flag.String("s", "", "address to run d3 viz server on")
	rpc := flag.Bool("r", false, "whether or not to turn on rpc")
	def := flag.Bool("default", false, "whether or not to load default config")
	ins := flag.Bool("inspect", false, "whether or not to inspect stack afterwards")
	quiet := flag.Bool("q", false, "supress obnoxious log messages")
	flag.Parse()
	logquiet = *quiet

	setuprpc = *rpc

	u.Debug = true
	runtime.GOMAXPROCS(10)

	if *serv != "" {
		go RunServer(*serv)
	}

	// Setup Configuration and inputs
	var scan *bufio.Scanner
	testconf := new(testConfig)
	if *cmdfile != "" {
		fiscan, err := ParseCommandFile(*cmdfile, testconf)
		if err != nil {
			fmt.Println(err)
			return
		}
		scan = fiscan
	} else {
		scan = bufio.NewScanner(os.Stdin)
		if *def { // Default configuration
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

	ctx, cancel := context.WithCancel(context.TODO())
	masterCtx = ctx

	// Build ipfs nodes as specified by the global array of configurations
	SetupNodes(ctx)

	defer func() {
		fi, err := os.Create("mem.prof")
		if err != nil {
			panic(err)
		}
		pprof.WriteHeapProfile(fi)
		fi.Close()
	}()

	fi, err := os.Create("cpu.prof")
	if err != nil {
		panic(err)
	}
	defer fi.Close()

	pprof.StartCPUProfile(fi)
	defer pprof.StopCPUProfile()

	// Begin command execution
	fmt.Println("Enter a command:")
	for scan.Scan() {
		if len(scan.Text()) == 0 {
			continue
		}

		// ignore comments
		if len(scan.Text()) > 0 && scan.Text()[0] == '#' {
			continue
		}
		if scan.Text() == "==" {
			// Switch over input to standard in
			scan = bufio.NewScanner(os.Stdin)
			continue
		}
		if !RunCommand(scan.Text()) {
			return
		}
	}

	cancel()
	fmt.Println("Cleaning up and printing bandwidth(I/O)")
	/*
		for _, c := range controllers {
			globalStats.BwStats = append(globalStats.BwStats, c.GetStatistics())
		}

		gsjson, err := json.MarshalIndent(globalStats, "", "\t")
		if err != nil {
			panic(err)
		}
		fmt.Println(string(gsjson))
	*/

	if *ins {
		time.Sleep(time.Second * 2)
		panic("lets take a look at things.")
	}
}
