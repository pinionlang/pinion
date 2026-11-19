package pinionrt

import (
	"math"
	"math/big"
	"strconv"
	"strings"
)

// ---------- Int / Float arithmetic ----------

func checkFloat(f float64) Value {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		Abort("R0104", "float result is not finite")
	}
	return f
}

func overflow(op string) {
	Abort("R0101", "integer overflow in "+op)
}

// Add is `a + b`.
func Add(a, b Value) Value {
	switch x := a.(type) {
	case int64:
		y := b.(int64)
		r := x + y
		if (r > x) != (y > 0) {
			overflow("+")
		}
		return r
	case float64:
		return checkFloat(x + b.(float64))
	case string:
		return x + b.(string)
	case *Dec:
		return x.Add(b.(*Dec))
	}
	panicInternal("operator + not defined for %s", typeName(a))
	return nil
}

// Sub is `a - b`.
func Sub(a, b Value) Value {
	switch x := a.(type) {
	case int64:
		y := b.(int64)
		r := x - y
		if (r < x) != (y > 0) {
			overflow("-")
		}
		return r
	case float64:
		return checkFloat(x - b.(float64))
	case *Dec:
		return x.Sub(b.(*Dec))
	}
	panicInternal("operator - not defined for %s", typeName(a))
	return nil
}

func mulInt(x, y int64) int64 {
	if x == 0 || y == 0 {
		return 0
	}
	r := x * y
	if r/y != x || (x == -1 && y == math.MinInt64) || (y == -1 && x == math.MinInt64) {
		overflow("*")
	}
	return r
}

// Mul is `a * b`.
func Mul(a, b Value) Value {
	switch x := a.(type) {
	case int64:
		return mulInt(x, b.(int64))
	case float64:
		return checkFloat(x * b.(float64))
	case *Dec:
		return x.Mul(b.(*Dec))
	}
	panicInternal("operator * not defined for %s", typeName(a))
	return nil
}

// Div is `a / b` (Int truncates toward zero).
func Div(a, b Value) Value {
	switch x := a.(type) {
	case int64:
		y := b.(int64)
		if y == 0 {
			Abort("R0103", "integer division by zero")
		}
		if x == math.MinInt64 && y == -1 {
			overflow("/")
		}
		return x / y
	case float64:
		return checkFloat(x / b.(float64))
	}
	panicInternal("operator / not defined for %s", typeName(a))
	return nil
}

// Mod is `a % b`.
func Mod(a, b Value) Value {
	switch x := a.(type) {
	case int64:
		y := b.(int64)
		if y == 0 {
			Abort("R0103", "integer division by zero")
		}
		if y == -1 {
			return int64(0)
		}
		return x % y
	case float64:
		return checkFloat(math.Mod(x, b.(float64)))
	}
	panicInternal("operator %% not defined for %s", typeName(a))
	return nil
}

// Neg is unary minus.
func Neg(a Value) Value {
	switch x := a.(type) {
	case int64:
		if x == math.MinInt64 {
			overflow("unary -")
		}
		return -x
	case float64:
		return -x
	case *Dec:
		return &Dec{u: new(big.Int).Neg(x.u), scale: x.scale}
	}
	panicInternal("unary - not defined for %s", typeName(a))
	return nil
}

// ParseIntLit parses an integer literal (decimal, may contain _).
func ParseIntLit(s string) int64 {
	s = strings.ReplaceAll(s, "_", "")
	n, err := strconv.ParseInt(s, 0, 64)
	if err != nil {
		u, err2 := strconv.ParseUint(s, 0, 64)
		if err2 == nil && u == 1<<63 {
			return math.MinInt64 // only valid under unary minus
		}
		Abort("R0101", "integer literal out of range: "+s)
	}
	return n
}

// FormatFloat renders a Float for display and JSON.
func FormatFloat(f float64) string {
	if f == 0 {
		return "0.0"
	}
	s := strconv.FormatFloat(f, 'g', -1, 64)
	if !strings.ContainsAny(s, ".eEn") {
		s += ".0"
	}
	return s
}

func floatJSON(f float64) string {
	if f == 0 {
		return "0"
	}
	abs := math.Abs(f)
	fmtc := byte('f')
	if abs < 1e-6 || abs >= 1e21 {
		fmtc = 'e'
	}
	return strconv.FormatFloat(f, fmtc, -1, 64)
}

// ---------- Dec ----------

// Dec is an arbitrary-precision decimal: u * 10^-scale.
type Dec struct {
	u     *big.Int
	scale int
}

var bigTen = big.NewInt(10)

func pow10(n int) *big.Int {
	return new(big.Int).Exp(bigTen, big.NewInt(int64(n)), nil)
}

// ParseDec parses "12.30" / "-1" / "0.5". ok=false on bad syntax.
func ParseDec(s string) (*Dec, bool) {
	if s == "" {
		return nil, false
	}
	t := strings.ReplaceAll(s, "_", "")
	neg := false
	if t[0] == '-' || t[0] == '+' {
		neg = t[0] == '-'
		t = t[1:]
	}
	intp, frac, hasDot := strings.Cut(t, ".")
	if intp == "" || (hasDot && frac == "") {
		return nil, false
	}
	for _, c := range intp + frac {
		if c < '0' || c > '9' {
			return nil, false
		}
	}
	u, ok := new(big.Int).SetString(intp+frac, 10)
	if !ok {
		return nil, false
	}
	if neg {
		u.Neg(u)
	}
	return &Dec{u: u, scale: len(frac)}, true
}

// DecLit builds a Dec from literal text.
func DecLit(s string) Value {
	d, ok := ParseDec(s)
	if !ok {
		panicInternal("bad Dec literal %q", s)
	}
	return d
}

func (d *Dec) rescale(s int) *big.Int {
	if s == d.scale {
		return d.u
	}
	return new(big.Int).Mul(d.u, pow10(s-d.scale))
}

// Add returns d + e.
func (d *Dec) Add(e *Dec) *Dec {
	s := max(d.scale, e.scale)
	return &Dec{u: new(big.Int).Add(d.rescale(s), e.rescale(s)), scale: s}
}

// Sub returns d - e.
func (d *Dec) Sub(e *Dec) *Dec {
	s := max(d.scale, e.scale)
	return &Dec{u: new(big.Int).Sub(d.rescale(s), e.rescale(s)), scale: s}
}

// Mul returns d * e.
func (d *Dec) Mul(e *Dec) *Dec {
	return &Dec{u: new(big.Int).Mul(d.u, e.u), scale: d.scale + e.scale}
}

// Cmp compares numerically.
func (d *Dec) Cmp(e *Dec) int {
	s := max(d.scale, e.scale)
	return d.rescale(s).Cmp(e.rescale(s))
}

func (d *Dec) normalized() *Dec {
	u := new(big.Int).Set(d.u)
	s := d.scale
	r := new(big.Int)
	for s > 0 {
		q, m := new(big.Int).QuoRem(u, bigTen, r)
		if m.Sign() != 0 {
			break
		}
		u = q
		s--
	}
	return &Dec{u: u, scale: s}
}

func (d *Dec) String() string {
	neg := d.u.Sign() < 0
	digits := new(big.Int).Abs(d.u).String()
	if d.scale > 0 {
		for len(digits) <= d.scale {
			digits = "0" + digits
		}
		digits = digits[:len(digits)-d.scale] + "." + digits[len(digits)-d.scale:]
	}
	if neg {
		return "-" + digits
	}
	return digits
}

// roundDiv divides n by d (d > 0 or < 0) rounding per mode.
func roundDiv(n, d *big.Int, mode string) *big.Int {
	q, r := new(big.Int).QuoRem(n, d, new(big.Int))
	if r.Sign() == 0 {
		return q
	}
	sign := n.Sign() * d.Sign()
	twice := new(big.Int).Abs(r)
	twice.Lsh(twice, 1)
	c := twice.Cmp(new(big.Int).Abs(d))
	inc := false
	switch mode {
	case "Down":
	case "Floor":
		inc = sign < 0
	case "Ceil":
		inc = sign > 0
	case "HalfUp":
		inc = c >= 0
	default: // HalfEven
		inc = c > 0 || (c == 0 && q.Bit(0) == 1)
	}
	if inc {
		q.Add(q, big.NewInt(int64(sign)))
	}
	return q
}

func roundMode(v Value) string {
	e, ok := v.(*Enum)
	if !ok {
		panicInternal("expected Round, got %s", typeName(v))
	}
	return e.V.Name
}

// DivDec is d.div(e, scale, round).
func (d *Dec) DivDec(e *Dec, scale int, mode string) *Dec {
	if e.u.Sign() == 0 {
		Abort("R0103", "decimal division by zero")
	}
	if scale < 0 {
		Abort("R0102", "negative scale")
	}
	n := new(big.Int).Mul(d.u, pow10(e.scale+scale))
	den := new(big.Int).Mul(e.u, pow10(d.scale))
	return &Dec{u: roundDiv(n, den, mode), scale: scale}
}

// Round rounds to scale digits.
func (d *Dec) Round(scale int, mode string) *Dec {
	if scale < 0 {
		Abort("R0102", "negative scale")
	}
	if scale >= d.scale {
		return &Dec{u: d.rescale(scale), scale: scale}
	}
	return &Dec{u: roundDiv(d.u, pow10(d.scale-scale), mode), scale: scale}
}

func (d *Dec) toFloat() float64 {
	f, _ := new(big.Float).SetPrec(200).Quo(new(big.Float).SetInt(d.u), new(big.Float).SetInt(pow10(d.scale))).Float64()
	return f
}

func (d *Dec) truncInt() int64 {
	q := new(big.Int).Quo(d.u, pow10(d.scale))
	if !q.IsInt64() {
		Abort("R0101", "Dec value out of Int range")
	}
	return q.Int64()
}

func intToDec(i int64) *Dec { return &Dec{u: big.NewInt(i), scale: 0} }

// ---------- parsing ----------

func parseIntStr(s string) Value {
	t := strings.TrimSpace(s)
	if t != s || t == "" {
		return Err(parseErr("InvalidFormat", s))
	}
	for i, c := range t {
		if !(c >= '0' && c <= '9') && !(i == 0 && (c == '-' || c == '+') && len(t) > 1) {
			return Err(parseErr("InvalidFormat", s))
		}
	}
	n, err := strconv.ParseInt(t, 10, 64)
	if err != nil {
		return Err(parseErr("OutOfRange", s))
	}
	return Ok(n)
}

func parseFloatStr(s string) Value {
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		if ne, ok := err.(*strconv.NumError); ok && ne.Err == strconv.ErrRange {
			return Err(parseErr("OutOfRange", s))
		}
		return Err(parseErr("InvalidFormat", s))
	}
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return Err(parseErr("InvalidFormat", s))
	}
	return Ok(f)
}

func parseDecStr(s string) Value {
	d, ok := ParseDec(s)
	if !ok || strings.Contains(s, "_") {
		return Err(parseErr("InvalidFormat", s))
	}
	return Ok(d)
}

func floatToInt(f float64) int64 {
	if f >= 9.223372036854775807e18 || f < -9.223372036854775808e18 {
		Abort("R0101", "Float value out of Int range")
	}
	return int64(f)
}
