package legacy

type CacheDeleteTarget struct {
	Source   string `json:"source"`
	CacheKey string `json:"cache_key"`
}

type CacheDeleteResult struct {
	Deleted     int      `json:"deleted"`
	Skipped     int      `json:"skipped"`
	Failed      int      `json:"failed"`
	DeletedKeys []string `json:"deleted_keys"`
}
