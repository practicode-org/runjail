package rules

import (
	"fmt"
	"io/ioutil"

	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
)

type Limits struct {
	AddressSpace    uint64  `yaml:"address_space_mb"` // Mb
	RunTime         float32 `yaml:"run_time_sec"`     // seconds
	FileDescriptors uint64  `yaml:"file_descriptors"` // count
	FileWrites      uint64  `yaml:"file_writes_mb"`   // Mb
	Threads         uint64  `yaml:"threads"`          // count
	Output          uint64  `yaml:"output_bytes"`     // bytes
}

type Stage struct {
	Name      string   `yaml:"name"`
	Command   string   `yaml:"command"`
	DependsOn string   `yaml:"depends_on"`
	Env       []string `yaml:"env"`
	Mounts    []string `yaml:"mounts"`
	Limits    *Limits  `yaml:"limits"`
}

type BuildStages struct {
	Stages   []Stage `yaml:"stages"`
	StageMap map[string]*Stage
}

var buildStages *BuildStages = nil

//
func StagesForTarget(target string) ([]*Stage, error) {
	curTarget := target
	result := make([]*Stage, 0)

	for curTarget != "" {
		stage, ok := buildStages.StageMap[curTarget]
		if !ok {
			return nil, fmt.Errorf("target %s is not found", curTarget)
		}
		result = append([]*Stage{stage}, result...)

		if stage.DependsOn == target {
			return nil, fmt.Errorf("circular reference to itself at stage %s found", stage.Name)
		}
		if len(result) > 64 {
			return nil, fmt.Errorf("to many stages are dependants of the target %s, circular reference?", target)
		}

		curTarget = stage.DependsOn
	}
	return result, nil
}

func (r *BuildStages) Check() error {
	for _, stage := range r.Stages {
		if stage.Name == "" {
			return fmt.Errorf("Stage.Name can't be empty, stage '%s'", stage.Name)
		} else if stage.Name == "init" {
			return fmt.Errorf("Stage.Name can't be 'init', stage '%s'", stage.Name)
		}
		if stage.Command == "" {
			return fmt.Errorf("Command can't be empty, stage '%s'", stage.Name)
		}

		limits := stage.Limits

		if limits.AddressSpace == 0 {
			return fmt.Errorf("Limit.AddressSpace can't be zero, stage '%s'", stage.Name)
		} else if limits.AddressSpace < 64 {
			log.Warningf("Limit.AddressSpace %d mb seems very low, stage '%s'\n", limits.AddressSpace, stage.Name)
		} else if limits.AddressSpace > 512 {
			log.Warningf("Limit.AddressSpace %d mb seems too high, stage '%s'\n", limits.AddressSpace, stage.Name)
		}

		if limits.RunTime == 0.0 {
			return fmt.Errorf("Limit.RunTime can't be zero, stage '%s'", stage.Name)
		} else if limits.RunTime < 0.5 {
			log.Warningf("Limit.RunTime %.1f sec seems very low, stage '%s'\n", limits.RunTime, stage.Name)
		} else if limits.RunTime > 60.0 {
			log.Warningf("Limit.RunTime %.1f sec seems too high, stage '%s'\n", limits.RunTime, stage.Name)
		}

		if limits.FileDescriptors == 0 {
			return fmt.Errorf("Limit.FileDescriptors can't be zero, stage '%s'", stage.Name)
		} else if limits.FileDescriptors < 3 {
			log.Warningf("Limit.FileDescriptors %d seems very low, stage '%s'\n", limits.FileDescriptors, stage.Name)
		} else if limits.FileDescriptors > 512 {
			log.Warningf("Limit.FileDescriptors %d seems too high, stage '%s'\n", limits.FileDescriptors, stage.Name)
		}

		if limits.FileWrites == 0 {
			return fmt.Errorf("Limit.FileWrites can't be zero, stage '%s'", stage.Name)
		} else if limits.FileWrites < 1 {
			log.Warningf("Limit.FileWrites %d mb seems very low, stage '%s'\n", limits.FileWrites, stage.Name)
		} else if limits.FileWrites > 100 {
			log.Warningf("Limit.FileWrites %d mb seems too high, stage '%s'\n", limits.FileWrites, stage.Name)
		}

		if limits.Threads == 0 {
			return fmt.Errorf("Limit.Threads can't be zero, stage '%s'", stage.Name)
		} else if limits.Threads < 64 {
			log.Warningf("Limit.Threads %d seems very low, stage '%s'\n", limits.Threads, stage.Name)
		} else if limits.Threads > 2000 {
			log.Warningf("Limit.Threads %d seems too high, stage '%s'\n", limits.Threads, stage.Name)
		}

		if limits.Output == 0 {
			return fmt.Errorf("Limit.Output can't be zero, stage '%s'", stage.Name)
		} else if limits.Output < 1024 {
			log.Warningf("Limit.Output %d bytes seems very low, stage '%s'\n", limits.Output, stage.Name)
		} else if limits.Output > 1024*1024*1024 { // 1 mb
			log.Warningf("Limit.Output %d bytes seems too high, stage '%s'\n", limits.Output, stage.Name)
		}
	}
	return nil
}

func LoadBuildStages(filesDir string, buildEnvName string) error {
	filePath := filesDir + "/" + buildEnvName + ".yml"
	bytes, err := ioutil.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read build stages file %s: %v", filePath, err)
	}

	bs := &BuildStages{}

	err = yaml.Unmarshal(bytes, bs)
	if err != nil {
		return fmt.Errorf("failed to unmarshal build stages file %s: %w", filePath, err)
	}

	bs.StageMap = make(map[string]*Stage)
	for i, stage := range bs.Stages {
		bs.StageMap[stage.Name] = &bs.Stages[i]
	}

	err = bs.Check()
	if err != nil {
		return fmt.Errorf("error in build stages file %s: %w", filePath, err)
	}

	buildStages = bs
	return nil
}
