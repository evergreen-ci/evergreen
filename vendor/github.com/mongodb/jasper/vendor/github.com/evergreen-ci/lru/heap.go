package lru

type fileObjectHeap []*FileObject

func (h fileObjectHeap) Len() int { return len(h) }

func (h fileObjectHeap) Less(i, j int) bool { return h[i].Time.Before(h[j].Time) }

func (h fileObjectHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].index = i
	h[j].index = j
}

func (h *fileObjectHeap) Push(x interface{}) {
	n := len(*h)
	item := x.(*FileObject)
	item.index = n
	*h = append(*h, item)
}

func (h *fileObjectHeap) Pop() interface{} {
	old := *h
	n := len(old)
	item := old[n-1]
	item.index = -1
	*h = old[0 : n-1]
	return item
}
