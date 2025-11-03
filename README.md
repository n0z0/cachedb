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

## Release tag

```sh
git tag v0.1.2
git push origin --tags
go list -m github.com/n0z0/cachedb@v0.1.2
```

## Usage

### Install Module

```sh
go get github.com/n0z0/cachedb
```

### Basic Usage

```go
package main

import (
    "fmt"
    "log"
    
    "github.com/n0z0/cachedb/cdc"
    "github.com/n0z0/cachedb/proto/cachepb"
)

func main() {
    // Connect to cache server
    client, conn, err := cdc.Connect()
    if err != nil {
        log.Fatalf("Failed to connect: %v", err)
    }
    defer conn.Close()
    
    // Set a key-value pair
    err = cdc.Set("user:1", "John Doe", client)
    if err != nil {
        log.Fatalf("Failed to set: %v", err)
    }
    
    // Get a value by key
    value, err := cdc.Get("user:1", client)
    if err != nil {
        log.Fatalf("Failed to get: %v", err)
    }
    
    if value != "" {
        fmt.Printf("Value: %s\n", value)
    } else {
        fmt.Println("Key not found")
    }
}
```

### API Reference

#### `Connect() (cachepb.CacheClient, error)`

Establishes a connection to the cache server.

- Returns: gRPC client connection or error

#### `Set(key, value string, client cachepb.CacheClient) error`

Sets a key-value pair in the cache.

- `key`: The cache key
- `value`: The value to store
- `client`: The gRPC client connection
- Returns: error or nil if successful

#### `Get(key string, client cachepb.CacheClient) (string, error)`

Retrieves a value by key from the cache.

- `key`: The cache key to retrieve
- `client`: The gRPC client connection
- Returns: The value as string and error (or empty string if key not found)
