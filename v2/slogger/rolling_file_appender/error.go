package rolling_file_appender

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
