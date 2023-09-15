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

type node struct {
	value      string
	compressed uint32
	order      uint64
	count      uint64
}

func main() {
	f, err := os.Open("enwik9")
	if err != nil {
		panic(err)
	}
	br := bufio.NewReader(f)
	all := map[string]*node{}
	order := []*node{}
	length := 4
	amount := 1024 / length
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
				value:      key,
				compressed: binary.LittleEndian.Uint32(slice),
				count:      1,
				order:      uint64(i),
			}
			all[next.value] = next
		} else {
			next.count++
		}
		order = append(order, next)
		fmt.Printf("\rkeys %v processed:%v", len(all), i*4)
		if err == io.EOF {
			break
		}
	}
	fmt.Println("")
	fmt.Println("summary:")
	fmt.Println("key size: ", len(all)*length)
	fmt.Println("unique: ", len(all))

	count := 0
	for _, n := range order {
		if n.count == 1 {
			count++
		} else if count != 0 {
			fmt.Println("run of ", count)
			count = 0
		}
	}

	slices.SortFunc(order, func(a, b *node) int {
		if a.count == b.count {
			return cmp.Compare(a.value, b.value)
		}
		return cmp.Compare(a.count, b.count)
	})
	prev := order[0]
	fmt.Println(prev.value, prev.count)
	for _, n := range order {
		if n.value != prev.value {
			fmt.Println(n.value, n.count)
			prev = n
		}
	}
}
