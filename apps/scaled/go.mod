module github.com/t0gun/spacescale/apps/scaled

go 1.26

replace github.com/t0gun/spacescale/packages/proto-go => ../../packages/proto-go

require (
	github.com/t0gun/spacescale/packages/proto-go v0.0.0-00010101000000-000000000000
	google.golang.org/grpc v1.79.2
)

require (
	golang.org/x/net v0.48.0 // indirect
	golang.org/x/sys v0.39.0 // indirect
	golang.org/x/text v0.33.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20251202230838-ff82c1b0f217 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
)
