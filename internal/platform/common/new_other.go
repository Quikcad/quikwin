//go:build !linux && !windows && !darwin

package common

import "errors"

func newPlatform(_ *Config) (any, error) {
	return nil, errors.New("quikwin: unsupported platform")
}
