// Package gcsfs implements golang.org/x/net/webdav.FileSystem on top of a
// Google Cloud Storage bucket.
//
// WebDAV paths map 1:1 to object keys (leading slash trimmed), matching the
// layout written by the previous MinIO S3 gateway so existing objects stay
// reachable. Directories are implicit: a name is a directory if any object
// exists under "<name>/". Empty directories are represented by a zero-byte
// ".mindavkeep" marker object.
package gcsfs

import (
	"context"
	"errors"
	"os"
	"path"
	"strings"
	"time"

	"cloud.google.com/go/storage"
	"golang.org/x/net/webdav"
	"google.golang.org/api/iterator"
)

const (
	keepName        = ".mindavkeep"
	keepContentType = "application/mindav-folder-keeper"
)

// FileSystem is a webdav.FileSystem backed by a single GCS bucket.
type FileSystem struct {
	bucket *storage.BucketHandle
}

var _ webdav.FileSystem = (*FileSystem)(nil)

// New returns a FileSystem rooted at the named bucket.
func New(client *storage.Client, bucket string) *FileSystem {
	return &FileSystem{bucket: client.Bucket(bucket)}
}

// toKey converts a WebDAV name to a GCS object key (no leading slash).
func toKey(name string) string {
	return strings.TrimPrefix(path.Clean("/"+name), "/")
}

func (fsys *FileSystem) Mkdir(ctx context.Context, name string, _ os.FileMode) error {
	key := toKey(name)
	if key == "" {
		return os.ErrInvalid
	}
	w := fsys.bucket.Object(key + "/" + keepName).NewWriter(ctx)
	w.ContentType = keepContentType
	return w.Close()
}

func (fsys *FileSystem) OpenFile(ctx context.Context, name string, flag int, _ os.FileMode) (webdav.File, error) {
	key := toKey(name)

	if flag&(os.O_WRONLY|os.O_RDWR) != 0 {
		if key == "" {
			return nil, os.ErrInvalid
		}
		w := fsys.bucket.Object(key).NewWriter(ctx)
		w.ContentType = "application/octet-stream"
		return &writeFile{name: key, w: w}, nil
	}

	if key == "" {
		return &dirFile{fsys: fsys, ctx: ctx, key: "", info: dirInfo("/")}, nil
	}

	attrs, err := fsys.bucket.Object(key).Attrs(ctx)
	if err == nil {
		return &readFile{ctx: ctx, obj: fsys.bucket.Object(key), size: attrs.Size, modTime: attrs.Updated, name: path.Base(key)}, nil
	}
	if !errors.Is(err, storage.ErrObjectNotExist) {
		return nil, err
	}

	isDir, err := fsys.isDir(ctx, key)
	if err != nil {
		return nil, err
	}
	if isDir {
		return &dirFile{fsys: fsys, ctx: ctx, key: key, info: dirInfo(path.Base(key))}, nil
	}
	return nil, os.ErrNotExist
}

func (fsys *FileSystem) RemoveAll(ctx context.Context, name string) error {
	key := toKey(name)
	if key == "" {
		return os.ErrInvalid
	}
	if err := fsys.bucket.Object(key).Delete(ctx); err != nil && !errors.Is(err, storage.ErrObjectNotExist) {
		return err
	}
	return fsys.deletePrefix(ctx, key+"/")
}

func (fsys *FileSystem) Rename(ctx context.Context, oldName, newName string) error {
	oldKey, newKey := toKey(oldName), toKey(newName)
	if oldKey == "" || newKey == "" {
		return os.ErrInvalid
	}

	if _, err := fsys.bucket.Object(oldKey).Attrs(ctx); err == nil {
		if err := fsys.copy(ctx, oldKey, newKey); err != nil {
			return err
		}
		return fsys.bucket.Object(oldKey).Delete(ctx)
	} else if !errors.Is(err, storage.ErrObjectNotExist) {
		return err
	}

	prefix := oldKey + "/"
	it := fsys.bucket.Objects(ctx, &storage.Query{Prefix: prefix})
	moved := false
	for {
		attrs, err := it.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return err
		}
		moved = true
		dst := newKey + "/" + strings.TrimPrefix(attrs.Name, prefix)
		if err := fsys.copy(ctx, attrs.Name, dst); err != nil {
			return err
		}
		if err := fsys.bucket.Object(attrs.Name).Delete(ctx); err != nil {
			return err
		}
	}
	if !moved {
		return os.ErrNotExist
	}
	return nil
}

func (fsys *FileSystem) Stat(ctx context.Context, name string) (os.FileInfo, error) {
	key := toKey(name)
	if key == "" {
		return dirInfo("/"), nil
	}
	attrs, err := fsys.bucket.Object(key).Attrs(ctx)
	if err == nil {
		return fileInfoFromAttrs(attrs), nil
	}
	if !errors.Is(err, storage.ErrObjectNotExist) {
		return nil, err
	}
	isDir, err := fsys.isDir(ctx, key)
	if err != nil {
		return nil, err
	}
	if isDir {
		return dirInfo(path.Base(key)), nil
	}
	return nil, os.ErrNotExist
}

// isDir reports whether any object exists under "<key>/".
func (fsys *FileSystem) isDir(ctx context.Context, key string) (bool, error) {
	it := fsys.bucket.Objects(ctx, &storage.Query{Prefix: key + "/"})
	_, err := it.Next()
	if err == nil {
		return true, nil
	}
	if errors.Is(err, iterator.Done) {
		return false, nil
	}
	return false, err
}

func (fsys *FileSystem) deletePrefix(ctx context.Context, prefix string) error {
	it := fsys.bucket.Objects(ctx, &storage.Query{Prefix: prefix})
	for {
		attrs, err := it.Next()
		if errors.Is(err, iterator.Done) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := fsys.bucket.Object(attrs.Name).Delete(ctx); err != nil && !errors.Is(err, storage.ErrObjectNotExist) {
			return err
		}
	}
}

func (fsys *FileSystem) copy(ctx context.Context, src, dst string) error {
	_, err := fsys.bucket.Object(dst).CopierFrom(fsys.bucket.Object(src)).Run(ctx)
	return err
}

// fileInfo is a static os.FileInfo for objects and synthetic directories.
type fileInfo struct {
	name    string
	size    int64
	mode    os.FileMode
	modTime time.Time
	dir     bool
}

func (fi *fileInfo) Name() string       { return fi.name }
func (fi *fileInfo) Size() int64        { return fi.size }
func (fi *fileInfo) Mode() os.FileMode  { return fi.mode }
func (fi *fileInfo) ModTime() time.Time { return fi.modTime }
func (fi *fileInfo) IsDir() bool        { return fi.dir }
func (fi *fileInfo) Sys() interface{}   { return nil }

func dirInfo(name string) *fileInfo {
	return &fileInfo{name: name, mode: os.ModeDir | 0o755, modTime: time.Now(), dir: true}
}

func fileInfoFromAttrs(a *storage.ObjectAttrs) *fileInfo {
	return &fileInfo{name: path.Base(a.Name), size: a.Size, mode: 0o644, modTime: a.Updated}
}
