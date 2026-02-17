# Chat System: Bugs and Security Issues

## Critical Bugs

### Bug #1: Users Cannot See Each Other's Messages

**Severity**: Critical
**Status**: Confirmed

**Description**:
Two different users in the same channel cannot see each other's messages.

**Root Cause**:
There are TWO bugs working together:

1. **Push Manager Subscription Issue** (`pkg/chat/push.go`):
   - When users join a channel via TUI (not `/join` command), the session is NOT subscribed to PushManager
   - The `handleJoin` command subscribes the session, but TUI's `joinChannel` doesn't
   - Without subscription, push notifications are NOT received

2. **Message ID Duplication Between Send and Receive** (`pkg/ui/pages/messages/messages.go`):
   - When sending a message, the TUI uses `chat.GenerateMessageID()` to create an ID
   - This ID is used for BOTH local echo AND channel inbox
   - However, if there's a race condition or timing issue, the same ID appears in both places
   - The TUI's `displayedIDs` map prevents re-displaying the same message
   - If another user sends a message that happens to have colliding timing, it might not show

**Evidence**:

From `pkg/chat/handler.go`, line 175-183:
```go
// Push notification to all online subscribers
notif := Notification{
    Channel: channel,
    MsgID:   msgID,
    From:    user.Name,
    Mention: false,
    Time:    msg.Timestamp,
}
c.pushMgr.PushNotification(channel, notif)
```

This only pushes to `pushMgr.sessions[channel]`. If sessions aren't subscribed, they don't receive notifications.

From `pkg/ui/pages/messages/messages.go`, line 459-497:
```go
func (m *Messages) joinChannel(channel string) {
    // ... join logic ...
    // NOTE: No subscription to pushMgr!
    // Should call: m.chat.pushMgr.Subscribe(channel, sess)
}
```

**Impact**:
- Real-time messaging doesn't work
- Users must rely on polling (500ms interval)
- Polling only works if messages are written to channel inbox correctly
- Combined with subscription issue, messages may never appear

**Fix**:
1. Add TUI session subscription when joining channels
2. Ensure message IDs are properly unique
3. Fix polling logic to handle edge cases

---

## Security Issues

### CVE-2025-XXXX: Missing Access Control

**Severity**: High (7.5/10)
**CVSS**: CVSS:3.1/AV:N/AC:L/PR:L/UI:N/S:U/C:H/I:H/A:H

**Description**:
The chat system does not enforce access control on channels. Any authenticated user can:

1. Join any channel and read all messages
2. Send messages to any channel
3. Read direct messages between other users (via file access)
4. Impersonate other users by modifying their inbox files

**Affected Components**:
- `pkg/chat/handler.go` - `sendChannelMessage()` - No membership check
- `pkg/chat/store/chatfile/store.go` - No file permissions enforcement
- `pkg/chat/commands.go` - `handleJoin()` - No authorization check

**Proof of Concept**:
```bash
# Alice can join #private-channel without authorization
ssh -p 23231 alice@localhost
> /join #private-channel
> Now you can read all messages
```

**Impact**:
- Information disclosure
- Privacy violation
- Potential data leak

**Mitigation**:
Implement channel ACLs:
```go
type ChannelACL struct {
    Owner      string
    Members    []string
    ReadOnly   []string
    Private    bool
}

func (ch *Channel) CanRead(user string) bool {
    if !ch.Private {
        return true
    }
    return ch.HasMember(user) || contains(ch.ReadOnly, user)
}

func (ch *Channel) CanWrite(user string) bool {
    return ch.HasMember(user)
}
```

---

### CVE-2025-XXXX: No Rate Limiting

**Severity**: Medium (6.5/10)
**CVSS**: CVSS:3.1/AV:N/AC:L/PR:L/UI:N/S:U/C:N/I:N/A:H

**Description**:
The chat system has no rate limiting on:
1. Message sending
2. Channel creation
3. Direct messages
4. API calls

**Attack Scenario**:
An attacker can:
1. Send thousands of messages per second
2. Fill up disk space with channel/user inboxes
3. Cause denial of service
4. Crash the server via resource exhaustion

**Affected Code**:
- `pkg/chat/handler.go` - `sendChannelMessage()` - No rate limit
- `pkg/chat/handler.go` - `sendDirectMessage()` - No rate limit

**Mitigation**:
```go
type RateLimiter struct {
    mu     sync.Mutex
    limits map[string]*TokenBucket
}

func (rl *RateLimiter) Allow(user, action string) bool {
    // Check rate limit
    // Return false if exceeded
}
```

---

### CVE-2025-XXXX: Missing Input Validation

**Severity**: Medium (5.3/10)
**CVSS**: CVSS:3.1/AV:N/AC:L/PR:L/UI:N/S:U/C:L/I:N/A:L

**Description**:
The chat system does not properly validate user input:

1. **No message size limit**: Can send unlimited size messages
2. **No sanitization**: HTML/control characters not escaped
3. **Mention spam**: Can @mention unlimited users
4. **Channel name injection**: Channel names not validated

**Affected Code**:
- `pkg/chat/commands.go` - No input length checks
- `pkg/chat/handler.go` - No content sanitization

**Attack Scenario**:
```bash
# Send 10MB message
> /join #test
> <paste 10MB of data>

# Crash other clients
> \x00\x01\x02...  # Control characters
```

**Mitigation**:
```go
const MaxMessageSize = 4096  // 4KB

func validateMessage(content string) error {
    if len(content) > MaxMessageSize {
        return fmt.Errorf("message too large")
    }
    // Sanitize control characters
    // Validate encoding
    return nil
}
```

---

## Design Issues

### Issue #1: No Message Encryption at Rest

**Severity**: Low
**Description**:
All messages are stored in plain text JSONL files. If the server is compromised, all chat history is exposed.

**Recommendation**:
Implement optional encryption:
```go
type EncryptedStore struct {
    cipher AEAD
    key    []byte
}

func (es *EncryptedStore) AppendChannelMsg(ch string, msg *ChannelMessage) error {
    data := json.Marshal(msg)
    encrypted := es.cipher.Seal(data, nonce, key)
    // Write encrypted data
}
```

---

### Issue #2: Weak Message ID Generation

**Severity**: Low
**Description**:
Message IDs use timestamp + sequence, which is predictable and can collide in high-frequency scenarios.

**Current Format**: `YYYYMMDDHHmmss-NNN`

**Issue**:
- Only 1000 IDs per second
- Predictable
- Can collide in distributed scenarios

**Recommendation**:
Use UUIDv7 or similar:
```go
func GenerateMessageID() string {
    // UUIDv7: time-ordered, unique, 128-bit
    return uuidv7.New().String()
}
```

---

### Issue #3: No Message Ordering Guarantees

**Severity**: Low
**Description**:
Messages are appended to files, but there's no guarantee of ordering in concurrent scenarios.

**Example**:
```
Time: User A sends message 1
Time: User B sends message 2
Time: Message 2 written first (slower disk for A)
Time: Message 1 written second

Result: Messages out of order in file
```

**Recommendation**:
Use message ID for ordering, not file position:
```go
msgs, _ := store.ReadChannelMsgs(ch, ReadOptions{})
sort.Slice(msgs, func(i, j int) bool {
    return msgs[i].Timestamp.Before(msgs[j].Timestamp)
})
```

---

### Issue #4: Race Condition in Push

**Severity**: Medium
**Description**:
There's a race between writing to channel inbox and pushing to subscribers.

**Code Flow**:
```go
// 1. Write to channel inbox
store.AppendChannelMsg(channel, msg)

// 2. Push to subscribers
pushMgr.PushNotification(channel, notif)
```

**Race**:
If subscriber polls between 1 and 2, they see the message twice (once in poll, once in push).

**Recommendation**:
Use message ID deduplication in TUI:
```go
if msg.ID != "" && !m.displayedIDs[msg.ID] {
    m.addLine(msg)
    m.displayedIDs[msg.ID] = true
}
```

---

## Performance Issues

### Issue #1: Linear Scan for Message Reads

**Severity**: Low
**Description**:
`ReadChannelMsgs` reads entire file and filters in-memory. O(n) complexity.

**Impact**:
With 100K messages, every read becomes slow.

**Recommendation**:
Implement message indexing:
```go
type MessageIndex struct {
    IDs     map[string]int64  // ID -> file offset
    TimeIndex []TimeEntry      // Sorted by time
}

func (mi *MessageIndex) LookupAfter(ts time.Time) []int64 {
    // Binary search for first message after ts
}
```

---

### Issue #2: No Connection Pooling

**Severity**: Low
**Description**:
Each message read opens/closes file.

**Recommendation**:
Use file handle pool or keep files open.

---

## Refactoring Opportunities

### 1. Separate Concerns

**Current**: `Chat` struct does too many things

**Recommendation**: Split into:
- `ChatManager`: High-level orchestration
- `MessageBus`: Event bus for messages
- `SessionManager`: Session lifecycle
- `StorageManager`: Storage operations

### 2. Use Interface for Storage

**Current**: Direct dependency on `FileStore`

**Recommendation**:
```go
type ChatStore interface {
    AppendChannelMsg(...) error
    ReadChannelMsgs(...) ([]*ChannelMessage, error)
    // ...
}

// Allows: MemoryStore, DBStore, S3Store, etc.
```

### 3. Add Metrics

**Recommendation**:
```go
type Metrics struct {
    MessagesSent      int64
    MessagesReceived  int64
    ActiveSessions    int
    Errors            int64
}
```

---

## Testing Gaps

### Missing Tests

1. **Concurrent access**: Multiple users sending at same time
2. **Message ordering**: Ensure order is preserved
3. **Subscription edge cases**: Join/leave during message send
4. **Recovery**: Corrupted inbox recovery
5. **Performance**: Large message sets

---

## Recommended Fix Priority

1. **Critical**: Fix message visibility bug (Bug #1)
2. **High**: Implement access control (CVE-2025-XXXX)
3. **High**: Add rate limiting (CVE-2025-XXXX)
4. **Medium**: Add input validation (CVE-2025-XXXX)
5. **Medium**: Fix push race condition (Issue #4)
6. **Low**: Improve message ID generation (Issue #2)
7. **Low**: Add encryption at rest (Issue #1)
