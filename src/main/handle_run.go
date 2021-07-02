package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	log "github.com/sirupsen/logrus"

	"github.com/practicode-org/runner/src/api"
	"github.com/practicode-org/runner/src/rules"
)

var wsUpgrader = websocket.Upgrader{}

type CloseEvent struct{}

const (
	GenericError = iota + 1
	SourceCodeError
)

//
func readSourceCode(conn *websocket.Conn) (string, error) {
	// TODO: use conn.readLimit
	_, data, err := conn.ReadMessage()
	if err != nil {
		return "", fmt.Errorf("failed to read from websocket: %w", err)
	}

	msg := api.ClientMessage{}
	err = json.Unmarshal(data, &msg)
	if err != nil {
		return "", fmt.Errorf("failed to unmarshal message: %w, text: '%s'", err, data[:64])
	}

	if msg.SourceCode == nil {
		return "", errors.New("no source code")
	}

	decodedSources, err := base64.StdEncoding.DecodeString(msg.SourceCode.Text)
	if err != nil {
		return "", fmt.Errorf("failed to decode base64: %w", err)
	}

	return string(decodedSources), nil
}

func putSourceCode(sourceCode string, sourcesDir string) error {
	if sourcesDir == "" {
		return errors.New("source_dir is empty")
	}

	fileName := "sources_0.txt"
	filePath := filepath.Join(sourcesDir, fileName)

	err := ioutil.WriteFile(filePath, []byte(sourceCode), 0660)
	if err != nil {
		return err
	}
	return nil
}

//
func messageSendLoop(conn *websocket.Conn, events chan interface{}, exitch chan<- struct{}) {
	for {
		event := <-events
		if _, close_ := event.(CloseEvent); close_ {
			break
		}

		// error messages also go to stdout
		if errMsg, ok := event.(api.Error); ok {
			log.Error("Error:", errMsg.Desc)
		}

		bytes, err := json.Marshal(&event)
		if err != nil {
			go func() {
				events <- api.Error{Code: GenericError, Desc: fmt.Sprintf("Failed to marshal message: %v", err)}
			}()
			continue
		}

		err = conn.WriteMessage(websocket.TextMessage, bytes)
		if err != nil {
			log.Errorf("Failer to write to websocket: %v\n", err)
			continue
		}
	}
	exitch <- struct{}{}
}

func wrapToJail(command string, limits *rules.Limits, sourcesDir string) (string, []string) {
	nsjailCmd := fmt.Sprintf("/usr/bin/nsjail --quiet --nice_level=0 --bindmount=/tmp/out --time_limit=%.1f --rlimit_as=%d --rlimit_core=0 --rlimit_fsize=%d --rlimit_nofile=%d --rlimit_nproc=%d --chroot / -- ",
		limits.RunTime,
		limits.AddressSpace,
		limits.FileWrites,
		limits.FileDescriptors,
		limits.Threads,
	)
	nsjailCmd += command
	nsjailCmd = strings.ReplaceAll(nsjailCmd, "{sources}", filepath.Join(sourcesDir, "sources_0.txt")) // TODO: support multiple source files

	return nsjailCmd, strings.Split(nsjailCmd, " ")
}

func runCommand(sendMessages chan interface{}, stage string, command string, limits *rules.Limits, sourcesDir string) bool {
	startTime := time.Now()

	jailedCommand, jailedArgs := wrapToJail(command, limits, sourcesDir)

	log.Infof("Running stage '%s' command: %s\n", stage, jailedCommand)

	cmd := exec.Command(jailedArgs[0], jailedArgs[1:]...)

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		sendMessages <- api.Error{Code: GenericError, Desc: fmt.Sprintf("Failed to get program's stdout pipe: %v", err), Stage: stage}
		return false
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		sendMessages <- api.Error{Code: GenericError, Desc: fmt.Sprintf("Failed to get program's stderr pipe: %v", err), Stage: stage}
		return false
	}

	var outputTransferred uint64

	pipeTransfer := func(type_ string, readFrom io.Reader) {
		for {
			buf := make([]byte, 512)
			n, err := readFrom.Read(buf)
			if err != nil && err != io.EOF {
				sendMessages <- api.Error{Code: GenericError, Desc: fmt.Sprintf("Error while reading stdout: %v", err), Stage: stage}
				break
			}
			if n == 0 && err == io.EOF {
				break
			}

			encodedStr := base64.StdEncoding.EncodeToString(buf[:n])
			sendMessages <- api.Output{Text: encodedStr, Type: type_, Stage: stage}

			// check limits
			transferredNew := atomic.AddUint64(&outputTransferred, uint64(n))
			if transferredNew >= limits.Output {
				err := cmd.Process.Kill() // doesn't kill child processes
				if err != nil {
					log.Errorf("Couldn't kill proc id: %d due to excessive output: %v", cmd.Process.Pid, err)
				} else {
					log.Infof("Killed process due to excessive output, stage %s", stage)
				}
				break
			}
		}
	}

	go pipeTransfer("stdout", stdoutPipe)
	go pipeTransfer("stderr", stderrPipe)

	err = cmd.Start()
	if err != nil {
		sendMessages <- api.Error{Code: GenericError, Desc: fmt.Sprintf("Failed to run program process: %v", err), Stage: stage}
		return false
	}

	sendMessages <- api.Event{Event: "started", Stage: stage}

	//
	procState, err := cmd.Process.Wait()
	if err != nil {
		sendMessages <- api.Error{Code: GenericError, Desc: fmt.Sprintf("Failed to wait program process: %v", err), Stage: stage}
		return false
	}

	exitCode := procState.ExitCode()
	duration := time.Since(startTime)

	log.Infof("Process exit code: %d, stage duration: %.2f sec, output: %d bytes\n", exitCode, duration.Seconds(), outputTransferred)

	sendMessages <- api.ExitCode{ExitCode: exitCode, Stage: stage}
	sendMessages <- api.Duration{DurationSec: duration.Seconds(), Stage: stage}
	sendMessages <- api.Event{Event: "completed", Stage: stage}

	return exitCode == 0
}

func handleRun(rules *rules.Rules, w http.ResponseWriter, r *http.Request) {
	log.Debugf("Got a request")

	c, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Errorf("Failed to upgrade to Websocket: %v\n", err)
		return
	}
	defer c.Close()

	// preparation
	sendMessages := make(chan interface{}, 256)
	msgLoopExited := make(chan struct{})

	defer func() {
		sendMessages <- CloseEvent{}
		<-msgLoopExited
		c.Close()
		close(sendMessages)
		log.Debugf("Done")
	}()

	go messageSendLoop(c, sendMessages, msgLoopExited)

	// read source code
	sourceText, err := readSourceCode(c)
	if err != nil {
		sendMessages <- api.Error{Code: SourceCodeError, Desc: fmt.Sprintf("Failed to read source code: %v", err), Stage: "init"}
		return
	}
	if uint64(len(sourceText)) > rules.SourcesSizeLimitBytes {
		sendMessages <- api.Error{Code: SourceCodeError, Desc: fmt.Sprintf("Reached source code size limit: %d", rules.SourcesSizeLimitBytes), Stage: "init"}
		return
	}

	err = putSourceCode(sourceText, rules.SourcesDir)
	if err != nil {
		sendMessages <- api.Error{Code: SourceCodeError, Desc: fmt.Sprintf("Failed to put source code: %v", err), Stage: "init"}
		return
	}

	// run stages
	for i := 0; i < len(rules.Stages); i++ {
		stage := rules.Stages[i]

		success := runCommand(sendMessages, stage.Name, stage.Command, stage.Limits, rules.SourcesDir)
		if !success {
			break
		}
	}
}
