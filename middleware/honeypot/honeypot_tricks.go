package honeypot

import (
	crnd "crypto/rand"
	"fmt"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"github.com/nunoOliveiraqwe/torii/internal/util"
	"go.uber.org/zap"
)

// pick returns a random element from a string slice.
func pick(s []string) string { return s[rand.Intn(len(s))] }

// Fake value pools — each request gets a random combination so no two responses
// are identical. None of these are real credentials. This is AI generated!!
var (
	fakeDbHosts     = []string{"db-primary.internal.local", "db-replica-02.prod.lan", "mysql-prod.us-east-1.rds.internal", "pgmaster.infra.local", "mariadb-01.dc2.internal"}
	fakeDbNames     = []string{"portal_production", "app_main", "webapp_prod_v3", "core_platform", "userdata_live"}
	fakeDbUsers     = []string{"portal_admin", "app_rw", "root", "db_migrate", "webapp_svc"}
	fakeDbPasswords = []string{"Pr0d!P@ssw0rd#2025", "x9$kL!mN2@vR", "Sup3r_S3cur3!#db", "p@ss1234PROD!", "Ch@ng3M3N0w!!"}
	fakeDbPorts     = []string{"3306", "5432", "3307", "33060"}
	fakeAppKeys     = []string{"base64:dGhpc2lzYWhvbmV5cG90ZmFrZWtleQ==", "base64:Zm9vQmFyQmF6UXV4MTIzNDU2Nzg5", "base64:cHJvZHVjdGlvbl9hcHBfa2V5XzIwMjU="}
	fakeAppURLs     = []string{"https://portal.internal.local", "https://app.corp.internal", "https://admin.prod.local", "https://platform.dc1.internal"}
	fakeRedisHosts  = []string{"redis.internal.local", "cache-01.prod.lan", "redis-cluster.dc2.internal", "elasticache.prod.internal"}
	fakeRedisPass   = []string{"r3d1s_s3cr3t", "R$cache_2025!", "redis_pr0d_x7m", "cl0ster_P@ss!"}
	fakeAwsKeys     = []string{"AKIAI44QH8DHBEXAAEEE", "AKIAZ7WBVG3KUPELSKM", "AKIAR9BNXCVOQEIRJMRE", "AKIAJ5MUEXAMPLEEXAM"}
	fakeAwsSecrets  = []string{"wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY", "je7MtGbClwBF/2Zp9Utk/h3yCo8nvbEXAMPLEKEY", "7FbxkYmTR3Qn4Nf0lExAmpLeKey9xBz2EXAMPLEKE"}
	fakeAwsBuckets  = []string{"portal-uploads-prod", "app-assets-production", "backup-data-2025", "static-cdn-prod"}
	fakeMailHosts   = []string{"smtp.internal.local", "mail.corp.local", "relay-01.prod.internal", "postfix.dc1.lan"}
	fakeMailUsers   = []string{"noreply@internal.local", "system@corp.local", "alerts@prod.internal", "deploy@infra.local"}
	fakeMailPass    = []string{"m@1l_s3cr3t!", "Smtp_Pr0d#22", "r3lay_p@ss!", "m4il_Svc_K3y"}
	fakeJwtSecrets  = []string{"c2VjcmV0X2p3dF90b2tlbl9mb3JfaW50ZXJuYWxfdXNl", "aG9uZXlwb3Rfc2VjcmV0X2tleV8yMDI1", "and0X3N1cGVyX3NlY3JldF9wcm9k"}
	fakeApiKeys     = []string{"sk-proj-f4k3-k3y-d0-n0t-us3-1n-pr0duct10n", "sk-live-4b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e", "sk-prod-x7m9k2p4-real-looking-key"}
	fakeEmails      = []string{"admin@internal.local", "root@corp.local", "sysadmin@prod.internal", "devops@infra.local"}
	fakeDeployMails = []string{"deploy@internal.local", "ci-bot@corp.local", "release@infra.local", "automation@prod.internal"}
	fakeDirFiles    = [][]string{
		{"database_backup_2025.sql.gz", "id_rsa", ".htpasswd", "wp-config.php.bak", "credentials.json", "deploy_key.pem", "admin_users.csv", "debug.log", "shadow", "passwd"},
		{"prod_dump.sql.gz", "id_ed25519", ".env.production", "config.php.old", "secrets.yaml", "tls-cert.pem", "users_export.csv", "error.log", "authorized_keys", "known_hosts"},
		{"backup_full_jan2025.tar.gz", "private.key", ".npmrc", "settings.py.bak", "vault-token.json", "ca-bundle.crt", "payment_records.csv", "access.log", "master.key", "database.yml"},
	}
)

// slowTricks hold a goroutine for an extended period to waste the attacker's
// connection pool. Every loop checks r.Context().Done() so the goroutine exits
// the moment the client disconnects or the server shuts down.
var slowTricks = []func(w http.ResponseWriter, r *http.Request, logger *zap.Logger){
	// Tarpit: drip-feed bytes to tie up the bot's connection pool
	func(w http.ResponseWriter, r *http.Request, logger *zap.Logger) {
		logger.Debug("HoneyPotMiddleware: mindFuck tactic=tarpit")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		flusher, canFlush := w.(http.Flusher)
		ctx := r.Context()
		deadline := time.Now().Add(30 * time.Second)
		junk := []byte("<!-- please wait -->\n")
		timer := time.NewTimer(0)
		<-timer.C // drain initial fire
		defer timer.Stop()
		for time.Now().Before(deadline) {
			select {
			case <-ctx.Done():
				return
			default:
			}
			if _, err := w.Write(junk[:1+rand.Intn(len(junk)-1)]); err != nil {
				return
			}
			if canFlush {
				flusher.Flush()
			}
			timer.Reset(time.Duration(500+rand.Intn(1500)) * time.Millisecond)
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
			}
		}
	},

	// Infinite chunked response: slowly stream a never-ending page
	func(w http.ResponseWriter, r *http.Request, logger *zap.Logger) {
		logger.Debug("HoneyPotMiddleware: mindFuck tactic=infiniteChunked")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Transfer-Encoding", "chunked")
		w.WriteHeader(http.StatusOK)
		flusher, canFlush := w.(http.Flusher)
		ctx := r.Context()
		_, _ = w.Write([]byte("<html><body><pre>\n"))
		deadline := time.Now().Add(30 * time.Second)
		timer := time.NewTimer(0)
		<-timer.C // drain initial fire
		defer timer.Stop()
		for time.Now().Before(deadline) {
			select {
			case <-ctx.Done():
				return
			default:
			}
			line := fmt.Sprintf("[%s] processing request... please wait\n", time.Now().Format(time.RFC3339))
			if _, err := w.Write([]byte(line)); err != nil {
				return
			}
			if canFlush {
				flusher.Flush()
			}
			timer.Reset(time.Duration(1000+rand.Intn(2000)) * time.Millisecond)
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
			}
		}
	},

	// Garbage blob: stream random bytes disguised as a backup file
	func(w http.ResponseWriter, r *http.Request, logger *zap.Logger) {
		logger.Debug("HoneyPotMiddleware: mindFuck tactic=garbageBlob")
		contentTypes := []string{
			"application/zip",
			"application/sql",
			"application/octet-stream",
			"application/gzip",
		}
		w.Header().Set("Content-Type", contentTypes[rand.Intn(len(contentTypes))])
		w.Header().Set("Content-Disposition", "attachment; filename=\"backup.sql.gz\"")
		w.WriteHeader(http.StatusOK)
		flusher, canFlush := w.(http.Flusher)
		ctx := r.Context()
		totalBytes := (1 + rand.Intn(4)) * 1024 * 1024
		chunk := make([]byte, 4096)
		crnd.Read(chunk)
		sent := 0
		timer := time.NewTimer(0)
		<-timer.C
		defer timer.Stop()
		for sent < totalBytes {
			select {
			case <-ctx.Done():
				return
			default:
			}
			n, err := w.Write(chunk)
			if err != nil {
				return
			}
			sent += n
			if canFlush {
				flusher.Flush()
			}
			// throttle: ~16-64 KB/s to keep the connection tied up
			timer.Reset(time.Duration(50+rand.Intn(200)) * time.Millisecond)
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
			}
		}
	},
}

// fastTricks respond instantly — zero goroutine cost beyond a normal request.
var fastTricks = []func(w http.ResponseWriter, r *http.Request, logger *zap.Logger){
	// Fake login page that POSTs to another honeypot path
	func(w http.ResponseWriter, _ *http.Request, logger *zap.Logger) {
		logger.Debug("HoneyPotMiddleware: mindFuck tactic=fakeLogin")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<!DOCTYPE html><html><head><title>Admin Login</title></head><body>
<h2>Admin Panel</h2>
<form method="POST" action="/wp-admin/login.php">
<label>Username:</label><input type="text" name="user"><br>
<label>Password:</label><input type="password" name="pass"><br>
<button type="submit">Log In</button>
</form></body></html>`))
	},

	// Redirect loop: send the bot on a wild goose chase
	func(w http.ResponseWriter, _ *http.Request, logger *zap.Logger) {
		logger.Debug("HoneyPotMiddleware: mindFuck tactic=redirectLoop")
		decoys := []string{
			"/admin/setup", "/install.php", "/backup/db", "/config/debug",
			"/.env.production.local", "/api/v1/internal/debug", "/server-status",
		}
		target := decoys[rand.Intn(len(decoys))]
		w.Header().Set("Location", target)
		w.WriteHeader(http.StatusFound)
	},

	// Fake directory listing full of juicy-looking files
	func(w http.ResponseWriter, _ *http.Request, logger *zap.Logger) {
		logger.Debug("HoneyPotMiddleware: mindFuck tactic=fakeDirectoryListing")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		files := fakeDirFiles[rand.Intn(len(fakeDirFiles))]
		var b strings.Builder
		b.WriteString("<html><head><title>Index of /backup/</title></head><body><h1>Index of /backup/</h1><hr><pre>\n")
		for _, f := range files {
			size := 1024 + rand.Intn(10485760)
			pad := 40 - len(f)
			if pad < 1 {
				pad = 1
			}
			b.WriteString(fmt.Sprintf("<a href=\"/backup/%s\">%s</a>%s%s %12d\n",
				f, f, strings.Repeat(" ", pad), "01-Jan-2025 12:00", size))
		}
		b.WriteString("</pre><hr></body></html>")
		_, _ = w.Write([]byte(b.String()))
	},

	// Fake 500 with stack trace leaking juicy-looking internal info
	func(w http.ResponseWriter, _ *http.Request, logger *zap.Logger) {
		logger.Debug("HoneyPotMiddleware: mindFuck tactic=fakeStackTrace")
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = fmt.Fprintf(w, `FATAL ERROR: Uncaught PDOException: SQLSTATE[HY000] [2002] Connection refused
Stack trace:
#0 /var/www/html/includes/db.php(42): PDO->__construct('mysql:host=%s...')
#1 /var/www/html/index.php(15): Database::connect()
#2 {main}

Database host: %s:%s
Database name: %s
Username: %s
`, pick(fakeDbHosts), pick(fakeDbHosts), pick(fakeDbPorts), pick(fakeDbNames), pick(fakeDbUsers))
	},

	// Fake .env file with honeypot credentials
	func(w http.ResponseWriter, _ *http.Request, logger *zap.Logger) {
		logger.Debug("HoneyPotMiddleware: mindFuck tactic=fakeEnvFile")
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, `APP_NAME=InternalPortal
APP_ENV=production
APP_KEY=%s
APP_DEBUG=true
APP_URL=%s

DB_CONNECTION=mysql
DB_HOST=%s
DB_PORT=%s
DB_DATABASE=%s
DB_USERNAME=%s
DB_PASSWORD=%s

REDIS_HOST=%s
REDIS_PASSWORD=%s
REDIS_PORT=6379

AWS_ACCESS_KEY_ID=%s
AWS_SECRET_ACCESS_KEY=%s
AWS_DEFAULT_REGION=us-east-1
AWS_BUCKET=%s

MAIL_MAILER=smtp
MAIL_HOST=%s
MAIL_PORT=587
MAIL_USERNAME=%s
MAIL_PASSWORD=%s
`, pick(fakeAppKeys), pick(fakeAppURLs),
			pick(fakeDbHosts), pick(fakeDbPorts), pick(fakeDbNames), pick(fakeDbUsers), pick(fakeDbPasswords),
			pick(fakeRedisHosts), pick(fakeRedisPass),
			pick(fakeAwsKeys), pick(fakeAwsSecrets), pick(fakeAwsBuckets),
			pick(fakeMailHosts), pick(fakeMailUsers), pick(fakeMailPass))
	},

	// Fake robots.txt that lures bots to more honeypot paths
	func(w http.ResponseWriter, _ *http.Request, logger *zap.Logger) {
		logger.Debug("HoneyPotMiddleware: mindFuck tactic=fakeRobotsTxt")
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`User-agent: *
Disallow: /admin/
Disallow: /api/v1/internal/
Disallow: /backup/
Disallow: /config/
Disallow: /debug/
Disallow: /deploy/
Disallow: /internal/
Disallow: /staging-api/
Disallow: /wp-admin/
Disallow: /.git/
Disallow: /phpmyadmin/
Disallow: /server-status/
Disallow: /grafana/
Disallow: /kibana/
Sitemap: https://internal.local/sitemap.xml
`))
	},

	// Fake API JSON response leaking tokens and user data
	func(w http.ResponseWriter, _ *http.Request, logger *zap.Logger) {
		logger.Debug("HoneyPotMiddleware: mindFuck tactic=fakeApiResponse")
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, `{
  "status": "ok",
  "debug": true,
  "version": "3.2.1-internal",
  "auth": {
    "jwt_secret": "%s",
    "api_key": "%s",
    "admin_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJhZG1pbiIsInJvbGUiOiJzdXBlcmFkbWluIn0.fake"
  },
  "database": {
    "host": "%s",
    "port": %s,
    "name": "%s",
    "user": "%s"
  },
  "users_sample": [
    {"id": 1, "email": "%s", "role": "superadmin"},
    {"id": 2, "email": "%s", "role": "deployer"}
  ]
}`, pick(fakeJwtSecrets), pick(fakeApiKeys),
			pick(fakeDbHosts), pick(fakeDbPorts), pick(fakeDbNames), pick(fakeDbUsers),
			pick(fakeEmails), pick(fakeDeployMails))
	},
}

type HoneyPotResponseConfig struct {
	TricksterMode bool   // Will reply with random tricks designed to waste attacker time; if set, statusCode and body are ignored
	MaxSlowTricks int    //Max number of slow tricks to use concurrently
	StatusCode    int    // HTTP status code to return for honeypot hits
	Body          string // Response body to return for honeypot hits
}

type HoneyPotConfig struct {
	CacheOpts *util.CacheOptions
	Paths     []string
	Response  HoneyPotResponseConfig
}

type honeyPotCacheEntry struct {
	ip       string
	lastSeen time.Time
}

func (h *honeyPotCacheEntry) Touch() {
	h.lastSeen = time.Now()
}

func (h *honeyPotCacheEntry) GetLastReadAt() time.Time {
	return h.lastSeen
}

type HoneyPotServer interface {
	AddIpToHoneyPot(ip string)
	IsHoneyPottedIp(ip string) bool
	IsHoneyPotPath(path string) bool
	Serve(w http.ResponseWriter, r *http.Request, logger *zap.Logger)
}

type TricksterServer struct {
	slowTricksSem chan struct{}
	cachedIps     *util.Cache[*honeyPotCacheEntry] //any cached entry means the IP is currently banned
	paths         []string
}

type StaticResponseServer struct {
	cachedIps  *util.Cache[*honeyPotCacheEntry] //any cached entry means the IP is currently banned
	statusCode int
	body       string
	paths      []string
}

func NewHoneyPotServer(honeyPotConf *HoneyPotConfig) (HoneyPotServer, error) {
	cache, err := util.NewCache[*honeyPotCacheEntry](honeyPotConf.CacheOpts)
	if err != nil {
		return nil, err
	}
	if honeyPotConf.Response.TricksterMode {
		return &TricksterServer{
			slowTricksSem: make(chan struct{}, honeyPotConf.Response.MaxSlowTricks),
			cachedIps:     cache,
			paths:         honeyPotConf.Paths,
		}, nil
	}
	return &StaticResponseServer{
		cachedIps:  cache,
		statusCode: honeyPotConf.Response.StatusCode,
		body:       honeyPotConf.Response.Body,
		paths:      honeyPotConf.Paths,
	}, nil

}

// Serve picks a random trick and executes it. Slow tricks are gated
// behind a bounded semaphore (defaultMaxSlowTricks). If all slow slots are in
// use, a fast trick is served instead — zero extra goroutine cost.
func (t *TricksterServer) Serve(w http.ResponseWriter, r *http.Request, logger *zap.Logger) {
	if rand.Intn(2) == 0 {
		select {
		case t.slowTricksSem <- struct{}{}:
			defer func() { <-t.slowTricksSem }()
			trick := slowTricks[rand.Intn(len(slowTricks))]
			trick(w, r, logger)
			return
		default:
			logger.Debug("HoneyPotMiddleware: slow trick slots full, falling back to fast trick")
		}
	}
	trick := fastTricks[rand.Intn(len(fastTricks))]
	trick(w, r, logger)
}

func (h *TricksterServer) IsHoneyPotPath(path string) bool {
	return isHoneyPotPath(h.paths, path)
}

func (h *TricksterServer) AddIpToHoneyPot(ip string) {
	addIpToHoneyPot(h.cachedIps, ip)
}

func (h *TricksterServer) IsHoneyPottedIp(ip string) bool {
	return isHoneyPottedIp(h.cachedIps, ip)
}

//<------------ static

func (t *StaticResponseServer) Serve(w http.ResponseWriter, _ *http.Request, _ *zap.Logger) {
	w.WriteHeader(t.statusCode)
	if t.body != "" {
		_, _ = w.Write([]byte(t.body))
	}
}

func (h *StaticResponseServer) IsHoneyPotPath(path string) bool {
	return isHoneyPotPath(h.paths, path)
}

func (h *StaticResponseServer) AddIpToHoneyPot(ip string) {
	addIpToHoneyPot(h.cachedIps, ip)
}

func (h *StaticResponseServer) IsHoneyPottedIp(ip string) bool {
	return isHoneyPottedIp(h.cachedIps, ip)
}

//static functions, they are the same for static and trickster, so it makes sense to do it

func isHoneyPotPath(paths []string, path string) bool {
	for _, p := range paths {
		if strings.HasPrefix(path, p) {
			return true
		}
	}
	return false
}

func addIpToHoneyPot(cache *util.Cache[*honeyPotCacheEntry], ip string) {
	entry := &honeyPotCacheEntry{
		ip:       ip,
		lastSeen: time.Now(),
	}
	cache.CacheValue(ip, entry)
}

func isHoneyPottedIp(cache *util.Cache[*honeyPotCacheEntry], ip string) bool {
	_, err := cache.GetValue(ip)
	return err == nil
}
