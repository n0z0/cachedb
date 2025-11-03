# cachedb

Key Value on Memory with TTL 5 Minutes

## Protocol Buffer

Install:

1. [protoc](https://github.com/protocolbuffers/protobuf/releases)
2. Golang Plugin:

   ```sh
   go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
   go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
   ```

3. Compile proto:

   ```sh
   protoc --go_out=. --go-grpc_out=. cache.proto
   ```
