package splunk

type APIResult struct {
	Results     []map[string]string `json:"results"`
	Fields      []APIField          `json:"fields"`
	Preview     bool                `json:"preview"`
	InitOffset  int                 `json:"init_offset"`
	Messages    []interface{}       `json:"messages"`
	Highlighted interface{}         `json:"highlighted"`
}

type APIField struct {
	Name string `json:"name"`
}
