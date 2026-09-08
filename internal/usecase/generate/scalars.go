package generate

import (
	"encoding/json"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

var formats = map[string]bool{"date": true, "time": true, "date-time": true, "duration": true, "uuid": true, "email": true, "hostname": true, "ipv4": true, "ipv6": true, "uri": true}

func jsonNumber(n float64) string { return strconv.FormatFloat(n, 'f', -1, 64) }
func rat(x any) *big.Rat {
	if n, ok := x.(json.Number); ok {
		r, ok := new(big.Rat).SetString(string(n))
		if ok {
			return r
		}
	}
	return nil
}
func floor(r *big.Rat) *big.Int {
	q, m := new(big.Int), new(big.Int)
	q.QuoRem(r.Num(), r.Denom(), m)
	if r.Sign() < 0 && m.Sign() != 0 {
		q.Sub(q, big.NewInt(1))
	}
	return q
}
func ceil(r *big.Rat) *big.Int {
	v := floor(r)
	if new(big.Rat).SetInt(v).Cmp(r) < 0 {
		v.Add(v, big.NewInt(1))
	}
	return v
}
func (g *generation) number(n *node, integer bool) (any, error) {
	m := n.raw
	lo, hi := big.NewRat(-1000, 1), big.NewRat(1000, 1)
	hasLo, hasHi := false, false
	exclusiveLo, exclusiveHi := false, false
	if r := rat(m["minimum"]); r != nil {
		lo = r
		hasLo = true
	}
	if r := rat(m["maximum"]); r != nil {
		hi = r
		hasHi = true
	}
	if r := rat(m["exclusiveMinimum"]); r != nil && (!hasLo || r.Cmp(lo) >= 0) {
		lo = r
		hasLo = true
		exclusiveLo = true
	}
	if r := rat(m["exclusiveMaximum"]); r != nil && (!hasHi || r.Cmp(hi) <= 0) {
		hi = r
		hasHi = true
		exclusiveHi = true
	}
	if hasLo && !hasHi && lo.Cmp(hi) > 0 {
		hi = new(big.Rat).Add(lo, big.NewRat(1000, 1))
	}
	if hasHi && !hasLo && hi.Cmp(lo) < 0 {
		lo = new(big.Rat).Sub(hi, big.NewRat(1000, 1))
	}
	step := rat(m["multipleOf"])
	if step != nil && step.Sign() <= 0 {
		return nil, invalid("schema.not_generatable", n.path, "multipleOf must be positive")
	}
	if integer {
		if step == nil {
			step = big.NewRat(1, 1)
		} else {
			step = new(big.Rat).SetInt(step.Num())
		}
	}
	if step == nil {
		// A finite decimal grid fine enough to include finite input bounds.
		scale := 3
		for _, x := range []any{m["minimum"], m["maximum"], m["exclusiveMinimum"], m["exclusiveMaximum"]} {
			if r := rat(x); r != nil {
				d := len(r.Denom().String()) + 1
				if d > scale {
					scale = d
				}
			}
		}
		if scale > 128 {
			scale = 128
		}
		step = new(big.Rat).SetFrac(big.NewInt(1), new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(scale)), nil))
	}
	lowRatio, highRatio := new(big.Rat).Quo(lo, step), new(big.Rat).Quo(hi, step)
	low, high := ceil(lowRatio), floor(highRatio)
	if exclusiveLo && new(big.Rat).SetInt(low).Cmp(lowRatio) == 0 {
		low.Add(low, big.NewInt(1))
	}
	if exclusiveHi && new(big.Rat).SetInt(high).Cmp(highRatio) == 0 {
		high.Sub(high, big.NewInt(1))
	}
	if low.Cmp(high) > 0 {
		return nil, invalid("schema.not_generatable", n.path, "No number satisfies the bounds")
	}
	span := new(big.Int).Add(new(big.Int).Sub(high, low), big.NewInt(1))
	offset := new(big.Int)
	bits := span.BitLen()
	buf := make([]byte, (bits+7)/8)
	for attempt := 0; attempt < g.plan.limits.Attempts; attempt++ {
		for i := range buf {
			buf[i] = byte(g.random.Uint64())
		}
		if bits%8 != 0 {
			buf[0] &= byte((1 << uint(bits%8)) - 1)
		}
		offset.SetBytes(buf)
		if offset.Cmp(span) < 0 {
			break
		}
		offset.SetInt64(0)
	}
	value := new(big.Rat).Mul(new(big.Rat).SetInt(new(big.Int).Add(low, offset)), step)
	text := value.FloatString(128)
	text = strings.TrimRight(strings.TrimRight(text, "0"), ".")
	if text == "-0" {
		text = "0"
	}
	if err := g.charge(len(text)); err != nil {
		return nil, err
	}
	return json.Number(text), nil
}
func (g *generation) text(n *node) (any, error) {
	low, high := integer(n.raw, "minLength", 0), integer(n.raw, "maxLength", max(12, integer(n.raw, "minLength", 0)))
	if low > high || low > g.plan.limits.StringLength {
		return nil, invalid("schema.not_generatable", n.path, "String bounds exceed limits")
	}
	high = min(high, g.plan.limits.StringLength)
	var s string
	if f, _ := n.raw["format"].(string); f != "" {
		s = g.formatted(f)
	} else if n.pattern != nil {
		if _, explicit := n.raw["maxLength"]; !explicit {
			high = g.plan.limits.StringLength
		}
		if n.pattern.min > high || n.pattern.max < low {
			return nil, invalid("schema.not_generatable", n.path, "Pattern conflicts with string length")
		}
		s = n.pattern.generate(g.random)
	} else {
		length := low + g.random.IntN(high-low+1)
		if err := g.charge(length*6 + 2); err != nil {
			return nil, err
		}
		b := make([]byte, length)
		alphabet := "abcdefghijklmnopqrstuvwxyz0123456789"
		for i := range b {
			b[i] = alphabet[g.random.IntN(len(alphabet))]
		}
		return string(b), nil
	}
	if err := g.charge(len(s)*6 + 2); err != nil {
		return nil, err
	}
	if utf8.RuneCountInString(s) > g.plan.limits.StringLength {
		return nil, invalid("schema.limit_exceeded", n.path, "Generated string exceeds limit")
	}
	return s, nil
}
func (g *generation) formatted(f string) string {
	t := time.Unix(1577836800+g.random.Int64N(315619200), 0).UTC()
	switch f {
	case "date":
		return t.Format("2006-01-02")
	case "time":
		return t.Format("15:04:05Z07:00")
	case "date-time":
		return t.Format(time.RFC3339)
	case "duration":
		return fmt.Sprintf("PT%dS", g.random.IntN(86400))
	case "uuid":
		b := make([]byte, 16)
		for i := range b {
			b[i] = byte(g.random.Uint64())
		}
		b[6] = b[6]&15 | 64
		b[8] = b[8]&63 | 128
		return fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:])
	case "email":
		return fmt.Sprintf("user%d@example.com", g.random.Uint64())
	case "hostname":
		return fmt.Sprintf("host%d.example.com", g.random.Uint64())
	case "uri":
		return fmt.Sprintf("https://example.com/items/%d", g.random.Uint64())
	case "ipv4":
		return fmt.Sprintf("192.0.2.%d", g.random.IntN(254)+1)
	case "ipv6":
		return fmt.Sprintf("2001:db8::%x", g.random.IntN(65535)+1)
	}
	return ""
}
