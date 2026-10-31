---
schema_version: 1
expected_failure: false
bug: false
source_hash: 698508c53ee295c20eff8b945a9328d4f0b486cb5473909c402871c02bb7f137
---

## Expected Events

```ndjson
{"type":"action","message":"$ ls /nonexistent-noodle-dir","timestamp":"2026-10-30T12:00:01Z"}
{"type":"error","message":"tool failed: ls: /nonexistent-noodle-dir: No such file or directory\n<shellId: 0 completed with exit code 1>","timestamp":"2026-10-30T12:00:02Z"}
{"type":"error","message":"tool failed: Path not allowed","timestamp":"2026-10-30T12:00:04Z"}
{"type":"error","message":"copilot exited with code 1","timestamp":"2026-10-30T12:00:05Z"}
```

## Expected Error

```json

```
