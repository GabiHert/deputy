package backend

import (
	"context"
	"sync"
	"time"

	"github.com/GabiHert/positron/internal/app"
)

var mcpCache struct {
	sync.Mutex
	at   time.Time
	list []app.MCPServerDTO
}

// ListMCPServers runs claude mcp list, at most every two minutes unless refresh is set.
func (b *Backend) ListMCPServers(ctx context.Context, refresh bool) ([]app.MCPServerDTO, error) {
	mcpCache.Lock()
	defer mcpCache.Unlock()
	if !refresh && mcpCache.list != nil && time.Since(mcpCache.at) < 2*time.Minute {
		return mcpCache.list, nil
	}
	servers, err := b.d.Runner.ListMCP(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]app.MCPServerDTO, 0, len(servers))
	for _, s := range servers {
		out = append(out, app.MCPServerDTO{Name: s.Name, Target: s.Target, Status: s.Status})
	}
	mcpCache.list, mcpCache.at = out, time.Now()
	return out, nil
}
