package mqtt

import (
	"container/list"
	"sync"

	"github.com/eclipse/paho.golang/paho"
)

// Paho unregisters a whole topic filter at once. Give it one route per filter so
// a failed SUBSCRIBE can remove just its own handler, even for concurrent callers.
type subscriberRoutes struct {
	mu     sync.Mutex
	router *paho.StandardRouter
	topics map[string]*topicHandlers
}

type topicHandlers struct {
	mu       sync.RWMutex
	handlers list.List // paho.MessageHandler, in registration order
}

func (h *topicHandlers) deliver(msg *paho.Publish) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for e := h.handlers.Front(); e != nil; e = e.Next() {
		e.Value.(paho.MessageHandler)(msg)
	}
}

func (r *subscriberRoutes) register(topic string, handler paho.MessageHandler) (rollback func()) {
	r.mu.Lock()
	defer r.mu.Unlock()
	h := r.topics[topic]
	if h == nil {
		h = &topicHandlers{}
		r.topics[topic] = h
		r.router.RegisterHandler(topic, h.deliver)
	}
	h.mu.Lock()
	entry := h.handlers.PushBack(handler)
	h.mu.Unlock()
	return func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		h.mu.Lock()
		h.handlers.Remove(entry)
		empty := h.handlers.Len() == 0
		h.mu.Unlock()
		if empty {
			r.router.UnregisterHandler(topic)
			delete(r.topics, topic)
		}
	}
}
