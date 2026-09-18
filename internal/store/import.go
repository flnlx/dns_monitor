package store

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"dnsmonitor/internal/model"
)

// ErrImportConflict asks the caller to refresh a preview after any server edit.
var ErrImportConflict = errors.New("服务器列表已变化，请重新预览 CSV 后导入")

// ServerSnapshot fingerprints all persisted server fields without changing the caller's order.
func ServerSnapshot(servers []model.Server) string {
	ordered := append([]model.Server{}, servers...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].ID < ordered[j].ID })
	encoded, _ := json.Marshal(ordered) // Server contains only JSON-safe primitive fields.
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

// ImportServers applies a validated preview atomically. The HTTP layer supplies canonical
// address keys and resolves existing IDs; this package deliberately has no monitor dependency.
func (s *Store) ImportServers(snapshot string, items []model.ServerImportItem) (model.ServerImportResult, error) {
	var result model.ServerImportResult
	tx, err := s.db.Begin()
	if err != nil {
		return result, err
	}
	defer tx.Rollback()
	rows, err := tx.Query("SELECT " + serverColumns + " FROM servers ORDER BY id")
	if err != nil {
		return result, err
	}
	current := make([]model.Server, 0)
	for rows.Next() {
		server, e := scanServer(rows)
		if e != nil {
			rows.Close()
			return result, e
		}
		current = append(current, server)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return result, err
	}
	if ServerSnapshot(current) != snapshot {
		return result, ErrImportConflict
	}
	byID := make(map[int64]model.Server, len(current))
	byKey := make(map[string]int64, len(current)+len(items))
	addKey := func(key string, id int64) {
		if previous, exists := byKey[key]; exists && previous != id {
			byKey[key] = 0
		} else {
			byKey[key] = id
		}
	}
	for _, server := range current {
		byID[server.ID] = server
		addKey(server.Address, server.ID)
	}
	// Canonical keys can collapse default ports and DNS stamps. Prepared existing IDs
	// provide that mapping; comparing these keys with stored address text would be wrong.
	for _, item := range items {
		if item.Server.ID > 0 {
			if _, exists := byID[item.Server.ID]; !exists {
				return model.ServerImportResult{}, fmt.Errorf("CSV 第 %d 行的覆盖目标不存在", item.Row)
			}
			if item.Key != "" {
				addKey(item.Key, item.Server.ID)
			}
		}
	}
	createdAt := time.Now().UnixMilli()
	for _, item := range items {
		if item.Action == "skip" {
			result.Skipped++
			continue
		}
		if item.Key == "" {
			return model.ServerImportResult{}, fmt.Errorf("CSV 第 %d 行缺少地址标识", item.Row)
		}
		server := item.Server
		switch item.Action {
		case "create":
			if server.ID != 0 {
				return model.ServerImportResult{}, fmt.Errorf("CSV 第 %d 行不能用新增操作覆盖现有服务器", item.Row)
			}
			if _, exists := byKey[item.Key]; exists {
				return model.ServerImportResult{}, fmt.Errorf("CSV 第 %d 行地址重复，请选择覆盖或跳过", item.Row)
			}
			server.CreatedAt = createdAt
			insert, e := tx.Exec("INSERT INTO servers(name,provider,address,protocol,enabled,trusted,notes,created_at) VALUES(?,?,?,?,?,?,?,?)", server.Name, server.Provider, server.Address, server.Protocol, server.Enabled, server.Trusted, server.Notes, server.CreatedAt)
			if e != nil {
				return model.ServerImportResult{}, e
			}
			server.ID, e = insert.LastInsertId()
			if e != nil {
				return model.ServerImportResult{}, e
			}
			result.Created++
		case "overwrite":
			if server.ID == 0 {
				server.ID = byKey[item.Key]
			}
			existing, exists := byID[server.ID]
			if !exists || server.ID == 0 {
				return model.ServerImportResult{}, fmt.Errorf("CSV 第 %d 行找不到唯一覆盖目标", item.Row)
			}
			server.CreatedAt = existing.CreatedAt
			update, e := tx.Exec("UPDATE servers SET name=?,provider=?,address=?,protocol=?,enabled=?,trusted=?,notes=? WHERE id=?", server.Name, server.Provider, server.Address, server.Protocol, server.Enabled, server.Trusted, server.Notes, server.ID)
			if e != nil {
				return model.ServerImportResult{}, e
			}
			changed, e := update.RowsAffected()
			if e != nil {
				return model.ServerImportResult{}, e
			}
			if changed != 1 {
				return model.ServerImportResult{}, fmt.Errorf("CSV 第 %d 行的覆盖目标不存在", item.Row)
			}
			result.Updated++
		default:
			return model.ServerImportResult{}, fmt.Errorf("CSV 第 %d 行导入操作无效", item.Row)
		}
		byID[server.ID] = server
		byKey[item.Key] = server.ID
	}
	if err = tx.Commit(); err != nil {
		return model.ServerImportResult{}, err
	}
	if result.Created > 0 || result.Updated > 0 {
		s.version.Add(1)
	}
	return result, nil
}
