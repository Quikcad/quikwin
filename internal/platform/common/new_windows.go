//go:build windows

package common

import "github.com/Quikcad/quikwin/internal/platform/win32"

func newPlatform(cfg *Config) (any, error) {
	return win32.New(cfg)
}
