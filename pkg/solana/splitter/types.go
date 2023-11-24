package splitter_token

type DataVersion uint8

const (
	UnknownDataVersion DataVersion = iota
	DataVersion1
)

func putDataVersion(dst []byte, v DataVersion, offset *int) {
	putUint8(dst, uint8(v), offset)
}

func getDataVersion(src []byte, dst *DataVersion, offset *int) {
	var v uint8
	getUint8(src, &v, offset)
	*dst = DataVersion(v)
}
