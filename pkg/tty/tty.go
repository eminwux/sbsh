/*
Copyright © 2025 Emiliano Spinella (eminwux)
*/

package tty

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"sbsh/pkg/llm"
	"strings"
	"syscall"

	"github.com/chzyer/readline"
	"github.com/creack/pty"
	"golang.org/x/sys/unix"
)

type Mode int

const (
	ModeSmart Mode = iota
	ModeConfig
	ModeBash
)

func getPrompt(currentMode Mode) string {
	switch currentMode {
	case ModeSmart:
		return "sbsh# "
	case ModeConfig:
		return "sbsh-config# "
	case ModeBash:
		return "sbsh-bash$ "
	}
	return "sbsh# "
}

// ReadLine provides advanced prompt with line editing and history
func readLine(prompt string, rl *readline.Instance) (string, error) {
	rl.SetPrompt(prompt)
	line, err := rl.Readline()
	if err == readline.ErrInterrupt {
		// Handle Ctrl+C while at empty prompt (just show prompt again)
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return line, nil
}

// showPrompt handles a single interactive session, and returns the next Mode.
func showPrompt(currentMode Mode, rl *readline.Instance) Mode {
	// Setup SIGINT handler (Ctrl+C)
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT)
	defer signal.Stop(sigChan)

	for {
		// Only use readline for smart/config modes!
		if currentMode == ModeBash {
			fmt.Println("Type 'exit' to return to smart mode.")

			customEnv := []string{
				"PATH=/usr/local/bin:/usr/bin:/bin",
				"HOME=" + os.Getenv("HOME"),
			}
			cmd := exec.Command("bash", "-i")
			cmd.Env = customEnv

			ptmx, err := pty.Start(cmd)
			if err != nil {
				panic(err)
			}
			defer ptmx.Close()

			// Give control of the terminal to the child process group
			pid := cmd.Process.Pid
			fd := ptmx.Fd()
			// This sets the foreground process group of the PTY to Bash's PID
			unix.IoctlSetPointerInt(int(fd), unix.TIOCSPGRP, pid)

			go func() { _, _ = io.Copy(ptmx, os.Stdin) }()
			go func() { _, _ = io.Copy(os.Stdout, ptmx) }()
			ptmx.Write([]byte("echo 'Hello from Go!'\n"))

			cmd.Wait()

			// After Bash/PTYS, switch back to smart mode and signal re-init readline
			return ModeSmart
		}

		prompt := getPrompt(currentMode)
		need, err := readLine(prompt, rl)
		if err != nil {
			fmt.Println("Bye!")
			os.Exit(0)
		}
		trimmed := strings.TrimSpace(need)

		switch currentMode {
		case ModeSmart:
			switch trimmed {
			case "config":
				return ModeConfig
			case "bash":
				rl.Close() // <--- close readline before Bash
				return ModeBash
			}
			if trimmed == "exit" || trimmed == "quit" {
				fmt.Println("Bye!")
				os.Exit(0)
			}
			done := make(chan struct{})
			go func() {
				llm.Run(trimmed)
				close(done)
			}()
			select {
			case <-done:
			case <-sigChan:
				// handle Ctrl+C
			}
		case ModeConfig:
			switch trimmed {
			case "smart":
				return ModeSmart
			case "bash":
				if rl != nil {
					rl.Close()
				}
				return ModeBash
			case "show":
				// showContext()
				continue
			}
			if strings.HasPrefix(trimmed, "set ") {
				// set context logic here
				continue
			}
		}
	}
}

func Start(currentMode Mode) {
	for {
		// Only initialize readline for non-Bash modes
		var rl *readline.Instance
		if currentMode != ModeBash {
			var err error
			rl, err = readline.NewEx(&readline.Config{
				Prompt:      getPrompt(currentMode),
				HistoryFile: "/tmp/sbsh.history",
			})
			if err != nil {
				fmt.Println("Failed to initialize readline:", err)
				os.Exit(1)
			}
			defer rl.Close()
		}
		nextMode := showPrompt(currentMode, rl)
		if rl != nil {
			rl.Close()
		}
		currentMode = nextMode
	}
}
