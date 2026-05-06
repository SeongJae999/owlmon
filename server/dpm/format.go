package dpm

import "strconv"

func itoa(n int) string { return strconv.Itoa(n) }

func ftoa(f float64, prec int) string {
	return strconv.FormatFloat(f, 'f', prec, 64)
}
