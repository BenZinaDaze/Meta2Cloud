// duplicates.go —— 重复目录合并（历史译名差异导致同媒体多目录的收敛）。
package pipeline

import (
	"fmt"
	"log"
	"regexp"
	"strings"

	"meta2cloud/internal/cloudfile"
)

// DuplicateFolderSource 重复目录分组来源接口（webui 层实现，可为 nil）。
type DuplicateFolderSource interface {
	DuplicateFolderGroups() []DuplicateFolderGroup
}

// DuplicateFolderInfo 重复组内的单个网盘目录。
type DuplicateFolderInfo struct {
	ID           string
	ModifiedTime int64
}

// DuplicateFolderGroup 同一媒体对应的多个网盘目录分组。
// Folders 按目录修改时间升序排列（最旧的在最前）。
type DuplicateFolderGroup struct {
	MediaType string
	TmdbID    int64
	Folders   []DuplicateFolderInfo
}

// ── 重复目录合并 ────────────────────────────────────────────

// mergeDuplicateFoldersPass 把同一媒体因译名差异产生的多个目录合并进钉扎目录。
// 每轮每个媒体只处理一次；dry-run 只记录日志。
func (p *Pipeline) mergeDuplicateFoldersPass() {
	if p.dupSource == nil || p.pinStore == nil {
		return
	}
	if !p.cfg.Pipeline.MergeDuplicatesEnabled() {
		return
	}
	groups := p.dupSource.DuplicateFolderGroups()
	if len(groups) == 0 {
		return
	}
	p.log("\n🔀 检查重复目录...")
	merged := 0
	for _, g := range groups {
		if p.mergeDuplicateGroup(g) {
			merged++
		}
	}
	if merged > 0 {
		p.log(fmt.Sprintf("  共合并 %d 个重复目录", merged), "SUCCESS")
	}
}

// mergeDuplicateGroup 合并一个媒体分组的重复目录，返回是否有目录被回收。
func (p *Pipeline) mergeDuplicateGroup(g DuplicateFolderGroup) bool {
	var pinTitle, pinYear string
	if p.dryRun {
		// dry-run 只读，不落库
		title, year, ok := p.pinStore.GetTitlePin(g.MediaType, g.TmdbID)
		if ok {
			pinTitle, pinYear = title, year
		}
	} else {
		title, year, err := p.pinStore.ResolveTitlePin(g.MediaType, g.TmdbID, "", "")
		if err == nil {
			pinTitle, pinYear = title, year
		}
	}

	// 组内目录按 ID 逐个定位（分类布局下媒体目录不在根目录直下，无法按名列举）
	var located []cloudfile.CloudFile
	for _, f := range g.Folders {
		cf, err := p.provider.GetFile(f.ID)
		if err != nil || cf.Trashed {
			continue
		}
		located = append(located, cf)
	}
	if len(located) == 0 {
		return false
	}

	// 整理名称仍为空（无钉扎记录、无入库历史）：取组内最旧目录的实际名称作为初始名落库
	if pinTitle == "" {
		for _, cf := range located { // located 保持 g.Folders 顺序（最旧在前）
			parsedTitle, parsedYear := splitFolderName(cf.Name)
			if parsedTitle == "" {
				continue
			}
			if p.dryRun {
				pinTitle, pinYear = parsedTitle, parsedYear
			} else if t, y, err := p.pinStore.ResolveTitlePin(g.MediaType, g.TmdbID, parsedTitle, parsedYear); err == nil {
				pinTitle, pinYear = t, y
			}
			if pinTitle != "" {
				p.log(fmt.Sprintf("      📌 整理名称以最旧目录为准：%s", pinnedFolderName(pinTitle, pinYear)), "INFO")
				break
			}
		}
	}
	if pinTitle == "" {
		p.log(fmt.Sprintf("  ⚠ 跳过重复组 tmdb_id=%d：无法确定整理名称（无整理名称记录、入库历史与可用目录）", g.TmdbID), "WARNING")
		return false
	}

	targetName := pinnedFolderName(pinTitle, pinYear)
	var target *cloudfile.CloudFile
	var candidates []cloudfile.CloudFile
	for _, cf := range located {
		if target == nil && cf.Name == targetName {
			cp := cf
			target = &cp
			continue
		}
		candidates = append(candidates, cf)
	}
	if target == nil {
		// 钉扎目录不存在（可能已被手动改名），跳过等待下次整理建立
		p.log(fmt.Sprintf("  ⚠ 跳过重复组 tmdb_id=%d：整理名称对应的目录 %q 不在网盘上（可能已被改名）", g.TmdbID, targetName), "WARNING")
		return false
	}
	recycled := false
	for _, candidate := range candidates {
		if candidate.Trashed {
			continue
		}
		if p.mergeFolderInto(candidate, *target) {
			recycled = true
		}
	}
	return recycled
}

// mergeFolderInto 把 candidate 目录内容搬入 target（镜像子目录结构，同名文件跳过）。
// 腾空（或仅剩元数据）后整个目录进回收站；返回是否完成回收。
func (p *Pipeline) mergeFolderInto(candidate, target cloudfile.CloudFile) bool {
	p.log(fmt.Sprintf("  🔀 合并重复目录：%s → %s", candidate.Name, target.Name))
	moved, skipped := p.moveFolderContents(candidate.ID, target.ID)
	if moved == 0 && skipped == 0 {
		return false
	}
	if p.dryRun {
		p.log(fmt.Sprintf("      [dry-run] 将移动 %d 个文件，跳过 %d 个同名文件", moved, skipped), "INFO")
		return false
	}
	p.log(fmt.Sprintf("      移动 %d 个文件，跳过 %d 个同名文件", moved, skipped), "INFO")
	if p.folderHasContent(candidate.ID) {
		p.log(fmt.Sprintf("      ⚠ 目录内仍有视频/字幕残留，保留目录：%s", candidate.Name), "WARNING")
		return false
	}
	if _, err := p.provider.TrashFile(candidate.ID); err != nil {
		log.Printf("回收重复目录失败 [%s]: %v", candidate.Name, err)
		p.log(fmt.Sprintf("      ❌ 回收目录失败：%v", err), "ERROR")
		return false
	}
	p.log(fmt.Sprintf("      🗑️ 已回收重复目录：%s（可在网盘回收站恢复）", candidate.Name), "SUCCESS")
	// 清理搬迁过程中在 target 下可能产生的空目录
	p.cleanupEmptyFolders(target.ID, target.Name, false)
	return true
}

// moveFolderContents 递归把 src 目录内容搬入 dst（镜像子目录，同名文件跳过）。
func (p *Pipeline) moveFolderContents(srcID, dstID string) (moved, skipped int) {
	items, err := p.provider.ListFiles(srcID, 200)
	if err != nil {
		log.Printf("合并重复目录：列举目录失败 [%s]: %v", srcID, err)
		return 0, 0
	}
	for _, item := range items {
		if item.IsFolder() {
			subDst := dstID
			if !p.dryRun {
				sub, err := cloudfile.GetOrCreateFolder(p.provider, item.Name, dstID)
				if err != nil {
					log.Printf("合并重复目录：创建子目录失败 %q: %v", item.Name, err)
					skipped++
					continue
				}
				subDst = sub.ID
			}
			m, s := p.moveFolderContents(item.ID, subDst)
			moved += m
			skipped += s
			continue
		}
		if p.dryRun {
			moved++
			continue
		}
		if existing, err := p.provider.FindFile(item.Name, dstID); err == nil && existing != nil {
			p.log(fmt.Sprintf("      跳过同名文件：%s", item.Name), "INFO")
			skipped++
			continue
		}
		if _, err := p.provider.MoveFile(item.ID, dstID, item.Name); err != nil {
			log.Printf("合并重复目录：移动文件失败 [%s]: %v", item.Name, err)
			skipped++
			continue
		}
		moved++
	}
	return moved, skipped
}

// folderHasContent 目录（含子目录）内是否还有视频或字幕文件。
func (p *Pipeline) folderHasContent(folderID string) bool {
	items, err := p.provider.ListFiles(folderID, 200)
	if err != nil {
		return true // 出错时保守处理，不回收
	}
	for _, item := range items {
		if item.IsFolder() {
			if p.folderHasContent(item.ID) {
				return true
			}
			continue
		}
		if item.IsVideo() || item.IsSubtitle() {
			return true
		}
	}
	return false
}

// pinnedFolderName 钉扎目录名：标题 (年份)。
func pinnedFolderName(title, year string) string {
	if year != "" {
		return fmt.Sprintf("%s (%s)", title, year)
	}
	return title
}

var folderYearRe = regexp.MustCompile(`^(.*?)\s*\((\d{4})\)$`)

// splitFolderName 把目录名拆为标题与年份（"标题 (2026)" → "标题", "2026"）。
func splitFolderName(name string) (title, year string) {
	name = strings.TrimSpace(name)
	if m := folderYearRe.FindStringSubmatch(name); m != nil {
		return strings.TrimSpace(m[1]), m[2]
	}
	return name, ""
}
