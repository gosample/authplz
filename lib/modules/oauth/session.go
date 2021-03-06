/*
 * OAuth Module Session Definitions
 * This session object is used internally to transfer user and expiry information to the storage providers
 *
 * AuthPlz Project (https://github.com/ryankurte/AuthPlz)
 * Copyright 2017 Ryan Kurte
 */
package oauth

import (
	"bytes"
	"encoding/gob"
	"time"
)

// Session is an OAuth session for module use
// Relevant data is persisted with each grant type object and returned using a similar object
// meeting the UserSession interface from the datastore
type Session struct {
	UserID          string
	Username        string
	Subject         string
	AccessExpiry    time.Time
	RefreshExpiry   time.Time
	AuthorizeExpiry time.Time
	IDExpiry        time.Time
}

// NewSession creates a new default session instance for a given user
func NewSession(userID, username string) *Session {
	return &Session{
		UserID:          userID,
		Username:        username,
		AccessExpiry:    time.Time{},
		RefreshExpiry:   time.Time{},
		AuthorizeExpiry: time.Time{},
		IDExpiry:        time.Time{},
	}
}

func (s *Session) GetUserID() string              { return s.UserID }
func (s *Session) GetUsername() string            { return s.Username }
func (s *Session) GetSubject() string             { return s.Subject }
func (s *Session) SetAccessExpiry(t time.Time)    { s.AccessExpiry = t }
func (s *Session) GetAccessExpiry() time.Time     { return s.AccessExpiry }
func (s *Session) SetRefreshExpiry(t time.Time)   { s.RefreshExpiry = t }
func (s *Session) GetRefreshExpiry() time.Time    { return s.RefreshExpiry }
func (s *Session) SetAuthorizeExpiry(t time.Time) { s.AuthorizeExpiry = t }
func (s *Session) GetAuthorizeExpiry() time.Time  { return s.AuthorizeExpiry }
func (s *Session) SetIDExpiry(t time.Time)        { s.IDExpiry = t }
func (s *Session) GetIDExpiry() time.Time         { return s.IDExpiry }

func (s *Session) Clone() interface{} {
	clone := Session{}

	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	dec := gob.NewDecoder(&buf)
	_ = enc.Encode(s)
	_ = dec.Decode(&clone)

	return &clone
}
