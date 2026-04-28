package fsutil

import (
	"io"
	"os"

	"go.uber.org/zap"
)

func FileExists(path string) bool {
	zap.S().Debugf("checking if file exists with path: %s", path)
	statInfo, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false
		}
		zap.S().Errorf("error checking if file exists, unknown state: %s", err)
		return false
	}
	return !statInfo.IsDir()
}

func DirExists(path string) bool {
	zap.S().Debugf("checking if dir exists with path: %s", path)
	statInfo, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false
		}
		zap.S().Errorf("error checking if dir exists, unknown state: %s", err)
		return false
	}
	return statInfo.IsDir()
}

func CopyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0640)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}
