package jasper

import (
	"context"
	"syscall"

	"github.com/pkg/errors"
)

// SignalEvent signals the event object represented by the given name.
func SignalEvent(ctx context.Context, name string) error {
	utf16EventName, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return errors.Wrapf(err, "failed to convert event name '%s'", name)
	}

	event, err := OpenEvent(utf16EventName)
	if err != nil {
		return errors.Wrapf(err, "failed to open event '%s'", name)
	}
	defer CloseHandle(event)

	if err := SetEvent(event); err != nil {
		return errors.Wrapf(err, "failed to signal event '%s'", name)
	}

	return nil
}
