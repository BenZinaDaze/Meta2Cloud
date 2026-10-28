// token_perm_test.go —— token 文件权限：写入即收紧、读取自愈历史宽松权限。
package trakt

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTokenFilePermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trakt-token.json")
	if err := os.WriteFile(path, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := SaveToken(Token{AccessToken: "a", RefreshToken: "r"}, path); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o600 {
		t.Errorf("SaveToken 后权限 = %o, want 600", st.Mode().Perm())
	}

	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := LoadToken(path); err != nil || !ok {
		t.Fatalf("LoadToken ok=%v err=%v", ok, err)
	}
	if st, err := os.Stat(path); err != nil || st.Mode().Perm() != 0o600 {
		t.Errorf("LoadToken 后权限应自愈为 600, got %o (err %v)", st.Mode().Perm(), err)
	}
}
