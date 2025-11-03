package cdc

import (
	"context"

	"github.com/n0z0/cachedb/proto/cachepb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func Connect() (cachepb.CacheClient, *grpc.ClientConn, error) {
	conn, err := grpc.NewClient("ipv4:127.0.0.1:50051", grpc.WithTransportCredentials(insecure.NewCredentials()))
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
