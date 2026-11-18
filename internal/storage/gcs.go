package storage

import (
	"context"
	"errors"
	"io"
	"strconv"
	"time"

	gcs "cloud.google.com/go/storage"
	"google.golang.org/api/iterator"
)

type GCS struct {
	client *gcs.Client
	bucket string
}

func NewGCS(ctx context.Context, bucket string) (*GCS, error) {
	if bucket == "" {
		return nil, ErrUnavailable
	}
	c, err := gcs.NewClient(ctx)
	if err != nil {
		return nil, err
	}
	return &GCS{client: c, bucket: bucket}, nil
}
func (*GCS) Name() string   { return "gcs" }
func (g *GCS) Close() error { return g.client.Close() }
func (g *GCS) Put(ctx context.Context, key string, b []byte, contentType string) (string, error) {
	if !ValidKey(key) {
		return "", ErrUnavailable
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	w := g.client.Bucket(g.bucket).Object(key).If(gcs.Conditions{DoesNotExist: true}).NewWriter(ctx)
	w.ContentType = contentType
	w.StorageClass = "STANDARD"
	w.ChunkSize = 256 * 1024
	if _, err := w.Write(b); err != nil {
		cancel()
		_ = w.Close()
		return "", err
	}
	if err := w.Close(); err != nil {
		return "", err
	}
	return strconv.FormatInt(w.Attrs().Generation, 10), nil
}
func (g *GCS) object(key, generation string) (*gcs.ObjectHandle, error) {
	if !ValidKey(key) {
		return nil, ErrUnavailable
	}
	o := g.client.Bucket(g.bucket).Object(key)
	if generation != "" {
		n, err := strconv.ParseInt(generation, 10, 64)
		if err != nil || n <= 0 {
			return nil, ErrUnavailable
		}
		o = o.Generation(n)
	}
	return o, nil
}
func (g *GCS) Open(ctx context.Context, key, generation string) (io.ReadCloser, error) {
	o, err := g.object(key, generation)
	if err != nil {
		return nil, err
	}
	return o.NewReader(ctx)
}
func (g *GCS) Delete(ctx context.Context, key, generation string) error {
	o, err := g.object(key, generation)
	if err != nil {
		return err
	}
	err = o.Delete(ctx)
	if errors.Is(err, gcs.ErrObjectNotExist) {
		return nil
	}
	return err
}
func (g *GCS) Expired(ctx context.Context, before time.Time) ([]Object, error) {
	iter := g.client.Bucket(g.bucket).Objects(ctx, &gcs.Query{Prefix: "receipts/"})
	var result []Object
	for {
		attrs, err := iter.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, err
		}
		if ValidKey(attrs.Name) && attrs.Created.Before(before) {
			result = append(result, Object{Key: attrs.Name, Generation: strconv.FormatInt(attrs.Generation, 10), CreatedAt: attrs.Created})
		}
	}
	return result, nil
}
