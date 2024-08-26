package components

import (
	"cmp"
	"encoding/binary"
	"encoding/gob"
	"encoding/xml"
	"fmt"
	"github.com/fatih/color"
	"html"
	"os"
	"regexp"
	"slices"
	"time"
)

/*
  <page>
    <title>AaA</title>
    <id>1</id>
    <revision>
      <id>32899315</id>
      <timestamp>2005-12-27T18:46:47Z</timestamp>
      <contributor>
        <username>Jsmethers</username>
        <id>614213</id>
      </contributor>
      <text xml:space="preserve">#REDIRECT [[AAA]]</text>
    </revision>
  </page>
*/

func addString(b []byte, s string) []byte {
	if len(s) > 255 {
		panic("too long")
	}
	b = append(b, byte(len(s)))
	return append(b, []byte(s)...)
}

type Info struct {
	Articles map[string]int
	Order    []string
	Words    map[string]int
	Unique   []string
	Lookup   map[string]int
}

func Process(p *Page, i *Info) *Info {
	if i == nil {
		i = &Info{
			Articles: map[string]int{},
			Order:    []string{},
			Words:    map[string]int{},
			Unique:   []string{},
			Lookup:   map[string]int{},
		}
	}
	re := regexp.MustCompile(`([\w]+ ?|[^\w]+ ?)`)
	chunks := re.FindAllString(p.Revision.Text, -1)
	for _, c := range chunks {
		i.Words[c]++
		i.Order = append(i.Order, c)
	}
	for k := range i.Words {
		i.Unique = append(i.Unique, k)
	}
	slices.SortFunc(i.Unique, func(a, b string) int {
		ca := i.Words[a]
		cb := i.Words[b]
		// descending order is what we need
		if ca == cb {
			return cmp.Compare(b, a)
		}
		return cmp.Compare(cb, ca)
	})

	fmt.Printf("%v\t%v\t%.2f\t%v\t%.2f\t%v\n", len(i.Unique), len(p.Revision.Text), float32(len(i.Unique))/float32(len(p.Revision.Text))*100, len(i.Order), float32(len(i.Order))/float32(len(p.Revision.Text))*100, p.Title)
	if p.Title == "Anarchism" {
		fmt.Println(i.Unique)
	}
	return i
}

func Compress(p *Page) []byte {
	buff := []byte{}
	buff = addString(buff, p.Title)
	if p.ID > 255 {
		//panic("id is too big")
	}
	//buff = append(buff, byte(p.ID))
	buff = binary.AppendUvarint(buff, p.ID)
	buff = binary.AppendUvarint(buff, p.Revision.ID)
	buff = binary.AppendUvarint(buff, uint64(p.Revision.Timestamp.Unix())) // just need a uint64
	buff = addString(buff, p.Revision.Contributor.UserName)
	buff = binary.AppendUvarint(buff, p.Revision.Contributor.ID)
	if p.Revision.Text[0] == '#' {
		buff = addString(buff, p.Revision.Text)
	} else {
		unescaped := html.UnescapeString(p.Revision.Text)
		fmt.Println("UNKNOWN\n", unescaped)
		//re := regexp.MustCompile(`([/[]{2}[^\[\]]+]{2})`)
		//chunks := re.FindAllString(unescaped, -1)
		//fmt.Println("chunks", chunks)
	}
	return buff
}

type Contributor struct {
	UserName string `xml:"username"`
	ID       uint64 `xml:"id"`
}

func (c *Contributor) String() string {
	return fmt.Sprintf("contributor:{%v %v}", c.ID, c.UserName)
}

type Revision struct {
	ID          uint64       `xml:"id"`
	Timestamp   *time.Time   `xml:"timestamp"`
	Contributor *Contributor `xml:"contributor"`
	Text        string       `xml:"text"`
}

func (r *Revision) String() string {
	return fmt.Sprintf("rev:{%v %v %v \"%v\"}", r.ID, r.Timestamp, r.Contributor, r.Text)
}

type Page struct {
	Title      string    `xml:"title"`
	ID         uint64    `xml:"id"`
	Revision   *Revision `xml:"revision"`
	Compressed []byte
}

func (p *Page) String() string {
	return fmt.Sprintf("page:{%v %v %v}", p.Title, p.ID, p.Revision)
}

type Wikipedia struct {
	Pages []*Page `xml:"page"`
}

func WikipediaFromFile(file string) (*Wikipedia, error) {
	w := &Wikipedia{}
	fmt.Println("reading precomputed struct")
	gobs, err := os.Open(file + ".gob")
	if err == nil {
		defer gobs.Close()
		d := gob.NewDecoder(gobs)

		err = d.Decode(&w)
		for _, p := range w.Pages {
			p.Revision.Text = html.UnescapeString(p.Revision.Text)
		}
		if err != nil {
			return nil, err
		}
		return w, nil
	}

	fmt.Println("failed: ", err)
	fmt.Println("reading file into memory")
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	err = xml.Unmarshal(data, &w)
	if err != nil {
		return nil, err
	}
	gobs, err = os.Create(file + ".gob")
	if err != nil {
		return nil, err

	}
	defer gobs.Close()
	e := gob.NewEncoder(gobs)

	err = e.Encode(w)
	if err != nil {
		return nil, err
	}
	return w, nil
}
