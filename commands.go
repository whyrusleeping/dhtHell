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
	cmds "github.com/jbenet/go-ipfs/core/commands"
	imp "github.com/jbenet/go-ipfs/importer"
	"github.com/jbenet/go-ipfs/peer"
	uio "github.com/jbenet/go-ipfs/unixfs/io"
	u "github.com/jbenet/go-ipfs/util"
)

type CmdFunc func(int, []string) error

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
}

func RunCommand(cmdstr string) bool {
	if cmdstr == "quit" {
		return false
	}
	cmdparts := strings.Split(cmdstr, " ")
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

	for _, idex := range idexlist {
		if idex >= len(nodes) || idex < 0 {
			fmt.Printf("Index %d out of range!\n", idex)
			return true
		}
		if len(cmdparts) < 2 {
			fmt.Println("must specify command!")
			return true
		}
		cmd := strings.ToLower(cmdparts[1])
		fnc, ok := commands[cmd]
		if !ok {
			fmt.Println("unrecognized command!")
		} else {
			err := fnc(idex, cmdparts)
			if err != nil {
				fmt.Printf("Error: %s\n", err)
			}
		}

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
		if idex >= len(nodes) || idex < 0 {
			fmt.Printf("Index %d out of range!\n", idex)
			return false
		}
		if len(cmdparts) < 2 {
			fmt.Println("must specify command!")
			return false
		}
		cmd := strings.ToLower(cmdparts[1])
		switch cmd {
		case "get":
			if len(cmdparts) < 4 {
				fmt.Println("Invalid args to expect get!")
				return false
			}
			return AssertGet(idex, cmdparts[2], cmdparts[3])
		default:
			fnc, ok := commands[cmd]
			if !ok {
				fmt.Println("unrecognized command!")
			} else {
				err := fnc(idex, cmdparts)
				if err != nil {
					fmt.Printf("Error: %s\n", err)
					return false
				}
			}
		}

	}
	return true
}

func Put(idex int, cmdparts []string) error {
	if len(cmdparts) < 4 {
		fmt.Println("put: '# put key val'")
		return ErrArgCount
	}
	fmt.Printf("putting value: '%s' for key '%s'\n", cmdparts[3], cmdparts[2])
	ctx, _ := context.WithDeadline(context.TODO(), time.Now().Add(time.Second*5))
	return nodes[idex].Routing.PutValue(ctx, u.Key(cmdparts[2]), []byte(cmdparts[3]))
}

func Get(idex int, cmdparts []string) error {
	if len(cmdparts) < 3 {
		fmt.Println("get: '# get key'")
		return ErrArgCount
	}
	ctx, _ := context.WithDeadline(context.TODO(), time.Now().Add(time.Second*5))
	val, err := nodes[idex].Routing.GetValue(ctx, u.Key(cmdparts[2]))
	if err != nil {
		return err
	}
	fmt.Printf("%d) Got value: '%s'\n", idex, string(val))
	return nil
}

func AssertGet(idex int, key, exp string) bool {
	ctx, _ := context.WithDeadline(context.TODO(), time.Now().Add(time.Second*5))
	val, err := nodes[idex].Routing.GetValue(ctx, u.Key(key))
	if err != nil {
		fmt.Printf("Get error: %s\n", err)
		return false
	}

	if string(val) != exp {
		fmt.Printf("expected '%s' but got '%s' instead.\n", exp, string(val))
		return false
	}

	fmt.Println("Expectation Successful!")
	return true
}

func Diag(idex int, cmdparts []string) error {
	diag, err := nodes[idex].Diagnostics.GetDiagnostic(time.Second * 5)
	if err != nil {
		return err
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
			return err
		}
	} else {
		cmds.PrintDiagnostics(diag, os.Stdout)
	}
	return nil
}

func Store(idex int, cmdparts []string) error {
	if len(cmdparts) < 4 {
		fmt.Println("store: '# store key val'")
		return ErrArgCount
	}
	err := nodes[idex].Datastore.Put(u.Key(cmdparts[2]).DsKey(), []byte(cmdparts[3]))
	if err != nil {
		return err
	}

	return nil
}

func Provide(idex int, cmdparts []string) error {
	if len(cmdparts) < 3 {
		fmt.Println("provide: '# provide key'")
		return ErrArgCount
	}
	ctx, _ := context.WithDeadline(context.TODO(), time.Now().Add(time.Second*5))
	err := nodes[idex].Routing.Provide(ctx, u.Key(cmdparts[2]))
	if err != nil {
		return err
	}
	return nil
}

func FindProv(idex int, cmdparts []string) error {
	if len(cmdparts) < 3 {
		fmt.Println("findprov: '# findprov key [count]'")
		return ErrArgCount
	}
	count := 1
	var err error
	if len(cmdparts) >= 4 {
		count, err = strconv.Atoi(cmdparts[3])
		if err != nil {
			return err
		}
	}
	ctx, _ := context.WithDeadline(context.TODO(), time.Now().Add(time.Second*5))
	pchan := nodes[idex].Routing.FindProvidersAsync(ctx, u.Key(cmdparts[2]), count)
	fmt.Printf("%d) Providers of '%s'\n", idex, cmdparts[2])
	for p := range pchan {
		fmt.Printf("\t%s\n", p)
	}
	return nil
}

func ReadFile(idex int, cmdparts []string) error {
	if len(cmdparts) < 3 {
		fmt.Println("readfile: '# add fileref'")
		return ErrArgCount
	}

	f, ok := files[cmdparts[2]]
	if !ok {
		fmt.Printf("No such file: %s\n", cmdparts[2])
		return u.ErrNotFound
	}

	nd, err := nodes[idex].DAG.Get(f.RootKey)
	if err != nil {
		return err
	}

	read, err := uio.NewDagReader(nd, nodes[idex].DAG)
	if err != nil {
		return err
	}
	b, err := ioutil.ReadAll(read)
	if err != nil {
		fmt.Println("Failed to read file.")
		return err
	}
	if !bytes.Equal(b, f.Data) {
		return errors.New("File we read doesnt match original bytes")
	}

	fmt.Println("Read File Succeeded!")
	return nil
}

func AddFile(idex int, cmdparts []string) error {
	if len(cmdparts) < 3 {
		fmt.Println("addfile: '# add fileref'")
		return ErrArgCount
	}

	f, ok := files[cmdparts[2]]
	if !ok {
		fmt.Printf("No such file: %s\n", cmdparts[2])
		return u.ErrNotFound
	}

	nd, err := imp.NewDagFromReader(f.GetReader())
	if err != nil {
		return err
	}

	err = nodes[idex].DAG.AddRecursive(nd)
	if err != nil {
		return err
	}
	return nil
}

func FindPeer(idex int, cmdparts []string) error {
	if len(cmdparts) < 3 {
		fmt.Println("findpeer: '# findpeer peerid'")
		return ErrArgCount
	}

	var search peer.ID
	if cmdparts[2][0] == '$' {
		n, err := strconv.Atoi(cmdparts[2][1:])
		if err != nil {
			return err
		}
		if n >= len(nodes) {
			return errors.New("specified peernum out of range")
		}
		search = nodes[n].Identity.ID()
	} else {
		search = peer.ID(b58.Decode(cmdparts[2]))
	}
	fmt.Printf("Searching for peer: %s\n", search)

	ctx, _ := context.WithDeadline(context.TODO(), time.Now().Add(time.Second*5))
	p, err := nodes[idex].Routing.FindPeer(ctx, search)
	if err != nil {
		return err
	}

	fmt.Printf("%d) Got peer: %s\n", idex, p)
	return nil
}

func GetBandwidth(idex int, cmdparts []string) error {
	in, out := nodes[idex].Network.GetBandwidthTotals()
	fmt.Printf("Bandwidth totals\n\tIn:  %d\n\tOut: %d\n", in, out)
	return nil
}
