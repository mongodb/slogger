// Copyright 2014 MongoDB, Inc.
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
//

// A threadsafe map from string to *queue.Queue

package retaining_level_filter_appender

import (
	"github.com/tolsen/slogger/v2"
	"github.com/tolsen/slogger/v2/queue"

	"fmt"
	"sync"
)

type logRetainer struct {
	logsByCategory map[string]*queue.Queue // map from categories to queues
	queueCapacity  int
	lock           sync.RWMutex
}

func newLogRetainer(queueCapacity int) *logRetainer {
	return &logRetainer{
		make(map[string]*queue.Queue),
		queueCapacity,
		sync.RWMutex{},
	}
}

func (self *logRetainer) clearLogs(category string) {
	self.lock.Lock()
	defer self.lock.Unlock()
	delete(self.logsByCategory, category)
}

func (self *logRetainer) logsQueue(category string, createIfAbsent bool) *queue.Queue {
	isReadLocked := true
	self.lock.RLock()
	defer func() {
		if isReadLocked {
			self.lock.RUnlock()
		}
	}()

	q, found := self.logsByCategory[category]

	if !found && createIfAbsent {
		q = queue.New(self.queueCapacity, nil)
		// escalate to write lock and add q to logsByCategory
		self.lock.RUnlock()
		isReadLocked = false
		self.lock.Lock()
		defer self.lock.Unlock()
		// check again as we lost all locks briefly
		q2, found := self.logsByCategory[category]
		if found {
			q = q2
		} else {
			self.logsByCategory[category] = q
		}
	}

	// Warning if you are adding new code here.  We may have either a
	// read lock or a write lock at this point.
	return q
}

func (self *logRetainer) retainLog(log *slogger.Log, category string) {
	self.logsQueue(category, true).Enqueue(log)
}

func (self *logRetainer) sendLogsToAppender(appender slogger.Appender, category string) []error {
	errs := make([]error, 0)

	logsQ := self.logsQueue(category, false)
	if logsQ == nil {
		// nothing to send
		return errs
	}

	// snapshot the length at the beginning of the for loop
	for logsLen := logsQ.Len(); logsLen > 0; logsLen-- {
		logInterface, err := logsQ.Dequeue()
		if err != nil {
			if _, ok := err.(queue.UnderflowError); !ok {
				// we somehow underflowed the queue.  we are done
				return errs
			}

			// Dequeue should only ever return UnderflowError but
			// might as well handle the case where it doesn't
			errs = append(errs, err)
			continue
		}

		log, ok := logInterface.(*slogger.Log)
		if !ok {
			errs = append(errs, fmt.Errorf("Not a log: %#v (%T)", logInterface, logInterface))
			continue
		}

		err = appender.Append(log)
		if err != nil {
			errs = append(errs, err)
		}
	}

	return errs
}
