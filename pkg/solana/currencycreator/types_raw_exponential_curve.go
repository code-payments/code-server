package currencycreator

import (
	"fmt"
	"math/big"
)

const (
	RawExponentialCurveDecimals = 18

	defaultCurveAString     = "11400230149967394933471" // 11400.230149967394933471
	defaultCurveBString     = "877175273521"            // 0.000000877175273521
	defaultCurveScaleString = "1000000000000000000"     // 10^18
)

var (
	DefaultCurveA [24]byte
	DefaultCurveB [24]byte
	DefaultCurveC [24]byte
)

const (
	RawExponentialCurveSize = (24 + // a
		24 + // b
		24) // c
)

func init() {
	aInt, _ := new(big.Int).SetString(defaultCurveAString, 10)
	bInt, _ := new(big.Int).SetString(defaultCurveBString, 10)

	aInt.FillBytes(DefaultCurveA[:])
	DefaultCurveA = [24]byte(bigToLittleEndian(DefaultCurveA[:]))

	bInt.FillBytes(DefaultCurveB[:])
	DefaultCurveB = [24]byte(bigToLittleEndian(DefaultCurveB[:]))

	DefaultCurveC = DefaultCurveB
}

type RawExponentialCurve struct {
	A [24]byte
	B [24]byte
	C [24]byte
}

func DefaultRawExponentialCurve() RawExponentialCurve {
	return RawExponentialCurve{
		A: DefaultCurveA,
		B: DefaultCurveB,
		C: DefaultCurveC,
	}
}

func (obj *RawExponentialCurve) Unmarshal(data []byte) error {
	if len(data) < RawExponentialCurveSize {
		return ErrInvalidAccountData
	}

	copy(obj.A[:], data[0:24])
	copy(obj.B[:], data[24:48])
	copy(obj.C[:], data[48:72])

	return nil
}

func (obj *RawExponentialCurve) ToExponentialCurve() *ExponentialCurve {
	scale, ok := new(big.Float).SetPrec(defaultExponentialCurvePrec).SetString("1000000000000000000") // 10^18
	if !ok {
		panic("Invalid scale string")
	}

	aInt := new(big.Int).SetBytes(littleToBigEndian(obj.A[:]))
	bInt := new(big.Int).SetBytes(littleToBigEndian(obj.B[:]))
	cInt := new(big.Int).SetBytes(littleToBigEndian(obj.C[:]))

	a := new(big.Float).Quo(new(big.Float).SetPrec(defaultExponentialCurvePrec).SetInt(aInt), scale)
	b := new(big.Float).Quo(new(big.Float).SetPrec(defaultExponentialCurvePrec).SetInt(bInt), scale)
	c := new(big.Float).Quo(new(big.Float).SetPrec(defaultExponentialCurvePrec).SetInt(cInt), scale)

	return &ExponentialCurve{a: a, b: b, c: c}
}

func (obj *RawExponentialCurve) String() string {
	exponentialCurve := obj.ToExponentialCurve()
	return fmt.Sprintf(
		"RawExponentialCurve{a=%s,b=%s,c=%s}",
		exponentialCurve.a.Text('f', RawExponentialCurveDecimals),
		exponentialCurve.b.Text('f', RawExponentialCurveDecimals),
		exponentialCurve.c.Text('f', RawExponentialCurveDecimals),
	)
}

func putRawExponentialCurve(dst []byte, v RawExponentialCurve, offset *int) {
	copy(dst[*offset:*offset+24], v.A[:])
	copy(dst[*offset+24:*offset+48], v.B[:])
	copy(dst[*offset+48:*offset+72], v.C[:])
	*offset += RawExponentialCurveSize
}
func getRawExponentialCurve(src []byte, dst *RawExponentialCurve, offset *int) error {
	err := dst.Unmarshal(src[*offset:])
	if err != nil {
		return err
	}
	*offset += RawExponentialCurveSize
	return nil
}
