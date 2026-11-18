package storage

import (
	"context"
	"errors"
	"io"
	"os"
	"time"
)

// os.Root confines all operations, including symlink traversal, to the private root.
type Local struct{ root *os.Root }

func NewLocal(path string) (*Local, error) {
	if path == "" {
		return nil, ErrUnavailable
	}
	if err := os.MkdirAll(path, 0700); err != nil {
		return nil, err
	}
	r, err := os.OpenRoot(path)
	if err != nil {
		return nil, err
	}
	if err := r.Chmod(".", 0700); err != nil {
		r.Close()
		return nil, err
	}
	if err := r.Mkdir("receipts", 0700); err != nil && !errors.Is(err, os.ErrExist) {
		r.Close()
		return nil, err
	}
	return &Local{root: r}, nil
}
func (*Local) Name() string   { return "local" }
func (l *Local) Close() error { return l.root.Close() }
func (l *Local) Put(ctx context.Context, key string, b []byte, _ string) (string, error) {
	if !ValidKey(key) {
		return "", ErrUnavailable
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	f, err := l.root.OpenFile(key, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return "", err
	}
	_, err = f.Write(b)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err == nil {
		err = ctx.Err()
	}
	if err != nil {
		_ = l.root.Remove(key)
	}
	return "", err
}
func (l *Local) Open(ctx context.Context, key, _ string) (io.ReadCloser, error) {
	if !ValidKey(key) {
		return nil, ErrUnavailable
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return l.root.Open(key)
}
func (l *Local) Delete(ctx context.Context, key, _ string) error {
	if !ValidKey(key) {
		return ErrUnavailable
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	err := l.root.Remove(key)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
func (l *Local) Expired(ctx context.Context, before time.Time) ([]Object, error) {
	f, err := l.root.Open("receipts")
	if err != nil {
		return nil, err
	}
	defer f.Close()
	entries, err := f.ReadDir(-1)
	if err != nil {
		return nil, err
	}
	var result []Object
	for _, e := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		key := "receipts/" + e.Name()
		if !ValidKey(key) || !e.Type().IsRegular() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			return nil, err
		}
		if info.ModTime().Before(before) {
			result = append(result, Object{Key: key, CreatedAt: info.ModTime()})
		}
	}
	return result, nil
}
