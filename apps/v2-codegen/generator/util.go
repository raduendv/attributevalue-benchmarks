package main

import (
	"strconv"
	"unsafe"
)

const (
	stringSize  = unsafe.Sizeof(string(""))
	stringAlign = unsafe.Alignof(string(""))

	intSize  = unsafe.Sizeof(int(0))
	intAlign = unsafe.Alignof(int(0))

	int8Size  = unsafe.Sizeof(int8(0))
	int8Align = unsafe.Alignof(int8(0))

	int16Size  = unsafe.Sizeof(int16(0))
	int16Align = unsafe.Alignof(int16(0))

	int32Size  = unsafe.Sizeof(int32(0))
	int32Align = unsafe.Alignof(int32(0))

	int64Size  = unsafe.Sizeof(int64(0))
	int64Align = unsafe.Alignof(int64(0))

	uintSize  = unsafe.Sizeof(uint(0))
	uintAlign = unsafe.Alignof(uint(0))

	uint8Size  = unsafe.Sizeof(uint8(0))
	uint8Align = unsafe.Alignof(uint8(0))

	uint16Size  = unsafe.Sizeof(uint16(0))
	uint16Align = unsafe.Alignof(uint16(0))

	uint32Size  = unsafe.Sizeof(uint32(0))
	uint32Align = unsafe.Alignof(uint32(0))

	uint64Size  = unsafe.Sizeof(uint64(0))
	uint64Align = unsafe.Alignof(uint64(0))

	float32Size  = unsafe.Sizeof(float32(0))
	float32Align = unsafe.Alignof(float32(0))

	float64Size  = unsafe.Sizeof(float64(0))
	float64Align = unsafe.Alignof(float64(0))

	complex64Size  = unsafe.Sizeof(complex64(0))
	complex64Align = unsafe.Alignof(complex64(0))

	complex128Size  = unsafe.Sizeof(complex128(0))
	complex128Align = unsafe.Alignof(complex128(0))
)

// memcpy copies size bytes from src to dst. It is the caller's responsibility to ensure that dst and src point to valid memory regions of at least size bytes.
func memcpy(dst, src unsafe.Pointer, size uintptr) {
	dstSlice := unsafe.Slice((*byte)(dst), size)
	srcSlice := unsafe.Slice((*byte)(src), size)
	copy(dstSlice, srcSlice)
}

// isLowerCase checks if the given byte represents a lowercase ASCII letter (a-z). It returns true if the byte is a lowercase letter, and false otherwise.
func isLowerCase(c byte) bool {
	return 'a' <= c && c <= 'z'
}

func pointer[T any](v T) *T {
	return &v
}

func unwrap[T any](v *T) T {
	if v == nil {
		return *new(T)
	}

	return *v
}

// firstNonZero returns the first non-zero value from the provided list of values. If all values are zero, it returns the zero value of type T.
func firstNonZero[T comparable](values ...T) T {
	zv := *new(T)

	for _, v := range values {
		if v != zv {
			return v
		}
	}

	return zv
}

func parseIntToInt(s string) int {
	v, _ := strconv.ParseInt(s, 10, strconv.IntSize)
	return int(v)
}

func parseIntToInt8(s string) int8 {
	v, _ := strconv.ParseInt(s, 10, 8)
	return int8(v)
}

func parseIntToInt16(s string) int16 {
	v, _ := strconv.ParseInt(s, 10, 16)
	return int16(v)
}

func parseIntToInt32(s string) int32 {
	v, _ := strconv.ParseInt(s, 10, 32)
	return int32(v)
}

func parseIntToInt64(s string) int64 {
	v, _ := strconv.ParseInt(s, 10, 64)
	return v
}

func parseUintToUint(s string) uint {
	v, _ := strconv.ParseUint(s, 10, strconv.IntSize)
	return uint(v)
}

func parseUintToUint8(s string) uint8 {
	v, _ := strconv.ParseUint(s, 10, 8)
	return uint8(v)
}

func parseUintToUint16(s string) uint16 {
	v, _ := strconv.ParseUint(s, 10, 16)
	return uint16(v)
}

func parseUintToUint32(s string) uint32 {
	v, _ := strconv.ParseUint(s, 10, 32)
	return uint32(v)
}

func parseUintToUint64(s string) uint64 {
	v, _ := strconv.ParseUint(s, 10, 64)
	return v
}

func parseFloatToFloat32(s string) float32 {
	v, _ := strconv.ParseFloat(s, 32)
	return float32(v)
}

func parseFloatToFloat64(s string) float64 {
	v, _ := strconv.ParseFloat(s, 64)
	return v
}
