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
	Attach(req *AttachRequest, reply *ResponseWithFD) error
	Metadata() (*TerminalDoc, error)
	State() (*TerminalStatusMode, error)
	Stop(args *StopArgs) error
	Write(req *WriteRequest) error
	Subscribe(req *SubscribeRequest, reply *ResponseWithFD) error
	Screenshot(args *ScreenshotArgs) (*ScreenshotResult, error)
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
	TerminalMethodResize     = TerminalService + ".Resize"
	TerminalMethodPing       = TerminalService + ".Ping"
	TerminalMethodAttach     = TerminalService + ".Attach"
	TerminalMethodDetach     = TerminalService + ".Detach"
	TerminalMethodMetadata   = TerminalService + ".Metadata"
	TerminalMethodState      = TerminalService + ".State"
	TerminalMethodStop       = TerminalService + ".Stop"
	TerminalMethodWrite      = TerminalService + ".Write"
	TerminalMethodSubscribe  = TerminalService + ".Subscribe"
	TerminalMethodScreenshot = TerminalService + ".Screenshot"
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

// AttachRequest is the argument for an Attach RPC. ClientID identifies the
// attaching client (used for attacher accounting and to label the IO fd
// the server hands back). FullCapture opts into replaying the entire raw
// capture buffer on attach — the legacy behavior — instead of the default
// bounded repaint synthesized from the live screen model. It mirrors the
// CLI's --full-capture flag, which plumbs through ClientSpec.FullCapture.
type AttachRequest struct {
	ClientID    ID
	FullCapture bool
}

// ResponseWithFD carries a normal JSON result plus OOB file descriptors.
type ResponseWithFD struct {
	JSON any   // what to JSON-encode into "result"
	FDs  []int // file descriptors to pass via SCM_RIGHTS
}

// ScreenshotArgs is the request for a Screenshot RPC. It carries no
// fields today; it exists so the wire shape can grow (e.g. a format
// selector) without breaking the method signature.
type ScreenshotArgs struct{}

// ScreenshotResult is a decoded snapshot of a terminal's current screen,
// produced by the in-server vt-parser rather than from the raw capture
// bytes. Text is the plain rendered grid; ANSI is the same grid with SGR
// foreground/background color sequences so colors survive the round trip.
type ScreenshotResult struct {
	Cols          int    `json:"cols"`
	Rows          int    `json:"rows"`
	CursorX       int    `json:"cursorX"`
	CursorY       int    `json:"cursorY"`
	CursorVisible bool   `json:"cursorVisible"`
	AltScreen     bool   `json:"altScreen"`
	Text          string `json:"text"`
	ANSI          string `json:"ansi"`
}
