package ip_filter

import (
	"bufio"
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/nunoOliveiraqwe/torii/internal/abuseipdb"
	"github.com/nunoOliveiraqwe/torii/internal/resolve"

	"go.uber.org/zap"
)

type IpLoader interface {
	StartRefreshTimer(ctx context.Context, callback func([]string)) ([]string, error)
	IsRefreshable() bool
	LoadAllowedIps() ([]string, error)
	LoadBlockedIps() ([]string, error)
}

type StaticLoader struct {
	allowedIps []string
	blockedIps []string
}

func (s *StaticLoader) StartRefreshTimer(_ context.Context, _ func([]string)) ([]string, error) {
	zap.S().Debugf("StaticLoader does not support refresh timer")
	return []string{}, nil
}

func (s *StaticLoader) IsRefreshable() bool {
	return false
}

func (s *StaticLoader) LoadAllowedIps() ([]string, error) {
	return s.allowedIps, nil
}

func (s *StaticLoader) LoadBlockedIps() ([]string, error) {
	return s.blockedIps, nil
}

func NewStaticLoader(allowedIps, blockedIps []string) *StaticLoader {
	return &StaticLoader{
		allowedIps: allowedIps,
		blockedIps: blockedIps,
	}
}

type AbuseIpDbBlockListLoader struct {
	confidenceInterval int
	apiKey             string
	refreshInterval    time.Duration
	allowedIps         []string
}

func NewAbuseIpDbBlockListLoader(apiKey string, confidenceInterval int, refreshInterval string, allowedIps []string) (*AbuseIpDbBlockListLoader, error) {
	resolvedKey, err := resolveValue(apiKey)
	if err != nil {
		return nil, fmt.Errorf("abuseipdb: resolving api key: %w", err)
	}

	interval, err := time.ParseDuration(refreshInterval)
	if err != nil {
		return nil, fmt.Errorf("abuseipdb: parsing refresh interval %q: %w", refreshInterval, err)
	}

	return &AbuseIpDbBlockListLoader{
		confidenceInterval: confidenceInterval,
		apiKey:             resolvedKey,
		refreshInterval:    interval,
		allowedIps:         allowedIps,
	}, nil
}

// resolveValue checks if the value uses the $resolver:value syntax (e.g.
// $env:API_KEY or $file:/run/secrets/key) and resolves it. Plain strings
// are returned as-is.
func resolveValue(value string) (string, error) {
	if !strings.HasPrefix(value, "$") {
		return value, nil
	}
	idx := strings.Index(value, ":")
	if idx < 0 {
		return "", fmt.Errorf("invalid resolver syntax: %s", value)
	}
	resolverKey := value[1:idx]
	resolver := resolve.GetResolver(resolverKey)
	if resolver == nil {
		return "", fmt.Errorf("unknown resolver: %s", resolverKey)
	}
	return resolver.Resolve(value[idx+1:])
}

func (s *AbuseIpDbBlockListLoader) StartRefreshTimer(ctx context.Context, callback func([]string)) ([]string, error) {
	ips, err := s.fetchBlockList()
	if err != nil {
		zap.S().Warnf("AbuseIPDB initial fetch failed, starting with empty block list: %v", err)
		ips = nil
	}

	ticker := time.NewTicker(s.refreshInterval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				zap.S().Infof("AbuseIPDB refresh goroutine stopped")
				return
			case <-ticker.C:
				zap.S().Infof("AbuseIPDB refresh tick, fetching block list")
				refreshed, err := s.fetchBlockList()
				if err != nil {
					zap.S().Errorf("AbuseIPDB refresh failed: %v", err)
					continue
				}
				callback(refreshed)
			}
		}
	}()

	return ips, nil
}

func (s *AbuseIpDbBlockListLoader) IsRefreshable() bool {
	return true
}

func (s *AbuseIpDbBlockListLoader) LoadAllowedIps() ([]string, error) {
	return s.allowedIps, nil
}

func (s *AbuseIpDbBlockListLoader) LoadBlockedIps() ([]string, error) {
	return s.fetchBlockList()
}

func (s *AbuseIpDbBlockListLoader) fetchBlockList() ([]string, error) {
	body, err := abuseipdb.NewClient(s.apiKey).
		BlackList().
		PlainText().
		ConfidenceMinimum(s.confidenceInterval).
		Fetch()
	if err != nil {
		return nil, err
	}
	defer body.Close()

	var ips []string
	scanner := bufio.NewScanner(body)
	for scanner.Scan() {
		if line := scanner.Text(); line != "" {
			ips = append(ips, line)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	zap.S().Infof("Fetched AbuseIPDB block list (%d IPs)", len(ips))
	return ips, nil
}
