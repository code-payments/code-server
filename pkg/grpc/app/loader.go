package app

import (
	"fmt"
	"net/url"
	"sync"

	"github.com/pkg/errors"
)

var (
	ctorMu sync.Mutex
	ctors  = make(map[string]FileLoaderCtor)
)

// RegisterFileLoaderCtor registers a FileLoader for the specified scheme.
func RegisterFileLoaderCtor(scheme string, ctr FileLoaderCtor) {
	ctorMu.Lock()
	defer ctorMu.Unlock()

	_, exists := ctors[scheme]
	if exists {
		panic(fmt.Sprintf("FileLoader already registered for scheme '%s'", scheme))
	}

	ctors[scheme] = ctr
}

// FileLoaderCtor constructs a FileLoader.
type FileLoaderCtor func() (FileLoader, error)

// FileLoader loads files at a specified URL.
type FileLoader interface {
	Load(url *url.URL) ([]byte, error)
}

// LoadFile loads a file at the specified URL using the corresponding
// registered FileLoader. If no scheme is specified, LocalLoader is used.
func LoadFile(fileURL string) ([]byte, error) {
	ctorMu.Lock()
	defer ctorMu.Unlock()

	u, err := url.Parse(fileURL)
	if err != nil {
		return nil, errors.Wrapf(err, "invalid file url %s", fileURL)
	}

	ctr, exists := ctors[u.Scheme]
	if !exists {
		return nil, errors.Errorf("no file loader for %s", u.Scheme)
	}

	l, err := ctr()
	if err != nil {
		return nil, errors.Wrapf(err, "failed get loader for '%s'", fileURL)
	}

	return l.Load(u)
}
