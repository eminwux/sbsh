# Terminal API Reference

The `TerminalController` manages the lifecycle and I/O of individual terminal sessions.

## Methods

### `Attach`
Connects a client to a running terminal. This method is unique because it passes raw file descriptors (PTY) to the client.

* **Request Method**: `TerminalController.Attach`
* **Arguments**: `id *ID` (The unique identifier of the terminal)
* **Returns**: `error`
* **Note**: Successful attachment results in the server sending Stdin/Stdout/Stderr file descriptors via Unix Out-of-Band (OOB) data.

### `Resize`
Updates the terminal's window size. Use this whenever the user's terminal window is dragged or resized.

* **Request Method**: `TerminalController.Resize`
* **Arguments**: 
    ```json
    {
      "Cols": 80,
      "Rows": 24
    }
    ```
* **Returns**: `void`

### `State`
Returns the current lifecycle status of the terminal process.

* **Request Method**: `TerminalController.State`
* **Returns**: `TerminalStatusMode` (Enum: `Initializing`, `Ready`, `Exited`, etc.)