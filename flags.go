package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
)

// savedEntry is one saved SETKEYINFO. The file holds no secret.
type savedEntry struct {
	ID       string `json:"id"`
	NoPrompt bool   `json:"no_prompt"`
}

type catalog struct {
	path string
}

// Has reports whether id is marked don't-ask-next-time.
// A missing file is an empty catalog. A read error is not marked.
func (c *catalog) Has(id string) bool {
	e, ok := c.find(id)
	return ok && e.NoPrompt
}

// Known reports whether id has a saved-entry line.
func (c *catalog) Known(id string) bool {
	_, ok := c.find(id)
	return ok
}

func (c *catalog) find(id string) (savedEntry, bool) {
	entries, err := c.load()
	if err != nil {
		return savedEntry{}, false
	}
	for _, e := range entries {
		if e.ID == id {
			return e, true
		}
	}
	return savedEntry{}, false
}

// Mark sets don't-ask-next-time. create adds a missing line.
// A missing line with create false is left missing.
func (c *catalog) Mark(id string, noPrompt, create bool) error {
	entries, err := c.load()
	if err != nil {
		return err
	}
	for i := range entries {
		if entries[i].ID != id {
			continue
		}
		if entries[i].NoPrompt == noPrompt {
			return nil
		}
		entries[i].NoPrompt = noPrompt
		return c.save(entries)
	}
	if !create {
		return nil
	}
	entries = append(entries, savedEntry{ID: id, NoPrompt: noPrompt})
	return c.save(entries)
}

// Ensure records id as still prompting when it has no line.
// An existing line is left unchanged.
func (c *catalog) Ensure(id string) error {
	if c.Known(id) {
		return nil
	}
	return c.Mark(id, false, true)
}

// Remove drops the line. A missing line is success.
func (c *catalog) Remove(id string) error {
	entries, err := c.load()
	if err != nil {
		return err
	}
	out := entries[:0]
	found := false
	for _, e := range entries {
		if e.ID == id {
			found = true
			continue
		}
		out = append(out, e)
	}
	if !found {
		return nil
	}
	return c.save(out)
}

func (c *catalog) List() ([]savedEntry, error) {
	return c.load()
}

func (c *catalog) load() ([]savedEntry, error) {
	b, err := os.ReadFile(c.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if len(bytes.TrimSpace(b)) == 0 {
		return nil, nil
	}
	var entries []savedEntry
	if err := json.Unmarshal(b, &entries); err != nil {
		return nil, err
	}
	seen := make(map[string]bool, len(entries))
	out := make([]savedEntry, 0, len(entries))
	for _, e := range entries {
		if e.ID == "" || seen[e.ID] {
			continue
		}
		seen[e.ID] = true
		out = append(out, e)
	}
	return out, nil
}

func (c *catalog) save(entries []savedEntry) error {
	if entries == nil {
		entries = []savedEntry{}
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(entries); err != nil {
		return err
	}
	return writePrivate(c.path, buf.Bytes(), 0600)
}
