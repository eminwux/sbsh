// Copyright 2025 Emiliano Spinella (eminwux)
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"errors"
	"fmt"
	"os"
	"time"
)

type TerminalController interface {
	Run(spec *TerminalSpec) error
	WaitReady() error
	WaitClose() error
	Ping(ping *PingMessage) (*PingMessage, error)
	Close(reason error) error
	Resize(ResizeArgs)
	Detach(id *ID) error
	Attach(id *ID, reply *ResponseWithFD) error
	Metadata() (*TerminalDoc, error)
	State() (*TerminalStatusMode, error)
	Stop(args *StopArgs) error
	Write(req *WriteRequest) error
	Subscribe(req *SubscribeRequest, reply *ResponseWithFD) error
}

type TerminalDoc struct {
	APIVersion Version          `json:"apiVersion" yaml:"apiVersion"`
	Kind       Kind             `json:"kind"       yaml:"kind"`
	Metadata   TerminalMetadata `json:"metadata"`
	Spec       TerminalSpec     `json:"spec"`
	Status     TerminalStatus   `json:"status"`
}

type TerminalMetadata struct {
	Name        string            `json:"name"                  yaml:"name"`
	Labels      map[string]string `json:"labels,omitempty"      yaml:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty" yaml:"annotations,omitempty"`
	CreatedAt   time.Time         `json:"createdAt,omitzero"    yaml:"createdAt,omitempty"`
}

// TerminalSpec defines how to run a terminal.
type TerminalSpec struct {
	ID     ID                `json:"id"`
	Kind   TerminalKind      `json:"kind"`
	Name   string            `json:"name"`
	Labels map[string]string `json:"labels"`

	Cwd         string   `json:"cwd,omitempty"`
	Command     string   `json:"command"`
	CommandArgs []string `json:"commandArgs"`
	EnvInherit  bool     `json:"inheritEnv"`
	Env         []string `json:"env"`
	Prompt      string   `json:"prompt"`
	SetPrompt   bool     `json:"setPrompt,omitempty"`

	RunPath     string `json:"runPath"`
	CaptureFile string `json:"captureFile"`
	LogFile     string `json:"logFile"`
	LogLevel    string `json:"logLevel"`
	SocketFile  string `json:"socketIO"`

	// MetadataDir overrides where the runner writes metadata.json. When
	// non-empty, the runner uses this directory verbatim and does NOT
	// create the legacy runPath/terminals/<id>/ subdir. The builder sets
	// this when all three of LogFile, CaptureFile, and SocketFile are
	// caller-supplied via With* options — the signal that the embedder
	// already owns the per-terminal directory layout and does not want
	// sbsh-injected subdirs under runPath. Empty preserves the legacy
	// behavior (and the discoverability of metadata.json via
	// pkg/discovery.ScanTerminals).
	MetadataDir string `json:"metadataDir,omitempty"`

	// SocketMode is the chmod applied to the control socket after Listen.
	// Zero means "use the runner default" (0600), preserving owner-only
	// behavior for callers that never set the field.
	SocketMode os.FileMode `json:"socketMode,omitempty"`
	// SocketGID is the numeric GID applied via chown(socket, -1, gid)
	// after Listen. nil leaves the group unchanged.
	SocketGID *int `json:"socketGid,omitempty"`

	// CaptureMode is the chmod applied to the per-terminal capture file
	// after Open. Zero means "use the runner default" (0600), preserving
	// owner-only behavior for callers that never set the field.
	CaptureMode os.FileMode `json:"captureMode,omitempty"`
	// CaptureGID is the numeric GID applied via chown(capture, -1, gid)
	// after Open. nil leaves the group unchanged.
	CaptureGID *int `json:"captureGid,omitempty"`

	// LogFileMode is the chmod applied to the per-terminal log file after
	// it is opened. Zero means "use the runner default" (0600), preserving
	// owner-only behavior for callers that never set the field.
	LogFileMode os.FileMode `json:"logFileMode,omitempty"`
	// LogFileGID is the numeric GID applied via chown(logfile, -1, gid)
	// after the log file is opened. nil leaves the group unchanged.
	LogFileGID *int `json:"logFileGid,omitempty"`

	ProfileName string     `json:"profileName"`
	Stages      StagesSpec `json:"stages"`

	// ShutdownGrace bounds how long the terminal waits for the child to
	// exit after SIGTERM before escalating to SIGKILL. Zero means "use the
	// runner's default" (30s, matching Kubernetes terminationGracePeriodSeconds).
	ShutdownGrace time.Duration `json:"shutdownGrace,omitempty"`

	// Children declares N child processes the terminal multiplexes onto a
	// single attach session. When empty, the spec retains its single-child
	// shape via the top-level Command/CommandArgs/CaptureFile fields. When
	// non-empty, the top-level Command must be empty (mutually exclusive)
	// and the supervisor spawns each child via its own ChildSpec. The
	// supervisor behavior that consumes this list (spawn, drain, gating,
	// switch, replay, lifecycle) ships in later phases of the supervisor
	// series; phase 1 introduces the schema only.
	Children []ChildSpec `json:"children,omitempty" yaml:"children,omitempty"`

	// CyclePrefix is the byte sequence the attach client interprets as the
	// "cycle to next child" escape (consumed in phase 4). Empty means "use
	// the supervisor default" (^A / "\x01"). Stored verbatim — the attach
	// client compares the inbound byte stream against this prefix literally.
	CyclePrefix string `json:"cyclePrefix,omitempty" yaml:"cyclePrefix,omitempty"`

	// SwitchReplayBytes bounds the trailing capture-tail length the
	// supervisor replays to the operator when cycling onto a child whose
	// PTY has already produced output (consumed in phase 5). Zero means
	// "use the supervisor default" (4096). Negative values are rejected by
	// Validate.
	SwitchReplayBytes int `json:"switchReplayBytes,omitempty" yaml:"switchReplayBytes,omitempty"`
}

// ChildSpec declares one child process supervised by a TerminalSpec whose
// Children list is non-empty. Each child is spawned with its own Command +
// CommandArgs and its own capture file; the supervisor multiplexes the
// children's PTY output onto whichever attach session is currently focused
// on that child (see TerminalSpec.CyclePrefix for the operator-visible
// cycle key).
//
// Turn is the cycle-order index used by phase 4's switch logic; children
// are visited in ascending Turn order, with ties broken by the order they
// appear in TerminalSpec.Children.
type ChildSpec struct {
	Name        string      `json:"name"                  yaml:"name"`
	Command     string      `json:"command"               yaml:"command"`
	CommandArgs []string    `json:"commandArgs,omitempty" yaml:"commandArgs,omitempty"`
	CaptureFile string      `json:"captureFile,omitempty" yaml:"captureFile,omitempty"`
	CaptureMode os.FileMode `json:"captureMode,omitempty" yaml:"captureMode,omitempty"`
	Turn        int         `json:"turn,omitempty"        yaml:"turn,omitempty"`
}

// Validate enforces the phase-1 schema rules on a TerminalSpec:
//
//   - Children may not coexist with a top-level Command (the two shapes are
//     mutually exclusive; the supervisor either runs one child via the
//     top-level Command/CommandArgs pair or N children via Children).
//   - At least one of Children or top-level Command must be set, so the
//     supervisor has something to spawn.
//   - Child names must be unique within a single spec, so phase 4's switch
//     logic can address children by name without ambiguity.
//   - Each child must carry a non-empty Name and Command (a child whose
//     identity or argv is unset cannot be supervised).
//   - SwitchReplayBytes must be non-negative.
//
// Validate is the canonical entry point library callers reach for before
// handing a spec to the runner. Phase 1 only adds the method; later phases
// wire it into the runner's startup path.
func (s *TerminalSpec) Validate() error {
	hasChildren := len(s.Children) > 0
	hasTopCmd := s.Command != ""

	if hasChildren && hasTopCmd {
		return errors.New("terminal spec: children and top-level command are mutually exclusive")
	}
	if !hasChildren && !hasTopCmd {
		return errors.New("terminal spec: either children or top-level command must be set")
	}
	if s.SwitchReplayBytes < 0 {
		return fmt.Errorf("terminal spec: switchReplayBytes must be non-negative, got %d", s.SwitchReplayBytes)
	}

	if hasChildren {
		seen := make(map[string]struct{}, len(s.Children))
		for i, c := range s.Children {
			if c.Name == "" {
				return fmt.Errorf("terminal spec: children[%d].name is required", i)
			}
			if c.Command == "" {
				return fmt.Errorf("terminal spec: children[%d] (%q).command is required", i, c.Name)
			}
			if _, dup := seen[c.Name]; dup {
				return fmt.Errorf("terminal spec: duplicate child name %q", c.Name)
			}
			seen[c.Name] = struct{}{}
		}
	}

	return nil
}

type TerminalStatus struct {
	Pid int `json:"pid"`
	// PidStart is an opaque per-process identifier captured at metadata
	// write time so callers that later signal Pid can reject a recycled
	// PID. Zero means "no token recorded" and consumers should treat the
	// PID as unverifiable. See internal/pidutil.
	PidStart uint64 `json:"pidStart,omitempty"`
	// ChildPgid is the child shell's process-group id, captured at PTY
	// start. The child is spawned with Setsid: true, so the pgid equals
	// the child's PID and lives in a different session from the
	// controller. `sb stop --force` (and the --kill-after escalation)
	// signal -ChildPgid before SIGKILL'ing the controller so the entire
	// child pgroup is reaped even when the shell traps SIGHUP. Zero
	// means "metadata predates this field" — callers must skip the
	// pgroup-kill rather than risk Kill(0, sig) hitting the caller's
	// own process group.
	ChildPgid       int                `json:"childPgid,omitempty"`
	Tty             string             `json:"tty"`
	State           TerminalStatusMode `json:"state"`
	SocketFile      string             `json:"socketCtrl"`
	BaseRunPath     string             `json:"baseRunPath"`
	TerminalRunPath string             `json:"terminalRunPath"`
	LogFile         string             `json:"logFile"`
	LogLevel        string             `json:"logLevel"`
	CaptureFile     string             `json:"captureFile"`
	Attachers       []string           `json:"attachers"`
	LastAttachedAt  time.Time          `json:"lastAttachedAt,omitzero" yaml:"lastAttachedAt,omitempty"`
}

type TerminalState int

const (
	TerminalBash TerminalState = iota
)

type TerminalKind int

const (
	TerminalLocal TerminalKind = iota
	TerminalSSH
)

type TerminalStatusMode int

const (
	// Initializing internal state and server startup.
	Initializing TerminalStatusMode = iota
	// Starting PTY and command execution.
	Starting
	// SettingUp shell environment.
	SettingUp
	// OnInit waiting for shell to be initialized.
	OnInit
	// Ready terminal is fully initialized and ready for use.
	Ready
	// PostAttach waiting for shell to be attached.
	PostAttach
	// Exited terminal has exited.
	Exited
)

func (s TerminalStatusMode) String() string {
	switch s {
	case Initializing:
		return "Initializing"
	case Starting:
		return "Starting"
	case SettingUp:
		return "SettingUp"
	case OnInit:
		return "OnInit"
	case Ready:
		return "Ready"
	case PostAttach:
		return "PostAttach"
	case Exited:
		return "Exited"
	default:
		return "Unknown"
	}
}

// RPC definitions

const TerminalService = "TerminalController"

const (
	TerminalMethodResize    = TerminalService + ".Resize"
	TerminalMethodPing      = TerminalService + ".Ping"
	TerminalMethodAttach    = TerminalService + ".Attach"
	TerminalMethodDetach    = TerminalService + ".Detach"
	TerminalMethodMetadata  = TerminalService + ".Metadata"
	TerminalMethodState     = TerminalService + ".State"
	TerminalMethodStop      = TerminalService + ".Stop"
	TerminalMethodWrite     = TerminalService + ".Write"
	TerminalMethodSubscribe = TerminalService + ".Subscribe"
)

type PingMessage struct {
	Message string
}

type ResizeArgs struct {
	Cols int
	Rows int
}

type StopArgs struct {
	Reason string
}

// WriteRequest carries bytes to push into a terminal's PTY input.
// Bytes are sent verbatim — the CLI layer is responsible for any
// caret notation or hex-escape interpretation.
type WriteRequest struct {
	Data []byte
}

// SubscribeRequest identifies the caller of a Subscribe RPC. ClientID
// is used only for server-side logging and attacher accounting; it does
// not appear in TerminalStatus.Attachers.
type SubscribeRequest struct {
	ClientID ID
}

// ResponseWithFD carries a normal JSON result plus OOB file descriptors.
type ResponseWithFD struct {
	JSON any   // what to JSON-encode into "result"
	FDs  []int // file descriptors to pass via SCM_RIGHTS
}
