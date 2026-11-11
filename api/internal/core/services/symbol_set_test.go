package services

import (
	"errors"
	"slices"
	"testing"
)

func TestSymbolSet_AddRemove(t *testing.T) {
	var saved []string
	s := NewSymbolSet([]string{"XAUUSDm", "XAUUSDm", ""}, func(l []string) error { saved = l; return nil })

	if got := s.List(); !slices.Equal(got, []string{"XAUUSDm"}) {
		t.Fatalf("initial = %v", got)
	}
	if err := s.Add("BTCUSDm"); err != nil {
		t.Fatal(err)
	}
	if err := s.Add("BTCUSDm"); !errors.Is(err, ErrSymbolExists) {
		t.Errorf("duplicate add err = %v", err)
	}
	if !slices.Equal(saved, []string{"XAUUSDm", "BTCUSDm"}) {
		t.Errorf("saved = %v", saved)
	}
	if err := s.Remove("XAUUSDm"); err != nil {
		t.Fatal(err)
	}
	if err := s.Remove("XAUUSDm"); !errors.Is(err, ErrSymbolNotFound) {
		t.Errorf("missing remove err = %v", err)
	}
	if got := s.List(); !slices.Equal(got, []string{"BTCUSDm"}) || !slices.Equal(saved, got) {
		t.Errorf("list = %v, saved = %v", got, saved)
	}
}

func TestSymbolSet_FailedSaveKeepsList(t *testing.T) {
	s := NewSymbolSet([]string{"XAUUSDm"}, func([]string) error { return errors.New("disk full") })
	if err := s.Add("BTCUSDm"); err == nil {
		t.Fatal("want error")
	}
	if s.Has("BTCUSDm") {
		t.Error("symbol added despite failed save")
	}
}
