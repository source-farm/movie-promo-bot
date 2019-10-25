package levenshtein

import (
	"testing"
)

var testCases = []struct {
	source   string
	target   string
	insCost  int
	delCost  int
	subCost  int
	distance int
}{
	{
		"",
		"",
		1,
		2,
		3,
		0,
	},
	{
		"",
		"a",
		1,
		2,
		3,
		1,
	},
	{
		"a",
		"",
		1,
		2,
		3,
		2,
	},
	{
		"a",
		"b",
		1,
		3,
		2,
		2,
	},
	{
		"abc",
		"cba",
		1,
		1,
		1,
		2,
	},
	{
		"push",
		"pull",
		2,
		3,
		7,
		10,
	},
	{
		"clockwise",
		"otherwise",
		1,
		1,
		1,
		5,
	},
}

func TestLevenshtein(t *testing.T) {
	for _, test := range testCases {
		distance := Distance(test.source, test.target, test.insCost, test.delCost, test.subCost)
		if test.distance != distance {
			t.Fatalf("%s -> %s got %d, expected %d", test.source, test.target, distance, test.distance)
		}
	}
}
