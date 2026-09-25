package hub

import (
	"encoding/json"
	"testing"
)

func TestPublishReachesOnlyGroup(t *testing.T) {
	h := New()
	a, unsubA := h.Subscribe(1)
	b, unsubB := h.Subscribe(2)
	defer unsubB()
	h.Publish(1, "trade", map[string]int{"id": 7})
	var e Event
	json.Unmarshal(<-a, &e)
	if e.Type != "trade" {
		t.Fatalf("event = %+v", e)
	}
	select {
	case <-b:
		t.Fatal("group 2 received group 1 event")
	default:
	}
	unsubA()
	unsubA()
	if h.Count(1) != 0 {
		t.Fatal("unsubscribe did not remove subscriber")
	}
	h.Publish(1, "trade", nil)
}

func TestSlowSubscriberDoesNotBlock(t *testing.T) {
	h := New()
	_, unsub := h.Subscribe(1)
	defer unsub()
	for range 100 {
		h.Publish(1, "trade", nil)
	}
}
