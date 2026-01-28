# sbsh API Reference

Welcome to the **Terminal-as-Code** API documentation. 
sbsh uses **JSON-RPC over Unix Domain Sockets** for managing terminal lifecycles.

## Core Controllers
- **[Supervisor API](./supervisor-api.md)**: Manages terminal lifecycle and session persistence.
- **[Terminal API](./terminal-api.md)**: Handles raw PTY I/O, window resizing, and process status.

## Architecture
sbsh separates the *Supervisor* (management) from the *Terminal* (execution), allowing sessions to persist even if the control process restarts.