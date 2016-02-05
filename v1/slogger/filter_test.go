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

package slogger

import (
	"testing"
)

func TestTurboFilterLevels(test *testing.T) {
	var filter TurboFilter
	filter = TurboLevelFilter(DEBUG)
	if filter(INFO, "Evaluation should continue") == false {
		test.Errorf("Expected greater level to continue evaluation")
	}

	filter = TurboLevelFilter(INFO)
	if filter(INFO, "Evaluation should continue") == false {
		test.Errorf("Expected equal level to continue evaluation")
	}

	filter = TurboLevelFilter(WARN)
	if filter(INFO, "Evaluation should %v", "halt") == true {
		test.Errorf("Expected lesser level to halt evaluation")
	}
}
