package stream

import (
	"sync"
)

type Broadcaster struct {
	mu          sync.RWMutex
	subscribers map[chan []byte]struct{}
	bufferSize  int
	lastFrame   []byte
	lastFrameMu sync.RWMutex
	closed      bool
}

func NewBroadcaster(bufferSize int) *Broadcaster {
	if bufferSize <= 0 {
		bufferSize = 10
	}
	return &Broadcaster{
		subscribers: make(map[chan []byte]struct{}),
		bufferSize:  bufferSize,
	}
}

func (b *Broadcaster) Broadcast(frame []byte) {
	if len(frame) == 0 {
		return
	}

	b.lastFrameMu.Lock()
	b.lastFrame = frame
	b.lastFrameMu.Unlock()

	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.closed {
		return
	}

	for ch := range b.subscribers {
		select {
		case ch <- frame:
		default:

		}
	}
}

func (b *Broadcaster) Subscribe() (<-chan []byte, func()) {
	b.mu.Lock()
	defer b.mu.Unlock()

	ch := make(chan []byte, b.bufferSize)

	b.lastFrameMu.RLock()
	if len(b.lastFrame) > 0 {
		select {
		case ch <- b.lastFrame:
		default:
		}
	}
	b.lastFrameMu.RUnlock()

	if b.closed {
		close(ch)
		return ch, func() {}
	}

	b.subscribers[ch] = struct{}{}

	var once sync.Once
	unsubscribe := func() {
		once.Do(func() {
			b.mu.Lock()
			delete(b.subscribers, ch)
			b.mu.Unlock()
			close(ch)
		})
	}

	return ch, unsubscribe
}

func (b *Broadcaster) LastFrame() []byte {
	b.lastFrameMu.RLock()
	defer b.lastFrameMu.RUnlock()
	if len(b.lastFrame) == 0 {
		return nil
	}
	cp := make([]byte, len(b.lastFrame))
	copy(cp, b.lastFrame)
	return cp
}

func (b *Broadcaster) SubscribersCount() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.subscribers)
}

func (b *Broadcaster) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return
	}
	b.closed = true

	for ch := range b.subscribers {
		close(ch)
		delete(b.subscribers, ch)
	}
}
