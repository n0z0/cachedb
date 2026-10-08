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

func SetWithTTL(key, value string, ttlSeconds int32, client cachepb.CacheClient) error {
	_, err := client.Set(context.Background(), &cachepb.SetRequest{
		Key:        key,
		Value:      []byte(value),
		TtlSeconds: ttlSeconds,
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

func Delete(key string, client cachepb.CacheClient) error {
	_, err := client.Delete(context.Background(), &cachepb.DeleteRequest{
		Key: key,
	})
	return err
}

func MSet(items map[string]string, client cachepb.CacheClient) (int32, error) {
	return MSetWithTTL(items, 0, client)
}

func MSetWithTTL(items map[string]string, ttlSeconds int32, client cachepb.CacheClient) (int32, error) {
	reqItems := make([]*cachepb.SetItem, 0, len(items))
	for k, v := range items {
		reqItems = append(reqItems, &cachepb.SetItem{
			Key:        k,
			Value:      []byte(v),
			TtlSeconds: ttlSeconds,
		})
	}
	resp, err := client.MSet(context.Background(), &cachepb.MSetRequest{
		Items: reqItems,
	})
	if err != nil {
		return 0, err
	}
	return resp.Count, nil
}

func MGet(keys []string, client cachepb.CacheClient) (map[string]string, error) {
	resp, err := client.MGet(context.Background(), &cachepb.MGetRequest{
		Keys: keys,
	})
	if err != nil {
		return nil, err
	}
	result := make(map[string]string, len(resp.Items))
	for _, item := range resp.Items {
		if item.Found {
			result[item.Key] = string(item.Value)
		}
	}
	return result, nil
}

func GetStats(client cachepb.CacheClient) (*cachepb.StatsResponse, error) {
	return client.GetStats(context.Background(), &cachepb.StatsRequest{})
}
