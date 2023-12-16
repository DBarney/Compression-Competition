package main

import (
	"cmp"
	"fmt"
	"slices"
	"sync"
)

type rule struct {
	Locations []int
	Produce   string
}

func (r rule) String() string {
	return fmt.Sprintf("l=%v p=%v", r.Locations, r.Produce)
}

type prediction struct {
	rules      map[int]map[string]*rule
	context    []string
	counts     map[string]int
	countOrder map[string]int
}

func predict(entries map[string]*prediction, first string, count int) []string {

	regenerated := []string{first}
	for i := 0; i < count-1; i++ {
		prediction := entries[regenerated[i]]
		next := prediction.predict(regenerated)
		regenerated = append(regenerated, next)
	}
	return regenerated
}

func fromFile(meta *file) map[string]*prediction {
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
	// this was never added
	entries[""].rules[0][""].Locations = []int{0}

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
		//fmt.Printf("working on '%v'\n", word)
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

	// cover the very first empty token. its not part of the Unique list
	entries[""].inflateRules(100, update)

	return entries
}

func (p *prediction) match(prev []string) (map[int]map[string]*rule, bool) {
	found := map[int]map[string]*rule{}
	loc := len(prev) - 1
	depth := []int{}
	for d := range p.rules {
		depth = append(depth, d)
	}
	slices.Sort(depth)
	for i := 0; i < len(depth); i++ {
		//for i := len(depth) - 1; i >= 0; i-- {
		d := depth[i]
		if d > len(prev) {
			continue
		}
		rules := p.rules[d]
		for expected, aRule := range rules {
			if loc-d < 0 {
				continue
			}
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
	//fmt.Println("found", prev, matchRules)
	keys := []int{}

	for key := range matchRules {
		keys = append(keys, key)
	}
	slices.Sort(keys)

	for i := 0; i < len(keys); i++ {
		//for i := len(keys) - 1; i >= 0; i-- {
		key := keys[i]
		rules := matchRules[key]
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
	//fmt.Println("working on", p.context[p.rules[0][""].Locations[0]])
	// pull the rules out of the first level
	byExpect := map[string][]int{}
	/*
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
			key := p.context[location-1]
			byExpect[key] = append(byExpect[key], location)
		}
	*/
	for _, location := range p.rules[0][""].Locations {
		key := p.context[location]
		byExpect[key] = append(byExpect[key], location)
	}

	for depth := 0; true; depth++ {
		if len(byExpect) == 0 {
			break
		}
		// we are going to be adding rules at this depth
		p.rules[depth] = map[string]*rule{}
		// stay is all locations that will be staying at this level
		stay := map[string][]int{}
		// collision is all locations that don't have a unique match
		collision := map[string][]int{}

		// we need to process the locations that have been
		// sorted into what they are expecting to find
		// to be processed in the same order every time
		// so we sort the keys
		keys := []string{}
		for k := range byExpect {
			keys = append(keys, k)
		}
		slices.SortFunc(keys, func(a, b string) int {
			// we want larger groups first
			la := len(byExpect[a])
			lb := len(byExpect[b])
			if la == lb {
				// sorter tokens might be better?
				// also try to do this with more common tokens
				return cmp.Compare(b, a)
			}
			// pick the option with more that go towards the same token
			return cmp.Compare(la, lb)
		})

		// skey is keys that will be skipped
		skey := []string{}
		// pkey is keys that will be processed
		pkey := []string{}
		for _, k := range keys {
			// lets just peg everything against the most common tokens O_o, or the first 128 tokens
			if p.countOrder[k] < 128 || k == "" {
				pkey = append(pkey, k)
			} else {
				skey = append(skey, k)
			}

			/*
				// lets peg every rule to a unique token
				if p.counts[k] == 1 {
					pkey = append(pkey, k)
				} else {
					skey = append(skey, k)
				}
			*/
		}

		// if nothing was a common token, just let one in
		if len(pkey) == 0 {
			pkey = []string{skey[0]}
			skey = skey[1:]
		}
		// move all skipped keys down a level
		for _, k := range skey {
			locations := byExpect[k]

			// all rules that have collisions need to be shifted down deeper
			for _, location := range locations {
				if location == depth {
					// we need to swap
					t := stay[k]
					stay[k] = []int{location}
					//fmt.Println("swapping", location, t)
					// work on the swapped values
					for _, location := range t {
						key := p.context[location-depth-1]
						collision[key] = append(collision[key], location)
					}
					continue
				}
				key := p.context[location-depth-1]
				//fmt.Println("passing", k, key)
				collision[key] = append(collision[key], location)
			}
		}

		for _, k := range pkey {
			v := byExpect[k]
			skip := []int{}

			values, ok := stay[k]
			// if we don't have one picked,
			// just grab the first one
			if !ok {
				//fmt.Println("grabbing", k)
				stay[k] = append(stay[k], v[0])
				v = v[1:]
				values = stay[k]
			} else {
				//fmt.Println("already there", k)
			}

			// maybe filter to largest group?
		outer:
			for _, l := range v {
				// ensure that what is being produced matches
				if p.context[values[0]+1] == p.context[l+1] {
					stay[k] = append(stay[k], l)
				} else {
					// two possible rules produce different tokens
					// but expect the same one
					//fmt.Println("rule collision", k, l)
					skip = append(skip, v...)
					skip = append(skip, stay[k]...)
					slices.Sort(skip)
					slices.Compact(skip)
					delete(stay, k)
					break outer
				}
			}

			// all rules that have collisions need to be shifted down deeper
			for _, location := range skip {
				if location == depth {
					// we need to swap
					t := stay[k]
					stay[k] = []int{location}
					//fmt.Println("swapping", location, t)
					// work on the swapped values
					for _, location := range t {
						key := p.context[location-depth-1]
						collision[key] = append(collision[key], location)
					}
					continue
				}
				//fmt.Println("dropping down", location)
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
			//fmt.Println("add rule", depth, expected, locations, p.context[locations[0]+1])
			// all these rules should produce the same token.
			p.rules[depth][expected] = &rule{
				Produce:   p.context[locations[0]+1],
				Locations: locations,
			}
		}

		byExpect = collision
	}
}
