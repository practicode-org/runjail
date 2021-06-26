package main

import (
    "encoding/json"
    "flag"
    "fmt"
    "github.com/gorilla/websocket"
    "io/ioutil"
    "log"
)

type SourceCode struct {
    Text string `json:"text"`
}

type ClientMessage struct {
    SourceCode *SourceCode `json:"source_code"`
}


var addr = flag.String("addr", "ws://localhost:1556/run", "runner ws address")
var inputFile = flag.String("input", "", "")

func main() {
    flag.Parse()

    c, _, err := websocket.DefaultDialer.Dial(*addr, nil)
	if err != nil {
		log.Fatal("Failed to dial:", err)
	}
	defer c.Close()

    text, err := ioutil.ReadFile(*inputFile)
	if err != nil {
		log.Fatal("Failed to open source code file:", err)
	}

    msg := ClientMessage{SourceCode: &SourceCode{Text: string(text)}}
    jtext, err := json.Marshal(msg)
	if err != nil {
		log.Fatal("Failed to json marshal:", err)
	}

    err = c.WriteMessage(websocket.TextMessage, jtext)
    if err != nil {
        log.Fatal("Failed to ws write:", err)
    }

    done := make(chan struct{})

    go func() {
		defer close(done)
		for {
			_, message, err := c.ReadMessage()
			if err != nil {
				log.Println("Failed to ws read:", err)
				return
			}
            fmt.Println("Message:", string(message))
		}
	}()

    <-done
}
