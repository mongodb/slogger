// Copyright 2013, 2015 MongoDB, Inc.
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

// Package async_appender provides a slogger Appender that supports
// asynchronous logging

package async_appender

import "github.com/mongodb/slogger/v2/slogger"

type AsyncAppender struct {
	Appender   slogger.Appender
	appendCh   chan *slogger.Log
	flushCh    chan (chan bool)
	errHandler func(error)
}

func New(appender slogger.Appender, channelCapacity int, errHandler func(error)) *AsyncAppender {
	asyncAppender := &AsyncAppender{
		Appender:   appender,
		appendCh:   make(chan *slogger.Log, channelCapacity),
		flushCh:    make(chan (chan bool)),
		errHandler: errHandler,
	}

	go asyncAppender.listenForAppends()

	return asyncAppender
}

func (self *AsyncAppender) Append(log *slogger.Log) error {
	select {
	case self.appendCh <- log:
		// nothing else to do
	default:
		// channel is full. log a warning
		self.appendCh <- self.fullWarningLog()
		self.appendCh <- log
	}
	return nil
}

func (self *AsyncAppender) Flush() error {
	replyCh := make(chan bool)
	self.flushCh <- replyCh
	for !(<-replyCh) {
		self.flushCh <- replyCh
	}
	return nil
}

func (self *AsyncAppender) appendToSubAppender(log *slogger.Log) {
	if err := self.Appender.Append(log); err != nil && self.errHandler != nil {
		self.errHandler(err)
	}
}

func (self *AsyncAppender) fullWarningLog() *slogger.Log {
	return internalWarningLog(
		"This AsyncAppender's append channel is full. The channelCapacity is %d.  You may want to increase it next time.",
		cap(self.appendCh),
	)
}

func internalWarningLog(messageFmt string, args ...interface{}) *slogger.Log {
	return slogger.SimpleLog("AsyncAppender", slogger.WARN, slogger.NoErrorCode, 3, messageFmt, args...)
}

// listenForAppends consumes appendCh and flushCh.  It consumes Logs
// coming down the appendCh, flushing the underlying Appender when
// necessary and the appendCh is empty.  It will reply to flushCh
// messages (via the given flushReplyCh) after flushing (or if nothing
// has ever been logged), increasing the chance that it will be able
// to reply true.
func (self *AsyncAppender) listenForAppends() {
	needsFlush := false
	for {
		if needsFlush {
			select {
			case log := <-self.appendCh:
				self.appendToSubAppender(log)
			default:
				self.Appender.Flush()
				needsFlush = false
			}
		} else {
			select {
			case log := <-self.appendCh:
				self.appendToSubAppender(log)
				needsFlush = true
			case flushReplyCh := <-self.flushCh:
				flushReplyCh <- (len(self.appendCh) <= 0)
			}
		}
	}
}
