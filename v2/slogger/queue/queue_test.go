// Copyright 2013 MongoDB, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package queue

import (
	"runtime"
	"sync"
	"testing"
)

func TestNew(t *testing.T) {
	var _ *Queue = New(10, func(interface{}) {})
}

func TestCap(t *testing.T) {
	q := New(10, func(interface{}) {})
	if q.Cap() != 10 {
		t.Fatal("q.Cap() != 10 !")
	}
}

func TestConcurrentEnqueue(t *testing.T) {
	runtime.GOMAXPROCS(runtime.NumCPU())

	num_groups, items_per_group := 10, 1000
	capacity := num_groups * items_per_group

	onForcedDequeueCalled := false
	q := New(capacity, func(interface{}) { onForcedDequeueCalled = true })

	var wg sync.WaitGroup
	for i := 0; i < num_groups; i++ {
		wg.Add(1)
		go enqueueGroup(q, i, items_per_group, &wg)
	}

	wg.Wait()

	if !q.IsFull() {
		t.Errorf("queue should now be full")
	}

	if onForcedDequeueCalled {
		t.Errorf("A dequeue should not have been forced")
	}

	last_seen_ints := make([]int, num_groups)

	// check that it's sorted within groups
	for i := 0; i < capacity; i++ {
		item, err := q.Dequeue()
		if err != nil {
			t.Fatalf("Error while dequeueing: %v", err)
		}

		concurrentItem, ok := item.(*concurrentTestItem)

		if !ok {
			t.Fatalf("Only concurrentTestItem's should be in this queue")
		}

		if last_seen_ints[concurrentItem.group] != concurrentItem.seq {
			t.Fatalf("last_seen_ints[item.group] should equal item.seq")
		}

		last_seen_ints[concurrentItem.group]++
	}

}

func TestDequeueUnderflow(t *testing.T) {
	q := New(10, func(interface{}) {})

	item, err := q.Dequeue()

	if _, ok := err.(UnderflowError); !ok {
		t.Errorf("Dequeueing an empty queue should return an UnderflowError")
	}

	if item != nil {
		t.Errorf("Dequeueing an empty queue should return a nil item")
	}
}

func TestEnqueueDequeue(t *testing.T) {
	onForcedDequeueCalled := false
	q := New(10, func(interface{}) { onForcedDequeueCalled = true })

	q.Enqueue("Hello")

	if onForcedDequeueCalled {
		t.Error("onForcedDequeued should not have been called")
	}

	b, err := q.Dequeue()

	if err != nil {
		t.Errorf("q.Dequeue() should not have returned an error: %v", err)
	}

	b_str, ok := b.(string)
	if !ok {
		t.Errorf("q.Dequeue() should have returned a string as that is what was enqueued")
	}

	if b_str != "Hello" {
		t.Errorf("q.Dequeue() should have returned what was enqueued (\"Hello\") but returned \"%s\" instead", b)
	}
}

func TestEnqueueOverCapacity(t *testing.T) {
	onForcedDequeueCalled := false
	q := New(1, func(interface{}) { onForcedDequeueCalled = true })

	q.Enqueue("Hello")
	if onForcedDequeueCalled {
		t.Error("onForcedDequeued should not have been called")
	}

	q.Enqueue("World")
	if !onForcedDequeueCalled {
		t.Error("onForcedDequeued should have been called")
	}

	b, err := q.Dequeue()

	if err != nil {
		t.Errorf("q.Dequeue() should not have returned an error: %v", err)
	}

	b_str, ok := b.(string)
	if !ok {
		t.Errorf("q.Dequeue() should have returned a string as that is what was enqueued")
	}

	if b_str != "World" {
		t.Errorf("q.Dequeue() should have returned what was enqueued (\"World\") but returned \"%s\" instead", b)
	}
}

func TestIsEmpty(t *testing.T) {
	q := New(10, func(interface{}) {})

	if !q.IsEmpty() {
		t.Errorf("An empty queue should return true for IsEmpty()")
	}

	q.Enqueue("Hello")
	if q.IsEmpty() {
		t.Errorf("A queue should not return true for IsEmpty() after enqueueing one item")
	}

	q.Dequeue()
	if !q.IsEmpty() {
		t.Errorf("The queue should now be empty")
	}
}

func TestIsFull(t *testing.T) {
	q := New(1, func(interface{}) {})

	if q.IsFull() {
		t.Errorf("An empty queue with a non-zero capacity should not be full")
	}

	q.Enqueue("Hello")
	if !q.IsFull() {
		t.Errorf("The queue should now be full")
	}

	q.Dequeue()
	if q.IsFull() {
		t.Errorf("The queue should not be full now")
	}
}

func TestLen(t *testing.T) {
	q := New(10, func(interface{}) {})

	if q.Len() != 0 {
		t.Errorf("An empty queue should have a Len of 0")
	}

	q.Enqueue("Hello")
	if q.Len() != 1 {
		t.Errorf("The queue should have a Len of 1 now")
	}

	q.Enqueue("World")
	if q.Len() != 2 {
		t.Errorf("The queue should have a Len of 2 now")
	}

	q.Dequeue()
	if q.Len() != 1 {
		t.Errorf("The queue should have a Len of 1 now")
	}

	q.Dequeue()
	if q.Len() != 0 {
		t.Errorf("The queue should have a Len of 0 now")
	}
}

type concurrentTestItem struct {
	group int
	seq   int
}

func enqueueGroup(q *Queue, group int, n int, wg *sync.WaitGroup) {
	defer wg.Done()
	for i := 0; i < n; i++ {
		q.Enqueue(&concurrentTestItem{group, i})
	}
}
