package main

import (
	"bytes"
	"cmp"
	"compress/gzip"
	"compress/lzw"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"regexp"
	"slices"
)

func main() {
	fmt.Println("reading file into memory")
	file, err := os.ReadFile(os.Args[1])
	if err != nil {
		panic(err)
	}
	//testCompression(file)

	fmt.Println("searching for words")
	re := regexp.MustCompile(`([\w]+|[^\w]+)[ ,-.;']+?`)
	words := re.FindAll(file, -1)
	fmt.Println("counting duplicates")
	counts := map[string]int{}
	swords := []string{}
	for _, v := range words {
		counts[string(v)]++
		swords = append(swords, string(v))
	}
	fmt.Println("sorting")
	slices.Sort(swords)
	fmt.Println("removing duplicates")
	swords = slices.Compact(swords)
	fmt.Println("unique words")
	fmt.Println(len(swords))
	fmt.Println("sorting according to weight")
	slices.SortFunc(swords, func(a, b string) int {
		if counts[a] == counts[b] {
			return cmp.Compare(b, a)
		}
		return cmp.Compare(counts[b], counts[a]) //reversed
	})
	for i := 0; i < 20; i++ {
		fmt.Printf("sorted sample: %v %v %v\n", swords[i], counts[swords[i]], []byte(swords[i]))
	}
	posMap := map[string]int{}
	for i, w := range swords {
		posMap[w] = i
	}
	size := 0
	for _, word := range swords {
		size += len(word) + 1 // one extra for length encoding
		fmt.Printf("word list size %v KB\r", size/1024)
	}
	fmt.Println("")
	longest := 0
	for _, w := range words {
		l := len(w)
		if l > longest {
			longest = l
			fmt.Printf("longest word %v\r", longest)
		}
	}
	fmt.Println("")
	buf := []byte{}
	for _, word := range words {
		pos, found := posMap[string(word)]
		if !found {
			panic("word was not found")
		}
		buf = binary.AppendUvarint(buf, uint64(pos))
	}
	fmt.Println("document list length ", len(buf)/1024, "KB")

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
