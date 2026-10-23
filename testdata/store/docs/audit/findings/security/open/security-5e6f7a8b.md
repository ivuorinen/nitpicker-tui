---
id: security-5e6f7a8b
auditor: security
severity: critical
category: security
area: cmd/server/main.go
status: open
found: 2026-10-21
---

# Request bodies logged with their credentials

## Problem

The debug middleware logs every request body, including the `password`
field of the login form.

## Evidence

`log.Printf("body=%s", body)` runs before the handler, for every route.

## Impact

Anyone with log access reads user passwords.

## Fix

Drop the body from the log line, or log only the route and status.
