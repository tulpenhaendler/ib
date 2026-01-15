package storage

import "encoding/json"

func serializeTags(tags map[string]string) (string, error) {
	data, err := json.Marshal(tags)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func deserializeTags(data string) (map[string]string, error) {
	var tags map[string]string
	err := json.Unmarshal([]byte(data), &tags)
	return tags, err
}
