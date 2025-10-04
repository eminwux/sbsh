package supervisorrunner

import (
	"log"
	"net"
	"os"

	"golang.org/x/term"
)

// toBashUIMode: set terminal to RAW, update flags
func (sr *SupervisorRunnerExec) toBashUIMode() error {
	lastTermState, err := toRawMode()
	if err != nil {
		log.Fatalf("MakeRaw: %v", err)
		return err
	}
	// defer func() { _ = term.Restore(int(os.Stdin.Fd()), oldState) }()

	sr.uiMode = UIBash
	sr.lastTermState = lastTermState
	return nil
}

// toSupervisorUIMode: set terminal to COOKED for your REPL
func (sr *SupervisorRunnerExec) toExitShell() error {
	if sr.lastTermState != nil {
		err := term.Restore(int(os.Stdin.Fd()), sr.lastTermState)
		if err != nil {
			log.Fatalf("MakeRaw: %v", err)
			return err
		}
	}

	sr.uiMode = UIExitShell
	return nil
}

func (sr *SupervisorRunnerExec) initTerminal(conn net.Conn) error {
	if err := writeterminal(conn, "export SBSH_SUP_SOCKET="+sr.supervisorSocketCtrl+"\n"); err != nil {
		return err
	}

	return nil

}

func writeterminal(conn net.Conn, input string) error {
	_, err := conn.Write([]byte(input))

	return err
}

func toRawMode() (*term.State, error) {
	state, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		log.Fatalf("[supervisor] MakeRaw terminal: %v", err)

	}

	return state, nil
}
