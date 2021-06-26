package main

import (
    "flag"
    "log"
    "net/http"
    "os"

    "github.com/practicode-org/runner/src/rules"
)

func handleHealth(w http.ResponseWriter, r *http.Request) {
    w.WriteHeader(http.StatusOK)
}

var rulesFile = flag.String("rules", "", "path to a rules .json or .yaml file")

func main() {
    flag.Parse()

    rules, err := rules.LoadRules(*rulesFile)
    if err != nil {
        log.Printf("Error: %v", err)
        os.Exit(1)
    }

    http.HandleFunc("/health", handleHealth)
    http.HandleFunc("/run", func(rs http.ResponseWriter, r *http.Request) {
        handleRun(rules, rs, r)
    })

    PORT := ":1556"
    log.Println("Starting serving at", PORT)
    http.ListenAndServe("0.0.0.0" + PORT, nil)
}
