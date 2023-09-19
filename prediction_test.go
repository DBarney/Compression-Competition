package main

import (
	"fmt"
	"github.com/google/go-cmp/cmp"
	"testing"
)

// func (p *prediction) addToken(prev []string, token string) bool {
func TestAddRule(t *testing.T) {
	tests := []struct {
		tokens  []string
		results []bool
		rules   map[int]map[string]*rule
	}{
		{
			tokens:  []string{"", "a", "b"},
			results: []bool{true},
		},
		{
			tokens:  []string{"", "a", "b", "a", "b"},
			results: []bool{true, false},
		},
		{
			tokens:  []string{"", "a", "b", "a", "c"},
			results: []bool{true, true},
		},
		{
			tokens:  []string{"", "c", "a", "b", "c", "a", "d"},
			results: []bool{true, true},
			rules: map[int]map[string]*rule{
				0: map[string]*rule{},
				1: map[string]*rule{},
				2: map[string]*rule{
					"": &rule{
						Locations: []int{2},
						Produce:   "b",
					},
					"b": &rule{
						Locations: []int{5},
						Produce:   "d",
					},
				},
			},
		},
		{
			tokens:  []string{"", "c", "a", "b", "c", "a", "d", "b", "c", "a", "f"},
			results: []bool{true, true, true},
			rules: map[int]map[string]*rule{
				0: map[string]*rule{},
				1: map[string]*rule{},
				2: map[string]*rule{
					"": &rule{
						Locations: []int{2},
						Produce:   "b",
					},
				},
				3: map[string]*rule{
					"a": &rule{
						Locations: []int{5},
						Produce:   "d",
					},
					"d": &rule{
						Locations: []int{9},
						Produce:   "f",
					},
				},
			},
		},
		{
			tokens: []string{"", "c", "a", "b", "c", "a", "d", "b", "c", "a", "f", "a", "g"},
			rules: map[int]map[string]*rule{
				0: map[string]*rule{},
				1: map[string]*rule{
					"f": &rule{
						Locations: []int{11},
						Produce:   "g",
					},
				},
				2: map[string]*rule{
					"": &rule{
						Locations: []int{2},
						Produce:   "b",
					},
				},
				3: map[string]*rule{
					"a": &rule{
						Locations: []int{5},
						Produce:   "d",
					},
					"d": &rule{
						Locations: []int{9},
						Produce:   "f",
					},
				},
			},
		},
		//TODO: duplicate rules that need to be separated out and made deeper
		{
			tokens:  []string{"", "c", "a", "b", "c", "a", "b", "x", "c", "a", "c"},
			results: []bool{true, false, true},
			rules: map[int]map[string]*rule{
				0: map[string]*rule{},
				1: map[string]*rule{},
				2: map[string]*rule{
					"": &rule{
						Locations: []int{2},
						Produce:   "b",
					},
					"b": &rule{
						Locations: []int{5},
						Produce:   "b",
					},
					"x": &rule{
						Locations: []int{9},
						Produce:   "c",
					},
				},
			},
		},
	}
	for i, test := range tests {
		t.Run(fmt.Sprintf("case: %v", i), func(t *testing.T) {
			p := &prediction{
				rules: map[int]map[string]*rule{},
			}
			t.Logf("ts: %v", test.tokens)
			found := 0
			for i := 0; i < len(test.tokens); i++ {
				if test.tokens[i] != "a" {
					continue
				}
				tokens := test.tokens[:i+1]
				next := test.tokens[i+1]
				t.Logf("ts: %v t: %v", tokens, next)
				added := p.addToken(tokens, next)
				if test.results != nil && test.results[found] != added {
					t.Logf("%v", p)
					if added {
						t.Fatalf("rule was added")
					} else {
						t.Fatalf("rule was not added")
					}
				} else {
					t.Log("√")
				}
				found++
			}
			if test.results != nil && found != len(test.results) {
				t.Fatalf("token was not found the correct number of times %v", found)
			}
			if test.rules != nil {
				if !cmp.Equal(test.rules, p.rules) {
					t.Log(cmp.Diff(p.rules, test.rules))
					t.Fatalf("rules do not match")
				}
			}
		})
	}
}
