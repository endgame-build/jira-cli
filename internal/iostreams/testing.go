package iostreams

import "bytes"

// TestIOStreams wraps IOStreams with accessible buffers for test assertions.
type TestIOStreams struct {
	*IOStreams
	OutBuf *bytes.Buffer
	ErrBuf *bytes.Buffer
	InBuf  *bytes.Buffer
}

// Test creates IOStreams backed by bytes.Buffer with TTY disabled and color off.
// Returns the wrapper for test assertions on captured output.
func Test() *TestIOStreams {
	inBuf := &bytes.Buffer{}
	outBuf := &bytes.Buffer{}
	errBuf := &bytes.Buffer{}

	return &TestIOStreams{
		IOStreams: &IOStreams{
			In:           inBuf,
			Out:          outBuf,
			Err:          errBuf,
			originalOut:  outBuf,
			stdoutIsTTY:  false,
			stderrIsTTY:  false,
			colorEnabled: false,
		},
		OutBuf: outBuf,
		ErrBuf: errBuf,
		InBuf:  inBuf,
	}
}
