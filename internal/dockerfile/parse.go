package dockerfile

import (
	"fmt"
	"strings"

	"github.com/moby/buildkit/frontend/dockerfile/instructions"
	"github.com/moby/buildkit/frontend/dockerfile/parser"
	"github.com/moby/buildkit/frontend/dockerfile/shell"
)

// Dockerfile represents a parsed Dockerfile.
type Dockerfile struct {
	// Raw is the original Dockerfile content.
	Raw string

	// Preamble holds ARG instructions before the first FROM.
	Preamble *Preamble

	// Stages is the ordered list of build stages.
	Stages []*Stage

	// StagesByTarget maps stage names to stages for quick lookup.
	StagesByTarget map[string]*Stage
}

// Preamble holds instructions before the first FROM (typically ARG).
type Preamble struct {
	Args []instructions.KeyValuePairOptional
}

// Stage represents a single build stage (FROM ... to next FROM or EOF).
type Stage struct {
	// Image is the base image (FROM value).
	Image string

	// Target is the stage name (AS value), empty if unnamed.
	Target string

	// Envs are ENV instructions in this stage.
	Envs []instructions.KeyValuePair

	// Args are ARG instructions in this stage.
	Args []instructions.KeyValuePairOptional

	// Users are USER instructions in this stage.
	Users []string

	// Commands is the list of parsed instructions in this stage.
	Commands []instructions.Command
}

// Parse parses Dockerfile content into a structured Dockerfile.
func Parse(content string) (*Dockerfile, error) {
	result, err := parser.Parse(strings.NewReader(content))
	if err != nil {
		return nil, fmt.Errorf("parsing Dockerfile: %w", err)
	}

	df := &Dockerfile{
		Raw:            content,
		Preamble:       &Preamble{},
		StagesByTarget: make(map[string]*Stage),
	}

	stages, metaArgs, err := instructions.Parse(result.AST, nil)
	if err != nil {
		return nil, fmt.Errorf("parsing instructions: %w", err)
	}

	// Preamble: ARGs before the first FROM.
	for _, arg := range metaArgs {
		df.Preamble.Args = append(df.Preamble.Args, arg.Args...)
	}

	// Parse stages.
	for _, s := range stages {
		stage := &Stage{
			Image:    s.BaseName,
			Target:   s.Name,
			Commands: s.Commands,
		}

		for _, cmd := range s.Commands {
			switch c := cmd.(type) {
			case *instructions.EnvCommand:
				stage.Envs = append(stage.Envs, c.Env...)
			case *instructions.ArgCommand:
				stage.Args = append(stage.Args, c.Args...)
			case *instructions.UserCommand:
				stage.Users = append(stage.Users, c.User)
			}
		}

		df.Stages = append(df.Stages, stage)
		if stage.Target != "" {
			df.StagesByTarget[stage.Target] = stage
		}
	}

	return df, nil
}

// FindBaseImage resolves the base image for the given target stage,
// expanding ARG variables and following stage references.
// If target is empty, the last stage is used.
func (d *Dockerfile) FindBaseImage(buildArgs map[string]string, target string) string {
	stage := d.findTargetStage(target)
	if stage == nil {
		return ""
	}

	image := d.expandVariables(stage.Image, buildArgs, nil, make(map[string]bool))
	return image
}

// FindUserStatement returns the last USER instruction in the target stage
// chain, resolving ARG/ENV variables. Returns empty string if no USER found.
func (d *Dockerfile) FindUserStatement(buildArgs, baseImageEnv map[string]string, target string) string {
	stage := d.findTargetStage(target)
	if stage == nil {
		return ""
	}

	return d.findUser(stage, buildArgs, baseImageEnv, make(map[string]bool))
}

// BuildContextFiles returns all source paths from COPY and ADD instructions
// that reference local files (not from other stages).
func (d *Dockerfile) BuildContextFiles() []string {
	seen := make(map[string]bool)
	var files []string

	for _, stage := range d.Stages {
		for _, cmd := range stage.Commands {
			var sources []string
			switch c := cmd.(type) {
			case *instructions.CopyCommand:
				if c.From != "" {
					continue // COPY --from=stage, skip
				}
				sources = c.SourcePaths
			case *instructions.AddCommand:
				sources = c.SourcePaths
			default:
				continue
			}
			for _, src := range sources {
				if !seen[src] {
					seen[src] = true
					files = append(files, src)
				}
			}
		}
	}

	return files
}

// EnsureFinalStageName adds "AS <name>" to the last FROM if it doesn't
// have one. Returns (stageName, modifiedContent, error).
// If the stage already has a name, modifiedContent is empty.
func EnsureFinalStageName(content, defaultName string) (string, string, error) {
	df, err := Parse(content)
	if err != nil {
		return "", "", err
	}

	if len(df.Stages) == 0 {
		return "", "", fmt.Errorf("no stages found in Dockerfile")
	}

	lastStage := df.Stages[len(df.Stages)-1]
	if lastStage.Target != "" {
		return lastStage.Target, "", nil
	}

	// Find the last FROM line and append AS <name>.
	modified := addStageName(content, defaultName)
	return defaultName, modified, nil
}

// RemoveSyntaxVersion strips the # syntax=... directive from Dockerfile content.
func RemoveSyntaxVersion(content string) string {
	lines := strings.Split(content, "\n")
	var result []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "# syntax=") || strings.HasPrefix(trimmed, "#syntax=") {
			continue
		}
		result = append(result, line)
	}
	return strings.Join(result, "\n")
}

// findTargetStage returns the stage matching the target name,
// or the last stage if target is empty.
func (d *Dockerfile) findTargetStage(target string) *Stage {
	if len(d.Stages) == 0 {
		return nil
	}

	if target == "" {
		return d.Stages[len(d.Stages)-1]
	}

	if stage, ok := d.StagesByTarget[target]; ok {
		return stage
	}

	return nil
}

// findUser walks the stage chain to find the last USER instruction.
func (d *Dockerfile) findUser(stage *Stage, buildArgs, baseImageEnv map[string]string, seen map[string]bool) string {
	if stage == nil {
		return ""
	}

	// Prevent circular references.
	key := stage.Image
	if stage.Target != "" {
		key = stage.Target
	}
	if seen[key] {
		return ""
	}
	seen[key] = true

	// If this stage has USER instructions, use the last one.
	if len(stage.Users) > 0 {
		user := stage.Users[len(stage.Users)-1]
		return d.expandVariables(user, buildArgs, baseImageEnv, make(map[string]bool))
	}

	// Walk to parent stage if the base image references another stage.
	if parent, ok := d.StagesByTarget[stage.Image]; ok {
		return d.findUser(parent, buildArgs, baseImageEnv, seen)
	}

	return ""
}

// expandVariables expands ${VAR} references in a string using build args,
// stage environment, and preamble args.
func (d *Dockerfile) expandVariables(value string, buildArgs, baseImageEnv map[string]string, seenStages map[string]bool) string {
	env := &envResolver{
		buildArgs:    buildArgs,
		baseImageEnv: baseImageEnv,
		preamble:     d.Preamble,
	}

	lex := shell.NewLex('\\')
	result, _, err := lex.ProcessWord(value, env)
	if err != nil {
		return value
	}

	// If the result references another stage, check for circular refs.
	if parent, ok := d.StagesByTarget[result]; ok && !seenStages[result] {
		seenStages[result] = true
		return d.expandVariables(parent.Image, buildArgs, baseImageEnv, seenStages)
	}

	return result
}

// envResolver implements shell.EnvGetter for variable resolution.
type envResolver struct {
	buildArgs    map[string]string
	baseImageEnv map[string]string
	preamble     *Preamble
}

func (e *envResolver) Get(key string) (string, bool) {
	// 1. Build args override everything.
	if v, ok := e.buildArgs[key]; ok {
		return v, true
	}

	// 2. Preamble ARG defaults.
	if e.preamble != nil {
		for _, arg := range e.preamble.Args {
			if arg.Key == key && arg.Value != nil {
				return *arg.Value, true
			}
		}
	}

	// 3. Base image environment.
	if v, ok := e.baseImageEnv[key]; ok {
		return v, true
	}

	return "", false
}

func (e *envResolver) Keys() []string {
	keys := make(map[string]bool)
	for k := range e.buildArgs {
		keys[k] = true
	}
	if e.preamble != nil {
		for _, arg := range e.preamble.Args {
			keys[arg.Key] = true
		}
	}
	for k := range e.baseImageEnv {
		keys[k] = true
	}

	result := make([]string, 0, len(keys))
	for k := range keys {
		result = append(result, k)
	}
	return result
}

// addStageName appends "AS <name>" to the last FROM line.
func addStageName(content, name string) string {
	lines := strings.Split(content, "\n")

	// Find the last FROM line.
	lastFromIdx := -1
	for i, line := range lines {
		trimmed := strings.TrimSpace(strings.ToUpper(line))
		if strings.HasPrefix(trimmed, "FROM ") {
			lastFromIdx = i
		}
	}

	if lastFromIdx < 0 {
		return content
	}

	// Handle line continuations.
	line := strings.TrimRight(lines[lastFromIdx], " \t")
	if strings.HasSuffix(line, "\\") {
		// Find the actual last line of the FROM instruction.
		for i := lastFromIdx; i < len(lines); i++ {
			trimmed := strings.TrimRight(lines[i], " \t")
			if !strings.HasSuffix(trimmed, "\\") {
				lines[i] = trimmed + " AS " + name
				break
			}
		}
	} else {
		lines[lastFromIdx] = line + " AS " + name
	}

	return strings.Join(lines, "\n")
}
