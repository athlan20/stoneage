package agentmcp

// 文档修订号提醒（STAGE-300，和 docs/agent-connect.py 的 STAGE-188 行为一致）：
// hello 返回的 docs_rev 和上次记下的不一样时，下一次工具结果带 docs_updated，提示读 stoneage://docs/changes。
// 记录和 agent-connect.py 共用 ~/.stoneage-agent/docs-rev.json（环境变量 STONEAGE_AGENT_STATE 可改）。

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
)

// defaultStatePath 返回 docs_rev 记录文件的默认位置。
func defaultStatePath() string {
	if p := os.Getenv("STONEAGE_AGENT_STATE"); p != "" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".stoneage-agent", "docs-rev.json")
	}
	return filepath.Join(home, ".stoneage-agent", "docs-rev.json")
}

// docKey 是记录里的键：WebSocket 用主机名（和 agent-connect.py 一样），TCP 用地址本身。
func docKey(server string) string {
	if isWebSocket(server) {
		if u, err := url.Parse(server); err == nil && u.Host != "" {
			return u.Host
		}
	}
	return server
}

// checkDocs 比对 hello 的 docs_rev：变了（且上次有记录）就留下 docs_updated 提示，然后把新值记下来。
func (s *Session) checkDocs(rev string) {
	key := docKey(s.cfg.Server)
	state := loadDocsState(s.cfg.StatePath)
	last, seen := state[key]
	if seen && last == rev {
		return
	}
	if seen {
		s.mu.Lock()
		s.docsNotice = fmt.Sprintf("文档已更新：docs_rev %s → %s。先读 stoneage://docs/changes 里新增的条目（每条标题带 docs_rev），再读相关章节，然后继续行动。", last, rev)
		s.mu.Unlock()
	}
	state[key] = rev
	if err := saveDocsState(s.cfg.StatePath, state); err != nil {
		s.log.note(fmt.Sprintf("无法记录文档版本到 %s：%v", s.cfg.StatePath, err))
	}
}

// takeDocsNotice 取出并清空待提示的 docs_updated。
func (s *Session) takeDocsNotice() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := s.docsNotice
	s.docsNotice = ""
	return n
}

// loadDocsState 读记录文件；读不到或格式不对时当作空记录。
func loadDocsState(path string) map[string]string {
	state := map[string]string{}
	raw, err := os.ReadFile(path)
	if err != nil {
		return state
	}
	_ = json.Unmarshal(raw, &state)
	if state == nil {
		state = map[string]string{}
	}
	return state
}

// saveDocsState 先写临时文件再替换，避免写一半留下坏文件。
func saveDocsState(path string, state map[string]string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	raw, err := json.Marshal(state)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
