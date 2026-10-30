// pan115_managed.go —— WebUI 场景的 pan115 Provider 装配（自 webui/runtime.go 下沉）：
// 共享 client getter + 根目录引用解析（带短 TTL 缓存），使 Provider 构造细节
// 不泄漏到运行时容器层。
package storage

import (
	"strings"
	"sync"
	"time"

	"panoply/internal/cloudfile"
	"panoply/internal/u115"
)

// resolveFolderRefPathOnly 仅按「网盘根起算的相对路径/名字」解析目录引用，
// 实现为纯 ListFiles，不含 GetFile 回退。供根目录回调使用：
// 回退 GetFile 会在 Provider.GetFile → 根目录解析 → Provider.GetFile 之间成环，
// 名字解析失败（如接口冷却）时无限递归直至栈溢出。失败时原样返回（兼容旧 ID 配置）。
func resolveFolderRefPathOnly(p cloudfile.Provider, value string) string {
	value = strings.TrimSpace(value)
	if value == "" || p == nil {
		return value
	}
	if id, ok := cloudfile.ResolveFolderPath(p, value); ok {
		return id
	}
	return value
}

// NewPan115ManagedProvider 构造 WebUI 共享的 pan115 Provider。
// clientGetter 每次调用返回当前 u115 客户端单例（配置热更后可能重建，须惰性取值）；
// rootRefs 返回待解析的根目录引用列表（配置值为目录名/相对路径，兼容旧 ID）。
// 根目录解析带 1 分钟 TTL 缓存（随 Provider 重建而失效）：GetFile 每次都要取
// 受管根目录，缓存避免重复解析消耗 QPS。
func NewPan115ManagedProvider(clientGetter func() *u115.Client, rootRefs func() []string) *Pan115Provider {
	p := NewPan115ProviderWithGetter(clientGetter)
	type rootResolveEntry struct {
		id string
		at time.Time
	}
	var (
		rootCacheMu sync.Mutex
		rootCache   = map[string]rootResolveEntry{}
	)
	p.LookupRoots = func() []string {
		// 延迟到 GetFile 时执行，此时 Provider 已初始化完毕。
		refs := rootRefs()
		rootCacheMu.Lock()
		defer rootCacheMu.Unlock()
		now := time.Now()
		ids := make([]string, 0, len(refs))
		for _, ref := range refs {
			if e, ok := rootCache[ref]; ok && now.Sub(e.at) < time.Minute {
				ids = append(ids, e.id)
				continue
			}
			id := resolveFolderRefPathOnly(p, ref)
			rootCache[ref] = rootResolveEntry{id: id, at: now}
			ids = append(ids, id)
		}
		return ids
	}
	return p
}
