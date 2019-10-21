package utils

import "math"

func Round(num float64) int {
	return int(num + math.Copysign(0.5, num))
}
