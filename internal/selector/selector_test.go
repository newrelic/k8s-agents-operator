package selector

import (
	"github.com/google/go-cmp/cmp"
	"testing"
)

func TestMatches(t *testing.T) {
	selectorNew := func(simpleSelector *SimpleSelector) Selector {
		s, _ := New(simpleSelector)
		return s
	}

	tests := []struct {
		name          string
		selector      Selector
		kv            map[string]string
		expectedMatch bool
	}{
		{
			name: "nil",
			selector: selectorNew(&SimpleSelector{
				MatchExact:       nil,
				MatchExpressions: nil,
			}),
			kv:            map[string]string{},
			expectedMatch: true,
		},
		{
			name: "empty + nil",
			selector: selectorNew(&SimpleSelector{
				MatchExact:       map[string]string{},
				MatchExpressions: nil,
			}),
			kv:            map[string]string{},
			expectedMatch: true,
		},
		{
			name: "nil + empty",
			selector: selectorNew(&SimpleSelector{
				MatchExact:       nil,
				MatchExpressions: []SelectorRequirement{},
			}),
			kv:            map[string]string{},
			expectedMatch: true,
		},
		{
			name: "empty",
			selector: selectorNew(&SimpleSelector{
				MatchExact:       map[string]string{},
				MatchExpressions: []SelectorRequirement{},
			}),
			kv:            map[string]string{},
			expectedMatch: true,
		},
		{
			name: "no map",
			selector: selectorNew(&SimpleSelector{
				MatchExact:       map[string]string{},
				MatchExpressions: []SelectorRequirement{},
			}),
			kv:            nil,
			expectedMatch: true,
		},
		{
			name: "empty map",
			selector: selectorNew(&SimpleSelector{
				MatchExact:       map[string]string{},
				MatchExpressions: []SelectorRequirement{},
			}),
			kv:            map[string]string{},
			expectedMatch: true,
		},

		{
			name: "matches",
			selector: selectorNew(&SimpleSelector{
				MatchExact:       map[string]string{"Z": "z"},
				MatchExpressions: []SelectorRequirement{},
			}),
			kv:            map[string]string{"Z": "z"},
			expectedMatch: true,
		},
		{
			name: "doesnt match, because of case sensitivity",
			selector: selectorNew(&SimpleSelector{
				MatchExact:       map[string]string{"Z": "Z"},
				MatchExpressions: []SelectorRequirement{},
			}),
			kv:            map[string]string{"Z": "z"},
			expectedMatch: false,
		},

		{
			name: "exists",
			selector: selectorNew(&SimpleSelector{
				MatchExpressions: []SelectorRequirement{
					{
						Key:      "R",
						Operator: "Exists",
					},
				},
			}),
			kv:            map[string]string{"R": "t"},
			expectedMatch: true,
		},
		{
			name: "doesnt exist",
			selector: selectorNew(&SimpleSelector{
				MatchExpressions: []SelectorRequirement{
					{
						Key:      "R",
						Operator: "Exists",
					},
				},
			}),
			kv:            map[string]string{"G": "t"},
			expectedMatch: false,
		},

		{
			name: "notexists, and its present",
			selector: selectorNew(&SimpleSelector{
				MatchExpressions: []SelectorRequirement{
					{
						Key:      "A",
						Operator: "NotExists",
					},
				},
			}),
			kv:            map[string]string{"A": "e"},
			expectedMatch: false,
		},
		{
			name: "notexists, and its not present",
			selector: selectorNew(&SimpleSelector{
				MatchExpressions: []SelectorRequirement{
					{
						Key:      "A",
						Operator: "NotExists",
					},
				},
			}),
			kv:            map[string]string{"R": "2"},
			expectedMatch: true,
		},

		//@todo: fix these below
		{
			name: "both, in",
			selector: selectorNew(&SimpleSelector{
				MatchExpressions: []SelectorRequirement{
					{
						Key:      "R",
						Operator: "In",
						Values:   []string{"a", "b"},
					},
				},
			}),
			kv:            map[string]string{},
			expectedMatch: true,
		},
		{
			name: "both, not equal",
			selector: selectorNew(&SimpleSelector{
				MatchExpressions: []SelectorRequirement{
					{
						Key:      "R",
						Operator: "!=",
						Values:   []string{"z"},
					},
				},
			}),
			kv:            map[string]string{},
			expectedMatch: true,
		},
		{
			name: "b, equal",
			selector: selectorNew(&SimpleSelector{
				MatchExpressions: []SelectorRequirement{
					{
						Key:      "R",
						Operator: "==",
						Values:   []string{"b"},
					},
				},
			}),
			kv:            map[string]string{},
			expectedMatch: true,
		},
		{
			name: "a, not in",
			selector: selectorNew(&SimpleSelector{
				MatchExpressions: []SelectorRequirement{
					{
						Key:      "R",
						Operator: "NotIn",
						Values:   []string{"b"},
					},
				},
			}),
			kv:            map[string]string{},
			expectedMatch: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actualMatch := tt.selector.Matches(tt.kv)
			if diff := cmp.Diff(tt.expectedMatch, actualMatch); diff != "" {
				t.Errorf("Unexpected diff (-want +got): %s", diff)
			}
		})
	}
}
