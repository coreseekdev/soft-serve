package chat

import (
	"sort"
	"strings"
)

// UserList provides user list functionality.
type UserList interface {
	Contains(name string) bool
	List() []string
}

// EnvUserList reads user list from environment variable.
// Format: SOFT_SERVE_CHAT_USERS=alice,bob,carol
type EnvUserList struct {
	users map[string]bool
}

// NewEnvUserList creates a new user list from environment variable value.
func NewEnvUserList(env string) *EnvUserList {
	users := make(map[string]bool)
	for _, name := range strings.Split(env, ",") {
		name = strings.TrimSpace(name)
		if name != "" {
			users[strings.ToLower(name)] = true
		}
	}
	return &EnvUserList{users: users}
}

// Contains checks if a user exists in the list.
func (l *EnvUserList) Contains(name string) bool {
	return l.users[strings.ToLower(name)]
}

// List returns all users in the list.
func (l *EnvUserList) List() []string {
	names := make([]string, 0, len(l.users))
	for name := range l.users {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// MapUserList is a simple map-based user list.
type MapUserList struct {
	users map[string]bool
}

// NewMapUserList creates a new map-based user list.
func NewMapUserList(users []string) *MapUserList {
	m := make(map[string]bool)
	for _, name := range users {
		name = strings.TrimSpace(name)
		if name != "" {
			m[strings.ToLower(name)] = true
		}
	}
	return &MapUserList{users: m}
}

// Contains checks if a user exists in the list.
func (l *MapUserList) Contains(name string) bool {
	return l.users[strings.ToLower(name)]
}

// List returns all users in the list.
func (l *MapUserList) List() []string {
	names := make([]string, 0, len(l.users))
	for name := range l.users {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Add adds a user to the list.
func (l *MapUserList) Add(name string) {
	l.users[strings.ToLower(name)] = true
}

// Remove removes a user from the list.
func (l *MapUserList) Remove(name string) {
	delete(l.users, strings.ToLower(name))
}

// CombinedUserList combines multiple user lists.
type CombinedUserList struct {
	lists []UserList
}

// NewCombinedUserList creates a new combined user list.
func NewCombinedUserList(lists ...UserList) *CombinedUserList {
	return &CombinedUserList{lists: lists}
}

// Contains checks if a user exists in any of the lists.
func (l *CombinedUserList) Contains(name string) bool {
	for _, list := range l.lists {
		if list.Contains(name) {
			return true
		}
	}
	return false
}

// List returns all unique users from all lists.
func (l *CombinedUserList) List() []string {
	seen := make(map[string]bool)
	var names []string
	for _, list := range l.lists {
		for _, name := range list.List() {
			if !seen[name] {
				seen[name] = true
				names = append(names, name)
			}
		}
	}
	sort.Strings(names)
	return names
}
