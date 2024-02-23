package localization

import "golang.org/x/text/language"

func isRtlScript(t language.Tag) bool {
	script, _ := t.Script()
	switch script.String() {
	case
		"Adlm",
		"Arab",
		"Aran",
		"Hebr",
		"Mand",
		"Mend",
		"Nkoo",
		"Rohg",
		"Samr",
		"Syrc",
		"Syre",
		"Syrj",
		"Syrn",
		"Thaa":
		return true
	}
	return false
}

func isDefaultLocale(locale language.Tag) bool {
	return locale.String() == defaultLocale.String()
}
