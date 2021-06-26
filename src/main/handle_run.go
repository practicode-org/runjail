package main

import (
    "encoding/json"
    "errors"
    "fmt"
    "log"
    "net/http"
    "os/exec"
    "io"

    "github.com/gorilla/websocket"
    "github.com/practicode-org/runner/src/api"
)

var wsUpgrader = websocket.Upgrader{}

type CloseEvent struct {}

const (
    GenericError = iota + 1
    WebsocketError
    SourceCodeError
    CompilerError
    MarshalError
    UserProgramError
)

var (
    errorNoSourceCode = errors.New("no source code provided")
)

type errorWebsocket struct {
    err error
}

func (e errorWebsocket) Error() string {
    return fmt.Sprintf("websocket error, %v", e.err)
}

func readSourceCode(conn *websocket.Conn) (string, error) {
	_, data, err := conn.ReadMessage()
    if err != nil {
        return "", errorWebsocket{err: err}
    }

    msg := api.ClientMessage{}
    err = json.Unmarshal(data, &msg)
    if err != nil {
        return "", fmt.Errorf("Failed to unmarshal message: %w, text: %s", err, data[:64])
    }

    if msg.SourceCode == nil {
        return "", errorNoSourceCode
    }

    return msg.SourceCode.Text, nil
}

func messageSendLoop(conn *websocket.Conn, events chan interface{}, exitch chan<-struct{}) {
    for {
        event := <-events
        if _, close_ := event.(CloseEvent); close_ {
            break
        }

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

func handleRun(w http.ResponseWriter, r *http.Request) {
    log.Println("Got a request")

    c, err := wsUpgrader.Upgrade(w, r, nil)
    if err != nil {
        fmt.Printf("Failed to upgrade to Websocket: %v\n", err)
        return
    }
    defer c.Close()

    //
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

    // Read source code
    sourceText, err := readSourceCode(c)
    if err != nil {
        sendMessages<- api.Error{Code: SourceCodeError, Desc: "Failed to read source code"}
        return
    }

    // Compilation phase
    outputPath := "/tmp/a.out"
    args := []string{"-o", outputPath,"-x", "c++", "-"}
    command := "/usr/bin/g++"

    log.Println("Running a compiler")
    proc := exec.Command(command, args...)

    stdin, err := proc.StdinPipe()
    if err != nil {
        sendMessages<- api.Error{Code: CompilerError, Desc: fmt.Sprintf("Failed to attach to stdin pipe: %v", err)}
        return
    }

    stdin.Write([]byte(sourceText))
    stdin.Close()

    sendMessages <- api.Event{"started:compilation"}

    output, err := proc.CombinedOutput()

    sendMessages <- api.Event{"completed:compilation"}
    if len(output) != 0 {
        sendMessages <- api.Output{string(output)}
    }
    sendMessages <- api.Event{"compilation_exit:" + fmt.Sprintf("%d", proc.ProcessState.ExitCode())}

    if err != nil {
        sendMessages<- api.Error{Code: CompilerError, Desc: fmt.Sprintf("Failed to run compiler process: %v", err)}
        return
    }

    // Run the program
    log.Println("Running a program")
    command = outputPath
    proc = exec.Command(command)

    stdoutPipe, err := proc.StdoutPipe()
    if err != nil {
        sendMessages<- api.Error{Code: UserProgramError, Desc: fmt.Sprintf("Failed to get program's stdout pipe: %v", err)}
        return
    }
    stderrPipe, err := proc.StderrPipe()
    if err != nil {
        sendMessages<- api.Error{Code: UserProgramError, Desc: fmt.Sprintf("Failed to get program's stderr pipe: %v", err)}
        return
    }

    pipeTransfer := func(sendMessages chan<- interface{}, readFrom io.Reader) {
        for {
            buf := make([]byte, 64)
            n, err := readFrom.Read(buf)
            if err != nil && err != io.EOF {
                sendMessages<- api.Error{Code: UserProgramError, Desc: fmt.Sprintf("Error while reading stdout: %v", err)}
                break
            }
            if n == 0 && err == io.EOF {
                break
            }
            sendMessages<- api.Output{string(buf[:n])}
        }
        // TODO: wait until goroutine finishes?
    }

    go pipeTransfer(sendMessages, stdoutPipe)
    go pipeTransfer(sendMessages, stderrPipe)

    err = proc.Start()
    if err != nil {
        sendMessages<- api.Error{Code: UserProgramError, Desc: fmt.Sprintf("Failed to run program process: %v", err)}
        return
    }

    sendMessages <- api.Event{"started:user_program"}

    procState, err := proc.Process.Wait()
    if err != nil {
        sendMessages<- api.Error{Code: UserProgramError, Desc: fmt.Sprintf("Failed to wait program process: %v", err)}
        return
    }

    exitCode := procState.ExitCode()
    log.Printf("User program done, exit code: %d\n", exitCode)

    sendMessages <- api.Event{"completed:user_program"}
    sendMessages <- api.Event{"user_program_exit:" + fmt.Sprintf("%d", exitCode)}
}
