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
	"sync"
	"time"
)

type rule struct {
	Locations []int
	Produce   string
}

func (r rule) String() string {
	return fmt.Sprintf("l=%v p=%v", r.Locations, r.Produce)
}

type entry struct {
	nextTokens map[string]map[int][]string
}

type prediction struct {
	rules   map[int]map[string]*rule
	context []string
}

func (p *prediction) match(prev []string) (map[int]map[string]*rule, bool) {
	found := map[int]map[string]*rule{}
	loc := len(prev) - 1
	for d, rules := range p.rules {
		for expected, aRule := range rules {
			if prev[loc-d] == expected {
				frules, ok := found[d]
				if !ok {
					frules = map[string]*rule{}
					found[d] = frules
				}
				frules[expected] = aRule
			} else {
				// check for partial matches
			}
		}
	}
	return found, len(found) != 0
}

func (p *prediction) subMatch(prev []string) int {
	max := -1
	loc := len(prev) - 1
	for d, rules := range p.rules {
		for _, aRule := range rules {
			for i := 0; i <= d; i++ {
				//fmt.Println("subMatch", prev[loc-i], prev[aRule.Location-i])
				for _, location := range aRule.Locations {
					if prev[loc-i] == prev[location-i] {
						if max < i {
							max = i
						}
					}
				}
			}
		}
	}
	return max + 1
}

func (p *prediction) addToken(prev []string, produce string) bool {
	loc := len(prev) - 1
	matchRules, didMatch := p.match(prev)
	//fmt.Println("checking for matching rules", matchRules, didMatch, prev)
	if !didMatch {
		minDepth := p.subMatch(prev)
		//fmt.Printf("no rules match, longest submatch is %v for %v %v\n", minDepth, prev, produce)
		rules, ok := p.rules[minDepth]
		if !ok {
			rules = map[string]*rule{}
			p.rules[minDepth] = rules
		}
		if rules[prev[loc-minDepth]] != nil {
			//fmt.Println(rules[prev[loc-minDepth]], prev[loc-minDepth], loc-minDepth)
			panic("we can't overwrite a rule")
		}
		rules[prev[loc-minDepth]] = &rule{
			Locations: []int{loc},
			Produce:   produce,
		}
		return true
	}
	// look for exact match
	for _, rules := range matchRules {
		for _, rule := range rules {
			if rule.Produce == produce {
				rule.Locations = append(rule.Locations, loc)
				//fmt.Println("! rule already exists")
				return false
			}
		}
	}

	// we have matches, but nothing producing our token
	// we need to adjust the matching rules backwards, then check again

	for d, rules := range matchRules {
		for expect, aRule := range rules {
			//fmt.Println("match ", d, expect, aRule)
			delete(p.rules[d], expect)
			newDepth := d + 1
			newRules, ok := p.rules[newDepth]
			if !ok {
				newRules = map[string]*rule{}
				p.rules[newDepth] = newRules
			}
			reinsert := map[int][]*rule{
				newDepth: []*rule{
					aRule,
				},
			}
			for len(reinsert) != 0 {
				next := map[int][]*rule{}
				for newDepth, rules := range reinsert {
					for _, aRule := range rules {
						for _, location := range aRule.Locations {
							newExpect := prev[location-newDepth]
							otherRule, ok := newRules[newExpect]
							if !ok {
								//fmt.Println("created new rule", location)
								newRules[newExpect] = &rule{
									Locations: []int{location},
									Produce:   aRule.Produce,
								}
								continue
							}
							if otherRule.Produce != aRule.Produce {
								continue
								delete(newRules, newExpect)
								fmt.Println("adjusting rules", otherRule.Produce, aRule.Produce)
								for _, location := range otherRule.Locations {
									next[newDepth+1] = append(next[newDepth+1], &rule{
										Locations: []int{location},
										Produce:   prev[location-newDepth-1],
									})
								}
								aRule.Produce = prev[location-newDepth-1]
								next[newDepth+1] = append(next[newDepth+1], otherRule, aRule)
								continue
							}
							//fmt.Println("joined with another rule", location)
							otherRule.Locations = append(otherRule.Locations, location)
							//fmt.Println("update", newDepth, newExpect, aRule)
						}
					}
				}
				reinsert = next
			}
		}
	}
	// wow! recursion!
	return p.addToken(prev, produce)
}

func (p *prediction) predict(prev []string) string {
	matchRules, found := p.match(prev)
	if !found {
		fmt.Println(prev, p.rules)
		panic("unable to predict next token")
	}
	// double check that we only have one?
	for _, rules := range matchRules {
		for _, aRule := range rules {
			return aRule.Produce
		}
	}
	panic("we should have had a rule match somewhere...")
}

func (p *prediction) addLocation(loc int) {
	rules, found := p.rules[0]
	if !found {
		rules = map[string]*rule{}
		p.rules[0] = rules
	}
	aRule, found := rules[""]
	if !found {
		aRule = &rule{}
		rules[""] = aRule
	}
	aRule.Locations = append(aRule.Locations, loc)
}

func (p *prediction) inflateRules() {
	// pull the rules out of the first level
	byExpect := map[string][]int{}

	// the largest producer group should stay at the 0 level, better encoding
	// 523,441 <-before:after-> 480,800
	mapping := map[string]int{}
	max := 0
	maxKey := ""
	for _, location := range p.rules[0][""].Locations {
		key := p.context[location+1]
		mapping[key]++
		count := mapping[key]
		if count > max {
			max = count
			maxKey = key
		}
	}

	// now add into the level 0 mapping
	locations := []int{}
	orig := p.rules[0][""]
	newRule := &rule{
		Produce: maxKey,
	}
	p.rules[0] = map[string]*rule{
		p.context[orig.Locations[0]]: newRule,
	}
	for _, location := range orig.Locations {
		key := p.context[location+1]
		if key != maxKey {
			locations = append(locations, location)
			continue
		}
		newRule.Locations = append(newRule.Locations, location)
	}

	for _, location := range locations {
		key := p.context[location]
		byExpect[key] = append(byExpect[key], location)
	}
	for depth := 1; true; depth++ {
		if len(byExpect) == 0 {
			break
		}
		p.rules[depth] = map[string]*rule{}
		stay := map[string][]int{}
		collision := map[string][]int{}
		// we should work in sorted order let larger groups of rules stay at this level
		for k, v := range byExpect {
			values, ok := stay[k]
			if !ok {
				stay[k] = append(stay[k], v[0])
				values = stay[k]
			}
			skip := []int{}
			// maybe filter to largest group?
			for _, l := range v[1:] {
				if p.context[values[0]+1] == p.context[l+1] {
					stay[k] = append(stay[k], l)
				} else {
					skip = append(skip, l)
				}
			}

			// all rules that have collisions need to be shifted down deeper
			for _, location := range skip {
				if location == depth {
					// we need to swap
					t := stay[k]
					stay[k] = []int{location}
					fmt.Println("swapping", location, t)
					// work on the swapped values
					for _, location := range t {
						key := p.context[location-depth-1]
						collision[key] = append(collision[key], location)
					}
					continue
				}
				key := p.context[location-depth-1]
				collision[key] = append(collision[key], location)
			}
		}

		// add in rules that are staying at this depth
		for expected, locations := range stay {
			_, ok := p.rules[depth][expected]
			if ok {
				panic("there should be no rules at this depth or key yet")
			}
			// all these rules should produce the same token.
			p.rules[depth][expected] = &rule{
				Produce:   p.context[locations[0]+1],
				Locations: locations,
			}
		}

		byExpect = collision
	}
}

func main() {
	fmt.Println("reading file into memory")
	file, err := os.ReadFile(os.Args[1])
	if err != nil {
		panic(err)
	}
	//testCompression(file)

	fmt.Println("searching for words")
	// CHANGES
	// adjust token to allow hyphenated words
	// 467,620 <-before:after-> 313,806
	// 'the' 'of' 'and' are just part of antoher token now
	// maybe 313983 Bytes
	// maybe 311658 Bytes
	re := regexp.MustCompile(`(the |and |in |to |a |is |as )?([\w-]+|[^\w]+)( of)?[ ,.;:]+`)
	//re := regexp.MustCompile(`([\w-]+|[^\w]+)`)
	words := re.FindAll(file, -1)
	fmt.Println("processing words")
	counts := map[string]int{}
	swords := []string{""}
	entries := map[string]*prediction{}
	needUpdate := time.NewTicker(time.Second / 10)
	update := func(pattern string, values ...interface{}) {
		select {
		case <-needUpdate.C:
			fmt.Fprintf(os.Stderr, pattern, values...)
		default:
		}
	}
	for i, v := range words {
		token := string(v)
		update("%05.2f stringing for '%v'      \r", float32(i)/float32(len(words))*100, token)
		swords = append(swords, token)
	}
	for i, token := range swords {
		if i == len(swords)-1 {
			break
		}
		p, found := entries[token]
		if !found {
			p = &prediction{
				rules:   map[int]map[string]*rule{},
				context: swords,
			}
			entries[token] = p
		}
		update("%05.2f populating locations for '%v'      \r", float32(i)/float32(len(words))*100, token)
		counts[token]++
		p.addLocation(i)
	}

	fmt.Println("getting unique tokens")
	unique := []string{}
	for k := range entries {
		unique = append(unique, k)
	}
	slices.Sort(unique)
	fmt.Println("inflating rules")
	wg := &sync.WaitGroup{}
	for i, token := range unique {
		pred := entries[token]
		wg.Add(1)
		go func() {
			defer wg.Done()
			pred.inflateRules()
			update("%05.2f inflating rules for '%v'      \r", float32(i)/float32(len(unique))*100, token)
		}()
	}
	wg.Wait()

	fmt.Println("")
	fmt.Println("all rules")
	count := 0
	mapped := map[string]int{}
	for i, s := range unique {
		mapped[s] = i
	}

	fmt.Println("encoidng as 000000100010000")
	fmt.Printf("would need to allocate %v MB\n", len(unique)*len(unique)/8/1024/1024)
	// each rule for a token would encode as either a byte or a uint32.
	// it would represent how many leading 0s to write before writing a single 1.
	// the index for the rules would be encoded as large as a uint32 as well
	fmt.Println("sparsely populating a map")
	ruleEntries := map[int][]uint64{}
	for i, k := range unique {
		update("%05.2f populating rules for '%v'      \r", float32(i)/float32(len(unique))*100, k)
		prediction := entries[k]
		for _, rules := range prediction.rules {
			for t, rule := range rules {
				ruleEntries[mapped[t]] = append(ruleEntries[mapped[t]], uint64(mapped[rule.Produce]))
			}
		}
	}
	// now we just go through the unique list again and build the buffer
	// probably should build the rule ids at the same time
	sparseRuleBuf := []byte{}
	ruleMapping := map[uint64]int{}
	count = 1
	for i, one := range unique {
		update("%05.2f populating rules for '%v'      \r", float32(i)/float32(len(unique))*100, one)
		values := ruleEntries[mapped[one]]
		slices.Sort(values)
		values = slices.Compact(values)
		prev := uint64(0)
		for _, v := range values {
			sparseRuleBuf = binary.AppendUvarint(sparseRuleBuf, v-prev)
			ruleMapping[uint64(mapped[one])<<32|uint64(v)] = count
			count++
			prev = v
		}
		sparseRuleBuf = binary.AppendUvarint(sparseRuleBuf, 0)
	}
	fmt.Printf("sparse rule buffer is %v B in size\n", len(sparseRuleBuf))

	fmt.Println("finding duplicate rules")
	// find duplicate rules
	ruleCount := map[uint64]int{}
	for _, k := range unique {
		prediction := entries[k]
		for d, rules := range prediction.rules {
			// record unique rules but not on depth 0, as thats shouldn't have dups that
			if d == 0 {
				continue
			}
			for t, rule := range rules {
				// would corrospend to ther tokens
				ruleCount[uint64(mapped[t])<<32|uint64(mapped[rule.Produce])]++
			}
		}
	}
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

	buff := []byte{}

	// record rules

	dbuff := map[int][]byte{}
	biggestRules := map[string]int{}
	maxd := 0
	fmt.Println("encoding rules")
	for _, k := range unique {
		prediction := entries[k]
		// just record the token in a different buffer
		buff = binary.AppendUvarint(buff, uint64(mapped[k]))
		for d, rules := range prediction.rules {
			if d > maxd {
				maxd = d
			}

			// sort the keys so we can just write differences
			keys := []int{}
			for t := range rules {
				keys = append(keys, mapped[t])
			}
			slices.Sort(keys)
			prev := 0
			// enableing delta encoding
			// 575,964 <- before:after-> 521,657
			delta := true
			// diff encoding between produce vs expect
			// 551,095 <- before:after-> 523,441
			diff := true

			allRuleMapping := []uint64{}
			for _, t := range keys {
				aRule := rules[unique[t]]
				id := uint64(t)<<32 | uint64(mapped[aRule.Produce])
				idx := ruleMapping[id]
				allRuleMapping = append(allRuleMapping, uint64(idx))
			}
			slices.Sort(allRuleMapping)
			allRuleMapping = slices.Compact(allRuleMapping)
			pruleid := uint64(0)
			for i, t := range keys {
				aRule := rules[unique[t]]
				biggestRules[k]++
				produce := mapped[aRule.Produce]
				count++
				if len(aRule.Locations) == 1 {
					//continue
				}
				l := len(dbuff[d])

				// dedup rules and enocde their ids
				// 480,861 <-before:after-> 476,051 1M
				// 49,612,624 <-before:after-> 47,353,642 100M
				enableRuleMap := true
				id := uint64(t)<<32 | uint64(mapped[aRule.Produce])
				if true {
					idx := allRuleMapping[i]
					dbuff[d] = binary.AppendUvarint(dbuff[d], idx-pruleid)
					pruleid = idx
				} else {
					idx, ok := ruleMap[id]
					if ok && enableRuleMap {
						//write out the duplicate rule
						dbuff[d] = binary.AppendUvarint(dbuff[d], uint64(0))
						dbuff[d] = binary.AppendUvarint(dbuff[d], uint64(idx))
						// we can still correctly do the delta encoding
						prev = t
					} else {
						if diff {
							dbuff[d] = binary.AppendVarint(dbuff[d], int64(produce-t))
						} else {
							// probably should do Uvarint instead
							dbuff[d] = binary.AppendVarint(dbuff[d], int64(produce))
						}

						// depth 0 matches do not need to encode the original token
						// 611,481 <-before:after-> 567,296
						if d != 0 {

							if delta {
								dbuff[d] = binary.AppendUvarint(dbuff[d], uint64(t-prev))
								prev = t
							} else {
								dbuff[d] = binary.AppendUvarint(dbuff[d], uint64(t))
							}
						}
					}
				}
				size := len(dbuff[d]) - l
				update("sample token:'%v' depth:'%v' expect:'%v' produce:'%v' locations:%v bytes %v\n", k, d, t, aRule.Produce, aRule.Locations, size)
			}
			// null terminate the list of rules maybe save %0.1 of the file size
			dbuff[d] = binary.AppendUvarint(dbuff[d], uint64(0))
		}
	}

	fmt.Println("number of tokens", len(swords))
	fmt.Println("number of unique tokens", len(entries))
	fmt.Printf("found %v rules\n", count)
	fmt.Printf("found %v duplicate rules with %v rules\n", ruleDup, len(ruleKey))
	fmt.Printf("%v rules / unique token\n", count/len(entries))
	fmt.Printf("GOAL %v bytes, or %v bytes / rule\n", 100*1024, 100*1024/count)
	fmt.Println("amount of bytes needed for single token", len(entries)/8)
	total := len(buff)
	for i := 0; i <= maxd; i++ {
		b := dbuff[i]
		fmt.Printf("depth %v maybe %v Bytes\n", i, len(b))
		total += len(b)
	}
	fmt.Printf("maybe %v KB and %v KB\n", total/1024, len(sparseRuleBuf)/1024)
	sbuff := []byte{}
	for _, v := range unique {
		sbuff = append(sbuff, []byte(v)...)
	}
	fmt.Printf("tokens maybe %v KB\n", len(sbuff)/1024)

	fmt.Println("top 10 complex tokens")
	slices.SortFunc(unique, func(a, b string) int {
		//reversed for decending
		return cmp.Compare(biggestRules[b], biggestRules[a])
	})
	for i := 0; i < 10; i++ {
		fmt.Println("token", unique[i], biggestRules[unique[i]])
	}
	return
	fmt.Println("regenerated text")
	// lets try and regenerate it!
	regenerated := []string{""}
	for i := 0; i < 51; i++ {
		prediction := entries[regenerated[i]]
		next := prediction.predict(regenerated)
		if swords[i+1] != next {
			fmt.Println("#", swords[i], next)
		}
		regenerated = append(regenerated, next)
	}
	fmt.Println(regenerated)

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
