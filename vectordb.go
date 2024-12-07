package main

import "math"

const isNormalizedPrecisionTolerance = 1e-6

// normalizeVector normalizes a vector.
//
// Code taken from https://github.com/philippgille/chromem-go/blob/main/vector.go
func normalizeVector(v []float32) []float32 {
	var norm float32
	for _, val := range v {
		norm += val * val
	}
	norm = float32(math.Sqrt(float64(norm)))

	res := make([]float32, len(v))
	for i, val := range v {
		res[i] = val / norm
	}

	return res
}

// isNormalized checks if the vector is normalized.
//
// Code taken from https://github.com/philippgille/chromem-go/blob/main/vector.go
func isNormalized(v []float32) bool {
	var sqSum float64
	for _, val := range v {
		sqSum += float64(val) * float64(val)
	}
	magnitude := math.Sqrt(sqSum)
	return math.Abs(magnitude-1) < isNormalizedPrecisionTolerance
}
