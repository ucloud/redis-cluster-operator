package utils

// Stringer implement the string interface
type Stringer interface {
	String() string
}

// SliceJoin concatenates the elements of a to create a single string. The separator string
// sep is placed between elements in the resulting string.
func SliceJoin(a []Stringer, sep string) string {
	switch len(a) {
	case 0:
		return ""
	case 1:
		return a[0].String()
	case 2:
		// Special case for common small values.
		// Remove if golang.org/issue/6714 is fixed
		return a[0].String() + sep + a[1].String()
	case 3:
		// Special case for common small values.
		// Remove if golang.org/issue/6714 is fixed
		return a[0].String() + sep + a[1].String() + sep + a[2].String()
	}
	n := len(sep) * (len(a) - 1)
	for i := 0; i < len(a); i++ {
		n += len(a[i].String())
	}

	b := make([]byte, n)
	bp := copy(b, a[0].String())
	for _, s := range a[1:] {
		bp += copy(b[bp:], sep)
		bp += copy(b[bp:], s.String())
	}
	return string(b)
}
