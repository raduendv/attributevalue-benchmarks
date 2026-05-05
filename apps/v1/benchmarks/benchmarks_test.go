package benchmarks

import (
	"pkg/model"
	"pkg/util"
	"testing"

	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbattribute"
)

func marshallBench[T any](b *testing.B, v T) {
	for b.Loop() {
		_, err := dynamodbattribute.MarshalMap(v)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func unmarshallBench[T any](b *testing.B, v map[string]*dynamodb.AttributeValue) {
	for b.Loop() {
		var o T
		err := dynamodbattribute.UnmarshalMap(v, &o)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func Benchmark_DynamodbAttribute_MarshalMap_User1KB(b *testing.B) {
	marshallBench[model.User](b, util.ExampleUser1KB)
}

func Benchmark_DynamodbAttribute_UnmarshalMap_User1KB(b *testing.B) {
	unmarshallBench[model.User](b, util.ExampleUser1KBMapV1)
}

func Benchmark_DynamodbAttribute_MarshalMap_User10KB(b *testing.B) {
	marshallBench[model.User](b, util.ExampleUser10KB)
}

func Benchmark_DynamodbAttribute_UnmarshalMap_User10KB(b *testing.B) {
	unmarshallBench[model.User](b, util.ExampleUser10KBMapV1)
}

func Benchmark_DynamodbAttribute_MarshalMap_User100KB(b *testing.B) {
	marshallBench[model.User](b, util.ExampleUser100KB)
}

func Benchmark_DynamodbAttribute_UnmarshalMap_User100KB(b *testing.B) {
	unmarshallBench[model.User](b, util.ExampleUser100KBMapV1)
}

func Benchmark_DynamodbAttribute_MarshalMap_User300KB(b *testing.B) {
	marshallBench[model.User](b, util.ExampleUser300KB)
}

func Benchmark_DynamodbAttribute_UnmarshalMap_User300KB(b *testing.B) {
	unmarshallBench[model.User](b, util.ExampleUser300KBMapV1)
}

func Benchmark_DynamodbAttribute_MarshalMap_String(b *testing.B) {
	marshallBench[util.StringStruct](b, util.ExampleStringStruct)
}

func Benchmark_DynamodbAttribute_UnmarshalMap_String(b *testing.B) {
	unmarshallBench[util.StringStruct](b, util.ExampleStringMapV1)
}

func Benchmark_DynamodbAttribute_MarshalMap_Int(b *testing.B) {
	marshallBench[util.IntStruct](b, util.ExampleIntStruct)
}

func Benchmark_DynamodbAttribute_UnmarshalMap_Int(b *testing.B) {
	unmarshallBench[util.IntStruct](b, util.ExampleIntMapV1)
}

func Benchmark_DynamodbAttribute_MarshalMap_Int8(b *testing.B) {
	marshallBench[util.Int8Struct](b, util.ExampleInt8Struct)
}

func Benchmark_DynamodbAttribute_UnmarshalMap_Int8(b *testing.B) {
	unmarshallBench[util.Int8Struct](b, util.ExampleInt8MapV1)
}

func Benchmark_DynamodbAttribute_MarshalMap_Int16(b *testing.B) {
	marshallBench[util.Int16Struct](b, util.ExampleInt16Struct)
}

func Benchmark_DynamodbAttribute_UnmarshalMap_Int16(b *testing.B) {
	unmarshallBench[util.Int16Struct](b, util.ExampleInt16MapV1)
}

func Benchmark_DynamodbAttribute_MarshalMap_Int32(b *testing.B) {
	marshallBench[util.Int32Struct](b, util.ExampleInt32Struct)
}

func Benchmark_DynamodbAttribute_UnmarshalMap_Int32(b *testing.B) {
	unmarshallBench[util.Int32Struct](b, util.ExampleInt32MapV1)
}

func Benchmark_DynamodbAttribute_MarshalMap_Int64(b *testing.B) {
	marshallBench[util.Int64Struct](b, util.ExampleInt64Struct)
}

func Benchmark_DynamodbAttribute_UnmarshalMap_Int64(b *testing.B) {
	unmarshallBench[util.Int64Struct](b, util.ExampleInt64MapV1)
}

func Benchmark_DynamodbAttribute_MarshalMap_Uint(b *testing.B) {
	marshallBench[util.UintStruct](b, util.ExampleUintStruct)
}

func Benchmark_DynamodbAttribute_UnmarshalMap_Uint(b *testing.B) {
	unmarshallBench[util.UintStruct](b, util.ExampleUintMapV1)
}

func Benchmark_DynamodbAttribute_MarshalMap_Uint8(b *testing.B) {
	marshallBench[util.Uint8Struct](b, util.ExampleUint8Struct)
}

func Benchmark_DynamodbAttribute_UnmarshalMap_Uint8(b *testing.B) {
	unmarshallBench[util.Uint8Struct](b, util.ExampleUint8MapV1)
}

func Benchmark_DynamodbAttribute_MarshalMap_Uint16(b *testing.B) {
	marshallBench[util.Uint16Struct](b, util.ExampleUint16Struct)
}

func Benchmark_DynamodbAttribute_UnmarshalMap_Uint16(b *testing.B) {
	unmarshallBench[util.Uint16Struct](b, util.ExampleUint16MapV1)
}

func Benchmark_DynamodbAttribute_MarshalMap_Uint32(b *testing.B) {
	marshallBench[util.Uint32Struct](b, util.ExampleUint32Struct)
}

func Benchmark_DynamodbAttribute_UnmarshalMap_Uint32(b *testing.B) {
	unmarshallBench[util.Uint32Struct](b, util.ExampleUint32MapV1)
}

func Benchmark_DynamodbAttribute_MarshalMap_Uint64(b *testing.B) {
	marshallBench[util.Uint64Struct](b, util.ExampleUint64Struct)
}

func Benchmark_DynamodbAttribute_UnmarshalMap_Uint64(b *testing.B) {
	unmarshallBench[util.Uint64Struct](b, util.ExampleUint64MapV1)
}
