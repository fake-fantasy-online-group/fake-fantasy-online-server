# Published client hello regression

`client-1.0.6-hello.payload.bin` is the payload (no opcode) emitted by the actual
`EndgameRequest("hello", requestID: "published-client-hello-probe")` and
`PrivateServerEndgameCodec.Encode` from published client commit
`36aab0ec93b1912905bd1e77165a63c951e50a6c`, tree
`fd1cd6ad44050972513661fe902c88c7f80743f1` (local equivalent HEAD e0e6f957).
Compiled using .NET 6 / Roslyn against the real Endgame application sources and
codec; no response fixture or manually invented request capabilities.

Future hello capability changes must regenerate this fixture with the client
serializer and run both protocol and scene PublishedClientHello regressions.
