---
schema_version: 1
expected_failure: false
bug: false
source_hash: 8d01e444968eff0ad75a051b244e85de04f454e68d3b6c8e440e9827f8879ddb
---

## Expected Events

```ndjson
{"type":"init","message":"session started","timestamp":"2026-10-30T12:00:00Z"}
{"type":"action","message":"read hello.txt","timestamp":"2026-10-30T12:00:02Z"}
{"type":"action","message":"text:noodle fixture","timestamp":"2026-10-30T12:00:04Z"}
{"type":"result","message":"turn complete","timestamp":"2026-10-30T12:00:05Z","cost_usd":0.0251847,"tokens_in":6,"tokens_out":57}
```

## Expected Error

```json

```
