package trustedproxy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	_ "github.com/nunoOliveiraqwe/torii/internal/trustedproxy/cloudflare" // register preset
)

func TestNewTrustedProxyResolver_Errors(t *testing.T) {
	tests := []struct {
		name   string
		cidrs  []string
		header string
	}{
		{"empty list", nil, ""},
		{"empty strings only", []string{"", ""}, ""},
		{"invalid CIDR", []string{"not-a-cidr/999"}, ""},
		{"invalid bare IP", []string{"xyz"}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewTrustedProxyResolver(tt.cidrs, tt.header)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}

func TestResolve_UntrustedRemoteAddr(t *testing.T) {
	r, _ := NewTrustedProxyResolver([]string{"10.0.0.0/8"}, "")
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "1.2.3.4:5678"
	req.Header.Set("X-Forwarded-For", "9.9.9.9")

	got := r.Resolve(req)
	if got != "1.2.3.4:5678" {
		t.Errorf("expected RemoteAddr unchanged, got %q", got)
	}
}

func TestResolve_TrustedRemoteAddr_XFF(t *testing.T) {
	r, _ := NewTrustedProxyResolver([]string{"10.0.0.0/8"}, "")
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.0.0.1:5678"
	req.Header.Set("X-Forwarded-For", "203.0.113.50, 10.0.0.2")

	got := r.Resolve(req)
	if got != "203.0.113.50:5678" {
		t.Errorf("expected 203.0.113.50:5678, got %q", got)
	}
}

func TestResolve_TrustedChain(t *testing.T) {
	r, _ := NewTrustedProxyResolver([]string{"10.0.0.0/8", "172.16.0.0/12"}, "")
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	req.Header.Set("X-Forwarded-For", "8.8.8.8, 172.16.0.5, 10.0.0.3")

	got := r.Resolve(req)
	if got != "8.8.8.8:1234" {
		t.Errorf("expected 8.8.8.8:1234, got %q", got)
	}
}

func TestResolve_NoXFFHeader(t *testing.T) {
	r, _ := NewTrustedProxyResolver([]string{"10.0.0.0/8"}, "")
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.0.0.1:5678"

	got := r.Resolve(req)
	if got != "10.0.0.1:5678" {
		t.Errorf("expected fallback to RemoteAddr, got %q", got)
	}
}

func TestResolve_CustomHeader(t *testing.T) {
	r, _ := NewTrustedProxyResolver([]string{"172.16.0.0/12"}, "CF-Connecting-IP")
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "172.16.0.1:8080"
	req.Header.Set("CF-Connecting-IP", "203.0.113.100")

	got := r.Resolve(req)
	if got != "203.0.113.100:8080" {
		t.Errorf("expected 203.0.113.100:8080, got %q", got)
	}
}

func TestResolve_AllTrustedFallback(t *testing.T) {
	r, _ := NewTrustedProxyResolver([]string{"10.0.0.0/8"}, "")
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.0.0.1:5678"
	req.Header.Set("X-Forwarded-For", "10.0.0.5, 10.0.0.6")

	got := r.Resolve(req)
	// All trusted — falls back to left-most.
	if got != "10.0.0.5:5678" {
		t.Errorf("expected 10.0.0.5:5678, got %q", got)
	}
}

func TestResolve_BareIPTrusted(t *testing.T) {
	r, _ := NewTrustedProxyResolver([]string{"192.168.1.1"}, "")
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "192.168.1.1:4000"
	req.Header.Set("X-Forwarded-For", "5.5.5.5")

	got := r.Resolve(req)
	if got != "5.5.5.5:4000" {
		t.Errorf("expected 5.5.5.5:4000, got %q", got)
	}
}

func TestResolve_IPv6(t *testing.T) {
	r, _ := NewTrustedProxyResolver([]string{"::1/128", "fd00::/8"}, "")
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "[::1]:5678"
	req.Header.Set("X-Forwarded-For", "2001:db8::1, fd00::5")

	got := r.Resolve(req)
	if got != "[2001:db8::1]:5678" {
		t.Errorf("expected [2001:db8::1]:5678, got %q", got)
	}
}

func TestWrapHandler(t *testing.T) {
	r, _ := NewTrustedProxyResolver([]string{"10.0.0.0/8"}, "")

	var capturedAddr string
	inner := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		capturedAddr = req.RemoteAddr
		w.WriteHeader(http.StatusOK)
	})

	handler := r.WrapHandler(inner)
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.0.0.1:5678"
	req.Header.Set("X-Forwarded-For", "42.42.42.42")
	rr := httptest.NewRecorder()
	handler(rr, req)

	if capturedAddr != "42.42.42.42:5678" {
		t.Errorf("expected rewritten RemoteAddr, got %q", capturedAddr)
	}
}

func TestResolve_XFFWithSpaces(t *testing.T) {
	r, _ := NewTrustedProxyResolver([]string{"10.0.0.0/8"}, "")
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.0.0.1:5678"
	req.Header.Set("X-Forwarded-For", "  203.0.113.1 , 10.0.0.2 , 10.0.0.3 ")

	got := r.Resolve(req)
	if got != "203.0.113.1:5678" {
		t.Errorf("expected 203.0.113.1:5678, got %q", got)
	}
}

// ── Preset tests ──

func TestPreset_Unknown(t *testing.T) {
	_, err := NewTrustedProxyResolverFromPreset(context.Background(), "aws-alb", nil, "", 0)
	if err == nil {
		t.Fatal("expected error for unknown preset, got nil")
	}
}

func TestPreset_EmptyWithCIDRs(t *testing.T) {
	// No preset, just manual CIDRs — no remote fetch, works like plain constructor.
	r, err := NewTrustedProxyResolverFromPreset(context.Background(), "", []string{"192.168.0.0/16"}, "", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "192.168.1.1:9000"
	req.Header.Set("X-Forwarded-For", "7.7.7.7")

	got := r.Resolve(req)
	if got != "7.7.7.7:9000" {
		t.Errorf("expected 7.7.7.7:9000, got %q", got)
	}
}

// TestPreset_Cloudflare_Integration fetches live Cloudflare ranges.
// Skipped in short mode (CI). Run locally with: go test ./internal/netutil/ -run Integration
func TestPreset_Cloudflare_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test that requires network access")
	}

	r, err := NewTrustedProxyResolverFromPreset(context.Background(), "cloudflare", nil, "", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should use CF-Connecting-IP by default.
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "173.245.48.1:443" // known CF range
	req.Header.Set("CF-Connecting-IP", "203.0.113.42")
	req.Header.Set("X-Forwarded-For", "should-be-ignored")

	got := r.Resolve(req)
	if got != "203.0.113.42:443" {
		t.Errorf("expected 203.0.113.42:443, got %q", got)
	}
}

func TestPreset_CloudflareWithExtraCIDRs_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test that requires network access")
	}

	r, err := NewTrustedProxyResolverFromPreset(context.Background(), "cloudflare", []string{"10.0.0.0/8"}, "", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Extra CIDR should also be trusted.
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.0.0.5:8080"
	req.Header.Set("CF-Connecting-IP", "1.2.3.4")

	got := r.Resolve(req)
	if got != "1.2.3.4:8080" {
		t.Errorf("expected 1.2.3.4:8080, got %q", got)
	}
}

func TestPreset_CloudflareHeaderOverride_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test that requires network access")
	}

	// Explicit header overrides preset default.
	r, err := NewTrustedProxyResolverFromPreset(context.Background(), "cloudflare", nil, "X-Real-IP", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "104.16.0.1:443" // CF range
	req.Header.Set("X-Real-IP", "5.5.5.5")
	req.Header.Set("CF-Connecting-IP", "should-be-ignored")

	got := r.Resolve(req)
	if got != "5.5.5.5:443" {
		t.Errorf("expected 5.5.5.5:443, got %q", got)
	}
}
