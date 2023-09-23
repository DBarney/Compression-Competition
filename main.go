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
	rules      map[int]map[string]*rule
	context    []string
	counts     map[string]int
	countOrder map[string]int
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
	fmt.Println(matchRules)
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

func (p *prediction) inflateRules(percent float32, update func(string, ...interface{})) {
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
		update("%05.2f inflating rules %v %v %v        \r", percent, p.context[orig.Locations[0]], depth, len(byExpect))
		if len(byExpect) == 0 {
			break
		}
		p.rules[depth] = map[string]*rule{}
		stay := map[string][]int{}
		collision := map[string][]int{}

		keys := []string{}
		for k := range byExpect {
			keys = append(keys, k)
		}
		slices.SortFunc(keys, func(a, b string) int {
			la := len(byExpect[a])
			lb := len(byExpect[b])
			if la == lb {
				// sorter tokens might be better?
				// also try to do this with more common tokens
				return cmp.Compare(b, a)
			}
			// pick the option with more that go towards the same token
			return cmp.Compare(len(byExpect[a]), len(byExpect[b]))
		})
		keys = slices.Compact(keys)

		skey := []string{}
		pkey := []string{}
		for _, k := range keys {
			// lets just peg everything against the most common tokens O_o, or the first
			if p.countOrder[k] < 128 || k == "" {
				pkey = append(pkey, k)
			} else {
				skey = append(skey, k)
			}
		}

		// if nothing was a common token, just let one in
		if len(pkey) == 0 {
			pkey = []string{skey[0]}
			skey = skey[1:]
		}
		// move all skipped keys down a level
		for _, k := range skey {
			skip := byExpect[k]

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

		for _, k := range pkey {
			v := byExpect[k]
			skip := []int{}

			values, ok := stay[k]
			if !ok {
				stay[k] = append(stay[k], v[0])
				v = v[1:]
				values = stay[k]
			}

			// maybe filter to largest group?
			for _, l := range v {
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
	meta := &struct {
		Words        []string
		Count        map[string]int
		CountOrder   []string
		CountLookup  map[string]int
		CountRLookup map[int]string
		Unique       []string
		Lookup       map[string]int
		RLookup      map[int]string
		Locations    map[string][]int
	}{}

	needUpdate := time.NewTicker(time.Second / 10)
	update := func(pattern string, values ...interface{}) {
		select {
		case <-needUpdate.C:
			fmt.Fprintf(os.Stderr, pattern, values...)
		default:
		}
	}

	fmt.Println("reading precomputed tokens")
	gobs, err := os.Open(os.Args[1] + ".gob")
	if err == nil {
		defer gobs.Close()
		d := gob.NewDecoder(gobs)

		err = d.Decode(&meta)
		if err != nil {
			panic(err)
		}
	} else {
		fmt.Println("failed: ", err)
		fmt.Println("reading file into memory")
		file, err := os.ReadFile(os.Args[1])
		if err != nil {
			panic(err)
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
		re := regexp.MustCompile(`([\w-]+|[^\w]+)[ ]+`)
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
		gobs, err = os.Create(os.Args[1] + ".gob")
		if err != nil {
			panic(err)

		}
		defer gobs.Close()
		e := gob.NewEncoder(gobs)

		err = e.Encode(meta)
		if err != nil {
			panic(err)
		}
	}
	entries := map[string]*prediction{}
	for i, word := range meta.Words {
		entries[word] = &prediction{
			rules: map[int]map[string]*rule{
				0: map[string]*rule{
					"": &rule{
						Locations: meta.Locations[word],
					},
				},
			},
			context:    meta.Words,
			counts:     meta.Count,
			countOrder: meta.CountLookup,
		}
		update("%05.2f building prediction structures\r", float32(i)/float32(len(meta.Words))*100)
	}

	fmt.Println("inflating rules")
	wg := &sync.WaitGroup{}
	work := make(chan struct {
		p *prediction
		i int
	})
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for w := range work {
				p := float32(w.i) / float32(len(meta.Unique)) * 100
				w.p.inflateRules(p, update)
			}
		}()
	}
	for i, word := range meta.Unique {
		work <- struct {
			p *prediction
			i int
		}{
			p: entries[word],
			i: i,
		}
	}
	close(work)
	wg.Wait()

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
	regenerated := []string{""}
	for i := 0; i < 51; i++ {
		prediction := entries[regenerated[i]]
		next := prediction.predict(regenerated)
		if meta.Words[i+1] != next {
			fmt.Println("#", meta.Words[i], next)
		}
		regenerated = append(regenerated, next)
	}
	fmt.Println(regenerated)

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
