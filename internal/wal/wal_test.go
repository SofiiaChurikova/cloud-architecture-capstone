package wal

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestOffsetsGoUp(t *testing.T) {
	l, _ := Open("")
	a, _ := l.Append(Entry{Table: "users", Key: "1", Value: []byte(`{"v":1}`)})
	b, _ := l.Append(Entry{Table: "users", Key: "1", Value: []byte(`{"v":2}`)})
	if a.Offset != 1 || b.Offset != 2 {
		t.Fatalf("offsets %d, %d, want 1, 2", a.Offset, b.Offset)
	}
}

func TestReopenKeepsEntries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "log.jsonl")
	l, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		l.Append(Entry{Table: "users", Key: "k", Value: []byte(`{"v":1}`)})
	}
	l.Append(Entry{Table: "users", Key: "k", Deleted: true})
	l.Close()

	l, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if l.Last() != 4 {
		t.Fatalf("last offset %d, want 4", l.Last())
	}
	if got := l.From(4, 0); len(got) != 1 || !got[0].Deleted {
		t.Fatalf("entry 4 should be a delete: %+v", got)
	}
	e, _ := l.Append(Entry{Table: "users", Key: "k", Value: []byte(`{"v":5}`)})
	if e.Offset != 5 {
		t.Fatalf("next offset %d, want 5", e.Offset)
	}
}

func TestBrokenLastLineIsDropped(t *testing.T) {
	path := filepath.Join(t.TempDir(), "log.jsonl")
	l, _ := Open(path)
	l.Append(Entry{Table: "users", Key: "1", Value: []byte(`{"v":1}`)})
	l.Close()

	f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	f.WriteString(`{"offset":2,"table":"us`)
	f.Close()

	l, _ = Open(path)
	if l.Last() != 1 {
		t.Fatalf("last offset %d, want 1", l.Last())
	}
	e, _ := l.Append(Entry{Table: "users", Key: "2", Value: []byte(`{"v":2}`)})
	l.Close()
	l, _ = Open(path)
	if e.Offset != 2 || l.Last() != 2 {
		t.Fatalf("after repair: offset %d, last %d", e.Offset, l.Last())
	}
}

func TestAppendAtNeedsNextOffset(t *testing.T) {
	l, _ := Open("")
	if err := l.AppendAt([]Entry{{Offset: 1}, {Offset: 2}}); err != nil {
		t.Fatal(err)
	}
	if err := l.AppendAt([]Entry{{Offset: 2}, {Offset: 3}}); err != nil {
		t.Fatalf("an old entry should be skipped: %v", err)
	}
	if err := l.AppendAt([]Entry{{Offset: 5}}); !errors.Is(err, ErrGap) {
		t.Fatalf("want ErrGap, got %v", err)
	}
	if l.Last() != 3 {
		t.Fatalf("last %d, want 3", l.Last())
	}
}

func TestWait(t *testing.T) {
	l, _ := Open("")
	go func() {
		time.Sleep(20 * time.Millisecond)
		l.Append(Entry{Table: "t", Key: "k"})
	}()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if !l.Wait(ctx, 1) {
		t.Fatal("Wait returned before the entry came")
	}
	ctx2, cancel2 := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel2()
	if l.Wait(ctx2, 2) {
		t.Fatal("Wait should time out")
	}
}

func TestVersionIsLeaderTime(t *testing.T) {
	path := filepath.Join(t.TempDir(), "log.jsonl")
	l, _ := Open(path)
	now := time.UnixMicro(1000)
	l.Now = func() time.Time { return now }

	a, _ := l.Append(Entry{Table: "t", Key: "1"})
	b, _ := l.Append(Entry{Table: "t", Key: "1"})
	now = time.UnixMicro(500)
	c, _ := l.Append(Entry{Table: "t", Key: "1"})
	if a.Version != 1000 || b.Version != 1001 || c.Version != 1002 {
		t.Fatalf("versions %d %d %d, want 1000 1001 1002", a.Version, b.Version, c.Version)
	}
	l.Close()

	l, _ = Open(path)
	l.Now = func() time.Time { return time.UnixMicro(10) }
	d, _ := l.Append(Entry{Table: "t", Key: "1"})
	if d.Version != 1003 {
		t.Fatalf("after reopen with an older clock version %d, want 1003", d.Version)
	}
}
