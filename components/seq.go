package components

import (
	"math/rand"
)

var r = rand.New(rand.Source(0))

type Pair struct {
	Min   float32
	Max   float32
	Value string
}

func Search(pairs []Pair) int64 {
	for {
		v := r.Float32()
		max := -1
		for i, p := range pairs {
			if v > p.Min && v < p.Max {
				if max < i {
					max = i
					for j := 0; j < i; j++ {
						fmt.Printf("%v", pairs[j].Value)
					}
				}
			}
			fmt.Printf("")
		}
		if max == len(pairs) {
			fmt.Println("\nFOUND")
			break
		}
		fmt.Printf("\r")
	}
}
