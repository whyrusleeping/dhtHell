package main

import (
	"bytes"
	"io"
	"io/ioutil"

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
	read := io.LimitReader(util.NewTimeSeededRand(), size)
	data, err := ioutil.ReadAll(read)
	if err != nil {
		panic(err)
	}

	return &FileInfo{
		Name: name,
		Data: data,
	}
}
