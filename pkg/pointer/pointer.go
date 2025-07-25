package pointer

import "time"

// String returns a pointer to the provided string value
func String(value string) *string {
	return &value
}

// StringOrDefault returns the pointer if not nil, otherwise the default value
func StringOrDefault(value *string, defaultValue string) *string {
	if value != nil {
		return value
	}
	return &defaultValue
}

// StringIfValid returns a pointer to the value if it's valid, otherwise nil
func StringIfValid(valid bool, value string) *string {
	if valid {
		return &value
	}
	return nil
}

// StringCopy returns a pointer that's a copy of the provided value
func StringCopy(value *string) *string {
	if value == nil {
		return nil
	}

	return String(*value)
}

// Int32 returns a pointer to the provided int32 value
func Int32(value int32) *int32 {
	return &value
}

// Int32OrDefault returns the pointer if not nil, otherwise the default value
func Int32OrDefault(value *int32, defaultValue int32) *int32 {
	if value != nil {
		return value
	}
	return &defaultValue
}

// Int32IfValid returns a pointer to the value if it's valid, otherwise nil
func Int32IfValid(valid bool, value int32) *int32 {
	if valid {
		return &value
	}
	return nil
}

// Int32Copy returns a pointer that's a copy of the provided value
func Int32Copy(value *int32) *int32 {
	if value == nil {
		return nil
	}

	return Int32(*value)
}

// Uint32 returns a pointer to the provided uint32 value
func Uint32(value uint32) *uint32 {
	return &value
}

// Uint32OrDefault returns the pointer if not nil, otherwise the default value
func Uint32OrDefault(value *uint32, defaultValue uint32) *uint32 {
	if value != nil {
		return value
	}
	return &defaultValue
}

// Uint32IfValid returns a pointer to the value if it's valid, otherwise nil
func Uint32IfValid(valid bool, value uint32) *uint32 {
	if valid {
		return &value
	}
	return nil
}

// Uint32Copy returns a pointer that's a copy of the provided value
func Uint32Copy(value *uint32) *uint32 {
	if value == nil {
		return nil
	}

	return Uint32(*value)
}

// Uint64 returns a pointer to the provided uint64 value
func Uint64(value uint64) *uint64 {
	return &value
}

// Uint64OrDefault returns the pointer if not nil, otherwise the default value
func Uint64OrDefault(value *uint64, defaultValue uint64) *uint64 {
	if value != nil {
		return value
	}
	return &defaultValue
}

// Uint64IfValid returns a pointer to the value if it's valid, otherwise nil
func Uint64IfValid(valid bool, value uint64) *uint64 {
	if valid {
		return &value
	}
	return nil
}

// Uint64Copy returns a pointer that's a copy of the provided value
func Uint64Copy(value *uint64) *uint64 {
	if value == nil {
		return nil
	}

	return Uint64(*value)
}

// Float64 returns a pointer to the provided float64 value
func Float64(value float64) *float64 {
	return &value
}

// Float64OrDefault returns the pointer if not nil, otherwise the default value
func Float64OrDefault(value *float64, defaultValue float64) *float64 {
	if value != nil {
		return value
	}
	return &defaultValue
}

// Float64IfValid returns a pointer to the value if it's valid, otherwise nil
func Float64IfValid(valid bool, value float64) *float64 {
	if valid {
		return &value
	}
	return nil
}

// Float64Copy returns a pointer that's a copy of the provided value
func Float64Copy(value *float64) *float64 {
	if value == nil {
		return nil
	}

	return Float64(*value)
}

// Time returns a pointer to the provided time.Time value
func Time(value time.Time) *time.Time {
	return &value
}

// TimeOrDefault returns the pointer if not nil, otherwise the default value
func TimeOrDefault(value *time.Time, defaultValue time.Time) *time.Time {
	if value != nil {
		return value
	}
	return &defaultValue
}

// TimeIfValid returns a pointer to the value if it's valid, otherwise nil
func TimeIfValid(valid bool, value time.Time) *time.Time {
	if valid {
		return &value
	}
	return nil
}

// TimeCopy returns a pointer that's a copy of the provided value
func TimeCopy(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}

	return Time(*value)
}
