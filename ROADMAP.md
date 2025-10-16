# 🗺️ sbsh Roadmap

## 🚧 In Progress

*(No items currently listed)*

## 📝 Backlog (with Priority)

### 🐞 Bugs
```markdown
- [ ] **A** If `sbsh run` fails supervisor must print to Stderr
- [ ] **A** If supervisor dies session status remains attached
- [ ] **A** sb detach SBSH_SUP_SOCKET fails on multi-attach
```

### 🅰️ Must Have
```markdown
- [ ] **A** sb attach id/name as positional argument
- [ ] **A** sbsh cmd as positional argument
- [ ] **A** Initial E2E testing
- [ ] **A** Compress capture with xz
- [ ] **A** Use default profile if set
- [ ] **A** Terminal replay based on capture (bytes+timestamps+i/o)
```

### 🅱️ Should Have
```markdown
- [ ] **B** Sort out architecture for SupervisedSession vs. Session
- [ ] **B** Remove AttachID and AttachName from SessionSpec
- [ ] **B** Add a prompt switch: auto | none — Explicit control without hacks.
- [ ] **B** If prompt is not set in a profile, use default
- [ ] **B** On attach: flag to print all capture; default tail ~100 lines
- [ ] **B** Supervisor Control API (start/stop/list/attach/tail)
- [ ] **B** Add profile to sb s l
- [ ] **B** Add tty device to sb l
- [ ] **B** sb stop
- [ ] **B** Optimize waitReady for initTerminal only State
```

### 🅲️ Nice to Have
```markdown
- [ ] **C** Save input history in session folder (with large size)
- [ ] **C** Jump out to supervisor with sentinel
- [ ] **C** Message of the Day (Motd)
- [ ] **C** Detach `sbsh run`, attach if `-i` with logs
- [ ] **C** Write to many sessions at the same time
```

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
- [X] **A** attach doesn't work with `--name`
- [X] **A** Add --verbose to sb to show logging
- [X] **A** Implement cwd in profiles
- [X] **A** Implement onInit commands in profile
- [X] **A** Auto complete (bash) - sbsh -p profile
- [X] **A** Auto complete (bash) - sb attach -n profile
- [X] **A** Implement a Ready status after `onInit`
- [X] **A** Attach enables input after Ready
```


## 🚀 Release v0.1.0 - 13-Oct-25
```markdown
- [X] **A** Launch sup+sess                                         DONE
- [X] **A** Launch sess + attach                                    DONE
- [X] **A** Detach + reattach                                       DONE
- [X] **A** Session statuses                                        DONE
- [X] **A** Session profiles                                        DONE
```
