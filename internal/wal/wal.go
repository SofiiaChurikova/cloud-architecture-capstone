package wal

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"
)

type Entry struct {
	Offset  uint64          `json:"offset"`
	Version uint64          `json:"version"`
	Table   string          `json:"table"`
	Key     string          `json:"key"`
	Value   json.RawMessage `json:"value,omitempty"`
	Deleted bool            `json:"deleted,omitempty"`
}

type Log struct {
	mu      sync.Mutex
	f       *os.File
	entries []Entry
	changed chan struct{}
	version uint64

	Now func() time.Time
}

var ErrGap = errors.New("entry does not follow the last offset")

func Open(path string) (*Log, error) {
	l := &Log{changed: make(chan struct{}), Now: time.Now}
	if path == "" {
		return l, nil
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}

	var good int64
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 2<<20)
	for sc.Scan() {
		line := sc.Bytes()
		var e Entry
		if json.Unmarshal(line, &e) != nil || e.Offset != uint64(len(l.entries))+1 {
			break
		}
		l.entries = append(l.entries, e)
		l.version = max(l.version, e.Version)
		good += int64(len(line)) + 1
	}
	if err := f.Truncate(good); err != nil {
		f.Close()
		return nil, err
	}
	if _, err := f.Seek(good, 0); err != nil {
		f.Close()
		return nil, err
	}
	l.f = f
	return l, nil
}

func (l *Log) Last() uint64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return uint64(len(l.entries))
}

func (l *Log) Append(e Entry) (Entry, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	e.Offset = uint64(len(l.entries)) + 1
	e.Version = uint64(l.Now().UnixMicro())
	if e.Version <= l.version {
		e.Version = l.version + 1
	}
	if err := l.write([]Entry{e}); err != nil {
		return Entry{}, err
	}
	return e, nil
}

func (l *Log) AppendAt(batch []Entry) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	var todo []Entry
	next := uint64(len(l.entries)) + 1
	for _, e := range batch {
		if e.Offset < next {
			continue
		}
		if e.Offset != next {
			return fmt.Errorf("%w: got %d, want %d", ErrGap, e.Offset, next)
		}
		todo = append(todo, e)
		next++
	}
	if len(todo) == 0 {
		return nil
	}
	return l.write(todo)
}

func (l *Log) write(batch []Entry) error {
	if l.f != nil {
		var buf []byte
		for _, e := range batch {
			b, err := json.Marshal(e)
			if err != nil {
				return err
			}
			buf = append(append(buf, b...), '\n')
		}
		if _, err := l.f.Write(buf); err != nil {
			return err
		}
		if err := l.f.Sync(); err != nil {
			return err
		}
	}
	l.entries = append(l.entries, batch...)
	for _, e := range batch {
		l.version = max(l.version, e.Version)
	}
	close(l.changed)
	l.changed = make(chan struct{})
	return nil
}

func (l *Log) From(offset uint64, max int) []Entry {
	l.mu.Lock()
	defer l.mu.Unlock()
	if offset < 1 {
		offset = 1
	}
	if offset > uint64(len(l.entries)) {
		return nil
	}
	end := len(l.entries)
	if max > 0 && int(offset-1)+max < end {
		end = int(offset-1) + max
	}
	return append([]Entry(nil), l.entries[offset-1:end]...)
}

func (l *Log) Wait(ctx context.Context, offset uint64) bool {
	for {
		l.mu.Lock()
		if uint64(len(l.entries)) >= offset {
			l.mu.Unlock()
			return true
		}
		ch := l.changed
		l.mu.Unlock()
		select {
		case <-ch:
		case <-ctx.Done():
			return false
		}
	}
}

func (l *Log) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.f == nil {
		return nil
	}
	err := l.f.Close()
	l.f = nil
	return err
}
