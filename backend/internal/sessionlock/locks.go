package sessionlock

import (
	"slices"
	"sync"
)

type Locks struct {
	mu    sync.Mutex
	locks map[string]*sync.Mutex
}

func New() *Locks {
	return &Locks{locks: map[string]*sync.Mutex{}}
}

func (l *Locks) Lock(ids ...string) func() {
	ids = slices.Clone(ids)
	slices.Sort(ids)
	var held []*sync.Mutex
	for _, id := range slices.Compact(ids) {
		l.mu.Lock()
		lock := l.locks[id]
		if lock == nil {
			lock = &sync.Mutex{}
			l.locks[id] = lock
		}
		l.mu.Unlock()
		lock.Lock()
		held = append(held, lock)
	}
	return func() {
		for _, lock := range held {
			lock.Unlock()
		}
	}
}
