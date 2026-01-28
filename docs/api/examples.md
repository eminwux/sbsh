# API Usage Examples

These examples demonstrate how to interact with the `sbsh` controllers using the Go client library.

## 1. Connecting to the Supervisor
To manage sessions, you first need to connect to the Unix socket.

```go
import "[github.com/eminwux/sbsh/pkg/supervisor](https://github.com/eminwux/sbsh/pkg/supervisor)"

// Initialize a client pointing to the supervisor socket
client := supervisor.NewUnix("/tmp/sbsh-supervisor.sock")
defer client.Close()

## 2. Detaching from a Session
If you want to disconnect your client but leave the terminal running in the background:

ctx := context.Background()
err := client.Detach(ctx)
if err != nil {
    log.Fatalf("Failed to detach: %v", err)
}

## 3. Resizing a Terminal
To update the terminal window dimensions:

import "[github.com/eminwux/sbsh/pkg/api](https://github.com/eminwux/sbsh/pkg/api)"

args := api.ResizeArgs{
    Cols: 120,
    Rows: 40,
}

err := terminalClient.Resize(args)