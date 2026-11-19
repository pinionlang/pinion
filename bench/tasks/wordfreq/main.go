package main

import (
	"fmt"
	"sort"
	"strings"
)

var syl = []string{"ka", "lo", "mi", "nu", "pe", "ra", "si", "to", "ve", "wa", "xi", "yo", "ze", "bu", "da", "fo"}

type entry struct {
	word  string
	count int
}

func main() {
	x := int64(42)
	parts := make([]string, 0, 300_000)
	for i := 0; i < 300_000; i++ {
		x = x * 48271 % 2147483647
		parts = append(parts, syl[x%16]+syl[x/16%16])
	}
	text := strings.Join(parts, " ")

	counts := map[string]int{}
	for _, w := range strings.Fields(text) {
		counts[w]++
	}
	ranked := make([]entry, 0, len(counts))
	for w, c := range counts {
		ranked = append(ranked, entry{w, c})
	}
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].count != ranked[j].count {
			return ranked[i].count > ranked[j].count
		}
		return ranked[i].word < ranked[j].word
	})
	fmt.Printf("distinct = %d\n", len(ranked))
	for _, e := range ranked[:10] {
		fmt.Printf("%s %d\n", e.word, e.count)
	}
}
