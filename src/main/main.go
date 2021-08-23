package main

import (
	"flag"
	"net/http"
	"net/url"
	"time"

	"github.com/gorilla/websocket"
	log "github.com/sirupsen/logrus"

	"github.com/practicode-org/worker/src/config"
	"github.com/practicode-org/worker/src/rules"
)

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

var rulesDirFlag = flag.String("rules-dir", "", "directory with .json or .yaml rules files")
var buildEnvNameFlag = flag.String("build-env", "", "name of the buildenv")
var backendAddrFlag = flag.String("backend-addr", "", "backend's ip address (optional)")
var listenAddrFlag = flag.String("listen-addr", "0.0.0.0:1556", "listen interface and port")
var logLevelFlag = flag.String("log-level", "info", "verbosity level: panic, fatal, error, warn, info, debug, trace")

func main() {
	err := config.DefaultConfig()
	if err != nil {
		log.Fatalf("Config error: %v", err)
	}
	flag.Parse()

	level, err := log.ParseLevel(*logLevelFlag)
	if err != nil {
		log.Fatalf("Failed to parse log-level: %v", err)
	}
	log.SetLevel(level)

	if *rulesDirFlag == "" {
		log.Fatalf("Fatal: rules-dir is empty")
	}
	if *buildEnvNameFlag == "" {
		log.Fatalf("Fatal: build-env is empty")
	}

	err = rules.LoadRules(*rulesDirFlag, *buildEnvNameFlag)
	if err != nil {
		log.Fatalf("Error: %v", err)
	}

	backendAddr := *backendAddrFlag
	if backendAddr != "" {
		u := url.URL{Scheme: "ws", Host: backendAddr, Path: "/bridge", RawQuery: "build_env=" + *buildEnvNameFlag}
		log.Infof("Auto-connect to backend mode, will dial to: %s", u.String())

		for {
			dialer := websocket.Dialer{Proxy: http.ProxyFromEnvironment, HandshakeTimeout: time.Second * 10}
			conn, _, err := dialer.Dial(u.String(), nil)
			if err != nil {
				log.Errorf("Failed to connect to %s: %v", u.String(), err)
				time.Sleep(time.Second * 3)
				continue
			}

			log.Infof("Connected to the backend %s", backendAddr)
			handleBackendConnection(conn) // Note: connection is closed inside
			time.Sleep(time.Second * 3)
		}
	} else {
		listenAddr := *listenAddrFlag
		log.Infof("Stay and listen mode, will listen on: %s", listenAddr)

		http.HandleFunc("/health", handleHealth)
		/*http.HandleFunc("/run", func(rs http.ResponseWriter, r *http.Request) {
			handleRun(rs, r, *buildEnvNameFlag)
		})*/

		err = http.ListenAndServe(listenAddr, nil)
		if err != nil {
			log.Fatalf("Failed to server: %v", err)
		}
	}
}
