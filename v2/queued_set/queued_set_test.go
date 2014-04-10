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

package queued_set

import (
	//	"fmt"
	"runtime"
	"sync"
	"testing"
)

func TestNew(t *testing.T) {
	var _ *QueuedSet = New(10)
}

func TestAddContains(t *testing.T) {
	qs := New(2)

	if qs.Contains("Hello") {
		t.Error("queued set should not contain \"Hello\" yet")
	}

	if qs.Contains("World") {
		t.Error("queued set should not contain \"World\" yet")
	}

	isNew := qs.Add("Hello")
	if !isNew {
		t.Error("isNew should have been true")
	}

	if !qs.Contains("Hello") {
		t.Error("queued set should now contain \"Hello\"")
	}

	if qs.Contains("World") {
		t.Error("queued set should not contain \"World\" yet")
	}

	isNew = qs.Add("World")
	if !isNew {
		t.Error("isNew should have been true")
	}

	if !qs.Contains("Hello") {
		t.Error("queued set should still contain \"Hello\"")
	}

	if !qs.Contains("World") {
		t.Error("queued set should now contain \"World\"")
	}

	isNew = qs.Add("Hello")
	if isNew {
		t.Error("isNew should be false now")
	}

	if !qs.Contains("Hello") {
		t.Error("queued set should still contain \"Hello\"")
	}

	if !qs.Contains("World") {
		t.Error("queued set should still contain \"World\"")
	}

	isNew = qs.Add("Bonjour")
	if !isNew {
		t.Error("isNew should have been true")
	}

	if !qs.Contains("Hello") {
		t.Error("queued set should still contain \"Hello\"")
	}

	if qs.Contains("World") {
		t.Error("queued set should no longer contain \"World\"")
	}

	if !qs.Contains("Bonjour") {
		t.Error("queued set should now contain \"Bonjour\"")
	}
}

func TestConcurrentAdd(t *testing.T) {
	runtime.GOMAXPROCS(runtime.NumCPU())

	num_groups, items_per_group, capacity := 10, 1000, 100

	qs := New(capacity)

	var wg sync.WaitGroup
	for i := 0; i < num_groups; i++ {
		wg.Add(1)
		go addGroup(qs, i, items_per_group, &wg)
	}

	wg.Wait()

	last_seen_ints := make([]int, num_groups)

	// check that it's sorted within groups
	for i := 0; i < capacity; i++ {
		item, err := qs.q.Dequeue()
		if err != nil {
			t.Fatalf("Error while dequeueing: %v", err)
		}

		concurrentItem, ok := item.(*concurrentTestItem)

		if !ok {
			t.Fatalf("Only concurrentTestItem's should be in this queue")
		}

		if last_seen_ints[concurrentItem.group] == 0 {
			last_seen_ints[concurrentItem.group] = concurrentItem.seq
		} else if last_seen_ints[concurrentItem.group] != concurrentItem.seq {
			t.Fatalf("last_seen_ints[item.group] should equal item.seq")
		}

		last_seen_ints[concurrentItem.group]++
	}
}

type concurrentTestItem struct {
	group int
	seq   int
}

func addGroup(qs *QueuedSet, group int, n int, wg *sync.WaitGroup) {
	defer wg.Done()
	for i := 0; i < n; i++ {
		qs.Add(&concurrentTestItem{group, i})
	}
}
