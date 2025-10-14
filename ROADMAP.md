# 🗺️ sbsh Roadmap

## 🚧 In Progress

*(No items currently listed)*

## 📝 Backlog (with Priority)

### 🐞 Bugs
```markdown
- [ ] **A** If `sbsh run` fails supervisor prints to Stderr
- [ ] **A** If supervisor dies the session remains attached
```

### 🅰️ Must Have
```markdown
- [ ] **A** Implement cwd in profiles
- [ ] **A** Implement onInit commands in profile
- [ ] **A** Implement a Ready status after `onInit`
- [ ] **A** Attach enables input after Ready
- [ ] **A** Auto complete (bash)
- [ ] **A** Use default profile if set
- [ ] **A** Implement commandArgs in profiles
- [ ] **A** Compress capture with xz
- [ ] **A** Terminal replay based on capture (bytes+timestamps+i/o)
- [ ] **A** If prompt is not set in profile, use default
```

### 🅱️ Should Have
```markdown
- [ ] **B** Sort out architecture for SupervisedSession vs. Session
- [ ] **B** Remove AttachID and AttachName from SessionSpec
- [ ] **B** Add a prompt switch: auto | none — Explicit control without hacks.
- [ ] **B** On attach: flag to print all capture; default tail ~100 lines
- [ ] **B** Supervisor Control API (start/stop/list/attach/tail)
- [ ] **B** Add profile to sb s l — UX nicety (listing shows profile).
- [ ] **B** sb stop
```

### 🅲️ Nice to Have
```markdown
- [ ] **C** Save input history in session folder (with large size)
- [ ] **C** Sb Command to add env variable to session
- [ ] **C** Jump out to supervisor with sentinel
- [ ] **C** Message of the Day (Motd)
- [ ] **C** Add tty device to sb l
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
```


## 🚀 Release v0.1.0 - 13-Oct-25
```markdown
- [X] **A** Launch sup+sess                                         DONE
- [X] **A** Launch sess + attach                                    DONE
- [X] **A** Detach + reattach                                       DONE
- [X] **A** Session statuses                                        DONE
- [X] **A** Session profiles                                        DONE
```
