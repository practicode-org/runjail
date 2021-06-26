package rules

import (
    "encoding/json"
    "fmt"
    "io/ioutil"
    "path/filepath"
    "time"

    "gopkg.in/yaml.v2"
)

type Limits struct {
    AddressSpace int `json:"address_space"`
    RSS int `json:"rss"`
    CpuTime time.Time `json:"cpu_time"`
    FileDescriptors int `json:"file_descriptors"`
    FileWrite int `json:"file_write"`
    Threads int `json:"threads"`
    DataSegment int `json:"data_segment"`
}

type Stage struct {
    Name string `json:"name"`
    Command string `json:"command"`
    Stdin string `json:"stdin"`
    Limits *Limits `json:"limits"`
}

type Rules struct {
    Stages []Stage `json:"stages"`
}


func ReadJsonRules(data []byte) (*Rules, error) {
    pipeline := &Rules{}
    err := json.Unmarshal(data, pipeline)
    if err != nil {
        return nil, err
    }
    return pipeline, nil
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
