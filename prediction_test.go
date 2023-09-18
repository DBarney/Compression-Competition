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
		rules   map[int][]*rule
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
			rules: map[int][]*rule{
				0: []*rule{},
				1: []*rule{},
				2: []*rule{
					&rule{
						Location: 2,
						Expected: "",
						Produce:  "b",
					},
					&rule{
						Location: 5,
						Expected: "b",
						Produce:  "d",
					},
				},
			},
		},
		{
			tokens:  []string{"", "c", "a", "b", "c", "a", "d", "b", "c", "a", "f"},
			results: []bool{true, true, true},
			rules: map[int][]*rule{
				0: []*rule{},
				1: []*rule{},
				2: []*rule{
					&rule{
						Location: 2,
						Expected: "",
						Produce:  "b",
					},
				},
				3: []*rule{
					&rule{
						Location: 5,
						Expected: "a",
						Produce:  "d",
					},
					&rule{
						Location: 9,
						Expected: "d",
						Produce:  "f",
					},
				},
			},
		},
		{
			tokens: []string{"", "c", "a", "b", "c", "a", "d", "b", "c", "a", "f", "a", "g"},
			rules: map[int][]*rule{
				0: []*rule{
					&rule{
						Location: 11,
						Expected: "f",
						Produce:  "g",
					},
				},
				1: []*rule{},
				2: []*rule{
					&rule{
						Location: 2,
						Expected: "",
						Produce:  "b",
					},
				},
				3: []*rule{
					&rule{
						Location: 5,
						Expected: "a",
						Produce:  "d",
					},
					&rule{
						Location: 9,
						Expected: "d",
						Produce:  "f",
					},
				},
			},
		},
	}
	for i, test := range tests {
		t.Run(fmt.Sprintf("case: %v", i), func(t *testing.T) {
			p := &prediction{
				rules: map[int][]*rule{},
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
