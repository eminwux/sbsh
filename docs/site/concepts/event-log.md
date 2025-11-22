# Event Log

sbsh captures complete I/O for every terminal, providing a full audit trail and enabling debugging and replay capabilities.

## Log Storage

Terminal logs are stored in `~/.sbsh/run/terminals/<id>/capture.log`, containing:

- All terminal input
- All terminal output
- Timestamps (planned)
- I/O direction markers

## Log Structure

The capture log contains raw terminal I/O:

```
[timestamp] > input data
[timestamp] < output data
```

This provides a complete record of all terminal activity.

## Use Cases

### Debugging

Review terminal logs to understand what happened:

```bash
# View terminal log
cat ~/.sbsh/run/terminals/my-terminal/capture.log
```

### Audit Trail

Complete I/O capture provides an audit trail for:

- Security auditing
- Compliance requirements
- Incident investigation
- Process documentation

### CI/CD Integration

Terminal logs are captured in CI/CD pipelines:

```yaml
# GitHub Actions
- name: Upload terminal logs
  if: failure()
  uses: actions/upload-artifact@v3
  with:
    name: terminal-logs
    path: ~/.sbsh/run/terminals/*
```

See the [CI/CD Guide](../guides/cicd.md) for details.

## Log Management

### Log Size

Terminal logs can grow large over time. Consider:

- Regular cleanup of old terminals
- Log rotation (planned)
- Compression (planned)

### Log Retention

Terminal logs persist until the terminal is pruned:

```bash
# Prune old terminals
sb prune --older-than 30d
```

## Metadata

Terminal metadata is stored alongside logs in `~/.sbsh/run/terminals/<id>/meta.json`:

- Terminal state
- Environment variables
- Profile information
- Lifecycle events
- Attacher history

## Future Enhancements

Planned features:

- Timestamp precision
- Log compression (xz)
- Terminal replay based on capture
- Structured log format

See the [Roadmap](../roadmap.md) for details.

## Related Concepts

- [Terminals](terminals.md) - Terminal lifecycle
- [Terminal State](terminal-state.md) - State management
- [CI/CD Guide](../guides/cicd.md) - CI/CD integration
