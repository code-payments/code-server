package common

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/pkg/errors"

	"github.com/code-payments/code-server/pkg/code/config"
	"github.com/code-payments/code-server/pkg/usdc"
)

var (
	CoreMintAccount, _    = NewAccountFromPublicKeyBytes(config.CoreMintPublicKeyBytes)
	CoreMintQuarksPerUnit = uint64(config.CoreMintQuarksPerUnit)
	CoreMintSymbol        = config.CoreMintSymbol
	CoreMintDecimals      = config.CoreMintDecimals

	UsdcMintAccount, _ = NewAccountFromPublicKeyBytes(usdc.TokenMint)
)

func FromCoreMintQuarks(quarks uint64) uint64 {
	return quarks / CoreMintQuarksPerUnit
}

func ToCoreMintQuarks(units uint64) uint64 {
	return units * CoreMintQuarksPerUnit
}

// todo: this needs tests
func StrToQuarks(val string) (int64, error) {
	parts := strings.Split(val, ".")
	if len(parts) > 2 {
		return 0, errors.New("invalid value")
	}

	if len(parts[0]) > 19-CoreMintDecimals {
		return 0, errors.New("value cannot be represented")
	}

	wholeUnits, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, err
	}

	var quarks uint64
	if len(parts) == 2 {
		if len(parts[1]) > CoreMintDecimals {
			return 0, errors.New("value cannot be represented")
		}

		padded := fmt.Sprintf("%s%s", parts[1], strings.Repeat("0", CoreMintDecimals-len(parts[1])))
		quarks, err = strconv.ParseUint(padded, 10, 64)
		if err != nil {
			return 0, errors.Wrap(err, "invalid decimal component")
		}
	}

	return int64(wholeUnits)*int64(CoreMintQuarksPerUnit) + int64(quarks), nil
}
