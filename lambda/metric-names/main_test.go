package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSplitJobs(t *testing.T) {

	//test cases
	tests := map[string]struct {
		jobs     []string
		expected [][]string
	}{
		"no jobs": {
			jobs:     []string{},
			expected: [][]string{},
		},
		"small list": {
			jobs:     []string{"a", "b", "c"},
			expected: [][]string{{"a", "b", "c"}},
		},
		"even list": {
			jobs:     []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"},
			expected: [][]string{{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"}},
		},
		"even list, second case": {
			jobs:     []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k", "l", "m", "n", "o", "p", "q", "r", "s", "t"},
			expected: [][]string{{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"}, {"k", "l", "m", "n", "o", "p", "q", "r", "s", "t"}},
		},
		"uneven list": {
			jobs:     []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k", "l"},
			expected: [][]string{{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"}, {"k", "l"}},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// setup
			expected := splitJobs(tc.jobs)
			assert.Equal(t, tc.expected, expected)
		})
	}
}
