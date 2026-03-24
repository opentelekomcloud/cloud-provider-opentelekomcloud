package identity

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/opentelekomcloud/cloud-provider-opentelekomcloud/pkg/config"
)

func newMockAKSKServer(t *testing.T, authHeaders *[]string, mu *sync.Mutex) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()

	// Version discovery endpoint
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			// Capture Authorization header for non-root requests
			if authHeaders != nil {
				mu.Lock()
				*authHeaders = append(*authHeaders, r.Header.Get("Authorization"))
				mu.Unlock()
			}
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"versions": map[string]interface{}{
				"values": []map[string]interface{}{
					{
						"id":     "v3.0",
						"status": "stable",
						"links": []map[string]interface{}{
							{"rel": "self", "href": ""},
						},
					},
				},
			},
		})
	})

	// Identity v3 endpoint
	mux.HandleFunc("/v3/", func(w http.ResponseWriter, r *http.Request) {
		// Capture Authorization header
		if authHeaders != nil {
			mu.Lock()
			*authHeaders = append(*authHeaders, r.Header.Get("Authorization"))
			mu.Unlock()
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
	})

	// Auth catalog endpoint
	mux.HandleFunc("/v3/auth/catalog", func(w http.ResponseWriter, r *http.Request) {
		if authHeaders != nil {
			mu.Lock()
			*authHeaders = append(*authHeaders, r.Header.Get("Authorization"))
			mu.Unlock()
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"catalog": []map[string]interface{}{
				{
					"type": "compute",
					"name": "nova",
					"endpoints": []map[string]interface{}{
						{
							"url":       "https://compute.example.com/v2.1",
							"region":    "eu-de",
							"interface": "public",
							"id":        "ep-1",
						},
					},
				},
			},
			"links": map[string]interface{}{
				"self":     "https://iam.example.com/v3/auth/catalog",
				"previous": nil,
				"next":     nil,
			},
		})
	})

	return httptest.NewServer(mux)
}

func TestAKSKProvider_Authenticate(t *testing.T) {
	var authHeaders []string
	var mu sync.Mutex
	server := newMockAKSKServer(t, &authHeaders, &mu)
	defer server.Close()

	opts := config.AuthOpts{
		AuthURL:    server.URL + "/v3",
		AccessKey:  "AKIAIOSFODNN7EXAMPLE",
		SecretKey:  "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		ProjectID:  "test-project-id",
		DomainName: "test-domain",
		Region:     "eu-de",
	}

	provider, err := NewAKSKProvider(opts)
	if err != nil {
		t.Fatalf("unexpected error creating provider: %v", err)
	}

	client, err := provider.GetProvider(context.Background())
	if err != nil {
		t.Fatalf("unexpected error authenticating: %v", err)
	}

	if client.AKSKAuthOptions.AccessKey != "AKIAIOSFODNN7EXAMPLE" {
		t.Errorf("AccessKey = %q, want %q", client.AKSKAuthOptions.AccessKey, "AKIAIOSFODNN7EXAMPLE")
	}

	// Verify SDK-HMAC-SHA256 headers were used in requests
	mu.Lock()
	defer mu.Unlock()
	foundHMAC := false
	for _, h := range authHeaders {
		if strings.HasPrefix(h, "SDK-HMAC-SHA256") {
			foundHMAC = true
			break
		}
	}
	if !foundHMAC {
		t.Errorf("expected SDK-HMAC-SHA256 Authorization header in requests, got: %v", authHeaders)
	}
}

func TestAKSKProvider_MissingCredentials(t *testing.T) {
	_, err := NewAKSKProvider(config.AuthOpts{
		AuthURL: "https://iam.example.com/v3",
	})
	if err == nil {
		t.Fatal("expected error for missing AK/SK credentials")
	}
}
