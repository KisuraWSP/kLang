package runtime

import (
	"fmt"
	"sync"
)

type MemoryRegion string

const (
	MemoryStack     MemoryRegion = "stack"
	MemoryHeap      MemoryRegion = "heap"
	MemoryTemporary MemoryRegion = "temporary"
)

type Memory struct {
	mu           sync.Mutex
	nextID       int
	objects      map[int]*Object
	stackObjects int
	heapObjects  int
	tempObjects  int
	stackBytes   int
	heapBytes    int
	tempBytes    int
}

type Object struct {
	Value            Value
	Region           MemoryRegion
	Bytes            int
	ImmutableBorrows int
	MutableBorrow    bool
}

type MemoryStats struct {
	StackObjects int
	HeapObjects  int
	TempObjects  int
	StackBytes   int
	HeapBytes    int
	TempBytes    int
	TotalObjects int
	TotalBytes   int
}

func NewMemory() *Memory {
	return &Memory{
		nextID:  1,
		objects: map[int]*Object{},
	}
}

func (memory *Memory) Allocate(value Value, region MemoryRegion) int {
	memory.mu.Lock()
	defer memory.mu.Unlock()
	if region == "" {
		region = MemoryStack
	}
	id := memory.nextID
	memory.nextID++
	size := valueSize(value)
	memory.objects[id] = &Object{Value: value, Region: region, Bytes: size}
	memory.addAccounting(region, size)
	return id
}

func (memory *Memory) Store(id int, value Value) {
	memory.mu.Lock()
	defer memory.mu.Unlock()
	if object, ok := memory.objects[id]; ok {
		oldBytes := object.Bytes
		object.Value = value
		object.Bytes = valueSize(value)
		memory.addBytes(object.Region, object.Bytes-oldBytes)
	}
}

func (memory *Memory) Stats() MemoryStats {
	memory.mu.Lock()
	defer memory.mu.Unlock()
	return MemoryStats{
		StackObjects: memory.stackObjects,
		HeapObjects:  memory.heapObjects,
		TempObjects:  memory.tempObjects,
		StackBytes:   memory.stackBytes,
		HeapBytes:    memory.heapBytes,
		TempBytes:    memory.tempBytes,
		TotalObjects: memory.stackObjects + memory.heapObjects + memory.tempObjects,
		TotalBytes:   memory.stackBytes + memory.heapBytes + memory.tempBytes,
	}
}

func (memory *Memory) addAccounting(region MemoryRegion, bytes int) {
	if region == MemoryTemporary {
		if bytes > 0 {
			memory.tempObjects++
		}
		memory.tempBytes += bytes
		return
	}
	if region == MemoryHeap {
		if bytes > 0 {
			memory.heapObjects++
		}
		memory.heapBytes += bytes
		return
	}
	if bytes > 0 {
		memory.stackObjects++
	}
	memory.stackBytes += bytes
}

func (memory *Memory) addBytes(region MemoryRegion, bytes int) {
	if region == MemoryTemporary {
		memory.tempBytes += bytes
		return
	}
	if region == MemoryHeap {
		memory.heapBytes += bytes
		return
	}
	memory.stackBytes += bytes
}

func (memory *Memory) BorrowImmutable(id int) error {
	memory.mu.Lock()
	defer memory.mu.Unlock()
	object, ok := memory.objects[id]
	if !ok {
		return Error{Message: fmt.Sprintf("unknown memory object %d", id)}
	}
	if object.MutableBorrow {
		return Error{Message: fmt.Sprintf("cannot immutably borrow object %d while it is mutably borrowed", id)}
	}
	object.ImmutableBorrows++
	return nil
}

func (memory *Memory) ReleaseImmutable(id int) {
	memory.mu.Lock()
	defer memory.mu.Unlock()
	if object, ok := memory.objects[id]; ok && object.ImmutableBorrows > 0 {
		object.ImmutableBorrows--
	}
}

func (memory *Memory) BorrowMutable(id int) error {
	memory.mu.Lock()
	defer memory.mu.Unlock()
	object, ok := memory.objects[id]
	if !ok {
		return Error{Message: fmt.Sprintf("unknown memory object %d", id)}
	}
	if object.MutableBorrow || object.ImmutableBorrows > 0 {
		return Error{Message: fmt.Sprintf("cannot mutably borrow object %d while borrowed", id)}
	}
	object.MutableBorrow = true
	return nil
}

func (memory *Memory) ReleaseMutable(id int) {
	memory.mu.Lock()
	defer memory.mu.Unlock()
	if object, ok := memory.objects[id]; ok {
		object.MutableBorrow = false
	}
}

func (memory *Memory) EnsureWritable(id int) error {
	if err := memory.BorrowMutable(id); err != nil {
		return err
	}
	memory.ReleaseMutable(id)
	return nil
}
