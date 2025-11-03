package cdc

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/n0z0/cachedb/proto/cachepb"
	"google.golang.org/grpc"
)

// MockCacheClient implements the cachepb.CacheClient interface for testing
type MockCacheClient struct {
	setFunc    func(ctx context.Context, req *cachepb.SetRequest, opts ...grpc.CallOption) (*cachepb.SetResponse, error)
	getFunc    func(ctx context.Context, req *cachepb.GetRequest, opts ...grpc.CallOption) (*cachepb.GetResponse, error)
	deleteFunc func(ctx context.Context, req *cachepb.DeleteRequest, opts ...grpc.CallOption) (*cachepb.DeleteResponse, error)
	clearFunc  func(ctx context.Context, req interface{}, opts ...grpc.CallOption) (interface{}, error)
}

func (m *MockCacheClient) Set(ctx context.Context, req *cachepb.SetRequest, opts ...grpc.CallOption) (*cachepb.SetResponse, error) {
	if m.setFunc != nil {
		return m.setFunc(ctx, req, opts...)
	}
	return &cachepb.SetResponse{}, nil
}

func (m *MockCacheClient) Get(ctx context.Context, req *cachepb.GetRequest, opts ...grpc.CallOption) (*cachepb.GetResponse, error) {
	if m.getFunc != nil {
		return m.getFunc(ctx, req, opts...)
	}
	return &cachepb.GetResponse{Found: false, Value: []byte{}}, nil
}

func (m *MockCacheClient) Delete(ctx context.Context, req *cachepb.DeleteRequest, opts ...grpc.CallOption) (*cachepb.DeleteResponse, error) {
	if m.deleteFunc != nil {
		return m.deleteFunc(ctx, req, opts...)
	}
	return &cachepb.DeleteResponse{}, nil
}

func (m *MockCacheClient) Clear(ctx context.Context, req interface{}, opts ...grpc.CallOption) (interface{}, error) {
	if m.clearFunc != nil {
		return m.clearFunc(ctx, req, opts...)
	}
	return struct{}{}, nil
}

func TestConnect(t *testing.T) {
	// Test the actual connection - this will fail if no server is running
	// but it's good to have a basic test
	client, _, err := Connect()

	// We expect this to fail in a test environment since there's no gRPC server
	if err != nil {
		// Expected error - no server running
		t.Logf("Connection failed as expected: %v", err)
	} else {
		// If connection succeeds, client should not be nil
		if client == nil {
			t.Error("Client should not be nil if connection succeeds")
		}
		// Clean up by closing the connection
		// Note: We can't directly assert the type here because the client
		// is a cachepb.CacheClient, not a grpc.ClientConn
		// In a real test, you might want to use a mock or interface stub
	}
}

func TestSet(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		value   string
		mockSet func() *MockCacheClient
		wantErr bool
		errMsg  string
	}{
		{
			name:  "successful set",
			key:   "test-key",
			value: "test-value",
			mockSet: func() *MockCacheClient {
				return &MockCacheClient{
					setFunc: func(ctx context.Context, req *cachepb.SetRequest, opts ...grpc.CallOption) (*cachepb.SetResponse, error) {
						return &cachepb.SetResponse{}, nil
					},
				}
			},
			wantErr: false,
		},
		{
			name:  "set error",
			key:   "test-key",
			value: "test-value",
			mockSet: func() *MockCacheClient {
				return &MockCacheClient{
					setFunc: func(ctx context.Context, req *cachepb.SetRequest, opts ...grpc.CallOption) (*cachepb.SetResponse, error) {
						return &cachepb.SetResponse{}, errors.New("set failed")
					},
				}
			},
			wantErr: true,
			errMsg:  "set failed",
		},
		{
			name:    "empty key",
			key:     "",
			value:   "test-value",
			mockSet: func() *MockCacheClient { return &MockCacheClient{} },
			wantErr: false, // This depends on the server-side validation
		},
		{
			name:    "empty value",
			key:     "test-key",
			value:   "",
			mockSet: func() *MockCacheClient { return &MockCacheClient{} },
			wantErr: false, // This depends on the server-side validation
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := tt.mockSet()

			err := Set(tt.key, tt.value, mockClient)

			if tt.wantErr {
				if err == nil {
					t.Error("Expected error but got none")
				} else if tt.errMsg != "" && err.Error() != tt.errMsg {
					t.Errorf("Expected error message '%s' but got '%s'", tt.errMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}
		})
	}
}

func TestGet(t *testing.T) {
	tests := []struct {
		name      string
		key       string
		mockGet   func() *MockCacheClient
		wantValue string
		wantErr   bool
		errMsg    string
		wantFound bool
	}{
		{
			name: "successful get - found",
			key:  "test-key",
			mockGet: func() *MockCacheClient {
				return &MockCacheClient{
					getFunc: func(ctx context.Context, req *cachepb.GetRequest, opts ...grpc.CallOption) (*cachepb.GetResponse, error) {
						return &cachepb.GetResponse{
							Found: true,
							Value: []byte("test-value"),
						}, nil
					},
				}
			},
			wantValue: "test-value",
			wantFound: true,
			wantErr:   false,
		},
		{
			name: "successful get - not found",
			key:  "non-existent-key",
			mockGet: func() *MockCacheClient {
				return &MockCacheClient{
					getFunc: func(ctx context.Context, req *cachepb.GetRequest, opts ...grpc.CallOption) (*cachepb.GetResponse, error) {
						return &cachepb.GetResponse{
							Found: false,
							Value: []byte{},
						}, nil
					},
				}
			},
			wantValue: "",
			wantFound: false,
			wantErr:   false,
		},
		{
			name: "get error",
			key:  "test-key",
			mockGet: func() *MockCacheClient {
				return &MockCacheClient{
					getFunc: func(ctx context.Context, req *cachepb.GetRequest, opts ...grpc.CallOption) (*cachepb.GetResponse, error) {
						return &cachepb.GetResponse{}, errors.New("get failed")
					},
				}
			},
			wantValue: "",
			wantFound: false,
			wantErr:   true,
			errMsg:    "get failed",
		},
		{
			name:      "empty key",
			key:       "",
			mockGet:   func() *MockCacheClient { return &MockCacheClient{} },
			wantValue: "",
			wantFound: false,
			wantErr:   false, // This depends on the server-side validation
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := tt.mockGet()

			value, err := Get(tt.key, mockClient)

			if tt.wantErr {
				if err == nil {
					t.Error("Expected error but got none")
				} else if tt.errMsg != "" && err.Error() != tt.errMsg {
					t.Errorf("Expected error message '%s' but got '%s'", tt.errMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}

			if value != tt.wantValue {
				t.Errorf("Expected value '%s' but got '%s'", tt.wantValue, value)
			}
		})
	}
}

func TestSetAndGetIntegration(t *testing.T) {
	mockClient := &MockCacheClient{
		setFunc: func(ctx context.Context, req *cachepb.SetRequest, opts ...grpc.CallOption) (*cachepb.SetResponse, error) {
			return &cachepb.SetResponse{}, nil
		},
		getFunc: func(ctx context.Context, req *cachepb.GetRequest, opts ...grpc.CallOption) (*cachepb.GetResponse, error) {
			return &cachepb.GetResponse{
				Found: true,
				Value: []byte("integration-test-value"),
			}, nil
		},
	}

	key := "integration-test-key"
	value := "integration-test-value"

	// Set the value
	err := Set(key, value, mockClient)
	if err != nil {
		t.Errorf("Unexpected error during set: %v", err)
	}

	// Get the value
	retrievedValue, err := Get(key, mockClient)
	if err != nil {
		t.Errorf("Unexpected error during get: %v", err)
	}
	if retrievedValue != value {
		t.Errorf("Expected value '%s' but got '%s'", value, retrievedValue)
	}
}

func TestSetAndGetWithSpecialCharacters(t *testing.T) {
	mockClient := &MockCacheClient{
		setFunc: func(ctx context.Context, req *cachepb.SetRequest, opts ...grpc.CallOption) (*cachepb.SetResponse, error) {
			return &cachepb.SetResponse{}, nil
		},
		getFunc: func(ctx context.Context, req *cachepb.GetRequest, opts ...grpc.CallOption) (*cachepb.GetResponse, error) {
			return &cachepb.GetResponse{
				Found: true,
				Value: []byte("value with spaces and special chars: !@#$%^&*()"),
			}, nil
		},
	}

	key := "special@#$%^&*()_+-={}[]|\\:;\"'<>,.?/~`"
	value := "value with spaces and special chars: !@#$%^&*()"

	// Set the value
	err := Set(key, value, mockClient)
	if err != nil {
		t.Errorf("Unexpected error during set: %v", err)
	}

	// Get the value
	retrievedValue, err := Get(key, mockClient)
	if err != nil {
		t.Errorf("Unexpected error during get: %v", err)
	}
	if retrievedValue != value {
		t.Errorf("Expected value '%s' but got '%s'", value, retrievedValue)
	}
}

func TestSetAndGetWithUnicode(t *testing.T) {
	mockClient := &MockCacheClient{
		setFunc: func(ctx context.Context, req *cachepb.SetRequest, opts ...grpc.CallOption) (*cachepb.SetResponse, error) {
			return &cachepb.SetResponse{}, nil
		},
		getFunc: func(ctx context.Context, req *cachepb.GetRequest, opts ...grpc.CallOption) (*cachepb.GetResponse, error) {
			return &cachepb.GetResponse{
				Found: true,
				Value: []byte("unicode-value-测试🚀"),
			}, nil
		},
	}

	key := "unicode-key-测试"
	value := "unicode-value-测试🚀"

	// Set the value
	err := Set(key, value, mockClient)
	if err != nil {
		t.Errorf("Unexpected error during set: %v", err)
	}

	// Get the value
	retrievedValue, err := Get(key, mockClient)
	if err != nil {
		t.Errorf("Unexpected error during get: %v", err)
	}
	if retrievedValue != value {
		t.Errorf("Expected value '%s' but got '%s'", value, retrievedValue)
	}
}

func TestGetNonExistentKey(t *testing.T) {
	mockClient := &MockCacheClient{
		getFunc: func(ctx context.Context, req *cachepb.GetRequest, opts ...grpc.CallOption) (*cachepb.GetResponse, error) {
			return &cachepb.GetResponse{
				Found: false,
				Value: []byte{},
			}, nil
		},
	}

	key := "non-existent-key"

	// Get the value
	value, err := Get(key, mockClient)

	// Should not return an error, but should return empty string
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if value != "" {
		t.Errorf("Expected empty string but got '%s'", value)
	}
}

func TestConcurrentAccess(t *testing.T) {
	mockClient := &MockCacheClient{
		setFunc: func(ctx context.Context, req *cachepb.SetRequest, opts ...grpc.CallOption) (*cachepb.SetResponse, error) {
			// Simulate some processing time
			time.Sleep(1 * time.Millisecond)
			return &cachepb.SetResponse{}, nil
		},
		getFunc: func(ctx context.Context, req *cachepb.GetRequest, opts ...grpc.CallOption) (*cachepb.GetResponse, error) {
			// Simulate some processing time
			time.Sleep(1 * time.Millisecond)
			return &cachepb.GetResponse{
				Found: true,
				Value: []byte("concurrent-value"),
			}, nil
		},
	}

	key := "concurrent-test-key"
	value := "concurrent-value"

	// Test concurrent Set operations
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			err := Set(key, value, mockClient)
			if err != nil {
				t.Errorf("Unexpected error during concurrent set: %v", err)
			}
			done <- true
		}()
	}

	// Wait for all Set operations to complete
	for i := 0; i < 10; i++ {
		<-done
	}

	// Test concurrent Get operations
	for i := 0; i < 10; i++ {
		go func() {
			retrievedValue, err := Get(key, mockClient)
			if err != nil {
				t.Errorf("Unexpected error during concurrent get: %v", err)
			}
			if retrievedValue != value {
				t.Errorf("Expected value '%s' but got '%s'", value, retrievedValue)
			}
			done <- true
		}()
	}

	// Wait for all Get operations to complete
	for i := 0; i < 10; i++ {
		<-done
	}
}
