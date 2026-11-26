package browser

import (
	"context"
	"errors"
	"time"

	"github.com/chromedp/chromedp"
)

var errBrowserGracefulExit = errors.New("Browser did not close gracefully; private browser files were retained")
var errBrowserExitIdentity = errors.New("Cannot verify the browser process identities; private browser files were retained")
var errBrowserExitWait = errors.New("Browser processes did not exit before the cleanup deadline; private browser files were retained")

// Retain native process identities before stopping their browser connection.
// Waiting on those handles after allocator shutdown avoids confusing parent
// exit with the completion of its owned children's file cleanup.
func closeBrowserProcesses(root context.Context, cancel, stop context.CancelFunc, graceful bool) error {
	observing, finish := context.WithTimeout(root, 2*time.Second)
	wait, err := prepareBrowserProcessExit(observing)
	finish()
	var gracefulErr error
	if graceful {
		closing, finish := context.WithTimeout(root, 5*time.Second)
		if chromedp.Cancel(closing) != nil {
			gracefulErr = errBrowserGracefulExit
		}
		finish()
	}
	cancel()
	stop()
	if wait != nil {
		closing, finish := context.WithTimeout(context.Background(), 2*time.Second)
		defer finish()
		if waitErr := wait(closing); waitErr != nil {
			return errors.Join(err, gracefulErr, waitErr)
		}
	}
	return errors.Join(err, gracefulErr)
}
