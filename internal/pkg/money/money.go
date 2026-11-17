// Package money validates THB inputs and performs exact satang arithmetic.
package money

import (
	"errors"
	"math/big"
	"regexp"
	"strings"
)

var ErrInvalid = errors.New("invalid monetary amount")
var inputPattern = regexp.MustCompile(`^[0-9]{1,13}(\.[0-9]{1,2})?$`)
var totalPattern = regexp.MustCompile(`^[0-9]+(\.[0-9]{1,2})?$`)

// Normalize accepts zero but rejects negatives, exponents, rounding and overflow.
func Normalize(v string) (string, error) {
	v = strings.TrimSpace(v)
	if !inputPattern.MatchString(v) {
		return "", ErrInvalid
	}
	n, err := parse(v)
	if err != nil {
		return "", err
	}
	return format(n), nil
}

func parse(v string) (*big.Int, error) {
	if !totalPattern.MatchString(v) {
		return nil, ErrInvalid
	}
	w, f, _ := strings.Cut(v, ".")
	f += strings.Repeat("0", 2-len(f))
	n, ok := new(big.Int).SetString(w+f, 10)
	if !ok {
		return nil, ErrInvalid
	}
	return n, nil
}

func format(n *big.Int) string {
	s := n.String()
	if len(s) < 3 {
		s = strings.Repeat("0", 3-len(s)) + s
	}
	return s[:len(s)-2] + "." + s[len(s)-2:]
}

// Replace computes total-old+next without an int64 or floating point limit.
func Replace(total, old, next string) (string, error) {
	t, err := parse(total)
	if err != nil {
		return "", err
	}
	o, err := parse(old)
	if err != nil {
		return "", err
	}
	n, err := parse(next)
	if err != nil {
		return "", err
	}
	t.Sub(t, o).Add(t, n)
	if t.Sign() < 0 {
		return "", ErrInvalid
	}
	return format(t), nil
}

func Compare(a, b string) (int, error) {
	x, err := parse(a)
	if err != nil {
		return 0, err
	}
	y, err := parse(b)
	if err != nil {
		return 0, err
	}
	return x.Cmp(y), nil
}

func Subtract(a, b string) (string, error) { return Replace(a, b, "0") }
