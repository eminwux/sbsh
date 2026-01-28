# Supervisor API Reference

The `SupervisorController` handles high-level process management and persistence.

## Methods

### `Run`
Starts a new supervised session based on a provided manifest.

* **Request Method**: `SupervisorController.Run`
* **Arguments**: `doc *SupervisorDoc` (The YAML-defined profile)
* **Returns**: `error`

### `Detach`
Signals the supervisor to safely disconnect the current client while keeping the terminal process alive in the background.

* **Request Method**: `SupervisorController.Detach`
* **Returns**: `error`
* **Key Behavior**: This is what allows `sbsh` to be persistent. The shell keeps running even after you detach.