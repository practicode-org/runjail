package main

import (
    "encoding/json"
    "errors"
    "fmt"
    "io"
    "log"
    "net/http"
    "os/exec"
    "strings"

    "github.com/gorilla/websocket"
    "github.com/practicode-org/runner/src/api"
    "github.com/practicode-org/runner/src/rules"
)

var wsUpgrader = websocket.Upgrader{}

type CloseEvent struct {}

const (
    GenericError = iota + 1
    WebsocketError
    SourceCodeError
    MarshalError
)


func readSourceCode(conn *websocket.Conn) (string, error) {
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

    return msg.SourceCode.Text, nil
}

func messageSendLoop(conn *websocket.Conn, events chan interface{}, exitch chan<-struct{}) {
    for {
        event := <-events
        if _, close_ := event.(CloseEvent); close_ {
            break
        }

        // error messages also go to stdout
        if errMsg, ok := event.(api.Error); ok {
            fmt.Println("Error:", errMsg.Desc)
        }

        bytes, err := json.Marshal(&event)
        if err != nil {
            go func() {
                events<- api.Error{Code: GenericError, Desc: fmt.Sprintf("Failed to marshal message: %v", err)}
            }()
            continue
        }

        err = conn.WriteMessage(websocket.TextMessage, bytes)
        if err != nil {
            fmt.Printf("Failed to write to websocket: %v\n", err)
            continue
        }
    }
    log.Println("Quit from messageSendLoop")
    exitch<- struct{}{}
}

func runCommand(sendMessages chan interface{}, stage string, command string, stdin string) {
    log.Printf("Running stage '%s' command: %s\n", stage, command)

    args := strings.Split(command, " ")

    proc := exec.Command(args[0], args[1:]...)

    stdinWriter, err := proc.StdinPipe()
    if err != nil {
        sendMessages<- api.Error{Code: GenericError, Desc: fmt.Sprintf("Failed to attach to stdin pipe: %v", err), Stage: stage}
        return
    }
    stdoutPipe, err := proc.StdoutPipe()
    if err != nil {
        sendMessages<- api.Error{Code: GenericError, Desc: fmt.Sprintf("Failed to get program's stdout pipe: %v", err), Stage: stage}
        return
    }
    stderrPipe, err := proc.StderrPipe()
    if err != nil {
        sendMessages<- api.Error{Code: GenericError, Desc: fmt.Sprintf("Failed to get program's stderr pipe: %v", err), Stage: stage}
        return
    }

    pipeTransfer := func(type_ string, readFrom io.Reader) {
        for {
            buf := make([]byte, 64)
            n, err := readFrom.Read(buf)
            if err != nil && err != io.EOF {
                sendMessages<- api.Error{Code: GenericError, Desc: fmt.Sprintf("Error while reading stdout: %v", err), Stage: stage}
                break
            }
            if n == 0 && err == io.EOF {
                break
            }
            sendMessages<- api.Output{Text: string(buf[:n]), Type: type_, Stage: stage}
        }
    }

    go pipeTransfer("stdout", stdoutPipe)
    go pipeTransfer("stderr", stderrPipe)

    err = proc.Start()
    if err != nil {
        sendMessages<- api.Error{Code: GenericError, Desc: fmt.Sprintf("Failed to run program process: %v", err), Stage: stage}
        return
    }

    sendMessages <- api.Event{Event: "started", Stage: stage}

    if stdin != "" {
        stdinWriter.Write([]byte(stdin))
    }
    stdinWriter.Close()

    procState, err := proc.Process.Wait()
    if err != nil {
        sendMessages<- api.Error{Code: GenericError, Desc: fmt.Sprintf("Failed to wait program process: %v", err), Stage: stage}
        return
    }

    exitCode := procState.ExitCode()
    log.Printf("Process exit code: %d\n", exitCode)

    sendMessages <- api.Event{Event: "completed", Stage: stage}
    sendMessages <- api.ExitCode{ExitCode: exitCode, Stage: stage}
}

func handleRun(rules *rules.Rules, w http.ResponseWriter, r *http.Request) {
    log.Println("Got a request")

    c, err := wsUpgrader.Upgrade(w, r, nil)
    if err != nil {
        fmt.Printf("Failed to upgrade to Websocket: %v\n", err)
        return
    }
    defer c.Close()

    // preparation
    sendMessages := make(chan interface{}, 256)
    msgLoopExited := make(chan struct{})

    defer func() {
        log.Println("Closing")
        sendMessages<- CloseEvent{}
        <-msgLoopExited
        c.Close()
        close(sendMessages)
        log.Println("Done")
    }()

    go messageSendLoop(c, sendMessages, msgLoopExited)

    // read source code
    sourceText, err := readSourceCode(c)
    if err != nil {
        sendMessages<- api.Error{Code: SourceCodeError, Desc: fmt.Sprintf("Failed to read source code: %v", err), Stage: "read_source"}
        return
    }

    // run stages
    for i := 0; i < len(rules.Stages); i++ {
        stage := rules.Stages[i]

        stdin := stage.Stdin
        if stdin == "{source-code}" {
            stdin = sourceText
        }

        runCommand(sendMessages, stage.Name, stage.Command, stdin)
    }
}
