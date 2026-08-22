package queue

type readyHeap struct {
	items    []Message
	ordering Ordering
}

func (h readyHeap) Len() int {
	return len(h.items)
}

func (h readyHeap) Less(i, j int) bool {
	left := h.items[i]
	right := h.items[j]

	if left.Priority != right.Priority {
		return left.Priority > right.Priority
	}

	if h.ordering == FIFO {
		return left.Sequence < right.Sequence
	}

	return left.Sequence > right.Sequence
}

func (h readyHeap) Swap(i, j int) {
	h.items[i], h.items[j] = h.items[j], h.items[i]
}

func (h *readyHeap) Push(value any) {
	h.items = append(h.items, value.(Message))
}

func (h *readyHeap) Pop() any {
	last := len(h.items) - 1
	message := h.items[last]
	h.items[last] = Message{}
	h.items = h.items[:last]
	return message
}
