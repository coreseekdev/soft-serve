package chat

import (
	"container/list"
	"sync"

	"github.com/charmbracelet/soft-serve/pkg/chat/types"
)

// UnreadCache provides LRU caching for unread counts.
type UnreadCache struct {
	mu      sync.RWMutex
	size    int
	cache   map[string]*list.Element
	lru     *list.List
	counts  map[string]int
}

// unreadEntry represents an entry in the unread cache.
type unreadEntry struct {
	key   string
	count int
}

// NewUnreadCache creates a new unread cache.
func NewUnreadCache(size int) *UnreadCache {
	if size <= 0 {
		size = 1000
	}
	return &UnreadCache{
		size:   size,
		cache:  make(map[string]*list.Element),
		lru:    list.New(),
		counts: make(map[string]int),
	}
}

// Get returns the unread count for a key.
func (c *UnreadCache) Get(key string) (int, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if elem, ok := c.cache[key]; ok {
		c.lru.MoveToFront(elem)
		return elem.Value.(*unreadEntry).count, true
	}
	return 0, false
}

// Set sets the unread count for a key.
func (c *UnreadCache) Set(key string, count int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, ok := c.cache[key]; ok {
		c.lru.MoveToFront(elem)
		elem.Value.(*unreadEntry).count = count
		return
	}

	// Add new entry
	entry := &unreadEntry{key: key, count: count}
	elem := c.lru.PushFront(entry)
	c.cache[key] = elem

	// Evict oldest if over capacity
	if c.lru.Len() > c.size {
		oldest := c.lru.Back()
		if oldest != nil {
			c.lru.Remove(oldest)
			delete(c.cache, oldest.Value.(*unreadEntry).key)
		}
	}
}

// Invalidate removes a key from the cache.
func (c *UnreadCache) Invalidate(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, ok := c.cache[key]; ok {
		c.lru.Remove(elem)
		delete(c.cache, key)
	}
}

// Clear clears the cache.
func (c *UnreadCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.cache = make(map[string]*list.Element)
	c.lru = list.New()
}

// Size returns the current cache size.
func (c *UnreadCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lru.Len()
}

// UnreadCounter provides unread message counting.
type UnreadCounter struct {
	cache *UnreadCache
	store types.ChatStore
}

// NewUnreadCounter creates a new unread counter.
func NewUnreadCounter(store types.ChatStore, cacheSize int) *UnreadCounter {
	return &UnreadCounter{
		cache: NewUnreadCache(cacheSize),
		store: store,
	}
}

// CountChannelUnread counts unread messages in a channel for a user.
func (uc *UnreadCounter) CountChannelUnread(channel, cursor string) int {
	// Try cache first
	cacheKey := channel + ":" + cursor
	if count, ok := uc.cache.Get(cacheKey); ok {
		return count
	}

	// Count from store
	opts := types.ReadOptions{
		AfterCursor: cursor,
		Types:       []string{"msg"},
		Limit:       10000, // High limit for counting
	}
	msgs, _ := uc.store.ReadChannelMsgs(channel, opts)
	count := len(msgs)

	// Cache the result
	uc.cache.Set(cacheKey, count)

	return count
}

// CountUserUnread counts unread messages for a user from another user.
func (uc *UnreadCounter) CountUserUnread(user, peer, cursor string) int {
	// Try cache first
	cacheKey := user + ":" + peer + ":" + cursor
	if count, ok := uc.cache.Get(cacheKey); ok {
		return count
	}

	// Count from store
	opts := types.ReadOptions{
		AfterCursor: cursor,
		Types:       []string{"dm"},
		Peer:        peer,
		Limit:       10000,
	}
	msgs, _ := uc.store.ReadUserMsgs(user, opts)
	count := len(msgs)

	// Cache the result
	uc.cache.Set(cacheKey, count)

	return count
}

// CountMentions counts unread mentions for a user.
func (uc *UnreadCounter) CountMentions(user, cursor string) int {
	// Try cache first
	cacheKey := "mentions:" + user + ":" + cursor
	if count, ok := uc.cache.Get(cacheKey); ok {
		return count
	}

	// Count from store
	opts := types.ReadOptions{
		AfterCursor: cursor,
		Types:       []string{"mention"},
		Limit:       10000,
	}
	msgs, _ := uc.store.ReadUserMsgs(user, opts)
	count := len(msgs)

	// Cache the result
	uc.cache.Set(cacheKey, count)

	return count
}

// Invalidate invalidates cache entries for a channel.
func (uc *UnreadCounter) Invalidate(channel string) {
	// We can't easily invalidate all entries for a channel
	// without tracking them, so we just clear the entire cache
	// for simplicity. A more sophisticated implementation
	// could track keys by channel.
	uc.cache.Clear()
}

// InvalidateUser invalidates cache entries for a user.
func (uc *UnreadCounter) InvalidateUser(user string) {
	// Same as above - clear entire cache for simplicity
	uc.cache.Clear()
}
