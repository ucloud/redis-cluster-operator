package utils

// Int32 returns the value of the int32 pointer passed in or
// 0 if the pointer is nil.
func Int32(v *int32) int32 {
	if v != nil {
		return *v
	}
	return 0
}
