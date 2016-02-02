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

// A level filter appender that retains logs (regardless of log
// levels).  Upon calling AppendRetainedLogs(category), all logs
// retained for the specified category are sent through.  This is
// useful for logging prior log messages of all log levels after
// entering an error state.

package retaining_level_filter_appender

import (
	"github.com/tolsen/slogger/v2/slogger"

	"sync"
)

type RetainingLevelFilterAppender struct {
	appender     slogger.Appender
	level        slogger.Level // protected by lock
	retainedLogs *logRetainer
	categoryKey  string // key to get category from log's context
	retention    bool   // protected by lock
	lock         sync.RWMutex
}

func New(categoryKey string, capacityPerCategory int, level slogger.Level, appender slogger.Appender) *RetainingLevelFilterAppender {
	return &RetainingLevelFilterAppender{
		appender,
		level,
		newLogRetainer(capacityPerCategory),
		categoryKey,
		true,
		sync.RWMutex{},
	}
}

func (self *RetainingLevelFilterAppender) Append(log *slogger.Log) error {
	self.retainLog(log)

	if log.Level < self.Level() {
		return nil
	}

	return self.appender.Append(log)
}

func (self *RetainingLevelFilterAppender) AppendRetainedLogs(category string) []error {
	return self.retainedLogs.sendLogsToAppender(self.appender, category)
}

func (self *RetainingLevelFilterAppender) ClearRetainedLogs(category string) {
	self.retainedLogs.clearLogs(category)
}

func (self *RetainingLevelFilterAppender) Flush() error {
	return self.appender.Flush()
}

func (self *RetainingLevelFilterAppender) Level() slogger.Level {
	self.lock.RLock()
	defer self.lock.RUnlock()
	return self.level
}

func (self *RetainingLevelFilterAppender) Retention() bool {
	self.lock.RLock()
	defer self.lock.RUnlock()
	return self.retention
}

func (self *RetainingLevelFilterAppender) SetLevel(level slogger.Level) {
	self.lock.Lock()
	defer self.lock.Unlock()
	self.level = level
}

func (self *RetainingLevelFilterAppender) SetRetention(retention bool) {
	self.lock.Lock()
	defer self.lock.Unlock()
	self.retention = retention
}

func (self *RetainingLevelFilterAppender) retainLog(log *slogger.Log) {
	if log.Context == nil || !self.Retention() {
		return
	}

	categoryInterface, found := log.Context.Get(self.categoryKey)
	if !found {
		return
	}

	category, ok := categoryInterface.(string)
	if !ok {
		// do not retain log if category is not a string
		return
	}

	self.retainedLogs.retainLog(log, category)
}
