package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
	"strings"
	"time"

	"code.google.com/p/go.net/context"
	b58 "github.com/jbenet/go-base58"
	"github.com/jbenet/go-ipfs/core"
	diagnostics "github.com/jbenet/go-ipfs/diagnostics"
	imp "github.com/jbenet/go-ipfs/importer"
	chunk "github.com/jbenet/go-ipfs/importer/chunk"
	"github.com/jbenet/go-ipfs/p2p/peer"
	uio "github.com/jbenet/go-ipfs/unixfs/io"
	u "github.com/jbenet/go-ipfs/util"
)

type NodeController interface {
	// Run a command on this node
	RunCommand(cmd []string) (string, error)

	// Shutdown this node
	Shutdown()

	GetStatistics() nodeBWInfo

	// return this nodes peer ID
	PeerID() peer.ID
}

type localNode struct {
	n *core.IpfsNode
}

func (l *localNode) RunCommand(cmdparts []string) (string, error) {
	if l.n == nil {
		return "", errors.New("Attempted to run command on dead node!")
	}
	cmd := strings.ToLower(cmdparts[1])
	if cmd == "expectget" {
		success := AssertGet(l.n, cmdparts[2], cmdparts[3])
		if !success {
			return "", errors.New("assert get failed")
		}
		return "assert get successful!\n", nil
	}
	fnc, ok := commands[cmd]
	if !ok {
		return "", fmt.Errorf("unrecognized command!")
	} else {
		out, err := fnc(l.n, cmdparts)
		if cmd == "kill" {
			l.n = nil
		}
		return out, err
	}
}

func (l *localNode) GetStatistics() nodeBWInfo {
	out := nodeBWInfo{}
	//bwi, bwo := l.n.Network.BandwidthTotals()
	//out.BwIn = bwi
	//out.BwOut = bwo
	//out.MesRecv, out.MesSend = l.n.Network.GetMessageCounts()
	return out
}

func (l *localNode) Shutdown() {
	if l.n != nil {
		l.n.Close()
		l.n = nil
	}
}

func (l *localNode) PeerID() peer.ID {
	return l.n.Identity
}

// A command func takes a node and a command to run on it
// and returns the output and any error encountered
type CmdFunc func(*core.IpfsNode, []string) (string, error)

var commands map[string]CmdFunc

func init() {
	commands = make(map[string]CmdFunc)
	commands["put"] = Put
	commands["get"] = Get
	commands["findprov"] = FindProv
	commands["store"] = Store
	commands["provide"] = Provide
	commands["diag"] = Diag
	commands["findpeer"] = FindPeer
	commands["bandwidth"] = GetBandwidth
	commands["add"] = AddFile
	commands["readfile"] = ReadFile
	commands["kill"] = KillNode
}

func RunCommand(cmdstr string) bool {
	var async bool
	if cmdstr == "quit" {
		return false
	}
	cmdparts := strings.Split(cmdstr, " ")

	if cmdparts[0] == "go" {
		async = true
		cmdparts = cmdparts[1:]
	}

	if cmdparts[0] == "expect" {
		if !Expect(cmdparts[1:]) {
			// maybe clean up a bit?
			fmt.Println("Expect failed! Halting!")
			os.Exit(-1)
		}
		return true
	}

	if cmdparts[0][0] == '@' {
		// create file
		fname := cmdparts[0][1:]
		switch cmdparts[1] {
		case "make":
			size, err := strconv.Atoi(cmdparts[2])
			if err != nil {
				fmt.Println(err)
				return false
			}
			fi := NewFile(fname, int64(size))
			files[fname] = fi
			fmt.Printf("Created '%s' = '%s'\n", fi.Name, fi.RootKey)
		default:
			fmt.Println("Unrecognized file operation")
			return false
		}
		return true
	}

	if cmdparts[0] == "sleep" {
		dur, err := strconv.Atoi(cmdparts[1])
		if err != nil {
			fmt.Println(err)
			return false
		}
		fmt.Printf("Sleeping for %d seconds.\n", dur)
		time.Sleep(time.Second * time.Duration(dur))
		return true
	}

	idexlist, err := ParseRange(cmdparts[0])
	if err != nil {
		fmt.Println(err)
		return true
	}

	if len(cmdparts) < 2 {
		fmt.Println("must specify command!")
		return true
	}

	if cmdparts[1] == "start" {
		StartNodes(idexlist)
		return true
	}

	if async {
		runCommandsAsync(idexlist, cmdparts, false)
	} else {
		runCommandsSync(idexlist, cmdparts)
	}

	return true
}

func StartNodes(idexlist []int) {
	for _, i := range idexlist {
		if controllers[i] != nil {
			fmt.Printf("ERROR: node %d already started.\n", i)
			continue
		}
		controllers[i] = &localNode{nodeFromConfig(masterCtx, configs[i])}
	}
}

func runCommandsSync(idexlist []int, cmdparts []string) {
	for _, idex := range idexlist {
		if idex >= len(controllers) || idex < 0 {
			fmt.Printf("Index %d out of range!\n", idex)
			return
		}
		if controllers[idex] == nil {
			fmt.Printf("Node %d has already been killed.\n", idex)
		}
		out, err := controllers[idex].RunCommand(cmdparts)
		if !logquiet {
			fmt.Print(out)
		}
		if err != nil {
			fmt.Printf("Error: %s\n", err)
		}
	}
}

func runCommandsAsync(idexlist []int, cmdparts []string, wait bool) {
	done := make(chan struct{})
	for _, i := range idexlist {
		if i >= len(controllers) || i < 0 {
			fmt.Printf("Index %d out of range!\n", i)
			return
		}
		if controllers[i] == nil {
			fmt.Printf("Node %d has already been killed.\n", i)
		}
	}
	for _, idex := range idexlist {
		if controllers[idex] == nil {
			fmt.Println("Attempted to run command on dead node!")
			continue
		}
		go func(i int) {
			out, err := controllers[idex].RunCommand(cmdparts)
			if !logquiet {
				fmt.Print(out)
			}
			if err != nil {
				fmt.Printf("Error: %s\n", err)
			}
			if wait {
				done <- struct{}{}
			}
		}(idex)
	}
	if wait {
		for _ = range idexlist {
			<-done
		}
	}
}

func AssertGet(n *core.IpfsNode, key, exp string) bool {
	ctx, _ := context.WithDeadline(context.TODO(), time.Now().Add(time.Second*5))
	val, err := n.Routing.GetValue(ctx, u.Key(key))
	if err != nil {
		fmt.Printf("Get error: %s\n", err)
		return false
	}

	if string(val) != exp {
		fmt.Printf("expected '%s' but got '%s' instead.\n", exp, string(val))
		return false
	}

	if !logquiet {
		fmt.Println("Expectation Successful!")
	}
	return true
}

func Expect(cmdparts []string) bool {
	idexlist, err := ParseRange(cmdparts[0])
	if err != nil {
		fmt.Println(err)
		return false
	}

	for _, idex := range idexlist {
		if idex >= len(controllers) || idex < 0 {
			fmt.Printf("Index %d out of range!\n", idex)
			return false
		}
		if len(cmdparts) < 2 {
			fmt.Println("must specify command!")
			return false
		}
		cmd := strings.ToLower(cmdparts[1])
		if cmd == "get" {
			cmdparts[1] = "expectget"
		}
		out, err := controllers[idex].RunCommand(cmdparts)
		if !logquiet {
			fmt.Print(out)
		}
		if err != nil {
			fmt.Printf("Error: %s\n", err)
			return false
		}

	}
	return true
}

func Put(n *core.IpfsNode, cmdparts []string) (string, error) {
	if len(cmdparts) < 4 {
		return fmt.Sprintln("put: '# put key val'"), ErrArgCount
	}
	msg := fmt.Sprintf("putting value: '%s' for key '%s'\n", cmdparts[3], cmdparts[2])
	ctx, _ := context.WithDeadline(context.TODO(), time.Now().Add(time.Second*5))
	return msg, n.Routing.PutValue(ctx, u.Key(cmdparts[2]), []byte(cmdparts[3]))
}

func Get(n *core.IpfsNode, cmdparts []string) (string, error) {
	if len(cmdparts) < 3 {
		return fmt.Sprintln("get: '# get key'"), ErrArgCount
	}
	ctx, _ := context.WithDeadline(context.TODO(), time.Now().Add(time.Second*5))
	val, err := n.Routing.GetValue(ctx, u.Key(cmdparts[2]))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Got value: '%s'\n", string(val)), nil
}

func Diag(n *core.IpfsNode, cmdparts []string) (string, error) {
	diag, err := n.Diagnostics.GetDiagnostic(time.Second * 5)
	if err != nil {
		return "", err
	}
	out := new(bytes.Buffer)
	var format string
	if len(cmdparts) > 2 {
		format = cmdparts[2]
	}
	switch {
	case format == "d3":
		b := diagnostics.GetGraphJson(diag)
		if _, err := out.Write(b); err != nil {
			return "", err
		}
		out.Write([]byte("\n"))
	case format == "json":
		enc := json.NewEncoder(out)
		if err := enc.Encode(diag); err != nil {
			return "", err
		}
		out.Write([]byte("\n"))
	default:
		fmt.Fprintln(out, diag)
	}
	return out.String(), nil
}

func Store(n *core.IpfsNode, cmdparts []string) (string, error) {
	if len(cmdparts) < 4 {
		return fmt.Sprintln("store: '# store key val'"), ErrArgCount
	}
	err := n.Datastore.Put(u.Key(cmdparts[2]).DsKey(), []byte(cmdparts[3]))
	if err != nil {
		return "", err
	}

	return "", nil
}

func Provide(n *core.IpfsNode, cmdparts []string) (string, error) {
	if len(cmdparts) < 3 {
		return fmt.Sprintln("provide: '# provide key'"), ErrArgCount
	}
	ctx, _ := context.WithDeadline(context.TODO(), time.Now().Add(time.Second*5))
	err := n.Routing.Provide(ctx, u.Key(cmdparts[2]))
	if err != nil {
		return "", err
	}
	return "", nil
}

func FindProv(n *core.IpfsNode, cmdparts []string) (string, error) {
	if len(cmdparts) < 3 {
		return fmt.Sprintln("findprov: '# findprov key [count]'"), ErrArgCount
	}
	count := 1
	var err error
	if len(cmdparts) >= 4 {
		count, err = strconv.Atoi(cmdparts[3])
		if err != nil {
			return "", err
		}
	}
	ctx, _ := context.WithDeadline(context.TODO(), time.Now().Add(time.Second*5))
	pchan := n.Routing.FindProvidersAsync(ctx, u.Key(cmdparts[2]), count)

	out := new(bytes.Buffer)
	fmt.Fprintf(out, "Providers of '%s'\n", cmdparts[2])
	for p := range pchan {
		fmt.Fprintf(out, "\t%s\n", p)
	}
	return out.String(), nil
}

func ReadFile(n *core.IpfsNode, cmdparts []string) (string, error) {
	if len(cmdparts) < 3 {
		return fmt.Sprintln("readfile: '# add fileref'"), ErrArgCount
	}

	f, ok := files[cmdparts[2]]
	if !ok {
		return fmt.Sprintf("No such file: %s\n", cmdparts[2]), u.ErrNotFound
	}

	if f.RootKey == "" {
		return "", errors.New("file hasnt been added by anyone else")
	}

	start := time.Now()
	nd, err := n.DAG.Get(f.RootKey)
	if err != nil {
		return "", err
	}

	read, err := uio.NewDagReader(nd, n.DAG)
	if err != nil {
		return "", err
	}
	b, err := ioutil.ReadAll(read)
	if err != nil {
		return fmt.Sprintln("Failed to read file."), err
	}
	end := time.Now()
	if !bytes.Equal(b, f.Data) {
		return "", errors.New("File we read doesnt match original bytes")
	}

	took := end.Sub(start)
	bps := float64(len(b)) / took.Seconds()

	trans := transferInfo{}
	trans.Time = took.Nanoseconds()
	trans.Size = len(b)
	trans.Speed = bps
	gslock.Lock()
	globalStats.Transfers = append(globalStats.Transfers, trans)
	gslock.Unlock()

	return fmt.Sprintf("Read File Succeeded: %f bytes per second\n", bps), nil
}

func AddFile(n *core.IpfsNode, cmdparts []string) (string, error) {
	if len(cmdparts) < 3 {
		return fmt.Sprintln("addfile: '# add fileref'"), ErrArgCount
	}

	f, ok := files[cmdparts[2]]
	if !ok {
		return fmt.Sprintf("No such file: %s\n", cmdparts[2]), u.ErrNotFound
	}

	nd, err := imp.BuildDagFromReader(f.GetReader(), n.DAG, n.Pinning.GetManual(), chunk.DefaultSplitter)
	if err != nil {
		return "", err
	}

	if f.RootKey == "" {
		k, err := nd.Key()
		if err != nil {
			return "", err
		}
		f.RootKey = k
	}

	err = n.DAG.AddRecursive(nd)
	if err != nil {
		return "", err
	}
	return "File Added\n", nil
}

func FindPeer(n *core.IpfsNode, cmdparts []string) (string, error) {
	out := new(bytes.Buffer)
	if len(cmdparts) < 3 {
		return fmt.Sprintln("findpeer: '# findpeer peerid'"), ErrArgCount
	}

	var search peer.ID
	if cmdparts[2][0] == '$' {
		n, err := strconv.Atoi(cmdparts[2][1:])
		if err != nil {
			return "", err
		}
		if n >= len(controllers) {
			return "", errors.New("specified peernum out of range")
		}
		search = controllers[n].PeerID()
	} else {
		search = peer.ID(b58.Decode(cmdparts[2]))
	}
	fmt.Fprintf(out, "Searching for peer: %s\n", search)

	ctx, _ := context.WithDeadline(context.TODO(), time.Now().Add(time.Second*5))
	p, err := n.Routing.FindPeer(ctx, search)
	if err != nil {
		return "", err
	}

	fmt.Fprintf(out, "%Got peer: %s\n", p)
	return out.String(), nil
}

func KillNode(n *core.IpfsNode, cmdparts []string) (string, error) {
	n.Close()
	return "Node Killed", nil
}

func GetBandwidth(n *core.IpfsNode, cmdparts []string) (string, error) {
	//in, out := n.Network.BandwidthTotals()
	in, out := -1, -1
	return fmt.Sprintf("Bandwidth totals\n\tIn:  %d\n\tOut: %d\n", in, out), nil
}
