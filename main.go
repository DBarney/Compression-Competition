package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
)

type node struct {
	value     string
	neighbors map[uint64]*node
}

func main() {
	f, err := os.Open("enwik9")
	if err != nil {
		panic(err)
	}

	br := bufio.NewReader(f)

	// infinite loop
	prev := int(0)
	count := 0
	amount := 100
	for i := 0; i < amount; i++ {

		b, err := br.ReadByte()

		if err != nil && !errors.Is(err, io.EOF) {
			fmt.Println(err)
			break
		}

		delta := int(b) - prev
		fmt.Println(string([]byte{b}), b, prev, delta)
		prev = int(b)
		count += bitcount(delta)

		if err != nil {
			// end of file
			break
		}
	}
	fmt.Println("original bytes: ", amount)
	fmt.Println("target bytes:", amount/10)
	fmt.Println("compressed bytes: ", count/8)
}

func bitcount(value int) int {
	v := math.Abs(float64(value))
	return 1
	if v == 0 {
		return 1
	} else if v == 1 {
		return 2
	} else if v <= 2 {
		return 3
	} else if v <= 4 {
		return 4
	} else if v <= 8 {
		return 5
	} else if v <= 16 {
		return 6
	} else if v <= 32 {
		return 6
	} else if v <= 64 {
		return 6
	} else if v <= 128 {
		return 6
	}
	return 7
}
