// Package storage stores private receipt bytes independently of financial data.
package storage

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"regexp"
	"time"
)

var ErrUnavailable = errors.New("storage provider unavailable")
var keyPattern = regexp.MustCompile(`^receipts/[a-f0-9]{32}$`)

func ValidKey(key string) bool { return keyPattern.MatchString(key) }
func NewKey() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return "receipts/" + hex.EncodeToString(b[:]), nil
}

type Object struct {
	Key        string
	Generation string
	CreatedAt  time.Time
}
type Provider interface {
	Name() string
	Put(context.Context, string, []byte, string) (string, error)
	Open(context.Context, string, string) (io.ReadCloser, error)
	Delete(context.Context, string, string) error
	Expired(context.Context, time.Time) ([]Object, error)
}
type Manager struct {
	current   string
	providers map[string]Provider
}

func NewManager(current string, providers ...Provider) *Manager {
	m := &Manager{current: current, providers: map[string]Provider{}}
	for _, p := range providers {
		if p != nil {
			m.providers[p.Name()] = p
		}
	}
	return m
}
func (m *Manager) Current() (Provider, error) {
	if m == nil {
		return nil, ErrUnavailable
	}
	return m.Get(m.current)
}
func (m *Manager) Get(name string) (Provider, error) {
	if m == nil {
		return nil, ErrUnavailable
	}
	p, ok := m.providers[name]
	if !ok {
		return nil, ErrUnavailable
	}
	return p, nil
}
func (m *Manager) Providers() []Provider {
	var ps []Provider
	if m != nil {
		for _, p := range m.providers {
			ps = append(ps, p)
		}
	}
	return ps
}
func (m *Manager) Close() error {
	var errs []error
	for _, p := range m.Providers() {
		if c, ok := p.(io.Closer); ok {
			errs = append(errs, c.Close())
		}
	}
	return errors.Join(errs...)
}
