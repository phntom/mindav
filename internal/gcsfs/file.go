package gcsfs

import (
	"context"
	"errors"
	"io"
	"os"
	"path"
	"strings"
	"time"

	"cloud.google.com/go/storage"
	"golang.org/x/net/webdav"
	"google.golang.org/api/iterator"
)

var (
	_ webdav.File = (*readFile)(nil)
	_ webdav.File = (*writeFile)(nil)
	_ webdav.File = (*dirFile)(nil)
)

// readFile streams an object for GET. Seek is supported via ranged reads so
// that net/http's ServeContent (used by the WebDAV handler) can size and range
// the response.
type readFile struct {
	ctx     context.Context
	obj     *storage.ObjectHandle
	size    int64
	modTime time.Time
	name    string
	offset  int64
	r       *storage.Reader
}

func (f *readFile) Read(p []byte) (int, error) {
	if f.offset >= f.size {
		return 0, io.EOF
	}
	if f.r == nil {
		r, err := f.obj.NewRangeReader(f.ctx, f.offset, -1)
		if err != nil {
			return 0, err
		}
		f.r = r
	}
	n, err := f.r.Read(p)
	f.offset += int64(n)
	return n, err
}

func (f *readFile) Seek(offset int64, whence int) (int64, error) {
	var abs int64
	switch whence {
	case io.SeekStart:
		abs = offset
	case io.SeekCurrent:
		abs = f.offset + offset
	case io.SeekEnd:
		abs = f.size + offset
	default:
		return 0, os.ErrInvalid
	}
	if abs < 0 {
		return 0, os.ErrInvalid
	}
	if abs != f.offset {
		if f.r != nil {
			_ = f.r.Close()
			f.r = nil
		}
		f.offset = abs
	}
	return abs, nil
}

func (f *readFile) Close() error {
	if f.r != nil {
		return f.r.Close()
	}
	return nil
}

func (f *readFile) Write([]byte) (int, error)          { return 0, os.ErrPermission }
func (f *readFile) Readdir(int) ([]os.FileInfo, error) { return nil, os.ErrInvalid }
func (f *readFile) Stat() (os.FileInfo, error) {
	return &fileInfo{name: f.name, size: f.size, mode: 0o644, modTime: f.modTime}, nil
}

// writeFile streams a PUT body into a new object, finalized on Close.
type writeFile struct {
	name    string
	w       *storage.Writer
	written int64
}

func (f *writeFile) Write(p []byte) (int, error) {
	n, err := f.w.Write(p)
	f.written += int64(n)
	return n, err
}

func (f *writeFile) Close() error                       { return f.w.Close() }
func (f *writeFile) Read([]byte) (int, error)           { return 0, os.ErrPermission }
func (f *writeFile) Seek(int64, int) (int64, error)     { return 0, os.ErrInvalid }
func (f *writeFile) Readdir(int) ([]os.FileInfo, error) { return nil, os.ErrInvalid }
func (f *writeFile) Stat() (os.FileInfo, error) {
	return &fileInfo{name: path.Base(f.name), size: f.written, mode: 0o644, modTime: time.Now()}, nil
}

// dirFile lists the children of a directory for PROPFIND.
type dirFile struct {
	fsys     *FileSystem
	ctx      context.Context
	key      string
	info     os.FileInfo
	children []os.FileInfo
	pos      int
	loaded   bool
}

func (d *dirFile) load() error {
	prefix := d.key
	if prefix != "" && !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	it := d.fsys.bucket.Objects(d.ctx, &storage.Query{Prefix: prefix, Delimiter: "/"})
	for {
		attrs, err := it.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return err
		}
		if attrs.Prefix != "" {
			d.children = append(d.children, dirInfo(path.Base(strings.TrimSuffix(attrs.Prefix, "/"))))
			continue
		}
		if path.Base(attrs.Name) == keepName {
			continue
		}
		d.children = append(d.children, fileInfoFromAttrs(attrs))
	}
	return nil
}

func (d *dirFile) Readdir(count int) ([]os.FileInfo, error) {
	if !d.loaded {
		if err := d.load(); err != nil {
			return nil, err
		}
		d.loaded = true
	}
	if count <= 0 {
		rest := d.children[d.pos:]
		d.pos = len(d.children)
		return rest, nil
	}
	if d.pos >= len(d.children) {
		return nil, io.EOF
	}
	end := d.pos + count
	if end > len(d.children) {
		end = len(d.children)
	}
	rest := d.children[d.pos:end]
	d.pos = end
	return rest, nil
}

func (d *dirFile) Stat() (os.FileInfo, error)     { return d.info, nil }
func (d *dirFile) Close() error                   { return nil }
func (d *dirFile) Read([]byte) (int, error)       { return 0, os.ErrInvalid }
func (d *dirFile) Write([]byte) (int, error)      { return 0, os.ErrInvalid }
func (d *dirFile) Seek(int64, int) (int64, error) { return 0, os.ErrInvalid }
