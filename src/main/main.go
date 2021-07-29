package main

import (
	"flag"
	"net/http"

	log "github.com/sirupsen/logrus"

	"github.com/practicode-org/runner/src/config"
	"github.com/practicode-org/runner/src/rules"
)

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

var rulesDir = flag.String("rules-dir", "", "directory with .json or .yaml rules files")

func main() {
	err := config.DefaultConfig()
	if err != nil {
		log.Fatalf("Config error: %v", err)
	}
	flag.Parse()

	err = rules.LoadRules(*rulesDir)
	if err != nil {
		log.Fatalf("Error: %v", err)
	}

	http.HandleFunc("/health", handleHealth)
	http.HandleFunc("/run", handleRun)

	PORT := ":1556"
	log.Infof("Starting serving at %s", PORT)
	err = http.ListenAndServe("0.0.0.0"+PORT, nil)
	if err != nil {
		log.Fatalf("Failed to server: %v", err)
	}
}
