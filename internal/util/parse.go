package util

import (
	"fmt"
	"strconv"
	"time"

	"go.uber.org/zap"
)

// ParseSizeString - parses a string with the following format: 1b, 1k, 1m, 1g and returns the size in bytes
func ParseSizeString(maxSize string) (int64, error) {
	zap.S().Debugf("Parsing size string: %s", maxSize)
	if len(maxSize) < 2 {
		return 0, fmt.Errorf("invalid size string: %s", maxSize)
	}
	acceptedSuffixes := map[string]int64{
		"b": 1,
		"k": 1024,
		"m": 1024 * 1024,
		"g": 1024 * 1024 * 1024,
	}
	stringSuffix := maxSize[len(maxSize)-1:]
	numberSuffix := maxSize[:len(maxSize)-1]
	multiplier, ok := acceptedSuffixes[stringSuffix]
	if !ok {
		zap.S().Errorf("Invalid size suffix: %s. Accepted suffixes are: b, k, m, g", stringSuffix)
		return 0, fmt.Errorf("invalid size suffix: %s", stringSuffix)
	}
	number, err := strconv.ParseInt(numberSuffix, 10, 64)
	if err != nil {
		zap.S().Errorf("Invalid number in size string: %s. Error: %v", numberSuffix, err)
		return 0, err
	}
	return number * multiplier, err
}

// ParseTimeString - parses a string with the following format: 1ms,1s,1m,1h
func ParseTimeString(timeStr string) (time.Duration, error) {
	zap.S().Debugf("Parsing time string: %s", timeStr)
	if len(timeStr) < 2 {
		return 0, fmt.Errorf("invalid time string: %s", timeStr)
	}
	acceptedSuffixes := map[string]time.Duration{
		"ms": time.Millisecond,
		"s":  time.Second,
		"m":  time.Minute,
		"h":  time.Hour,
	}
	stringSuffix := timeStr[len(timeStr)-1:]
	numberSuffix := timeStr[:len(timeStr)-1]
	unit, ok := acceptedSuffixes[stringSuffix]
	if !ok {
		zap.S().Errorf("Invalid time suffix: %s. Accepted suffixes are: ms, s, m, h", stringSuffix)
		return 0, fmt.Errorf("invalid time suffix: %s. Accepted suffixes are: ms, s, m, h", stringSuffix)
	}
	number, err := strconv.ParseInt(numberSuffix, 10, 64)
	if err != nil {
		zap.S().Errorf("Invalid number in time string: %s. Error: %v", numberSuffix, err)
		return 0, err
	}
	return time.Duration(number) * unit, err
}
