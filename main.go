package main

import (
	"context"
	"log"
	"net"

	"github.com/coocood/freecache"
	"google.golang.org/grpc"

	"github.com/n0z0/cachedb/proto/cachepb"
)

const (
	cacheSizeBytes   = 100 * 1024 * 1024 // 100MB
	defaultTTL       = 300               // 5 menit
	maxValueSizeByte = 64 * 1024         // 64KB
)

type cacheServer struct {
	cache *freecache.Cache
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
	// cek ukuran value
	if len(req.Value) > maxValueSizeByte {
		// kita tolak aja
		return &cachepb.SetResponse{Ok: false}, nil
	}

	ttl := int(req.TtlSeconds)
	if ttl <= 0 {
		ttl = defaultTTL
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

func main() {
	fc := freecache.NewCache(cacheSizeBytes)

	lis, err := net.Listen("tcp", "127.0.0.1:50051")
	if err != nil {
		log.Fatalf("listen: %v", err)
	}

	s := grpc.NewServer()
	cachepb.RegisterCacheServer(s, &cacheServer{cache: fc})

	log.Println("CacheDB server on 127.0.0.1:50051")
	if err := s.Serve(lis); err != nil {
		log.Fatalf("serve: %v", err)
	}
}
