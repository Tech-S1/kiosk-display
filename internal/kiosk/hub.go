package kiosk

import (
	"encoding/json"
	"sync"
)

type Client struct {
	Frames chan []byte
	Status chan []byte
}

type FrameHub struct {
	mu      sync.Mutex
	clients map[*Client]struct{}
}

func newFrameHub() *FrameHub {
	return &FrameHub{clients: make(map[*Client]struct{})}
}

func (h *FrameHub) Subscribe() *Client {
	c := &Client{
		Frames: make(chan []byte, 2),
		Status: make(chan []byte, 4),
	}
	h.mu.Lock()
	h.clients[c] = struct{}{}
	h.mu.Unlock()
	return c
}

func (h *FrameHub) Unsubscribe(c *Client) {
	h.mu.Lock()
	if _, ok := h.clients[c]; ok {
		delete(h.clients, c)
		close(c.Frames)
		close(c.Status)
	}
	h.mu.Unlock()
}

func (h *FrameHub) Viewers() int {
	h.mu.Lock()
	n := len(h.clients)
	h.mu.Unlock()
	return n
}

func (h *FrameHub) Broadcast(frame []byte) {
	if len(frame) == 0 {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for c := range h.clients {
		select {
		case c.Frames <- frame:
		default:
			select {
			case <-c.Frames:
			default:
			}
			select {
			case c.Frames <- frame:
			default:
			}
		}
	}
}

func (h *FrameHub) BroadcastStatus(v any) {
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for c := range h.clients {
		select {
		case c.Status <- b:
		default:
			select {
			case <-c.Status:
			default:
			}
			select {
			case c.Status <- b:
			default:
			}
		}
	}
}
