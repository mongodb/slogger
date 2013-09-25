package test_util

import "testing"

func AssertNoErrors(test *testing.T, errs []error) {
	if len(errs) != 0 {
		test.Errorf("Expected to be empty: %v", errs)
	}
}

