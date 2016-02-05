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

package retaining_level_filter_appender

import (
	"github.com/tolsen/slogger/v2/slogger"
	tu "github.com/tolsen/slogger/v2/slogger/test_util"

	"bytes"
	"strings"
	"testing"
)

func TestRetain(t *testing.T) {
	buffer := new(bytes.Buffer)
	stringAppender := slogger.NewStringAppender(buffer)
	retainingAppender := New("category", 1000, slogger.WARN, stringAppender)
	logger := &slogger.Logger{
		Prefix:    "",
		Appenders: []slogger.Appender{retainingAppender},
	}

	context1 := slogger.NewContext()
	context1.Add("category", "CATEGORY_1")

	context2 := slogger.NewContext()
	context2.Add("category", "CATEGORY_2")

	_, errs := logger.Logf(slogger.WARN, "_MESSAGE_A_")
	tu.AssertNoErrors(t, errs)
	assertBufferContains(t, buffer, "_MESSAGE_A_")

	_, errs = logger.Logf(slogger.INFO, "_MESSAGE_B_")
	tu.AssertNoErrors(t, errs)
	assertBufferDoesNotContain(t, buffer, "_MESSAGE_B_")

	_, errs = logger.LogfWithContext(slogger.WARN, "_MESSAGE_C_", context1)
	tu.AssertNoErrors(t, errs)
	assertBufferContains(t, buffer, "_MESSAGE_C_")

	_, errs = logger.LogfWithContext(slogger.INFO, "_MESSAGE_D_", context1)
	tu.AssertNoErrors(t, errs)
	assertBufferDoesNotContain(t, buffer, "_MESSAGE_D_")

	_, errs = logger.LogfWithContext(slogger.INFO, "_MESSAGE_E_", context2)
	tu.AssertNoErrors(t, errs)
	assertBufferDoesNotContain(t, buffer, "_MESSAGE_E_")

	errs = retainingAppender.AppendRetainedLogs("CATEGORY_1")
	tu.AssertNoErrors(t, errs)
	assertBufferContains(t, buffer, "_MESSAGE_D_")
	assertBufferDoesNotContain(t, buffer, "_MESSAGE_E_")

	errs = retainingAppender.AppendRetainedLogs("CATEGORY_2")
	tu.AssertNoErrors(t, errs)
	assertBufferContains(t, buffer, "_MESSAGE_E_")

	retainingAppender.SetLevel(slogger.INFO)

	_, errs = logger.Logf(slogger.INFO, "_MESSAGE_F_")
	tu.AssertNoErrors(t, errs)
	assertBufferContains(t, buffer, "_MESSAGE_F_")

	_, errs = logger.LogfWithContext(slogger.DEBUG, "_MESSAGE_G_", context1)
	tu.AssertNoErrors(t, errs)
	assertBufferDoesNotContain(t, buffer, "_MESSAGE_G_")

	errs = retainingAppender.AppendRetainedLogs("CATEGORY_1")
	tu.AssertNoErrors(t, errs)
	assertBufferContains(t, buffer, "_MESSAGE_G_")

	retainingAppender.SetRetention(false)

	_, errs = logger.LogfWithContext(slogger.DEBUG, "_MESSAGE_H_", context1)
	tu.AssertNoErrors(t, errs)
	assertBufferDoesNotContain(t, buffer, "_MESSAGE_H_")

	errs = retainingAppender.AppendRetainedLogs("CATEGORY_1")
	tu.AssertNoErrors(t, errs)
	assertBufferDoesNotContain(t, buffer, "_MESSAGE_H_")
}

func assertBufferContains(t *testing.T, buffer *bytes.Buffer, str string) {
	bufString := buffer.String()
	if !strings.Contains(bufString, str) {
		t.Fatalf("Expected %v to be in:\n%v", str, bufString)
	}
}

func assertBufferDoesNotContain(t *testing.T, buffer *bytes.Buffer, str string) {
	bufString := buffer.String()
	if strings.Contains(bufString, str) {
		t.Fatalf("Expected %v to not be in:\n%v", str, bufString)
	}
}

// func assertNil(t *testing.T, x interface{}) {
// 	if x != nil {
// 		t.Fatal("Expected to be nil: %v", x)
// 	}
// }
