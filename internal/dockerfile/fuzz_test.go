package dockerfile

import "testing"

func FuzzParse(f *testing.F) {
	f.Add("FROM ubuntu:22.04")
	f.Add("FROM golang:1.22 AS builder\nRUN go build .\nFROM alpine:3.20\nCOPY --from=builder /app /app")
	f.Add("ARG BASE=ubuntu:22.04\nFROM ${BASE}\nUSER node")
	f.Add("FROM scratch")
	f.Add("# syntax=docker/dockerfile:1\nFROM node:20")
	f.Add("")
	f.Add("not a dockerfile")

	f.Fuzz(func(t *testing.T, content string) {
		// Parse must not panic on any input.
		df, err := Parse(content)
		if err != nil {
			return
		}

		// Exercise methods that process parsed output.
		df.FindBaseImage(nil, "")
		df.FindUserStatement(nil, nil, "")
	})
}

func FuzzEnsureFinalStageName(f *testing.F) {
	f.Add("FROM ubuntu:22.04", "dev_container")
	f.Add("FROM golang:1.22 AS builder\nFROM alpine:3.20", "dev_container")
	f.Add("FROM node:20 AS app", "dev_container")
	f.Add("", "dev_container")

	f.Fuzz(func(t *testing.T, content, name string) {
		// EnsureFinalStageName must not panic on any input.
		_, _, _ = EnsureFinalStageName(content, name)
	})
}
