package engine

import (
	"errors"
	"fmt"
	"testing"
)

func TestErrNoContainer_As(t *testing.T) {
	wrapped := fmt.Errorf("outer: %w", &ErrNoContainer{WorkspaceID: "my-ws"})

	var target *ErrNoContainer
	if !errors.As(wrapped, &target) {
		t.Fatal("errors.As should unwrap ErrNoContainer")
	}
	if target.WorkspaceID != "my-ws" {
		t.Errorf("WorkspaceID = %q, want %q", target.WorkspaceID, "my-ws")
	}
	if target.Error() == "" {
		t.Error("Error() should return a non-empty string")
	}
}

func TestErrContainerStopped_As(t *testing.T) {
	wrapped := fmt.Errorf("outer: %w", &ErrContainerStopped{WorkspaceID: "my-ws", ContainerID: "abc123"})

	var target *ErrContainerStopped
	if !errors.As(wrapped, &target) {
		t.Fatal("errors.As should unwrap ErrContainerStopped")
	}
	if target.WorkspaceID != "my-ws" {
		t.Errorf("WorkspaceID = %q, want %q", target.WorkspaceID, "my-ws")
	}
	if target.ContainerID != "abc123" {
		t.Errorf("ContainerID = %q, want %q", target.ContainerID, "abc123")
	}
	if target.Error() == "" {
		t.Error("Error() should return a non-empty string")
	}
}

func TestErrComposeNotAvailable_As(t *testing.T) {
	wrapped := fmt.Errorf("outer: %w", &ErrComposeNotAvailable{})

	var target *ErrComposeNotAvailable
	if !errors.As(wrapped, &target) {
		t.Fatal("errors.As should unwrap ErrComposeNotAvailable")
	}
	if target.Error() == "" {
		t.Error("Error() should return a non-empty string")
	}
}
