// +build !linux,!darwin

package subprocess

import (
	"context"
)

// placeholders for windows tests

func (o *OOMTracker) Clear(ctx context.Context) error {
	return nil
}

func (o *OOMTracker) Check(ctx context.Context) error {
	return nil
}
