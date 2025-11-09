# 🗺️ sbsh Roadmap

## 🚧 In Progress

*(No items currently listed)*

## 📝 Backlog (with Priority)

### 🐞 Bugs
```markdown
- [ ] **A** `sbsh terminal` does not expose tty in ps
```

### 🅰️ Must Have
```markdown
- [ ] **A** Lifecycle - Review and correct waitReady in Terminal and Supervisor
- [ ] **A** API - Change SupervisorMetadata and TerminalMetadata to Doc
- [ ] **A** API - Add SupervisorMetadata to SupervisorDoc
- [ ] **A** API - Add TerminalMetadata to TerminalDoc
- [ ] **A** Configuration - Introduce ConfigurationDoc config.yaml
    (fields: runPath, profilesFile, logLevel)
- [ ] **A** Configuration - Introduce profilesDirectory in ConfigurationDoc with conflict resolution
- [ ] **A** Configuration - Introduce default TerminalProfileDoc in ConfigurationDoc
- [ ] **A** Supervisor - Introduce SupervisorDoc and SupervisorProfileDoc
- [ ] **A** Configuration - Add default SupervisorProfileDoc in Configuration
- [ ] **A** Profile - Refactor to honour profile document apiVersion
- [ ] **A** Profile - Add apiVersion migration path for future versions
- [ ] **A** Profile - Add optional terminalName field to TerminalProfileSpec for persistent terminals
- [ ] **A** Terminal - Add setPrompt true/false in TerminalDoc and TerminalProfileDoc
- [ ] **A** Terminal - `sbsh terminal` -f TerminalDoc and TerminalProfileDoc
- [ ] **A** Terminal - Implement restart logic
    (reuse TerminalDoc from Exited terminal, preserve ID/paths/capture)
- [ ] **A** Terminal - Implement profile name resolution
    (if Active → attach, if Exited → restart with same spec/paths, if not found → create new)
- [ ] **A** Supervisor - `sbsh terminal` should detach from terminal and run in background
- [ ] **A** Supervisor - If `sbsh terminal` fails supervisor must print and log error
- [ ] **A** Terminal - Add logMode field to TerminalSpec and TerminalProfileSpec (file/stdout)
- [ ] **A** Terminal - `sbsh terminal` new --log-mode=file/stdout, file by default
- [ ] **A** Supervisor - Add disable-detach flag in SupervisorSpec, nested case
- [ ] **A** Supervisor - Implement disable-detach handling for nested supervisors
- [ ] **A** Terminal - Add captureMode field to TerminalSpec and TerminalProfileSpec (raw/disable)
- [ ] **A** API - Add process spawning API in pkg/spawn
    (NewSupervisor, NewTerminal)
- [ ] **A** API - Add builder helpers in pkg/builder
    (BuildTerminalSpecFromProfile, BuildTerminalSpecFromFile)
- [ ] **A** API - Add lifecycle management (Handle types with WaitReady, Close, etc.)
- [ ] **A** API - Add API documentation and usage examples
```

### 🅱️ Should Have
```markdown
- [ ] **B** Terminal - Add and implement captureMode xz (integrate xz library)
- [ ] **B** sb stop id/name (terminal/supervisor)
- [ ] **B** sb log id/name (terminal/supervisor) + -f
- [ ] **B** sb attach --read-only or -r
- [ ] **B** On sbsh's `sbsh terminal` print log until socket connect
- [ ] **B** If prompt is not set in a profile, use global default
- [ ] **B** Terminal - On attach: flag to print all capture; default tail ~100 lines
- [ ] **B** API - Supervisor Control API (start/stop/list/attach/tail)
- [ ] **B** Auto-prune terminals and supervisors
```

### 🅲️ Nice to Have
```markdown
- [ ] **C** Save input history in session folder (with large size)
- [ ] **C** Message of the Day (Motd)
- [ ] **C** Write to many sessions at the same time
- [ ] **C** Compress capture with xz
- [ ] **C** Terminal replay based on capture (bytes+timestamps+i/o)
- [ ] **C** sb attach auto selects when only one active terminal
```
## 🚫 Limitations

### 🅳️ Won't Have

*(No items currently listed)*

## ✅ Finished
```markdown
- [X] **A** Modify all `%w: %w` to `%w: %v`				            DONE
- [X] **A** Remove `SetCurrentSession` from supervisor			    DONE
- [X] **A** Put some order in `ptysPipes` and `ioClients`	    	DONE
- [X] **A** Divide spec and status in metadata				        DONE
- [X] **A** Set primary super vs. secondary supers			        CANCELLED
- [X] **A** Fix Attach, Detached, Exited statuses			        DONE
- [X] **A** Change `closeCh`, replace with `context.Cancel`	        DONE
- [X] **A** Dynamic prompt based on profile			            	DONE
- [X] **A** `sbsh run` supervisor with profiles			            DONE
- [X] **A** Add env from profile in sbsh				            DONE
- [X] **B** Add purge to delete old sessions			        	DONE
- [X] **C** Hide Exited sessions, add `-a` to show them all	    	DONE
- [X] **A** BUG: Close all channels				                	DONE
- [X] **A** On supervisor reattach, the prompt is generated again   DONE
- [X] **A** On CTRL+C to session run, the status is Detached        DONE
- [X] **A** `sbsh run` logs → `~/.sbsh/run/session/1f/sbsh.log`     DONE
- [X] **A** `sbsh logs` → `~/.sbsh/run/supervisor/1f/sbsh.log`      DONE
- [X] **B** Solve `\r\n` everywhere                                 DONE
- [X] **B** Print event logs in `sbsh run`, attach/detach, etc.     DONE
- [X] **B** Add metadata for supervisor                             DONE
- [X] **C** Correct internal vs. pkg in Go code                     DONE
- [X] **A** attach doesn't work with `--name`                       DONE
- [X] **A** Add --verbose to sb to show logging                     DONE
- [X] **A** Implement cwd in profiles                               DONE
- [X] **A** Implement onInit commands in profile                    DONE
- [X] **A** Auto complete (bash) - sbsh -p profile                  DONE
- [X] **A** Auto complete (bash) - sb attach -n profile             DONE
- [X] **A** Implement a Ready status after `onInit`                 DONE
- [X] **A** Attach enables input after Ready                        DONE
- [X] **A** Remove hardcoded quotes in export PS1                   DONE
- [X] **A** sb attach --name autocomplete hide Exited sessions      DONE
- [X] **A** Add boolean to ProfileSpec to include current Env Vars  DONE
- [X] **B** Add tty device to sb l                                  DONE
- [X] **A** Add profile to sb s l                                   DONE
- [X] **A** sbsh use default profile if set in profiles.yaml        DONE
- [ ] **A** If supervisor dies session status remains attached      CANCELLED
- [X] **A** sb get terminals/supervisors/profiles                   DONE
- [X] **A** sb get terminal/supervisor id/name                      DONE
- [X] **A** sb get profile name                                     DONE
- [X] **A** sb attach id/name as positional argument                DONE
- [X] **A** sb detach  id/name as positional argument               DONE
- [X] **B** Remove AttachID and AttachName from SessionSpec         DONE
- [X] **B** sbsh/sb disallow illegal positional arguments           DONE
- [X] **A** sbsh run - | get terminal spec through Stdin            DONE
- [X] **B** Sort out architecture for SupervisedSession vs. Session DONE
- [X] **A** Initial E2E testing                                     DONE
- [X] **A** Add --version in sb sbsh                                DONE
- [X] **A** Refactor session to terminal                            DONE
- [X] **A** `sbsh` does not run as shell terminal                   DONE
- [X] **A** `sbsh run` change to `sbsh terminal`                    DONE
- [X] **B** Optimize waitReady State RPC instead of Metadata        DONE
```
