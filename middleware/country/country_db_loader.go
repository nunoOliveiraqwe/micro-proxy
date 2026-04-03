package country

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/nunoOliveiraqwe/torii/internal/fsutil"
	"github.com/nunoOliveiraqwe/torii/internal/util"
	"go.uber.org/zap"
)

type DbLoader interface {
	isRefreshable() bool
	load() ([]byte, error)
}

type staticFileDbLoader struct {
	pathToFile string
}

func NewStaticFileDbLoader(filePath string) DbLoader {
	return &staticFileDbLoader{pathToFile: filePath}
}

type downloadDbLoader struct {
	url              string
	maxFileSizeBytes int64
}

func NewDownloadDbLoader(url string, maxSize string) (DbLoader, error) {
	size, err := util.ParseSizeString(maxSize)
	if err != nil {
		return nil, err
	}
	return &downloadDbLoader{
		url:              url,
		maxFileSizeBytes: size,
	}, nil
}

func (s *staticFileDbLoader) isRefreshable() bool {
	return false
}

func (s *staticFileDbLoader) load() ([]byte, error) {
	zap.S().Debug("Resolving country db from static file at path: %s", s.pathToFile)
	fileExists := fsutil.FileExists(s.pathToFile)
	if !fileExists {
		zap.S().Errorf("Country db file does not exist at path: %s", s.pathToFile)
		return nil, fmt.Errorf("country db file does not exist at path: %s", s.pathToFile)
	}
	file, err := os.ReadFile(s.pathToFile)
	if err != nil {
		zap.S().Errorf("Failed to read country db file at path: %s, error: %v", s.pathToFile, err)
		return nil, err
	}
	return file, nil
}

func (d *downloadDbLoader) isRefreshable() bool {
	return true
}

func (d *downloadDbLoader) load() ([]byte, error) {
	zap.S().Debugf("Resolving country db from download at url: %s", d.url)
	c := http.Client{
		Timeout: 30 * time.Second,
	}
	resp, err := c.Get(d.url)
	if err != nil {
		zap.S().Errorf("Failed to download country db from url: %s, error: %v", d.url, err)
		return nil, err
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			zap.S().Warnf("Failed to close response body after downloading country db from url: %s, error: %v", d.url, err)
		}
	}(resp.Body)
	if resp.StatusCode != http.StatusOK {
		zap.S().Errorf("Failed to download country db from url: %s, status code: %d", d.url, resp.StatusCode)
		return nil, fmt.Errorf("failed to download country db from url: %s, status code: %d", d.url, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, d.maxFileSizeBytes))
	if err != nil {
		zap.S().Errorf("Failed to read response body after downloading country db from url: %s, error: %v", d.url, err)
		return nil, err
	}
	return body, nil
}
