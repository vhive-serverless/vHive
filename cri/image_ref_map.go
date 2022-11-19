package cri

var imageRefMap = make(map[string]string)

func Put(key string, value string) {
	imageRefMap[key] = value
}

func Get(key string) string {
	return imageRefMap[key]
}
