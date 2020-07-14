// +build !linux,!darwin

package jasper

import (
	"context"
)

// placeholders for windows tests

func (o *oomTrackerImpl) Clear(ctx context.Context) error {
	return nil
}

func (o *oomTrackerImpl) Check(ctx context.Context) error {
	return nil
}
