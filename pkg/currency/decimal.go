package currency

func GetDecimals(code Code) int {
	switch code {
	case AFN,
		ALL,
		BIF,
		CLP,
		COP,
		DJF,
		GNF,
		IQD,
		IDR,
		IRR,
		ISK,
		JPY,
		KMF,
		KPW,
		KRW,
		LAK,
		LBP,
		MGA,
		MMK,
		MRU,
		PYG,
		RSD,
		RWF,
		SLL,
		SOS,
		SYP,
		TZS,
		UGX,
		UYU,
		VND,
		VUV,
		XAF,
		XOF,
		XPF,
		YER:
		return 0
	case BHD,
		JOD,
		KWD,
		LYD,
		OMR,
		TND:
		return 3
	}
	return 2
}
