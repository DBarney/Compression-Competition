package main

import (
	"bufio"
	"cmp"
	"encoding/binary"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"
	"slices"
)

type node struct {
	value      string
	compressed uint32
	order      uint64
	count      uint64
}

/*
  <page>
    <title>AaA</title>
    <id>1</id>
    <revision>
      <id>32899315</id>
      <timestamp>2005-12-27T18:46:47Z</timestamp>
      <contributor>
        <username>Jsmethers</username>        <id>614213</id>
      </contributor>
      <text xml:space="preserve">#REDIRECT [[AAA]]</text>
    </revision>
  </page>
*/
type page struct {
	title string
	id    uint32
	rev_id
	rev_ts
	rev_user
	rev_user_id
	text []byte
}

func (p *page) String() string {
	return "page"
}

func parsePage(x *xml.Decoder) *page {
	p := &page{}
	for {
		t, err := x.Token()
		if err != nil {
			panic(err)
		}
		switch elem := t.(type) {
		case xml.EndElement:
		case xml.CharData:
		case xml.StartElement:
			v, err := x.Token()
			if err != nil {
				panic(err)
			}
			switch elem.Name.Local {
			case "title":
				p.title = string(v)
			case "id":
				p.title = strconv.Atoi(string(v))
			case "revision":
				parseRevision(x, p)
			case "text":
				p.text = v
			}
		}
	}
	return p
}

func parseRevision(x *xml.Decoder, p *page) {
	for {
		t, err := x.Token()
		if err != nil {
			panic(err)
		}
		switch elem := t.(type) {
		case xml.EndElement:
		case xml.CharData:
		case xml.StartElement:
	
}
func main() {
	f, err := os.Open("enwik9")
	if err != nil {
		panic(err)
	}
	x := xml.NewDecoder(f)

	for i := 0; i < 1000; i++ {
		t, err := x.Token()
		if err != nil {
			panic(err)
		}
		switch elem := t.(type) {
		case xml.EndElement:
		case xml.CharData:
		case xml.StartElement:
			if elem.Name.Local == "page" {
				p := parsePage(x)
				fmt.Println(p)
				return
			}
		default:
			fmt.Println(reflect.TypeOf(t))
		}
	}
	return
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
