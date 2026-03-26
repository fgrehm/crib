package engine

// ProgressPhase identifies a stage of the devcontainer lifecycle.
type ProgressPhase string

const (
	PhaseInit     ProgressPhase = "init"
	PhaseBuild    ProgressPhase = "build"
	PhaseCreate   ProgressPhase = "create"
	PhasePlugins  ProgressPhase = "plugins"
	PhaseHooks    ProgressPhase = "hooks"
	PhaseSnapshot ProgressPhase = "snapshot"
	PhaseRestart  ProgressPhase = "restart"
)

// ProgressEvent carries a structured progress update from the engine.
type ProgressEvent struct {
	Phase   ProgressPhase
	Message string
	Done    bool // true when this event marks the end of its phase
}
