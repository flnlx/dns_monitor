package httpapi

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"dnsmonitor/internal/model"
	"dnsmonitor/internal/monitor"
	"dnsmonitor/internal/store"
)

var serverCSVHeader = []string{"name", "provider", "address", "enabled", "trusted", "notes"}

type importPreviewRow struct {
	Row            int           `json:"row"`
	Server         model.Server  `json:"server"`
	Key            string        `json:"key"`
	Duplicate      bool          `json:"duplicate"`
	Existing       *model.Server `json:"existing"`
	DuplicateOfRow int           `json:"duplicate_of_row,omitempty"`
	Error          string        `json:"error,omitempty"`
}
type importPreview struct {
	Snapshot string             `json:"snapshot"`
	Rows     []importPreviewRow `json:"rows"`
	Errors   int                `json:"errors"`
}
type importDecision struct {
	Row    int    `json:"row"`
	Action string `json:"action"`
}
type importRequest struct {
	CSV       string           `json:"csv"`
	Snapshot  string           `json:"snapshot"`
	Decisions []importDecision `json:"decisions"`
}

func normalizeServerInput(server *model.Server) error {
	server.Name = strings.TrimSpace(server.Name)
	server.Provider = strings.TrimSpace(server.Provider)
	server.Notes = strings.TrimSpace(server.Notes)
	if !utf8.ValidString(server.Name+server.Provider+server.Notes) || len(server.Name) == 0 || len(server.Name) > 160 || len(server.Provider) > 160 || len(server.Notes) > 2000 {
		return errors.New("名称必填且最多 160 字节，供应商最多 160 字节，备注最多 2000 字节；请使用 UTF-8")
	}
	address, protocol, err := monitor.ValidateAddress(server.Address)
	if err != nil {
		return err
	}
	server.Address, server.Protocol = address, protocol
	return nil
}

func parseCSVBool(value string, fallback bool) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "":
		return fallback, nil
	case "true", "1", "yes", "是":
		return true, nil
	case "false", "0", "no", "否":
		return false, nil
	default:
		return false, fmt.Errorf("布尔字段应填写 true/false 或 1/0，收到 %q", value)
	}
}

func prepareImport(raw string, servers []model.Server) (importPreview, error) {
	p := importPreview{Snapshot: store.ServerSnapshot(servers), Rows: []importPreviewRow{}}
	if !utf8.ValidString(raw) {
		return p, errors.New("CSV 必须使用 UTF-8 编码（可带 BOM）")
	}
	if len(raw) > 900*1024 {
		return p, errors.New("CSV 不得超过 900 KiB")
	}
	r := csv.NewReader(strings.NewReader(strings.TrimPrefix(raw, "\ufeff")))
	r.FieldsPerRecord = -1
	header, err := r.Read()
	if err != nil {
		return p, errors.New("CSV 缺少有效表头，请下载模板")
	}
	aliases := map[string]string{"名称": "name", "供应商": "provider", "地址": "address", "dns地址": "address", "启用": "enabled", "可信": "trusted", "备注": "notes"}
	columns := map[string]int{}
	for i, v := range header {
		v = strings.ToLower(strings.TrimSpace(v))
		if alias, ok := aliases[v]; ok {
			v = alias
		}
		switch v {
		case "name", "provider", "address", "enabled", "trusted", "notes":
		default:
			return p, fmt.Errorf("不支持 CSV 字段 %q，请按模板填写", v)
		}
		if _, exists := columns[v]; exists {
			return p, fmt.Errorf("CSV 表头包含重复字段 %q", v)
		}
		columns[v] = i
	}
	for _, required := range []string{"name", "address"} {
		if _, ok := columns[required]; !ok {
			return p, fmt.Errorf("CSV 缺少 %s 列", required)
		}
	}
	byKey := map[string][]model.Server{}
	for _, server := range servers {
		key, e := monitor.AddressKey(server.Address)
		if e != nil {
			return p, fmt.Errorf("现有服务器 %s 的地址无法规范化: %w", server.Name, e)
		}
		byKey[key] = append(byKey[key], server)
	}
	firstRow := map[string]int{}
	for rowNumber := 1; ; rowNumber++ {
		cells, e := r.Read()
		if e == io.EOF {
			break
		}
		if e != nil {
			return p, fmt.Errorf("第 %d 条 CSV 记录格式错误: %w", rowNumber, e)
		}
		if rowNumber > 1000 {
			return p, errors.New("每次最多导入 1000 台服务器，请分批导入")
		}
		row := importPreviewRow{Row: rowNumber}
		if len(cells) != len(header) {
			row.Error = "列数与表头不一致"
			p.Rows = append(p.Rows, row)
			p.Errors++
			continue
		}
		cell := func(name string) string {
			if col, ok := columns[name]; ok {
				return cells[col]
			}
			return ""
		}
		row.Server = model.Server{Name: cell("name"), Provider: cell("provider"), Address: cell("address"), Notes: cell("notes")}
		if row.Server.Enabled, e = parseCSVBool(cell("enabled"), true); e == nil {
			row.Server.Trusted, e = parseCSVBool(cell("trusted"), false)
		}
		if e == nil {
			e = normalizeServerInput(&row.Server)
		}
		if e == nil {
			row.Key, e = monitor.AddressKey(row.Server.Address)
		}
		if e != nil {
			row.Error = e.Error()
			p.Errors++
			p.Rows = append(p.Rows, row)
			continue
		}
		matches := byKey[row.Key]
		if len(matches) > 1 {
			row.Error = fmt.Sprintf("现有配置有 %d 条相同 DNS 地址，请先整理重复节点后重试", len(matches))
			p.Errors++
		} else if len(matches) == 1 {
			existing := matches[0]
			row.Existing = &existing
			row.Server.ID = existing.ID
			for _, name := range []string{"provider", "enabled", "trusted", "notes"} {
				if _, supplied := columns[name]; supplied {
					continue
				}
				switch name {
				case "provider":
					row.Server.Provider = existing.Provider
				case "enabled":
					row.Server.Enabled = existing.Enabled
				case "trusted":
					row.Server.Trusted = existing.Trusted
				case "notes":
					row.Server.Notes = existing.Notes
				}
			}
			row.Duplicate = true
		}
		if prior, ok := firstRow[row.Key]; ok {
			row.Duplicate = true
			row.DuplicateOfRow = prior
		} else {
			firstRow[row.Key] = rowNumber
		}
		p.Rows = append(p.Rows, row)
	}
	if len(p.Rows) == 0 {
		return p, errors.New("CSV 没有服务器记录")
	}
	return p, nil
}

func (s *Server) previewServerImport(w http.ResponseWriter, r *http.Request) {
	var input struct {
		CSV string `json:"csv"`
	}
	if err := decode(w, r, &input); err != nil {
		problem(w, 400, err)
		return
	}
	servers, err := s.Store.ListServers()
	if err != nil {
		problem(w, 500, err)
		return
	}
	preview, err := prepareImport(input.CSV, servers)
	if err != nil {
		problem(w, 400, err)
		return
	}
	jsonOut(w, 200, preview)
}
func (s *Server) importServers(w http.ResponseWriter, r *http.Request) {
	var input importRequest
	if err := decode(w, r, &input); err != nil {
		problem(w, 400, err)
		return
	}
	servers, err := s.Store.ListServers()
	if err != nil {
		problem(w, 500, err)
		return
	}
	if input.Snapshot == "" || input.Snapshot != store.ServerSnapshot(servers) {
		problem(w, 409, "服务器列表已变化，请重新预览后确认导入")
		return
	}
	preview, err := prepareImport(input.CSV, servers)
	if err != nil {
		problem(w, 400, err)
		return
	}
	if preview.Errors > 0 {
		problem(w, 400, "CSV 有无效记录，请先修正所有标记的错误")
		return
	}
	decisions := map[int]string{}
	for _, decision := range input.Decisions {
		if decision.Row < 1 || decision.Row > len(preview.Rows) || (decision.Action != "skip" && decision.Action != "overwrite") {
			problem(w, 400, "无效的重复记录处理选择")
			return
		}
		if _, exists := decisions[decision.Row]; exists {
			problem(w, 400, "同一记录有多个处理选择")
			return
		}
		if !preview.Rows[decision.Row-1].Duplicate {
			problem(w, 400, "只需要为重复记录选择处理方式")
			return
		}
		decisions[decision.Row] = decision.Action
	}
	items := make([]model.ServerImportItem, 0, len(preview.Rows))
	for _, row := range preview.Rows {
		action := "create"
		if row.Duplicate {
			var ok bool
			action, ok = decisions[row.Row]
			if !ok {
				problem(w, 400, fmt.Sprintf("请为第 %d 条重复记录选择跳过或覆盖", row.Row))
				return
			}
		}
		items = append(items, model.ServerImportItem{Row: row.Row, Key: row.Key, Action: action, Server: row.Server})
	}
	result, err := s.Store.ImportServers(input.Snapshot, items)
	if errors.Is(err, store.ErrImportConflict) {
		problem(w, 409, "服务器列表已变化，请重新预览后确认导入")
		return
	}
	if err != nil {
		problem(w, 400, err)
		return
	}
	s.Monitor.Wake()
	jsonOut(w, 200, result)
}

func writeServerCSV(w http.ResponseWriter, filename string, servers []model.Server) {
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename="+filename)
	_, _ = io.WriteString(w, "\ufeff")
	writer := csv.NewWriter(w)
	writer.UseCRLF = true
	_ = writer.Write(serverCSVHeader)
	for _, server := range servers {
		row := []string{server.Name, server.Provider, server.Address, strconv.FormatBool(server.Enabled), strconv.FormatBool(server.Trusted), server.Notes}
		for i := range row {
			row[i] = csvSafe(row[i])
		}
		if err := writer.Write(row); err != nil {
			return
		}
	}
	writer.Flush()
}
func (s *Server) exportServers(w http.ResponseWriter, r *http.Request) {
	servers, err := s.Store.ListServers()
	if err != nil {
		problem(w, 500, err)
		return
	}
	writeServerCSV(w, "dns-monitor-servers.csv", servers)
}
func (s *Server) serverTemplate(w http.ResponseWriter, r *http.Request) {
	writeServerCSV(w, "dns-monitor-servers-template.csv", []model.Server{{Name: "Cloudflare 示例", Provider: "Cloudflare", Address: "1.1.1.1", Enabled: true, Trusted: false, Notes: "示例行，请替换或删除；可信须由用户自行确认"}, {Name: "Google DoH 示例", Provider: "Google", Address: "https://dns.google/dns-query", Enabled: true, Trusted: false, Notes: "地址决定协议；enabled/trusted 使用 true 或 false"}})
}

func (s *Server) probeAll(w http.ResponseWriter, r *http.Request) {
	status, err := s.Monitor.QueueAll()
	if err != nil {
		problem(w, 409, err)
		return
	}
	jsonOut(w, 202, status)
}
