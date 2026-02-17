# Chat Protocol Design

## Overview

This document describes the chat system architecture for Soft Serve, using a hybrid Pull + Push model.

## Architecture

### Storage Model

```
data/chat/
├── %23general.jsonl     # Channel messages (#general)
├── %23random.jsonl      # Channel messages (#random)
├── %40alice.jsonl       # User mailbox (@alice)
└── %40bob.jsonl         # User mailbox (@bob)
```

### Key Design Decisions

1. **Channel messages stored in shared files** - Single source of truth
2. **User mailboxes store only state** - Subscriptions and read cursors
3. **No notification storage** - Calculated on connect, pushed in real-time
4. **Append-only (JSONL)** - No deletions, no compression needed

## Message Types

### Channel Messages (`#channel.jsonl`)

```json
{"type":"msg","id":"202602161200000001","ts":"2026-02-16T12:00:00Z","from":"alice","content":"Hello @bob","mentions":["bob"]}
{"type":"msg","id":"202602161200000002","ts":"2026-02-16T12:00:01Z","from":"bob","content":"Hi alice!","mentions":[]}
{"type":"join","ts":"...","user":"carol"}
{"type":"leave","ts":"...","user":"dave"}
{"type":"snapshot","ts":"...","topic":"","members":["alice","bob","carol"]}
```

### User Mailbox Messages (`@user.jsonl`)

```json
{"type":"sub","ts":"2026-02-16T10:00:00Z","inbox":"#general","cursor":"202602160900000000"}
{"type":"mark","ts":"2026-02-16T12:00:00Z","inbox":"#general","cursor":"202602161200000002"}
{"type":"unsub","ts":"2026-02-16T14:00:00Z","inbox":"#random"}
```

## Message ID Format

```
Format: YYYYMMDDHHMMSSMMMM-SSSS
        ├─────────────────┤ ├──┤
        Microsecond timestamp Sequence (4 digits)

Example: 202602161200000001-0001

Properties:
- Time-sortable (lexicographic order = chronological order)
- Globally unique (microsecond timestamp + sequence)
- No coordination needed (single writer per channel)
```

## User State

### State Structure

```go
type UserState struct {
    Subscriptions map[string]*Subscription
}

type Subscription struct {
    SubCursor  string  // When subscribed (messages before = historical)
    ReadCursor string  // Last read message (messages after = unread)
}
```

### State Reconstruction

User state is rebuilt by replaying the user's mailbox:

1. Read all messages from `@user.jsonl`
2. Find last snapshot (if any)
3. Apply events after snapshot in order:
   - `sub`: Add subscription with cursor
   - `mark`: Update read cursor for inbox
   - `unsub`: Remove subscription

## Message Flow

### 1. Sending a Message

```
Alice sends "Hello @bob" to #general

1. Generate message ID: 202602161200000001-0001
2. Write to #general.jsonl:
   {type:"msg", id:"...", from:"alice", content:"Hello @bob", mentions:["bob"]}
3. Push notification to online subscribers (see Push Model)
```

### 2. Subscribing to Channel

```
Bob subscribes to #general

1. Get current latest message ID: 202602161100000000
2. Write to @bob.jsonl:
   {type:"sub", inbox:"#general", cursor:"202602161100000000"}
3. Return to Bob: {subscribed: true, cursor: "...", historical: 100}
4. Bob can optionally pull historical messages (cursor and before)
```

### 3. Reading New Messages

```
Bob reads messages in #general

1. Get subscription state from @bob.jsonl
2. Read cursor = "202602161100000000"
3. Query #general.jsonl for messages after cursor:
   GET #general.jsonl?after=202602161100000000
4. Display messages to Bob
```

### 4. Marking as Read

```
Bob marks messages as read

1. Write to @bob.jsonl:
   {type:"mark", inbox:"#general", cursor:"202602161200000005"}
2. Update in-memory state
```

## Push Model (Online Users)

### Session Management

```go
type Session struct {
    User      string
    Notify    chan Notification
    Channels  map[string]bool  // Subscribed channels
}

type Notification struct {
    Channel string
    MsgID   string
    From    string
    Mention bool  // True if @user in message
}
```

### Push Flow

```
New message arrives in #general

1. For each online session:
   a. Check if session is subscribed to #general
   b. If yes, push notification:
      {channel: "#general", msg_id: "...", from: "alice", mention: true}
2. Client receives notification and decides:
   - Pull full message content
   - Or wait and batch pull
```

### Watch Mode (CLI)

```bash
ssh host chat watch #general

# Client subscribes to notifications
# On notification:
#   1. Pull message content from #general.jsonl
#   2. Display: [12:00] <alice> Hello @bob
```

## Pull Model (Offline Users)

### On Connect

```
Bob connects after being offline

1. Rebuild user state from @bob.jsonl
2. For each subscribed channel:
   a. Get read cursor
   b. Count messages after cursor
   c. Count mentions after cursor
3. Return summary:
   {
     "#general": {"unread": 50, "mentions": 3},
     "#random": {"unread": 10, "mentions": 0}
   }
```

### Pulling Messages

```bash
# Pull all unread in #general
ssh host chat pull #general

# Pull only mentions across all channels
ssh host chat pull --mentions

# Pull historical messages
ssh host chat pull #general --before CURSOR --limit 50
```

## API Commands

### Subscription Commands

```bash
# Subscribe to channel
chat sub #channel
# Returns: {subscribed: true, cursor: "...", historical_count: 100}

# Unsubscribe
chat unsub #channel

# List subscriptions
chat subs
# Returns: {channels: ["#general", "#random"]}
```

### Reading Commands

```bash
# Get unread summary
chat status
# Returns: {"#general": {"unread": 50, "mentions": 3}}

# Pull unread messages
chat pull #channel

# Pull only mentions
chat pull --mentions

# Mark as read (up to latest)
chat mark #channel

# Mark specific cursor as read
chat mark #channel --cursor ID

# Watch for new messages
chat watch #channel
chat watch --all
```

### Sending Commands

```bash
# Send message
chat send #channel "Hello world"

# Send from stdin
echo "Hello" | chat send #channel -
```

## Message ID Comparison

Message IDs are compared lexicographically (string comparison):

```
"202602161200000001" < "202602161200000002"  ✓ Correct
"202602161159599999" < "202602161200000000"  ✓ Correct

# After cursor means ID > cursor
unread = messages where msg.ID > read_cursor
```

## Edge Cases

### User Subscribes Mid-Conversation

```
1. Channel has messages 001-100
2. User subscribes at cursor=100
3. New messages 101-150 arrive
4. User has 50 unread messages (101-150)
5. Messages 001-100 are "historical" - user can pull if desired
```

### User Was Offline for Long Time

```
1. User's read_cursor = 100
2. 1000 new messages arrive (101-1100)
3. User connects:
   - Unread count: 1000
   - No notification accumulation (calculated on connect)
4. User can:
   - Pull all 1000
   - Pull last N
   - Pull only mentions
   - Mark all as read without reading
```

### Mention Tracking

```
Mentions are tracked in channel message:
{..., "mentions": ["bob", "carol"]}

When user pulls:
- Filter messages where username is in mentions array
- No separate mention storage needed
```

## Comparison to Old Model

| Aspect | Old Model | New Model |
|--------|-----------|-----------|
| Notification storage | Per message per user | None (calculated) |
| Storage growth | O(users × messages) | O(messages + user_state) |
| Offline handling | Accumulate notifications | Calculate on connect |
| Read status | Cursor comparison issues | Clear sub/read cursors |
| Mention tracking | Separate notifications | Query from channel |
| Push support | None | Session-based push |
