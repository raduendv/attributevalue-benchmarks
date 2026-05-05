package benchmarks

import (
	"pkg/util"
	"testing"

	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

var ExampleUser1KB = GenerateUser(util.Size1KB)
var ExampleUser10KB = GenerateUser(util.Size10KB)
var ExampleUser100KB = GenerateUser(util.Size100KB)
var ExampleUser300KB = GenerateUser(util.Size300KB)

var (
	_, ExampleUser1KBMapV2   = util.MustMarshalV1V2(ExampleUser1KB)
	_, ExampleUser10KBMapV2  = util.MustMarshalV1V2(ExampleUser10KB)
	_, ExampleUser100KBMapV2 = util.MustMarshalV1V2(ExampleUser100KB)
	_, ExampleUser300KBMapV2 = util.MustMarshalV1V2(ExampleUser300KB)
)

func marshallBench[T any](b *testing.B, v T) {
	for b.Loop() {
		_, err := attributevalue.MarshalMap(v)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func unmarshallBench[T any](b *testing.B, v map[string]types.AttributeValue) {
	for b.Loop() {
		var o T
		err := attributevalue.UnmarshalMap(v, &o)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func Benchmark_DynamodbAttribute_MarshalMap_User1KB(b *testing.B) {
	marshallBench[User](b, ExampleUser1KB)
}

func Benchmark_DynamodbAttribute_UnmarshalMap_User1KB(b *testing.B) {
	unmarshallBench[User](b, ExampleUser1KBMapV2)
}

func Benchmark_DynamodbAttribute_MarshalMap_User10KB(b *testing.B) {
	marshallBench[User](b, ExampleUser10KB)
}

func Benchmark_DynamodbAttribute_UnmarshalMap_User10KB(b *testing.B) {
	unmarshallBench[User](b, ExampleUser10KBMapV2)
}

func Benchmark_DynamodbAttribute_MarshalMap_User100KB(b *testing.B) {
	marshallBench[User](b, ExampleUser100KB)
}

func Benchmark_DynamodbAttribute_UnmarshalMap_User100KB(b *testing.B) {
	unmarshallBench[User](b, ExampleUser100KBMapV2)
}

func Benchmark_DynamodbAttribute_MarshalMap_User300KB(b *testing.B) {
	marshallBench[User](b, ExampleUser300KB)
}

func Benchmark_DynamodbAttribute_UnmarshalMap_User300KB(b *testing.B) {
	unmarshallBench[User](b, ExampleUser300KBMapV2)
}

func Benchmark_DynamodbAttribute_MarshalMap_String(b *testing.B) {
	marshallBench[util.StringStruct](b, util.ExampleStringStruct)
}

func Benchmark_DynamodbAttribute_UnmarshalMap_String(b *testing.B) {
	unmarshallBench[util.StringStruct](b, util.ExampleStringMapV2)
}

func Benchmark_DynamodbAttribute_MarshalMap_Int(b *testing.B) {
	marshallBench[util.IntStruct](b, util.ExampleIntStruct)
}

func Benchmark_DynamodbAttribute_UnmarshalMap_Int(b *testing.B) {
	unmarshallBench[util.IntStruct](b, util.ExampleIntMapV2)
}

func Benchmark_DynamodbAttribute_MarshalMap_Int8(b *testing.B) {
	marshallBench[util.Int8Struct](b, util.ExampleInt8Struct)
}

func Benchmark_DynamodbAttribute_UnmarshalMap_Int8(b *testing.B) {
	unmarshallBench[util.Int8Struct](b, util.ExampleInt8MapV2)
}

func Benchmark_DynamodbAttribute_MarshalMap_Int16(b *testing.B) {
	marshallBench[util.Int16Struct](b, util.ExampleInt16Struct)
}

func Benchmark_DynamodbAttribute_UnmarshalMap_Int16(b *testing.B) {
	unmarshallBench[util.Int16Struct](b, util.ExampleInt16MapV2)
}

func Benchmark_DynamodbAttribute_MarshalMap_Int32(b *testing.B) {
	marshallBench[util.Int32Struct](b, util.ExampleInt32Struct)
}

func Benchmark_DynamodbAttribute_UnmarshalMap_Int32(b *testing.B) {
	unmarshallBench[util.Int32Struct](b, util.ExampleInt32MapV2)
}

func Benchmark_DynamodbAttribute_MarshalMap_Int64(b *testing.B) {
	marshallBench[util.Int64Struct](b, util.ExampleInt64Struct)
}

func Benchmark_DynamodbAttribute_UnmarshalMap_Int64(b *testing.B) {
	unmarshallBench[util.Int64Struct](b, util.ExampleInt64MapV2)
}

func Benchmark_DynamodbAttribute_MarshalMap_Uint(b *testing.B) {
	marshallBench[util.UintStruct](b, util.ExampleUintStruct)
}

func Benchmark_DynamodbAttribute_UnmarshalMap_Uint(b *testing.B) {
	unmarshallBench[util.UintStruct](b, util.ExampleUintMapV2)
}

func Benchmark_DynamodbAttribute_MarshalMap_Uint8(b *testing.B) {
	marshallBench[util.Uint8Struct](b, util.ExampleUint8Struct)
}

func Benchmark_DynamodbAttribute_UnmarshalMap_Uint8(b *testing.B) {
	unmarshallBench[util.Uint8Struct](b, util.ExampleUint8MapV2)
}

func Benchmark_DynamodbAttribute_MarshalMap_Uint16(b *testing.B) {
	marshallBench[util.Uint16Struct](b, util.ExampleUint16Struct)
}

func Benchmark_DynamodbAttribute_UnmarshalMap_Uint16(b *testing.B) {
	unmarshallBench[util.Uint16Struct](b, util.ExampleUint16MapV2)
}

func Benchmark_DynamodbAttribute_MarshalMap_Uint32(b *testing.B) {
	marshallBench[util.Uint32Struct](b, util.ExampleUint32Struct)
}

func Benchmark_DynamodbAttribute_UnmarshalMap_Uint32(b *testing.B) {
	unmarshallBench[util.Uint32Struct](b, util.ExampleUint32MapV2)
}

func Benchmark_DynamodbAttribute_MarshalMap_Uint64(b *testing.B) {
	marshallBench[util.Uint64Struct](b, util.ExampleUint64Struct)
}

func Benchmark_DynamodbAttribute_UnmarshalMap_Uint64(b *testing.B) {
	unmarshallBench[util.Uint64Struct](b, util.ExampleUint64MapV2)
}
