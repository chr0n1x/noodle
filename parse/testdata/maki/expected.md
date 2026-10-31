---
schema_version: 1
expected_failure: false
bug: false
source_hash: 48df85bcb0c130f3ff3f314c90f92d027726118ee82c95881f96473c92dcb450
---

## Expected Events

```ndjson
{"type":"init","message":"session started","timestamp":"2026-10-30T12:00:00Z"}
{"type":"action","message":"read /tmp/noodle-fx/hello.txt","timestamp":"2026-10-30T12:00:01Z"}
{"type":"action","message":"text:noodle fixture","timestamp":"2026-10-30T12:00:03Z"}
{"type":"result","message":"turn complete","timestamp":"2026-10-30T12:00:04Z","cost_usd":0.0821508,"tokens_in":6,"tokens_out":115}
```

## Expected Error

```json

```
