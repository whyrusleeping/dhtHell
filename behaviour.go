package main

/*
import (
	"fmt"
	"io"
	"math/rand"
	"time"

	imp "github.com/jbenet/go-ipfs/importer"
	mdag "github.com/jbenet/go-ipfs/merkledag"
	util "github.com/jbenet/go-ipfs/util"
)

type milestone struct {
	interest float64
	point    time.Duration
}

type interestCurve struct {
	milestones []milestone
}

func (ic *interestCurve) getCurrentInterest(offset time.Duration) float64 {
	previ := float64(0)
	for _, m := range ic.milestones {
		if offset < m.point {
			delta := float64(offset) / float64(m.point)
			incr := m.interest - previ
			return (previ + (incr * delta)) / 100
		}
		previ = m.interest
		offset -= m.point
	}
	return previ
}

type Behaviour struct {
	Type int

	// Multiplier for interest curve
	Magnitude float64

	// ....
	Lifespan time.Duration

	// interest curve, describing interest in this object over time
	curve *interestCurve

	// description of the content
	Key     util.Key
	ValSize int64

	// Attributes about the distribution of values in the data
	// affects compressability of the data
	DataType int

	// Node who first adds this content to the network
	Initiator int

	involvedNodes map[int]struct{}
	unaddedNodes  map[int]struct{}
}

func (b *Behaviour) Start() {
	for i, _ := range nodes {
		if i != b.Initiator {
			b.unaddedNodes[i] = struct{}{}
		}
	}
	b.involvedNodes[b.Initiator] = struct{}{}
	nd := b.genData()
	err := nodes[b.Initiator].DAG.AddRecursive(nd)
	if err != nil {
		panic(err)
	}

	tick := time.NewTicker(time.Millisecond * 50)
	begin := time.Now()
	for _ = range tick.C {
		inter := b.curve.getCurrentInterest(time.Now().Sub(begin))
		req := int(inter * float64(len(nodes)))
		if req > len(b.involvedNodes) {
			b.AddNode()
		} else if req < len(b.involvedNodes) {
			fmt.Printf("invnodes = %d, req = %d\n", len(b.involvedNodes), req)
			if len(b.involvedNodes) > 1 {
				b.RemoveNode()
			}
		}

		if time.Now().After(begin.Add(b.Lifespan)) {
			fmt.Println("Behaviour over!")
			return
		}
	}
}


func (b *Behaviour) RemoveNode() {
	fmt.Println("Removing node from behaviour!")
	i := rand.Intn(len(b.involvedNodes))
	var which int
	for k, _ := range b.unaddedNodes {
		if i == 0 {
			which = k
			break
		}
		i--
	}

	delete(b.involvedNodes, which)
	b.unaddedNodes[which] = struct{}{}
	go func(i int) {
			nd, err := nodes[i].DAG.Get(b.Key)
			if err != nil {
				fmt.Printf("Remove Node Error: %s\n", err)
				return
			}
			err = nodes[i].DAG.Remove(nd)
			if err != nil {
				fmt.Printf("Remove Node Error: %s\n", err)
				return
			}
	}(which)
}

func (b *Behaviour) AddNode() {
	fmt.Println("Adding node to behaviour!")
	if len(b.unaddedNodes) == 0 {
		fmt.Println("Tried to add more nodes than exist...")
		return
	}
	i := rand.Intn(len(b.unaddedNodes))
	var which int
	for k, _ := range b.unaddedNodes {
		if i == 0 {
			which = k
			break
		}
		i--
	}
	delete(b.unaddedNodes, which)
	b.involvedNodes[which] = struct{}{}
	go func(i int) {
		nd, err := nodes[i].DAG.Get(b.Key)
		if err != nil {
			fmt.Printf("DAG GET ERROR: %s\n", err)
			return
		}
		_ = nd
		fmt.Printf("Got node!")
	}(which)
}

func (b *Behaviour) genData() *mdag.Node {
	read := io.LimitReader(util.NewTimeSeededRand(), b.ValSize)
	nd, err := imp.NewDagFromReader(read)
	if err != nil {
		panic(err)
	}
	return nd
}
*/
