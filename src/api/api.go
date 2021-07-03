package api


type SourceFile struct {
    Name string `json:"name"`
    Text string `json:"text"`
}

type ClientMessage struct {
    SourceFiles []SourceFile `json:"source_files"`
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
    Text string `json:"output"` // base64 encoded
    Type string `json:"type"`
    Stage string `json:"stage"`
}

type Error struct {
    Code int `json:"error"`
    Desc string `json:"description"`
    Stage string `json:"stage"`
}
