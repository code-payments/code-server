package currencycreator

import (
	"math"
	"math/big"
)

// Note: Generated with Grok 4 based on curve.rs, and not 100% accurate with on-chain program

const (
	DefaultCurveDecimals = 18

	DefaultCurveAString     = "11400230149967394933471" // 11400.230149967394933471
	DefaultCurveBString     = "877175273521"            // 0.000000877175273521
	DefaultCurveScaleString = "1000000000000000000"     // 10^18

	defaultCurvePrec = 128
)

type ExponentialCurve struct {
	a *big.Float
	b *big.Float
	c *big.Float
}

func (curve *ExponentialCurve) SpotPriceAtSupply(currentSupply *big.Float) *big.Float {
	cTimesS := new(big.Float).Mul(curve.c, currentSupply)
	exp := expBig(cTimesS)
	return new(big.Float).Mul(new(big.Float).Mul(curve.a, curve.b), exp)
}

func (curve *ExponentialCurve) TokensToValue(currentSupply, tokens *big.Float) *big.Float {
	newSupply := new(big.Float).Add(currentSupply, tokens)
	cs := new(big.Float).Mul(curve.c, currentSupply)
	ns := new(big.Float).Mul(curve.c, newSupply)
	expCS := expBig(cs)
	expNS := expBig(ns)
	abOverC := new(big.Float).Quo(new(big.Float).Mul(curve.a, curve.b), curve.c)
	diff := new(big.Float).Sub(expNS, expCS)
	return new(big.Float).Mul(abOverC, diff)
}

func (curve *ExponentialCurve) ValueToTokens(currentSupply, value *big.Float) *big.Float {
	abOverC := new(big.Float).Quo(new(big.Float).Mul(curve.a, curve.b), curve.c)
	expCS := expBig(new(big.Float).Mul(curve.c, currentSupply))
	term := new(big.Float).Add(new(big.Float).Quo(value, abOverC), expCS)
	lnTerm := logBig(term)
	result := new(big.Float).Quo(lnTerm, curve.c)
	return new(big.Float).Sub(result, currentSupply)
}

func DefaultExponentialCurve() *ExponentialCurve {
	scale, ok := new(big.Float).SetPrec(defaultCurvePrec).SetString(DefaultCurveScaleString)
	if !ok {
		panic("Invalid scale string")
	}

	aInt, _ := new(big.Int).SetString(DefaultCurveAString, 10)
	bInt, _ := new(big.Int).SetString(DefaultCurveBString, 10)

	a := new(big.Float).Quo(new(big.Float).SetPrec(defaultCurvePrec).SetInt(aInt), scale)
	b := new(big.Float).Quo(new(big.Float).SetPrec(defaultCurvePrec).SetInt(bInt), scale)
	c := new(big.Float).Copy(b)

	return &ExponentialCurve{a: a, b: b, c: c}
}

func expBig(x *big.Float) *big.Float {
	prec := x.Prec()
	result := big.NewFloat(1).SetPrec(prec)
	term := big.NewFloat(1).SetPrec(prec)
	for i := 1; i < 1000; i++ {
		term = term.Mul(term, x)
		term = term.Quo(term, big.NewFloat(float64(i)))
		old := new(big.Float).Copy(result)
		result = result.Add(result, term)
		if term.Cmp(new(big.Float).SetFloat64(0)) == 0 {
			break
		}
		if old.Cmp(result) == 0 {
			break
		}
	}
	return result
}

func logBig(y *big.Float) *big.Float {
	if y.Sign() <= 0 {
		panic("log of non-positive number")
	}
	yf, _ := y.Float64()
	z := big.NewFloat(math.Log(yf)).SetPrec(y.Prec())
	for range 50 {
		expz := expBig(z)
		adjustment := new(big.Float).Quo(y, expz)
		z = z.Add(z, adjustment)
		z = z.Sub(z, big.NewFloat(1))
	}
	return z
}
