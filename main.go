package main

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
)

type node struct {
	value     string
	visited   bool
	neighbors map[uint64]*node
}

func main() {
	f, err := os.Open("enwik9")
	if err != nil {
		panic(err)
	}

	br := bufio.NewReader(f)
	all := map[string]*node{}
	first := &node{
		value:     "",
		neighbors: map[uint64]*node{},
	}
	current := first
	amount := 1024 * 1024 * 1024
	count := uint64(0)
	unique := uint64(0)
	length := 4
	for i := 0; i < amount; i++ {
		slice := make([]byte, length)
		_, err := io.ReadFull(br, slice)
		if err != nil && !errors.Is(err, io.EOF) {
			panic(err)
		}
		key := string(slice)
		next, ok := all[key]
		if !ok {
			next = &node{
				value:     key,
				neighbors: map[uint64]*node{},
			}
			all[next.value] = next
			unique++
		}
		_, exists := current.neighbors[count]
		if exists {
			count++
		}
		current.neighbors[count] = next
		fmt.Printf("\rkeys %v overlap:%v processed:%v", len(all), count, i*4)
		current = next
		if err == io.EOF {
			break
		}
	}
	fmt.Println("")
	fmt.Println("summary:")
	fmt.Println("key size: ", len(all)*length)
	fmt.Println("unique: ", unique)
	fmt.Println("sample:")
	i := 0
	for k, v := range all {
		if i == 10 {
			break
		}
		i++
		fmt.Printf("%v: %v\n", k, v)
	}
	fmt.Println("rebuilt:")
	current = first
	count = 0
	for i := 0; i < 100; i++ {
		fmt.Printf("%v", current.value)

		c, ok := current.neighbors[count]
		if !ok {
			break
		}
		current = c
		if current.visited == true {
			for first.visited {
				first.visited = false
				first = first.neighbors[count]
			}
			count++
		}
		current.visited = true
	}
	keys := []string{}
	for k := range all {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	keyf, err := os.OpenFile("keys", os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0755)
	if err != nil {
		panic(err)
	}
	defer keyf.Close()
	valuesf, err := os.OpenFile("values", os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0755)
	if err != nil {
		panic(err)
	}
	defer valuesf.Close()
	pkey := uint32(0)
	for _, k := range keys {
		num := binary.LittleEndian.Uint32([]byte(k))
		buf := make([]byte, 10)
		delta := uint64(num - pkey)
		n := binary.PutUvarint(buf, delta)
		_, err := keyf.Write(buf[:n])
		if err != nil {
			panic(err)
		}
		pnum := uint64(0)
		nkeys := []uint64{}
		// maps don't do smallet to biggest, so we need to sort
		for pos := range all[k].neighbors {
			nkeys = append(nkeys, pos)
		}
		slices.Sort(nkeys)
		for _, pos := range nkeys {
			delta := pos - pnum

			n := binary.PutUvarint(buf, delta)
			_, err := valuesf.Write(buf[:n])
			if err != nil {
				panic(err)
			}
			pnum = pos

		}
		_, err = valuesf.Write([]byte{0, 0})
		if err != nil {
			panic(err)
		}
		pkey = num
	}
}
