# payments-api

[![CI has the Zoomies](https://zoomies.sh/badge.svg)](https://zoomies.sh)

A small charges service written with the Go standard library: create a charge,
read it back, and retry safely with an idempotency key.

This repository belongs to the public demo of
[Zoomies](https://zoomies.sh), a self-hosted GitHub Actions runner fleet. It
exists to give the demo fleet something to check out and build. The workflows in
`.github/workflows/` run on a schedule on a Zoomies runner, so the jobs you see
in the demo's web UI came from a repository like this one.

```sh
go test ./...
ADDR=127.0.0.1:8080 go run .
```

| Route | What it does |
| --- | --- |
| `POST /v1/charges` | Create a charge. Send `Idempotency-Key` to make a retry safe. |
| `GET /v1/charges/{id}` | Read a charge back. |
| `GET /healthz` | Answers `{"ok":true}`. |

Amounts are whole numbers of the currency's smallest unit, so `1250` is 12.50.
