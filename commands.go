package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"code.google.com/p/go.net/context"
	cmds "github.com/jbenet/go-ipfs/core/commands"
	"github.com/jbenet/go-ipfs/peer"
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
}

func RunCommand(cmdstr string) bool {
	if cmdstr == "quit" {
		return false
	}
	cmdparts := strings.Split(cmdstr, " ")
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

func FindPeer(idex int, cmdparts []string) error {
	if len(cmdparts) < 3 {
		fmt.Println("findpeer: '# findpeer peerid'")
		return ErrArgCount
	}

	ctx, _ := context.WithDeadline(context.TODO(), time.Now().Add(time.Second*5))
	p, err := nodes[idex].Routing.FindPeer(ctx, peer.ID(cmdparts[2]))
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
