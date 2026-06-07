package runtime

import "fmt"

type Memory struct {
	nextID  int
	objects map[int]*Object
}

type Object struct {
	Value            Value
	ImmutableBorrows int
	MutableBorrow    bool
}

func NewMemory() *Memory {
	return &Memory{
		nextID:  1,
		objects: map[int]*Object{},
	}
}

func (memory *Memory) Allocate(value Value) int {
	id := memory.nextID
	memory.nextID++
	memory.objects[id] = &Object{Value: value}
	return id
}

func (memory *Memory) Store(id int, value Value) {
	if object, ok := memory.objects[id]; ok {
		object.Value = value
	}
}

func (memory *Memory) BorrowImmutable(id int) error {
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
	if object, ok := memory.objects[id]; ok && object.ImmutableBorrows > 0 {
		object.ImmutableBorrows--
	}
}

func (memory *Memory) BorrowMutable(id int) error {
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
