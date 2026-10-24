package storage

import (
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"path"
	"time"
)

// StorageInterface abstracts file storage so a driver (local, s3, ...) can be
// swapped without touching callers. Only LocalStorage is wired up currently —
// main.go constructs it directly.
type StorageInterface interface {
	Upload(file multipart.File, header *multipart.FileHeader, folder string) (string, error)
	Delete(path string) error
	GetURL(path string) string
	// Open returns ErrNotFound when rel doesn't exist.
	Open(rel string) (*Object, error)
}

// Object is an opened file; Body is seekable so http.ServeContent can answer
// range requests.
type Object struct {
	Body    io.ReadSeekCloser
	Size    int64
	ModTime time.Time
}

var ErrNotFound = errors.New("file not found")

// Serve writes obj as the response (Content-Type from name's extension, range
// and conditional requests handled) and closes it.
func Serve(w http.ResponseWriter, r *http.Request, obj *Object, name string) {
	defer obj.Body.Close()
	http.ServeContent(w, r, path.Base(name), obj.ModTime, obj.Body)
}

var allowedMimeTypes = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
	"image/webp": true,
}
