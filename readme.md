# Practicode Runjail
Service written in Go that gets source code, compiles it, run the compiled program and returns its output text.

Its responsibilities:
- listens to a Websocket
- compiles the source code according specified rules
- sends compiler's and user program's output back to the user
- enforces limitations: used memory, used time, output file size, number of threads and others
- sends events to a client, such as: compilation started, ended, program started, etc

## How to build
`make` or `sudo docker build -f docker/Dockerfile -t practicode-runjail .`
