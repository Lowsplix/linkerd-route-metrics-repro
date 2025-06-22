module github.com/kostyay/grpc-web-example/time-client

go 1.23

toolchain go1.24.2

require (
	github.com/kostyay/grpc-web-example/time/goclient v0.0.0
	google.golang.org/grpc v1.72.1
)

require (
	github.com/golang/protobuf v1.5.4 // indirect
	golang.org/x/net v0.35.0 // indirect
	golang.org/x/sys v0.30.0 // indirect
	golang.org/x/text v0.22.0 // indirect
	google.golang.org/genproto v0.0.0-20200526211855-cb27e3aa2013 // indirect
	google.golang.org/protobuf v1.36.6 // indirect
)

replace github.com/kostyay/grpc-web-example/time/goclient => ../time/goclient
