package main

import (
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/go-keyring/keyring"
)

const serviceName = "pinentry-store"

type store interface {
	Get(id string) ([]byte, error)
	Put(id string, secret []byte) error
	Delete(id string) error
}

type memStore struct {
	m map[string][]byte
}

func newMemStore() *memStore {
	return &memStore{m: map[string][]byte{}}
}

func (s *memStore) Get(id string) ([]byte, error) {
	v, ok := s.m[id]
	if !ok {
		return nil, os.ErrNotExist
	}
	return append([]byte(nil), v...), nil
}

func (s *memStore) Put(id string, secret []byte) error {
	s.m[id] = append([]byte(nil), secret...)
	return nil
}

func (s *memStore) Delete(id string) error {
	delete(s.m, id)
	return nil
}

type fileStore struct {
	path string
}

func (s *fileStore) Get(id string) ([]byte, error) {
	m, err := s.load()
	if err != nil {
		return nil, err
	}
	v, ok := m[id]
	if !ok {
		return nil, os.ErrNotExist
	}
	return v, nil
}

func (s *fileStore) Put(id string, secret []byte) error {
	m, err := s.load()
	if err != nil {
		return err
	}
	m[id] = append([]byte(nil), secret...)
	return s.save(m)
}

func (s *fileStore) Delete(id string) error {
	m, err := s.load()
	if err != nil {
		return err
	}
	delete(m, id)
	return s.save(m)
}

func (s *fileStore) load() (map[string][]byte, error) {
	b, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string][]byte{}, nil
	}
	if err != nil {
		return nil, err
	}
	m := map[string][]byte{}
	for _, line := range strings.Split(string(b), "\n") {
		if line == "" {
			continue
		}
		idHex, secHex, _ := strings.Cut(line, " ")
		id, err := hex.DecodeString(idHex)
		if err != nil {
			continue
		}
		sec, err := hex.DecodeString(secHex)
		if err != nil {
			continue
		}
		m[string(id)] = sec
	}
	return m, nil
}

func (s *fileStore) save(m map[string][]byte) error {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		b.WriteString(hex.EncodeToString([]byte(k)))
		b.WriteByte(' ')
		b.WriteString(hex.EncodeToString(m[k]))
		b.WriteByte('\n')
	}
	return writePrivate(s.path, []byte(b.String()), 0600)
}

type osStore struct{}

func secretMissing(err error) bool {
	return errors.Is(err, os.ErrNotExist) || errors.Is(err, keyring.ErrNotFound)
}

func (osStore) Get(id string) ([]byte, error) {
	return keyring.Get(serviceName, id)
}

func (osStore) Put(id string, secret []byte) error {
	return keyring.Set(serviceName, id, secret)
}

func (osStore) Delete(id string) error {
	return keyring.Delete(serviceName, id)
}

func openStore(cfg config) store {
	if cfg.Store == "file" {
		return &fileStore{path: secretPath()}
	}
	return osStore{}
}

func writePrivate(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	name := f.Name()
	done := false
	defer func() {
		if !done {
			os.Remove(name)
		}
	}()
	if err := f.Chmod(mode); err != nil {
		f.Close()
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(name, path); err != nil {
		return err
	}
	done = true
	return nil
}
