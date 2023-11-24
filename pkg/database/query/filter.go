package query

type Filter struct {
	Value uint64
	Valid bool
}

func NewFilter(value uint64) Filter {
	return Filter{
		Value: value,
		Valid: true,
	}
}

func (f *Filter) IsValid() bool {
	return f.Valid
}

func (f *Filter) Set(value uint64) {
	f.Value = value
	f.Valid = true
}
