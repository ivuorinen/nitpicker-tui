---
id: audit-1a2b3c4d
auditor: audit
severity: high
category: correctness
area: internal/cache/cache.go
status: open
found: 2026-10-20
location: internal/cache/cache.go:40-52
---

# Cache entry read after its lock is released

## Problem

`Get` copies the entry pointer under the read lock, then reads its fields
after `RUnlock`. A concurrent `Set` can replace the fields in between.

## Evidence

```go
c.mu.RLock()
e := c.items[key]
c.mu.RUnlock()
return e.value, e.expires.After(now)
```

## Impact

A caller can receive a value with the expiry of a different write.

## Fix

Read `e.value` and `e.expires` before `RUnlock`, or store entries by value.
