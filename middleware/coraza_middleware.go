package middleware

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"

	coreruleset "github.com/corazawaf/coraza-coreruleset"
	"github.com/corazawaf/coraza/v3"
	"github.com/corazawaf/coraza/v3/types"
	"github.com/nunoOliveiraqwe/torii/internal/netutil"
	"github.com/nunoOliveiraqwe/torii/metrics"
	"go.uber.org/zap"
)

type wafResponseWriter struct {
	w           http.ResponseWriter
	tx          types.Transaction
	statusCode  int
	wroteHeader bool
}

func (rw *wafResponseWriter) Header() http.Header {
	return rw.w.Header()
}

func (rw *wafResponseWriter) WriteHeader(code int) {
	if rw.wroteHeader {
		return
	}
	rw.wroteHeader = true
	rw.statusCode = code

	for k, vals := range rw.w.Header() {
		for _, v := range vals {
			rw.tx.AddResponseHeader(k, v)
		}
	}

	if it := rw.tx.ProcessResponseHeaders(code, "HTTP/1.1"); it != nil {
		code = it.Status
		rw.statusCode = code
	}

	rw.w.WriteHeader(code)
}

func (rw *wafResponseWriter) Write(b []byte) (int, error) {
	if !rw.wroteHeader {
		rw.WriteHeader(http.StatusOK)
	}
	n, err := rw.w.Write(b)
	if n > 0 {
		_, _, _ = rw.tx.WriteResponseBody(b[:n])
	}
	return n, err
}

func (rw *wafResponseWriter) Unwrap() http.ResponseWriter {
	return rw.w
}

type corazaConfig struct {
	waf            coraza.WAF
	inspectReqBody bool
	inspectResBody bool
}

func CorazaWAFMiddleware(_ context.Context, next http.HandlerFunc, conf Config) http.HandlerFunc {
	cfg, err := parseCorazaConfig(conf)
	if err != nil {
		zap.S().Errorf("CorazaWAFMiddleware: failed to initialize WAF: %v. Failing closed.", err)
		return func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "CorazaWAFMiddleware misconfigured", http.StatusServiceUnavailable)
		}
	}
	return func(w http.ResponseWriter, r *http.Request) {
		logger := GetRequestLoggerFromContext(r)
		tx := cfg.waf.NewTransaction()
		defer func() {
			tx.ProcessLogging()
			_ = tx.Close()
		}()

		clientIP, clientPort := netutil.SplitHostPort(r.RemoteAddr)
		if clientIP == "" {
			logger.Error("CorazaWAFMiddleware: failed to get client IP")
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		serverHost, serverPort := netutil.SplitHostPort(r.Host)
		tx.ProcessConnection(clientIP, clientPort, serverHost, serverPort)
		tx.ProcessURI(r.URL.String(), r.Method, r.Proto)

		for k, vals := range r.Header {
			for _, v := range vals {
				tx.AddRequestHeader(k, v)
			}
		}
		if r.Host != "" {
			tx.AddRequestHeader("Host", r.Host)
		}

		if it := tx.ProcessRequestHeaders(); it != nil {
			metrics.CreateAndAddBlockInfo(r, "coraza-waf",
				fmt.Sprintf("Blocked by Coraza at phase 1: rule %d, status %d", it.RuleID, it.Status))
			w.WriteHeader(it.Status)
			return
		}

		if cfg.inspectReqBody && r.Body != nil && r.Body != http.NoBody {
			bodyBytes, err := io.ReadAll(r.Body)
			_ = r.Body.Close()
			if err != nil {
				logger.Warn("CorazaMiddleware: error reading request body", zap.Error(err))
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			if it, _, err := tx.ReadRequestBodyFrom(bytes.NewReader(bodyBytes)); it != nil {
				metrics.CreateAndAddBlockInfo(r, "coraza-waf",
					fmt.Sprintf("Blocked by Coraza at phase 2: rule %d, status %d", it.RuleID, it.Status))
				w.WriteHeader(it.Status)
				return
			} else if err != nil {
				logger.Warn("CorazaMiddleware: error feeding request body to WAF", zap.Error(err))
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			if it, err := tx.ProcessRequestBody(); it != nil {
				metrics.CreateAndAddBlockInfo(r, "coraza-waf",
					fmt.Sprintf("Blocked by Coraza at phase 3: rule %d, status %d", it.RuleID, it.Status))
				w.WriteHeader(it.Status)
				return
			} else if err != nil {
				logger.Warn("CorazaMiddleware: error processing request body", zap.Error(err))
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			r.Body = io.NopCloser(bytes.NewReader(bodyBytes))
			r.ContentLength = int64(len(bodyBytes))
		}

		wrappedWriter := &wafResponseWriter{w: w, tx: tx}
		next(wrappedWriter, r)

		if cfg.inspectResBody {
			if it, err := tx.ProcessResponseBody(); it != nil {
				metrics.CreateAndAddBlockInfo(r, "coraza-waf",
					fmt.Sprintf("Blocked by Coraza at phase 4: rule %d, status %d", it.RuleID, it.Status))
			} else if err != nil {
				logger.Warn("CorazaMiddleware: error processing response body", zap.Error(err))
			}
		}
	}
}

func parseCorazaConfig(conf Config) (*corazaConfig, error) {
	zap.S().Debug("Parsing Coraza WAF configuration")

	/**
	  coraza-waf:
	  	paranoia-level: 1      # 1-4, default 1
	  	inbound-threshold: 5   # inbound anomaly score threshold
	  	outbound-threshold: 4  # outbound anomaly score threshold
	  	mode: detect           # detect | block
	  	inspect-request-body: false  # buffer and inspect request bodies
	  	inspect-response-body: false # inspect response bodies at phase 4
	  	exclusions:            # list of rule IDs to exclude
	  	   - "12345"
	  	   - "67890"
	**/

	paranoiaLevel := ParseIntOpt(conf.Options, "paranoia-level", 1)
	inboundThreshold := ParseIntOpt(conf.Options, "inbound-threshold", 5)
	outboundThreshold := ParseIntOpt(conf.Options, "outbound-threshold", 4)
	mode, _ := ParseStringOpt(conf.Options, "mode", "detect")
	if mode != "block" && mode != "detect" {
		zap.S().Warnf("Unknown coraza-waf mode %q, defaulting to detect", mode)
		mode = "detect"
	}
	inspectReqBody := ParseBoolOpt(conf.Options, "inspect-request-body", false)
	inspectResBody := ParseBoolOpt(conf.Options, "inspect-response-body", false)
	exclusions, _ := ParseStringSliceOpt(conf.Options, "exclusions", nil)

	directives := fmt.Sprintf(`
        SecRuleEngine %s
        SecAction "id:900000,phase:1,nolog,pass,t:none,setvar:tx.paranoia_level=%d"
        SecAction "id:900110,phase:1,nolog,pass,t:none,setvar:tx.inbound_anomaly_score_threshold=%d,setvar:tx.outbound_anomaly_score_threshold=%d"
    `, secRuleEngine(mode), paranoiaLevel, inboundThreshold, outboundThreshold)

	for _, id := range exclusions {
		directives += fmt.Sprintf("\nSecRuleRemoveById %s", id)
	}

	waf, err := coraza.NewWAF(
		coraza.NewWAFConfig().
			WithDirectives(directives).
			WithRootFS(coreruleset.FS),
	)
	if err != nil {
		return nil, err
	}
	return &corazaConfig{
		waf:            waf,
		inspectReqBody: inspectReqBody,
		inspectResBody: inspectResBody,
	}, nil
}

func secRuleEngine(mode string) string {
	if mode == "block" {
		return "On"
	}
	return "DetectionOnly"
}
