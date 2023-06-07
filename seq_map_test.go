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

		if len(s.trackerUUIDs) != 1 {
			t.Errorf("Expected length of trackerUUIDs to be 2, got %d", len(s.trackerUUIDs))
		}

		if _, ok := s.seqMap[u2]; !ok {
			t.Errorf("Expected seqMap to contain UUID %s", u2.String())
		}
	})

	t.Run("putElem", func(t *testing.T) {
		seq := 1
		s.putElem(u, seq, time.Now())

		// 检查 seqMap 中是否包含正确的元素
		if _, ok := s.seqMap[u][seq]; !ok {
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

		if _, ok := s.seqMap[u][seq]; ok {
			t.Errorf("Expected seqMap[%s][%d] to be removed", u.String(), seq)
		}
	})

	// test peekFirst
	t.Run("peekFirst", func(t *testing.T) {
		seq := 2
		s.putElem(u, seq, time.Now())

		elem := s.peekFirst()

		// 检查 peekFirst 是否返回链表的第一个元素
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
