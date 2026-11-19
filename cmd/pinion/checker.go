package main

import (
	"github.com/pinionlang/pinion/internal/check"
	"github.com/pinionlang/pinion/internal/driver"
)

func init() { driver.Checker = check.Check }
