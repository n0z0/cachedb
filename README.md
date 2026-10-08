# cachedb

Key Value on Memory with Configurable TTL (Default: 10 Hours / 36,000 Seconds)

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

## Instalasi & Upgrade Otomatis

### Windows (PowerShell)
Jalankan perintah berikut untuk mengunduh binary rilis terbaru dan otomatis mendaftarkannya ke PATH pengguna:
```powershell
irm https://raw.githubusercontent.com/n0z0/cachedb/main/install.ps1 | iex
```

### Linux (Bash)
Jalankan perintah berikut di terminal:
```bash
curl -fsSL https://raw.githubusercontent.com/n0z0/cachedb/main/install.sh | bash
```

---

## Menjalankan Server

Setelah terpasang, jalankan server CacheDB di terminal mana saja:
```sh
# Default port :50051 dengan TTL 10 jam (36000 detik)
cachedb

# Mengatur custom TTL (contoh: 1 jam = 3600 detik)
cachedb -ttl 3600

# Menggunakan custom port dan TTL 10 jam
cachedb -port :50052 -ttl 36000
```

---

## Release Otomatis

Rilis dibuat otomatis oleh [GitHub Actions](.github/workflows/release.yml) setiap kali ada push ke branch `main` pada file source code Go atau proto:
- Versi patch dinaikkan secara otomatis dari tag terakhir (misal `v0.1.6` -> `v0.1.7`).
- Binary langsung siap pakai (`cachedb_windows_amd64.exe` dan `cachedb_linux_amd64`) serta arsip bundel di-upload langsung ke halaman Releases.

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
)

func main() {
 // Connect to cache server
 client, conn, err := cdc.Connect("127.0.0.1:50051")
 if err != nil {
  log.Fatalf("Failed to connect: %v", err)
 }
 defer conn.Close()

 // Set a key-value pair
 err = cdc.Set("10.14.203.14", "8765", client)
 if err != nil {
  log.Fatalf("Failed to set: %v", err)
 }

 // Get a value by key
 value, err := cdc.Get("10.14.203.14", client)
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

#### `Connect(address string) (cachepb.CacheClient, error)`

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
