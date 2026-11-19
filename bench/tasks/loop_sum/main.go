package main

import "fmt"

func main() {
	var s int64
	for i := int64(1); i <= 5_000_000; i++ {
		s += i * i % 1_000_003
	}
	fmt.Printf("sum = %d\n", s)
}
