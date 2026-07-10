package api

import (
	"fmt"

	"github.com/mtzanidakis/dodo/internal/config"
)

func Serve(cfg config.Config, version, commit string) error {
	_ = cfg
	_ = version
	_ = commit
	return fmt.Errorf("not implemented")
}
