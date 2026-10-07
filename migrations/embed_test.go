package migrations

import (
	"testing"
	"testing/fstest"
)

func TestMigrationExtensionsRejectConflictsAndFreeze(t *testing.T) {
	s := NewSources()
	if e := s.Register(fstest.MapFS{"001_base.sql": &fstest.MapFile{Data: []byte("-- +goose Up\nSELECT 1;")}}); e != nil {
		t.Fatal(e)
	}
	if e := s.Register(fstest.MapFS{"001_duplicate.sql": &fstest.MapFile{}}); e == nil {
		t.Fatal("duplicate migration version")
	}
	s.Freeze()
	if e := s.Register(fstest.MapFS{"002_next.sql": &fstest.MapFile{}}); e == nil {
		t.Fatal("running source mutable")
	}
}
