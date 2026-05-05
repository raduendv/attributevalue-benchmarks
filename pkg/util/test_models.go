package util

// !IMPORTANT!
// This is written here to be able to use util methods without causing a circular reference

import (
	"pkg/model"
	"time"

	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	v2types "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbattribute"
)

type TestStruct[T any] struct {
	model.ID

	Value            T     `dynamodbav:"value"`
	Pointer          *T    `dynamodbav:"pointer"`
	DoublePointer    **T   `dynamodbav:"double_pointer"`
	TriplePointer    ***T  `dynamodbav:"triple_pointer"`
	QuadruplePointer ****T `dynamodbav:"quadruple_pointer"`

	EmbeddedStruct[T]
}

type EmbeddedStruct[T any] struct {
	EmbeddedValue            T     `dynamodbav:"embedded_value"`
	EmbeddedPointer          *T    `dynamodbav:"embedded_pointer"`
	EmbeddedDoublePointer    **T   `dynamodbav:"embedded_double_pointer"`
	EmbeddedTriplePointer    ***T  `dynamodbav:"embedded_triple_pointer"`
	EmbeddedQuadruplePointer ****T `dynamodbav:"embedded_quadruple_pointer"`
}

// string like
type StringStruct TestStruct[string]
type ByteStruct TestStruct[byte]
type RuneStruct TestStruct[rune]

// ints
type IntStruct TestStruct[int]
type Int8Struct TestStruct[int8]
type Int16Struct TestStruct[int16]
type Int32Struct TestStruct[int32]
type Int64Struct TestStruct[int64]

// uints
type UintStruct TestStruct[uint]
type Uint8Struct TestStruct[uint8]
type Uint16Struct TestStruct[uint16]
type Uint32Struct TestStruct[uint32]
type Uint64Struct TestStruct[uint64]

// floats
type Float32Struct TestStruct[float32]
type Float64Struct TestStruct[float64]

// complexes
type Complex64Struct TestStruct[complex64]
type Complex128Struct TestStruct[complex128]

// other
type BoolStruct TestStruct[bool]
type TimeStruct TestStruct[time.Time]

// lists
type StringListStruct TestStruct[[]string]
type IntListStruct TestStruct[[]int]
type UintListStruct TestStruct[[]uint]
type FloatListStruct TestStruct[[]float64]
type ComplexListStruct TestStruct[[]complex128]
type BoolListStruct TestStruct[[]bool]
type TimeListStruct TestStruct[[]time.Time]

// maps
type StringMapStruct TestStruct[map[string]string]
type IntMapStruct TestStruct[map[string]int]
type UintMapStruct TestStruct[map[string]uint]
type FloatMapStruct TestStruct[map[string]float64]
type ComplexMapStruct TestStruct[map[string]complex128]
type BoolMapStruct TestStruct[map[string]bool]
type TimeMapStruct TestStruct[map[string]time.Time]

var ExampleStringStruct = StringStruct{
	ID:               model.ID{"test-string"},
	Value:            "Hello, World!",
	Pointer:          Pointer("Hello, World!"),
	DoublePointer:    Pointer(Pointer("Hello, World!")),
	TriplePointer:    Pointer(Pointer(Pointer("Hello, World!"))),
	QuadruplePointer: Pointer(Pointer(Pointer(Pointer("Hello, World!")))),
	EmbeddedStruct: EmbeddedStruct[string]{
		EmbeddedValue:            "Hello, World!",
		EmbeddedPointer:          Pointer("Hello, World!"),
		EmbeddedDoublePointer:    Pointer(Pointer("Hello, World!")),
		EmbeddedTriplePointer:    Pointer(Pointer(Pointer("Hello, World!"))),
		EmbeddedQuadruplePointer: Pointer(Pointer(Pointer(Pointer("Hello, World!")))),
	},
}

var ExampleIntStruct = IntStruct{
	ID:               model.ID{"test-int"},
	Value:            42,
	Pointer:          Pointer(42),
	DoublePointer:    Pointer(Pointer(42)),
	TriplePointer:    Pointer(Pointer(Pointer(42))),
	QuadruplePointer: Pointer(Pointer(Pointer(Pointer(42)))),
	EmbeddedStruct: EmbeddedStruct[int]{
		EmbeddedValue:            42,
		EmbeddedPointer:          Pointer(42),
		EmbeddedDoublePointer:    Pointer(Pointer(42)),
		EmbeddedTriplePointer:    Pointer(Pointer(Pointer(42))),
		EmbeddedQuadruplePointer: Pointer(Pointer(Pointer(Pointer(42)))),
	},
}

var ExampleInt8Struct = Int8Struct{
	ID:               model.ID{"test-int8"},
	Value:            int8(42),
	Pointer:          Pointer(int8(42)),
	DoublePointer:    Pointer(Pointer(int8(42))),
	TriplePointer:    Pointer(Pointer(Pointer(int8(42)))),
	QuadruplePointer: Pointer(Pointer(Pointer(Pointer(int8(42))))),
	EmbeddedStruct: EmbeddedStruct[int8]{
		EmbeddedValue:            int8(42),
		EmbeddedPointer:          Pointer(int8(42)),
		EmbeddedDoublePointer:    Pointer(Pointer(int8(42))),
		EmbeddedTriplePointer:    Pointer(Pointer(Pointer(int8(42)))),
		EmbeddedQuadruplePointer: Pointer(Pointer(Pointer(Pointer(int8(42))))),
	},
}

var ExampleInt16Struct = Int16Struct{
	ID:               model.ID{"test-int"},
	Value:            int16(42),
	Pointer:          Pointer(int16(42)),
	DoublePointer:    Pointer(Pointer(int16(42))),
	TriplePointer:    Pointer(Pointer(Pointer(int16(42)))),
	QuadruplePointer: Pointer(Pointer(Pointer(Pointer(int16(42))))),
	EmbeddedStruct: EmbeddedStruct[int16]{
		EmbeddedValue:            int16(42),
		EmbeddedPointer:          Pointer(int16(42)),
		EmbeddedDoublePointer:    Pointer(Pointer(int16(42))),
		EmbeddedTriplePointer:    Pointer(Pointer(Pointer(int16(42)))),
		EmbeddedQuadruplePointer: Pointer(Pointer(Pointer(Pointer(int16(42))))),
	},
}

var ExampleInt32Struct = Int32Struct{
	ID:               model.ID{"test-int"},
	Value:            int32(42),
	Pointer:          Pointer(int32(42)),
	DoublePointer:    Pointer(Pointer(int32(42))),
	TriplePointer:    Pointer(Pointer(Pointer(int32(42)))),
	QuadruplePointer: Pointer(Pointer(Pointer(Pointer(int32(42))))),
	EmbeddedStruct: EmbeddedStruct[int32]{
		EmbeddedValue:            int32(42),
		EmbeddedPointer:          Pointer(int32(42)),
		EmbeddedDoublePointer:    Pointer(Pointer(int32(42))),
		EmbeddedTriplePointer:    Pointer(Pointer(Pointer(int32(42)))),
		EmbeddedQuadruplePointer: Pointer(Pointer(Pointer(Pointer(int32(42))))),
	},
}

var ExampleInt64Struct = Int64Struct{
	ID:               model.ID{"test-int"},
	Value:            int64(42),
	Pointer:          Pointer(int64(42)),
	DoublePointer:    Pointer(Pointer(int64(42))),
	TriplePointer:    Pointer(Pointer(Pointer(int64(42)))),
	QuadruplePointer: Pointer(Pointer(Pointer(Pointer(int64(42))))),
	EmbeddedStruct: EmbeddedStruct[int64]{
		EmbeddedValue:            int64(42),
		EmbeddedPointer:          Pointer(int64(42)),
		EmbeddedDoublePointer:    Pointer(Pointer(int64(42))),
		EmbeddedTriplePointer:    Pointer(Pointer(Pointer(int64(42)))),
		EmbeddedQuadruplePointer: Pointer(Pointer(Pointer(Pointer(int64(42))))),
	},
}

var ExampleUintStruct = UintStruct{
	ID:               model.ID{"test-uint"},
	Value:            42,
	Pointer:          Pointer(uint(42)),
	DoublePointer:    Pointer(Pointer(uint(42))),
	TriplePointer:    Pointer(Pointer(Pointer(uint(42)))),
	QuadruplePointer: Pointer(Pointer(Pointer(Pointer(uint(42))))),
	EmbeddedStruct: EmbeddedStruct[uint]{
		EmbeddedValue:            42,
		EmbeddedPointer:          Pointer(uint(42)),
		EmbeddedDoublePointer:    Pointer(Pointer(uint(42))),
		EmbeddedTriplePointer:    Pointer(Pointer(Pointer(uint(42)))),
		EmbeddedQuadruplePointer: Pointer(Pointer(Pointer(Pointer(uint(42))))),
	},
}

var ExampleUint8Struct = Uint8Struct{
	ID:               model.ID{"test-int8"},
	Value:            uint8(42),
	Pointer:          Pointer(uint8(42)),
	DoublePointer:    Pointer(Pointer(uint8(42))),
	TriplePointer:    Pointer(Pointer(Pointer(uint8(42)))),
	QuadruplePointer: Pointer(Pointer(Pointer(Pointer(uint8(42))))),
	EmbeddedStruct: EmbeddedStruct[uint8]{
		EmbeddedValue:            uint8(42),
		EmbeddedPointer:          Pointer(uint8(42)),
		EmbeddedDoublePointer:    Pointer(Pointer(uint8(42))),
		EmbeddedTriplePointer:    Pointer(Pointer(Pointer(uint8(42)))),
		EmbeddedQuadruplePointer: Pointer(Pointer(Pointer(Pointer(uint8(42))))),
	},
}

var ExampleUint16Struct = Uint16Struct{
	ID:               model.ID{"test-int"},
	Value:            uint16(42),
	Pointer:          Pointer(uint16(42)),
	DoublePointer:    Pointer(Pointer(uint16(42))),
	TriplePointer:    Pointer(Pointer(Pointer(uint16(42)))),
	QuadruplePointer: Pointer(Pointer(Pointer(Pointer(uint16(42))))),
	EmbeddedStruct: EmbeddedStruct[uint16]{
		EmbeddedValue:            uint16(42),
		EmbeddedPointer:          Pointer(uint16(42)),
		EmbeddedDoublePointer:    Pointer(Pointer(uint16(42))),
		EmbeddedTriplePointer:    Pointer(Pointer(Pointer(uint16(42)))),
		EmbeddedQuadruplePointer: Pointer(Pointer(Pointer(Pointer(uint16(42))))),
	},
}

var ExampleUint32Struct = Uint32Struct{
	ID:               model.ID{"test-int"},
	Value:            uint32(42),
	Pointer:          Pointer(uint32(42)),
	DoublePointer:    Pointer(Pointer(uint32(42))),
	TriplePointer:    Pointer(Pointer(Pointer(uint32(42)))),
	QuadruplePointer: Pointer(Pointer(Pointer(Pointer(uint32(42))))),
	EmbeddedStruct: EmbeddedStruct[uint32]{
		EmbeddedValue:            uint32(42),
		EmbeddedPointer:          Pointer(uint32(42)),
		EmbeddedDoublePointer:    Pointer(Pointer(uint32(42))),
		EmbeddedTriplePointer:    Pointer(Pointer(Pointer(uint32(42)))),
		EmbeddedQuadruplePointer: Pointer(Pointer(Pointer(Pointer(uint32(42))))),
	},
}

var ExampleUint64Struct = Uint64Struct{
	ID:               model.ID{"test-int"},
	Value:            uint64(42),
	Pointer:          Pointer(uint64(42)),
	DoublePointer:    Pointer(Pointer(uint64(42))),
	TriplePointer:    Pointer(Pointer(Pointer(uint64(42)))),
	QuadruplePointer: Pointer(Pointer(Pointer(Pointer(uint64(42))))),
	EmbeddedStruct: EmbeddedStruct[uint64]{
		EmbeddedValue:            uint64(42),
		EmbeddedPointer:          Pointer(uint64(42)),
		EmbeddedDoublePointer:    Pointer(Pointer(uint64(42))),
		EmbeddedTriplePointer:    Pointer(Pointer(Pointer(uint64(42)))),
		EmbeddedQuadruplePointer: Pointer(Pointer(Pointer(Pointer(uint64(42))))),
	},
}

var ExampleUser1KB = GenerateUser(Size1KB)
var ExampleUser10KB = GenerateUser(Size10KB)
var ExampleUser100KB = GenerateUser(Size100KB)
var ExampleUser300KB = GenerateUser(Size300KB)

var (
	ExampleUser1KBMapV1, ExampleUser1KBMapV2     = MustMarshalV1V2(ExampleUser1KB)
	ExampleUser10KBMapV1, ExampleUser10KBMapV2   = MustMarshalV1V2(ExampleUser10KB)
	ExampleUser100KBMapV1, ExampleUser100KBMapV2 = MustMarshalV1V2(ExampleUser100KB)
	ExampleUser300KBMapV1, ExampleUser300KBMapV2 = MustMarshalV1V2(ExampleUser300KB)
)

func MustMarshalV1V2[T any](v T) (map[string]*dynamodb.AttributeValue, map[string]v2types.AttributeValue) {
	v1Map, err := dynamodbattribute.MarshalMap(v)
	if err != nil {
		panic(err)
	}

	v2Map, err := attributevalue.MarshalMap(v)
	if err != nil {
		panic(err)
	}

	return v1Map, v2Map
}

var (
	ExampleStringMapV1, ExampleStringMapV2 = MustMarshalV1V2(ExampleStringStruct)
	ExampleIntMapV1, ExampleIntMapV2       = MustMarshalV1V2(ExampleIntStruct)
	ExampleInt8MapV1, ExampleInt8MapV2     = MustMarshalV1V2(ExampleInt8Struct)
	ExampleInt16MapV1, ExampleInt16MapV2   = MustMarshalV1V2(ExampleInt16Struct)
	ExampleInt32MapV1, ExampleInt32MapV2   = MustMarshalV1V2(ExampleInt32Struct)
	ExampleInt64MapV1, ExampleInt64MapV2   = MustMarshalV1V2(ExampleInt64Struct)
	ExampleUintMapV1, ExampleUintMapV2     = MustMarshalV1V2(ExampleUintStruct)
	ExampleUint8MapV1, ExampleUint8MapV2   = MustMarshalV1V2(ExampleUint8Struct)
	ExampleUint16MapV1, ExampleUint16MapV2 = MustMarshalV1V2(ExampleUint16Struct)
	ExampleUint32MapV1, ExampleUint32MapV2 = MustMarshalV1V2(ExampleUint32Struct)
	ExampleUint64MapV1, ExampleUint64MapV2 = MustMarshalV1V2(ExampleUint64Struct)
)
