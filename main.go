package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"strings"

	"github.com/coocood/freecache"
	"google.golang.org/grpc"

	"github.com/n0z0/cachedb/cdc"
	"github.com/n0z0/cachedb/proto/cachepb"
)

var (
	version     = "dev"
	showVersion = flag.Bool("version", false, "Tampilkan versi lalu keluar")
	addrFlag    = flag.String("addr", "127.0.0.1:50051", "gRPC address untuk listen (default: 127.0.0.1:50051)")
	portFlag    = flag.String("port", "", "gRPC port untuk listen (contoh: :50051 atau 50051)")
	ttlFlag     = flag.Int("ttl", defaultTTL, "Default TTL data dalam detik (default: 36000 / 10 jam)")

	// CLI inspect flags
	getKey   = flag.String("get", "", "CLI mode: Ambil nilai key dari server CacheDB")
	delKey   = flag.String("del", "", "CLI mode: Hapus key dari server CacheDB")
	setKey   = flag.String("set", "", "CLI mode: Set key ke server CacheDB")
	setVal   = flag.String("val", "", "CLI mode: Nilai untuk -set")
	getStats = flag.Bool("stats", false, "CLI mode: Tampilkan statistik server CacheDB")
	target   = flag.String("target", "127.0.0.1:50051", "Target address server untuk CLI mode")
)

const (
	cacheSizeBytes   = 100 * 1024 * 1024 // 100MB
	defaultTTL       = 36000             // 10 jam (36000 detik)
	maxValueSizeByte = 64 * 1024         // 64KB
)

type cacheServer struct {
	cache      *freecache.Cache
	defaultTTL int
	cachepb.UnimplementedCacheServer
}

func (s *cacheServer) Get(ctx context.Context, req *cachepb.GetRequest) (*cachepb.GetResponse, error) {
	val, err := s.cache.Get([]byte(req.Key))
	if err != nil {
		return &cachepb.GetResponse{Found: false}, nil
	}
	return &cachepb.GetResponse{
		Value: val,
		Found: true,
	}, nil
}

func (s *cacheServer) Set(ctx context.Context, req *cachepb.SetRequest) (*cachepb.SetResponse, error) {
	if len(req.Value) > maxValueSizeByte {
		return &cachepb.SetResponse{Ok: false}, nil
	}

	ttl := int(req.TtlSeconds)
	if ttl <= 0 {
		ttl = s.defaultTTL
	}

	err := s.cache.Set([]byte(req.Key), req.Value, ttl)
	if err != nil {
		return &cachepb.SetResponse{Ok: false}, nil
	}
	return &cachepb.SetResponse{Ok: true}, nil
}

func (s *cacheServer) Delete(ctx context.Context, req *cachepb.DeleteRequest) (*cachepb.DeleteResponse, error) {
	ok := s.cache.Del([]byte(req.Key))
	return &cachepb.DeleteResponse{Ok: ok}, nil
}

func (s *cacheServer) MGet(ctx context.Context, req *cachepb.MGetRequest) (*cachepb.MGetResponse, error) {
	res := make([]*cachepb.KeyValueItem, len(req.Keys))
	for i, k := range req.Keys {
		val, err := s.cache.Get([]byte(k))
		if err != nil {
			res[i] = &cachepb.KeyValueItem{
				Key:   k,
				Found: false,
			}
		} else {
			res[i] = &cachepb.KeyValueItem{
				Key:   k,
				Value: val,
				Found: true,
			}
		}
	}
	return &cachepb.MGetResponse{Items: res}, nil
}

func (s *cacheServer) MSet(ctx context.Context, req *cachepb.MSetRequest) (*cachepb.MSetResponse, error) {
	var count int32
	for _, item := range req.Items {
		if len(item.Value) > maxValueSizeByte {
			continue
		}
		ttl := int(item.TtlSeconds)
		if ttl <= 0 {
			ttl = s.defaultTTL
		}
		err := s.cache.Set([]byte(item.Key), item.Value, ttl)
		if err == nil {
			count++
		}
	}
	return &cachepb.MSetResponse{Count: count, Ok: true}, nil
}

func (s *cacheServer) GetStats(ctx context.Context, req *cachepb.StatsRequest) (*cachepb.StatsResponse, error) {
	return &cachepb.StatsResponse{
		EntryCount:       s.cache.EntryCount(),
		HitCount:         s.cache.HitCount(),
		MissCount:        s.cache.MissCount(),
		HitRate:          s.cache.HitRate(),
		EvacuateCount:    s.cache.EvacuateCount(),
		ExpiredCount:     s.cache.ExpiredCount(),
		MemoryLimitBytes: cacheSizeBytes,
	}, nil
}

func handleCLIMode() bool {
	if *getKey == "" && *delKey == "" && *setKey == "" && !*getStats {
		return false
	}

	client, conn, err := cdc.Connect(*target)
	if err != nil {
		log.Fatalf("[CLI] Gagal terhubung ke CacheDB %s: %v", *target, err)
	}
	defer conn.Close()

	if *getStats {
		stats, err := cdc.GetStats(client)
		if err != nil {
			log.Fatalf("[CLI] Gagal mengambil stats: %v", err)
		}
		fmt.Printf("=== CacheDB Server Stats (%s) ===\n", *target)
		fmt.Printf("Total Entries : %d\n", stats.EntryCount)
		fmt.Printf("Hit Count     : %d\n", stats.HitCount)
		fmt.Printf("Miss Count    : %d\n", stats.MissCount)
		fmt.Printf("Hit Rate      : %.2f%%\n", stats.HitRate*100)
		fmt.Printf("Evacuate Count: %d\n", stats.EvacuateCount)
		fmt.Printf("Expired Count : %d\n", stats.ExpiredCount)
		fmt.Printf("Memory Limit  : %d MB\n", stats.MemoryLimitBytes/(1024*1024))
		return true
	}

	if *getKey != "" {
		val, err := cdc.Get(*getKey, client)
		if err != nil {
			log.Fatalf("[CLI] Gagal get key: %v", err)
		}
		if val == "" {
			fmt.Printf("Key %q: (not found)\n", *getKey)
		} else {
			fmt.Printf("Key %q: %s\n", *getKey, val)
		}
		return true
	}

	if *setKey != "" {
		err := cdc.Set(*setKey, *setVal, client)
		if err != nil {
			log.Fatalf("[CLI] Gagal set key: %v", err)
		}
		fmt.Printf("Key %q berhasil diset: %s\n", *setKey, *setVal)
		return true
	}

	if *delKey != "" {
		err := cdc.Delete(*delKey, client)
		if err != nil {
			log.Fatalf("[CLI] Gagal delete key: %v", err)
		}
		fmt.Printf("Key %q berhasil dihapus\n", *delKey)
		return true
	}

	return true
}

func main() {
	flag.Parse()
	if *showVersion {
		fmt.Println("cachedb", version)
		return
	}

	if handleCLIMode() {
		return
	}

	listenTarget := *addrFlag
	if *portFlag != "" {
		p := *portFlag
		if strings.HasPrefix(p, ":") {
			listenTarget = "127.0.0.1" + p
		} else if !strings.Contains(p, ":") {
			listenTarget = "127.0.0.1:" + p
		} else {
			listenTarget = p
		}
	}

	fc := freecache.NewCache(cacheSizeBytes)

	lis, err := net.Listen("tcp", listenTarget)
	if err != nil {
		log.Fatalf("listen failed on %s: %v", listenTarget, err)
	}

	s := grpc.NewServer()
	cachepb.RegisterCacheServer(s, &cacheServer{cache: fc, defaultTTL: *ttlFlag})

	log.Printf("[*] CacheDB %s server listening on %s (default TTL: %d detik / %.1f jam)", version, listenTarget, *ttlFlag, float64(*ttlFlag)/3600.0)
	if err := s.Serve(lis); err != nil {
		log.Fatalf("serve: %v", err)
	}
}
