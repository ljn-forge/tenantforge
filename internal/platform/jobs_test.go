package platform

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/google/uuid"
)

func TestCSVFormulaEscapingAndTypes(t *testing.T) {
	if csvCell("=CMD()") != "'=CMD()" {
		t.Fatal("spreadsheet formula injection")
	}
	if v, e := parseCSVValue(Field{Type: "boolean"}, "false"); e != nil || v != false {
		t.Fatal("boolean parsing")
	}
	if _, e := parseCSVValue(Field{Type: "number"}, "NaN"); e == nil {
		t.Fatal("nonfinite number")
	}
}
func TestLocalStorageTraversalAndRoundtrip(t *testing.T) {
	ctx := context.Background()
	s, e := NewStorage(ctx, Config{StorageBackend: "local", DataDir: t.TempDir()})
	if e != nil {
		t.Fatal(e)
	}
	if e = s.Put(ctx, "../escape", bytes.NewBufferString("x"), 1, "text/plain"); e == nil {
		t.Fatal("path traversal")
	}
	key := uuid.NewString()
	if e = s.Put(ctx, key, bytes.NewBufferString("hello"), 5, "text/plain"); e != nil {
		t.Fatal(e)
	}
	f, e := s.Open(ctx, key)
	if e != nil {
		t.Fatal(e)
	}
	b, e := io.ReadAll(f)
	f.Close()
	if e != nil || string(b) != "hello" {
		t.Fatal("stored content")
	}
	if e = s.Delete(ctx, key); e != nil {
		t.Fatal(e)
	}
}
