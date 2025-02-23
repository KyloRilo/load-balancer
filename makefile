default:
	go mod tidy
	protoc --go_out=. --go_opt=paths=source_relative     --go-grpc_out=. --go-grpc_opt=paths=source_relative     proto/balancer.proto

run:
	go run .