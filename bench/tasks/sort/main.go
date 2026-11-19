package main

import (
	"cmp"
	"fmt"
	"slices"
)

type rec struct{ key, id int64 }

func checksum(xs []int64) int64 {
	var h int64
	for _, v := range xs {
		h = (h*31 + v) % 1_000_000_007
	}
	return h
}

func main() {
	x := int64(12345)
	nums := make([]int64, 0, 200_000)
	recs := make([]rec, 0, 200_000)
	for i := int64(0); i < 200_000; i++ {
		x = x * 48271 % 2147483647
		nums = append(nums, x%1_000_000)
		recs = append(recs, rec{x % 1000, i})
	}
	slices.Sort(nums)
	fmt.Printf("ints: min = %d max = %d checksum = %d\n", nums[0], nums[len(nums)-1], checksum(nums))
	slices.SortStableFunc(recs, func(a, b rec) int { return cmp.Compare(a.key, b.key) })
	ids := make([]int64, len(recs))
	for i, r := range recs {
		ids[i] = r.id
	}
	fmt.Printf("recs: checksum = %d\n", checksum(ids))
}
