# Practicode Runner
Service written in Go that gets source code, compiles it, run the compiled program and returns its output text.

Its responsibilities:
- lives for a some time, waiting for a particular user
- accepts source code via HTTP request
- checks authentication token
- upgrades connection to Websocket
- does not allow parallel requests (one user can compile one program at a time)
- controls maximum request rate

- checks against limitations: size in bytes
- searches for forbidden symbols
- does not compile if source code hasn't changed

- compiles the source code according specified rules
- sends compiler output back to the user
- enforces limitations for the compiler: used memory, used time, output file size

- runs the program and sends its output back to the user
- accepts input for the program
- enforces limitations for the program: used memory, used time, output size
- sends events to the user, such as: compilation started, ended, program started, etc
- skips compilation if the source code didn't change

- exits after some time being idle
