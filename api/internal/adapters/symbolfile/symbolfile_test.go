package symbolfile

import (
	"path/filepath"
	"slices"
	"testing"
)

func TestSaveLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "symbols.json")

	if _, found, err := Load(path); err != nil || found {
		t.Fatalf("missing file: found=%v err=%v", found, err)
	}
	if err := Save(path, []string{"XAUUSDm", "BTCUSDm"}); err != nil {
		t.Fatal(err)
	}
	got, found, err := Load(path)
	if err != nil || !found || !slices.Equal(got, []string{"XAUUSDm", "BTCUSDm"}) {
		t.Fatalf("got=%v found=%v err=%v", got, found, err)
	}
}
