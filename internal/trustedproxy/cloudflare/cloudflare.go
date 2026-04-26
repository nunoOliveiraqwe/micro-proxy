package cloudflare

import (
	"bufio"
	"net/http"
	"strings"
	"time"

	"github.com/nunoOliveiraqwe/torii/internal/trustedproxy/resolver"
	"go.uber.org/zap"
)

type CloudFlarePreset struct {
	URLs []string
}

var cloudflarePreset = CloudFlarePreset{
	URLs: []string{
		"https://www.cloudflare.com/ips-v4/",
		"https://www.cloudflare.com/ips-v6/",
	},
}

func init() {
	err := resolver.AddProxyResolver("cloudflare", &cloudflarePreset)
	if err != nil {
		zap.S().Errorf("registering cloudflare preset: %v", err)
		return
	}
}

func (c *CloudFlarePreset) DefaultHeader() string {
	return "CF-Connecting-IP"
}

func (c *CloudFlarePreset) DefaultRefresh() time.Duration {
	return 24 * time.Hour
}

func (c *CloudFlarePreset) Resolve() []string {
	var all []string
	client := &http.Client{Timeout: 15 * time.Second}

	for _, u := range c.URLs {
		resp, err := client.Get(u)
		if err != nil {
			zap.S().Errorf("fetching %s: %v", u, err)
			return []string{}
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			zap.S().Errorf("fetching %s: HTTP %d", u, resp.StatusCode)
			return []string{}
		}

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line != "" && !strings.HasPrefix(line, "#") {
				all = append(all, line)
			}
		}
		if err := scanner.Err(); err != nil {
			zap.S().Errorf("reading %s: %v", u, err)
			return []string{}
		}
	}

	if len(all) == 0 {
		zap.S().Warnf("trusted-proxy: no CIDRs fetched from %d URL(s)", len(c.URLs))
		return []string{}
	}

	zap.S().Infof("trusted-proxy: fetched %d CIDRs from %d URL(s)", len(all), len(c.URLs))
	return all
}
