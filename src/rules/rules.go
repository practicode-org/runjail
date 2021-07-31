package rules

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"path"
	"path/filepath"
	"strings"

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
	Name    string   `json:"name"`
	Command string   `json:"command"`
	Env     []string `json:"env"`
	Mounts  []string `json:"mounts"`
	Limits  *Limits  `json:"limits"`
}

type Rules struct {
	Stages []Stage `json:"stages"`
}

var (
	RulesMap = make(map[string]*Rules)
)

//
func (r *Rules) Check() error {
	// check every stage
	// TODO: print rule name
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

func LoadRules(filesDir string) error {
	files, err := ioutil.ReadDir(filesDir)
	if err != nil {
		return fmt.Errorf("failed to ReadDir: %w", err)
	}

	ruleNames := make([]string, 0)

	for _, file := range files {
		ext := filepath.Ext(file.Name())
		if ext != ".yaml" && ext != ".yml" && ext != ".json" {
			continue
		}

		filePath := path.Join(filesDir, file.Name())
		bytes, err := ioutil.ReadFile(filePath)
		if err != nil {
			return fmt.Errorf("failed to open rules file %s: %w", file.Name(), err)
		}

		rules := &Rules{}

		if ext == ".json" {
			err = json.Unmarshal(bytes, rules)
		} else {
			err = yaml.Unmarshal(bytes, rules)
		}
		if err != nil {
			return fmt.Errorf("failed to unmarshal rules file %s: %w", file.Name(), err)
		}

		err = rules.Check()
		if err != nil {
			return fmt.Errorf("error in rules file %s: %w", file.Name(), err)
		}

		shortName := strings.TrimSuffix(file.Name(), ext)
		if len(shortName) == 0 {
			return fmt.Errorf("failed to trim suffix %s for file %s", ext, file.Name())
		}

		RulesMap[shortName] = rules
		ruleNames = append(ruleNames, shortName)
	}

	if len(RulesMap) > 0 {
		log.Infof("Loaded %d rules from %s: [%s]\n", len(RulesMap), filesDir, strings.Join(ruleNames, ", "))
	} else {
		log.Warningf("No rules found in %s\n", filesDir)
	}
	return nil
}
