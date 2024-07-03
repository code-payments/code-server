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

// Uint8 returns a pointer to the provided uint8 value
func Uint8(value uint8) *uint8 {
	return &value
}

// Uint8OrDefault returns the pointer if not nil, otherwise the default value
func Uint8OrDefault(value *uint8, defaultValue uint8) *uint8 {
	if value != nil {
		return value
	}
	return &defaultValue
}

// Uint8IfValid returns a pointer to the value if it's valid, otherwise nil
func Uint8IfValid(valid bool, value uint8) *uint8 {
	if valid {
		return &value
	}
	return nil
}

// Uint8Copy returns a pointer that's a copy of the provided value
func Uint8Copy(value *uint8) *uint8 {
	if value == nil {
		return nil
	}

	return Uint8(*value)
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
