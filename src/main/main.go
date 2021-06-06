package main

import (
    "encoding/json"
    "log"
    "net/http"
    "os/exec"
    "io"

    "github.com/gorilla/websocket"
)

var wsUpgrader = websocket.Upgrader{}

func handleHealth(w http.ResponseWriter, r *http.Request) {
    w.WriteHeader(http.StatusOK)
}


type SourceCode struct {
    Text string `json:"text"`
}

type ClientMessage struct {
    Sources *SourceCode `json:"source_code"`
}

type Event = string

type OutputEntry = string

type RunnerMessage struct {
    Events []Event `json:"events"`
    Outputs []OutputEntry `json:"outputs"`
}

func handleRun(w http.ResponseWriter, r *http.Request) {
    log.Println("Got a request")
    if r.Body == nil {
        log.Printf("No body in request")
        w.WriteHeader(http.StatusBadRequest)
        return
    }
    sourceText, err := io.ReadAll(r.Body)
    if err != nil {
        log.Printf("Failed to read all from body: %v", err)
        return
    }

    c, err := wsUpgrader.Upgrade(w, r, nil)
    if err != nil {
        log.Printf("Failed to upgrade to Websocket: %v", err)
        return
    }
    defer c.Close()

    log.Println("Upgraded to Websocket")
	/*mt, data, err := c.ReadMessage()
    if err != nil {
        log.Printf("Failed to read message from client: %v", err)
        return
    }*/

    /*msg := ClientMessage{}
    err = json.Unmarshal(data, &msg)
    if err != nil {
        log.Printf("Failed to unmarshal message from client: %v, text: %s", err, data[:64])
        return
    }

    if msg.Sources == nil {
        log.Printf("No source code sent from client")
        return
    }*/

    outputPath := "/tmp/a.out"
    args := []string{"-o", outputPath,"-x", "c++", "-"}
    command := "/usr/bin/g++"

    log.Println("Running a compiler")
    proc := exec.Command(command, args...)

    stdin, err := proc.StdinPipe()
    if err != nil {
        log.Printf("Failed to run attach stdin pipe: %v", err)
        return
    }

    stdin.Write(sourceText)
    stdin.Close()

    output, err := proc.CombinedOutput()
    if err != nil {
        log.Printf("Failed to run compiler process and get combined output: %v", err)
        log.Printf("Output: %v", string(output))
        return
    }

    exitCode := proc.ProcessState.ExitCode()
    outputStr := string(output)

    log.Printf("Compiler done, text: |%s|, exit code: %d\n", outputStr, exitCode)

    outMsg := RunnerMessage{Events: make([]Event, 0), Outputs: make([]OutputEntry, 0)}
    outMsg.Events = append(outMsg.Events, "started:compilation")
    if outputStr != "" {
        outMsg.Outputs = append(outMsg.Outputs, outputStr)
    }
    outBytes, err := json.Marshal(&outMsg)
    if err != nil {
        log.Println("Failed to marshal output message:", err)
        return
    }
    log.Println("OUT:", string(outBytes))
    err = c.WriteMessage(websocket.TextMessage, outBytes)
    if err != nil {
        log.Println("Failed to write to websocket:", err)
        return
    }

    // Run the program
    log.Println("Running a compiler")
    command = outputPath
    proc = exec.Command(command)

    output, err = proc.CombinedOutput()
    /*if err != nil {
        log.Printf("Failed to run user process and get combined output: %v", err)
        log.Printf("Output: %v", string(output))
        return
    }*/

    exitCode = proc.ProcessState.ExitCode()
    outputStr = string(output)
    log.Printf("User program done, text: |%s|, exit code: %d\n", outputStr, exitCode)

    /*err = proc.Run()
    if err != nil {
        log.Printf("Failed to run compiler process: %v", err)
        return
    }*/
    //err = c.WriteMessage(mt, output)
    outMsg = RunnerMessage{Events: make([]Event, 0), Outputs: make([]OutputEntry, 0)}
    outMsg.Events = append(outMsg.Events, "started:run")
    if outputStr != "" {
        outMsg.Outputs = append(outMsg.Outputs, outputStr)
    }
    outBytes, err = json.Marshal(&outMsg)
    if err != nil {
        log.Println("Failed to marshal output message:", err)
        return
    }
    log.Println("OUT:", string(outBytes))
    err = c.WriteMessage(websocket.TextMessage, outBytes)
    if err != nil {
        log.Println("Failed to write to websocket:", err)
        return
    }

    log.Println("Done")
}

func main() {
    http.HandleFunc("/health", handleHealth)
    http.HandleFunc("/run", handleRun)

    PORT := ":1556"
    log.Println("Starting serving at", PORT)
    http.ListenAndServe("0.0.0.0" + PORT, nil)
}
