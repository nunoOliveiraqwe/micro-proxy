package resolve

import (
	"fmt"
	"os"
	"strings"

	"go.uber.org/zap"
)

type Resolver interface {
	getResolverKey() string
	Resolve(string) (string, error)
}

type FileResolver struct{} //to be used with docker secrets or similar structures, where the path is the key and the content of the file is the value

type EnvResolver struct{}

func (f *FileResolver) Resolve(path string) (string, error) {
	zap.S().Infof("Resolving file path: %s", path)
	b, err := os.ReadFile(path)
	if err != nil {
		zap.S().Errorf("Failed to read file %s: %v", path, err)
		return "", fmt.Errorf("failed to read file %s: %v", path, err)
	}
	return strings.TrimRight(string(b), "\n"), nil //remove trailing newline
}

func (f *FileResolver) getResolverKey() string {
	return "file"
}

func (e *EnvResolver) Resolve(key string) (string, error) {
	zap.S().Infof("Resolving environment variable: %s", key)
	value, exists := os.LookupEnv(key)
	if !exists {
		zap.S().Errorf("Environment variable %s not found", key)
		return "", fmt.Errorf("environment variable %s not found", key)
	}
	return value, nil
}

func (e *EnvResolver) getResolverKey() string {
	return "env"
}
