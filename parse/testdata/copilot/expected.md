---
schema_version: 1
expected_failure: false
bug: false
source_hash: 9431b7827795b4e9e89a6e6154343eabfeb0ae5adeab40ad659c2a0ffc2d73ac
---

## Expected Events

```ndjson
{"type":"init","message":"session started","timestamp":"2026-10-30T12:00:01Z"}
{"type":"action","message":"view /tmp/noodle-fx/hello.txt","timestamp":"2026-10-30T12:00:03Z"}
{"type":"action","message":"text:noodle fixture","timestamp":"2026-10-30T12:00:05Z"}
{"type":"result","message":"turn complete","timestamp":"2026-10-30T12:00:06Z"}
```

## Expected Error

```json

```
