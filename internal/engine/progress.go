package engine

// ProgressPhase identifies a stage of the devcontainer lifecycle.
type ProgressPhase string

const (
	PhaseInit    ProgressPhase = "init"
	PhaseBuild   ProgressPhase = "build"
	PhaseCreate  ProgressPhase = "create"
	PhasePlugins ProgressPhase = "plugins"
	PhaseHooks   ProgressPhase = "hooks"
	PhaseRestart ProgressPhase = "restart"
)

// ProgressEvent carries a structured progress update from the engine.
type ProgressEvent struct {
	Phase   ProgressPhase
	Message string
}
