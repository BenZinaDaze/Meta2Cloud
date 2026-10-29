package webui

import (
	"os"
	"testing"
)

// 运行中配置文件被改坏（如手改 YAML 缩进出错）时，getConfigLocked 应沿用
// 最近一次可用配置继续服务，而非静默清零回默认值。
func TestGetConfigKeepsLastGoodOnBadEdit(t *testing.T) {
	rt := InitRuntime(t.TempDir())
	defer rt.Shutdown()

	first := rt.GetConfig()
	if first.WebUI.Username == "" {
		t.Fatalf("首次加载应得到有效配置，实际 username=%q", first.WebUI.Username)
	}
	if err := os.WriteFile(rt.configPath, []byte("u115: [broken\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// 强制失效签名，绕开 mtime 粒度对测试确定性的影响
	rt.mu.Lock()
	rt.configSig = [3]int64{}
	cfg := rt.getConfigLocked()
	rt.mu.Unlock()

	if cfg.WebUI.Username != first.WebUI.Username {
		t.Errorf("坏配置应沿用上次可用配置，实际 username=%q（期望 %q）", cfg.WebUI.Username, first.WebUI.Username)
	}
	if cfg.U115.RootFolderID != first.U115.RootFolderID {
		t.Errorf("root_folder_id 不应被清零：实际 %q（期望 %q）", cfg.U115.RootFolderID, first.U115.RootFolderID)
	}
}
