// +build !windows

package rolling_file_appender

import (
	"os"
)

func createHidden(path string) (*os.File, error) {
	return os.Create(path)
}
