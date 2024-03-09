package probing

import (
	"math"

	"github.com/google/uuid"
)

type sequenceBucket struct {
	bitmap    []bool
	usedCount int64
}

type idGenerator struct {
	awaitingSequences         map[uuid.UUID]*sequenceBucket
	currentCount              int
	currentUUID               uuid.UUID
	oldestUUIDQueue           []uuid.UUID
	totalOutstandingSequences int64
	deletionPolicyFn          func(*idGenerator)
}

func newIDGenerator(deletionPolicy func(*idGenerator)) *idGenerator {
	return &idGenerator{
		awaitingSequences: make(map[uuid.UUID]*sequenceBucket),
		deletionPolicyFn:  deletionPolicy,
	}
}

func (g *idGenerator) next() (uuid.UUID, int) {
	if g.currentCount == 0 {
		g.currentUUID = uuid.New()
	}

	var ok bool
	if _, ok = g.awaitingSequences[g.currentUUID]; !ok {
		inFlightSeqs := new(sequenceBucket)
		inFlightSeqs.bitmap = make([]bool, math.MaxUint16)
		g.awaitingSequences[g.currentUUID] = inFlightSeqs
		g.oldestUUIDQueue = append(g.oldestUUIDQueue, g.currentUUID)

	}

	nextSeq := g.currentCount
	nextUUID := g.currentUUID
	g.totalOutstandingSequences++

	g.awaitingSequences[g.currentUUID].bitmap[g.currentCount] = true
	g.awaitingSequences[g.currentUUID].usedCount++

	// Run the deletion policy. May modify awaitingSequences
	g.deletionPolicyFn(g)

	g.currentCount = (g.currentCount + 1) % math.MaxUint16
	return nextUUID, nextSeq
}

func (g *idGenerator) find(UUID uuid.UUID, sequenceNum int) bool {
	if val, ok := g.awaitingSequences[UUID]; ok {
		return val.bitmap[sequenceNum]
	}
	return false
}

func (g *idGenerator) retire(UUID uuid.UUID, sequenceNum int) {
	if val, ok := g.awaitingSequences[UUID]; ok {
		val.bitmap[sequenceNum] = false
		val.usedCount--
	}
	g.totalOutstandingSequences--
}

func (g *idGenerator) retireBucket(UUID uuid.UUID) {
	if val, ok := g.awaitingSequences[UUID]; ok {
		g.totalOutstandingSequences -= val.usedCount
		delete(g.awaitingSequences, UUID)
	}
}

func RemoveAfterMaxItems(maxItems int64) func(*idGenerator) {
	return func(g *idGenerator) {
		if g.totalOutstandingSequences == maxItems {
			toDeleteUUID := g.oldestUUIDQueue[0]
			g.oldestUUIDQueue = g.oldestUUIDQueue[1:]
			g.retireBucket(toDeleteUUID)
		}
	}
}
