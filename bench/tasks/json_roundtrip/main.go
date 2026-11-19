package main

import (
	"encoding/json"
	"fmt"
	"os"
)

type Rec struct {
	ID     int64    `json:"id"`
	Name   string   `json:"name"`
	Tags   []string `json:"tags"`
	Score  int64    `json:"score"`
	Active bool     `json:"active"`
}

func main() {
	x := int64(7)
	recs := make([]Rec, 0, 20_000)
	for i := int64(0); i < 20_000; i++ {
		x = x * 48271 % 2147483647
		recs = append(recs, Rec{i, fmt.Sprintf("user%d", i), []string{fmt.Sprintf("t%d", i%7), fmt.Sprintf("g%d", i%11)}, x % 1000, x%3 == 0})
	}
	text, err := json.Marshal(recs)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	var back []Rec
	if err := json.Unmarshal(text, &back); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	var score, active, tags int64
	for _, r := range back {
		score += r.Score
		if r.Active {
			active++
		}
		tags += int64(len(r.Tags))
	}
	fmt.Printf("bytes = %d\n", len(text))
	fmt.Printf("records = %d score = %d active = %d tags = %d\n", len(back), score, active, tags)
}
