package api


type SourceCode struct {
    Text string `json:"text"`
}

type ClientMessage struct {
    SourceCode *SourceCode `json:"source_code"`
}

type Event struct {
    Event string `json:"event"`
    Stage string `json:"stage"`
}

type ExitCode struct {
    ExitCode int `json:"exit_code"`
    Stage string `json:"stage"`
}

type Duration struct {
    DurationSec float64 `json:"duration_sec"`
    Stage string `json:"stage"`
}

type Output struct {
    Text string `json:"output"`
    Type string `json:"type"`
    Stage string `json:"stage"`
}

type Error struct {
    Code int `json:"error"`
    Desc string `json:"description"`
    Stage string `json:"stage"`
}
