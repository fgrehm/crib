package dockerfile

import (
	"strings"
	"testing"
)

func TestParse_SimpleDockerfile(t *testing.T) {
	df, err := Parse("FROM ubuntu:22.04\nRUN echo hello\n")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(df.Stages) != 1 {
		t.Fatalf("expected 1 stage, got %d", len(df.Stages))
	}
	if df.Stages[0].Image != "ubuntu:22.04" {
		t.Errorf("Image = %q, want %q", df.Stages[0].Image, "ubuntu:22.04")
	}
	if df.Stages[0].Target != "" {
		t.Errorf("Target = %q, want empty", df.Stages[0].Target)
	}
}

func TestParse_MultistageDockerfile(t *testing.T) {
	content := `FROM golang:1.21 AS builder
RUN go build -o app
FROM alpine:3.18 AS runner
COPY --from=builder /app /app
`
	df, err := Parse(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(df.Stages) != 2 {
		t.Fatalf("expected 2 stages, got %d", len(df.Stages))
	}
	if df.Stages[0].Target != "builder" {
		t.Errorf("stage 0 Target = %q, want %q", df.Stages[0].Target, "builder")
	}
	if df.Stages[1].Target != "runner" {
		t.Errorf("stage 1 Target = %q, want %q", df.Stages[1].Target, "runner")
	}
	if _, ok := df.StagesByTarget["builder"]; !ok {
		t.Error("StagesByTarget should contain 'builder'")
	}
}

func TestParse_PreambleArgs(t *testing.T) {
	content := `ARG BASE_IMAGE=ubuntu
ARG BASE_VERSION=22.04
FROM ${BASE_IMAGE}:${BASE_VERSION}
`
	df, err := Parse(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(df.Preamble.Args) != 2 {
		t.Fatalf("expected 2 preamble args, got %d", len(df.Preamble.Args))
	}
	if df.Preamble.Args[0].Key != "BASE_IMAGE" {
		t.Errorf("preamble arg 0 Key = %q, want %q", df.Preamble.Args[0].Key, "BASE_IMAGE")
	}
}

func TestParse_EnvAndUser(t *testing.T) {
	content := `FROM ubuntu:22.04
ENV HOME=/home/dev
ENV EDITOR=vim
USER dev
RUN echo hello
USER root
`
	df, err := Parse(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	stage := df.Stages[0]
	if len(stage.Envs) != 2 {
		t.Errorf("expected 2 ENVs, got %d", len(stage.Envs))
	}
	if len(stage.Users) != 2 {
		t.Errorf("expected 2 USERs, got %d", len(stage.Users))
	}
	if stage.Users[1] != "root" {
		t.Errorf("last USER = %q, want %q", stage.Users[1], "root")
	}
}

func TestFindBaseImage_NoArgs(t *testing.T) {
	df, err := Parse("FROM ubuntu:22.04\n")
	if err != nil {
		t.Fatal(err)
	}

	image := df.FindBaseImage(nil, "")
	if image != "ubuntu:22.04" {
		t.Errorf("got %q, want %q", image, "ubuntu:22.04")
	}
}

func TestFindBaseImage_WithARGDefaults(t *testing.T) {
	content := `ARG BASE=alpine
ARG VERSION=3.18
FROM ${BASE}:${VERSION}
`
	df, err := Parse(content)
	if err != nil {
		t.Fatal(err)
	}

	image := df.FindBaseImage(nil, "")
	if image != "alpine:3.18" {
		t.Errorf("got %q, want %q", image, "alpine:3.18")
	}
}

func TestFindBaseImage_WithBuildArgOverride(t *testing.T) {
	content := `ARG BASE=alpine
FROM ${BASE}:latest
`
	df, err := Parse(content)
	if err != nil {
		t.Fatal(err)
	}

	image := df.FindBaseImage(map[string]string{"BASE": "ubuntu"}, "")
	if image != "ubuntu:latest" {
		t.Errorf("got %q, want %q", image, "ubuntu:latest")
	}
}

func TestFindBaseImage_MultistageThroughStageRef(t *testing.T) {
	content := `FROM ubuntu:22.04 AS base
RUN echo setup
FROM base AS builder
RUN echo build
`
	df, err := Parse(content)
	if err != nil {
		t.Fatal(err)
	}

	// FindBaseImage on "builder" should resolve through "base" to "ubuntu:22.04".
	image := df.FindBaseImage(nil, "builder")
	if image != "ubuntu:22.04" {
		t.Errorf("got %q, want %q", image, "ubuntu:22.04")
	}
}

func TestFindBaseImage_SpecificTarget(t *testing.T) {
	content := `FROM golang:1.21 AS builder
FROM alpine:3.18 AS runner
`
	df, err := Parse(content)
	if err != nil {
		t.Fatal(err)
	}

	image := df.FindBaseImage(nil, "builder")
	if image != "golang:1.21" {
		t.Errorf("got %q, want %q", image, "golang:1.21")
	}

	image = df.FindBaseImage(nil, "runner")
	if image != "alpine:3.18" {
		t.Errorf("got %q, want %q", image, "alpine:3.18")
	}
}

func TestFindUserStatement_ExplicitUser(t *testing.T) {
	content := `FROM ubuntu:22.04
USER vscode
`
	df, err := Parse(content)
	if err != nil {
		t.Fatal(err)
	}

	user := df.FindUserStatement(nil, nil, "")
	if user != "vscode" {
		t.Errorf("got %q, want %q", user, "vscode")
	}
}

func TestFindUserStatement_LastUserWins(t *testing.T) {
	content := `FROM ubuntu:22.04
USER dev
USER root
`
	df, err := Parse(content)
	if err != nil {
		t.Fatal(err)
	}

	user := df.FindUserStatement(nil, nil, "")
	if user != "root" {
		t.Errorf("got %q, want %q", user, "root")
	}
}

func TestFindUserStatement_NoUser(t *testing.T) {
	content := `FROM ubuntu:22.04
RUN echo hello
`
	df, err := Parse(content)
	if err != nil {
		t.Fatal(err)
	}

	user := df.FindUserStatement(nil, nil, "")
	if user != "" {
		t.Errorf("got %q, want empty", user)
	}
}

func TestFindUserStatement_WithARGVariable(t *testing.T) {
	content := `ARG USERNAME=devuser
FROM ubuntu:22.04
USER ${USERNAME}
`
	df, err := Parse(content)
	if err != nil {
		t.Fatal(err)
	}

	user := df.FindUserStatement(nil, nil, "")
	if user != "devuser" {
		t.Errorf("got %q, want %q", user, "devuser")
	}
}

func TestFindUserStatement_MultistageResolution(t *testing.T) {
	content := `FROM ubuntu:22.04 AS base
USER devuser
FROM base AS builder
RUN echo build
`
	df, err := Parse(content)
	if err != nil {
		t.Fatal(err)
	}

	// "builder" has no USER, should walk to "base" and find "devuser".
	user := df.FindUserStatement(nil, nil, "builder")
	if user != "devuser" {
		t.Errorf("got %q, want %q", user, "devuser")
	}
}

func TestBuildContextFiles(t *testing.T) {
	content := `FROM ubuntu:22.04 AS builder
COPY app /app
COPY files /files
ADD data.tar.gz /data
FROM builder AS runner
COPY --from=builder /app /app
ADD extra /extra
`
	df, err := Parse(content)
	if err != nil {
		t.Fatal(err)
	}

	files := df.BuildContextFiles()

	// Should include: app, files, data.tar.gz, extra
	// Should NOT include: /app (from COPY --from=builder)
	want := map[string]bool{
		"app":         true,
		"files":       true,
		"data.tar.gz": true,
		"extra":       true,
	}

	if len(files) != len(want) {
		t.Fatalf("got %d files %v, want %d", len(files), files, len(want))
	}
	for _, f := range files {
		if !want[f] {
			t.Errorf("unexpected file %q", f)
		}
	}
}

func TestEnsureFinalStageName_AddsName(t *testing.T) {
	content := "FROM ubuntu:22.04\nRUN echo hello\n"
	name, modified, err := EnsureFinalStageName(content, "crib_final")
	if err != nil {
		t.Fatal(err)
	}

	if name != "crib_final" {
		t.Errorf("name = %q, want %q", name, "crib_final")
	}
	if !strings.Contains(modified, "AS crib_final") {
		t.Errorf("modified should contain 'AS crib_final', got:\n%s", modified)
	}
}

func TestEnsureFinalStageName_PreservesExisting(t *testing.T) {
	content := "FROM ubuntu:22.04 AS myname\nRUN echo hello\n"
	name, modified, err := EnsureFinalStageName(content, "crib_final")
	if err != nil {
		t.Fatal(err)
	}

	if name != "myname" {
		t.Errorf("name = %q, want %q", name, "myname")
	}
	if modified != "" {
		t.Errorf("modified should be empty when name exists, got:\n%s", modified)
	}
}

func TestRemoveSyntaxVersion(t *testing.T) {
	content := "# syntax=docker/dockerfile:1.4\nFROM ubuntu:22.04\n"
	result := RemoveSyntaxVersion(content)

	if strings.Contains(result, "syntax=") {
		t.Errorf("should not contain syntax directive, got:\n%s", result)
	}
	if !strings.Contains(result, "FROM ubuntu:22.04") {
		t.Error("should preserve FROM instruction")
	}
}

func TestRemoveSyntaxVersion_NoSyntax(t *testing.T) {
	content := "FROM ubuntu:22.04\nRUN echo hello\n"
	result := RemoveSyntaxVersion(content)

	if result != content {
		t.Errorf("should be unchanged, got:\n%s", result)
	}
}

func TestParse_EmptyContent(t *testing.T) {
	_, err := Parse("")
	if err == nil {
		t.Fatal("expected error for empty Dockerfile")
	}
}

func TestParse_InvalidContent(t *testing.T) {
	// "INVALID" is not a valid Dockerfile command, but the parser is lenient.
	// Only truly broken syntax causes errors.
	_, err := Parse("FROM\n")
	if err == nil {
		t.Fatal("expected error for invalid FROM")
	}
}

func TestFindBaseImage_EmptyStages(t *testing.T) {
	df := &Dockerfile{
		StagesByTarget: make(map[string]*Stage),
	}

	image := df.FindBaseImage(nil, "")
	if image != "" {
		t.Errorf("got %q, want empty", image)
	}
}

func TestFindBaseImage_NonexistentTarget(t *testing.T) {
	df, err := Parse("FROM ubuntu:22.04\n")
	if err != nil {
		t.Fatal(err)
	}

	image := df.FindBaseImage(nil, "nonexistent")
	if image != "" {
		t.Errorf("got %q, want empty", image)
	}
}

func TestFindUserStatement_CircularReference(t *testing.T) {
	// Manually construct a circular stage reference to verify no infinite loop.
	df := &Dockerfile{
		Stages: []*Stage{
			{Image: "stageB", Target: "stageA"},
			{Image: "stageA", Target: "stageB"},
		},
		StagesByTarget: map[string]*Stage{},
	}
	df.StagesByTarget["stageA"] = df.Stages[0]
	df.StagesByTarget["stageB"] = df.Stages[1]

	// Should return empty string without hanging.
	user := df.FindUserStatement(nil, nil, "stageA")
	if user != "" {
		t.Errorf("got %q, want empty for circular reference", user)
	}
}
