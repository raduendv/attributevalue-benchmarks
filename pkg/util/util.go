package util

import (
	"pkg/model"
	"time"
)

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

func CreateEvent(name string, value float64, ts time.Time, unit string) model.Event {
	return model.Event{
		API:            Pointer(name),
		RawValue:       Pointer(value),
		Type:           Pointer("AttributeValue"),
		Service:        Pointer("DynamoDB"),
		ClientID:       Pointer("v1"),
		Timestamp:      (*model.MetricTime)(Pointer(ts)),
		Version:        Pointer(1),
		AttemptCount:   Pointer(1),
		AttemptLatency: nil,
		StandardUnit:   Pointer(unit),
	}
}
