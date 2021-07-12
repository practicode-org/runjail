package main

import (
	"flag"
	"net/http"

	log "github.com/sirupsen/logrus"

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
		log.Fatalf("Error: %v", err)
	}

	http.HandleFunc("/health", handleHealth)
	http.HandleFunc("/run", func(rs http.ResponseWriter, r *http.Request) {
		handleRun(rules, rs, r)
	})

	PORT := ":1556"
	log.Infof("Starting serving at %s", PORT)
	err = http.ListenAndServe("0.0.0.0"+PORT, nil)
	if err != nil {
		log.Fatalf("Failed to server: %v", err)
	}
}
