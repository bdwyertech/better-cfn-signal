module better-cfn-signal

go 1.23

require (
	github.com/aws/aws-sdk-go-v2 v1.41.0
	github.com/aws/aws-sdk-go-v2/credentials v1.19.6
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.18.16
	github.com/aws/aws-sdk-go-v2/service/cloudformation v1.67.1
	github.com/aws/aws-sdk-go-v2/service/ec2 v1.257.2
	github.com/aws/smithy-go v1.24.0
	github.com/mattn/go-isatty v0.0.20
	github.com/sirupsen/logrus v1.9.3
)

require (
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.4.16 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.7.16 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.13.4 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.13.16 // indirect
	golang.org/x/sys v0.30.0 // indirect
)
