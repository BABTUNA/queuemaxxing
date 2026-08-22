package queue

type delayedHeap []Message

func (h delayedHeap) Len() int {
	return len(h)
}

func (h delayedHeap) Less(i, j int) bool {
	if !h[i].AvailableAt.Equal(h[j].AvailableAt) {
		return h[i].AvailableAt.Before(h[j].AvailableAt)
	}

	return h[i].Sequence < h[j].Sequence
}

func (h delayedHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
}

func (h *delayedHeap) Push(value any) {
	*h = append(*h, value.(Message))
}

func (h *delayedHeap) Pop() any {
	items := *h
	last := len(items) - 1
	message := items[last]
	items[last] = Message{}
	*h = items[:last]
	return message
}
