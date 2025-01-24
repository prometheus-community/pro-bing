package probing

import (
	"github.com/google/uuid"
	"time"
)

type seqMap struct {
	curUUID uuid.UUID
	head    *elem
	tail    *elem
	uuidMap map[uuid.UUID]map[int]*elem
}
type elem struct {
	uuid uuid.UUID
	seq  int
	time time.Time
	prev *elem
	next *elem
}

func newSeqMap(u uuid.UUID) seqMap {
	s := seqMap{
		curUUID: u,
		head:    &elem{},
		tail:    &elem{},
		uuidMap: map[uuid.UUID]map[int]*elem{},
	}
	s.uuidMap[u] = make(map[int]*elem)
	s.head.next = s.tail
	s.tail.prev = s.head
	return s
}

func (s seqMap) newSeqMap(u uuid.UUID) {
	s.curUUID = u
	s.uuidMap[u] = make(map[int]*elem)
}

func (s seqMap) putElem(uuid uuid.UUID, seq int, now time.Time) {
	e := &elem{
		uuid: uuid,
		seq:  seq,
		time: now,
		prev: s.tail.prev,
		next: s.tail,
	}
	s.tail.prev.next = e
	s.tail.prev = e
	s.uuidMap[uuid][seq] = e
}
func (s seqMap) getElem(uuid uuid.UUID, seq int) (*elem, bool) {
	e, ok := s.uuidMap[uuid][seq]
	return e, ok
}
func (s seqMap) removeElem(e *elem) {
	e.prev.next = e.next
	e.next.prev = e.prev
	if m, ok := s.uuidMap[e.uuid]; ok {
		delete(m, e.seq)
	}
}
func (s seqMap) peekFirst() *elem {
	if s.head.next == s.tail {
		return nil
	}
	return s.head.next
}
func (s seqMap) getCurUUID() uuid.UUID {
	return s.curUUID
}

func (s seqMap) checkUUIDExist(u uuid.UUID) bool {
	_, ok := s.uuidMap[u]
	return ok
}
