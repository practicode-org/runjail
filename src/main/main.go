package main

import (
    "log"
    "net/http"
)

func handleHealth(w http.ResponseWriter, r *http.Request) {
    w.WriteHeader(http.StatusOK)
}

func main() {
    http.HandleFunc("/health", handleHealth)
    http.HandleFunc("/run", handleRun)

    PORT := ":1556"
    log.Println("Starting serving at", PORT)
    http.ListenAndServe("0.0.0.0" + PORT, nil)
}
