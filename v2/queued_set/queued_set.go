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
	"github.com/tolsen/slogger/v2/queue"
	"sync"
)

type QueuedSet struct {
	q *queue.Queue
	set map[interface{}]int
	lock sync.RWMutex
}

func New(capacity int) *QueuedSet {
	qs := &QueuedSet{}
	qs.q = queue.New(capacity, qs.delete)
	qs.set = make(map[interface{}]int)
	return qs
}

// returns true iff item was not yet present
func (self *QueuedSet) Add(item interface{}) (isNew bool) {
	self.lock.Lock()
	defer self.lock.Unlock()
	count := self.set[item]
	self.q.Enqueue(item)
	self.set[item] = count + 1
	return count == 0
}

func (self *QueuedSet) Contains(item interface{}) bool {
	self.lock.RLock()
	defer self.lock.RUnlock()
	return self.set[item] != 0
}

// delete assumes the lock is already held
func (self *QueuedSet) delete(item interface{}) {
	count := self.set[item]
	if count <= 1 {
		delete(self.set, item)
	} else {
		self.set[item] = count - 1
	}
	return

}
