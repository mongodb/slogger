package rolling_file_appender

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// state is serialized to and deserialized from disk.  Changes to this
// structure should be done in a backward-compatible fashion.  If new
// fields are added here then they will be read in as the zero value
// when reading in older versions of the state file.  Existing fields'
// types should not be changed as that will break reading in older
// versions of the state file.
type state struct {
	LogStartTime time.Time `json:"logStartTime"`
}

func newState(logStartTime time.Time) *state {
	return &state{logStartTime}
}

func readState(path string) (*state, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, OpenError{path, err}
	}
	defer file.Close()

	var decodedState *state
	decoder := json.NewDecoder(file)
	if err = decoder.Decode(&decodedState); err != nil {
		return nil, DecodeError{path, err}
	}

	return decodedState, nil
}

func stateExists(path string) (bool, error) {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}

		return false, StatError{path, err}
	}

	return true, nil
}

func (self *state) write(path string) error {
	file, err := createHidden(path)
	if err != nil {
		return OpenError{path, err}
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	if err = encoder.Encode(self); err != nil {
		return EncodeError{path, err}
	}

	return nil
}

func (self *RollingFileAppender) statePath() string {
	newBase := ".slogger-state-" + filepath.Base(self.absPath)
	return filepath.Join(filepath.Dir(self.absPath), newBase)
}
