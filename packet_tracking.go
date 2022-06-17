package probing

import (
	"sync"
	"time"

	"github.com/google/uuid"
)

type PacketTracker struct {
	currentUUID  uuid.UUID
	packets      map[uuid.UUID]PacketSequence
	sequence     int
	nextSequence int
	timeout      time.Duration
	timeoutCh    chan *inFlightPacket

	mutex sync.RWMutex
}

type PacketSequence struct {
	packets map[int]inFlightPacket
}

func (ps PacketSequence) NewInflightPacket(sequence int) {
	ps.packets[sequence] = inFlightPacket{}
}

func (ps PacketSequence) GetPacket(sequence int) (inFlightPacket, bool) {
	packet, ok := ps.packets[sequence]
	return packet, ok
}

func (ps PacketSequence) RemovePacket(sequence int) {
	delete(ps.packets, sequence)
}

type inFlightPacket struct {
	timeoutTimer *time.Timer
}

func newPacketTracker(t time.Duration) *PacketTracker {
	firstUUID := uuid.New()
	var firstSequence = map[uuid.UUID]map[int]struct{}{}
	firstSequence[firstUUID] = make(map[int]struct{})

	return &PacketTracker{
		packets:  map[uuid.UUID]PacketSequence{},
		sequence: 0,
		timeout:  t,
	}
}

func (t *PacketTracker) AddPacket() int {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	if t.nextSequence > 65535 {
		newUUID := uuid.New()
		t.packets[newUUID] = PacketSequence{}
		t.currentUUID = newUUID
		t.nextSequence = 0
	}

	t.sequence = t.nextSequence
	t.packets[t.currentUUID].NewInflightPacket(t.sequence)
	// if t.timeout > 0 {
	// 	t.packets[t.currentUUID][t.sequence].timeoutTimer = time.Timer(t.timeout)
	// }
	t.nextSequence++
	return t.sequence
}

// DeletePacket removes a packet from the tracker.
func (t *PacketTracker) DeletePacket(u uuid.UUID, seq int) {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	if t.hasPacket(u, seq) {
		// if _, ok := t.packets[u].GetPacket(seq) ; ok != nil {
		// 	t.packets[u][seq].timeoutTimer.Stop()
		// }
		t.packets[u].RemovePacket(seq)
	}
}

func (t *PacketTracker) hasPacket(u uuid.UUID, seq int) bool {
	inflight, ok := t.packets[u]
	if ok == false {
		return ok
	}
	_, ok = inflight.GetPacket(seq)
	return ok
}

// HasPacket checks the tracker to see if it's currently tracking a packet.
func (t *PacketTracker) HasPacket(u uuid.UUID, seq int) bool {
	t.mutex.RLock()
	defer t.mutex.Unlock()

	return t.hasPacket(u, seq)
}

func (t *PacketTracker) HasUUID(u uuid.UUID) bool {
	_, hasUUID := t.packets[u]
	return hasUUID
}

func (t *PacketTracker) CurrentUUID() uuid.UUID {
	// t.mutex.RLock()
	// defer t.mutex.Unlock()

	return t.currentUUID
}
