package errors

import (
	"fmt"

	"golang.org/x/xerrors"
)

var (
	ErrArgument               = New("error argument")
	ErrEmptyRegistry          = New("registry is empty")
	ErrDegradePass            = New("degrade with pass")
	ErrInvalidConfig          = New("invalid format of default config")
	ErrInvalidCache           = New("invalid format of cache, it could be [FormatConsul|FormatDiscovery]")
	ErrInvalidService         = New("invalid service config")
	ErrNotFound               = New("not found")
	ErrNilConfig              = New("nil config")
	ErrBalancerNotImplemented = New("algorithm has not implemented")
	ErrClosed                 = New("registry is closed. please recreate")
)

type wrapError struct {
	msg   string
	err   error
	frame xerrors.Frame
}

func (e *wrapError) Error() string {
	return fmt.Sprint(e)
}

func (e *wrapError) Format(s fmt.State, v rune) { xerrors.FormatError(e, s, v) }

func (e *wrapError) FormatError(p xerrors.Printer) (next error) {
	p.Print(e.msg)
	e.frame.Format(p)
	return e.err
}

func (e *wrapError) Unwrap() error {
	return e.err
}

func New(err string) error {
	return xerrors.New(err)
}

func Wrap(err error) error {
	return &wrapError{
		err:   err,
		frame: xerrors.Caller(1),
	}
}

func UnWrap(err error) error {
	return xerrors.Unwrap(err)
}

func Errorf(format string, v ...interface{}) error {
	return xerrors.Errorf(format, v...)
}

func Is(err, target error) bool {
	return xerrors.Is(err, target)
}
