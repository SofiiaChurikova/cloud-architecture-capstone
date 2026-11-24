package store

import "testing"

func TestLastWriteWins(t *testing.T) {
	s := New()
	s.Apply("t", "k", Item{Value: []byte(`{"v":2}`), Version: 200})
	if s.Apply("t", "k", Item{Value: []byte(`{"v":1}`), Version: 100}) {
		t.Fatal("an older version replaced a newer one")
	}
	if s.Apply("t", "k", Item{Value: []byte(`{"v":1}`), Version: 200}) {
		t.Fatal("the same version was applied twice")
	}
	if !s.Apply("t", "k", Item{Deleted: true, Version: 300}) {
		t.Fatal("a newer delete was ignored")
	}
	if it, _ := s.Get("t", "k"); !it.Deleted || it.Version != 300 {
		t.Fatalf("got %+v, want the delete with version 300", it)
	}
}
