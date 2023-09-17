package main

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"compress/lzw"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
)

func main() {
	fmt.Println("reading file into memory")
	file, err := os.ReadFile(os.Args[1])
	testCompression(file)
	fileBuf := bytes.NewBuffer(file)
	if err != nil {
		panic(err)
	}

	br := bufio.NewReader(fileBuf)
	all := []uint64{}
	mapping := map[string][]string{}
	length := 4
	prev := make([]byte, length)
	i := 0
	chars := map[byte]int{}
	for {
		current := make([]byte, length)
		_, err := io.ReadFull(br, current)
		if err != nil && !errors.Is(err, io.EOF) {
			panic(err)
		}
		for _, c := range current {
			chars[c]++
		}
		key := binary.BigEndian.Uint64(append(prev, current...))
		mapping[string(prev)] = append(mapping[string(prev)], string(current))
		all = append(all, key)
		prev = current
		if i%100000 == 0 {
			fmt.Printf("\rkeys %v processed:%v", len(all), i)
		}
		i++
		if err == io.EOF {
			break
		}
	}
	fmt.Println("")

	c := byte(0)
	count := 0
	for k, v := range chars {
		if v > count {
			count = v
			c = k
		}
	}
	fmt.Printf("most common character: %v '%v'\n", c, string([]byte{c}))

	fmt.Println("building tokens")
	fileBuf = bytes.NewBuffer(file)
	br = bufio.NewReader(fileBuf)
	ptoken := []byte{}
	allTokens := []string{}
	for {
		token, err := br.ReadBytes(c)
		if err != nil && !errors.Is(err, io.EOF) {
			panic(err)
		}
		allTokens = append(allTokens, string(append(ptoken, token...)))
		ptoken = token
		if err == io.EOF {
			break
		}
	}

	fmt.Println(len(allTokens))
	slices.Sort(allTokens)
	allTokens = slices.Compact(allTokens)
	fmt.Println(len(allTokens))

	buf := []byte{}
	for k, vs := range mapping {
		buf = append(buf, []byte(k)...)
		for _, v := range vs {

			buf = append(buf, []byte(v)...)
		}
	}
	fmt.Println("compressing sorted file")
	testCompression(buf)

	fmt.Println("custom compression")
	slices.Sort(all)
	all = slices.Compact(all)
	nprev := uint64(0)
	buf = []byte{}
	delta := uint64(0)
	for i := 0; i < len(all); i++ {
		val := all[i]
		ddelta := (val - nprev) - delta
		delta = val - nprev
		buf = binary.AppendUvarint(buf, ddelta)
	}
	testCompression(buf)
	fmt.Println("removing duplicate mapping entries")
	for k, v := range mapping {
		slices.Sort(v)
		slices.Compact(v)
		mapping[k] = v
	}

	dist := map[int]int{}
	max := 0
	for _, v := range mapping {
		c := len(v)
		dist[c]++
		if max < c {
			max = c
		}
	}

	buf = []byte{}
	for i := 0; i <= max; i++ {
		v := dist[i]
		if v == 0 {
			continue
		}
	}
	fmt.Println("building skip list")

	fileBuf = bytes.NewBuffer(file)
	br = bufio.NewReader(fileBuf)
	prev = make([]byte, length)
	path := []uint64{}
	i = 0
	for {
		current := make([]byte, length)
		_, err := io.ReadFull(br, current)
		if err != nil && !errors.Is(err, io.EOF) {
			panic(err)
		}
		key := string(current)
		next := mapping[string(prev)]
		prev = current
		pos, found := slices.BinarySearch(next, key)
		if !found {
			fmt.Println(len(key), len(path), current)
			panic("unable to find data??")
		}
		path = append(path, uint64(pos))
		// now we need to find the index

		if i%100000 == 0 {
			fmt.Printf("\rprocessed:%v", i)
		}
		i++
		if err == io.EOF {
			break
		}
	}
	fmt.Println("")

	buf = []byte{}
	// I tried compressing runs of numbers 0,0,0,0 into 0,4 but it made the index bigger
	for _, v := range path {
		buf = binary.AppendUvarint(buf, v)
	}

	testCompression(buf)
}
func testCompression(buf []byte) {
	fmt.Printf("normal size: %v KB\n", len(buf)/1024)

	res := &bytes.Buffer{}
	gw, err := gzip.NewWriterLevel(res, gzip.BestCompression)
	if err != nil {
		panic(err)
	}
	compress := map[string]io.WriteCloser{
		"lzw":  lzw.NewWriter(res, lzw.MSB, 8),
		"gzip": gw,
	}
	for name, w := range compress {
		res.Reset()
		w.Write(buf)
		w.Close()
		fmt.Printf("%v size: %v KB\n", name, res.Len()/1024)
	}
	fmt.Println("")
}
