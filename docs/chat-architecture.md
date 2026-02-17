# Chat System Architecture

## Overview

The Soft Serve chat system is an IRC-inspired chat implementation with persistent storage and real-time messaging capabilities. It uses a novel "three-copy" message delivery model to ensure reliable message delivery and history.

## Design Philosophy

### Core Principles

1. **Message Immutability**: Once written, messages cannot be deleted or edited
2. **Event Sourcing**: All state changes are stored as immutable events
3. **Snapshot/Replay**: System state is rebuilt by replaying events from snapshots
4. **Pull + Push**: New messages are both pushed to online users and available for pull

### Message ID Generation

Messages use globally unique IDs in the format: `YYYYMMDDHHmmss-NNN`

- Time-based prefix ensures ordering
- Sequence number handles high-frequency messages
- Example: `20250117153045-001`

## Architecture Diagram

```
┌─────────────────────────────────────────────────────────────────┐
│                          Chat Core                              │
│  ┌──────────────┐  ┌───────────────┐  ┌────────────────────┐  │
│  │   Chat       │  │ PushManager   │  │   ChatStore        │  │
│  │   (Main)     │  │ (Real-time)   │  │   (Storage)        │  │
│  └──────────────┘  └───────────────┘  └────────────────────┘  │
└─────────────────────────────────────────────────────────────────┘
         │                    │                       │
         │                    │                       │
    ┌────▼────┐         ┌────▼────┐           ┌─────▼──────┐
    │ Sessions │         │ Notify  │           │ FileStore  │
    └────┬────┘         └────┬────┘           └─────┬──────┘
         │                   │                       │
         │                   │                       │
    ┌────▼────┐         ┌────▼────┐           ┌─────▼──────┐
    │   SSH   │         │ Channel │           │  JSONL     │
    │  TUI    │         │ Chans   │           │  Files     │
    └─────────┘         └─────────┘           └────────────┘
```

## Message Flow

### Three-Copy Delivery Model

When a user sends a message to a channel:

```
User sends message to #general
        │
        ▼
┌───────────────────────────────────────┐
│ 1. Generate unique message ID        │
│    (e.g., 20250117153045-001)        │
└───────────────────────────────────────┘
        │
        ▼
┌───────────────────────────────────────┐
│ 2. Write to channel inbox             │
│    File: chat/%23general.jsonl        │
│    - Visible to all channel members   │
│    - Persistent history               │
└───────────────────────────────────────┘
        │
        ▼
┌───────────────────────────────────────┐
│ 3. Write to sender's user inbox       │
│    File: chat/@alice.jsonl            │
│    - For sender's message history     │
│    - Direction: "out"                 │
└───────────────────────────────────────┘
        │
        ▼
┌───────────────────────────────────────┐
│ 4. Check for @mentions                │
│    - Write mention to mentioned users │
│    - Type: "mention"                  │
└───────────────────────────────────────┘
        │
        ▼
┌───────────────────────────────────────┐
│ 5. Push to online subscribers         │
│    - Real-time notification           │
│    - Non-blocking                     │
└───────────────────────────────────────┘
```

### Direct Message Flow

```
Alice sends DM to Bob
        │
        ▼
┌───────────────────────────────────────┐
│ 1. Write to Alice's inbox             │
│    File: chat/@alice.jsonl            │
│    - Type: "dm"                       │
│    - Direction: "out"                 │
│    - Peer: "@bob"                     │
└───────────────────────────────────────┘
        │
        ▼
┌───────────────────────────────────────┐
│ 2. Write to Bob's inbox               │
│    File: chat/@bob.jsonl              │
│    - Type: "dm"                       │
│    - Direction: "in"                  │
│    - From: "alice"                    │
│    - Peer: "@alice"                   │
└───────────────────────────────────────┘
        │
        ▼
┌───────────────────────────────────────┐
│ 3. Push to Bob if online              │
│    - Via session notification         │
└───────────────────────────────────────┘
```

## Data Structures

### Message Types

#### Channel Messages (`ChannelMessage`)

```go
type ChannelMessage struct {
    Type      ChannelMessageType // msg, join, leave, topic, snapshot
    ID        string             // Unique message ID
    Timestamp time.Time

    // For "msg" type:
    From    string   // Sender username
    Content string   // Message content
    Mentions []string // Mentioned users

    // For "join/leave/topic" types:
    User  string // Username
    Topic string // New topic

    // For snapshot type:
    Members []string
}
```

#### User Messages (`UserMessage`)

```go
type UserMessage struct {
    Type      UserMessageType // dm, channel, mention, sub, unsub, mark, snapshot
    ID        string
    Timestamp time.Time

    // For "dm/channel" types:
    Direction string // "in" or "out"
    Peer      string // @user or #channel
    From      string // Sender (for received messages)
    Content   string

    // For "sub/mark" types:
    Inbox  string // #channel or @user
    Cursor string // Message ID

    // For "snapshot" type:
    State *UserState
}
```

#### User State

```go
type UserState struct {
    Subscriptions map[string]*Subscription  // New format
    Cursors       map[string]string         // Legacy format
}

type Subscription struct {
    SubCursor  string // Cursor when subscribed
    ReadCursor string // Last read message ID
}
```

## Storage Layout

### Directory Structure

```
{data_path}/chat/
├── %23general.jsonl        # #general channel inbox
├── %23random.jsonl         # #random channel inbox
├── %40alice.jsonl          # @alice user inbox
├── %40bob.jsonl            # @bob user inbox
└── ...
```

### File Format (JSONL)

Each file is a JSONL file (one JSON object per line):

```jsonl
{"type":"snapshot","ts":"2025-01-17T15:30:00Z","topic":"","members":[]}
{"type":"msg","id":"20250117153045-001","ts":"2025-01-17T15:30:45Z","from":"alice","content":"Hello!"}
{"type":"msg","id":"20250117153046-002","ts":"2025-01-17T15:30:46Z","from":"bob","content":"Hi Alice!"}
```

## Components

### 1. Chat Core (`chat.go`)

Main orchestrator that:
- Manages channels and users
- Generates message IDs
- Coordinates storage and push
- Handles lifecycle (init/shutdown)

### 2. Storage Layer (`store/chatfile/`)

**FileStore** implements persistent storage:
- `AppendChannelMsg`: Write to channel inbox
- `AppendUserMsg`: Write to user inbox
- `ReadChannelMsgs`: Read channel history
- `ReadUserMsgs`: Read user mailbox
- `RebuildChannelState`: Rebuild channel from events
- `RebuildUserState`: Rebuild user state from events

**Key Features**:
- Lock-based concurrency control
- State rebuilding from snapshots
- Event replay for consistency

### 3. Push Manager (`push.go`)

Manages real-time message delivery:
- `Subscribe`: Register session for channel notifications
- `PushNotification`: Broadcast to subscribers
- `Unsubscribe`: Remove subscription

**Push Strategy**:
- Non-blocking (drops if channel full)
- Goroutine-per-subscriber for parallel delivery
- Retry mechanism for reliability

### 4. Session Handler (`handler.go`)

Handles SSH session lifecycle:
- User authentication
- Session creation/cleanup
- Message input loop
- Command routing

### 5. Commands (`commands.go`)

IRC-style commands:
- `/join #channel` - Join a channel
- `/leave #channel` - Leave a channel
- `/select #channel|@user` - Switch target
- `/dm @user message` - Send DM
- `/mark #channel|@user` - Mark as read
- `/history` - Show message history
- `/mentions` - Show mentions
- `/topic` - View/set topic
- `/who` - List channel members
- `/help` - Show help

### 6. TUI (`pkg/ui/pages/messages/`)

Terminal UI for chat:
- Message display with timestamps
- Input field with history
- Channel tabs
- Real-time polling (500ms interval)

## Message Polling vs Push

### Pull (Polling)

- **TUI polls channel inbox every 500ms**
- Reads recent messages
- Filters already-displayed messages
- Filters sender's own messages (local echo)

### Push (Real-time)

- **PushManager broadcasts to online subscribers**
- Uses notification channels
- Non-blocking (drops if full)
- For immediate delivery

### Hybrid Approach

The system uses both:
1. **Push**: For online users (immediate notification)
2. **Pull**: For catching up on history and polling

## Concurrency Model

### Locking

File-based locking with `LockManager`:
- Per-inbox locks (channel or user)
- RLock for reads, Lock for writes
- Prevents concurrent writes to same file

### Session Management

- Global session map: `map[string]*ChatSession`
- User-to-session mapping
- Channel-to-subscribers mapping

## State Rebuilding

### Channel State

1. Find latest `snapshot` message
2. Apply events after snapshot:
   - `join`: Add member
   - `leave`: Remove member
   - `topic`: Update topic

### User State

1. Find latest `snapshot` message
2. Apply events after snapshot:
   - `sub`: Add subscription
   - `unsub`: Remove subscription
   - `mark`: Update read cursor

## Limitations

### Design Constraints

- **No message deletion/editing**: By design
- **No read receipts**: Read marks are user-local
- **No typing indicators**: Not implemented
- **No message threading**: Optional field exists but unused

### Known Issues

1. **Race condition**: Between message write and push
2. **No access control**: Any authenticated user can join any channel
3. **No rate limiting**: Vulnerable to spam
4. **No message size limits**: Vulnerable to DoS
5. **TUI filtering bug**: Users may not see each other's messages

## Security Considerations

### Current Security Model

- **Authentication**: Via SSH public keys
- **Authorization**: No per-channel authorization
- **Input validation**: Basic only
- **Rate limiting**: None

### Recommended Improvements

1. **Channel ACLs**: Access control lists
2. **Rate limiting**: Per-user and per-channel
3. **Message size limits**: Maximum message length
4. **Input sanitization**: Prevent injection attacks
5. **Audit logging**: Track all messages and actions

## Performance

### Scalability

- **File I/O**: O(n) for reading messages (linear scan)
- **Lock contention**: Per-inbox, low contention expected
- **Memory**: Channel state in memory, user state in memory
- **Push**: O(n) subscribers per channel

### Optimization Opportunities

1. **Message indexing**: For faster range queries
2. **Batch writes**: Reduce syscalls
3. **Connection pooling**: For concurrent access
4. **Caching**: Frequently accessed messages
