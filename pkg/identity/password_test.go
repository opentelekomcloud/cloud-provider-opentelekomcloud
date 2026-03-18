package identity

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/opentelekomcloud/cloud-provider-opentelekomcloud/pkg/config"
)

// keystoneTokenResponse returns a minimal Keystone v3 token response.
func keystoneTokenResponse() map[string]interface{} {
	return map[string]interface{}{
		"token": map[string]interface{}{
			"methods":    []string{"password"},
			"expires_at": "2099-01-01T00:00:00.000000Z",
			"user": map[string]interface{}{
				"id":   "user-id",
				"name": "test-user",
				"domain": map[string]interface{}{
					"id":   "domain-id",
					"name": "test-domain",
				},
			},
			"project": map[string]interface{}{
				"id":   "project-id",
				"name": "test-project",
				"domain": map[string]interface{}{
					"id":   "domain-id",
					"name": "test-domain",
				},
			},
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
		},
	}
}

func newMockKeystoneServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()

	// Version discovery endpoint
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
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

	// Token creation endpoint
	mux.HandleFunc("/v3/auth/tokens", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Subject-Token", "test-token-id")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(keystoneTokenResponse())
	})

	return httptest.NewServer(mux)
}

func TestPasswordProvider_Authenticate(t *testing.T) {
	server := newMockKeystoneServer(t)
	defer server.Close()

	opts := config.AuthOpts{
		AuthURL:    server.URL + "/v3",
		Username:   "test-user",
		Password:   "test-password",
		DomainName: "test-domain",
		TenantName: "test-project",
		Region:     "eu-de",
	}

	provider, err := NewPasswordProvider(opts)
	if err != nil {
		t.Fatalf("unexpected error creating provider: %v", err)
	}

	client, err := provider.GetProvider(context.Background())
	if err != nil {
		t.Fatalf("unexpected error authenticating: %v", err)
	}

	if client.TokenID == "" {
		t.Error("expected non-empty TokenID after authentication")
	}
}

func TestPasswordProvider_AuthFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
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
			return
		}
		http.Error(w, `{"error": {"message": "unauthorized"}}`, http.StatusUnauthorized)
	}))
	defer server.Close()

	opts := config.AuthOpts{
		AuthURL:    server.URL + "/v3",
		Username:   "bad-user",
		Password:   "bad-pass",
		DomainName: "domain",
	}

	provider, err := NewPasswordProvider(opts)
	if err != nil {
		t.Fatalf("unexpected error creating provider: %v", err)
	}

	_, err = provider.GetProvider(context.Background())
	if err == nil {
		t.Fatal("expected authentication error")
	}
}

func TestPasswordProvider_MissingCredentials(t *testing.T) {
	_, err := NewPasswordProvider(config.AuthOpts{
		AuthURL: "https://iam.example.com/v3",
	})
	if err == nil {
		t.Fatal("expected error for missing username/password")
	}
}
