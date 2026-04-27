package util

func Pointer[T any](v T) *T {
	return &v
}

func Unwrap[T any](v *T) T {
	if v == nil {
		return *new(T)
	}

	return *v
}

type numeric interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 |
		~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 | ~uintptr |
		~float32 | ~float64 |
		~string
}

func Min[T numeric](a ...T) T {
	if len(a) == 0 {
		return *new(T)
	}

	min := a[0]

	for _, v := range a {
		if v < min {
			min = v
		}
	}

	return min
}
