package main

import (
	"bytes"
	"cmp"
	"compress/gzip"
	"compress/lzw"
	"encoding/binary"
	"encoding/gob"
	"fmt"
	"io"
	"os"
	"regexp"
	"slices"
	"time"
)

var needUpdate *time.Ticker

func init() {
	needUpdate = time.NewTicker(time.Second / 10)
}
func update(pattern string, values ...interface{}) {
	select {
	case <-needUpdate.C:
		fmt.Fprintf(os.Stderr, pattern, values...)
	default:
	}
}

type file struct {
	Words        []string
	Count        map[string]int
	CountOrder   []string
	CountLookup  map[string]int
	CountRLookup map[int]string
	Unique       []string
	Lookup       map[string]int
	RLookup      map[int]string
	Locations    map[string][]int
}

func loadFile(name string) (*file, error) {
	meta := &file{}

	fmt.Println("reading precomputed tokens")
	gobs, err := os.Open(name + ".gob")
	if err == nil {
		defer gobs.Close()
		d := gob.NewDecoder(gobs)

		err = d.Decode(&meta)
		if err != nil {
			return nil, err
		}
	} else {
		fmt.Println("failed: ", err)
		fmt.Println("reading file into memory")
		file, err := os.ReadFile(name)
		if err != nil {
			return nil, err
		}

		fmt.Println("searching for words")
		// CHANGES
		// adjust token to allow hyphenated words
		// 467,620 <-before:after-> 313,806
		// 'the' 'of' 'and' are just part of antoher token now
		// maybe 313983 Bytes
		// maybe 311658 Bytes

		// I probably should start pre computing these tokens
		// it takes for ever to generate them
		//re := regexp.MustCompile(`(the |and |in |to |a |is |as )?([\w-]+|[^\w]+)( of)?[ ,.;:]+`)
		re := regexp.MustCompile(`([\w-]+|[^\w]+)[ ]*`)
		chunks := re.FindAll(file, -1)
		fmt.Println("processing chunks")
		meta.Words = []string{""}
		meta.Count = map[string]int{}
		meta.Locations = map[string][]int{}
		for i, v := range chunks {
			token := string(v)
			update("%05.2f converting to strings\r", float32(i)/float32(len(chunks))*100)
			meta.Words = append(meta.Words, token)
			meta.Count[token]++
			meta.Unique = append(meta.Unique, token)
			meta.Locations[token] = append(meta.Locations[token], i+1) // to account for the first ""
		}
		meta.Words = append(meta.Words, "") // just incase we try and look beyond the bounds

		slices.Sort(meta.Unique)
		meta.Unique = slices.Compact(meta.Unique)
		meta.Lookup = map[string]int{}
		meta.RLookup = map[int]string{}
		for i, token := range meta.Unique {
			meta.Lookup[token] = i
			meta.RLookup[i] = token
		}
		meta.CountOrder = meta.Unique[:]
		slices.SortFunc(meta.CountOrder, func(a, b string) int {
			ca := meta.Count[a]
			cb := meta.Count[b]
			// descending order is what we need
			if ca == cb {
				return cmp.Compare(b, a)
			}
			return cmp.Compare(cb, ca)
		})
		meta.CountLookup = map[string]int{}
		meta.CountRLookup = map[int]string{}
		for i, token := range meta.CountOrder {
			meta.CountLookup[token] = i
			meta.CountRLookup[i] = token
		}
		gobs, err = os.Create(name + ".gob")
		if err != nil {
			return nil, err

		}
		defer gobs.Close()
		e := gob.NewEncoder(gobs)

		err = e.Encode(meta)
		if err != nil {
			return nil, err
		}
	}
	return meta, nil
}
func buildBuckets(meta *file) {
	fmt.Println("summary")
	fmt.Println("word count", len(meta.Words))
	fmt.Println("unique word count", len(meta.Unique))
	fmt.Println("buckets")
	// how many of each section there are
	buckets := map[int]int{}
	single := 0
	for i, word := range meta.CountOrder {
		idx := i / 256
		c := meta.Count[word]
		if c == 1 {
			single++
			continue
		}
		buckets[idx] += c
	}
	// how many section encoding will take
	fmt.Println("single", single)
	fmt.Println("avg spacing", len(meta.Words)/single)
	bytes := []byte{}

	fmt.Println("building out bucked data")
	wordBucket := map[int][]byte{}
	singleBucket := []byte{}
	order := []byte{}
	bucketWidth := 256
	maxWord := len(meta.Unique)/bucketWidth + 1
	for _, word := range meta.Words {
		c := meta.Count[word]
		if c <= 1 {
			singleBucket = binary.AppendUvarint(singleBucket, uint64(len(word)))
			singleBucket = append(singleBucket, []byte(word)...)
			order = binary.AppendUvarint(order, uint64(maxWord))
			continue
		}
		id := meta.CountLookup[word]
		idx := id / bucketWidth
		rem := id % bucketWidth
		wordBucket[idx] = append(wordBucket[idx], byte(rem))
		order = binary.AppendUvarint(order, uint64(idx))
	}
	allBytes := []byte{}
	for i := 0; i < len(wordBucket); i++ {
		b := wordBucket[i]
		allBytes = append(allBytes, b...)
	}
	bucketSize := len(allBytes)
	fmt.Println("unique bucket size")
	testCompression(singleBucket)
	fmt.Println("order size")
	testCompression(order)
	fmt.Println("bucket data")
	testCompression(allBytes)
	allBytes = append(allBytes, singleBucket...)
	allBytes = append(allBytes, order...)

	fmt.Println("everything")
	testCompression(allBytes)

	for i := 0; i < 10; i++ {
		fmt.Printf("bucket size %v: %2.2f \n", i, float32(len(wordBucket[i]))/float32(bucketSize))
	}

	for i := 0; i < 100; i++ {
		fmt.Printf("%v", meta.CountRLookup[int(wordBucket[0][i])])
	}
	fmt.Println("\n", order[:100])
	return
	// I need a beter order.
	// I need the order to be according to count, and then the first time it shows up
	fmt.Println("sorting according to amount and order of appearance")
	slices.SortFunc(meta.CountOrder, func(a, b string) int {
		ca := meta.Count[a]
		cb := meta.Count[b]
		// reverse a and b to do decending order
		if ca == cb {
			return cmp.Compare(meta.Locations[b][0], meta.Locations[a][0])
		}
		return cmp.Compare(cb, ca)
	})

	var i int
	for s := 0; s < len(meta.CountOrder)/255; s++ {
		update("%2.2f writing 256 word chunks\r", float32(s)/float32(len(meta.CountOrder))*100)
		for i = 0; i < 256; i++ {
			word := meta.CountOrder[i+255*s]
			if meta.Count[word] == 1 {
				continue
			}
			bytes = binary.AppendUvarint(bytes, uint64(len(word)))
			bytes = append(bytes, []byte(word)...)
		}

		for _, word := range meta.Words {
			id := meta.CountLookup[word]
			if id <= s*255 || id > (s+1)*255 {
				continue
			}
			if meta.Count[word] == 1 {
				continue
			}
			bytes = append(bytes, byte(id))
		}
	}
	fmt.Println("")
	testCompression(bytes)

	fmt.Println("writing out unique words")
	for j := i; j < len(meta.CountOrder); j++ {
		word := meta.CountOrder[j]
		bytes = binary.AppendUvarint(bytes, uint64(len(word)))
		bytes = append(bytes, []byte(word)...)
	}
	// now write out locaion diffs
	fmt.Println("writing out unique locations")
	prev := 0
	for i = 0; i < len(meta.CountOrder); i++ {
		word := meta.CountOrder[i]
		if meta.Count[word] != 1 {
			continue
		}
		if len(meta.Locations[word]) > 1 {
			panic("we should only ever have 1 location at this point")
		}
		loc := meta.Locations[word][0]
		bytes = binary.AppendUvarint(bytes, uint64(prev-loc))
		prev = loc

	}
	fmt.Printf("word buffer %v KB in size\n", len(bytes)/1024)
	fmt.Printf("%2.8f bytes per word\n", float32(len(bytes))/float32(len(meta.Words)))

	testCompression(bytes)
	return
}

func buildRules(meta *file) {
	entries := fromFile(meta)

	fmt.Println("building rules")

	ruleEntries := map[int][]uint64{}
	ruleCount := map[uint64]int{}
	ruleMapping := map[uint64]int{}
	ruleProduces := map[uint64]string{}
	ruleExpects := map[uint64]string{}
	ruleID := 0
	for i, k := range meta.Unique {
		update("%05.2f examining rules\r", float32(i)/float32(len(meta.Unique))*100)
		prediction := entries[k]
		for d, rules := range prediction.rules {
			// we don't care about rules that only match a single thing
			/*
				before
					number of tokens 126816
					number of unique tokens 14536
					found 191188 rules
					found 13512 duplicate rules with 4401 rules
					13 rules / unique token

				after
					number of tokens 126816
					number of unique tokens 14536
					found 177248 rules
					found 13515 duplicate rules with 4410 rules
					12 rules / unique token
			*/
			if d == 0 {
				continue
			}
			for t, rule := range rules {
				ruleEntries[meta.Lookup[t]] = append(ruleEntries[meta.Lookup[t]], uint64(meta.Lookup[rule.Produce]))
				key := uint64(meta.Lookup[t])<<32 | uint64(meta.Lookup[rule.Produce])
				amt := ruleCount[key]
				ruleCount[key]++
				if amt == 0 {
					ruleProduces[key] = rule.Produce
					ruleExpects[key] = t
					ruleMapping[key] = ruleID
					ruleID++
				}
			}
		}
	}

	fmt.Println("finding duplicate rules")
	// find duplicate rules
	ruleDup := 0
	ruleKey := []uint64{}
	for k, v := range ruleCount {
		if v > 1 {
			ruleKey = append(ruleKey, k)
			ruleDup += v
		}
	}

	fmt.Println("sorting duplicate rules")
	slices.Sort(ruleKey)
	fmt.Println("getting unique rules")
	slices.Compact(ruleKey)
	fmt.Println("sorting unique rules")
	slices.SortFunc(ruleKey, func(a, b uint64) int {
		if ruleCount[b] == ruleCount[a] {
			return cmp.Compare(b, a)
		}
		return cmp.Compare(ruleCount[b], ruleCount[a])
	})

	// setup mapping from rule id to sorted index
	ruleMap := map[uint64]int{}
	for k, v := range ruleKey {
		ruleMap[v] = k
	}
	// now we just go through the unique list again and build the buffer
	// probably should build the rule ids at the same time
	sparseRuleBuf := []byte{}
	for i, one := range meta.Unique {
		update("%05.2f encoding rules table\r", float32(i)/float32(len(meta.Unique))*100)
		values := ruleEntries[meta.Lookup[one]]
		slices.Sort(values)
		values = slices.Compact(values)
		prev := uint64(0)
		for _, v := range values {
			// encode a rough order. if this rule is one of 128 that fit in the first byte
			// we set the first bit to be 1, otherwise it is 0
			key := uint64(meta.Lookup[one])<<32 | v
			order := ruleMap[key]
			shifted := (v - prev) << 1
			if order < 128 {
				shifted |= 1
			}
			sparseRuleBuf = binary.AppendUvarint(sparseRuleBuf, shifted)
			prev = v
		}
		sparseRuleBuf = binary.AppendUvarint(sparseRuleBuf, 0)
	}
	fmt.Printf("sparse rule buffer is %v B in size\n", len(sparseRuleBuf))

	buff := []byte{}

	// record rules

	dbuff := map[int][]byte{}
	biggestRules := map[string]int{}
	maxd := 0
	fmt.Println("encoding rules")
	count := 0
	for _, k := range meta.Unique {
		prediction := entries[k]
		// just record the token in a different buffer
		buff = binary.AppendUvarint(buff, uint64(meta.Lookup[k]))
		dep := 0
		for d, rules := range prediction.rules {
			if d > dep {
				dep = d
			}
			if d > maxd {
				maxd = d
			}

			// sort the keys so we can just write differences
			keys := []int{}
			for t := range rules {
				keys = append(keys, meta.Lookup[t])
			}
			slices.Sort(keys)

			allRuleMapping := []uint64{}
			for _, t := range keys {
				expect := meta.RLookup[t]
				aRule := rules[expect]
				if aRule == nil {
					fmt.Println("!!!!", expect, t, aRule)
					allRuleMapping = append(allRuleMapping, uint64(0))
					continue
				}
				id := uint64(t)<<32 | uint64(meta.Lookup[aRule.Produce])
				idx := ruleMap[id]
				//idx := ruleMapping[id]
				allRuleMapping = append(allRuleMapping, uint64(idx))
			}
			slices.Sort(allRuleMapping)
			//allRuleMapping = slices.Compact(allRuleMapping)
			pruleid := uint64(0)
			for i, t := range keys {
				expect := meta.RLookup[t]
				aRule := rules[expect]
				if aRule == nil {
					continue
				}
				biggestRules[k]++
				produce := meta.Lookup[aRule.Produce]
				count++
				l := len(dbuff[d])

				// just write out the token id, we don't care about rules that match a<-a->b
				// they can only match in a single location
				if d == 0 {
					dbuff[d] = binary.AppendUvarint(dbuff[d], uint64(produce))
					continue
				}
				idx := allRuleMapping[i]
				dbuff[d] = binary.AppendUvarint(dbuff[d], idx-pruleid)
				pruleid = idx

				size := len(dbuff[d]) - l
				update("sample token:'%v' depth:'%v' expect:'%v' produce:'%v' locations:%v bytes %v\n", k, d, expect, aRule.Produce, aRule.Locations, size)
			}
			// null terminate the list of rules maybe save %0.1 of the file size
			// only one option is possible at a depth of 0, so no need to null terminate
			if d != 0 {
				dbuff[d] = binary.AppendUvarint(dbuff[d], uint64(0))
			}
		}
		if dep != len(prediction.rules)-1 {
			fmt.Println(dep, len(prediction.rules))
			panic("skipped a rule entry?")
		}
	}
	total := 0
	for i := 0; i <= maxd; i++ {
		b := dbuff[i]
		//fmt.Printf("depth %v maybe %v Bytes\n", i, len(b))
		total += len(b)
	}

	fmt.Println("number of tokens", len(meta.Words))
	fmt.Println("number of unique tokens", len(meta.Unique))
	fmt.Printf("encoded %v rules\n", count)
	fmt.Printf("%.2f bytes / rule\n", float32(total)/float32(count))
	fmt.Printf("found %v duplicate rules with %v rules\n", ruleDup, len(ruleKey))
	fmt.Printf("%v rules / unique token\n", count/len(meta.Unique))
	fmt.Printf("%.2f rules / total token\n", float32(count)/float32(len(meta.Words)))

	fmt.Printf("maybe %v KB and %v KB\n", total/1024, len(sparseRuleBuf)/1024)
	sbuff := []byte{}
	for _, v := range meta.Unique {
		sbuff = append(sbuff, []byte(v)...)
	}
	fmt.Printf("tokens maybe %v KB\n", len(sbuff)/1024)
	fmt.Printf("%.8f bytes / token\n", float32(total+len(sparseRuleBuf)+len(sbuff))/float32(len(meta.Words)))

	fmt.Println("top 10 complex tokens")
	slices.SortFunc(meta.Unique, func(a, b string) int {
		//reversed for decending
		return cmp.Compare(biggestRules[b], biggestRules[a])
	})
	for i := 0; i < 128; i++ {
		fmt.Printf("token %v count:%v rule:%v\n", meta.Unique[i], meta.Count[meta.Unique[i]], biggestRules[meta.Unique[i]])
	}
	for i := 0; i < 128; i++ {
		key := ruleKey[i]
		fmt.Printf("rule %v '%v'=>'%v' is used %v times\n", ruleMap[key], ruleExpects[key], ruleProduces[key], ruleCount[key])
	}
	return
	fmt.Println("regenerated text")
	// lets try and regenerate it!

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

func main() {
	meta, err := loadFile(os.Args[1])
	if err != nil {
		panic(err)
	}
	buildRules(meta)
}
