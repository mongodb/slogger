// Copyright 2013, 2016 MongoDB, Inc.
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

import "sync"

type Context struct {
	fields map[string]interface{}
	lock   sync.RWMutex
}

func NewContext() *Context {
	c := new(Context)
	c.fields = make(map[string]interface{}, 0)
	c.lock = sync.RWMutex{}
	return c
}

func (c *Context) Add(key string, value interface{}) {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.fields[key] = value
}

func (c *Context) Get(key string) (value interface{}, found bool) {
	c.lock.RLock()
	defer c.lock.RUnlock()
	value, found = c.fields[key]
	return
}

func (c *Context) Keys() []string {
	c.lock.RLock()
	defer c.lock.RUnlock()
	keys := make([]string, len(c.fields))
	i := 0
	for k, _ := range c.fields {
		keys[i] = k
		i++
	}
	return keys
}

func (c *Context) Len() int {
	c.lock.RLock()
	defer c.lock.RUnlock()
	return len(c.fields)
}

func (c *Context) Remove(key string) {
	c.lock.Lock()
	defer c.lock.Unlock()
	delete(c.fields, key)
}
