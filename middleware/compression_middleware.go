package middleware

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/klauspost/compress/gzip"
	"github.com/klauspost/compress/zlib"
	"go.uber.org/zap"
)

type compressInitFunc func(w http.ResponseWriter) *CompressionResponseWriter

type CompressionResponseWriter struct {
	w                http.ResponseWriter
	compressorWriter io.WriteCloser
}

func (c *CompressionResponseWriter) Header() http.Header {
	return c.w.Header()
}

func (c *CompressionResponseWriter) WriteHeader(statusCode int) {
	c.w.WriteHeader(statusCode)
}

func (c *CompressionResponseWriter) Write(b []byte) (int, error) {
	return c.compressorWriter.Write(b)
}

func (c *CompressionResponseWriter) Close() error {
	return c.compressorWriter.Close()
}

func (c *CompressionResponseWriter) Flush() {
	// Flush the compressor's internal buffers to the underlying writer first
	if f, ok := c.compressorWriter.(interface{ Flush() error }); ok {
		_ = f.Flush()
	}
	if f, ok := c.w.(http.Flusher); ok {
		f.Flush()
	}
}

func (c *CompressionResponseWriter) Unwrap() http.ResponseWriter {
	return c.w
}

type lazyCompressionWriter struct {
	w        http.ResponseWriter
	initFunc compressInitFunc
	confType string
	logger   *zap.Logger
	crw      *CompressionResponseWriter
	passthru bool
	decided  bool
}

// decide inspects the current response headers and chooses whether to compress.
// It must be called exactly once, after headers are finalized (i.e. in WriteHeader).
func (lw *lazyCompressionWriter) decide() {
	if lw.decided {
		return
	}
	lw.decided = true

	if lw.w.Header().Get("Content-Encoding") != "" {
		// Upstream already compressed — don't double-compress
		lw.passthru = true
		lw.logger.Debug("Upstream already set Content-Encoding, skipping compression")
		return
	}
	lw.w.Header().Set("Content-Encoding", lw.confType)
	lw.w.Header().Del("Content-Length")
	lw.crw = lw.initFunc(lw.w)
}

func (lw *lazyCompressionWriter) Header() http.Header {
	return lw.w.Header()
}

func (lw *lazyCompressionWriter) WriteHeader(statusCode int) {
	lw.decide()
	lw.w.WriteHeader(statusCode)
}

func (lw *lazyCompressionWriter) Write(b []byte) (int, error) {
	// If WriteHeader was never called explicitly, Go would send a 200.
	// we trigger our decision here so headers are set before the implicit flush.
	if !lw.decided {
		lw.WriteHeader(http.StatusOK)
	}
	if lw.passthru {
		return lw.w.Write(b)
	}
	return lw.crw.Write(b)
}

func (lw *lazyCompressionWriter) Flush() {
	if !lw.decided {
		// Nothing written yet, nothing to flush
		return
	}
	if lw.crw != nil {
		lw.crw.Flush()
	} else if f, ok := lw.w.(http.Flusher); ok {
		f.Flush()
	}
}

func (lw *lazyCompressionWriter) Unwrap() http.ResponseWriter {
	return lw.w
}

func CompressionMiddleware(_ context.Context, next http.HandlerFunc, conf Config) http.HandlerFunc {
	cType, initFunc, err := parseCompressionsOptions(conf)
	if err != nil {
		zap.S().Errorf("CompressionMiddleware failed to parse configuration: %v. Failing closed.", err)
		return func(writer http.ResponseWriter, request *http.Request) {
			http.Error(writer, "CompressionMiddleware misconfigured", http.StatusServiceUnavailable)
		}
	}

	return func(w http.ResponseWriter, r *http.Request) {
		logger := GetRequestLoggerFromContext(r)
		if !strings.Contains(r.Header.Get("Accept-Encoding"), conf.Type) {
			logger.Debug("Client does not support compression, skipping compression middleware")
			next(w, r)
			return
		}

		lw := &lazyCompressionWriter{
			w:        w,
			initFunc: initFunc,
			confType: cType,
			logger:   logger,
		}
		defer func() {
			if lw.crw != nil {
				if err := lw.crw.Close(); err != nil {
					logger.Error("Failed to close compressor writer", zap.Error(err))
				}
			}
		}()

		next(lw, r)
	}
}

func parseCompressionsOptions(conf Config) (string, compressInitFunc, error) {
	zap.S().Debugf("Parsing compression options: %v", conf)

	compressionLevel := ParseIntOpt(conf.Options, "level", gzip.BestCompression)

	typeComp, err := ParseStringRequired(conf.Options, "type")
	if err != nil {
		return "", nil, err
	}

	typeComp = strings.ToLower(typeComp)

	if typeComp != "gzip" && typeComp != "zlib" {
		return "", nil, fmt.Errorf("invalid compression type: %s", typeComp)
	}

	if typeComp == "gzip" {
		gzTest, err := gzip.NewWriterLevel(io.Discard, compressionLevel)
		if err != nil {
			return "", nil, fmt.Errorf("invalid gzip compression level %d: %w", compressionLevel, err)
		}
		err = gzTest.Close()
		if err != nil {
			zap.S().Warnf("Failed to close gzip test writer: %v", err)
		}
		return typeComp, func(w http.ResponseWriter) *CompressionResponseWriter {
			gz, _ := gzip.NewWriterLevel(w, compressionLevel)
			return &CompressionResponseWriter{w: w, compressorWriter: gz}
		}, nil
	}

	zw, err := zlib.NewWriterLevel(io.Discard, compressionLevel)
	if err != nil {
		return "", nil, fmt.Errorf("invalid zlib compression level %d: %w", compressionLevel, err)
	}
	err = zw.Close()
	if err != nil {
		zap.S().Warnf("Failed to close zlib test writer: %v", err)
	}
	return typeComp, func(w http.ResponseWriter) *CompressionResponseWriter {
		zw, _ := zlib.NewWriterLevel(w, compressionLevel)
		return &CompressionResponseWriter{w: w, compressorWriter: zw}
	}, nil
}
