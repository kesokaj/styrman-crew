package ingress

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestFetchIP_ValidIPv4(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "  203.0.113.42\n")
	}))
	defer server.Close()

	client := server.Client()
	ip, err := fetchIP(client, server.URL)
	if err != nil {
		t.Fatalf("fetchIP() error = %v", err)
	}
	if ip != "203.0.113.42" {
		t.Errorf("fetchIP() = %q, want %q", ip, "203.0.113.42")
	}
}

func TestFetchIP_RejectsIPv6(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "2001:db8::1\n")
	}))
	defer server.Close()

	client := server.Client()
	_, err := fetchIP(client, server.URL)
	if err == nil {
		t.Error("fetchIP() should reject IPv6 addresses (devbox only uses IPv4 A records)")
	}
}

func TestFetchIP_InvalidResponse(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"html error page", "<html><body>Error</body></html>"},
		{"empty", ""},
		{"garbage text", "not-an-ip-address"},
		{"partial IP", "192.168.1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprint(w, tt.body)
			}))
			defer server.Close()

			client := server.Client()
			_, err := fetchIP(client, server.URL)
			if err == nil {
				t.Errorf("fetchIP() with body %q should return error", tt.body)
			}
		})
	}
}

func TestFetchIP_Timeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		fmt.Fprint(w, "1.2.3.4")
	}))
	defer server.Close()

	client := &http.Client{Timeout: 100 * time.Millisecond}
	_, err := fetchIP(client, server.URL)
	if err == nil {
		t.Error("fetchIP() should timeout")
	}
}

func TestFetchIP_HTTPError(t *testing.T) {
	_, err := fetchIP(http.DefaultClient, "http://127.0.0.1:1") // should fail to connect
	if err == nil {
		t.Error("fetchIP() with unreachable server should return error")
	}
}
