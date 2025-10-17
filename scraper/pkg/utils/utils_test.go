package utils

import (
	"fmt"
	"testing"
)

func TestMultiSplit(t *testing.T) {
	cases := []string{
		" watched, liked and rated ",
		" watched, rated ",
	}

	for _, c := range cases {
		fmt.Printf("%#v\n", MultiSplit(c, ", ", " and "))
	}
}
