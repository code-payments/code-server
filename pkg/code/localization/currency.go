package localization

import (
	"golang.org/x/text/language"
	"golang.org/x/text/message"
	"golang.org/x/text/number"

	currency_lib "github.com/code-payments/code-server/pkg/currency"
	"github.com/pkg/errors"

	chatpb "github.com/code-payments/code-protobuf-api/generated/go/chat/v1"
)

var symbolByCode = map[currency_lib.Code]string{
	currency_lib.AED: "د.إ",
	currency_lib.AFN: "؋",
	currency_lib.ALL: "Lek",
	currency_lib.ANG: "ƒ",
	currency_lib.AOA: "Kz",
	currency_lib.ARS: "$",
	currency_lib.AUD: "$",
	currency_lib.AWG: "ƒ",
	currency_lib.AZN: "₼",
	currency_lib.BAM: "KM",
	currency_lib.BDT: "৳",
	currency_lib.BBD: "$",
	currency_lib.BGN: "лв",
	currency_lib.BMD: "$",
	currency_lib.BND: "$",
	currency_lib.BOB: "$b",
	currency_lib.BRL: "R$",
	currency_lib.BSD: "$",
	currency_lib.BWP: "P",
	currency_lib.BYN: "Br",
	currency_lib.BZD: "BZ$",
	currency_lib.CAD: "$",
	currency_lib.CHF: "CHF",
	currency_lib.CLP: "$",
	currency_lib.CNY: "¥",
	currency_lib.COP: "$",
	currency_lib.CRC: "₡",
	currency_lib.CUC: "$",
	currency_lib.CUP: "₱",
	currency_lib.CZK: "Kč",
	currency_lib.DKK: "kr",
	currency_lib.DOP: "RD$",
	currency_lib.EGP: "£",
	currency_lib.ERN: "£",
	currency_lib.EUR: "€",
	currency_lib.FJD: "$",
	currency_lib.FKP: "£",
	currency_lib.GBP: "£",
	currency_lib.GEL: "₾",
	currency_lib.GGP: "£",
	currency_lib.GHS: "¢",
	currency_lib.GIP: "£",
	currency_lib.GNF: "FG",
	currency_lib.GTQ: "Q",
	currency_lib.GYD: "$",
	currency_lib.HKD: "$",
	currency_lib.HNL: "L",
	currency_lib.HRK: "kn",
	currency_lib.HUF: "Ft",
	currency_lib.IDR: "Rp",
	currency_lib.ILS: "₪",
	currency_lib.IMP: "£",
	currency_lib.INR: "₹",
	currency_lib.IRR: "﷼",
	currency_lib.ISK: "kr",
	currency_lib.JEP: "£",
	currency_lib.JMD: "J$",
	currency_lib.JPY: "¥",
	currency_lib.KGS: "лв",
	currency_lib.KHR: "៛",
	currency_lib.KMF: "CF",
	currency_lib.KPW: "₩",
	currency_lib.KRW: "₩",
	currency_lib.KYD: "$",
	currency_lib.KZT: "лв",
	currency_lib.LAK: "₭",
	currency_lib.LBP: "£",
	currency_lib.LKR: "₨",
	currency_lib.LRD: "$",
	currency_lib.LTL: "Lt",
	currency_lib.LVL: "Ls",
	currency_lib.MGA: "Ar",
	currency_lib.MKD: "ден",
	currency_lib.MMK: "K",
	currency_lib.MNT: "₮",
	currency_lib.MUR: "₨",
	currency_lib.MXN: "$",
	currency_lib.MYR: "RM",
	currency_lib.MZN: "MT",
	currency_lib.NAD: "$",
	currency_lib.NGN: "₦",
	currency_lib.NIO: "C$",
	currency_lib.NOK: "kr",
	currency_lib.NPR: "₨",
	currency_lib.NZD: "$",
	currency_lib.OMR: "﷼",
	currency_lib.PAB: "B/.",
	currency_lib.PEN: "S/.",
	currency_lib.PHP: "₱",
	currency_lib.PKR: "₨",
	currency_lib.PLN: "zł",
	currency_lib.PYG: "Gs",
	currency_lib.QAR: "﷼",
	currency_lib.RON: "lei",
	currency_lib.RSD: "Дин.",
	currency_lib.RUB: "₽",
	currency_lib.RWF: "RF",
	currency_lib.SAR: "﷼",
	currency_lib.SBD: "$",
	currency_lib.SCR: "₨",
	currency_lib.SEK: "kr",
	currency_lib.SGD: "$",
	currency_lib.SHP: "£",
	currency_lib.SOS: "S",
	currency_lib.SRD: "$",
	currency_lib.SSP: "£",
	currency_lib.STD: "Db",
	currency_lib.SVC: "$",
	currency_lib.SYP: "£",
	currency_lib.THB: "฿",
	currency_lib.TOP: "T$",
	currency_lib.TRY: "₺",
	currency_lib.TTD: "TT$",
	currency_lib.TWD: "NT$",
	currency_lib.UAH: "₴",
	currency_lib.USD: "$",
	currency_lib.UYU: "$U",
	currency_lib.UZS: "лв",
	currency_lib.VND: "₫",
	currency_lib.XCD: "$",
	currency_lib.YER: "﷼",
	currency_lib.ZAR: "R",
	currency_lib.ZMW: "ZK",
}

// FormatFiat formats a currency amount into a string in the provided locale
func FormatFiat(locale language.Tag, code currency_lib.Code, amount float64, ofKin bool) (string, error) {
	isRtlScript := isRtlScript(locale)

	localizedAmount := func() string {
		decimals := 2
		if code == currency_lib.KIN {
			decimals = 0
			amount = float64(uint64(amount))
		}

		printer := message.NewPrinter(locale)
		amountAsDecimal := number.Decimal(amount, number.Scale(decimals))
		formattedAmount := printer.Sprint(amountAsDecimal)

		symbol, ok := symbolByCode[code]
		if ok {
			if isRtlScript {
				return formattedAmount + symbol
			}
			return symbol + formattedAmount
		}
		return formattedAmount
	}()

	if ofKin {
		suffixKey := CoreOfKin
		if code == currency_lib.KIN {
			suffixKey = CoreKin
		}

		translatedSuffix, err := LocalizeKey(locale, suffixKey)
		if err != nil {
			return "", err
		}

		if isRtlScript {
			return translatedSuffix + " " + localizedAmount, nil
		}
		return localizedAmount + " " + translatedSuffix, nil
	}

	return localizedAmount, nil
}

// LocalizeFiatWithVerb is like FormatFiat, but includes a verb for the interaction
// with the currency amount
func LocalizeFiatWithVerb(locale language.Tag, verb chatpb.ExchangeDataContent_Verb, code currency_lib.Code, amount float64, ofKin bool) (string, error) {
	localizedAmount, err := FormatFiat(locale, code, amount, ofKin)
	if err != nil {
		return "", err
	}

	var key string
	var isAmountBeforeVerb bool
	switch verb {
	case chatpb.ExchangeDataContent_GAVE:
		key = VerbGave
	case chatpb.ExchangeDataContent_RECEIVED:
		key = VerbReceived
	case chatpb.ExchangeDataContent_WITHDREW:
		key = VerbWithdrew
	case chatpb.ExchangeDataContent_DEPOSITED:
		key = VerbDeposited
	case chatpb.ExchangeDataContent_SENT:
		key = VerbSpent
	case chatpb.ExchangeDataContent_RETURNED:
		key = VerbReturned
		isAmountBeforeVerb = true
	case chatpb.ExchangeDataContent_SPENT:
		key = VerbSpent
	case chatpb.ExchangeDataContent_PAID:
		key = VerbPaid
	case chatpb.ExchangeDataContent_PURCHASED:
		key = VerbPurchased
	default:
		return "", errors.Errorf("verb %s is not supported", verb)
	}

	localizedVerbText, err := LocalizeKey(locale, key)
	if err != nil {
		return "", err
	}

	if isRtlScript(locale) {
		isAmountBeforeVerb = !isAmountBeforeVerb
	}

	if isAmountBeforeVerb {
		return localizedAmount + " " + localizedVerbText, nil
	}
	return localizedVerbText + " " + localizedAmount, nil
}
