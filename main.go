package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
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
	length := 5
	for i := 0; i < amount; i++ {
		key, err := br.ReadString(' ')
		if err != nil && !errors.Is(err, io.EOF) {
			panic(err)
		}
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
}
