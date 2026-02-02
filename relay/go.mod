module github.com/goautomatik/relay

go 1.24.0

toolchain go1.24.5

require (
	github.com/goautomatik/core-server v0.0.0
	github.com/kelseyhightower/envconfig v1.4.0
	google.golang.org/grpc v1.78.0
)

require (
	golang.org/x/net v0.47.0 // indirect
	golang.org/x/sys v0.38.0 // indirect
	golang.org/x/text v0.31.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20251029180050-ab9386a59fda // indirect
	google.golang.org/protobuf v1.36.11 // indirect
)

replace github.com/goautomatik/core-server => ../
