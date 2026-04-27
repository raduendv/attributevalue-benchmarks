module main

go 1.26.0

replace pkg => ./../../pkg

require (
	github.com/aws/aws-sdk-go v1.55.8
	pkg v0.0.0-00010101000000-000000000000
)

require (
	github.com/aws/aws-sdk-go-v2 v1.41.6 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.4.22 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.7.22 // indirect
	github.com/aws/aws-sdk-go-v2/service/cloudwatch v1.56.2 // indirect
	github.com/aws/aws-sdk-go-v2/service/dynamodb v1.57.2 // indirect
	github.com/aws/smithy-go v1.25.0 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
)
