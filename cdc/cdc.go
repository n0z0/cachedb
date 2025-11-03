package cdc

import (
	"context"

	"github.com/n0z0/cachedb/proto/cachepb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func Connect(address string) (cachepb.CacheClient, *grpc.ClientConn, error) {
	// Use a standard host:port target; the "ipv4:" prefix is not a valid gRPC target
	conn, err := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, nil, err
	}
	client := cachepb.NewCacheClient(conn)
	return client, conn, nil
}
func Set(key, value string, client cachepb.CacheClient) error {
	_, err := client.Set(context.Background(), &cachepb.SetRequest{
		Key:   key,
		Value: []byte(value),
	})
	return err
}
func Get(key string, client cachepb.CacheClient) (string, error) {
	resp, err := client.Get(context.Background(), &cachepb.GetRequest{
		Key: key,
	})
	if err != nil {
		return "", err
	}
	if !resp.Found {
		return "", nil
	}
	return string(resp.Value), nil
}
