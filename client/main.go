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

type SourceFile struct {
    Name string `json:"name"`
	Text string `json:"text"`
}

type ClientMessage struct {
	SourceFiles []SourceFile `json:"source_files"`
}

var addr = flag.String("addr", "ws://localhost:1556/run", "runner ws address")
var inputFile = flag.String("input", "", "")

func main() {
	flag.Parse()

	text, err := ioutil.ReadFile(*inputFile)
	if err != nil {
		log.Fatal("Failed to open source code file:", err)
	}

	c, _, err := websocket.DefaultDialer.Dial(*addr, nil)
	if err != nil {
		log.Fatal("Failed to dial:", err)
	}
	defer c.Close()

	encodedText := base64.StdEncoding.EncodeToString(text)

    msg := ClientMessage{SourceFiles: []SourceFile{SourceFile{Name: "src0", Text: encodedText}}}
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
				log.Printf("Output: %s", string(outputDecoded))
			} else {
				fmt.Println("Message:", string(message))
			}
		}
	}()

	<-done
}
