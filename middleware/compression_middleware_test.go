package middleware

import (
	"compress/flate"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/klauspost/compress/gzip"
	"github.com/klauspost/compress/zlib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testBody = "Hello, compressed world! This is some text to compress."

func compressedHandler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, err := w.Write([]byte(testBody))
		require.NoError(t, err)
	}
}

func TestParseCompressionsOptions_MissingType(t *testing.T) {
	conf := Config{Type: "Compression", Options: map[string]interface{}{}}
	_, _, err := parseCompressionsOptions(conf)
	assert.Error(t, err)
}

func TestParseCompressionsOptions_InvalidType(t *testing.T) {
	conf := Config{Type: "Compression", Options: map[string]interface{}{"type": "brotli"}}
	_, _, err := parseCompressionsOptions(conf)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid compression type")
}

func TestParseCompressionsOptions_Gzip(t *testing.T) {
	conf := Config{Type: "Compression", Options: map[string]interface{}{"type": "gzip"}}
	_, fn, err := parseCompressionsOptions(conf)
	require.NoError(t, err)
	assert.NotNil(t, fn)
}

func TestParseCompressionsOptions_Zlib(t *testing.T) {
	conf := Config{Type: "Compression", Options: map[string]interface{}{"type": "zlib"}}
	_, fn, err := parseCompressionsOptions(conf)
	require.NoError(t, err)
	assert.NotNil(t, fn)
}

func TestParseCompressionsOptions_InvalidLevel(t *testing.T) {
	conf := Config{Type: "Compression", Options: map[string]interface{}{"type": "gzip", "level": float64(999)}}
	_, _, err := parseCompressionsOptions(conf)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid gzip compression level")
}

func TestParseCompressionsOptions_InvalidLevelZlib(t *testing.T) {
	conf := Config{Type: "Compression", Options: map[string]interface{}{"type": "zlib", "level": float64(999)}}
	_, _, err := parseCompressionsOptions(conf)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid zlib compression level")
}

func TestCompressionMiddleware_Gzip(t *testing.T) {
	conf := Config{
		Type:    "gzip",
		Options: map[string]interface{}{"type": "gzip", "level": float64(gzip.BestSpeed)},
	}
	handler := CompressionMiddleware(context.Background(), compressedHandler(t), conf)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "gzip", rec.Header().Get("Content-Encoding"))

	gr, err := gzip.NewReader(rec.Body)
	require.NoError(t, err)
	defer gr.Close()

	body, err := io.ReadAll(gr)
	require.NoError(t, err)
	assert.Equal(t, testBody, string(body))
}

func TestCompressionMiddleware_Zlib(t *testing.T) {
	conf := Config{
		Type:    "zlib",
		Options: map[string]interface{}{"type": "zlib", "level": float64(flate.BestSpeed)},
	}
	handler := CompressionMiddleware(context.Background(), compressedHandler(t), conf)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept-Encoding", "zlib")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "zlib", rec.Header().Get("Content-Encoding"))

	zr, err := zlib.NewReader(rec.Body)
	require.NoError(t, err)
	defer zr.Close()

	body, err := io.ReadAll(zr)
	require.NoError(t, err)
	assert.Equal(t, testBody, string(body))
}

func TestCompressionMiddleware_NoAcceptEncoding(t *testing.T) {
	conf := Config{
		Type:    "gzip",
		Options: map[string]interface{}{"type": "gzip"},
	}
	handler := CompressionMiddleware(context.Background(), compressedHandler(t), conf)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	// No Accept-Encoding header
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Empty(t, rec.Header().Get("Content-Encoding"))
	assert.Equal(t, testBody, rec.Body.String())
}

func TestCompressionResponseWriter_Header(t *testing.T) {
	rec := httptest.NewRecorder()
	gz, _ := gzip.NewWriterLevel(rec, gzip.DefaultCompression)
	crw := &CompressionResponseWriter{w: rec, compressorWriter: gz}

	crw.Header().Set("X-Test", "value")
	assert.Equal(t, "value", rec.Header().Get("X-Test"))
}

func TestCompressionResponseWriter_WriteHeader(t *testing.T) {
	rec := httptest.NewRecorder()
	gz, _ := gzip.NewWriterLevel(rec, gzip.DefaultCompression)
	crw := &CompressionResponseWriter{w: rec, compressorWriter: gz}

	crw.WriteHeader(http.StatusCreated)
	assert.Equal(t, http.StatusCreated, rec.Code)
}

func TestCompressionResponseWriter_Unwrap(t *testing.T) {
	rec := httptest.NewRecorder()
	gz, _ := gzip.NewWriterLevel(rec, gzip.DefaultCompression)
	crw := &CompressionResponseWriter{w: rec, compressorWriter: gz}

	assert.Equal(t, rec, crw.Unwrap())
}

func TestCompressionResponseWriter_Flush(t *testing.T) {
	rec := httptest.NewRecorder()
	gz, _ := gzip.NewWriterLevel(rec, gzip.DefaultCompression)
	crw := &CompressionResponseWriter{w: rec, compressorWriter: gz}

	_, err := crw.Write([]byte("flush me"))
	require.NoError(t, err)

	crw.Flush()
	// After flush, compressed bytes should have been written to the recorder
	assert.Greater(t, rec.Body.Len(), 0)
}
