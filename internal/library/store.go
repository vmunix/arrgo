// Package library provides Store and Tx for database access.
package library

import (
	"database/sql"
	"fmt"
)

// querier abstracts *sql.DB and *sql.Tx for shared query logic.
type querier interface {
	QueryRow(query string, args ...any) *sql.Row
	Query(query string, args ...any) (*sql.Rows, error)
	Exec(query string, args ...any) (sql.Result, error)
}

// Store provides access to content data.
type Store struct {
	db *sql.DB
}

// NewStore creates a new library store.
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// Begin starts a transaction.
func (s *Store) Begin() (*Tx, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	return &Tx{tx: tx}, nil
}

// Tx wraps a database transaction with the same methods as Store.
type Tx struct {
	tx *sql.Tx
}

// Commit commits the transaction.
func (t *Tx) Commit() error {
	return t.tx.Commit()
}

// Rollback aborts the transaction.
func (t *Tx) Rollback() error {
	return t.tx.Rollback()
}
