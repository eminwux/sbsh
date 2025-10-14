# 🗺️ sbsh Roadmap

## 🚧 In Progress

*(No items currently listed)*

## 📝 Backlog (with Priority)

### 🐞 Bugs
```markdown
- [ ] **A** attach doesn't work with `--name`
- [ ] **A** `sbsh run` fails; the supervisor does not detect it
```

### 🅰️ Must Have
```markdown
- [ ] **A** Implement a Ready status after `onInit`
- [ ] **A** Implement onInit commands in profile
- [ ] **A** Session stop command
- [ ] **A** Bash autocomplete
- [ ] **A** Detach `sbsh run` except if run with `-i`
- [ ] **A** Add flag to sb to show logging
- [ ] **A** no prompt, default prompt
- [ ] **A** Add tty device to sb l

```

### 🅱️ Should Have
```markdown
- [ ] **B** Supervisor can run `sbsh run` on demand through API
- [ ] **B** Control supervisor via API
- [ ] **B** Write to many sessions at the same time
- [ ] **B** Sort out architecture for SupervisedSession vs. Session
- [ ] **B** Remove AttachID and AttachName from SessionSpec
```

### 🅲️ Nice to Have
```markdown
- [ ] **C** Save bash history in session folder (with large size)
- [ ] **C** Command to add env variable through session ctrlSocket
- [ ] **C** Jump out to supervisor with sentinel
- [ ] **C** Message of the Day (Motd)
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
```


## 🚀 Release v0.1.0 - 13-Oct-25
```markdown
- [X] **A** Launch sup+sess                                         DONE
- [X] **A** Launch sess + attach                                    DONE
- [X] **A** Detach + reattach                                       DONE
- [X] **A** Session statuses                                        DONE
- [X] **A** Session profiles                                        DONE
```
