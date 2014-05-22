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



