# Practicode worker
A service written in Go that gets client's source code, compiles it, runs the compiled program and returns its output text.

Its responsibilities:
- connects to a practicode backend proxy via WS and gets run requests from clients
- compiles source code according to specified rules
- sends compiler's and user program's output back to the user (through backend proxy
- enforces limitations: used memory, used time, output file size, number of threads and others
- sends events to a client, such as: compilation started, ended, program started, etc

## How to build
`make` or `sudo docker build -f docker/Dockerfile.cpp -t practicode-worker .`
