package main

import (
	"bytes"
	"cmp"
	"compress/gzip"
	"compress/lzw"
	"fmt"
	"io"
	"os"
	"regexp"
	"slices"
)

type rule struct {
	Expected string
	Location int
	Produce  string
}

type entry struct {
	nextTokens map[string]map[int][]string
}

type prediction struct {
	rules map[int][]*rule
}

func (p *prediction) addToken(prev []string, produce string) bool {
	loc := len(prev) - 1
	i := 0
	for {
		check := prev[loc-i]
		rules, found := p.rules[i] // check for rules that match at current depth
		fmt.Printf("checking at depth %v, found %v rules\n", i, len(rules))
		if !found {
			// we can add our rule in
			p.rules[i] = append(p.rules[i], &rule{
				Location: loc,
				Expected: check,
				Produce:  produce,
			})
			return true
		}
		foundPartial := false
		for ruleidx, r := range rules {
			fmt.Printf("'%v'=>'%v' where\n\trule identifier='%v' and back %v locations.\n\tand we are looking for '%v'\n", r.Expected, produce, prev[r.Location-i], i, check)
			if prev[r.Location-i] == r.Expected && r.Produce == produce {
				fmt.Println("we already have a match")
				return false // we already have a match
			}
			// need to see if we partially match another rule
			partialMatch := true
			for j := 0; j <= i; j++ {
				fmt.Printf("checking %v %v\n", prev[r.Location-j], prev[loc-j])
				if prev[r.Location-j] != prev[loc-j] {
					partialMatch = false
				}
			}
			if partialMatch {
				// bump the found rule further back
				if r.Location-i-1 < 0 {
					fmt.Println("can't look back further")
					foundPartial = true
					break
				}

				// how to do this correctly....
				// mutating an array AND map I am iterating
				p.rules[i] = append(rules[:ruleidx], rules[ruleidx+1:]...)
				r.Expected = prev[r.Location-i-1]
				p.rules[i+1] = append(p.rules[i+1], r)
				fmt.Println("shifting existing rule")
				foundPartial = true
			}
		}
		if !foundPartial && len(rules) != 0 {
			fmt.Println("we do not have a partial match")
			p.rules[i] = append(p.rules[i], &rule{
				Location: loc,
				Expected: check,
				Produce:  produce,
			})
			return true
		}
		i++
	}
	panic("should never run")
}

func main() {
	fmt.Println("reading file into memory")
	file, err := os.ReadFile(os.Args[1])
	if err != nil {
		panic(err)
	}
	//testCompression(file)

	fmt.Println("searching for words")
	re := regexp.MustCompile(`[\w]+`)
	words := re.FindAll(file, -1)
	fmt.Println("counting duplicates")
	counts := map[string]int{}
	swords := []string{}
	prev := ""
	entries := map[string]*entry{
		"": &entry{
			nextTokens: map[string]map[int][]string{},
		},
	}
	for i, v := range words {
		if i == 30 {
			//return
		}
		token := string(v)
		ent, found := entries[prev]
		if !found {
			ent = &entry{
				nextTokens: map[string]map[int][]string{},
			}
			entries[prev] = ent
		}
		locations, found := ent.nextTokens[token]
		if len(ent.nextTokens) > 0 && !found {
			panic("need to adjust")
		} else if found {
			fmt.Println("checking token prediciton", prev, token)
			// we need to see if we already have a match

			match := false
			for location, ts := range locations {
				fmt.Println(swords[location-len(ts)], swords[i-len(ts)])
				if swords[location-len(ts)] == swords[i-len(ts)] {
					match = true
				}
			}

			if match {
				fmt.Println("token already encoded")
			} else {
				panic("token needs to be adjusted")
			}
		} else {
			locations = map[int][]string{
				i: []string{prev},
			}

			ent.nextTokens[token] = locations
		}

		fmt.Printf("processing words '%v' %v/%v \n", token, i, len(words))
		//counts[token]++
		swords = append(swords, token)
		prev = token
	}
	fmt.Println("")

	return
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
	if false {
		for i := 0; i < 30; i++ {
			fmt.Printf("sorted sample: %v %v %v\n", swords[i], counts[swords[i]], []byte(swords[i]))
		}
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
