package u115

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// TorrentFileItem BT 种子内的单个文件条目。
type TorrentFileItem struct {
	Path   string
	Size   int64
	Wanted int64
	Raw    map[string]any
}

// TorrentInfo BT 种子解析结果。
type TorrentInfo struct {
	FileSize    int64
	TorrentName string
	FileCount   int64
	InfoHash    string
	Files       []TorrentFileItem
	Raw         map[string]any
}

// OfflineTask 云下载任务。
type OfflineTask struct {
	InfoHash     string
	Name         string
	Status       int
	PercentDone  float64
	Size         int64
	AddTime      *int64
	LastUpdate   *int64
	FileID       *string
	DeleteFileID *string
	URL          *string
	WpPathID     *string
	Raw          map[string]any
}

// IsFinished 是否完成。
func (t OfflineTask) IsFinished() bool { return t.Status == 2 }

// IsDownloading 是否下载中。
func (t OfflineTask) IsDownloading() bool { return t.Status == 1 }

// IsFailed 是否失败。
func (t OfflineTask) IsFailed() bool { return t.Status == -1 }

// OfflineTaskList 云下载任务分页结果。
type OfflineTaskList struct {
	Page      int
	PageCount int
	Count     int
	Tasks     []OfflineTask
	Raw       map[string]any
}

// QuotaExpireInfo 配额过期明细。
type QuotaExpireInfo struct {
	Surplus    int64
	ExpireTime int64
	Raw        map[string]any
}

// QuotaPackage 单个配额包。
type QuotaPackage struct {
	Name       string
	Count      int64
	Used       int64
	Surplus    int64
	ExpireInfo []QuotaExpireInfo
	Raw        map[string]any
}

// OfflineQuotaInfo 云下载配额信息。
type OfflineQuotaInfo struct {
	Count    int64
	Used     int64
	Surplus  int64
	Packages []QuotaPackage
	Raw      map[string]any
}

// OfflineUrlAddResult 添加链接任务的逐条结果。
type OfflineUrlAddResult struct {
	State    bool
	Code     int64
	Message  string
	URL      string
	InfoHash *string
	Raw      map[string]any
}

// OfflineClient 115 云下载接口客户端。
type OfflineClient struct {
	client *Client
}

// NewOfflineClient 构造云下载客户端，复用底层 Client 的认证与限流。
func NewOfflineClient(c *Client) *OfflineClient { return &OfflineClient{client: c} }

// ParseTorrent 解析 BT 种子内容。
func (o *OfflineClient) ParseTorrent(torrentSHA1, pickCode string) ([]TorrentInfo, error) {
	payload, err := o.client.request(http.MethodPost, proAPI, "/open/offline/torrent", requestOpts{
		form: url.Values{
			"torrent_sha1": {torrentSHA1},
			"pick_code":    {pickCode},
		},
		authRequired: true,
	})
	if err != nil {
		return nil, err
	}
	var results []TorrentInfo
	for _, item := range jMapSlice(payload, "data") {
		var files []TorrentFileItem
		for _, child := range jMapSlice(item, "torrent_filelist") {
			files = append(files, TorrentFileItem{
				Path:   jString(child, "path"),
				Size:   jInt(child, "size", 0),
				Wanted: jInt(child, "wanted", 0),
				Raw:    child,
			})
		}
		results = append(results, TorrentInfo{
			FileSize:    jInt(item, "file_size", 0),
			TorrentName: jString(item, "torrent_name"),
			FileCount:   jInt(item, "file_count", 0),
			InfoHash:    jString(item, "info_hash"),
			Files:       files,
			Raw:         item,
		})
	}
	return results, nil
}

// GetTaskList 获取云下载任务列表。
func (o *OfflineClient) GetTaskList(page int) (OfflineTaskList, error) {
	payload, err := o.client.request(http.MethodGet, proAPI, "/open/offline/get_task_list", requestOpts{
		params:       url.Values{"page": {strconv.Itoa(page)}},
		authRequired: true,
	})
	if err != nil {
		return OfflineTaskList{}, err
	}
	data := jMap(payload, "data")
	var tasks []OfflineTask
	for _, item := range jMapSlice(data, "tasks") {
		tasks = append(tasks, OfflineTask{
			InfoHash:     jString(item, "info_hash"),
			Name:         jString(item, "name"),
			Status:       int(jInt(item, "status", 0)),
			PercentDone:  jFloat(item, "percentDone", 0),
			Size:         jInt(item, "size", 0),
			AddTime:      jIntPtr(item, "add_time"),
			LastUpdate:   jIntPtr(item, "last_update"),
			FileID:       jStringPtr(item, "file_id"),
			DeleteFileID: jStringPtr(item, "delete_file_id"),
			URL:          jStringPtr(item, "url"),
			WpPathID:     jStringPtr(item, "wp_path_id"),
			Raw:          item,
		})
	}
	count := int(jInt(data, "count", int64(len(tasks))))
	return OfflineTaskList{
		Page:      int(jInt(data, "page", int64(page))),
		PageCount: int(jInt(data, "page_count", 0)),
		Count:     count,
		Tasks:     tasks,
		Raw:       data,
	}, nil
}

// GetAllTasks 自动翻页拉取全部云下载任务。
func (o *OfflineClient) GetAllTasks() ([]OfflineTask, error) {
	first, err := o.GetTaskList(1)
	if err != nil {
		return nil, err
	}
	tasks := make([]OfflineTask, 0, first.Count)
	tasks = append(tasks, first.Tasks...)
	for page := 2; page <= max(first.PageCount, 1); page++ {
		next, err := o.GetTaskList(page)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, next.Tasks...)
	}
	return tasks, nil
}

// GetQuotaInfo 获取云下载配额信息。
func (o *OfflineClient) GetQuotaInfo() (OfflineQuotaInfo, error) {
	payload, err := o.client.request(http.MethodGet, proAPI, "/open/offline/get_quota_info", requestOpts{
		authRequired: true,
	})
	if err != nil {
		return OfflineQuotaInfo{}, err
	}
	data := jMap(payload, "data")
	var packages []QuotaPackage
	for _, item := range jMapSlice(data, "package") {
		var expireInfo []QuotaExpireInfo
		for _, exp := range jMapSlice(item, "expire_info") {
			expireInfo = append(expireInfo, QuotaExpireInfo{
				Surplus:    jInt(exp, "surplus", 0),
				ExpireTime: jInt(exp, "expire_time", 0),
				Raw:        exp,
			})
		}
		packages = append(packages, QuotaPackage{
			Name:       jString(item, "name"),
			Count:      jInt(item, "count", 0),
			Used:       jInt(item, "used", 0),
			Surplus:    jInt(item, "surplus", 0),
			ExpireInfo: expireInfo,
			Raw:        item,
		})
	}
	return OfflineQuotaInfo{
		Count:    jInt(data, "count", 0),
		Used:     jInt(data, "used", 0),
		Surplus:  jInt(data, "surplus", 0),
		Packages: packages,
		Raw:      data,
	}, nil
}

// AddTaskURLs 添加云下载链接任务；支持多个链接。
// wpPathID 传空串表示不指定。
func (o *OfflineClient) AddTaskURLs(urls []string, wpPathID string) ([]OfflineUrlAddResult, error) {
	urlText := strings.Join(urls, "\n")
	form := url.Values{"urls": {urlText}}
	if wpPathID != "" {
		form.Set("wp_path_id", wpPathID)
	}
	payload, err := o.client.request(http.MethodPost, proAPI, "/open/offline/add_task_urls", requestOpts{
		form:         form,
		authRequired: true,
	})
	if err != nil {
		return nil, err
	}
	var results []OfflineUrlAddResult
	for _, item := range jMapSlice(payload, "data") {
		results = append(results, OfflineUrlAddResult{
			State:    jBool(item, "state", false),
			Code:     jInt(item, "code", 0),
			Message:  jString(item, "message"),
			URL:      jString(item, "url"),
			InfoHash: jStringPtr(item, "info_hash"),
			Raw:      item,
		})
	}
	return results, nil
}

// AddTaskBTArgs 添加 BT 云下载任务参数。
type AddTaskBTArgs struct {
	InfoHash    string
	Wanted      []int // 选中的文件序号，空表示全部
	SavePath    string
	TorrentSHA1 string
	PickCode    string
	WpPathID    string
}

// AddTaskBT 添加 BT 云下载任务。
func (o *OfflineClient) AddTaskBT(args AddTaskBTArgs) error {
	wanted := make([]string, 0, len(args.Wanted))
	for _, i := range args.Wanted {
		wanted = append(wanted, strconv.Itoa(i))
	}
	form := url.Values{
		"info_hash":    {args.InfoHash},
		"wanted":       {strings.Join(wanted, ",")},
		"save_path":    {args.SavePath},
		"torrent_sha1": {args.TorrentSHA1},
		"pick_code":    {args.PickCode},
	}
	if args.WpPathID != "" {
		form.Set("wp_path_id", args.WpPathID)
	}
	_, err := o.client.request(http.MethodPost, proAPI, "/open/offline/add_task_bt", requestOpts{
		form:         form,
		authRequired: true,
	})
	return err
}

// DelTask 删除单个云下载任务。
func (o *OfflineClient) DelTask(infoHash string, delSourceFile int) error {
	_, err := o.client.request(http.MethodPost, proAPI, "/open/offline/del_task", requestOpts{
		form: url.Values{
			"info_hash":       {infoHash},
			"del_source_file": {strconv.Itoa(delSourceFile)},
		},
		authRequired: true,
	})
	return err
}

// 云下载任务清空 flag 枚举。
const (
	ClearFlagDone         = 0 // 清空已完成
	ClearFlagAll          = 1 // 清空全部
	ClearFlagFailed       = 2 // 清空失败
	ClearFlagRunning      = 3 // 清空进行中
	ClearFlagDoneWithFile = 4 // 清空已完成任务并清空对应源文件
	ClearFlagAllWithFile  = 5 // 清空全部任务并清空对应源文件
)

// ClearTasks 按类型清空云下载任务。
func (o *OfflineClient) ClearTasks(flag int) error {
	_, err := o.client.request(http.MethodPost, proAPI, "/open/offline/clear_task", requestOpts{
		form:         url.Values{"flag": {strconv.Itoa(flag)}},
		authRequired: true,
	})
	return err
}
