//go:build darwin

package common

import "github.com/Quikcad/quikwin/internal/platform/cocoa"

func newPlatform(cfg *Config) (any, error) {
	return cocoa.New(cfg)
}
