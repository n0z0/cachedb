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

Setelah terpasang, jalankan server CacheDB di terminal mana saja. Secara default, CacheDB mengikat diri ke **`127.0.0.1:50051` (Localhost)** untuk keamanan maksimal agar tidak dapat diakses dari jaringan luar/LAN tanpa konfigurasi rumit:

```sh
# Default: listen di 127.0.0.1:50051 dengan TTL 10 jam (36000 detik)
cachedb

# Mengatur custom TTL (contoh: 1 jam = 3600 detik)
cachedb -ttl 3600

# Menggunakan custom port di localhost (contoh: 127.0.0.1:50052)
cachedb -port :50052 -ttl 36000

# Eksplisit listen address (misal jika ingin listen di IP interface tertentu)
cachedb -addr 127.0.0.1:50051
```

---

## Mode CLI / Observability & Threat Intelligence

Binary `cachedb` juga dapat digunakan sebagai tool CLI untuk menginspeksi, memonitor metrik, melihat intelijen ancaman penyerang, atau menguji nilai cache langsung dari terminal tanpa perlu menjalankan program terpisah:

```sh
# 1. Melihat statistik server (Entry count, Hit/Miss, Hit Rate %, Memory limit)
cachedb -stats

# 2. Live Active Threat Registry (Melihat tabel live semua IP penyerang aktif di memory)
cachedb -actors

# 3. Attacker Threat Dossier (Profil ancaman lengkap untuk sebuah IP)
cachedb -actor 192.168.1.150

# 4. Firewall Blocklist Exporter (Ekspor daftar IP HIGH/CRITICAL untuk iptables/Windows Firewall)
cachedb -blocklist

# 5. Mengambil nilai suatu key
cachedb -get 192.168.1.150

# 6. Menulis / mengupdate key
cachedb -set testkey -val "hello-world"

# 7. Menghapus key
cachedb -del testkey

# Target ke host/port tertentu (default: 127.0.0.1:50051)
cachedb -target 127.0.0.1:50052 -actors
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

	// 1. Set a single key-value pair
	err = cdc.Set("10.14.203.14", "8765", client)
	if err != nil {
		log.Fatalf("Failed to set: %v", err)
	}

	// 2. Get a value by key
	value, err := cdc.Get("10.14.203.14", client)
	if err != nil {
		log.Fatalf("Failed to get: %v", err)
	}
	if value != "" {
		fmt.Printf("Value: %s\n", value)
	}

	// 3. Batch MSet (Mengirim banyak key sekaligus dalam 1 RPC call)
	items := map[string]string{
		"actor:os:10.14.203.14":      "Linux",
		"actor:scanner:10.14.203.14": "Nmap",
		"actor:risk:10.14.203.14":    "85",
	}
	count, _ := cdc.MSet(items, client)
	fmt.Printf("Disimpan %d item sekaligus\n", count)

	// 4. Batch MGet (Mengambil banyak key sekaligus)
	results, _ := cdc.MGet([]string{"actor:os:10.14.203.14", "actor:risk:10.14.203.14"}, client)
	for k, v := range results {
		fmt.Printf("%s = %s\n", k, v)
	}

	// 5. Cek server stats
	stats, _ := cdc.GetStats(client)
	fmt.Printf("Total entries: %d, Hit Rate: %.2f%%\n", stats.EntryCount, stats.HitRate*100)
}
```

### API Reference (`cdc` package)

- `Connect(address string) (cachepb.CacheClient, *grpc.ClientConn, error)`: Membuka koneksi gRPC ke CacheDB.
- `Set(key, value string, client cachepb.CacheClient) error`: Menyimpan key-value dengan default server TTL.
- `SetWithTTL(key, value string, ttlSeconds int32, client cachepb.CacheClient) error`: Menyimpan key-value dengan custom TTL.
- `Get(key string, client cachepb.CacheClient) (string, error)`: Mengambil value string berdasarkan key.
- `Delete(key string, client cachepb.CacheClient) error`: Menghapus key dari cache.
- `MSet(items map[string]string, client cachepb.CacheClient) (int32, error)`: Batch set banyak key sekaligus.
- `MSetWithTTL(items map[string]string, ttlSeconds int32, client cachepb.CacheClient) (int32, error)`: Batch set dengan custom TTL.
- `MGet(keys []string, client cachepb.CacheClient) (map[string]string, error)`: Batch get banyak key sekaligus.
- `GetStats(client cachepb.CacheClient) (*cachepb.StatsResponse, error)`: Mengambil metrik performa CacheDB.
