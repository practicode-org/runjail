package main

import (
    "encoding/base64"
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

    encodedText := base64.StdEncoding.EncodeToString(text)

    msg := ClientMessage{SourceCode: &SourceCode{Text: encodedText}}
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

            msg := struct {
                Output string `json:"output"`
            }{}
            err = json.Unmarshal(message, &msg)
			if err != nil {
				log.Println("Failed to Unmarshal JSON message:", err)
			}

            if msg.Output != "" {
                outputDecoded, err := base64.StdEncoding.DecodeString(msg.Output)
                if err != nil {
                    log.Println("Failed to Decode base64:", err)
                    continue
                }
                log.Println("Output:", string(outputDecoded))
            }
		}
	}()

    <-done
}
