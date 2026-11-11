package services

import (
	"errors"
	"slices"
	"sync"
)

// ErrSymbolExists and ErrSymbolNotFound are returned by SymbolSet.Add and Remove.
var (
	ErrSymbolExists   = errors.New("symbol already tracked")
	ErrSymbolNotFound = errors.New("symbol not tracked")
)

// SymbolSet is the list of symbols the gateway tracks (backfills and serves).
// It is safe for concurrent use. Every change is passed to save, if set,
// before it becomes visible; a failed save leaves the set unchanged.
type SymbolSet struct {
	mu   sync.RWMutex
	list []string
	save func([]string) error
}

func NewSymbolSet(initial []string, save func([]string) error) *SymbolSet {
	var list []string
	for _, s := range initial {
		if s != "" && !slices.Contains(list, s) {
			list = append(list, s)
		}
	}
	return &SymbolSet{list: list, save: save}
}

// List returns a copy of the tracked symbols in the order they were added.
func (s *SymbolSet) List() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return slices.Clone(s.list)
}

func (s *SymbolSet) Has(sym string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return slices.Contains(s.list, sym)
}

func (s *SymbolSet) Add(sym string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if slices.Contains(s.list, sym) {
		return ErrSymbolExists
	}
	return s.commit(append(slices.Clone(s.list), sym))
}

func (s *SymbolSet) Remove(sym string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := slices.Index(s.list, sym)
	if i < 0 {
		return ErrSymbolNotFound
	}
	return s.commit(slices.Delete(slices.Clone(s.list), i, i+1))
}

func (s *SymbolSet) commit(next []string) error {
	if s.save != nil {
		if err := s.save(next); err != nil {
			return err
		}
	}
	s.list = next
	return nil
}
