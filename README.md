# Sentinel2 Uploader

Uploader application for Sentinel2.

## Requirements

- Go 1.25+
- Task 3.x
- `golangci-lint` v2 (for lint task)
- `zip` (for release archive task)

## Common Tasks

- `task build`: build versioned release archives for Linux/Windows/macOS.
- `task lint:go`: run Go lint checks.
- `task test:go`: run Go tests with `-tags headless`.
- `task clean`: remove build artifacts and local tool caches.

## Local Run

- `task run`: run uploader for current host platform.
- `task run:headless`: run current host platform binary with `--headless`.

Build+run variants are also available:
- `task build:run`
- `task build:run:headless`
