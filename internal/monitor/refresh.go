package monitor

import (
	"errors"

	"dnsmonitor/internal/model"
)

// QueueAll starts one bounded batch over the enabled server snapshot. A waiting
// batch member is separate from the single-server queue, so a server already in
// flight receives another complete round, and batches are not capped at 256.
func (m *Monitor) QueueAll() (model.RefreshStatus, error) {
	cfg, err := m.st.GetConfig()
	if err != nil {
		return model.RefreshStatus{}, err
	}
	if cfg.Concurrency == 0 {
		return model.RefreshStatus{}, errors.New("探测已暂停（并发为 0），请先恢复探测")
	}
	servers, err := m.st.ListServers()
	if err != nil {
		return model.RefreshStatus{}, err
	}
	m.mu.Lock()
	if m.status.Refresh != nil && m.status.Refresh.Pending {
		status := *m.status.Refresh
		m.mu.Unlock()
		return status, nil
	}
	m.batch = make(map[int64]bool, len(servers))
	for _, server := range servers {
		if server.Enabled {
			m.batch[server.ID] = false
		}
	}
	m.batchSequence++
	m.status.Refresh = &model.RefreshStatus{ID: m.batchSequence, Total: len(m.batch), Pending: len(m.batch) > 0}
	// A probe started before this request must not refill the reference cache.
	m.cacheGeneration++
	m.cache = make(map[string]cachedProbe)
	for _, state := range m.referenceCollections {
		state.nextAt = 0
	}
	status := *m.status.Refresh
	m.mu.Unlock()
	m.Wake()
	return status, nil
}

func (m *Monitor) batchWaitingLocked(serverID int64) bool {
	started, exists := m.batch[serverID]
	return exists && !started
}

func (m *Monitor) updateBatchLocked(valid map[int64]bool, paused bool) {
	if m.status.Refresh == nil || !m.status.Refresh.Pending {
		return
	}
	for id := range m.batch {
		if !valid[id] {
			delete(m.batch, id)
			m.status.Refresh.Completed++
			m.status.Refresh.Error = "部分服务器已禁用或删除，已跳过"
		}
	}
	if len(m.batch) == 0 {
		m.status.Refresh.Pending = false
	} else if paused {
		m.cancelBatchLocked("探测已暂停，本轮刷新未完成；恢复探测后可重新点击刷新")
	}
}

func (m *Monitor) completeBatchLocked(c completion) {
	if c.refreshID == 0 || m.status.Refresh == nil || !m.status.Refresh.Pending || c.refreshID != m.status.Refresh.ID {
		return
	}
	if started, exists := m.batch[c.serverID]; exists && started {
		delete(m.batch, c.serverID)
		m.status.Refresh.Completed++
		if c.error != "" {
			m.status.Refresh.Error = c.error
		}
		m.status.Refresh.Pending = len(m.batch) > 0
	}
}

func (m *Monitor) cancelBatchLocked(reason string) {
	if m.status.Refresh != nil && m.status.Refresh.Pending {
		m.status.Refresh.Pending = false
		m.status.Refresh.Error = reason
		clear(m.batch)
	}
}
