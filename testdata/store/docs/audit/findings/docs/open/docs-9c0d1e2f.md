---
id: docs-9c0d1e2f
auditor: docs
severity: low
area: README.md
status: open
found: 2026-10-19
---

# README lists a flag that no longer exists

## Problem

The README documents `-config`, which was removed.

## Evidence

`flag.String("config", ...)` appears nowhere in the code.

## Impact

Users try a flag that fails with "flag provided but not defined".

## Fix

Remove the `-config` row from the README.
