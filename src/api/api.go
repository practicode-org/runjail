package api

// Client -> Backend
type SourceFile struct {
	Name string `json:"name"`
	Text string `json:"text"`
	Hash string `json:"hash"`
}

type ClientMessage struct {
	SourceFiles []SourceFile `json:"source_files"`
	Command     string       `json:"command"`
	RequestID   string       `json:"request_id"`
	// name of a target stage, ex: "run_tests"
	Target string `json:"target"`
}

type TestCheck struct {
	Type string `json:"type"`
	Arg  string `json:"arg"`
}

type TestCase struct {
	Description string `json:"description"`
	//Explanation string `json:"explanation"`
	//StealResultsFrom int `json:"steal_results_from"`
	//Stdin string `json:"stdin"`
	Checks []TestCheck `json:"checks"`
}

type TestSuite struct {
	InitTestCases []TestCase `json:"init_test_cases"`
	TestCases     []TestCase `json:"test_cases"`
}

// Backend -> Client

// Possible events:
// "started" - start of a stage
// "finished" - stage ended
type StageEvent struct {
	Event     string `json:"event"`
	Stage     string `json:"stage"`
	TestCase  string `json:"test_case,omitempty"` // index of a test case being run (if applied)
	RequestID string `json:"request_id"`
}

type ExitCode struct {
	ExitCode  int    `json:"exit_code"`
	Stage     string `json:"stage"`
	RequestID string `json:"request_id"`
}

type Duration struct {
	DurationSec float64 `json:"duration_sec"`
	Stage       string  `json:"stage"`
	RequestID   string  `json:"request_id"`
}

type Output struct {
	Text      string `json:"output"` // base64 encoded
	Type      string `json:"type"`
	Stage     string `json:"stage"`
	RequestID string `json:"request_id"`
}

type TestResult struct {
	TestCase  string `json:"test_case"` // index of a test case being run (if applied)
	Result    bool   `json:"result"`
	Stage     string `json:"stage"`
	RequestID string `json:"request_id"`
}

type Error struct {
	Desc      string `json:"description"`
	Stage     string `json:"stage"`
	RequestID string `json:"request_id"`
}

// The last message, meaning there will be no more messages for this request_id
type Finish struct {
	Finish    bool   `json:"finish"`
	RequestID string `json:"request_id"`
}
