package api


type SourceCode struct {
    Text string `json:"text"`
}

type ClientMessage struct {
    SourceCode *SourceCode `json:"source_code"`
}

type Event struct {
    Event string `json:"event"`
}

type Output struct {
    Text string `json:"output"`
}

type Error struct {
    Code int `json:"error"`
    Desc string `json:"description"`
}
