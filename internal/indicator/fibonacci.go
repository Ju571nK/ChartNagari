package indicator

// fibonacci computes Fibonacci retracement levels between swingHigh and swingLow.
// swingHigh must be > swingLow; otherwise an empty map is returned.
//
// Level formula:  price = swingHigh - (swingHigh - swingLow) * ratio
//   FIB_0   = swingLow  (ratio 1.0)
//   FIB_236           (ratio 0.236)
//   FIB_382           (ratio 0.382)
//   FIB_500           (ratio 0.5)
//   FIB_618           (ratio 0.618)
//   FIB_786           (ratio 0.786)
//   FIB_100 = swingHigh (ratio 0.0)
func fibonacci(swingHigh, swingLow float64) map[string]float64 {
	result := make(map[string]float64)
	if swingHigh <= swingLow {
		return result
	}
	diff := swingHigh - swingLow
	levels := []struct {
		key   string
		ratio float64
	}{
		{"FIB_0", 1.0},
		{"FIB_236", 0.236},
		{"FIB_382", 0.382},
		{"FIB_500", 0.5},
		{"FIB_618", 0.618},
		{"FIB_786", 0.786},
		{"FIB_100", 0.0},
	}
	for _, l := range levels {
		result[l.key] = swingHigh - diff*l.ratio
	}
	return result
}
