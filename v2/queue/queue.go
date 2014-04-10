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

// A thread-safe fixed-capacity queue.  Deletes last item on overflow

package queue

type Queue struct {
	items           chan interface{}
	onForcedDequeue func(interface{})
}

func New(capacity int, onForcedDequeue func(interface{})) *Queue {
	return &Queue{
		make(chan interface{}, capacity),
		onForcedDequeue,
	}
}

func (q *Queue) Cap() int {
	return cap(q.items)
}

func (q *Queue) Dequeue() (interface{}, error) {
	select {
	case item := <-q.items:
		return item, nil
	default:
		return nil, UnderflowError{}
	}
}

func (q *Queue) Enqueue(item interface{}) {
	for {
		select {
		case q.items <- item:
			return
		default:
			// q.items must be full
			// force a dequeue in order to make room
			select {
			case item := <-q.items:
				if q.onForcedDequeue != nil {
					q.onForcedDequeue(item)
				}
			default:
				// wow, we went from full to empty very fast.  let's start over
			}
		}
	}
}

func (q *Queue) IsEmpty() bool {
	return q.Len() == 0
}

func (q *Queue) IsFull() bool {
	return q.Len() == q.Cap()
}

func (q *Queue) Len() int {
	return len(q.items)
}

type UnderflowError struct {
}

func (UnderflowError) Error() string {
	return "Queue underflowed"
}
