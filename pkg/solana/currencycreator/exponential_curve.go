package currencycreator

import (
	"math"
	"math/big"
)

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

// What is the cost to buy tokensToBuy given the currentSupply?
func (curve *ExponentialCurve) CostToBuyTokens(currentSupply, tokensToBuy *big.Float) *big.Float {
	newSupply := new(big.Float).Add(currentSupply, tokensToBuy)
	cs := new(big.Float).Mul(curve.c, currentSupply)
	ns := new(big.Float).Mul(curve.c, newSupply)
	expCs := expBig(cs)
	expNs := expBig(ns)
	abOverC := new(big.Float).Quo(new(big.Float).Mul(curve.a, curve.b), curve.c)
	diff := new(big.Float).Sub(expNs, expCs)
	return new(big.Float).Mul(abOverC, diff)
}

// How much value is received when selling tokensToSell with currentValueLocked in the reserves?
func (curve *ExponentialCurve) ValueFromSellingTokens(currentValue, tokensToSell *big.Float) *big.Float {
	abOverC := new(big.Float).Quo(new(big.Float).Mul(curve.a, curve.b), curve.c)
	cvPlusAbOverC := new(big.Float).Add(currentValue, abOverC)
	cTimesTokensToSell := new(big.Float).Mul(curve.c, tokensToSell)
	exp := expBig(new(big.Float).Neg(cTimesTokensToSell))
	oneMinusExp := new(big.Float).Sub(big.NewFloat(1.0), exp)
	return new(big.Float).Mul(cvPlusAbOverC, oneMinusExp)
}

// How many tokens will be bought for a value given the currentSupply?
func (curve *ExponentialCurve) TokensBoughtForValue(currentSupply, value *big.Float) *big.Float {
	abOverC := new(big.Float).Quo(new(big.Float).Mul(curve.a, curve.b), curve.c)
	expCs := expBig(new(big.Float).Mul(curve.c, currentSupply))
	term := new(big.Float).Add(new(big.Float).Quo(value, abOverC), expCs)
	lnTerm := logBig(term)
	result := new(big.Float).Quo(lnTerm, curve.c)
	return new(big.Float).Sub(result, currentSupply)
}

// How many tokens should be exchanged for a value given the currentValue?
func (curve *ExponentialCurve) TokensForValueExchange(currentValue, value *big.Float) *big.Float {
	abOverC := new(big.Float).Quo(new(big.Float).Mul(curve.a, curve.b), curve.c)
	newValue := new(big.Float).Sub(currentValue, value)
	currentValueOverAbOverC := new(big.Float).Quo(currentValue, abOverC)
	newValueOverAbOverC := new(big.Float).Quo(newValue, abOverC)
	lnTerm1 := logBig(new(big.Float).Add(big.NewFloat(1.0), currentValueOverAbOverC))
	lnTerm2 := logBig(new(big.Float).Add(big.NewFloat(1.0), newValueOverAbOverC))
	diffLnTerms := new(big.Float).Sub(lnTerm1, lnTerm2)
	return new(big.Float).Quo(diffLnTerms, curve.c)
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
