package rolling_file_appender

import (
	"fmt"
)

type CloseError struct {
	Filename string
	Err      error
}

func (self CloseError) Error() string {
	return fmt.Sprintf(
		"rolling_file_appender: Failed to close %s: %s",
		self.Filename,
		self.Err.Error(),
	)
}

func IsCloseError(err error) bool {
	_, ok := err.(CloseError)
	return ok
}

type MinorRotationError struct {
	Err error
}

func (self MinorRotationError) Error() string {
	return ("rolling_file_appender: minor error while rotating logs: " + self.Err.Error())
}

func IsMinorRotationError(err error) bool {
	_, ok := err.(MinorRotationError)
	return ok
}

type NoFileError struct{}

func (NoFileError) Error() string {
	return "rolling_file_appender: No log file to write to"
}

func IsNoFileError(err error) bool {
	_, ok := err.(NoFileError)
	return ok
}

type OpenError struct {
	Filename string
	Err      error
}

func (self OpenError) Error() string {
	return fmt.Sprintf(
		"rolling_file_appender: Failed to open %s: %s",
		self.Filename,
		self.Err.Error(),
	)
}

func IsOpenError(err error) bool {
	_, ok := err.(OpenError)
	return ok
}

type RenameError struct {
	OldFilename string
	NewFilename string
	Err         error
}

func (self RenameError) Error() string {
	return fmt.Sprintf(
		"rolling_file_appender: Failed to rename %s to %s: %s",
		self.OldFilename,
		self.NewFilename,
		self.Err.Error(),
	)
}

func IsRenameError(err error) bool {
	_, ok := err.(RenameError)
	return ok
}

type WriteError struct {
	Filename string
	Err      error
}

func (self WriteError) Error() string {
	return fmt.Sprintf(
		"rolling_file_appender: Failed to write to %s: %s",
		self.Filename,
		self.Err.Error(),
	)
}

func IsWriteError(err error) bool {
	_, ok := err.(WriteError)
	return ok
}

type EncodeError struct {
	Filename string
	Err      error
}

func (self EncodeError) Error() string {
	return fmt.Sprintf(
		"rolling_file_appender: Failed to encode state to %s: %s",
		self.Filename,
		self.Err.Error(),
	)
}

func IsEncodeError(err error) bool {
	_, ok := err.(EncodeError)
	return ok
}

type DecodeError struct {
	Filename string
	Err      error
}

func (self DecodeError) Error() string {
	return fmt.Sprintf(
		"rolling_file_appender: Failed to decode state from %s: %s",
		self.Filename,
		self.Err.Error(),
	)
}

func IsDecodeError(err error) bool {
	_, ok := err.(DecodeError)
	return ok
}

type StatError struct {
	Filename string
	Err      error
}

func (self StatError) Error() string {
	return fmt.Sprintf(
		"rolling_file_appender: Failed to stat %s: %s",
		self.Filename,
		self.Err.Error(),
	)
}

func IsStatError(err error) bool {
	_, ok := err.(StatError)
	return ok
}
