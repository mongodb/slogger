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

package slogger

import (
	"sync"
)

type LogCache struct {
	// A `LogCache` might be accessed concurrently throughout the
	// program. Therefore, the code calling `Log` acquires a mutex for
	// writing to (and potentially reading from) the
	// ring. Alternatively, if channels are more efficient, the `Log`
	// method can instead pass the *Log through a channel. A goroutine
	// on the other end can be the sole maintainer of the `LogCache`,
	// removing the need for a mutex.
	sync.Mutex
	idx   int
	items []*Log
}

var Cache LogCache

func CapLogCache(size int) {
	Cache.Lock()
	defer Cache.Unlock()

	Cache.idx = 0
	Cache.items = make([]*Log, size, size)
}

func (self *LogCache) Add(log *Log) {
	self.Lock()
	defer self.Unlock()

	if len(self.items) == 0 {
		return
	}

	self.items[self.idx] = log
	self.idx++
	if self.idx >= len(self.items) {
		self.idx = 0
	}
}

func (self *LogCache) Len() int {
	if self.items[self.idx] == nil {
		return self.idx
	}

	return len(self.items)
}

func (self *LogCache) Copy() []*Log {
	self.Lock()
	defer self.Unlock()

	var offset int
	switch {
	case self.Len() < len(self.items):
		offset = 0
	default:
		offset = self.idx
	}

	ret := make([]*Log, 0, self.Len())
	for idx := 0; idx < self.Len(); idx++ {
		accessIdx := (idx + offset) % len(self.items)
		ret = append(ret, self.items[accessIdx])
	}

	return ret
}
