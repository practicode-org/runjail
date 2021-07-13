package main

import (
	"crypto/md5"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
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
		if totalSize > rules.SourcesSizeLimitBytes {
			return nil, fmt.Errorf("reached source code size limit: %d", rules.SourcesSizeLimitBytes)
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
	defer func() {
		exitch <- struct{}{}
	}()

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
			log.Errorf("Failed to write to websocket: %v\n", err)
			return
		}
	}
}

//
func messageRecvLoop(conn *websocket.Conn, events chan interface{}, exitch chan<- struct{}) {
	defer func() {
		exitch <- struct{}{}
	}()

	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return
		}

		msg := api.ClientMessage{}
		err = json.Unmarshal(data, &msg)
		if err != nil {
			log.Errorf("Failed to unmarshal message: %w, text: '%s'", err, data[:64])
			continue
		}

		if msg.Command == "" {
			log.Errorf("Received an empty command")
		}

		events <- msg.Command
	}
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

func runCommand(sendMessages chan interface{}, clientCommands chan interface{}, stage rules.Stage, sourceFiles []string) bool {
	startTime := time.Now()

	jailedCommand, jailedArgs := wrapToJail(stage.Command, stage.Env, stage.Mounts, stage.Limits, sourceFiles)

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
	log.Debugf("Started process pid %d", cmd.Process.Pid)

	// listen to commands from the client
	var killed int32
	quitCmdLoop := make(chan struct{})
	go func() {
		proc := cmd.Process
		quit := false
		for !quit {
			select {
			case cmd, ok := <-clientCommands:
				if !ok {
					break
				}
				if cmd == "stop" {
					err := KillProcess(proc)
					if err != nil {
						log.Errorf("Failed to kill process pid %d: %v", proc.Pid, err)
					}
					atomic.StoreInt32(&killed, 1)
					break
				} else {
					log.Errorf("Received unknown client command: %q", cmd)
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
		sendMessages <- api.Error{Code: GenericError, Desc: fmt.Sprintf("Failed to wait program process: %v", err), Stage: stage.Name}
		return false
	}

	exitCode := procState.ExitCode()
	duration := time.Since(startTime)

	if atomic.LoadInt32(&killed) != 0 {
		log.Infof("Process killed by client request, exit code: %d, stage duration: %.2f sec, output: %d bytes", exitCode, duration.Seconds(), outputTransferred)
	} else {
		log.Infof("Process exit code: %d, stage duration: %.2f sec, output: %d bytes", exitCode, duration.Seconds(), outputTransferred)
	}

	sendMessages <- api.ExitCode{ExitCode: exitCode, Stage: stage.Name}
	sendMessages <- api.Duration{DurationSec: duration.Seconds(), Stage: stage.Name}
	sendMessages <- api.Event{Event: "completed", Stage: stage.Name}

	return exitCode == 0
}

func handleRun(rules *rules.Rules, w http.ResponseWriter, r *http.Request) {
	log.Debugf("Got a request")

	wsUpgrader.CheckOrigin = func(r *http.Request) bool { return true }
	c, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Errorf("Failed to upgrade to Websocket: %v\n", err)
		return
	}
	defer c.Close()

	// preparation
	sendMessages := make(chan interface{}, 256)
	recvMessages := make(chan interface{}, 4)
	msgSendExited := make(chan struct{})
	msgRecvExited := make(chan struct{})
	go messageSendLoop(c, sendMessages, msgSendExited)

	defer func() {
		sendMessages <- CloseEvent{}
		<-msgSendExited
		c.Close()
		<-msgRecvExited
		close(sendMessages)
		close(recvMessages)
		log.Debugf("Done")
	}()

	// read source code
	sourceFiles, err := receiveSourceCode(c, rules)
	if err != nil {
		sendMessages <- api.Error{Code: SourceCodeError, Desc: fmt.Sprintf("Failed to read source code: %v", err), Stage: "init"}
		return
	}

	// TODO: use RECV loop for source code
	go messageRecvLoop(c, recvMessages, msgRecvExited)

	// run stages
	for i := 0; i < len(rules.Stages); i++ {
		success := runCommand(sendMessages, recvMessages, rules.Stages[i], sourceFiles)
		if !success {
			break
		}
	}
}
