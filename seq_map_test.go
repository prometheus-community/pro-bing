package probing

import (
	"github.com/google/uuid"
	"testing"
	"time"
)

func TestSeqMap(t *testing.T) {
	u := uuid.New()
	s := newSeqMap(u)
	t.Run("newSeqMap", func(t *testing.T) {
		u2 := uuid.New()
		s.newSeqMap(u2)

		if _, ok := s.uuidMap[u2]; !ok {
			t.Errorf("Expected seqMap to contain UUID %s", u2.String())
		}
	})

	t.Run("putElem", func(t *testing.T) {
		seq := 1
		s.putElem(u, seq, time.Now())

		// Check that seqMap contains the correct elements
		if _, ok := s.uuidMap[u][seq]; !ok {
			t.Errorf("Expected seqMap[%s][%d] to exist", u.String(), seq)
		}

		if s.peekFirst().seq != seq {
			t.Errorf("Expected tail.prev.seq to be %d, got %d", seq, s.tail.prev.seq)
		}
	})

	t.Run("getElem", func(t *testing.T) {
		seq := 1
		elem, ok := s.getElem(u, seq)

		if !ok {
			t.Errorf("Expected getElem to return true for existing element")
		}

		if elem.seq != seq {
			t.Errorf("Expected element's seq to be %d, got %d", seq, elem.seq)
		}
	})

	t.Run("removeElem", func(t *testing.T) {
		seq := 1
		elem, ok := s.getElem(u, seq)
		if !ok {
			t.Fatalf("Expected getElem to return true for existing element")
		}

		s.removeElem(elem)

		if _, ok := s.uuidMap[u][seq]; ok {
			t.Errorf("Expected seqMap[%s][%d] to be removed", u.String(), seq)
		}
	})

	// test peekFirst
	t.Run("peekFirst", func(t *testing.T) {
		seq := 2
		s.putElem(u, seq, time.Now())

		elem := s.peekFirst()

		// Check that peekFirst returns the first element of the linked list
		if elem.seq != seq {
			t.Errorf("Expected peekFirst to return element with seq %d, got %d", seq, elem.seq)
		}
	})
}

func TestSeqMap2(t *testing.T) {
	u := uuid.New()
	s := newSeqMap(u)
	for i := 0; i < 100; i++ {
		s.putElem(u, i, time.Now())
	}
	for i := 0; i < 100; i++ {
		e, ok := s.getElem(u, i)
		AssertTrue(t, ok && e.seq == i)
	}
	for i := 0; i < 20; i++ {
		first := s.peekFirst()
		AssertTrue(t, first.seq == i)
		s.removeElem(first)
	}
}
