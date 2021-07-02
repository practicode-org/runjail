package rules

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
)

type Limits struct {
	AddressSpace    uint64  `json:"address_space_mb"` // Mb
	RunTime         float32 `json:"run_time_sec"`     // seconds
	FileDescriptors uint64  `json:"file_descriptors"` // count
	FileWrites      uint64  `json:"file_writes_mb"`   // Mb
	Threads         uint64  `json:"threads"`          // count
	Output          uint64  `json:"output_bytes"`     // bytes
}

type Stage struct {
	Name    string  `json:"name"`
	Command string  `json:"command"`
	Limits  *Limits `json:"limits"`
}

type Rules struct {
	SourcesDir            string  `json:"sources_dir"`
	SourcesSizeLimitBytes uint64  `json:"sources_size_limit_bytes"` // Bytes
	Stages                []Stage `json:"stages"`
}

//
func (r *Rules) Check() error {
	if r.SourcesDir == "" {
		return fmt.Errorf("SourcesDir can't be empty")
	}

	// check sources directory
	stat, err := os.Stat(r.SourcesDir)
	if err != nil {
		return fmt.Errorf("failed to get stats of SourcesDir: %s", r.SourcesDir)
	}
	if !stat.IsDir() {
		return fmt.Errorf("SourcesDir is not a directory: %s", r.SourcesDir)
	}
	if stat.Mode()&0666 != 0666 { // need rwxrwrwx rights
		return fmt.Errorf("SourcesDir has unsuitable permissions: %v", stat.Mode())
	}

	//
	if r.SourcesSizeLimitBytes == 0 {
		return fmt.Errorf("SourcesSizeLimitBytes can't be zero")
	} else if r.SourcesSizeLimitBytes < 1024 {
		log.Warningf("SourcesSizeLimitBytes %d seems very low\n", r.SourcesSizeLimitBytes)
	} else if r.SourcesSizeLimitBytes > 1024*1024*10 {
		log.Warningf("SourcesSizeLimitBytes %d seems too high\n", r.SourcesSizeLimitBytes)
	}

	// check every stage
	for _, stage := range r.Stages {
		if stage.Name == "" {
			return fmt.Errorf("Stage.Name can't be empty")
		} else if stage.Name == "init" {
			return fmt.Errorf("Stage.Name can't be 'init'")
		}
		if stage.Command == "" {
			return fmt.Errorf("Command can't be empty, stage '%s'", stage.Name)
		}

		// check limits
		limits := stage.Limits

		if limits.AddressSpace == 0 {
			return fmt.Errorf("Limit.AddressSpace can't be zero")
		} else if limits.AddressSpace < 64 {
			log.Warningf("Limit.AddressSpace %d mb seems very low\n", limits.AddressSpace)
		} else if limits.AddressSpace > 512 {
			log.Warningf("Limit.AddressSpace %d mb seems too high\n", limits.AddressSpace)
		}

		if limits.RunTime == 0.0 {
			return fmt.Errorf("Limit.RunTime can't be zero")
		} else if limits.RunTime < 0.5 {
			log.Warningf("Limit.RunTime %.1f sec seems very low\n", limits.RunTime)
		} else if limits.RunTime > 60.0 {
			log.Warningf("Limit.RunTime %.1f sec seems too high\n", limits.RunTime)
		}

		if limits.FileDescriptors == 0 {
			return fmt.Errorf("Limit.FileDescriptors can't be zero")
		} else if limits.FileDescriptors < 3 {
			log.Warningf("Limit.FileDescriptors %d seems very low\n", limits.FileDescriptors)
		} else if limits.FileDescriptors > 512 {
			log.Warningf("Limit.FileDescriptors %d seems too high\n", limits.FileDescriptors)
		}

		if limits.FileWrites == 0 {
			return fmt.Errorf("Limit.FileWrites can't be zero")
		} else if limits.FileWrites < 1 {
			log.Warningf("Limit.FileWrites %d mb seems very low\n", limits.FileWrites)
		} else if limits.FileWrites > 100 {
			log.Warningf("Limit.FileWrites %d mb seems too high\n", limits.FileWrites)
		}

		if limits.Threads == 0 {
			return fmt.Errorf("Limit.Threads can't be zero")
		} else if limits.Threads < 64 {
			log.Warningf("Limit.Threads %d seems very low\n", limits.Threads)
		} else if limits.Threads > 2000 {
			log.Warningf("Limit.Threads %d seems too high\n", limits.Threads)
		}

		if limits.Output == 0 {
			return fmt.Errorf("Limit.Output can't be zero")
		} else if limits.Output < 1024 {
			log.Warningf("Limit.Output %d bytes seems very low\n", limits.Output)
		} else if limits.Output > 1024*1024*1024 { // 1 mb
			log.Warningf("Limit.Output %d bytes seems too high\n", limits.Output)
		}
	}
	return nil
}

//
func ReadJsonRules(data []byte) (*Rules, error) {
	rules := &Rules{}
	err := json.Unmarshal(data, rules)
	if err != nil {
		return nil, err
	}

	if rules.SourcesDir == "" {
		log.Fatal("Misconfig: Empty sources_dir")
	}

	err = rules.Check()
	if err != nil {
		return nil, err
	}
	return rules, nil
}

func LoadRules(filePath string) (*Rules, error) {
	bytes, err := ioutil.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open rules file: %w", err)
	}

	ext := filepath.Ext(filePath)
	if ext == ".yaml" || ext == ".yml" {
		pipeline := &Rules{}
		err := yaml.Unmarshal(bytes, pipeline)
		if err != nil {
			return nil, fmt.Errorf("failed to load rules from yaml file: %w", err)
		}
		return pipeline, nil
	}

	return ReadJsonRules(bytes)
}
