package hub

import (
	"encoding/json"
	"sync"
)

type Event struct {
	Type string `json:"type"`
	Data any    `json:"data"`
}

type Hub struct {
	mu   sync.Mutex
	subs map[int64]map[chan []byte]struct{}
}

func New() *Hub {
	return &Hub{subs: map[int64]map[chan []byte]struct{}{}}
}

func (h *Hub) Subscribe(group int64) (<-chan []byte, func()) {
	ch := make(chan []byte, 32)
	h.mu.Lock()
	if h.subs[group] == nil {
		h.subs[group] = map[chan []byte]struct{}{}
	}
	h.subs[group][ch] = struct{}{}
	h.mu.Unlock()
	return ch, func() {
		h.mu.Lock()
		if _, ok := h.subs[group][ch]; ok {
			delete(h.subs[group], ch)
			close(ch)
			if len(h.subs[group]) == 0 {
				delete(h.subs, group)
			}
		}
		h.mu.Unlock()
	}
}

func (h *Hub) Publish(group int64, typ string, data any) {
	msg, err := json.Marshal(Event{Type: typ, Data: data})
	if err != nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs[group] {
		select {
		case ch <- msg:
		default:
		}
	}
}

func (h *Hub) Count(group int64) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.subs[group])
}
