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
    "unicode"

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
func receiveSourceCode(conn *websocket.Conn, rules *rules.Rules) ([]string, error) {
	// TODO: use conn.readLimit
	_, data, err := conn.ReadMessage()
	if err != nil {
		return nil, fmt.Errorf("failed to read from websocket: %w", err)
	}

	msg := api.ClientMessage{}
	err = json.Unmarshal(data, &msg)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal message: %w, text: '%s'", err, data[:64])
	}
	if msg.SourceFiles == nil || len(msg.SourceFiles) == 0 {
		return nil, errors.New("no source code")
	}

    var totalSize uint64

    for i, sf := range msg.SourceFiles {
        // check file name
        if len(sf.Name) == 0 || len(sf.Name) > 64 {
            return nil, fmt.Errorf("source file name is too long")
        }
        if sf.Name[0] == '.' || sf.Name[len(sf.Name)-1] == '.' {
            return nil, fmt.Errorf("wrong source file name: %s", sf.Name)
        }
        if idx := strings.IndexFunc(sf.Name, func(r rune) bool { return !unicode.IsDigit(r) && !('a' <= r && r <= 'z') && !('A' <= r && r <= 'Z') && r != '_' && r != '.' }); idx != -1 {
            return nil, fmt.Errorf("forbidden character in the source file name at index %d", idx)
        }

        // decode
        decodedSources, err := base64.StdEncoding.DecodeString(sf.Text)
        if err != nil {
            return nil, fmt.Errorf("failed to decode base64: %w", err)
        }
        msg.SourceFiles[i].Text = string(decodedSources)
        totalSize += uint64(len(decodedSources))

        // check size limit
        if totalSize > rules.SourcesSizeLimitBytes {
            return nil, fmt.Errorf("reached source code size limit: %d", rules.SourcesSizeLimitBytes)
        }
    }

    fileNames := []string{}

    for _, sf := range msg.SourceFiles {
        filePath := filepath.Join(rules.SourcesDir, sf.Name)

        err := ioutil.WriteFile(filePath, []byte(sf.Text), 0660)
        if err != nil {
            return nil, err
        }
        fileNames = append(fileNames, filePath)
    }

	return fileNames, nil
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

func wrapToJail(command string, limits *rules.Limits, sourceFiles []string) (string, []string) {
	nsjailCmd := fmt.Sprintf("/usr/bin/nsjail --quiet --nice_level=0 --bindmount=/tmp/out --time_limit=%.1f --rlimit_as=%d --rlimit_core=0 --rlimit_fsize=%d --rlimit_nofile=%d --rlimit_nproc=%d --chroot / -- ",
		limits.RunTime,
		limits.AddressSpace,
		limits.FileWrites,
		limits.FileDescriptors,
		limits.Threads,
	)
	nsjailCmd += command
    sourceFilesStr := strings.Join(sourceFiles, " ")
	nsjailCmd = strings.ReplaceAll(nsjailCmd, "{sources}", sourceFilesStr)
	return nsjailCmd, strings.Split(nsjailCmd, " ")
}

func runCommand(sendMessages chan interface{}, stage rules.Stage, sourceFiles []string) bool {
	startTime := time.Now()

	jailedCommand, jailedArgs := wrapToJail(stage.Command, stage.Limits, sourceFiles)

	log.Infof("Running stage '%s' command: %s\n", stage.Name, jailedCommand)

	cmd := exec.Command(jailedArgs[0], jailedArgs[1:]...)

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		sendMessages <- api.Error{Code: GenericError, Desc: fmt.Sprintf("Failed to get program's stdout pipe: %v", err), Stage: stage.Name}
		return false
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		sendMessages <- api.Error{Code: GenericError, Desc: fmt.Sprintf("Failed to get program's stderr pipe: %v", err), Stage: stage.Name}
		return false
	}

	var outputTransferred uint64

	pipeTransfer := func(type_ string, readFrom io.Reader) {
		for {
			buf := make([]byte, 512)
			n, err := readFrom.Read(buf)
			if err != nil && err != io.EOF {
				sendMessages <- api.Error{Code: GenericError, Desc: fmt.Sprintf("Error while reading stdout: %v", err), Stage: stage.Name}
				break
			}
			if n == 0 && err == io.EOF {
				break
			}

			encodedStr := base64.StdEncoding.EncodeToString(buf[:n])
			sendMessages <- api.Output{Text: encodedStr, Type: type_, Stage: stage.Name}

			// check limits
			transferredNew := atomic.AddUint64(&outputTransferred, uint64(n))
			if transferredNew >= stage.Limits.Output {
				err := cmd.Process.Kill() // doesn't kill child processes
				if err != nil {
					log.Errorf("Couldn't kill proc id: %d due to excessive output: %v", cmd.Process.Pid, err)
				} else {
					log.Infof("Killed process due to excessive output, stage %s", stage.Name)
				}
				break
			}
		}
	}

	go pipeTransfer("stdout", stdoutPipe)
	go pipeTransfer("stderr", stderrPipe)

	err = cmd.Start()
	if err != nil {
		sendMessages <- api.Error{Code: GenericError, Desc: fmt.Sprintf("Failed to run program process: %v", err), Stage: stage.Name}
		return false
	}

	sendMessages <- api.Event{Event: "started", Stage: stage.Name}

	//
	procState, err := cmd.Process.Wait()
	if err != nil {
		sendMessages <- api.Error{Code: GenericError, Desc: fmt.Sprintf("Failed to wait program process: %v", err), Stage: stage.Name}
		return false
	}

	exitCode := procState.ExitCode()
	duration := time.Since(startTime)

	log.Infof("Process exit code: %d, stage duration: %.2f sec, output: %d bytes\n", exitCode, duration.Seconds(), outputTransferred)

	sendMessages <- api.ExitCode{ExitCode: exitCode, Stage: stage.Name}
	sendMessages <- api.Duration{DurationSec: duration.Seconds(), Stage: stage.Name}
	sendMessages <- api.Event{Event: "completed", Stage: stage.Name}

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
	sourceFiles, err := receiveSourceCode(c, rules)
	if err != nil {
		sendMessages <- api.Error{Code: SourceCodeError, Desc: fmt.Sprintf("Failed to read source code: %v", err), Stage: "init"}
		return
	}

	// run stages
	for i := 0; i < len(rules.Stages); i++ {
		stage := rules.Stages[i]

		success := runCommand(sendMessages, stage, sourceFiles)
		if !success {
			break
		}
	}
}
