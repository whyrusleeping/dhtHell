package main

import (
	"bytes"
	"io"
	"io/ioutil"

	imp "github.com/jbenet/go-ipfs/importer"
	uio "github.com/jbenet/go-ipfs/unixfs/io"
	"github.com/jbenet/go-ipfs/util"
)

var files map[string]*FileInfo

func init() {
	files = make(map[string]*FileInfo)
}

type FileInfo struct {
	Name    string
	Data    []byte
	RootKey util.Key
}

func (fi *FileInfo) GetReader() io.Reader {
	return bytes.NewReader(fi.Data)
}

func NewFile(name string, size int64) *FileInfo {
	read := io.LimitReader(util.NewFastRand(), size)
	nd, err := imp.NewDagFromReader(read)
	if err != nil {
		panic(err)
	}
	dread, err := uio.NewDagReader(nd, nil)
	if err != nil {
		panic(err)
	}
	data, err := ioutil.ReadAll(dread)
	if err != nil {
		panic(err)
	}
	k, err := nd.Key()
	if err != nil {
		panic(err)
	}

	return &FileInfo{
		Name:    name,
		Data:    data,
		RootKey: k,
	}
}
