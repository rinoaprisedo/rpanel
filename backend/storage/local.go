package storage

import (
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

// LocalStorage stores files on the local disk, outside the webroot.
type LocalStorage struct {
	BasePath string
	BaseURL  string
	MaxSize  int64
}

func NewLocalStorage(basePath, baseURL string, maxSize int64) *LocalStorage {
	_ = os.MkdirAll(basePath, 0o755)
	return &LocalStorage{BasePath: basePath, BaseURL: baseURL, MaxSize: maxSize}
}

func (s *LocalStorage) Upload(file multipart.File, header *multipart.FileHeader, folder string) (string, error) {
	if header.Size > s.MaxSize {
		return "", fmt.Errorf("file exceeds max size of %d bytes", s.MaxSize)
	}

	buf := make([]byte, 512)
	n, err := file.Read(buf)
	if err != nil && err != io.EOF {
		return "", err
	}
	contentType := http.DetectContentType(buf[:n])
	if !allowedMimeTypes[contentType] {
		return "", fmt.Errorf("unsupported file type: %s", contentType)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", err
	}

	filename := uuid.New().String() + extForMime[contentType]

	dir := filepath.Join(s.BasePath, folder)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}

	relPath := filepath.Join(folder, filename)
	destPath := filepath.Join(s.BasePath, relPath)

	out, err := os.Create(destPath)
	if err != nil {
		return "", err
	}
	defer out.Close()

	if _, err := io.Copy(out, file); err != nil {
		return "", err
	}

	return filepath.ToSlash(relPath), nil
}

func (s *LocalStorage) Delete(path string) error {
	full := filepath.Join(s.BasePath, path)
	if err := os.Remove(full); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (s *LocalStorage) GetURL(path string) string {
	return strings.TrimRight(s.BaseURL, "/") + "/" + strings.TrimLeft(path, "/")
}

func (s *LocalStorage) Open(rel string) (*Object, error) {
	// Rooted clean so "../" can't climb out of BasePath.
	full := filepath.Join(s.BasePath, filepath.FromSlash(filepath.ToSlash(filepath.Clean("/"+rel))))
	f, err := os.Open(full)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	info, err := f.Stat()
	if err != nil || info.IsDir() {
		f.Close()
		return nil, ErrNotFound
	}
	return &Object{Body: f, Size: info.Size(), ModTime: info.ModTime()}, nil
}
