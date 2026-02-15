//go:build filestore

package db

// NewNoopDB creates a no-op database wrapper for filestore mode.
// It returns a *DB with nil sqlx.DB. The TransactionContext method
// will handle the nil case gracefully.
func NewNoopDB() *DB {
	return &DB{
		DB: nil,
	}
}
