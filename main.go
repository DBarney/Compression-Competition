package main

import (
	"bufio"
	"cmp"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
)

type skip struct {
	count uint32
	next  byte
}

type node struct {
	value   uint32
	next    map[byte]uint32
	ordered []byte
}

func main() {
	f, err := os.Open(os.Args[1])
	if err != nil {
		panic(err)
	}
	br := bufio.NewReader(f)
	all := map[uint32]*node{}
	length := 4
	slice := make([]byte, length)
	_, err = io.ReadFull(br, slice)
	if err != nil {
		panic(err)
	}
	key := binary.BigEndian.Uint32(slice)
	current := &node{
		next: map[byte]uint32{},
	}
	all[key] = current
	first := key
	i := 0
	for {
		b, err := br.ReadByte()
		if err != nil && !errors.Is(err, io.EOF) {
			panic(err)
		}
		current.next[b]++
		slice = append(slice[1:], b)
		key := binary.BigEndian.Uint32(slice)
		next, ok := all[key]
		if !ok {
			next = &node{
				next: map[byte]uint32{},
			}
			all[key] = next
		}
		current = next
		if i%1000 == 0 {
			fmt.Printf("\rkeys %v processed:%v", len(all), i)
		}
		i++
		if err == io.EOF {
			break
		}
	}

	fmt.Println("")
	fmt.Println("ordering bytes")
	for _, node := range all {
		ord := []byte{}
		for k := range node.next {
			ord = append(ord, k)
		}
		// swap args, we want decending order
		slices.SortFunc(ord, func(a, b byte) int {
			if node.next[b] == node.next[a] {
				return cmp.Compare(b, a)
			}
			return cmp.Compare(node.next[b], node.next[a])
		})
		node.ordered = ord
	}
	fmt.Println("building diff")
	_, err = f.Seek(4, 0)
	if err != nil {
		panic(err)
	}
	br = bufio.NewReader(f)
	buf := []byte{0, 0, 0, 0}
	binary.BigEndian.PutUint32(buf, first)
	count := uint32(0)
	skips := []*skip{}
	for {
		b, err := br.ReadByte()
		if err != nil && !errors.Is(err, io.EOF) {
			panic(err)
		}
		key := binary.BigEndian.Uint32(buf)
		node := all[key]
		if len(node.ordered) == 0 {
			break
		}
		buf = append(buf[1:], b)
		if node.ordered[0] == b {
			count++
		} else {
			skips = append(skips, &skip{
				count: count,
				next:  b,
			})
			count = 0
		}
		if err != nil {
			break
		}
	}
	fmt.Println("")

	fmt.Println("summary:")
	fmt.Println("key size: ", len(all)*length)
	fmt.Println("unique: ", len(all))
	/*
		fmt.Println("example:")

		buf = []byte{0, 0, 0, 0}
		binary.BigEndian.PutUint32(buf, first)
		fmt.Printf("%v", string(buf))
		for i = 0; i < 100; i++ {
			key := binary.BigEndian.Uint32(buf)
			node := all[key]
			b := node.ordered[0]
			buf = append(buf[1:], b)
			fmt.Printf("%v", string([]byte{b}))
		}
	*/
	kf, err := os.OpenFile(os.Args[1]+".shrink", os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0755)
	if err != nil {
		panic(err)
	}
	ordered := []uint32{}
	for k := range all {
		ordered = append(ordered, k)
	}
	slices.Sort(ordered)
	prev := uint32(0)
	for _, key := range ordered {
		node := all[key]
		buf := []byte{0, 0, 0, 0, 0, 0, 0, 0}
		out := []byte{}
		n := binary.PutUvarint(buf, uint64(key-prev))
		out = append(out, buf[:n]...)
		n = binary.PutUvarint(buf, uint64(len(node.next)))
		out = append(out, buf[:n]...)
		prev = key
		for c := range node.next {
			out = append(out, c)
			// no uvarint as it makes it bigger
		}
		_, err = kf.Write(buf)
		if err != nil {
			panic(err)
		}
	}
	_, err = kf.Write([]byte{0, 0, 0, 0})
	if err != nil {
		panic(err)
	}
	out := []byte{}
	for _, skip := range skips {
		binary.AppendUvarint(out, uint64(skip.count))
		out = append(out, skip.next)
	}
	fmt.Println("skiplist size: ", len(out))
	_, err = kf.Write(out)
	if err != nil {
		panic(err)
	}
}
