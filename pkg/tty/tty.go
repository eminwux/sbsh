/*
Copyright © 2025 Emiliano Spinella (eminwux)
*/

package tty

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"

	"github.com/chzyer/readline"
	"github.com/creack/pty"
	"golang.org/x/term"
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

// handleModeSession handles a single interactive session, and returns the next Mode.
func handleModeSession(currentMode Mode, rl *readline.Instance) Mode {
	// Setup SIGINT handler (Ctrl+C)
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT)
	defer signal.Stop(sigChan)

	// This is an infinite loop
	for {
		// Handle BASH session
		if currentMode == ModeBash {
			fmt.Println("Type 'exit' to return to smart mode.")

			customEnv := []string{
				"PATH=/usr/local/bin:/usr/bin:/bin",
				"HOME=" + os.Getenv("HOME"),
			}
			cmd := exec.Command("bash")
			cmd.Env = customEnv

			// Initialize ptmx/pts
			ptmx, err := pty.Start(cmd)
			if err != nil {
				panic(err)
			}
			defer ptmx.Close()

			// Give control of the terminal to the child process group
			// pid := cmd.Process.Pid
			// fd := ptmx.Fd()
			// This sets the foreground process group of the PTY to Bash's PID
			// unix.IoctlSetPointerInt(int(fd), unix.TIOCSPGRP, pid)

			go func() { _, _ = io.Copy(ptmx, os.Stdin) }()
			go func() { _, _ = io.Copy(os.Stdout, ptmx) }()

			ptmx.Write([]byte("echo 'Hello from Go!'\n"))

			cmd.Wait()

			// After Bash/PTYS, switch back to smart mode and signal re-init readline
			return ModeSmart
		}

		// Here starts the main loop to get each line
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
				// llm.Run(trimmed)
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

var rl *readline.Instance

func Start(currentMode Mode) {

	switch currentMode {
	case ModeSmart:
		StartSupervisor()
	case ModeBash:
		StartBash()
	}
}

func initSupervisor() {
	var err error
	prompt := getPrompt(ModeSmart)
	rl, err = readline.NewEx(&readline.Config{
		Prompt:      prompt,
		HistoryFile: "/tmp/sbsh.history",
	})
	if err != nil {
		fmt.Println("Failed to initialize readline:", err)
		os.Exit(1)
	}
}

func workSupervisor() {
	prompt := getPrompt(ModeSmart)
	for {
		need, err := readLine(prompt, rl)
		if err != nil {
			fmt.Println("Bye!")
			os.Exit(0)
		}
		trimmed := strings.TrimSpace(need)
		fmt.Print(trimmed)
	}
}

func StartSupervisor() {

	initSupervisor()
	defer rl.Close()
	workSupervisor()

}

func StartBash() {
	cmd := exec.Command("bash", "-i")
	cmd.Env = append(os.Environ(), "TERM=xterm-256color", "COLORTERM=truecolor")

	// Start the process in a new session so it has its own process group
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setctty: true, // make the child the controlling TTY
		Setsid:  true, // new session
	}

	// Initialize ptmx/pts
	ptmx, err := pty.Start(cmd)
	if err != nil {
		panic(err)
	}
	defer ptmx.Close()

	// Resize PTY on our terminal resize
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGWINCH)
	go func() {
		for range ch {
			_ = pty.InheritSize(os.Stdin, ptmx)
		}
	}()
	ch <- syscall.SIGWINCH // initial size

	// Put sbsh terminal into raw mode so ^C (0x03) is passed through
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		log.Fatalf("MakeRaw: %v", err)
	}
	defer func() { _ = term.Restore(int(os.Stdin.Fd()), oldState) }()

	fmt.Print("(sbsh) bash session started. Ctrl-C goes to bash; Ctrl-D exits.\n\r")

	supervisorCh := make(chan bool, 1)
	errCh := make(chan error, 2)

	// pipe between stdin and pty
	pr, pw := io.Pipe()

	// goroutine 1: always read from os.Stdin into pipe
	go func() {
		_, _ = io.Copy(pw, os.Stdin)
	}()
	go func() {
		buf := make([]byte, 1024)
		toSupervisor := false
		for {
			n, err := pr.Read(buf)
			if err != nil {
				errCh <- err
				return
			}
			select {
			case toSupervisor = <-supervisorCh:
				// toggle supervisor state; if false, drop data
			default:
			}
			if !toSupervisor {
			}
			_, _ = ptmx.Write(buf[:n])
		}
	}()

	go func() {
		buf := make([]byte, 8193)
		var s SentinelFSM
		for {
			n, err := ptmx.Read(buf)
			// fmt.Printf("\n\r*****read %d\n\r", n)
			if n > 0 {
				found, cleaned := s.Feed(buf[:n])
				if found {
					ptmx.Write([]byte(`echo "smart - I'm the supervisor, here to help"` + "\n"))
					// do NOT forward the sentinel to the user
					os.Stdout.Write(cleaned)
					select {
					case supervisorCh <- true: // fire-and-forget
					default: // drop if already signaled
					}
					continue
				}
				os.Stdout.Write(buf[:n])

			}
			if err != nil || n == 0 {
				errCh <- err
				return
			}
		}
	}()

	ptmx.Write([]byte("echo 'Hello from Go!'\n"))
	ptmx.Write([]byte(`export PS1="(sbsh) $PS1"` + "\n"))
	ptmx.Write([]byte(`__sbsh_emit() { printf '\033]1337;sbsh\007'; }` + "\n"))
	ptmx.Write([]byte(`smart()  { __sbsh_emit;  }` + "\n"))

	// Wait until either side closes (e.g., bash exits)
	if err := <-errCh; err != nil && err != io.EOF {
		log.Printf("stream error: %v\r", err)
	}

	_ = cmd.Wait()
}

func switchSupervisor() {

	_, err := readLine("(sbsh-sv)", rl)
	if err != nil {
		fmt.Println("Bye!")
		os.Exit(0)
	}
}

var (
	esc      = byte(0x1b)
	bel      = byte(0x07)
	head     = []byte{esc, ']', '1', '3', '3', '7', ';', 's', 'b', 's', 'h'} // ESC]1337;sbsh
	body     = []byte{']', '1', '3', '3', '7', ';', 's', 'b', 's', 'h'}      // ESC]1337;sbsh
	sentinel = append(append([]byte{esc}, body...), []byte{bel}...)
)

type SentinelFSM struct {
	matchedBytes int
}

func (s *SentinelFSM) Feed(chunk []byte) (found bool, emit []byte) {
	var emitBuf []byte

	for _, b := range chunk {
		if s.matchedBytes > 0 {
			nextByte := sentinel[s.matchedBytes]
			if nextByte == b {
				s.matchedBytes++
			} else {
				emitBuf = append(emitBuf, sentinel[:s.matchedBytes]...)
				emitBuf = append(emitBuf, b)
				s.matchedBytes = 0
			}
		} else {
			if b == sentinel[0] {
				s.matchedBytes++
			} else {
				emitBuf = append(emitBuf, b)
			}
		}
	}

	var result bool
	if s.matchedBytes == len(sentinel) {
		result = true
		s.matchedBytes = 0
	}

	return result, emitBuf

}

func scanForSentinel(stream []byte) (found bool, cleaned []byte) {
	i := 0
	for i <= len(stream)-len(head) {
		if bytes.HasPrefix(stream[i:], head) {
			// Find BEL terminator
			j := i + len(head)
			for j < len(stream) && stream[j] != bel {
				j++
			}

			if stream[j] == bel {
				cleaned = append(cleaned, stream[:i]...)
				return true, cleaned

			}
		}
		i++
	}
	return false, stream
}

func Start2(currentMode Mode) {
	// this is an infinite loop
	var rl *readline.Instance
	defer rl.Close()
	for {
		// Only initialize readline for non-Bash modes
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
		}
		nextMode := handleModeSession(currentMode, rl)
		if rl != nil {
			rl.Close()
		}
		currentMode = nextMode
	}
}
