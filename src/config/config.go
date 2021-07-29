package config

import (
	"fmt"
	"os"

	log "github.com/sirupsen/logrus"
)

type Config struct {
	SourcesDir            string `json:"sources_dir"`
	SourcesSizeLimitBytes uint64 `json:"sources_size_limit_bytes"` // Bytes
}

var (
	Cfg Config
)

func DefaultConfig() error {
	Cfg.SourcesDir = "/tmp/sources"
	Cfg.SourcesSizeLimitBytes = 8000

	// check sources directory
	stat, err := os.Stat(Cfg.SourcesDir)
	if err != nil {
		return fmt.Errorf("failed to get stats of SourcesDir: %s", Cfg.SourcesDir)
	}
	if !stat.IsDir() {
		return fmt.Errorf("SourcesDir is not a directory: %s", Cfg.SourcesDir)
	}
	if stat.Mode()&0666 != 0666 { // need rwxrwrwx rights
		return fmt.Errorf("SourcesDir has unsuitable permissions: %v", stat.Mode())
	}

	//
	if Cfg.SourcesSizeLimitBytes == 0 {
		return fmt.Errorf("SourcesSizeLimitBytes can't be zero")
	} else if Cfg.SourcesSizeLimitBytes < 1024 {
		log.Warningf("SourcesSizeLimitBytes %d seems very low\n", Cfg.SourcesSizeLimitBytes)
	} else if Cfg.SourcesSizeLimitBytes > 1024*1024*10 {
		log.Warningf("SourcesSizeLimitBytes %d seems too high\n", Cfg.SourcesSizeLimitBytes)
	}
	return nil
}
