package generate

import (
	"fmt"
	"math/rand/v2"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

type atom struct {
	choices  []string
	min, max int
}
type pattern struct {
	atoms    []atom
	min, max int
	re       *regexp.Regexp
}

func parsePattern(s string, limit int) (*pattern, error) {
	bad := func() (*pattern, error) { return nil, fmt.Errorf("Pattern is outside the bounded V1 subset") }
	if !strings.HasPrefix(s, "^") || !strings.HasSuffix(s, "$") {
		return bad()
	}
	body := s[1 : len(s)-1]
	p := &pattern{}
	for len(body) > 0 {
		a := atom{min: 1, max: 1}
		switch body[0] {
		case '\\':
			if len(body) < 2 {
				return bad()
			}
			if body[1] == 'd' {
				a.choices = strings.Split("0123456789", "")
			} else if strings.ContainsRune(`.-_()[]{}?+*^$|\`, rune(body[1])) {
				a.choices = []string{body[1:2]}
			} else {
				return bad()
			}
			body = body[2:]
		case '[':
			end := strings.IndexByte(body, ']')
			if end < 2 {
				return bad()
			}
			// Escapes inside classes are outside this subset, including a range endpoint.
			if strings.ContainsRune(body[1:end], '\\') {
				return bad()
			}
			r := []rune(body[1:end])
			for i := 0; i < len(r); i++ {
				if r[i] == '^' || r[i] == '\\' || r[i] == '[' {
					return bad()
				}
				if i+2 < len(r) && r[i+1] == '-' {
					if r[i] > r[i+2] || r[i+2]-r[i] > 256 {
						return bad()
					}
					for c := r[i]; c <= r[i+2]; c++ {
						a.choices = append(a.choices, string(c))
					}
					i += 2
				} else {
					a.choices = append(a.choices, string(r[i]))
				}
			}
			body = body[end+1:]
		case '(':
			end := strings.IndexByte(body, ')')
			if end < 1 {
				return bad()
			}
			for _, x := range strings.Split(body[1:end], "|") {
				if x == "" || strings.ContainsAny(x, `[](){}?+*.^$\`) {
					return bad()
				}
				a.choices = append(a.choices, x)
			}
			body = body[end+1:]
		default:
			r, n := utf8.DecodeRuneInString(body)
			if strings.ContainsRune(`.*+?{}()|^$]`, r) {
				return bad()
			}
			a.choices = []string{string(r)}
			body = body[n:]
		}
		if len(body) > 0 && body[0] == '?' {
			a.min = 0
			body = body[1:]
		} else if len(body) > 0 && body[0] == '{' {
			end := strings.IndexByte(body, '}')
			if end < 2 {
				return bad()
			}
			parts := strings.Split(body[1:end], ",")
			for _, part := range parts {
				if part == "" || (len(part) > 1 && part[0] == '0') {
					return bad()
				}
				for _, c := range part {
					if c < '0' || c > '9' {
						return bad()
					}
				}
			}
			if len(parts) > 2 {
				return bad()
			}
			x, err := strconv.Atoi(parts[0])
			if err != nil || x < 0 || x > limit {
				return bad()
			}
			a.min = x
			a.max = x
			if len(parts) == 2 {
				x, err = strconv.Atoi(parts[1])
				if err != nil || x < a.min || x > limit {
					return bad()
				}
				a.max = x
			}
			body = body[end+1:]
		}
		low, high := limit, 0
		for _, c := range a.choices {
			n := utf8.RuneCountInString(c)
			if n < low {
				low = n
			}
			if n > high {
				high = n
			}
		}
		p.min += a.min * low
		p.max += a.max * high
		if p.max > limit {
			return bad()
		}
		p.atoms = append(p.atoms, a)
	}
	re, err := regexp.Compile(s)
	if err != nil {
		return bad()
	}
	p.re = re
	return p, nil
}
func (p *pattern) generate(r *rand.Rand) string {
	var b strings.Builder
	for _, a := range p.atoms {
		count := a.min + r.IntN(a.max-a.min+1)
		for range count {
			b.WriteString(a.choices[r.IntN(len(a.choices))])
		}
	}
	return b.String()
}
