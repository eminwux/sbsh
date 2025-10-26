# 🗺️ sbsh Roadmap

## 🚧 In Progress

*(No items currently listed)*

## 📝 Backlog (with Priority)

### 🐞 Bugs
```markdown
```

### 🅰️ Must Have
```markdown
- [ ] **A** Refactor session to terminal
- [ ] **A** sbsh run - | get terminal spec through Stdin
- [ ] **A** If `sbsh run` fails supervisor must print and log error
- [ ] **A** Initial E2E testing
```

### 🅱️ Should Have
```markdown
- [ ] **B** sb stop id/name (terminal/supervisor)
- [ ] **B** sb log id/name (terminal/supervisor) + -f
- [ ] **B** On sbsh's `sbsh run` print log until socket connect
- [ ] **B** sb attach --read-only or -r
- [ ] **B** Sort out architecture for SupervisedSession vs. Session
- [ ] **B** Add a prompt switch: auto | none — Explicit control without hacks.
- [ ] **B** If prompt is not set in a profile, use global default
- [ ] **B** On attach: flag to print all capture; default tail ~100 lines
- [ ] **B** Supervisor Control API (start/stop/list/attach/tail)
- [ ] **B** Optimize waitReady for initTerminal State RPC instead of Metadata
- [ ] **B** Restrict profile to single-terminal with name
```

### 🅲️ Nice to Have
```markdown
- [ ] **C** Save input history in session folder (with large size)
- [ ] **C** Jump out to supervisor with sentinel
- [ ] **C** Message of the Day (Motd)
- [ ] **C** Detach `sbsh run`, attach if `-i` with logs
- [ ] **C** Write to many sessions at the same time
- [ ] **C** Compress capture with xz
- [ ] **C** Terminal replay based on capture (bytes+timestamps+i/o)
```
## 🚫 Limitations
- [ ] **A** sb detach not working on multi-attach (SBSH_SUP_ID)

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
```
