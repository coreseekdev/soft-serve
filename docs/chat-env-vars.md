# Chat Environment Variables

This document describes the environment variables for configuring the chat system in Soft Serve.

## Environment Variables

All chat-related environment variables are prefixed with `SOFT_SERVE_CHAT_`.

### `SOFT_SERVE_CHAT_ENABLED`

- **Description**: Toggles the chat system on/off
- **Type**: Boolean
- **Default**: `true`
- **Example**:
  ```bash
  export SOFT_SERVE_CHAT_ENABLED=true
  ```

### `SOFT_SERVE_CHAT_DATA_PATH`

- **Description**: Path to the directory where chat data will be stored
- **Type**: String (path)
- **Default**: `chat` (relative to data directory)
- **Example**:
  ```bash
  export SOFT_SERVE_CHAT_DATA_PATH=/var/lib/soft-serve/chat
  ```

### `SOFT_SERVE_CHAT_DEFAULT_HISTORY_DAYS`

- **Description**: Default number of days of message history to display
- **Type**: Integer
- **Default**: `7`
- **Example**:
  ```bash
  export SOFT_SERVE_CHAT_DEFAULT_HISTORY_DAYS=30
  ```

### `SOFT_SERVE_CHAT_USERS`

- **Description**: Comma-separated list of allowed chat users. Leave empty to allow all authenticated users.
- **Type**: String (comma-separated list)
- **Default**: `` (empty - all users allowed)
- **Example**:
  ```bash
  export SOFT_SERVE_CHAT_USERS="alice,bob,charlie"
  ```

## Configuration File

Alternatively, you can configure the chat system in the `config.yaml` file:

```yaml
chat:
  enabled: true
  data_path: "chat"
  default_history_days: 7
  users: ""
```

## Complete Example

### Environment Variables

```bash
export SOFT_SERVE_CHAT_ENABLED=true
export SOFT_SERVE_CHAT_DATA_PATH=/opt/soft-serve/data/chat
export SOFT_SERVE_CHAT_DEFAULT_HISTORY_DAYS=14
export SOFT_SERVE_CHAT_USERS="user1,user2"
```

### Configuration File

```yaml
chat:
  enabled: true
  data_path: "/opt/soft-serve/data/chat"
  default_history_days: 14
  users: "user1,user2"
```

## Notes

- Environment variables take precedence over configuration file values
- Relative paths in `data_path` are resolved relative to the Soft Serve data directory
- When `users` is empty, all authenticated SSH users can access the chat system
- Chat data is stored in JSONL format with one message per line
