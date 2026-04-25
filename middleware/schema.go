package middleware

// OptionFieldType defines the type of a middleware configuration field for UI rendering.
type OptionFieldType string

const (
	FieldTypeString      OptionFieldType = "string"
	FieldTypeBool        OptionFieldType = "bool"
	FieldTypeInt         OptionFieldType = "int"
	FieldTypeFloat       OptionFieldType = "float"
	FieldTypeSelect      OptionFieldType = "select"
	FieldTypeMultiSelect OptionFieldType = "multiselect"
	FieldTypeStringList  OptionFieldType = "stringlist"
	FieldTypeMap         OptionFieldType = "map"
)

// OptionField describes a single configuration option for a middleware.
type OptionField struct {
	Key         string          `json:"key"`
	Label       string          `json:"label"`
	Type        OptionFieldType `json:"type"`
	Required    bool            `json:"required"`
	Default     interface{}     `json:"default,omitempty"`
	Placeholder string          `json:"placeholder,omitempty"`
	HelpText    string          `json:"help_text,omitempty"`
	Choices     []string        `json:"choices,omitempty"`
	Group       string          `json:"group,omitempty"`
}

// MiddlewareSchema describes a middleware type and its configuration options.
type MiddlewareSchema struct {
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Terminates  bool          `json:"terminates"`
	Fields      []OptionField `json:"fields"`
}

var cacheFields = []OptionField{
	{Key: "cache-ttl", Label: "Cache TTL", Type: FieldTypeString, Default: "1h", Group: "cache",
		Placeholder: "e.g. 24h, 30m",
		HelpText:    "Time to live for cached entries. After this duration, entries expire and are removed."},
	{Key: "cleanup-interval", Label: "Cleanup Interval", Type: FieldTypeString, Default: "1h", Group: "cache",
		Placeholder: "e.g. 1h, 30m",
		HelpText:    "How often the cache sweeps for expired entries."},
	{Key: "max-cache-size", Label: "Max Cache Size", Type: FieldTypeInt, Default: 100000, Group: "cache",
		Placeholder: "e.g. 100000",
		HelpText:    "Maximum number of entries in the cache. Oldest entries are evicted when full."},
}

// GetMiddlewareSchemas returns the schema for all available middlewares.
func GetMiddlewareSchemas() []MiddlewareSchema {
	schemas := []MiddlewareSchema{
		{
			Name:        "RequestId",
			Description: "Generates unique request IDs for tracing requests across proxied services.",
			Fields: []OptionField{
				{Key: "prefix", Label: "Prefix", Type: FieldTypeString,
					Placeholder: "e.g. my-proxy",
					HelpText:    "Custom prefix for generated request IDs. If not set, a prefix is auto-generated from the hostname.",
				},
			},
		},
		{
			Name:        "RequestLog",
			Description: "Logs every incoming request with method, URL, user-agent, and remote address.",
			Fields: []OptionField{
				{Key: "request-log-path", Label: "Access Log File Path", Type: FieldTypeString,
					Placeholder: "e.g. access.log",
					HelpText:    "File path to write access logs. If not set, logs are written to standard output only",
				},
			},
		},
		{
			Name:        "Metrics",
			Description: "Collects request/response metrics: latency, status codes, bytes transferred. Powers the dashboard charts.",
			Fields:      []OptionField{},
		},
		{
			Name:        "Headers",
			Description: "Manipulates request and response headers. Can set, strip, or validate headers.",
			Fields: []OptionField{
				{Key: "set-headers-req", Label: "Set Request Headers", Type: FieldTypeMap,
					HelpText: "Headers to add or override on incoming requests. Values starting with $ are resolved dynamically (e.g., $file:/path/to/secret)."},
				{Key: "set-headers-res", Label: "Set Response Headers", Type: FieldTypeMap,
					HelpText: "Headers to add or override on outgoing responses."},
				{Key: "strip-headers-req", Label: "Strip Request Headers", Type: FieldTypeStringList,
					HelpText: "Header names to remove from incoming requests before proxying."},
				{Key: "strip-headers-res", Label: "Strip Response Headers", Type: FieldTypeStringList,
					HelpText: "Header names to remove from outgoing responses."},
				{Key: "cmp-headers-req", Label: "Required Request Headers", Type: FieldTypeMap,
					HelpText: "Headers that must match exact values. If any don't match, the request is rejected with 401 Unauthorized."},
			},
		},
		{
			Name:        "Cors",
			Description: "Handles Cross-Origin Resource Sharing (CORS) preflight and response headers.",
			Fields: []OptionField{
				{Key: "allowed-origins", Label: "Allowed Origins", Type: FieldTypeStringList, Default: []string{"*"},
					HelpText: "Origins allowed to make cross-origin requests. Use * to allow all."},
				{Key: "allowed-methods", Label: "Allowed Methods", Type: FieldTypeMultiSelect,
					Choices:  []string{"GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS", "HEAD"},
					Default:  []string{"GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS"},
					HelpText: "HTTP methods allowed in cross-origin requests."},
				{Key: "allowed-headers", Label: "Allowed Headers", Type: FieldTypeStringList,
					Default:  []string{"Content-Type", "Authorization"},
					HelpText: "Headers the client is allowed to send in cross-origin requests."},
				{Key: "expose-headers", Label: "Expose Headers", Type: FieldTypeStringList,
					HelpText: "Response headers the browser is allowed to access from JavaScript."},
				{Key: "allow-credentials", Label: "Allow Credentials", Type: FieldTypeBool, Default: false,
					HelpText: "Allow cookies and credentials in cross-origin requests. Cannot be used with origin *."},
				{Key: "max-age", Label: "Max Age (seconds)", Type: FieldTypeInt, Default: 0,
					HelpText: "How long the browser can cache preflight results. 0 means no caching."},
			},
		},
		{
			Name:        "RateLimiter",
			Description: "Enforces request rate limits to protect backends from overload. Supports global or per-client IP limiting.",
			Fields: append([]OptionField{
				{Key: "mode", Label: "Mode", Type: FieldTypeSelect, Default: "global",
					Choices:  []string{"global", "per-client"},
					HelpText: "Global: single rate limit shared by all clients. Per-client: separate limit per IP address."},
				{Key: "limiter-req.rate-per-second", Label: "Rate (req/sec)", Type: FieldTypeFloat, Required: true,
					Placeholder: "e.g. 5",
					HelpText:    "Maximum sustained request rate in requests per second."},
				{Key: "limiter-req.burst", Label: "Burst Size", Type: FieldTypeInt, Required: true,
					Placeholder: "e.g. 10",
					HelpText:    "Maximum number of requests allowed in a burst above the sustained rate."},
			}, cacheFields...),
		},
		{
			Name:        "CountryBlock",
			Description: "Blocks or allows requests based on geographic location using MaxMind GeoIP databases.",
			Fields: append([]OptionField{
				{Key: "source.mode", Label: "Database Source", Type: FieldTypeSelect, Required: true,
					Choices:  []string{"local", "remote"},
					Group:    "source",
					HelpText: "Local: load from a file on disk. Remote: download from a URL."},
				{Key: "source.path", Label: "Database Path / URL", Type: FieldTypeString, Required: true,
					Group:       "source",
					Placeholder: "e.g. /path/to/geo.mmdb or https://...",
					HelpText:    "File path (local mode) or URL (remote mode) to the MaxMind MMDB database."},
				{Key: "source.country-field", Label: "Country Field", Type: FieldTypeString, Required: true,
					Group:    "source",
					Default:  "country_code",
					HelpText: "Field name in the database for country codes. Use dots for nested fields (e.g., country.iso_code)."},
				{Key: "source.continent-field", Label: "Continent Field", Type: FieldTypeString,
					Group:    "source",
					HelpText: "Field name for continent codes. Optional. Use dots for nested fields (e.g., continent.code)."},
				{Key: "source.max-size", Label: "Max Download Size", Type: FieldTypeString,
					Group: "source", Default: "300m",
					HelpText: "Maximum download size for remote databases (e.g., 300m, 1g). Ignored for local mode."},
				{Key: "source.refresh-interval", Label: "Refresh Interval", Type: FieldTypeString,
					Group:       "source",
					Placeholder: "e.g. 24h",
					HelpText:    "How often to re-download the remote database. Ignored for local mode."},
				{Key: "on-unknown", Label: "Unknown IP Action", Type: FieldTypeSelect, Default: "block",
					Choices:  []string{"allow", "block"},
					HelpText: "What to do when the country/continent cannot be determined for an IP."},
				{Key: "country-list-mode", Label: "Country List Mode", Type: FieldTypeSelect,
					Choices:  []string{"allow", "block"},
					HelpText: "Allow: only listed countries can access. Block: listed countries are denied."},
				{Key: "country-list", Label: "Country Codes", Type: FieldTypeStringList,
					Placeholder: "e.g. PT, US, CA",
					HelpText:    "ISO 3166-1 alpha-2 country codes. When both country and continent lists are set, country acts as an override."},
				{Key: "continent-list-mode", Label: "Continent List Mode", Type: FieldTypeSelect,
					Choices:  []string{"allow", "block"},
					HelpText: "Allow: only listed continents can access. Block: listed continents are denied."},
				{Key: "continent-list", Label: "Continent Codes", Type: FieldTypeStringList,
					Placeholder: "e.g. EU, NA, AS",
					HelpText:    "Continent codes (AF, AN, AS, EU, NA, OC, SA). Acts as the broad policy when combined with country list."},
			}, cacheFields...),
		},
		{
			Name:        "IpFilter",
			Description: "Blocks or allows requests based on client IP address or CIDR range. Optionally fetches block lists from AbuseIPDB.",
			Fields: []OptionField{
				{Key: "allow", Label: "Allow List", Type: FieldTypeStringList,
					Placeholder: "e.g. 192.168.1.0/24",
					HelpText:    "IP addresses or CIDR blocks that are always allowed, overriding the block list. Supports IPv4 and IPv6."},
				{Key: "block", Label: "Block List", Type: FieldTypeStringList,
					Placeholder: "e.g. 94.0.0.0/24",
					HelpText:    "IP addresses or CIDR blocks to deny. Ignored when AbuseIPDB is configured (it provides the block list)."},
				{Key: "abuseipdb-api-key", Label: "AbuseIPDB API Key", Type: FieldTypeString,
					Placeholder: "e.g. $env:ABUSEIPDB_API_KEY",
					HelpText:    "API key for AbuseIPDB. Supports resolver syntax ($env:VAR, $file:/path). When set, the block list is fetched from AbuseIPDB instead of using the static block list."},
				{Key: "abuseipdb-confidence-minimum", Label: "AbuseIPDB Confidence Minimum", Type: FieldTypeInt, Default: 90,
					HelpText: "Minimum abuse confidence score (1-100). Only IPs at or above this score are blocked."},
				{Key: "abuseipdb-refresh-interval", Label: "AbuseIPDB Refresh Interval", Type: FieldTypeString,
					Placeholder: "e.g. 24h, 12h",
					HelpText:    "How often to re-fetch the block list from AbuseIPDB. Required when AbuseIPDB is configured."},
			},
		},
		{
			Name:        "Redirect",
			Description: "Redirects requests to a different target. Internal mode proxies transparently; external mode sends an HTTP redirect; external-scheme-only mode preserves the incoming host and only changes the scheme (ideal for HTTP→HTTPS across multiple subdomains).",
			Fields: []OptionField{
				{Key: "mode", Label: "Mode", Type: FieldTypeSelect, Default: "external",
					Choices:  []string{"internal", "external", "external-scheme-only"},
					HelpText: "Internal: proxy the request transparently (client doesn't know). External: send an HTTP 3xx redirect to a fixed target. External-scheme-only: send an HTTP 3xx redirect preserving the incoming host, only changing the scheme (e.g. http → https)."},
				{Key: "target", Label: "Target", Type: FieldTypeString, Required: true,
					Placeholder: "e.g. http://192.168.1.100:8080, host:port, or https (for external-scheme-only)",
					HelpText:    "Target URL or host:port for internal/external modes. For external-scheme-only, use just the scheme: 'https' or 'http'."},
				{Key: "status-code", Label: "Status Code", Type: FieldTypeInt,
					Default:  302,
					HelpText: "HTTP redirect status code (301=permanent, 302=temporary, 307/308=preserve method). Required for external and external-scheme-only modes."},
				{Key: "drop-path", Label: "Drop Path", Type: FieldTypeBool, Default: true,
					HelpText: "If true, the original request path is stripped. The target receives only its own path. Defaults to false for external-scheme-only."},
				{Key: "drop-query", Label: "Drop Query", Type: FieldTypeBool, Default: true,
					HelpText: "If true, the original query parameters are stripped. Defaults to false for external-scheme-only."},
			},
		},
		{
			Name:        "BodySizeLimit",
			Description: "Limits the maximum size of incoming request bodies to protect against large uploads.",
			Fields: []OptionField{
				{Key: "max-size", Label: "Max Body Size", Type: FieldTypeString, Required: true,
					Placeholder: "e.g. 10m, 1g, 512k",
					HelpText:    "Maximum allowed request body size. Supports k (kilobytes), m (megabytes), g (gigabytes)."},
			},
		},
		{
			Name:        "Timeout",
			Description: "Sets the maximum time allowed for the entire request to complete.",
			Fields: []OptionField{
				{Key: "request-timeout", Label: "Request Timeout", Type: FieldTypeString, Required: true,
					Placeholder: "e.g. 30s, 1m, 5m",
					HelpText:    "Maximum allowed time for the entire request processing. Supports s (seconds), m (minutes), h (hours)."},
			},
		},
		{
			Name:        "HoneyPot",
			Description: "Detects and traps bots by monitoring access to honeypot paths. Caches and blocks offending IPs.",
			Fields: append([]OptionField{
				{Key: "defaults", Label: "Default Path Groups", Type: FieldTypeMultiSelect,
					Choices:  []string{"php", "git", "infra", "secrets", "iot", "backups", "cgi"},
					HelpText: "Predefined groups of common attack paths. php: .env, wp-login.php, wp-admin, etc. git: .git/config, .svn. infra: /actuator, /.aws/credentials. secrets: SSH keys, .htpasswd, private keys. iot: router exploits, DVR/camera panels. backups: .zip, .sql dumps. cgi: /cgi-bin/."},
				{Key: "paths", Label: "Custom Paths", Type: FieldTypeStringList,
					Placeholder: "e.g. /my-custom-trap",
					HelpText:    "Additional custom paths to treat as honeypots. Any request matching these paths triggers the trap."},
				{Key: "response.trickster-mode", Label: "Trickster Mode", Type: FieldTypeBool, Default: false, Group: "response",
					HelpText: "Reply with deceptive responses designed to waste bot resources (slow tarpits, infinite streams, fake pages). If enabled, status-code and body are ignored."},
				{Key: "response.status-code", Label: "Status Code", Type: FieldTypeInt, Default: 403, Group: "response",
					HelpText: "HTTP status code for honeypot responses (e.g., 403, 404). Ignored if trickster mode is enabled."},
				{Key: "response.body", Label: "Response Body", Type: FieldTypeString, Default: "Forbidden", Group: "response",
					HelpText: "Response body text for honeypot hits. Ignored if trickster mode is enabled."},
				{Key: "response.max-slow-tricks", Label: "Max Slow Tricks", Type: FieldTypeInt, Default: 10, Group: "response",
					HelpText: "Maximum concurrent slow-trick responses. Careful: these tie up connections on both sides. Keep this low."},
			}, cacheFields...),
		},
		{
			Name:        "UserAgentBlocker",
			Description: "Blocks requests from known bots, scanners, and unwanted user agents using pattern matching.",
			Fields: append([]OptionField{
				{Key: "block-empty-ua", Label: "Block Empty User-Agent", Type: FieldTypeBool, Default: true,
					HelpText: "Block requests that have no User-Agent header. Most legitimate browsers always send one."},
				{Key: "block-defaults", Label: "Block Categories", Type: FieldTypeMultiSelect,
					Choices:  []string{"scanners", "recon", "scrapers", "headless", "malicious", "http-clients", "cli-tools", "ai-crawlers", "seo-bots", "generic", "social", "search-engines"},
					HelpText: "Built-in categories of bot User-Agent patterns to block. scanners: vulnerability scanners. recon: internet-wide scanners. scrapers: web scrapers. headless: headless browsers. malicious: known bad bots. http-clients: libraries like python-requests, curl. cli-tools: wget, httpie. ai-crawlers: AI training crawlers. seo-bots: Ahrefs, Semrush, etc. generic: broad bot indicators. social: Facebook, Twitter preview bots. search-engines: Google, Bing, etc."},
				{Key: "block", Label: "Custom Block Patterns", Type: FieldTypeStringList,
					Placeholder: "e.g. bad-bot-name",
					HelpText:    "Custom User-Agent substrings to block. Case-insensitive matching."},
				{Key: "allow-defaults", Label: "Allow Categories", Type: FieldTypeMultiSelect,
					Choices:  []string{"search-engines", "social", "seo-bots", "generic", "http-clients", "cli-tools", "scanners", "recon", "scrapers", "headless", "malicious", "ai-crawlers"},
					HelpText: "Built-in categories to always allow, overriding blocks. search-engines: Google, Bing, etc. social: Facebook, Twitter, Slack, etc. seo-bots: Ahrefs, Semrush, etc."},
				{Key: "allow", Label: "Custom Allow Patterns", Type: FieldTypeStringList,
					Placeholder: "e.g. my-good-bot",
					HelpText:    "Custom User-Agent substrings to always allow. If a UA matches both allow and block, allow wins."},
			}, cacheFields...),
		},
		{
			Name:        "StaticResponse",
			Description: "Returns a fixed HTTP response without forwarding to any backend. Useful for maintenance pages, health stubs, or canned API responses.",
			Terminates:  true,
			Fields: []OptionField{
				{Key: "status-code", Label: "Status Code", Type: FieldTypeInt, Required: true,
					Placeholder: "e.g. 200, 404, 503",
					HelpText:    "HTTP status code to return (100-599)."},
				{Key: "response-body", Label: "Response Body", Type: FieldTypeString,
					Placeholder: "e.g. {\"status\":\"ok\"} or <h1>Under Maintenance</h1>",
					HelpText:    "Body text to return. Leave empty for a body-less response (e.g. 204)."},
				{Key: "content-type", Label: "Content-Type", Type: FieldTypeString,
					Default:  "text/plain; charset=utf-8",
					HelpText: "Content-Type header for the response body. Only sent when a body is set."},
				{Key: "headers", Label: "Custom Headers", Type: FieldTypeMap,
					HelpText: "Additional headers to include in the response (e.g. Retry-After, Cache-Control)."},
			},
		},
		{
			Name:        "StaticFileServer",
			Description: "Serves static files from a local directory. Supports SPA fallback, index files, cache control, and dotfile protection. Terminates the request — no backend is needed.",
			Terminates:  true,
			Fields: []OptionField{
				{Key: "root", Label: "Root Directory", Type: FieldTypeString, Required: true,
					Placeholder: "e.g. /var/www/html or ./public",
					HelpText:    "Absolute or relative path to the directory containing static files to serve."},
				{Key: "index-file", Label: "Index File", Type: FieldTypeString, Default: "index.html",
					HelpText: "File to serve when a directory is requested. Defaults to index.html."},
				{Key: "spa", Label: "SPA Mode", Type: FieldTypeBool, Default: false,
					HelpText: "Single Page Application mode. When enabled, unmatched paths fall back to the root index file instead of returning 404."},
				{Key: "allow-dot-files", Label: "Allow Dotfiles", Type: FieldTypeBool, Default: false,
					HelpText: "Allow serving hidden files (names starting with a dot, e.g. .env, .git). Disabled by default for security."},
			},
		},
		{
			Name:        "Compression",
			Description: "Compresses response bodies using gzip or zlib to reduce bandwidth usage.",
			Fields: []OptionField{
				{Key: "type", Label: "Compression Type", Type: FieldTypeSelect, Required: true,
					Choices:  []string{"gzip", "zlib"},
					Default:  "gzip",
					HelpText: "Compression algorithm to use. gzip is the most widely supported by browsers."},
				{Key: "level", Label: "Compression Level", Type: FieldTypeInt,
					Default:     9,
					Placeholder: "1-9",
					HelpText:    "Compression level (1=fastest/least compression, 9=slowest/best compression). Defaults to best compression."},
			},
		},
		{
			Name:        "BasicAuth",
			Description: "Protects routes with HTTP Basic Authentication. Passwords are verified using Argon2id hashing.",
			Fields: []OptionField{
				{Key: "realm", Label: "Realm", Type: FieldTypeString, Required: true,
					Placeholder: "e.g. Internal",
					HelpText:    "Authentication realm displayed in the browser's login prompt."},
				{Key: "credentials", Label: "Credentials", Type: FieldTypeMap, Required: true,
					Placeholder: "username → argon2id hash",
					HelpText:    "Map of usernames to Argon2id password hashes. Generate hashes with the included argon2 CLI tool."},
			},
		},
		{
			Name:        "CircuitBreaker",
			Description: "Implements the circuit breaker pattern to stop sending requests to a failing backend, giving it time to recover.",
			Fields: []OptionField{
				{Key: "failure-threshold", Label: "Failure Threshold", Type: FieldTypeInt, Required: true,
					Placeholder: "e.g. 5",
					HelpText:    "Number of consecutive failures (5xx or timeout) before the circuit opens and starts rejecting requests."},
				{Key: "recovery-time", Label: "Recovery Time", Type: FieldTypeString, Required: true,
					Placeholder: "e.g. 30s, 1m",
					HelpText:    "How long to wait in open state before allowing a test request through (half-open state)."},
				{Key: "half-open-success-threshold", Label: "Half-Open Success Threshold", Type: FieldTypeInt, Default: 3,
					HelpText: "Number of consecutive successful requests in half-open state needed to close the circuit and resume normal traffic."},
			},
		},
		{
			Name:        "CorazaWaf",
			Description: "Web Application Firewall powered by Coraza and the OWASP Core Rule Set (CRS). Inspects requests and responses for common attacks such as SQL injection, XSS, and protocol violations.",
			Fields: []OptionField{
				{Key: "paranoia-level", Label: "Paranoia Level", Type: FieldTypeSelect, Default: "1",
					Choices:  []string{"1", "2", "3", "4"},
					HelpText: "CRS paranoia level (1–4). Higher levels enable more aggressive rules but may increase false positives. Level 1 is recommended for most deployments."},
				{Key: "inbound-threshold", Label: "Inbound Anomaly Threshold", Type: FieldTypeInt, Default: 5,
					Placeholder: "e.g. 5",
					HelpText:    "Cumulative anomaly score a request must reach before being blocked. Lower values are stricter."},
				{Key: "outbound-threshold", Label: "Outbound Anomaly Threshold", Type: FieldTypeInt, Default: 4,
					Placeholder: "e.g. 4",
					HelpText:    "Cumulative anomaly score a response must reach before being flagged. Lower values are stricter."},
				{Key: "mode", Label: "Mode", Type: FieldTypeSelect, Default: "detect",
					Choices:  []string{"detect", "block"},
					HelpText: "Detect: log rule matches but allow the request through. Block: actively reject requests that exceed the anomaly threshold."},
				{Key: "inspect-request-body", Label: "Inspect Request Body", Type: FieldTypeBool, Default: false,
					HelpText: "Buffer and inspect request bodies (phases 2–3). Adds latency and memory usage. Disable for routes that only need header/URL inspection."},
				{Key: "inspect-response-body", Label: "Inspect Response Body", Type: FieldTypeBool, Default: false,
					HelpText: "Inspect response bodies (phase 4). Only useful in detect mode since the response has already been sent to the client."},
				{Key: "exclusions", Label: "Rule Exclusions", Type: FieldTypeStringList,
					Placeholder: "e.g. 920170, 941100",
					HelpText:    "CRS rule IDs to exclude. Use this to suppress known false positives for your application."},
			},
		},
	}
	for i, s := range schemas {
		if entry, exists := registry[s.Name]; exists && entry.Terminates {
			schemas[i].Terminates = true
		} else {
			schemas[i].Terminates = false
		}
	}
	return schemas
}
