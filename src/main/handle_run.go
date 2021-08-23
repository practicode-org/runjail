package main

import (
	"crypto/md5"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"
	"unicode"

	"github.com/gorilla/websocket"
	log "github.com/sirupsen/logrus"

	"github.com/practicode-org/runner/src/api"
	"github.com/practicode-org/runner/src/config"
	"github.com/practicode-org/runner/src/rules"
)

var wsUpgrader = websocket.Upgrader{}

type CloseEvent struct{}

const (
	GenericError = iota + 1
	SourceCodeError
)

func receiveSourceCode(recvMessages <-chan []byte) ([]string, error) {
	bytes := <-recvMessages

	msg := api.ClientMessage{}
	err := json.Unmarshal(bytes, &msg)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal message: %w, text: %s", err, trimLongString(string(bytes), 64))
	}
	if msg.SourceFiles == nil || len(msg.SourceFiles) == 0 {
		return nil, errors.New("no source code when it's expected")
	}

	var totalSize uint64

	for i, sf := range msg.SourceFiles {
		// check file name
		if len(sf.Name) == 0 || len(sf.Name) > 64 {
			return nil, fmt.Errorf("source file name is too long")
		}
		if sf.Name[0] == '.' || sf.Name[len(sf.Name)-1] == '.' {
			return nil, fmt.Errorf("wrong source file name %q", sf.Name)
		}
		if len(sf.Hash) != md5.Size*2 {
			return nil, fmt.Errorf("wrong hash length (%d), must be %d for hex MD5", len(sf.Hash), md5.Size*2)
		}
		if idx := strings.IndexFunc(sf.Name, func(r rune) bool {
			return !unicode.IsDigit(r) && !('a' <= r && r <= 'z') && !('A' <= r && r <= 'Z') && r != '_' && r != '.'
		}); idx != -1 {
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
		if totalSize > config.Cfg.SourcesSizeLimitBytes {
			return nil, fmt.Errorf("reached source code size limit: %d", config.Cfg.SourcesSizeLimitBytes)
		}

		// check hash
		computedHash := fmt.Sprintf("%x", md5.Sum(decodedSources))
		for j := 0; j < md5.Size*2; j++ {
			if computedHash[j] != sf.Hash[j] {
				return nil, fmt.Errorf("hash for %q doesn't match the source code", sf.Name)
			}
		}
	}

	fileNames := []string{}

	for _, sf := range msg.SourceFiles {
		filePath := filepath.Join(config.Cfg.SourcesDir, sf.Name)

		err := ioutil.WriteFile(filePath, []byte(sf.Text), 0660) // rw-/rw-/---
		if err != nil {
			return nil, err
		}
		fileNames = append(fileNames, filePath)
	}

	return fileNames, nil
}

func KillProcess(proc *os.Process) error {
	// TODO: this doesn't kill child processes
	return proc.Signal(os.Kill)
}

func wrapToJail(command string, env []string, mounts []string, limits *rules.Limits, sourceFiles []string) (string, []string) {
	envStr := ""
	for _, envVar := range env {
		envStr += " --env=" + envVar
	}

	mountStr := ""
	for _, mountDir := range mounts {
		mountStr += " --bindmount=" + mountDir
	}

	nsjailCmd := fmt.Sprintf("/usr/bin/nsjail --really_quiet --nice_level=0%s%s --time_limit=%.1f --rlimit_as=%d --rlimit_core=0 --rlimit_fsize=%d --rlimit_nofile=%d --rlimit_nproc=%d --chroot / -- ",
		mountStr,
		envStr,
		limits.RunTime,
		limits.AddressSpace,
		limits.FileWrites,
		limits.FileDescriptors,
		limits.Threads,
	)
	nsjailCmd += command
	// replace {sources} with source files paths
	sourceFilesStr := strings.Join(sourceFiles, " ")
	nsjailCmd = strings.ReplaceAll(nsjailCmd, "{sources}", sourceFilesStr)
	return nsjailCmd, strings.Split(nsjailCmd, " ")
}

func runCommand(sendMessages chan<- interface{}, clientCommands <-chan []byte, stage rules.Stage, sourceFiles []string, requestID string) bool {
	startTime := time.Now()

	jailedCommand, jailedArgs := wrapToJail(stage.Command, stage.Env, stage.Mounts, stage.Limits, sourceFiles)

	log.Infof("Running stage '%s' command: %s\n", stage.Name, jailedCommand)

	cmd := exec.Command(jailedArgs[0], jailedArgs[1:]...)

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		sendMessages <- api.Error{Code: GenericError, Desc: fmt.Sprintf("Failed to get program's stdout pipe: %v", err), Stage: stage.Name, RequestID: requestID}
		return false
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		sendMessages <- api.Error{Code: GenericError, Desc: fmt.Sprintf("Failed to get program's stderr pipe: %v", err), Stage: stage.Name, RequestID: requestID}
		return false
	}

	var outputTransferred uint64

	pipeTransfer := func(type_ string, readFrom io.Reader) {
		for {
			buf := make([]byte, 512)
			n, err := readFrom.Read(buf)
			if err != nil && err != io.EOF {
				sendMessages <- api.Error{Code: GenericError, Desc: fmt.Sprintf("Error while reading stdout: %v", err), Stage: stage.Name, RequestID: requestID}
				break
			}
			if n == 0 && err == io.EOF {
				break
			}

			encodedStr := base64.StdEncoding.EncodeToString(buf[:n])
			sendMessages <- api.Output{Text: encodedStr, Type: type_, Stage: stage.Name, RequestID: requestID}

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
		sendMessages <- api.Error{Code: GenericError, Desc: fmt.Sprintf("Failed to run program process: %v", err), Stage: stage.Name, RequestID: requestID}
		return false
	}

	sendMessages <- api.StageEvent{Event: "started", Stage: stage.Name, RequestID: requestID}
	log.Debugf("Started process pid %d", cmd.Process.Pid)

	// listen to commands from the client
	var killed int32
	quitCmdLoop := make(chan struct{})
	go func() {
		proc := cmd.Process
		quit := false
		for !quit {
			select {
			case bytes, ok := <-clientCommands:
				if !ok {
					break
				}
				msg := api.ClientMessage{}
				err := json.Unmarshal(bytes, &msg)
				if err != nil {
					log.Errorf("Failed to unmarshal: %v, message: %s", err, trimLongString(string(bytes), 64))
					continue
				}
				if msg.Command == "stop" {
					err := KillProcess(proc)
					if err != nil {
						log.Errorf("Failed to kill process pid %d: %v", proc.Pid, err)
					}
					atomic.StoreInt32(&killed, 1)
					break
				} else {
					log.Errorf("Received unknown client message (expected a stop command only), message: %s", trimLongString(string(bytes), 64))
				}
			case <-quitCmdLoop:
				quit = true
				break
			}
		}
	}()
	defer func() { close(quitCmdLoop) }()

	//
	procState, err := cmd.Process.Wait()
	if err != nil {
		sendMessages <- api.Error{Code: GenericError, Desc: fmt.Sprintf("Failed to wait program process: %v", err), Stage: stage.Name, RequestID: requestID}
		return false
	}

	exitCode := procState.ExitCode()
	duration := time.Since(startTime)

	if atomic.LoadInt32(&killed) != 0 {
		log.Infof("Process killed by client request, exit code: %d, stage duration: %.2f sec, output: %d bytes", exitCode, duration.Seconds(), outputTransferred)
	} else {
		log.Infof("Process exit code: %d, stage duration: %.2f sec, output: %d bytes", exitCode, duration.Seconds(), outputTransferred)
	}

	sendMessages <- api.ExitCode{ExitCode: exitCode, Stage: stage.Name, RequestID: requestID}
	sendMessages <- api.Duration{DurationSec: duration.Seconds(), Stage: stage.Name, RequestID: requestID}
	sendMessages <- api.StageEvent{Event: "completed", Stage: stage.Name, RequestID: requestID}

	return exitCode == 0
}

/*func handleRun(w http.ResponseWriter, r *http.Request, buildEnv string) {
	log.Debugf("Got a request")

	query := r.URL.Query()

	requestBuildEnv := query.Get("build_env")
	if requestBuildEnv == "" {
		log.Errorf("Error: got a request without build_env\n")
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if buildEnv != requestBuildEnv {
		log.Errorf("Error: got a request with build_env %s, but %s is loaded\n", buildEnv, requestBuildEnv)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	wsUpgrader.CheckOrigin = func(r *http.Request) bool { return true }
	c, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Errorf("Failed to upgrade to Websocket: %v\n", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer c.Close()

	handleWsConnection(c, "no-request-id") // TODO: fix
}*/

func handleRequest(requestID string, recvMessages <-chan []byte, sendMessages chan<- interface{}) {
	// read source code
	sourceFiles, err := receiveSourceCode(recvMessages)
	if err != nil {
		sendMessages <- api.Error{Code: SourceCodeError, Desc: fmt.Sprintf("Failed to read source code: %v", err), Stage: "init", RequestID: requestID}
		return
	}

	// run stages
	for i := 0; i < len(rules.Rule.Stages); i++ {
		success := runCommand(sendMessages, recvMessages, rules.Rule.Stages[i], sourceFiles, requestID)
		if !success {
			break
		}
	}
	sendMessages <- api.Finish{Finish: true, RequestID: requestID}
}
