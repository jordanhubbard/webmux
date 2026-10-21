package httpapi

import (
	"context"
	"os"
	"time"

	"github.com/jordanhubbard/webmux/server/internal/openrigcoord"
)

// WebMux supervises the optional coordinator while it is serving. Detached tmux
// agents survive WebMux updates; an existing daemon and unrelated rigs are reused.
func (s *Server) startCoordination() {
	if os.Getenv("WEBMUX_COORDINATION_DISABLED") == "1" {
		return
	}
	s.coordinationOnce.Do(func() {
		ctx, cancel := context.WithCancel(context.Background())
		s.coordinationCancel = cancel
		go func() {
			ticker := time.NewTicker(30 * time.Second)
			defer ticker.Stop()
			for {
				if executable, _ := openRigExecutable(); executable != "" {
					check, finish := context.WithTimeout(ctx, 25*time.Second)
					_, err := openrigcoord.Run(check, s.store.Home, "service", "ensure", "")
					finish()
					if err != nil && ctx.Err() == nil {
						s.logger.Warn("OpenRig coordinator unavailable", "error", err)
					}
				}
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
				}
			}
		}()
	})
}
