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
	f, err := os.Open(os.Args[1])
	if err != nil {
		panic(err)
	}
	br := bufio.NewReader(f)
	all := []uint64{}
	mapping := map[string][]string{}
	length := 4
	prev := make([]byte, length)
	i := 0
	for {
		current := make([]byte, length)
		_, err := io.ReadFull(br, current)
		if err != nil && !errors.Is(err, io.EOF) {
			panic(err)
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
	l := len(all)
	all = slices.Compact(all)
	fmt.Println("duplicates removed:", l-len(all))
	fmt.Println("min duplicate size:", (l-len(all))/1024/1024*8, "MB")
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

	fmt.Println("distribution")
	buf = []byte{}
	for i := 0; i <= max; i++ {
		v := dist[i]
		if v == 0 {
			continue
		}
	}
	fmt.Println("building skip list")

	_, err = f.Seek(0, 0)
	if err != nil {
		panic(err)
	}
	br = bufio.NewReader(f)
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
	freq := map[uint64]uint64{}
	// I tried compressing runs of numbers 0,0,0,0 into 0,4 but it made the index bigger
	for _, v := range path {
		freq[v]++
		buf = binary.AppendUvarint(buf, v)
	}

	for i := 0; i < 10; i++ {
		fmt.Printf("depth: %v = %v => %.2f%%\n", i, freq[uint64(i)], float64(freq[uint64(i)])/float64(len(path))*100)
	}
	fmt.Println("")
	fmt.Println("index element count", len(path))
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
}
