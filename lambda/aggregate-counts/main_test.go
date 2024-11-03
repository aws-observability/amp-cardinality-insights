package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTopN(t *testing.T) {

	//test cases
	tests := map[string]struct {
		input    map[string]int64
		expected CardinalityList
	}{
		"empty list": {
			input:    map[string]int64{},
			expected: CardinalityList{},
		},
		"example1": {
			input: map[string]int64{
				"a": 1,
				"b": 50,
				"c": 15,
			},
			expected: CardinalityList{
				Cardinality{"b", 50},
				Cardinality{"c", 15},
				Cardinality{"a", 1},
			},
		},
		"example2": {
			input: map[string]int64{
				"a": 1,
				"b": 50,
				"c": 15,
				"d": 80,
				"e": 6,
				"f": 90,
				"g": 26,
			},
			expected: CardinalityList{
				Cardinality{"f", 90},
				Cardinality{"d", 80},
				Cardinality{"b", 50},
				Cardinality{"g", 26},
				Cardinality{"c", 15},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// setup
			expected := topN(tc.input, 5)
			assert.Equal(t, tc.expected, expected)
		})
	}
}
