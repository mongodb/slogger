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
	"fmt"
	"regexp"
)

type Context struct {
	fields map[string]interface{}
}

func NewContext() *Context {
	c := new(Context)
	c.fields = make(map[string]interface{}, 0)
	return c
}

func (c *Context) Add(key string, value interface{}) {
	c.fields[key] = value
}

func (c *Context) Get(key string) (value interface{}, found bool) {
	value, found = c.fields[key]
	return
}

func (c *Context) Keys() []string {
	keys := make([]string, len(c.fields))
	i := 0
	for k, _ := range c.fields {
		keys[i] = k
		i++
	}
	return keys
}

func (c *Context) Len() int {
	return len(c.fields)
}

func (c *Context) Remove(key string) {
	delete(c.fields, key)
}

var contextInterpolateRx *regexp.Regexp = regexp.MustCompile("\\{[^}]*\\}")

func (c *Context) interpolateString(str string) string {
	replacer := func(s string) string {
		key := s[1 : len(s)-1] // trim off curly braces
		val, found := c.fields[key]
		if found {
			return fmt.Sprint(val)
		}
		return fmt.Sprint(nil)
	}

	return contextInterpolateRx.ReplaceAllStringFunc(str, replacer)
}
