package drift

// fittingPrefix returns the longest rune prefix found by binary search that
// satisfies fits. fit is false only when even an empty prefix cannot satisfy
// the caller's fixed prompt overhead.
func fittingPrefix(text string, fits func(string) (bool, error)) (prefix string, fit bool, err error) {
	if ok, err := fits(text); err != nil || ok {
		return text, ok, err
	}
	if ok, err := fits(""); err != nil || !ok {
		return "", ok, err
	}

	runes := []rune(text)
	low, high := 0, len(runes)-1
	best := 0
	for low <= high {
		middle := low + (high-low)/2
		candidate := string(runes[:middle+1])
		ok, err := fits(candidate)
		if err != nil {
			return "", false, err
		}
		if ok {
			best = middle + 1
			low = middle + 1
		} else {
			high = middle - 1
		}
	}
	return string(runes[:best]), true, nil
}
